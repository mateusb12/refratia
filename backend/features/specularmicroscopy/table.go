package specularmicroscopy

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// tableRow is one row of the NIDEK result table. Rows are found by the text of
// their label (the table order differs between exports), the value is the
// right-most integer of the row (the unit column sits between label and value).
type tableRow struct {
	key   string // app field name
	label *regexp.Regexp
	min   float64 // plausible range from _field_spec.json
	max   float64
}

func row(key, pattern string, min, max float64) tableRow {
	return tableRow{key: key, label: regexp.MustCompile(`(?i)` + pattern), min: min, max: max}
}

var tableRows = []tableRow{
	row("number_of_cells", `number\s*of\s*cells|\(\s*NUM\s*\)`, 1, 1000),
	row("average_cell_area_um2", `average\s*area|\(\s*AVG\s*\)`, 100, 3000),
	row("standard_deviation_um2", `standard\s*dev|\(\s*SD\s*\)`, 1, 2000),
	row("coefficient_of_variation_percent", `coefficient|\(\s*CV\s*\)`, 0, 100),
	row("maximum_cell_area_um2", `max\s*area|\(\s*MAX\s*\)`, 100, 10000),
	row("minimum_cell_area_um2", `min\s*area|\(\s*MIN\s*\)`, 10, 3000),
	row("hexagonal_cells_percent", `hexagonal|\(\s*HEX\s*\)`, 0, 100),
	row("corneal_thickness_um", `corneal\s*thickness|\(\s*CT\s*\)`, 300, 750),
}

var rowInteger = regexp.MustCompile(`\b\d{1,5}\b`)

// parseTableRows reads the remaining result-table fields of one panel from its
// OCR lines. A value must be an integer inside the field's plausible range; the
// physically linked fields are cross-checked (CV = SD/AVG, density ~ 1e6/AVG)
// and the inconsistent ones are dropped rather than guessed.
func parseTableRows(density float64, haveDensity bool, texts ...string) map[string]float64 {
	// Every reading votes for the value it finds in each row; a value needs
	// two agreeing readings, or to be the only value any reading produced; a
	// tie is no value.
	votes := map[string]map[float64]int{}
	for _, text := range texts {
		seen := map[string]bool{}
		for _, line := range strings.Split(text, "\n") {
			for _, r := range tableRows {
				loc := r.label.FindStringIndex(line)
				if loc == nil || seen[r.key] {
					continue
				}
				numbers := rowInteger.FindAllString(line[loc[1]:], -1)
				if len(numbers) == 0 {
					continue
				}
				value, err := strconv.ParseFloat(numbers[len(numbers)-1], 64)
				if err != nil || value < r.min || value > r.max {
					continue
				}
				seen[r.key] = true
				if votes[r.key] == nil {
					votes[r.key] = map[float64]int{}
				}
				votes[r.key][value]++
			}
		}
	}
	out := map[string]float64{}
	for key, counts := range votes {
		best, bestN, tie := 0.0, 0, false
		for value, n := range counts {
			switch {
			case n > bestN:
				best, bestN, tie = value, n, false
			case n == bestN:
				tie = true
			}
		}
		if tie || (bestN < 2 && len(counts) > 1) {
			continue
		}
		out[key] = best
	}

	avg, hasAvg := out["average_cell_area_um2"]
	if hasAvg && haveDensity && math.Abs(density*avg/1e6-1) > 0.15 {
		delete(out, "average_cell_area_um2")
		hasAvg = false
	}
	sd, hasSD := out["standard_deviation_um2"]
	cv, hasCV := out["coefficient_of_variation_percent"]
	if hasAvg && hasSD && hasCV && math.Abs(sd/avg*100-cv) > 3 {
		delete(out, "standard_deviation_um2")
		delete(out, "coefficient_of_variation_percent")
	}
	return out
}
