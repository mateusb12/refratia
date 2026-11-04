package pentacam

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestParsePentacamFieldNumberRequiresPrintedDecimal(t *testing.T) {
	if _, ok := parsePentacamFieldNumber("K1 435D", `\bk1\b`, true); ok {
		t.Fatal("não pode transformar 435 em 43.5")
	}

	got, ok := parsePentacamFieldNumber("K1 43.5D", `\bk1\b`, true)
	if !ok || got != 43.5 {
		t.Fatalf("decimal impresso deveria ser aceito: %v %v", got, ok)
	}
}

func TestPentacamFocusedTKCDash(t *testing.T) {
	got, ok := pentacamFocusedTKC([]string{"CKI 1.01   TKC: —"})
	if !ok || got != "—" {
		t.Fatalf("TKC inesperado: %#v %v", got, ok)
	}
}

func TestExtractPDFMariaGoreteODRecoversVisibleFields(t *testing.T) {
	fixturePath := os.Getenv("PENTACAM_FIXTURE")
	if fixturePath == "" {
		fixturePath = "../../../frontend/data/maria-gorete/PENTACAM OD MARIA GORETE.pdf"
	}

	data, errorValue := os.ReadFile(fixturePath)
	if errorValue != nil {
		t.Fatal(errorValue)
	}

	exam, errorValue := ExtractPDF(context.Background(), data)
	if errorValue != nil {
		t.Fatal(errorValue)
	}

	assertPentacamFieldValue(t, exam, "pachymetry", "thinnest_um", 531)
	assertPentacamFieldValue(t, exam, "belin_ambrosio", "d", 1.05)
	assertPentacamFieldValue(t, exam, "topometric_indices_8mm", "ihd", 0.018)
	assertPentacamFieldValue(t, exam, "anterior_segment", "internal_anterior_chamber_depth_mm", 2.66)
}

func TestDetectEyeMariaGoreteOE(t *testing.T) {
	data, errorValue := os.ReadFile(
		"../../../frontend/data/maria-gorete/PENTACAM OE MARIA GORETE.pdf",
	)
	if errorValue != nil {
		t.Fatal(errorValue)
	}

	eye, errorValue := DetectEye(context.Background(), data)
	if errorValue != nil {
		t.Fatal(errorValue)
	}
	if eye != "OS" {
		t.Fatalf("olho esquerdo esperado como OS, recebido %q", eye)
	}
}

func TestExtractPDFMariaGoreteOERecoversRequiredFields(t *testing.T) {
	data, errorValue := os.ReadFile(
		"../../../frontend/data/maria-gorete/PENTACAM OE MARIA GORETE.pdf",
	)
	if errorValue != nil {
		t.Fatal(errorValue)
	}

	exam, errorValue := ExtractPDF(context.Background(), data)
	if errorValue != nil {
		t.Fatal(errorValue)
	}

	missing := []string{}
	requiredPaths := [][]string{
		{"anterior_cornea", "k1_d"},
		{"anterior_cornea", "k2_d"},
		{"anterior_cornea", "km_d"},
		{"anterior_cornea", "astigmatism_d"},
		{"pachymetry", "thinnest_um"},
		{"belin_ambrosio", "d"},
		{"belin_ambrosio", "art_max"},
		{"topometric_indices_8mm", "isv"},
		{"topometric_indices_8mm", "iva"},
		{"topometric_indices_8mm", "iha"},
		{"topometric_indices_8mm", "ihd"},
		{"topometric_indices_8mm", "ki"},
		{"topometric_indices_8mm", "cki"},
		{"corneal_rings", "zernike", "5mm", "z31_coma"},
		{"anterior_segment", "internal_anterior_chamber_depth_mm"},
		{"cataract_preop", "total_corneal_z40_6mm_um"},
	}
	for _, path := range requiredPaths {
		if !hasNumberAtAnyPath(exam, path) {
			missing = append(missing, strings.Join(path, "."))
		}
	}
	if len(missing) == len(requiredPaths) {
		t.Fatalf("nenhum campo numérico do OE foi extraído; ausentes: %v", missing)
	}
}

func assertPentacamFieldValue(t *testing.T, exam map[string]any, section, field string, expected float64) {
	t.Helper()

	sectionValues, ok := exam[section].(map[string]any)
	if !ok {
		t.Fatalf("seção %q ausente: %#v", section, exam[section])
	}

	actual, ok := sectionValues[field].(float64)
	if !ok || actual != expected {
		t.Fatalf("campo %s.%s esperado %v, recebido %#v", section, field, expected, sectionValues[field])
	}
}
