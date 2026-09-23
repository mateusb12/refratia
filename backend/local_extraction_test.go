package main

import (
	intakefeature "refratia/backend/features/intake"
	"testing"
)

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
		{Metadata: intakefeature.FileMetadata{Filename: "bio.pdf"}},
	}

	if gaps := collectLocalGaps(analysis, files); len(gaps) != 0 {
		t.Fatalf("esperava zero gaps; recebeu %v", gaps)
	}
}

func TestCompleteLocalSpecularMicroscopyIsResolved(t *testing.T) {
	analysis := map[string]any{
		"exams": map[string]any{
			"specular_microscopy": map[string]any{
				"eyes": map[string]any{
					"OD": map[string]any{"cell_density_cells_per_mm2": 2403.0},
					"OS": map[string]any{"cell_density_cells_per_mm2": 2184.0},
				},
			},
		},
	}
	if !localResolvedExamKeys(analysis)["specular_microscopy"] {
		t.Fatal("microscopia especular completa não foi marcada como resolvida")
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

func TestRetinographyLocalCompleteIsResolved(
	t *testing.T,
) {
	analysis := map[string]any{
		"exams": map[string]any{
			"fundus_retinography": map[string]any{
				"id":             "PACIENTE TESTE",
				"device_or_mode": "Retina",
				"eyes": map[string]any{
					"OD": map[string]any{
						"patient_id":    "PACIENTE TESTE",
						"eye":           "OD",
						"exam_datetime": "2026-08-28 16:37:10",
						"mode":          "Retina",
					},
					"OS": map[string]any{
						"patient_id":    "PACIENTE TESTE",
						"eye":           "OS",
						"exam_datetime": "2026-08-28 16:37:38",
						"mode":          "Retina",
					},
				},
			},
		},
	}

	if !retinographyLocalComplete(analysis) {
		t.Fatal(
			"retinografia OD+OS completa deveria ser considerada resolvida",
		)
	}

	resolved := localResolvedExamKeys(analysis)

	if !resolved["fundus_retinography"] {
		t.Fatal(
			"fundus_retinography local completo não foi marcado como resolvido",
		)
	}
}

func TestRetinographyLocalIncompleteEyeIsNotResolved(
	t *testing.T,
) {
	analysis := map[string]any{
		"exams": map[string]any{
			"fundus_retinography": map[string]any{
				"id":             "PACIENTE TESTE",
				"device_or_mode": "Retina",
				"eyes": map[string]any{
					"OD": map[string]any{
						"patient_id":    "PACIENTE TESTE",
						"eye":           "OD",
						"exam_datetime": "2026-08-28 16:37:10",
						"mode":          "Retina",
					},
				},
			},
		},
	}

	if retinographyLocalComplete(analysis) {
		t.Fatal(
			"retinografia apenas OD não deve ser considerada completa",
		)
	}

	if localResolvedExamKeys(analysis)["fundus_retinography"] {
		t.Fatal(
			"fundus_retinography incompleto foi marcado como resolvido",
		)
	}
}

func TestRetinographyLocalRejectsIdentityDivergence(
	t *testing.T,
) {
	analysis := map[string]any{
		"exams": map[string]any{
			"fundus_retinography": map[string]any{
				"id":             "PACIENTE TESTE",
				"device_or_mode": "Retina",
				"eyes": map[string]any{
					"OD": map[string]any{
						"patient_id":    "PACIENTE TESTE",
						"eye":           "OD",
						"exam_datetime": "2026-08-28 16:37:10",
						"mode":          "Retina",
					},
					"OS": map[string]any{
						"patient_id":    "OUTRO PACIENTE",
						"eye":           "OS",
						"exam_datetime": "2026-08-28 16:37:38",
						"mode":          "Retina",
					},
				},
			},
		},
	}

	if retinographyLocalComplete(analysis) {
		t.Fatal(
			"divergência de identidade entre OD e OS não pode ser considerada completa",
		)
	}
}

func TestSpecularMicroscopyAOPartialProducesODGap(
	t *testing.T,
) {
	analysis := map[string]any{
		"exams": map[string]any{
			"specular_microscopy": map[string]any{
				"eyes": map[string]any{
					"OS": map[string]any{
						"cell_density_cells_per_mm2": 3366.0,
					},
				},
			},
		},
	}

	files := []uploadedFile{
		{
			Metadata: intakefeature.FileMetadata{
				Filename: "micro.jpg",
				ExamType: "MICROSCOPIA_ESPECULAR",
				Eye:      "AO",
			},
		},
	}

	gaps :=
		specularMicroscopyLocalGaps(
			analysis,
			files,
		)

	expected :=
		"specular_microscopy.eyes.OD.cell_density_cells_per_mm2"

	if len(gaps) != 1 ||
		gaps[0] != expected {
		t.Fatalf(
			"esperava somente gap OD; recebeu %#v",
			gaps,
		)
	}

	if localResolvedExamKeysForFiles(
		analysis,
		files,
	)["specular_microscopy"] {
		t.Fatal(
			"AO com somente OS não pode estar resolvido",
		)
	}
}

func TestSpecularMicroscopyAOCompleteHasNoGaps(
	t *testing.T,
) {
	analysis := map[string]any{
		"exams": map[string]any{
			"specular_microscopy": map[string]any{
				"eyes": map[string]any{
					"OD": map[string]any{
						"cell_density_cells_per_mm2": 3312.0,
					},
					"OS": map[string]any{
						"cell_density_cells_per_mm2": 3366.0,
					},
				},
			},
		},
	}

	files := []uploadedFile{
		{
			Metadata: intakefeature.FileMetadata{
				Filename: "micro.jpg",
				ExamType: "MICROSCOPIA_ESPECULAR",
				Eye:      "AO",
			},
		},
	}

	if gaps :=
		specularMicroscopyLocalGaps(
			analysis,
			files,
		); len(gaps) != 0 {
		t.Fatalf(
			"AO completo não deveria ter gaps: %#v",
			gaps,
		)
	}

	if !localResolvedExamKeysForFiles(
		analysis,
		files,
	)["specular_microscopy"] {
		t.Fatal(
			"AO com OD+OS deveria estar resolvido",
		)
	}
}

func TestSpecularMicroscopyODOnlyCanBeComplete(
	t *testing.T,
) {
	analysis := map[string]any{
		"exams": map[string]any{
			"specular_microscopy": map[string]any{
				"eyes": map[string]any{
					"OD": map[string]any{
						"cell_density_cells_per_mm2": 3312.0,
					},
				},
			},
		},
	}

	files := []uploadedFile{
		{
			Metadata: intakefeature.FileMetadata{
				Filename: "micro-od.jpg",
				ExamType: "MICROSCOPIA_ESPECULAR",
				Eye:      "OD",
			},
		},
	}

	if gaps :=
		specularMicroscopyLocalGaps(
			analysis,
			files,
		); len(gaps) != 0 {
		t.Fatalf(
			"OD solicitado e presente deveria estar completo: %#v",
			gaps,
		)
	}

	if !localResolvedExamKeysForFiles(
		analysis,
		files,
	)["specular_microscopy"] {
		t.Fatal(
			"OD-only solicitado deveria estar resolvido",
		)
	}
}

func TestSpecularMicroscopyOSOnlyCanBeComplete(
	t *testing.T,
) {
	analysis := map[string]any{
		"exams": map[string]any{
			"specular_microscopy": map[string]any{
				"eyes": map[string]any{
					"OS": map[string]any{
						"cell_density_cells_per_mm2": 3366.0,
					},
				},
			},
		},
	}

	files := []uploadedFile{
		{
			Metadata: intakefeature.FileMetadata{
				Filename: "micro-os.jpg",
				ExamType: "MICROSCOPIA_ESPECULAR",
				Eye:      "OS",
			},
		},
	}

	if !localResolvedExamKeysForFiles(
		analysis,
		files,
	)["specular_microscopy"] {
		t.Fatal(
			"OS-only solicitado deveria estar resolvido",
		)
	}
}

func TestSpecularMicroscopyPartialFileStaysInFallback(
	t *testing.T,
) {
	filename := "micro.jpg"

	analysis := map[string]any{
		"patient": map[string]any{
			"full_name":  "Paciente Teste",
			"birth_date": "2000-01-01",
		},
		"verificacao_identidade": []any{
			map[string]any{
				"source": filename,
			},
		},
		"exams": map[string]any{
			"specular_microscopy": map[string]any{
				"source": []any{
					filename,
				},
				"eyes": map[string]any{
					"OS": map[string]any{
						"cell_density_cells_per_mm2": 3366.0,
					},
				},
			},
		},
	}

	files := []uploadedFile{
		{
			Metadata: intakefeature.FileMetadata{
				Filename: filename,
				ExamType: "MICROSCOPIA_ESPECULAR",
				Eye:      "AO",
			},
		},
	}

	fallback :=
		localFallbackFiles(
			analysis,
			files,
		)

	if len(fallback) != 1 {
		t.Fatalf(
			"arquivo AO parcial precisa permanecer no fallback; recebeu %d",
			len(fallback),
		)
	}
}
