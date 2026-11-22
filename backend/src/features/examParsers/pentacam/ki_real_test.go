package pentacam

import "testing"

// KI printed on colour-filled cells (ademildo: white digits on red, label read
// "Kt") and on plain cells, from verified ground truth.
func TestKiFromRealDocuments(t *testing.T) {
	checkRealIndices(t, "ki", map[string]map[string]any{
		"ademildo":     {"OD": 1.08, "OS": 1.07},
		"creso-aversa": {"OD": 1.04, "OS": 1.04},
		"francisco":    {"OD": 1.02, "OS": 1.02},
		"maria-gorete": {"OD": 0.97, "OS": 0.99},
		"rolmey":       {"OD": 1.01, "OS": 1.01},
	})
}
