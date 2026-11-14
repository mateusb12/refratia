package examdate

import "testing"

func TestPlausibleBirth(t *testing.T) {
	cases := []struct {
		birth, exam string
		want        bool
	}{
		{"1979-12-14", "2026-11-04", true},
		{"2026-11-04", "2026-11-04", false}, // birth == exam: misread label
		{"2027-01-01", "2026-11-04", false}, // birth after exam
		{"2015-01-01", "2026-11-04", false}, // age 11
		{"1900-01-01", "2026-11-04", false}, // age > 110
		{"2008-11-05", "2026-11-04", false}, // one day short of 18
		{"2008-11-04", "2026-11-04", true},  // exactly 18
		{"", "2026-11-04", false},
	}
	for _, c := range cases {
		if got := PlausibleBirth(c.birth, c.exam); got != c.want {
			t.Errorf("PlausibleBirth(%q,%q)=%v want %v", c.birth, c.exam, got, c.want)
		}
	}
}

func TestFromSlashOrderUsesDeclaredOrder(t *testing.T) {
	if got := FromSlashOrder("05/12/1967", "MDY"); got != "1967-05-12" {
		t.Fatalf("MDY: %q", got)
	}
	if got := FromSlashOrder("05/12/1967", "DMY"); got != "1967-12-05" {
		t.Fatalf("DMY: %q", got)
	}
	if got := FromSlashOrder("13/12/1967", "MDY"); got != "" {
		t.Fatalf("invalid MDY must fail, got %q", got)
	}
}
