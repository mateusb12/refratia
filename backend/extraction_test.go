package main

import "testing"

func TestConfirmedAnalysisRejectsDifferentFile(t *testing.T) {
	files := []uploadedFile{{Metadata: intakeFile{Filename: "exame.pdf", SHA256: "hash-correto"}}}
	valid := `{"patient":{},"exams":{},"source_files":[{"path":"exame.pdf","sha256":"hash-correto"}]}`
	if _, err := confirmedAnalysis(valid, files); err != nil {
		t.Fatalf("expected matching analysis: %v", err)
	}

	tampered := `{"patient":{},"exams":{},"source_files":[{"path":"exame.pdf","sha256":"outro-hash"}]}`
	if _, err := confirmedAnalysis(tampered, files); err == nil {
		t.Fatal("expected mismatched file to be rejected")
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
