package pentacam

import (
	"testing"

	"refratia/backend/src/features/fileIdentification/extractionfields"
)

func TestNormalizePrinted(t *testing.T) {
	d1 := extractionfields.Rule{Kind: "num", Decimals: 1, Min: 0, Max: 100, HasRange: true}
	d3signed := extractionfields.Rule{Kind: "num", Decimals: 3, Min: -0.5, Max: 1.5, HasRange: true}
	isv := extractionfields.Rule{Kind: "int", Decimals: 0, Min: 0, Max: 300, HasRange: true}
	cases := []struct {
		raw  string
		rule extractionfields.Rule
		want float64
		ok   bool
	}{
		{"[40.5 D", d1, 40.5, true},
		{"46D", d1, 4.6, true},     // dot lost by OCR
		{"}45.5D", d1, 45.5, true}, // border + unit
		{"6.0°", d1, 6.0, true},
		{"5.55", d1, 0, false},  // wrong number of decimals
		{"1.20", d1, 1.2, true}, // unit "D" read as a trailing zero
		{"0173", d3signed, 0.173, true},
		{"-0.173", d3signed, -0.173, true},
		{"-0173", d3signed, -0.173, true}, // sign preserved through re-insertion
		{"-173", d3signed, 0, false},      // too few digits to place the separator
		{"15", isv, 15, true},
		{"6.7", isv, 0, false},   // integer field with separator
		{"12 34", isv, 0, false}, // two numbers
		{"", isv, 0, false},
		{"999", isv, 0, false}, // out of range
	}
	for _, c := range cases {
		got, ok := normalizePrinted(c.raw, c.rule)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("normalizePrinted(%q) = %v,%v want %v,%v", c.raw, got, ok, c.want, c.ok)
		}
	}
}
