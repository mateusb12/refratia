package intake

import "testing"

func TestValidID(t *testing.T) {
	if !ValidID("intake-20260815T155045Z-da4bf173") {
		t.Fatal("expected generated intake ID to be valid")
	}

	if ValidID("../../case") {
		t.Fatal("expected invalid path-like ID to be rejected")
	}
}
