package refractometry

import "testing"

func TestStrictAveragePreservesSignAndPrintedFormat(t *testing.T) {
	cases := []struct {
		text string
		want [3]string
		ok   bool
	}{
		{"AVE-3.50-6.7515", [3]string{"-3.50", "-6.75", "15"}, true},
		{"AVE+5.00-1.50100", [3]string{"+5.00", "-1.50", "100"}, true},
		{"AVE -8.00 -7.25 172", [3]string{"-8.00", "-7.25", "172"}, true},
		{"AVE-3.5", [3]string{}, false},
	}
	for _, c := range cases {
		m := strictAverage.FindStringSubmatch(c.text)
		if (m != nil) != c.ok {
			t.Fatalf("%q: match=%v", c.text, m != nil)
		}
		if c.ok && (m[1] != c.want[0] || m[2] != c.want[1] || m[3] != c.want[2]) {
			t.Fatalf("%q: got %v", c.text, m[1:])
		}
	}
}

func TestOnQuarterGrid(t *testing.T) {
	for v, want := range map[float64]bool{-1.5: true, 5: true, -6.75: true, -1.58: false, -1.9: false} {
		if onQuarterGrid(v) != want {
			t.Fatalf("onQuarterGrid(%v) != %v", v, want)
		}
	}
}
