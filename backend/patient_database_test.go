package main

import (
	"errors"
	"testing"
)

func TestPatientIdentityRejectsConflictingIdentity(t *testing.T) {
	analysis := map[string]any{
		"patient": map[string]any{
			"full_name":  "AVERSA MARTINELLI CRESO",
			"birth_date": "11/04/2026",
		},
		"verificacao_identidade": []any{
			map[string]any{"nascimento_lido": "14/12/1979"},
			map[string]any{"nascimento_lido": "11/04/2026"},
		},
	}

	_, _, _, err := patientIdentity(analysis)
	if !errors.Is(err, errPatientIdentity) {
		t.Fatalf("expected identity error, got %v", err)
	}
}
