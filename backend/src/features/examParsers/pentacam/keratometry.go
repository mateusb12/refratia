package pentacam

import "math"

// The Córnea Ant. box prints K1, K2, Km and the astigmatism, each rounded to
// 0.1 D from the same measurement, so two identities always hold within the
// rounding of the printed cells:
//
//	Km = (K1 + K2) / 2        astigmatism = K2 - K1
//
// A reading that breaks them is a misread glyph (1 vs 7, a lost "0."), not a
// property of the eye.
//
// Tolerances follow from the rounding (each cell is within 0.05 of its true
// value): Km vs the printed mean can differ by 0.05+0.05, the astigmatism vs
// the printed K2-K1 by 0.05+0.05+0.05. The epsilon absorbs float error.
const (
	kmTol    = 0.1 + 1e-6
	astigTol = 0.15 + 1e-6
)

var keratoKeys = []string{"k1_d", "k2_d", "km_d", "astigmatism_d"}

// keratoChecks evaluates the two identities on the values that are present.
// checked[i] says whether identity i could be evaluated, holds[i] whether it
// holds (only meaningful when checked).
func keratoChecks(v map[string]float64) (checked, holds [2]bool) {
	k1, ok1 := v["k1_d"]
	k2, ok2 := v["k2_d"]
	if km, ok := v["km_d"]; ok && ok1 && ok2 {
		checked[0], holds[0] = true, math.Abs(km-(k1+k2)/2) <= kmTol
	}
	if a, ok := v["astigmatism_d"]; ok && ok1 && ok2 {
		checked[1], holds[1] = true, math.Abs((k2-k1)-a) <= astigTol
	}
	return
}

// keratoDrop returns the keys that cannot be trusted. When one identity holds
// and the other fails, the values of the holding identity agree with each
// other and the key outside it is the odd one out; otherwise the failing
// identity does not say which of its values is wrong, so all of them go.
func keratoDrop(v map[string]float64) []string {
	checked, holds := keratoChecks(v)
	failed := func(i int) bool { return checked[i] && !holds[i] }
	switch {
	case failed(0) && failed(1):
		return keratoKeys
	case failed(0) && checked[1]:
		return []string{"km_d"}
	case failed(1) && checked[0]:
		return []string{"astigmatism_d"}
	case failed(0):
		return []string{"k1_d", "k2_d", "km_d"}
	case failed(1):
		return []string{"k1_d", "k2_d", "astigmatism_d"}
	}
	return nil
}

// keratoResolve looks for the one assignment of the cells' candidate readings
// (every value any OCR variant produced) that satisfies the identities, for
// keys whose accepted reading is missing or breaks them. It answers only when
// that assignment is unique and well supported: both identities hold, or one
// holds and every reading it changes was seen at least 3 times. Accepted
// readings that are consistent are never overridden.
func keratoResolve(accepted map[string]float64, votes map[string]map[float64]int) map[string]float64 {
	violated := len(keratoDrop(accepted)) > 0
	open := false
	for _, key := range keratoKeys {
		if _, ok := accepted[key]; !ok && len(votes[key]) > 0 {
			open = true
		}
	}
	if !violated && !open {
		return nil
	}
	var solutions []map[string]float64
	var walk func(i int, cur map[string]float64)
	walk = func(i int, cur map[string]float64) {
		if i == len(keratoKeys) {
			if len(keratoDrop(cur)) > 0 {
				return
			}
			checked, _ := keratoChecks(cur)
			count, minVotes := 0, 1<<30
			for _, c := range checked {
				if c {
					count++
				}
			}
			for key, val := range cur {
				if a, ok := accepted[key]; !ok || a != val {
					minVotes = min(minVotes, votes[key][val])
				}
			}
			if count == 2 || (count == 1 && minVotes >= 3) {
				solutions = append(solutions, cloneValues(cur))
			}
			return
		}
		key := keratoKeys[i]
		if a, ok := accepted[key]; ok && !violated {
			cur[key] = a
			walk(i+1, cur)
			delete(cur, key)
			return
		}
		if len(votes[key]) == 0 {
			if a, ok := accepted[key]; ok {
				cur[key] = a
			}
			walk(i+1, cur)
			delete(cur, key)
			return
		}
		for val := range votes[key] {
			cur[key] = val
			walk(i+1, cur)
		}
		delete(cur, key)
	}
	walk(0, map[string]float64{})
	if len(solutions) != 1 {
		return nil
	}
	return solutions[0]
}

func cloneValues(m map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
