package pentacam

import (
	"path/filepath"
	"strings"
	"testing"
)

// TKC cells read visually on every local Pentacam (independent audit): the
// priscila-neres reports print "KK 2-3" (OD) and "KK 2" (OS) on red cells;
// every other folder prints the dash. A dash must come back as the
// printed_dash hint with no value, a class as its canonical KC value, and
// never as a bare number.
func TestTKCFromRealDocuments(t *testing.T) {
	files, _ := filepath.Glob("../../../../../frontend/data/*/PENTACAM*.pdf")
	if len(files) == 0 {
		t.Skip("no real Pentacam documents available")
	}
	for _, f := range files {
		exam, hints := cachedExtractPDFWithHints(t, f)
		topometric, _ := exam["topometric_indices_8mm"].(map[string]any)
		got := topometric["tkc"]
		dash := false
		for _, h := range hints {
			dash = dash || h.Spec == "tkc" && h.Status == "not_applicable" && h.Strategy == "printed_dash"
		}
		name := filepath.Base(filepath.Dir(f)) + "/" + filepath.Base(f)
		if filepath.Base(filepath.Dir(f)) == "priscila-neres" {
			want := map[bool]string{true: "KC2-3", false: "KC2"}[strings.Contains(filepath.Base(f), " OD ")]
			if got != want || dash {
				t.Errorf("%s: tkc %#v (dash hint %v), want %q", name, got, dash, want)
			}
			continue
		}
		if got != nil || !dash {
			t.Errorf("%s: tkc %#v (dash hint %v), want the printed dash", name, got, dash)
		}
	}
}
