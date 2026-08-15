package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
