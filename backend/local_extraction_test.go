package main

import "testing"

func syntheticCompleteIOL(v float64, source string) map[string]any {
	eye := func(x float64) map[string]any {
		return map[string]any{
			"axial_length_mm": x,
			"keratometry": map[string]any{
				"k1_d":                 x + 1,
				"k2_d":                 x + 2,
				"mean_k_d":             x + 3,
				"astigmatism_d":        x + 4,
				"astigmatism_axis_deg": x + 5,
			},
			"anterior_chamber_depth_mm": x + 6,
			"lens_thickness_mm":         x + 7,
			"white_to_white_mm":         x + 8,
			"target_refraction_d":       x + 9,
		}
	}

	return map[string]any{
		"source": []any{source},
		"eyes": map[string]any{
			"OD": eye(v),
			"OS": eye(v + 20),
		},
	}
}

func TestLocalIOLCannotBeOverwrittenByFallback(t *testing.T) {
	local := map[string]any{
		"exams": map[string]any{
			"iol_calculation": syntheticCompleteIOL(10, "bio.pdf"),
		},
	}

	fallback := map[string]any{
		"patient": map[string]any{
			"full_name":  "Paciente Sintético",
			"birth_date": "2000-01-01",
		},
		"exams": map[string]any{
			"iol_calculation": syntheticCompleteIOL(99, "bio.pdf"),
		},
	}

	resolved := localResolvedExamKeys(local)
	if !resolved["iol_calculation"] {
		t.Fatal("IOL local completo não foi marcado como resolvido")
	}

	stripLocallyResolvedExams(fallback, resolved)
	mergeMissingValues(local, fallback)

	exams := local["exams"].(map[string]any)
	iol := exams["iol_calculation"].(map[string]any)
	od := iol["eyes"].(map[string]any)["OD"].(map[string]any)

	if got := od["axial_length_mm"]; got != float64(10) {
		t.Fatalf("fallback sobrescreveu valor local: got=%v", got)
	}

	patient := local["patient"].(map[string]any)
	if patient["full_name"] != "Paciente Sintético" {
		t.Fatal("fallback não preencheu gap de identidade")
	}
}

func TestCompleteLocalExtractionHasZeroGaps(t *testing.T) {
	analysis := map[string]any{
		"patient": map[string]any{
			"full_name":  "Paciente Sintético",
			"birth_date": "2000-01-01",
		},
		"verificacao_identidade": []any{
			map[string]any{
				"status": "ok",
				"source": "bio.pdf",
			},
		},
		"exams": map[string]any{
			"iol_calculation": syntheticCompleteIOL(10, "bio.pdf"),
		},
	}

	files := []uploadedFile{
		{Metadata: intakeFile{Filename: "bio.pdf"}},
	}

	if gaps := collectLocalGaps(analysis, files); len(gaps) != 0 {
		t.Fatalf("esperava zero gaps; recebeu %v", gaps)
	}
}

func TestStripLocallyResolvedExamsRemovesStaleInvalidExamWarning(t *testing.T) {
	analysis := map[string]any{
		"exams": map[string]any{
			"iol_calculation": map[string]any{
				"source": []any{"BIO SRK-T AO.pdf"},
			},
			"oct_retina": map[string]any{
				"source": []any{"oct.pdf"},
			},
		},
		"extraction_notes": map[string]any{
			"invalid_exams": []any{
				map[string]any{
					"exam":   "iol_calculation",
					"reason": "payload não é um objeto",
				},
				map[string]any{
					"exam":   "oct_retina",
					"reason": "source ausente ou inválido",
				},
			},
		},
	}

	stripLocallyResolvedExams(
		analysis,
		map[string]bool{
			"iol_calculation": true,
		},
	)

	exams := analysis["exams"].(map[string]any)

	if _, exists := exams["iol_calculation"]; exists {
		t.Fatal("locally resolved IOL must be removed from fallback exams")
	}

	if _, exists := exams["oct_retina"]; !exists {
		t.Fatal("unrelated fallback exam must remain untouched")
	}

	notes := analysis["extraction_notes"].(map[string]any)
	invalid := notes["invalid_exams"].([]any)

	if len(invalid) != 1 {
		t.Fatalf(
			"expected only unrelated warning to remain, got %#v",
			invalid,
		)
	}

	item := invalid[0].(map[string]any)

	if item["exam"] != "oct_retina" {
		t.Fatalf(
			"stale IOL warning survived: %#v",
			invalid,
		)
	}
}
