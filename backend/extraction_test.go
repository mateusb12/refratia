package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPreparedPromptLabelIncludesPDFPage(t *testing.T) {
	label := preparedPromptLabel(preparedFile{File: uploadedFile{Metadata: intakeFile{Filename: "exame.pdf"}}, Page: 8})
	if label != "Arquivo: exame.pdf — página 8" {
		t.Fatalf("unexpected page label: %q", label)
	}
}

func TestPrepareExtractionFilesRejectsInvalidPDF(t *testing.T) {
	_, err := prepareExtractionFiles(context.Background(), []uploadedFile{{Metadata: intakeFile{Filename: "exame.pdf", ContentType: "application/pdf"}, Data: []byte("not a pdf")}})
	if err == nil {
		t.Fatal("expected invalid PDF to fail during preprocessing")
	}
}

func TestStoredAnalysisRejectsDifferentFile(t *testing.T) {
	files := []intakeFile{{Filename: "exame.pdf", SHA256: "hash-correto", Key: "drafts/intake/exame.pdf"}}
	valid := map[string]any{"source_files": []any{map[string]any{"path": "exame.pdf", "sha256": "hash-correto"}}}
	if err := validateStoredAnalysis(valid, files); err != nil {
		t.Fatalf("expected matching analysis: %v", err)
	}

	tampered := map[string]any{"source_files": []any{map[string]any{"path": "exame.pdf", "sha256": "outro-hash"}}}
	if err := validateStoredAnalysis(tampered, files); err == nil {
		t.Fatal("expected mismatched file to be rejected")
	}
}

func TestValidIntakeID(t *testing.T) {
	if !validIntakeID("intake-20260815T155045Z-da4bf173") || validIntakeID("../../case") {
		t.Fatal("expected only generated intake IDs to be accepted")
	}
}

func TestDeleteCaseRequiresToken(t *testing.T) {
	t.Setenv("CASE_DELETE_TOKEN", "secret")
	request := httptest.NewRequest(http.MethodDelete, "/api/cases/case-20260815T155045Z-da4bf173", nil)
	response := httptest.NewRecorder()
	caseHandler(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized deletion, got %d", response.Code)
	}
}

func TestValidatePatientJSONUsesOfficialExamKeys(t *testing.T) {
	valid := `{"patient":{},"exams":{"pentacam_corneal_tomography":{"source":[]}}}`
	if err := validatePatientJSON(valid); err != nil {
		t.Fatalf("expected official contract: %v", err)
	}

	unknown := `{"patient":{},"exams":{"pentacam":{"source":[]}}}`
	if err := validatePatientJSON(unknown); err == nil {
		t.Fatal("expected unknown exam key to be rejected")
	}
}

func TestDecodeAnalysisAllowsSingleExamWithMalformedAbsentExam(t *testing.T) {
	analysis, err := decodeAnalysis(`{"patient":{"full_name":"Paciente"},"exams":{"fundus_retinography":{"source":["olho.jpeg"]},"iol_calculation":null}}`)
	if err != nil {
		t.Fatalf("expected isolated exam to be accepted: %v", err)
	}
	exams := analysis["exams"].(map[string]any)
	if _, exists := exams["iol_calculation"]; exists {
		t.Fatal("expected malformed absent exam to be removed")
	}
}

func TestPentacamRepairFillsOnlyMissingMetrics(t *testing.T) {
	analysis := map[string]any{"exams": map[string]any{
		"pentacam_corneal_tomography": map[string]any{
			"source": []any{"od.pdf", "os.pdf"},
			"eyes": map[string]any{
				"OD": map[string]any{"general": map[string]any{"pachymetry_thinnest_um": 529.0, "k_max_anterior_diopters": 44.2}},
				"OS": map[string]any{"general": map[string]any{"pachymetry_thinnest_um": 526.0, "k_max_anterior_diopters": 45.5}},
			},
		},
	}}
	files := []uploadedFile{
		{Metadata: intakeFile{Filename: "od.pdf"}},
		{Metadata: intakeFile{Filename: "os.pdf"}},
		{Metadata: intakeFile{Filename: "biometria.pdf"}},
	}

	selected := pentacamFilesNeedingRepair(analysis, files)
	if len(selected) != 2 || selected[0].Metadata.Filename != "od.pdf" || selected[1].Metadata.Filename != "os.pdf" {
		t.Fatalf("expected only Pentacam sources, got %#v", selected)
	}

	mergePentacamRepair(analysis, map[string]any{"eyes": map[string]any{
		"OD": map[string]any{
			"general":        map[string]any{"k_max_anterior_diopters": 99.0},
			"belin_ambrosio": map[string]any{"d": 0.65, "art_max": 416.0},
		},
		"OS": map[string]any{
			"belin_ambrosio": map[string]any{"d": 2.27, "art_max": 366.0},
		},
	}})

	exam := pentacamExam(analysis)
	if pentacamNeedsRepair(exam) {
		t.Fatal("expected targeted repair to complete Pentacam metrics")
	}
	od := exam["eyes"].(map[string]any)["OD"].(map[string]any)
	if od["general"].(map[string]any)["k_max_anterior_diopters"] != 44.2 {
		t.Fatal("repair must not overwrite an existing extracted value")
	}
}
