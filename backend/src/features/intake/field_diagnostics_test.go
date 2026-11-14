package intake

import "testing"

func TestValidateTouchedEyesDropsInvalidAndRecords(t *testing.T) {
	analysis := map[string]any{"exams": map[string]any{}}
	before := snapshotEyes(analysis)
	analysis["exams"].(map[string]any)["refractometry"] = map[string]any{"eyes": map[string]any{
		"OD": map[string]any{"axis_deg": 188.0, "sphere_d": 5.0},
	}}
	file := uploadedFile{}
	file.Metadata.SHA256 = "abcdef0123456789"
	validateTouchedEyes(analysis, file, before)

	od := analysis["exams"].(map[string]any)["refractometry"].(map[string]any)["eyes"].(map[string]any)["OD"].(map[string]any)
	if _, ok := od["axis_deg"]; ok {
		t.Fatal("invalid axis must be removed")
	}
	if od["sphere_d"] != 5.0 {
		t.Fatal("valid sphere must be kept")
	}
	var unreadable int
	for _, raw := range analysis["extraction_diagnostics"].([]any) {
		diagnostic := raw.(map[string]any)
		if diagnostic["field"] == "axis_deg" && diagnostic["status"] == "unreadable" && diagnostic["stage"] == "validation" {
			unreadable++
		}
	}
	if unreadable != 1 {
		t.Fatalf("want one unreadable axis diagnostic, got %d", unreadable)
	}
}

func TestRecordIdentityNotReadLeavesEntryAndDiagnostics(t *testing.T) {
	analysis := map[string]any{}
	file := uploadedFile{}
	file.Metadata.Filename, file.Metadata.SHA256 = "f.pdf", "abcd"
	recordIdentityNotRead(analysis, file, "OD")
	entries := analysis["verificacao_identidade"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["date_order"] != "MDY" {
		t.Fatalf("identity entry missing: %v", entries)
	}
	if len(analysis["extraction_diagnostics"].([]any)) != 3 {
		t.Fatal("one diagnostic per identity field expected")
	}
}
