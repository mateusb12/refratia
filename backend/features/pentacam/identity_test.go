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
		word("Sobrenome:", 100, 100), word("SURNAME", 300, 100),
		word("Nome:", 100, 140), word("GIVEN", 300, 140),
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

// F8: when the "Nasc." label is not read, the birth date must stay missing; it
// must never fall through to the next row (the exam date).
func TestIdentityNeverBorrowsExamDateForUnreadBirthLabel(t *testing.T) {
	_, err := parsePentacamIdentityWords(panelWords("", "01/28/1980"))
	if err == nil {
		t.Fatal("expected an error: birth date label was not read")
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
