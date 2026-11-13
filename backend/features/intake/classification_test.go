package intake

import (
	"strings"
	"testing"
)

func TestParseClassificationsNormalizesContractKeys(
	t *testing.T,
) {
	classifications, err :=
		ParseClassifications(
			[]string{
				`[{"sha256":"abc","examType":"pentacam_corneal_tomography","eye":"od"}]`,
			},
		)

	if err != nil {
		t.Fatalf(
			"unexpected classification error: %v",
			err,
		)
	}

	classification :=
		classifications["abc"]

	if classification.ExamType != "pentacam_corneal_tomography" ||
		classification.Eye != "OD" {
		t.Fatalf(
			"classification was not normalized: %+v",
			classification,
		)
	}
}

func TestParseClassificationsAcceptsRefractometry(
	t *testing.T,
) {
	classifications, err := ParseClassifications([]string{
		`[{"sha256":"auto-file","examType":"refractometry","eye":"AO"}]`,
	})

	if err != nil {
		t.Fatalf("unexpected refractometry classification error: %v", err)
	}

	classification := classifications["auto-file"]
	if classification.ExamType != "refractometry" || classification.Eye != "AO" {
		t.Fatalf("refractometry classification was not normalized: %+v", classification)
	}
}

func TestParseClassificationsForcesSpecularMicroscopyToAO(t *testing.T) {
	classifications, err := ParseClassifications([]string{
		`[{"sha256":"micro-file","examType":"specular_microscopy","eye":"OD"}]`,
	})

	if err != nil {
		t.Fatalf("unexpected microscopy classification error: %v", err)
	}

	classification := classifications["micro-file"]
	if classification.ExamType != "specular_microscopy" || classification.Eye != "AO" {
		t.Fatalf("specular microscopy was not forced to AO: %+v", classification)
	}
}

func TestParseClassificationsRejectsUnknownExamType(
	t *testing.T,
) {
	_, err :=
		ParseClassifications(
			[]string{
				`[{"sha256":"abc","examType":"BATATA","eye":"OD"}]`,
			},
		)

	if err == nil {
		t.Fatal(
			"expected unknown examType to be rejected",
		)
	}
}

func TestParseClassificationsRejectsUnknownEye(
	t *testing.T,
) {
	_, err :=
		ParseClassifications(
			[]string{
				`[{"sha256":"abc","examType":"pentacam_corneal_tomography","eye":"XYZ"}]`,
			},
		)

	if err == nil {
		t.Fatal(
			"expected unknown eye to be rejected",
		)
	}
}

func TestCanonicalExamFilename(
	t *testing.T,
) {
	name :=
		CanonicalExamFilename(
			"Daniela Nogueira",
			"PENTACAM",
			"OD",
			"pdf",
			"0123456789abcdef",
		)

	if !strings.HasPrefix(
		name,
		"PENTACAM__OD__DANIELA_NOGUEIRA__",
	) ||
		!strings.HasSuffix(
			name,
			".pdf",
		) {
		t.Fatalf(
			"unexpected canonical filename: %s",
			name,
		)
	}
}

func TestCanonicalClassificationFromFilename(
	t *testing.T,
) {
	examType, eye, ok :=
		CanonicalClassificationFromFilename(
			"PENTACAM__OD__DANIELA_NOGUEIRA__20260921_093937.pdf",
		)

	if !ok ||
		examType != "pentacam_corneal_tomography" ||
		eye != "OD" {
		t.Fatalf(
			"unexpected classification: %q %q %v",
			examType,
			eye,
			ok,
		)
	}

	if _, _, ok :=
		CanonicalClassificationFromFilename(
			"arquivo-sem-padrao.pdf",
		); ok {
		t.Fatal(
			"non-canonical filename was recognized",
		)
	}
}

func TestClassificationFromNewFilenameFormat(t *testing.T) {
	tests := []struct {
		name, examType, eye string
	}{
		{"PENTACAM OD MARIA GORETE.pdf", "pentacam_corneal_tomography", "OD"},
		{"PENTACAM OE MARIA GORETE.pdf", "pentacam_corneal_tomography", "OS"},
		{"BIOMETRIA AO MARIA GORETE.pdf", "iol_calculation", "AO"},
		{"AUTO AO MARIA GORETE.pdf", "refractometry", "AO"},
		{"MICROSCOPIA AO MARIA GORETE.jpeg", "specular_microscopy", "AO"},
	}

	for _, test := range tests {
		examType, eye, ok := CanonicalClassificationFromFilename(test.name)
		if !ok || examType != test.examType || eye != test.eye {
			t.Fatalf("%q classified as %q/%q/%v", test.name, examType, eye, ok)
		}
	}
}

func TestClassificationFromFlexibleFilenameFormat(t *testing.T) {
	tests := []struct {
		name, examType, eye string
	}{
		{"PENTACAM CRESO AVERSA OD .pdf", "pentacam_corneal_tomography", "OD"},
		{"PENTACAM CRESO AVERSA OS .pdf", "pentacam_corneal_tomography", "OS"},
		{"BIOMETRIA SKRT AO.pdf", "iol_calculation", "AO"},
		{"MICROSCOPIA ESPECULAR.jpeg", "specular_microscopy", "AO"},
	}

	for _, test := range tests {
		examType, eye, ok := CanonicalClassificationFromFilename(test.name)
		if !ok || examType != test.examType || eye != test.eye {
			t.Fatalf("%q classified as %q/%q/%v", test.name, examType, eye, ok)
		}
	}
}

func TestClassifyTextByDeviceTitle(t *testing.T) {
	cases := map[string]string{
		"OCULUS - PENTACAM Geral":           "pentacam_corneal_tomography",
		"EyeSuite IOL, V4.12.0":             "iol_calculation",
		"Calculation ID (CID): 12":          "iol_calculation",
		"@@TP-15 simK 3mm":                  "corneal_topography_other",
		"PRK-6808 [REF] <R> SPH CYL AX":     "refractometry",
		"nothing recognisable here at all":  "",
		"PENTACAM and EyeSuite on one page": "",
	}
	for text, want := range cases {
		if got := ClassifyText(text); got != want {
			t.Errorf("ClassifyText(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestCanonicalExamFilenameIsUniquePerFileAndReparses(t *testing.T) {
	a := CanonicalExamFilename("Ana", "fundus_retinography", "OD", "jpeg", "aaaaaaaa11")
	b := CanonicalExamFilename("Ana", "fundus_retinography", "OD", "jpeg", "bbbbbbbb22")
	if a == b {
		t.Fatal("same exam+eye with different content must not share a draft key")
	}
	if !strings.HasPrefix(CanonicalExamFilename("Ana", "refractometry", "AO", "jpeg", "cc"), "AUTO__AO__ANA__") {
		t.Fatal("refractometry must be labelled AUTO")
	}
	if exam, eye, ok := CanonicalClassificationFromFilename(a); !ok || exam != "fundus_retinography" || eye != "OD" {
		t.Fatalf("canonical name must reparse, got %q %q %v", exam, eye, ok)
	}
}
