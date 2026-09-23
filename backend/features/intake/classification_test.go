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

	if classification.ExamType != "PENTACAM" ||
		classification.Eye != "OD" {
		t.Fatalf(
			"classification was not normalized: %+v",
			classification,
		)
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

func TestPrefilterPDFCandidatesSuggestsPentacamForNinePages(
	t *testing.T,
) {
	candidates :=
		prefilterPDFCandidates(9)

	if len(candidates) != 1 {
		t.Fatalf(
			"expected one candidate, got %d",
			len(candidates),
		)
	}

	if candidates[0].ExamType !=
		"pentacam_corneal_tomography" {
		t.Fatalf(
			"unexpected candidate: %+v",
			candidates[0],
		)
	}

	if candidates[0].Score <= 0 {
		t.Fatalf(
			"expected positive Pentacam score: %+v",
			candidates[0],
		)
	}
}

func TestPrefilterPDFCandidatesDoesNotGuessOnePagePDF(
	t *testing.T,
) {
	candidates :=
		prefilterPDFCandidates(1)

	if len(candidates) != 0 {
		t.Fatalf(
			"one-page PDF should not be guessed: %+v",
			candidates,
		)
	}
}
