package cases

import "testing"

func TestValidIDMatchesIntakeStampShape(t *testing.T) {
	for caseID, want := range map[string]bool{
		"case-20260815T155045Z-da4bf173":   true,
		"intake-20260815T155045Z-da4bf173": false,
		"case-20260815T155045Z-zz4bf173":   false,
		"case-20260815T155045Z-da4bf17":    false,
		"case-../../x":                     false,
		"case-":                            false,
	} {
		if got := ValidID(caseID); got != want {
			t.Errorf("ValidID(%q)=%v want %v", caseID, got, want)
		}
	}
}
