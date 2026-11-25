package intake

import (
	"os"
	"path/filepath"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func TestParsePrintedTimestampUsesDeviceOrder(t *testing.T) {
	cases := []struct{ raw, order, want string }{
		{"11/06/2026 09:56:02", "MDY", "2026-11-06 09:56:02"},  // Pentacam
		{"06/11/26 - 10:17", "DMY", "2026-11-06 10:17"},        // EyeSuite
		{"06/NOV/2026 09:55:59", "DMY", "2026-11-06 09:55:59"}, // NIDEK
		{"LS900 06/11/2026 - 1", "DMY", "2026-11-06"},
	}
	for _, c := range cases {
		if got, ok := parsePrintedTimestamp(c.raw, c.order); !ok || got != c.want {
			t.Errorf("%q %s = %q %v, want %q", c.raw, c.order, got, ok, c.want)
		}
	}
	if _, ok := parsePrintedTimestamp("31/02/2026", "DMY"); ok {
		t.Error("impossible date accepted")
	}
}

func TestMergeClaudeResultMapsValidatesAndKeepsDash(t *testing.T) {
	analysis := map[string]any{}
	file := UploadedFile{Metadata: FileMetadata{Filename: "PENTACAM_OD.pdf", SHA256: "abc", Eye: "OD"}}
	_, err := mergeClaudeResult(analysis, file, "pentacam_corneal_tomography", claudeFileResult{
		ExamType:        "pentacam_corneal_tomography",
		PrintedExamDate: ptr("11/06/2026 09:56:02"),
		Measurements: []claudeMeasurement{
			{Eye: "OD", Field: "k1_d", Status: "read", Value: ptr(40.9), Page: 3},
			{Eye: "OD", Field: "pachy_thinnest_um", Status: "read", Value: ptr(5840.0), Page: 3}, // out of range
			{Eye: "OD", Field: "tkc", Status: "read", Text: ptr("-"), Page: 8},
			{Eye: "OD", Field: "isv", Status: "unreadable", Page: 8},
			{Eye: "OS", Field: "k2_d", Status: "read", Value: ptr(42.1), Page: 3}, // wrong eye for an OD file
			{Eye: "OD", Field: "invented_index", Status: "read", Value: ptr(1.0), Page: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	eyes := analysis["exams"].(map[string]any)["pentacam_corneal_tomography"].(map[string]any)["eyes"].(map[string]any)
	od := eyes["OD"].(map[string]any)
	if od["anterior_cornea"].(map[string]any)["k1_d"] != 40.9 {
		t.Errorf("k1_d not mapped: %v", od)
	}
	if pachy, _ := od["pachymetry"].(map[string]any); pachy != nil && pachy["thinnest_um"] != nil {
		t.Error("implausible pachymetry kept")
	}
	if _, ok := eyes["OS"]; ok {
		t.Error("OS value from an OD file kept")
	}
	status := map[string]string{}
	for _, raw := range analysis["extraction_diagnostics"].([]any) {
		d := raw.(map[string]any)
		status[d["field"].(string)] = d["status"].(string) + "/" + d["stage"].(string)
	}
	for field, want := range map[string]string{"k1_d": "valid/validation", "pachy_thinnest_um": "unreadable/validation", "tkc": "not_applicable/recognition", "isv": "unreadable/recognition", "km_d": "missing/mapping"} {
		if status[field] != want {
			t.Errorf("%s diagnostic = %q, want %q", field, status[field], want)
		}
	}
	if !pentacamTKCPrintedDash(analysis, "abc", "OD") {
		t.Error("TKC dash not recorded with the OCR convention")
	}
	entry := analysis["verificacao_identidade"].([]any)[0].(map[string]any)
	if entry["timestamp_lido"] != "2026-11-06 09:56:02" || entry["date_order"] != "MDY" {
		t.Errorf("identity entry = %v", entry)
	}
	if _, err := mergeClaudeResult(map[string]any{}, file, "pentacam_corneal_tomography", claudeFileResult{ExamType: "iol_calculation"}); err == nil {
		t.Error("exam type mismatch accepted")
	}
}

func TestClaudeVisionRejectsUnapprovedDocuments(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "_anon-review", "p"), 0o755)
	os.MkdirAll(filepath.Join(dir, "p", "anonymized"), 0o755)
	os.WriteFile(filepath.Join(dir, "p", "anonymized", "A_OD.pdf"), []byte("anonymized"), 0o644)
	os.WriteFile(filepath.Join(dir, "p", "anonymized", "B_OD.pdf"), []byte("needs review"), 0o644)
	os.WriteFile(filepath.Join(dir, "_anon-review", "p", "report.json"), []byte(`[{"output":"x/A_OD.pdf","status":"OK"},{"output":"x/B_OD.pdf","status":"NEEDS_REVIEW"}]`), 0o644)
	fixture := filepath.Join(dir, "fixture.json")
	os.WriteFile(fixture, []byte(`{"files":{}}`), 0o644)
	t.Setenv("CLAUDE_VISION_APPROVED_DIR", dir)
	t.Setenv("CLAUDE_VISION_FIXTURE", fixture)

	for _, data := range []string{"needs review", "original identifiable"} {
		file := UploadedFile{Data: []byte(data), Metadata: FileMetadata{Filename: "x.pdf", SHA256: sha256Hex([]byte(data)), ExamType: "pentacam_corneal_tomography", Eye: "OD"}}
		if _, err := extractPatientClaude(t.Context(), []UploadedFile{file}, LocalIdentity{FullName: "A B", BirthDate: "1970-01-01"}); err == nil {
			t.Errorf("%q accepted", data)
		}
	}
	if approved, _ := approvedAnonymizedFiles(); len(approved) != 1 {
		t.Errorf("approved = %v", approved)
	}
}
