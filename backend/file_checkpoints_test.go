package main

import (
	intakefeature "refratia/backend/features/intake"
	"testing"
)

func TestProcessedCheckpointKeyUsesVersionAndSHA256(
	testingContext *testing.T,
) {
	hash :=
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	actual :=
		processedFileCheckpointObjectKey(
			hash,
		)

	expected :=
		"file-checkpoints/v1/" +
			hash +
			"/result.json"

	if actual != expected {
		testingContext.Fatalf(
			"checkpoint path = %q; want %q",
			actual,
			expected,
		)
	}
}

func TestUploadedCheckpointKeyUsesVersionAndSHA256(
	testingContext *testing.T,
) {
	hash :=
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" +
			"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	actual :=
		uploadedFileCheckpointObjectKey(
			hash,
		)

	expected :=
		"file-checkpoints/v1/" +
			hash +
			"/source"

	if actual != expected {
		testingContext.Fatalf(
			"source path = %q; want %q",
			actual,
			expected,
		)
	}
}

func TestValidFileCheckpointSHA256(
	testingContext *testing.T,
) {
	valid :=
		"0123456789abcdef0123456789abcdef" +
			"0123456789abcdef0123456789abcdef"

	if !validFileCheckpointSHA256(
		valid,
	) {
		testingContext.Fatal(
			"valid SHA-256 should be accepted",
		)
	}

	if validFileCheckpointSHA256(
		"1234",
	) {
		testingContext.Fatal(
			"short SHA-256 must be rejected",
		)
	}
}

func TestBuildAnalysisForPentacamCheckpointKeepsOnlyCurrentEye(
	testingContext *testing.T,
) {
	filename :=
		"PENTACAM__OD__PACIENTE__20260917.pdf"

	analysis :=
		map[string]any{
			"source_files": []any{
				map[string]any{
					"path": filename,
					"exam": "pentacam_corneal_tomography",
					"eye":  "OD",
				},
				map[string]any{
					"path": "PENTACAM__OS__PACIENTE__20260917.pdf",
					"exam": "pentacam_corneal_tomography",
					"eye":  "OS",
				},
			},
			"exams": map[string]any{
				"pentacam_corneal_tomography": map[string]any{
					"eyes": map[string]any{
						"OD": map[string]any{
							"value": "right",
						},
						"OS": map[string]any{
							"value": "left",
						},
					},
				},
			},
		}

	file :=
		uploadedFile{
			Metadata: intakefeature.FileMetadata{
				Filename:    filename,
				ContentType: "application/pdf",
				Size:        1234,
				SHA256: "cccccccccccccccccccccccccccccccc" +
					"cccccccccccccccccccccccccccccccc",
			},
		}

	checkpointAnalysis :=
		buildAnalysisForFileCheckpoint(
			analysis,
			file,
			"PENTACAM",
			"OD",
		)

	if checkpointAnalysis == nil {
		testingContext.Fatal(
			"checkpoint analysis should exist",
		)
	}

	exams :=
		checkpointAnalysis["exams"].(map[string]any)

	pentacam :=
		exams["pentacam_corneal_tomography"].(map[string]any)

	eyes :=
		pentacam["eyes"].(map[string]any)

	if _, found :=
		eyes["OD"]; !found {
		testingContext.Fatal(
			"OD must remain in OD checkpoint",
		)
	}

	if _, found :=
		eyes["OS"]; found {
		testingContext.Fatal(
			"OS must not leak into OD checkpoint",
		)
	}

	sources :=
		checkpointAnalysis["source_files"].([]any)

	if len(sources) != 1 {
		testingContext.Fatalf(
			"checkpoint should contain one source, got %d",
			len(sources),
		)
	}
}
