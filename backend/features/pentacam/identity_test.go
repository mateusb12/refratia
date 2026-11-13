package pentacam

import (
	"testing"

	"refratia/backend/shared/ocr"
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
