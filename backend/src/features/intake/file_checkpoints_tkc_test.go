package intake

import (
	"reflect"
	"strings"
	"testing"

	"refratia/backend/src/features/fileIdentification/extractionfields"
)

func tkcDiagnostic(sha, eye, status, stage string) map[string]any {
	return map[string]any{
		"file_sha256": sha, "exam_type": "pentacam_corneal_tomography", "eye": eye,
		"field": "tkc", "status": status, "stage": stage,
	}
}

// A printed TKC dash counts as complete only when this document's extraction
// recorded it as observed not applicable for this eye; anything else still
// blocks reuse.
func TestPentacamCheckpointTKC(t *testing.T) {
	sha := strings.Repeat("f1", 32)
	dash := tkcDiagnostic(sha, "OD", "not_applicable", "recognition")
	withoutISV := pentacamEye(0, nil)
	delete(withoutISV["topometric_indices_8mm"].(map[string]any), "isv")
	nullTKC := pentacamEye(0, nil)
	nullTKC["topometric_indices_8mm"].(map[string]any)["tkc"] = nil
	for _, test := range []struct {
		name        string
		eye         map[string]any
		diagnostics []any
		complete    bool
	}{
		{"read value", pentacamEye(0, "Normal"), nil, true},
		{"printed dash recorded for this document and eye", pentacamEye(0, nil), []any{dash}, true},
		{"bare null", nullTKC, nil, false},
		{"absent, nothing recorded", pentacamEye(0, nil), nil, false},
		{"unreadable", pentacamEye(0, nil), []any{tkcDiagnostic(sha, "OD", "unreadable", "validation")}, false},
		{"missing", pentacamEye(0, nil), []any{tkcDiagnostic(sha, "OD", "missing", "mapping")}, false},
		{"dash recorded for another document", pentacamEye(0, nil), []any{tkcDiagnostic(strings.Repeat("f2", 32), "OD", "not_applicable", "recognition")}, false},
		{"dash recorded for the other eye", pentacamEye(0, nil), []any{tkcDiagnostic(sha, "OS", "not_applicable", "recognition")}, false},
		{"screen absent (region not_applicable)", pentacamEye(0, nil), []any{tkcDiagnostic(sha, "OD", "not_applicable", "region")}, false},
		{"dash recorded but ISV missing", withoutISV, []any{dash}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			analysis := map[string]any{"exams": map[string]any{"pentacam_corneal_tomography": map[string]any{
				"eyes": map[string]any{"OD": test.eye},
			}}}
			if test.diagnostics != nil {
				analysis["extraction_diagnostics"] = test.diagnostics
			}
			if got := fileCheckpointResultIsComplete(analysis, "pentacam_corneal_tomography", "OD", sha); got != test.complete {
				t.Fatalf("complete = %v; want %v", got, test.complete)
			}
		})
	}
}

// pentacamHintedDoc runs the Pentacam exam writer with the extractor's hints,
// so the diagnostics come from the real validation path.
func pentacamHintedDoc(name, sha, eye string, payload map[string]any, hints ...extractionfields.Hint) boundaryDoc {
	return boundaryDoc{
		file: pentacamFile(name, sha, eye), examType: "pentacam_corneal_tomography", eye: eye,
		write: func(analysis map[string]any, file UploadedFile) error {
			defer validateTouchedEyes(analysis, file, snapshotEyes(analysis))
			mergeLocalPentacamExam(analysis, eye, file.Metadata.Filename, payload)
			setFieldHints(analysis, "pentacam_corneal_tomography", eye, hints)
			return nil
		},
	}
}

// End to end: a dash eye is reused with fresh parity; an unreadable TKC or a
// missing ISV is extracted again.
func TestPentacamCheckpointTKCReuse(t *testing.T) {
	printedDash := extractionfields.Hint{Spec: "tkc", Status: "not_applicable", Stage: "recognition", Strategy: "printed_dash"}
	unreadable := extractionfields.Hint{Spec: "tkc", Status: "unreadable", Stage: "recognition"}
	conflicting := extractionfields.Hint{Spec: "tkc", Status: "conflicting", Stage: "recognition", Strategy: "focused_tkc"}
	classified := extractionfields.Hint{Spec: "tkc", Status: "valid", Stage: "recognition", Strategy: "focused_tkc+label_anchor"}
	withoutISV := pentacamEye(0, nil)
	delete(withoutISV["topometric_indices_8mm"].(map[string]any), "isv")
	for _, test := range []struct {
		name string
		doc  boundaryDoc
		want string
	}{
		{"printed dash", pentacamHintedDoc("od.pdf", "f1", "OD", pentacamEye(0, nil), printedDash), "hit"},
		{"unreadable TKC", pentacamHintedDoc("od.pdf", "f1", "OD", pentacamEye(0, nil), unreadable), "not_found"},
		{"conflicting TKC", pentacamHintedDoc("od.pdf", "f1", "OD", pentacamEye(0, nil), conflicting), "not_found"},
		{"classified TKC", pentacamHintedDoc("od.pdf", "f1", "OD", pentacamEye(0, "KC2-3"), classified), "hit"},
		{"no TKC hint", pentacamHintedDoc("od.pdf", "f1", "OD", pentacamEye(0, nil)), "not_found"},
		{"printed dash, ISV missing", pentacamHintedDoc("od.pdf", "f1", "OD", withoutISV, printedDash), "not_found"},
	} {
		t.Run(test.name, func(t *testing.T) {
			storage := newMemoryStorage()
			runBoundaryBatch(t, storage, false, []boundaryDoc{test.doc})
			batch := []boundaryDoc{pentacamDoc("os.pdf", "f3", "OS", 1), test.doc}
			cached, outcomes := runBoundaryBatch(t, storage, false, batch)
			fresh, _ := runBoundaryBatch(t, newMemoryStorage(), true, batch)
			if outcomes[1] != test.want {
				t.Fatalf("outcomes %v; want %s", outcomes, test.want)
			}
			if !reflect.DeepEqual(boundaryJSON(cached), boundaryJSON(fresh)) {
				t.Fatalf("cached differs from forced-fresh (%v)", outcomes)
			}
		})
	}
}
