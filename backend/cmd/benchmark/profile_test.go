package main

import "testing"

func TestUnionMergesOverlapsAndClips(t *testing.T) {
	iv := []interval{{10, 20}, {15, 30}, {40, 50}, {0, 5}}
	if got := union(iv, 0, 100); got != 5+20+10 {
		t.Fatalf("union = %d, want 35", got)
	}
	if got := union(iv, 12, 45); got != 18+5 {
		t.Fatalf("clipped union = %d, want 23", got)
	}
}

func TestSweepSplitsWallTimeByWhatIsInFlight(t *testing.T) {
	// two overlapping "a" calls, then "b" alone, then nothing until hi
	ev := []tagged{{0, 10, "a"}, {5, 15, "a"}, {15, 20, "b"}}
	var total, twoA, onlyB, idle int64
	sweep(ev, 0, 25, func(dt int64, act map[string]int) {
		total += dt
		switch {
		case act["a"] == 2:
			twoA += dt
		case act["b"] == 1 && act["a"] == 0:
			onlyB += dt
		case act["a"] == 0 && act["b"] == 0:
			idle += dt
		}
	})
	if total != 25 || twoA != 5 || onlyB != 5 || idle != 5 {
		t.Fatalf("total %d twoA %d onlyB %d idle %d, want 25 5 5 5", total, twoA, onlyB, idle)
	}
}
