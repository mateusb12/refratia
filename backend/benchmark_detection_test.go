package main

import "testing"

func completePentacamBenchmarkPayload() map[string]any {
	return map[string]any{
		"anterior_cornea": map[string]any{
			"k1_d": 43.5, "k2_d": 43.8, "km_d": 43.6, "astigmatism_d": 0.2,
		},
		"pachymetry":     map[string]any{"thinnest_um": 529.0},
		"belin_ambrosio": map[string]any{"d": 0.65, "art_max": 416.0},
		"topometric_indices_8mm": map[string]any{
			"isv": 10.0, "iva": 0.09, "iha": 4.2, "ki": 1.01, "cki": 1.0, "tkc": "—",
		},
		"corneal_rings": map[string]any{
			"zernike": map[string]any{"5mm": map[string]any{"z31_coma": 0.097}},
		},
		"anterior_segment": map[string]any{"internal_anterior_chamber_depth_mm": 3.36},
		"cataract_preop":   map[string]any{"total_corneal_z40_6mm_um": 0.287},
	}
}

func TestBenchmarkPentacamKeepsSixteenFields(t *testing.T) {
	fields, extracted := benchmarkPentacamResult(completePentacamBenchmarkPayload())

	if len(fields) != 16 || extracted != 16 {
		t.Fatalf("Pentacam benchmark must remain 16/16, got %d/%d", extracted, len(fields))
	}
}

func TestBenchmarkMicroscopyFilenameDoesNotCountAsExtraction(t *testing.T) {
	fields, extracted := benchmarkSpecularMicroscopyResult()

	if len(fields) != 1 || extracted != 0 || fields[0].Found {
		t.Fatalf("microscopy benchmark must remain 0/1, got %#v and %d/%d", fields, extracted, len(fields))
	}
}
