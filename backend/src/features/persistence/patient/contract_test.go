package patient

import "testing"

func TestValidatePatientJSONUsesOfficialExamKeys(t *testing.T) {
	valid := `{"patient":{},"exams":{"pentacam_corneal_tomography":{"source":[]}}}`
	if err := ValidateJSON(valid); err != nil {
		t.Fatalf("expected official contract: %v", err)
	}

	unknown := `{"patient":{},"exams":{"pentacam":{"source":[]}}}`
	if err := ValidateJSON(unknown); err == nil {
		t.Fatal("expected unknown exam key to be rejected")
	}
}
