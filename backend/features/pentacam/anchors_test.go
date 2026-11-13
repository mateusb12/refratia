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
	ok := map[string]any{"k1_d": 43.5, "k2_d": 44.0, "km_d": 43.8, "astigmatism_d": 0.5}
	if c := crossCheckAstigmatism(ok); len(c) != 0 || ok["astigmatism_d"] != 0.5 {
		t.Fatal("consistent keratometry must stay")
	}
	// Km agrees with K1/K2: the astigmatism is the odd one out.
	bad := map[string]any{"k1_d": 43.5, "k2_d": 44.0, "km_d": 43.8, "astigmatism_d": 5.0}
	if c := crossCheckAstigmatism(bad); len(c) != 1 || c[0].Status != "conflicting" || bad["astigmatism_d"] != nil || bad["k1_d"] != 43.5 {
		t.Fatal("astigmatism read as 5 instead of 0.5 must be removed alone")
	}
	// Without Km nobody can tell which side is wrong: none stays valid.
	blind := map[string]any{"k1_d": 42.7, "k2_d": 44.1, "astigmatism_d": 2.0}
	if c := crossCheckAstigmatism(blind); len(c) != 3 || blind["k1_d"] != nil || blind["k2_d"] != nil || blind["astigmatism_d"] != nil {
		t.Fatalf("inconsistent K1/K2/astigmatism without Km must all go, got %d", len(c))
	}
}

func TestKeratoResolve(t *testing.T) {
	// K1 misread 1 -> 7 (majority); Km and astigmatism both point at 42.1.
	got := keratoResolve(
		map[string]float64{"k1_d": 42.7, "k2_d": 44.1, "km_d": 43.1, "astigmatism_d": 2},
		map[string]map[float64]int{"k1_d": {42.7: 11, 42.1: 1}, "k2_d": {44.1: 11}, "km_d": {43.1: 13, 43.7: 5}, "astigmatism_d": {2: 29}},
	)
	if got["k1_d"] != 42.1 || got["km_d"] != 43.1 {
		t.Fatalf("expected the consistent assignment, got %v", got)
	}
	// Astigmatism lost its leading "1.": the printed K cells fix it.
	got = keratoResolve(
		map[string]float64{"k1_d": 43.9, "k2_d": 45.1, "km_d": 44.5, "astigmatism_d": 2},
		map[string]map[float64]int{"astigmatism_d": {2: 6, 1.2: 5}},
	)
	if got["astigmatism_d"] != 1.2 {
		t.Fatalf("got %v", got)
	}
	// Consistent accepted values are never overridden; nothing to resolve.
	if got := keratoResolve(
		map[string]float64{"k1_d": 43.5, "k2_d": 44.0, "km_d": 43.8, "astigmatism_d": 0.5},
		map[string]map[float64]int{"astigmatism_d": {0.5: 9, 5: 3}}); got != nil {
		t.Fatalf("got %v", got)
	}
	// Rounding edge: printed values are multiples of 0.1, so a legitimate
	// mismatch is exactly 0.1 (francisco OS shape) and 0.2 is a misread.
	if k := keratoDrop(map[string]float64{"k1_d": 42.9, "k2_d": 44.0, "km_d": 43.5, "astigmatism_d": 1.0}); len(k) != 0 {
		t.Fatalf("0.1 edge must hold, dropped %v", k)
	}
	if k := keratoDrop(map[string]float64{"k1_d": 43.0, "k2_d": 44.2, "km_d": 43.7}); len(k) != 0 {
		t.Fatalf("km 0.1 off must hold, dropped %v", k)
	}
	if k := keratoDrop(map[string]float64{"k1_d": 43.0, "k2_d": 44.2, "km_d": 43.8}); len(k) == 0 {
		t.Fatal("km 0.2 off must fail")
	}
	if k := keratoDrop(map[string]float64{"k1_d": 43.0, "k2_d": 44.2, "km_d": 43.6, "astigmatism_d": 1.0}); len(k) != 1 || k[0] != "astigmatism_d" {
		t.Fatalf("0.2 apart must drop only the odd key, got %v", k)
	}
	// Never computed: no read candidate for the astigmatism means no value, even
	// with K1/K2/Km known (the printed cell is rounded independently).
	if got := keratoResolve(
		map[string]float64{"k1_d": 42.9, "k2_d": 44.0, "km_d": 43.4},
		map[string]map[float64]int{}); got["astigmatism_d"] != 0 || len(got) != 0 {
		t.Fatalf("astigmatism must not be derived, got %v", got)
	}
	// Ambiguous: two candidates both fit, no answer.
	if got := keratoResolve(
		map[string]float64{"k1_d": 43.5, "k2_d": 44.0, "km_d": 43.8},
		map[string]map[float64]int{"astigmatism_d": {0.5: 9, 0.6: 9}}); got != nil {
		t.Fatalf("got %v", got)
	}
}

func TestLeadingNonDigits(t *testing.T) {
	for text, want := range map[string]int{"[12": 1, "fi2": 2, "}[45.5D": 2, "534": 0, "[aan": 4} {
		if got := leadingNonDigits(text); got != want {
			t.Errorf("leadingNonDigits(%q)=%d want %d", text, got, want)
		}
	}
}
