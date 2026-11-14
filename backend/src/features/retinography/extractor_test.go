package retinography

import "testing"

func TestParseTextExtractsRetinaHeader(
	t *testing.T,
) {
	text := `ID PACIENTE TESTE
Eye OD

Mode Retina

Time 2026-07-30 12:08:22`

	got, err := ParseText(text)
	if err != nil {
		t.Fatal(err)
	}

	if got.PatientID != "PACIENTE TESTE" {
		t.Fatalf(
			"patient id inesperado: %q",
			got.PatientID,
		)
	}

	if got.Eye != "OD" {
		t.Fatalf(
			"eye inesperado: %q",
			got.Eye,
		)
	}

	if got.Mode != "Retina" {
		t.Fatalf(
			"mode inesperado: %q",
			got.Mode,
		)
	}

	if got.ExamDateTime != "2026-07-30 12:08:22" {
		t.Fatalf(
			"datetime inesperado: %q",
			got.ExamDateTime,
		)
	}
}

func TestParseTextRejectsMissingEye(
	t *testing.T,
) {
	_, err := ParseText(`ID PACIENTE TESTE
Mode Retina
Time 2026-01-01 10:20:30`)

	if err == nil {
		t.Fatal(
			"texto sem lateralidade não pode ser aceito",
		)
	}
}

func TestParseTextDoesNotInferMode(
	t *testing.T,
) {
	_, err := ParseText(`ID PACIENTE TESTE
Eye OS
Time 2026-01-01 10:20:30`)

	if err == nil {
		t.Fatal(
			"mode ausente não pode ser inferido",
		)
	}
}
