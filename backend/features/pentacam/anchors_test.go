package pentacam

import (
	"testing"

	"refratia/backend/shared/ocr"
)

func w(text string, left, top, width, height int) ocr.Word {
	return ocr.Word{Text: text, Left: left, Top: top, Width: width, Height: height}
}

// A label split by an icon glyph is still found, the topmost match wins
// (anterior box before the posterior one) and ColumnOf binds repeated labels
// to the index box.
func TestPickLabel(t *testing.T) {
	words := []ocr.Word{
		w("Prof.", 100, 500, 80, 28), w("+", 190, 500, 14, 28), w("Camara", 210, 500, 90, 28), w("Ant.", 310, 500, 60, 28), w("Ext.:", 380, 500, 60, 28),
		w("K1", 100, 100, 30, 28), w("K1", 100, 300, 30, 28),
		w("ISV:", 700, 400, 56, 28), w("KI:", 700, 460, 33, 28), w("KI", 100, 600, 33, 28),
	}
	spans := labelSpans(words, 12)

	acd, ok := pickLabel(spans, anchorField{Label: lbl(`prof c.mara ant ext`)}, nil)
	if !ok || acd.left != 100 {
		t.Fatalf("split label not found: %+v %v", acd, ok)
	}
	k1, _ := pickLabel(spans, anchorField{Label: lbl(`k[1il]`)}, nil)
	if k1.top != 100 {
		t.Fatalf("topmost K1 expected, got top=%d", k1.top)
	}
	isv, _ := pickLabel(spans, anchorField{Label: lbl(`isv`)}, nil)
	ki, ok := pickLabel(spans, anchorField{Label: lbl(`k[i1l]`), ColumnOf: "isv"}, map[string]labelSpan{"isv": isv})
	if !ok || ki.left != 700 {
		t.Fatalf("index-box KI expected, got %+v", ki)
	}
}

func TestTrimWordBoxCutsBorderAndUnit(t *testing.T) {
	word := w("[40.5D", 100, 0, 60, 30) // 6 chars, 10px each
	ext := trimWordBox(word)
	if e := ext[0]; e[0] < 105 || e[0] > 110 || e[1] < 150 || e[1] > 160 {
		t.Fatalf("tight extent %v does not isolate the number", e)
	}
	if e := ext[1]; e[0] > 100 || e[1] < 160 {
		t.Fatalf("loose extent %v must keep the whole word", e)
	}
}

func TestAllEqual(t *testing.T) {
	a, b := anchoredValue{Value: 0.2}, anchoredValue{Value: 2}
	if !allEqual([]anchoredValue{a}) || !allEqual([]anchoredValue{a, a}) || allEqual([]anchoredValue{a, b}) {
		t.Fatal("allEqual must require every reading to match")
	}
}

func TestCrossCheckAstigmatism(t *testing.T) {
	ok := map[string]any{"k1_d": 43.5, "k2_d": 44.0, "astigmatism_d": 0.5}
	if c := crossCheckAstigmatism(ok); len(c) != 0 || ok["astigmatism_d"] != 0.5 {
		t.Fatal("consistent astigmatism must stay")
	}
	bad := map[string]any{"k1_d": 43.5, "k2_d": 44.0, "astigmatism_d": 5.0}
	if c := crossCheckAstigmatism(bad); len(c) != 1 || c[0].Status != "conflicting" || bad["astigmatism_d"] != nil {
		t.Fatal("astigmatism read as 5 instead of 0.5 must be removed and conflict")
	}
}

func TestDominantReading(t *testing.T) {
	mk := func(v float64, votes int) anchoredValue {
		return anchoredValue{Value: v, Hint: FieldHint{Votes: votes}}
	}
	if d := dominant([]anchoredValue{mk(45.3, 19), mk(53.1, 6)}); d == nil || d.Value != 45.3 {
		t.Fatal("a reading with 3x the votes must win")
	}
	if dominant([]anchoredValue{mk(45.3, 19), mk(53.1, 10)}) != nil {
		t.Fatal("close readings must stay conflicting")
	}
	if dominant([]anchoredValue{mk(45.3, 8), mk(53.1, 1)}) != nil {
		t.Fatal("weak evidence must stay conflicting")
	}
}

func TestLeadingNonDigits(t *testing.T) {
	for text, want := range map[string]int{"[12": 1, "fi2": 2, "}[45.5D": 2, "534": 0, "[aan": 4} {
		if got := leadingNonDigits(text); got != want {
			t.Errorf("leadingNonDigits(%q)=%d want %d", text, got, want)
		}
	}
}
