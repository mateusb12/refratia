package specularmicroscopy

import "testing"

func TestParseOCRPanelsExtractsDensityAndMapsPrintedLaterality(t *testing.T) {
	exam, evidence, err := ParseOCRPanels([]OCRPanel{
		{Laterality: "R", Text: "Cell Density (CD) cells/mm² 2403"},
		{Laterality: "L", Text: "Cell Density (CD) cells/mm² 2184"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 2 {
		t.Fatalf("expected evidence for both eyes, got %d", len(evidence))
	}

	od := exam["eyes"].(map[string]any)["OD"].(map[string]any)
	if od["cell_density_cells_per_mm2"] != float64(2403) {
		t.Fatalf("unexpected OD payload: %#v", od)
	}
	os := exam["eyes"].(map[string]any)["OS"].(map[string]any)
	if os["cell_density_cells_per_mm2"] != float64(2184) {
		t.Fatalf("unexpected OS payload: %#v", os)
	}
}

func TestParseOCRPanelsRequiresClinicalContext(t *testing.T) {
	if _, _, err := ParseOCRPanels([]OCRPanel{{Laterality: "R", Text: "Number of Cells 2403"}}); err == nil {
		t.Fatal("text without Cell Density context must not produce a value")
	}
	if _, _, err := ParseOCRPanels([]OCRPanel{{Laterality: "R", Text: "Cell Density (CD) cells/mm²"}}); err == nil {
		t.Fatal("label without a numeric value must not produce a value")
	}
}

func TestParseOCRPanelsUnknownLateralityIsNotInvented(t *testing.T) {
	if _, _, err := ParseOCRPanels([]OCRPanel{{Laterality: "", Text: "Cell Density (CD) cells/mm² 2403"}}); err == nil {
		t.Fatal("unknown laterality must not be mapped to an eye")
	}
}

func TestParseCellDensityAcceptsC0FromRealOCR(
	t *testing.T,
) {
	value, _, ok := parseCellDensity(
		"Cell Density (C0) 2403",
	)

	if !ok || value != 2403 {
		t.Fatalf(
			"esperava 2403; value=%v ok=%v",
			value,
			ok,
		)
	}
}

func TestParseCellDensityAcceptsValueOnFollowingLine(
	t *testing.T,
) {
	value, _, ok := parseCellDensity(
		"Cell Density (CO)\ncells/mm? 2184",
	)

	if !ok || value != 2184 {
		t.Fatalf(
			"esperava 2184; value=%v ok=%v",
			value,
			ok,
		)
	}
}

func TestParseCellDensityDoesNotConfuseNumberOfCells(
	t *testing.T,
) {
	if value, _, ok := parseCellDensity(
		"Number of Cells (NUM) cells 2403",
	); ok {
		t.Fatalf(
			"Number of Cells não pode virar densidade: %v",
			value,
		)
	}
}

func TestLooksLikeSpecularMicroscopyTextAcceptsCellDensity(
	t *testing.T,
) {
	if !looksLikeSpecularMicroscopyText(
		"Cell Density (CD) cells/mm² 3312",
	) {
		t.Fatal(
			"Cell Density com valor plausível deveria reconhecer microscopia",
		)
	}
}

func TestLooksLikeSpecularMicroscopyTextRejectsNumberOfCells(
	t *testing.T,
) {
	if looksLikeSpecularMicroscopyText(
		"Number of Cells (NUM) cells 3312",
	) {
		t.Fatal(
			"Number of Cells isolado não pode reconhecer microscopia",
		)
	}
}

func TestLooksLikeSpecularMicroscopyTextRejectsCellDensityWithoutValue(
	t *testing.T,
) {
	if looksLikeSpecularMicroscopyText(
		"Cell Density (CD) cells/mm²",
	) {
		t.Fatal(
			"Cell Density sem valor não deve reconhecer microscopia",
		)
	}
}

func TestParseCellDensityAcceptsRealNIDEKDistortion(
	t *testing.T,
) {
	value, contextText, ok :=
		parseCellDensity(
			"cece cell Dersiy (0) icells/m?| 3366 |",
		)

	if !ok || value != 3366 {
		t.Fatalf(
			"esperava densidade 3366 do OCR real; value=%v context=%q ok=%v",
			value,
			contextText,
			ok,
		)
	}
}

func TestParseCellDensitySemanticFallbackStillRejectsNumberOfCells(
	t *testing.T,
) {
	if value, _, ok :=
		parseCellDensity(
			"Number of Cells (NUM) cells 3312",
		); ok {
		t.Fatalf(
			"Number of Cells não pode virar densidade: %v",
			value,
		)
	}
}

func TestParseCellDensitySemanticFallbackRejectsUnrelatedCellNumber(
	t *testing.T,
) {
	if value, _, ok :=
		parseCellDensity(
			"Hexagonal Cells (HEX) % 65",
		); ok {
		t.Fatalf(
			"Hexagonal Cells não pode virar densidade: %v",
			value,
		)
	}
}
