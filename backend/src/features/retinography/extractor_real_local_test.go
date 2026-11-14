package retinography

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExtractRealRetinaFixtures(
	t *testing.T,
) {
	if os.Getenv("REFRATIA_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("teste de integração desabilitado; use REFRATIA_RUN_INTEGRATION_TESTS=1")
	}
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("tesseract não instalado")
	}

	type fixture struct {
		path         string
		patientID    string
		eye          string
		examDateTime string
	}

	fixtures := []fixture{
		{
			path: filepath.Join(
				"..",
				"..",
				"..",
				"frontend",
				"data",
				"francisco",
				"RETINA__OD__JOAO_FRANCISCO_DE_AGUIAR__20260828_163710.jpeg",
			),
			patientID:    "JOAO FRANCISCO DE AGUIAR",
			eye:          "OD",
			examDateTime: "2026-08-28 16:37:10",
		},
		{
			path: filepath.Join(
				"..",
				"..",
				"..",
				"frontend",
				"data",
				"francisco",
				"RETINA__OS__JOAO_FRANCISCO_DE_AGUIAR__20260828_163738.jpeg",
			),
			patientID:    "JOAO FRANCISCO DE AGUIAR",
			eye:          "OS",
			examDateTime: "2026-08-28 16:37:38",
		},
		{
			path: filepath.Join(
				"..",
				"..",
				"..",
				"frontend",
				"data",
				"rolmey",
				"RETINA__OD__ROLMEY_ARANTES_SILVA__20260730_120822.jpeg",
			),
			patientID:    "ROLMEY ARANTES SILVA",
			eye:          "OD",
			examDateTime: "2026-07-30 12:08:22",
		},
		{
			path: filepath.Join(
				"..",
				"..",
				"..",
				"frontend",
				"data",
				"rolmey",
				"RETINA__OS__ROLMEY_ARANTES_SILVA__20260730_121009.jpeg",
			),
			patientID:    "ROLMEY ARANTES SILVA",
			eye:          "OS",
			examDateTime: "2026-07-30 12:10:09",
		},
	}

	for _, tc := range fixtures {
		tc := tc

		t.Run(
			filepath.Base(tc.path),
			func(t *testing.T) {
				data, err := os.ReadFile(tc.path)

				if os.IsNotExist(err) {
					t.Skipf(
						"fixture ausente: %s",
						tc.path,
					)
				}

				if err != nil {
					t.Fatal(err)
				}

				result, err := Extract(
					context.Background(),
					data,
				)
				if err != nil {
					t.Fatal(err)
				}

				if result.PatientID != tc.patientID {
					t.Fatalf(
						"patient_id: got=%q want=%q OCR=%q",
						result.PatientID,
						tc.patientID,
						result.RawText,
					)
				}

				if result.Eye != tc.eye {
					t.Fatalf(
						"eye: got=%q want=%q OCR=%q",
						result.Eye,
						tc.eye,
						result.RawText,
					)
				}

				if result.Mode != "Retina" {
					t.Fatalf(
						"mode inesperado: %q",
						result.Mode,
					)
				}

				if result.ExamDateTime != tc.examDateTime {
					t.Fatalf(
						"datetime: got=%q want=%q OCR=%q",
						result.ExamDateTime,
						tc.examDateTime,
						result.RawText,
					)
				}

				eyes, ok := result.Exam["eyes"].(map[string]any)

				if !ok {
					t.Fatalf(
						"eyes ausente: %#v",
						result.Exam,
					)
				}

				eyePayload, ok := eyes[tc.eye].(map[string]any)

				if !ok {
					t.Fatalf(
						"payload de %s ausente: %#v",
						tc.eye,
						eyes,
					)
				}

				if eyePayload["patient_id"] != tc.patientID {
					t.Fatalf(
						"patient_id não preservado: %#v",
						eyePayload,
					)
				}

				if eyePayload["eye"] != tc.eye {
					t.Fatalf(
						"eye não preservado: %#v",
						eyePayload,
					)
				}

				if eyePayload["exam_datetime"] != tc.examDateTime {
					t.Fatalf(
						"exam_datetime não preservado: %#v",
						eyePayload,
					)
				}
			},
		)
	}
}
