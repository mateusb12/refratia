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

func TestParsePentacamZ31RecoversOCRWithoutDecimal(t *testing.T) {
	for _, text := range []string{
		"Z31 (Coma): fo. 090 um (207°)",
		"-, 231 (Comal: fo. 090 pm (207°)",
	} {
		got, ok := parsePentacamZ31(text)
		if !ok || got != 0.09 {
			t.Fatalf("Z31 inesperado para %q: %v %v", text, got, ok)
		}
	}
}

func TestExtractPDFMariaGoreteODRecoversVisibleFields(t *testing.T) {
	if os.Getenv("REFRATIA_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("teste de integração desabilitado; use REFRATIA_RUN_INTEGRATION_TESTS=1")
	}
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
	if os.Getenv("REFRATIA_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("teste de integração desabilitado; use REFRATIA_RUN_INTEGRATION_TESTS=1")
	}
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
	if os.Getenv("REFRATIA_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("teste de integração desabilitado; use REFRATIA_RUN_INTEGRATION_TESTS=1")
	}
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
	if len(missing) > 0 {
		t.Fatalf("campos numéricos do OE ausentes: %v", missing)
	}
}

func TestExtractPDFCresoAversaRecoversCriticalFieldsForBothEyes(t *testing.T) {
	if os.Getenv("REFRATIA_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("teste de integração desabilitado; use REFRATIA_RUN_INTEGRATION_TESTS=1")
	}
	cases := []struct {
		filename string
		expected map[string]float64
	}{
		{
			filename: "../../../frontend/data/creso-aversa/PENTACAM CRESO AVERSA OD .pdf",
			expected: map[string]float64{
				"anterior_cornea.k1_d":                                43.9,
				"anterior_cornea.k2_d":                                45.1,
				"anterior_cornea.km_d":                                44.5,
				"anterior_cornea.astigmatism_d":                       1.2,
				"pachymetry.thinnest_um":                              540,
				"belin_ambrosio.d":                                    1.22,
				"belin_ambrosio.art_max":                              419,
				"topometric_indices_8mm.isv":                          15,
				"topometric_indices_8mm.iva":                          0.12,
				"topometric_indices_8mm.iha":                          5.5,
				"topometric_indices_8mm.ihd":                          0.008,
				"topometric_indices_8mm.ki":                           1.04,
				"topometric_indices_8mm.cki":                          1.01,
				"corneal_rings.zernike.5mm.z31_coma":                  0.09,
				"anterior_segment.internal_anterior_chamber_depth_mm": 3.23,
				"cataract_preop.total_corneal_z40_6mm_um":             0.249,
			},
		},
		{
			filename: "../../../frontend/data/creso-aversa/PENTACAM CRESO AVERSA OS .pdf",
			expected: map[string]float64{
				"belin_ambrosio.d":                        0.98,
				"belin_ambrosio.art_max":                  444,
				"pachymetry.thinnest_um":                  537,
				"topometric_indices_8mm.iha":              7.9,
				"topometric_indices_8mm.ihd":              0.012,
				"topometric_indices_8mm.ki":               1.04,
				"cataract_preop.total_corneal_z40_6mm_um": 0.173,
			},
		},
	}

	for _, testCase := range cases {
		data, err := os.ReadFile(testCase.filename)
		if err != nil {
			t.Fatal(err)
		}

		exam, err := ExtractPDF(context.Background(), data)
		if err != nil {
			t.Fatal(err)
		}

		for path, expected := range testCase.expected {
			parts := strings.Split(path, ".")
			if !hasNumberAtAnyPath(exam, parts) {
				t.Errorf("%s: expected %s=%v, got missing", testCase.filename, path, expected)
				continue
			}

			actual := numberAtPath(exam, parts)
			if actual != expected {
				t.Errorf("%s: expected %s=%v, got %v", testCase.filename, path, expected, actual)
			}
		}

		if testCase.filename == "../../../frontend/data/creso-aversa/PENTACAM CRESO AVERSA OD .pdf" {
			topometric := exam["topometric_indices_8mm"].(map[string]any)
			if got := topometric["tkc"]; got != nil {
				t.Errorf("%s: expected topometric_indices_8mm.tkc=—, got %#v", testCase.filename, got)
			}
		}

		if testCase.filename == "../../../frontend/data/creso-aversa/PENTACAM CRESO AVERSA OS .pdf" {
			topometric := exam["topometric_indices_8mm"].(map[string]any)
			if got := topometric["tkc"]; got != nil {
				t.Errorf("%s: expected topometric_indices_8mm.tkc=—, got %#v", testCase.filename, got)
			}
		}
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

func numberAtPath(root map[string]any, path []string) float64 {
	var current any = root
	for _, key := range path {
		values, ok := current.(map[string]any)
		if !ok {
			return 0
		}
		current = values[key]
	}
	value, _ := current.(float64)
	return value
}
