package pentacam

import (
	"path/filepath"
	"strings"
	"testing"
)

// absentOr marks an expectation "this value or absent, never another".
type absentOr float64

// checkRealIndices extracts every real Pentacam document (gitignored
// frontend/data; absent -> skip) and compares one topometric index per
// folder/eye. Values are the printed indices (verified ground truth).
func checkRealIndices(t *testing.T, field string, want map[string]map[string]any) {
	files, _ := filepath.Glob("../../../../../frontend/data/*/PENTACAM*.pdf")
	if len(files) == 0 {
		t.Skip("no real Pentacam documents available")
	}
	for _, f := range files {
		folder := filepath.Base(filepath.Dir(f))
		name := strings.ToUpper(filepath.Base(f))
		eye := "OS"
		if strings.Contains(name, " OD ") || strings.Contains(name, "__OD__") {
			eye = "OD"
		}
		exp, ok := want[folder][eye]
		if !ok {
			continue
		}
		exam := cachedExtractPDF(t, f)
		section, _ := exam["topometric_indices_8mm"].(map[string]any)
		got, present := section[field].(float64)
		switch e := exp.(type) {
		case absentOr:
			if present && got != float64(e) {
				t.Errorf("%s %s: %s %v, want %v or absent", folder, eye, field, got, float64(e))
			}
		case float64:
			if !present || got != e {
				t.Errorf("%s %s: %s %v (present=%v), want %v", folder, eye, field, got, present, e)
			}
		}
	}
}

func TestIsvFromRealDocuments(t *testing.T) {
	checkRealIndices(t, "isv", map[string]map[string]any{
		"francisco":    {"OD": 9.0, "OS": 14.0},
		"maria-gorete": {"OD": 14.0, "OS": 12.0},
		"rolmey":       {"OD": 10.0},
		"creso-aversa": {"OD": 15.0, "OS": absentOr(17)},
		"ademildo":     {"OD": 23.0, "OS": absentOr(18)},
	})
}
