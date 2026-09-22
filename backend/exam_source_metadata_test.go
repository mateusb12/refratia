package main

import "testing"

func TestClassifySourceFilesFromExamsBackfillsLegacyMetadata(
	testingContext *testing.T,
) {
	analysis :=
		map[string]any{
			"exams": map[string]any{
				"iol_calculation": map[string]any{
					"source": []any{
						"EYESUITE__AO__JOAO.pdf",
					},
				},
				"pentacam_corneal_tomography": map[string]any{
					"source": []any{
						"PENTACAM__OD__JOAO.pdf",
						"PENTACAM__OS__JOAO.pdf",
					},
				},
				"fundus_retinography": map[string]any{
					"source": []any{
						"RETINA__OD__JOAO.jpeg",
						"RETINA__OS__JOAO.jpeg",
					},
				},
				"specular_microscopy": map[string]any{
					"source": []any{
						"MICROSCOPIA_ESPECULAR__AO__JOAO.jpeg",
					},
				},
			},
			"source_files": []any{
				map[string]any{
					"path":
						"cases/case-x/aaa11111-EYESUITE__AO__JOAO.pdf",
				},
				map[string]any{
					"path":
						"cases/case-x/bbb22222-PENTACAM__OD__JOAO.pdf",
				},
				map[string]any{
					"path":
						"cases/case-x/ccc33333-PENTACAM__OS__JOAO.pdf",
				},
				map[string]any{
					"path":
						"cases/case-x/ddd44444-RETINA__OD__JOAO.jpeg",
				},
				map[string]any{
					"path":
						"cases/case-x/eee55555-RETINA__OS__JOAO.jpeg",
				},
				map[string]any{
					"path":
						"cases/case-x/fff66666-MICROSCOPIA_ESPECULAR__AO__JOAO.jpeg",
				},
			},
		}

	classifySourceFilesFromExams(
		analysis,
	)

	sources :=
		analysis["source_files"].([]any)

	assertSource :=
		func(
			index int,
			expectedExam string,
			expectedEye string,
		) {
			source :=
				sources[index].(map[string]any)

			if source["exam"] !=
				expectedExam {

				testingContext.Fatalf(
					"source %d exam = %#v; want %q",
					index,
					source["exam"],
					expectedExam,
				)
			}

			if source["eye"] !=
				expectedEye {

				testingContext.Fatalf(
					"source %d eye = %#v; want %q",
					index,
					source["eye"],
					expectedEye,
				)
			}
		}

	assertSource(
		0,
		"iol_calculation",
		"AO",
	)

	assertSource(
		1,
		"pentacam_corneal_tomography",
		"OD",
	)

	assertSource(
		2,
		"pentacam_corneal_tomography",
		"OS",
	)

	assertSource(
		3,
		"fundus_retinography",
		"OD",
	)

	assertSource(
		4,
		"fundus_retinography",
		"OS",
	)

	assertSource(
		5,
		"specular_microscopy",
		"AO",
	)
}

func TestClassifySourceFilesFromExamsPreservesExistingMetadata(
	testingContext *testing.T,
) {
	analysis :=
		map[string]any{
			"exams": map[string]any{
				"iol_calculation": map[string]any{
					"source": []any{
						"EYESUITE__AO__PACIENTE.pdf",
					},
				},
			},
			"source_files": []any{
				map[string]any{
					"path":
						"cases/case-x/12345678-EYESUITE__AO__PACIENTE.pdf",
					"exam":
						"custom_existing_exam",
					"eye":
						"OD",
				},
			},
		}

	classifySourceFilesFromExams(
		analysis,
	)

	source :=
		analysis["source_files"].
			([]any)[0].
			(map[string]any)

	if source["exam"] !=
		"custom_existing_exam" {

		testingContext.Fatal(
			"existing exam metadata must not be overwritten",
		)
	}

	if source["eye"] !=
		"OD" {

		testingContext.Fatal(
			"existing eye metadata must not be overwritten",
		)
	}
}

func TestSourceFilenameMatchesConfirmedPrefix(
	testingContext *testing.T,
) {
	if !sourceFilenameMatches(
		"cases/case-x/714e3cca-EYESUITE__AO__JOAO.pdf",
		"EYESUITE__AO__JOAO.pdf",
	) {
		testingContext.Fatal(
			"confirmed storage prefix should match original source filename",
		)
	}
}
