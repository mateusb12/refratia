package eyesuite

import (
	"testing"

	"refratia/backend/shared/ocr"
)

func rowWords(top int, texts ...string) []ocr.Word {
	var words []ocr.Word
	for i, text := range texts {
		words = append(words, ocr.Word{Text: text, Left: 10 + i*60, Top: top, Width: 50, Height: 20})
	}
	return words
}

// One unreadable label must not discard the fields that were read (F16).
func TestParseEyeSuiteEyeReturnsPartialResults(t *testing.T) {
	var words []ocr.Word
	words = append(words, rowWords(10, "AL:", "23.49", "mm")...)
	words = append(words, rowWords(60, "K1:", "44.15", "D")...)
	words = append(words, rowWords(110, "ACD:", "3.74", "mm")...) // LT, WTW, K2... absent

	got, err := parseEyeSuiteEye(words)
	if err != nil {
		t.Fatalf("partial eye must not error: %v", err)
	}
	if got["axial_length_mm"] != 23.49 || got["anterior_chamber_depth_mm"] != 3.74 {
		t.Fatalf("read fields lost: %v", got)
	}
	if _, present := got["lens_thickness_mm"]; present {
		t.Fatal("unread field must be absent, not zero")
	}
	if got["keratometry"].(map[string]any)["k1_d"] != 44.15 {
		t.Fatalf("keratometry lost: %v", got)
	}
}

func TestParseEyeSuiteEyeEmptyIsAnError(t *testing.T) {
	if _, err := parseEyeSuiteEye(rowWords(10, "nothing", "here")); err == nil {
		t.Fatal("an eye with no field at all must be an error")
	}
}

func TestParseEyeSuiteEyeReadsRadiiAxesCCTADAndStatus(t *testing.T) {
	var words []ocr.Word
	words = append(words, rowWords(10, "Phakic")...)
	words = append(words, rowWords(40, "CCT", "[um]", "534", "K2[D/mm/°]", "45,20/7,47", "@", "78")...)
	words = append(words, rowWords(70, "AD", "[mm]", "3,21", "K", "[D/mm]", "44,67/7,56")...)
	words = append(words, rowWords(100, "AL", "[mm]", "23,49", "K1[D/mm/°]", "44,15/7,64", "@", "168")...)
	got, err := parseEyeSuiteEye(words)
	if err != nil {
		t.Fatal(err)
	}
	k := got["keratometry"].(map[string]any)
	if got["status"] != "Phakic" || got["central_corneal_thickness_um"] != 534.0 || got["aqueous_depth_mm"] != 3.21 ||
		k["k1_radius_mm"] != 7.64 || k["k1_axis_deg"] != 168.0 || k["k2_radius_mm"] != 7.47 || k["k2_axis_deg"] != 78.0 ||
		k["mean_radius_mm"] != 7.56 || k["mean_k_d"] != 44.67 {
		t.Fatalf("unexpected: %v", got)
	}
}
