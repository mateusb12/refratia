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
	for _, e := range ext {
		if e[0] < 105 || e[0] > 110 || e[1] < 150 || e[1] > 160 {
			t.Fatalf("extent %v does not isolate the number", e)
		}
	}
}

func TestAllEqual(t *testing.T) {
	a, b := anchoredValue{Value: 0.2}, anchoredValue{Value: 2}
	if !allEqual([]anchoredValue{a}) || !allEqual([]anchoredValue{a, a}) || allEqual([]anchoredValue{a, b}) {
		t.Fatal("allEqual must require every reading to match")
	}
}
