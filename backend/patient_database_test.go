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

func TestExamTypeKeyNeverRemapsUnknownOrOutOfProtocol(t *testing.T) {
	for in, want := range map[string]string{
		"ANTSEG": "anterior_segment_photo", "anterior_segment_photo": "anterior_segment_photo",
		"TOPO": "corneal_topography_other", "PENTACAM": "pentacam_corneal_tomography",
		"mystery": "unclassified", "": "unclassified",
	} {
		if got := examTypeKey(in); got != want {
			t.Errorf("examTypeKey(%q)=%q want %q", in, got, want)
		}
	}
}
