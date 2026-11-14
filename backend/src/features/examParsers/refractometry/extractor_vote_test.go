package refractometry

import (
	"testing"
)

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

func TestMedoidAxisIgnoresOneMisreadRow(t *testing.T) {
	if got := medoidAxis([][3]float64{{0, 0, 102}, {0, 0, 101}, {0, 0, 180}}); axisDistance(got, 101) > 1.5 {
		t.Fatalf("got %v", got)
	}
	if got := medoidAxis([][3]float64{{0, 0, 178}, {0, 0, 2}, {0, 0, 1}}); axisDistance(got, 1) > 2 {
		t.Fatalf("axes wrap at 180, got %v", got)
	}
}

func TestZeroConfusionsRecoverSlashedZero(t *testing.T) {
	has := func(token string, want float64) bool {
		for _, c := range zeroConfusions(token) {
			if c.value == want && validOnGrid(0, want) || c.value == want && want <= 180 {
				return true
			}
		}
		return false
	}
	if !has("+5.08", 5.00) || !has("-1.58", -1.50) || !has("188", 100) {
		t.Fatal("8 read for a slashed zero must be recoverable")
	}
	// The expansion never turns a digit into anything but 0.
	for _, c := range zeroConfusions("1.25") {
		if c.value != 1.25 {
			t.Fatalf("unexpected candidate %v", c)
		}
	}
}

func TestMajority(t *testing.T) {
	if v, ok := majority(map[float64]int{-1.5: 5, -1: 3}, 9); !ok || v != -1.5 {
		t.Fatal("strict majority expected")
	}
	if _, ok := majority(map[float64]int{-1.5: 4, -1: 4}, 9); ok {
		t.Fatal("no majority expected")
	}
}
