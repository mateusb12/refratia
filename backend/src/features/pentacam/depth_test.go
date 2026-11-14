package pentacam

import "testing"

func TestDepthResolveSelectsExternalDepthFromReadCandidates(t *testing.T) {
	// ext split 3.3/3.5/3.9, internal read, vertex split 534/538/634.
	got := depthResolve(
		map[string]float64{"acd_internal_mm": 3.36},
		map[string]map[float64]int{
			"acd_external_mm":  {3.3: 8, 3.5: 6, 3.9: 25},
			"vertex_normal_um": {534: 5, 538: 4, 634: 6},
		})
	if got["acd_external_mm"] != 3.9 {
		t.Fatalf("ext = %v", got)
	}
	if _, ok := got["vertex_normal_um"]; ok {
		t.Fatalf("vertex 534 vs 538 are both consistent, must stay open: %v", got)
	}
}

func TestDepthResolveExcludesGlyphFlipInPachymetry(t *testing.T) {
	got := depthResolve(
		map[string]float64{"acd_internal_mm": 2.71, "acd_external_mm": 3.25},
		map[string]map[float64]int{"vertex_normal_um": {545: 4, 645: 6}})
	if got["vertex_normal_um"] != 545 {
		t.Fatalf("got %v", got)
	}
}

func TestDepthResolveNeverInventsOrOverrides(t *testing.T) {
	if got := depthResolve(map[string]float64{"acd_internal_mm": 3.36},
		map[string]map[float64]int{"acd_external_mm": {3.3: 8}, "vertex_normal_um": {534: 5}}); len(got) != 0 {
		t.Fatalf("no consistent candidate, got %v", got)
	}
	if got := depthResolve(map[string]float64{"acd_internal_mm": 3.36},
		map[string]map[float64]int{"acd_external_mm": {3.9: 1}, "vertex_normal_um": {534: 5}}); len(got) != 0 {
		t.Fatalf("single read is not enough, got %v", got)
	}
	if got := depthResolve(map[string]float64{"acd_internal_mm": 3.36, "acd_external_mm": 3.9, "vertex_normal_um": 534}, nil); got != nil {
		t.Fatalf("nothing open, got %v", got)
	}
}
