package specularmicroscopy

import "testing"

func TestParseTableRows(t *testing.T) {
	text := "Number of Cells (NUM) cells 239\n" +
		"Cell Density (CD) cells/mm2 2868\n" +
		"Average Area (AVG) um2 335\n" +
		"Standard Deviation (SD) um2 98\n" +
		"Coefficient of Variation (CV) % 29\n" +
		"Max Area (MAX) um2 899\n" +
		"Min Area (MIN) um2 133\n" +
		"Hexagonal Cells(HEX) % 68\n" +
		"Corneal Thickness (CT) um 534"
	got := parseTableRows(2868, true, text)
	want := map[string]float64{"number_of_cells": 239, "average_cell_area_um2": 335, "standard_deviation_um2": 98,
		"coefficient_of_variation_percent": 29, "maximum_cell_area_um2": 899, "minimum_cell_area_um2": 133,
		"hexagonal_cells_percent": 68, "corneal_thickness_um": 534}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v want %v", k, got[k], v)
		}
	}
}

func TestParseTableRowsDropsInconsistentValues(t *testing.T) {
	// density 2868 implies an average area near 350: 3350 is a lost-dot misread.
	got := parseTableRows(2868, true, "Average Area (AVG) um2 3350\nStandard Deviation (SD) um2 98")
	if _, ok := got["average_cell_area_um2"]; ok {
		t.Fatal("inconsistent average area must be dropped")
	}
	// out of plausible range is dropped
	if _, ok := parseTableRows(2868, true, "Corneal Thickness (CT) um 5340")["corneal_thickness_um"]; ok {
		t.Fatal("out-of-range thickness must be dropped")
	}
}

// Readings vote; a value needs two agreeing readings, a lone stray one loses.
func TestParseTableRowsMergesPasses(t *testing.T) {
	n := "Number of Cells (NUM) cells 239"
	ct := "Corneal Thickness (CT) um 534"
	got := parseTableRows(2868, true, n+"\n"+ct, "garbage\n"+ct, n+"\nCorneal Thickness (CT) um 4")
	if got["number_of_cells"] != 239 || got["corneal_thickness_um"] != 534 {
		t.Fatalf("%v", got)
	}
}
