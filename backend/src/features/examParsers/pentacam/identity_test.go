package pentacam

import (
	"testing"

	"refratia/backend/src/shared/ocr"
)

func word(text string, left, top int) ocr.Word {
	return ocr.Word{Text: text, Left: left, Top: top, Width: 40 + 10*len(text), Height: 24}
}

// panelWords builds the identity panel with one label per row and its value
// to the right, on the same band.
func panelWords(birthLabel string, birth string) []ocr.Word {
	words := []ocr.Word{
		word("Sobrenome:", 100, 60), word("SURNAME", 300, 60),
		word("Nome:", 100, 100), word("GIVEN", 300, 100),
		word("Data", 100, 220), word("de", 160, 220), word("Exame:", 200, 220), word("09/21/2026", 300, 220), word("Hora:", 500, 220), word("09:39:37", 580, 220),
	}
	if birthLabel != "" {
		words = append(words, word(birthLabel, 100, 180))
	}
	if birth != "" {
		words = append(words, word(birth, 300, 180), word("Olho:", 500, 180), word("Direito", 580, 180))
	}
	return words
}

func TestIdentityBindsEachValueToItsLabelBand(t *testing.T) {
	got, err := parsePentacamIdentityWords(panelWords("Nasc.:", "01/28/1980"))
	if err != nil {
		t.Fatal(err)
	}
	if got.FullName != "GIVEN SURNAME" || got.BirthDateRaw != "01/28/1980" || got.ExamDateRaw != "09/21/2026" {
		t.Fatalf("unexpected identity: %+v", got)
	}
	if got.DateOrder != "MDY" {
		t.Fatalf("Pentacam date order must be declared MDY, got %q", got.DateOrder)
	}
}

// F8: with only the exam date on the panel (the birth row unread), the birth
// date must stay missing; it must never be taken from the exam row.
func TestIdentityNeverBorrowsExamDateForUnreadBirthRow(t *testing.T) {
	_, err := parsePentacamIdentityWords(panelWords("", ""))
	if err == nil {
		t.Fatal("expected an error: birth date row was not read")
	}
}

// The labels are often lost by the OCR: the stacked date boxes alone locate the
// birth and exam rows, and the names sit above them at the same pitch.
func TestIdentityWorksWithoutAnyLabelText(t *testing.T) {
	words := []ocr.Word{
		word("SURNAME", 300, 60), word("GIVEN", 300, 100),
		word("|01/28/1980", 300, 180), word("09/21/2026", 300, 220),
	}
	got, err := parsePentacamIdentityWords(words)
	if err != nil {
		t.Fatal(err)
	}
	if got.FullName != "GIVEN SURNAME" || got.ExamDateRaw != "09/21/2026" || got.BirthDateRaw != "01/28/1980" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestIdentityDropsImplausibleBirthDate(t *testing.T) {
	cases := map[string]string{
		"birth equals exam date":  "09/21/2026",
		"patient younger than 18": "09/21/2015",
		"birth after exam":        "10/01/2026",
	}
	for name, birth := range cases {
		got, err := parsePentacamIdentityWords(panelWords("Nasc.:", birth))
		if err != nil {
			// the whole identity is refused only when name AND birth are missing
			continue
		}
		if got.BirthDateRaw != "" || got.BirthDateIssue != "conflicting" {
			t.Errorf("%s: expected the birth date to be dropped as conflicting, got %+v", name, got)
		}
	}
}

func TestStripBoxBorderKeepsLetterStems(t *testing.T) {
	g := newGray(60, 30)
	for y := 0; y < 30; y++ { // full-height border at x=2
		g.SetGray(2, y, black)
	}
	for y := 8; y < 22; y++ { // a letter stem, about half the height
		g.SetGray(20, y, black)
	}
	out := stripBoxBorder(g)
	if out.Bounds().Dx() >= 60 {
		t.Fatal("border was not cut")
	}
	if out.GrayAt(20, 15).Y != 0 {
		t.Fatal("letter stem was damaged")
	}
}

// Screen text on the same row but far to the right (segment list, colour
// scale) must not leak into the name (it did: "° 2/25 Segmento: 294° ...").
func TestIdentityNameStopsAtGapAndIgnoresDigitWords(t *testing.T) {
	words := panelWords("Nasc.:", "01/28/1980")
	words = append(words, word("2/25", 1400, 60), word("Segmento:", 1500, 60))
	words = append(words, word("294°", 380, 60))
	got, err := parsePentacamIdentityWords(words)
	if err != nil {
		t.Fatal(err)
	}
	if got.FullName != "GIVEN SURNAME" {
		t.Fatalf("name leaked screen text: %q", got.FullName)
	}
}

// A year with a lost digit is "not read", never a conflicting value; the
// device's unpadded M/D/YYYY is a valid reading.
func TestDropMalformedDatesTreatsLostDigitAsNotRead(t *testing.T) {
	got := dropMalformedDates(Identity{FullName: "A B", BirthDateRaw: "01/28/198", ExamDateRaw: "9/21/2026"})
	if got.BirthDateRaw != "" || got.BirthDateIssue != "missing" || got.ExamDateRaw != "09/21/2026" {
		t.Fatalf("unexpected identity: %+v", got)
	}
	if got := dropMalformedDates(Identity{BirthDateRaw: "1/2/1980", BirthDateIssue: "conflicting"}); got.BirthDateIssue != "conflicting" {
		t.Fatalf("a real conflict must stay: %+v", got)
	}
}

// Unpadded dates (as the device prints them) still get the plausibility check:
// an implausible birth date is conflicting, a plausible one is kept, padded.
func TestDropMalformedDatesChecksPlausibilityOfUnpaddedDates(t *testing.T) {
	if got := dropMalformedDates(Identity{BirthDateRaw: "2/5/2020", ExamDateRaw: "11/6/2026"}); got.BirthDateRaw != "" || got.BirthDateIssue != "conflicting" {
		t.Fatalf("age 6 must be conflicting: %+v", got)
	}
	if got := dropMalformedDates(Identity{BirthDateRaw: "11/6/2026", ExamDateRaw: "11/6/2026"}); got.BirthDateIssue != "conflicting" {
		t.Fatalf("birth == exam must be conflicting: %+v", got)
	}
	if got := dropMalformedDates(Identity{BirthDateRaw: "2/5/1970", ExamDateRaw: "11/6/2026"}); got.BirthDateRaw != "02/05/1970" || got.BirthDateIssue != "" {
		t.Fatalf("plausible unpadded date must be kept padded: %+v", got)
	}
}

// A malformed exam date (birth date unverifiable) or birth date sends the reader
// to the next screen; a well-formed pair or an already-flagged conflict does not.
func TestNeedsAnotherScreen(t *testing.T) {
	for _, c := range []struct {
		id   Identity
		want bool
	}{
		{Identity{BirthDateRaw: "2/5/1970", ExamDateRaw: "11/6/2026"}, false},
		{Identity{BirthDateRaw: "2/5/197", ExamDateRaw: "11/6/2026"}, true},
		{Identity{BirthDateRaw: "2/5/1970", ExamDateRaw: "11/6/202"}, true},
		{Identity{BirthDateRaw: "2/5/1970"}, false},
		{Identity{BirthDateIssue: "conflicting"}, false},
	} {
		if got := needsAnotherScreen(c.id); got != c.want {
			t.Errorf("%+v: got %v want %v", c.id, got, c.want)
		}
	}
}
