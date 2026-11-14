package pentacam

import "math"

// The external anterior chamber depth is measured from the corneal
// epithelium, the internal one from the endothelium, so at the corneal vertex
//
//	external ACD = internal ACD + vertex pachymetry / 1000
//
// holds within the printed rounding: the two depths are printed with 2
// decimals (each within 0.005 of its true value) and the pachymetry in whole
// micrometres (within 0.5 um = 0.0005 mm).
const depthTol = 0.005 + 0.005 + 0.0005 + 1e-6

// depthKeys: external depth, internal depth, vertex pachymetry.
var depthKeys = []string{"acd_external_mm", "acd_internal_mm", "vertex_normal_um"}

func depthHolds(ext, in, vertexUm float64) bool {
	return math.Abs(ext-in-vertexUm/1000) <= depthTol
}

// depthResolve fills the depth cells whose reading is missing or split. It
// never computes a value: each cell can only take a value that OCR actually
// read at least twice (the base rule of readValueBox). A cell is answered only
// when every assignment of the candidate readings that satisfies the identity
// agrees on it; accepted readings are never overridden.
func depthResolve(accepted map[string]float64, votes map[string]map[float64]int) map[string]float64 {
	options := make([][]float64, len(depthKeys))
	open := false
	for i, key := range depthKeys {
		if a, ok := accepted[key]; ok {
			options[i] = []float64{a}
			continue
		}
		open = true
		for v, n := range votes[key] {
			if n >= 2 {
				options[i] = append(options[i], v)
			}
		}
		if len(options[i]) == 0 {
			return nil
		}
	}
	if !open {
		return nil
	}
	var solutions [][3]float64
	for _, e := range options[0] {
		for _, in := range options[1] {
			for _, p := range options[2] {
				if depthHolds(e, in, p) {
					solutions = append(solutions, [3]float64{e, in, p})
				}
			}
		}
	}
	if len(solutions) == 0 {
		return nil
	}
	out := map[string]float64{}
	for i, key := range depthKeys {
		if _, ok := accepted[key]; ok {
			continue
		}
		agree := true
		for _, s := range solutions[1:] {
			agree = agree && s[i] == solutions[0][i]
		}
		if agree {
			out[key] = solutions[0][i]
		}
	}
	return out
}
