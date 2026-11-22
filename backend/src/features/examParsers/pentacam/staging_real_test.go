package pentacam

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"refratia/backend/src/shared/ocr"
	pdfutil "refratia/backend/src/shared/pdf"
)

// Real documents live in the gitignored frontend/data folder; absent -> skip.
// Every Pentacam prints the ABCD staging line under the chart of the topometric screen.
func TestReadStagingFromRealDocuments(t *testing.T) {
	files, _ := filepath.Glob("../../../../../frontend/data/*/PENTACAM*.pdf")
	if len(files) == 0 {
		t.Skip("no real Pentacam documents available")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		pages, _, _ := pdfutil.Inspect(ctx, data)
		screens, _ := DetectScreens(ctx, data, pages)
		page := screens[ScreenTopometric]
		rendered, err := pdfutil.RenderPageAtDPI(ctx, data, page, 450)
		if err != nil {
			t.Fatal(err)
		}
		img, err := ocr.DecodeImage(rendered)
		if err != nil {
			t.Fatal(err)
		}
		values, _ := readAnchored(ctx, map[Screen]int{ScreenTopometric: page}, map[int]image.Image{page: img})
		// Printed lines read visually: priscila-neres carries "+" grades (the
		// device prints no space after a "+"); every other folder prints all zeros.
		want := "A0 B0 C0 D"
		if filepath.Base(filepath.Dir(f)) == "priscila-neres" {
			want = map[bool]string{true: "A3 B4+ C4+ D", false: "A3 B4+ C3 D"}[strings.Contains(filepath.Base(f), " OD ")]
		}
		if got := values["abcd_staging"].Text; got != want {
			t.Errorf("%s/%s: staging line %q, want %q", filepath.Base(filepath.Dir(f)), filepath.Base(f), got, want)
		}
	}
}

func isStaging(s string) bool { got, ok := parseStaging(s); return ok && got == s }

func TestParseStaging(t *testing.T) {
	cases := map[string]string{
		"A2 B1 C3 D2": "A2 B1 C3 D2", "A0 B0 C0 D": "A0 B0 C0 D", "AO BO COD": "A0 B0 C0 D",
		"a4 b4 c4 d4": "A4 B4 C4 D4",
		"A3 B4+C4+D":  "A3 B4+ C4+ D", "A3 B4+C3 D": "A3 B4+ C3 D", "A3 B4+ C4+ D4+": "A3 B4+ C4+ D4+", "A0B0C0D": "A0 B0 C0 D",
	}
	for in, want := range cases {
		if got, ok := parseStaging(in); !ok || got != want {
			t.Errorf("%q: got %q %v, want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "A5 B0 C0 D", "A0 B0 C0", "A0 B0 C0 D5", "0 1 2 3 4", "Asfericidade", "B0 A0 C0 D", "A0 B0 C0 D0 D0", "A3 B4++ C4 D", "A3 B+ C4 D", "A3 B5+ C4 D"} {
		if got, ok := parseStaging(in); ok {
			t.Errorf("%q must not parse, got %q", in, got)
		}
	}
}

func TestStagingShapes(t *testing.T) {
	for _, row := range []string{"A0 BO COD", "A3 B4 C4 D", "AO BO CO D"} {
		if !stagingRow.MatchString(row) {
			t.Errorf("%q must look like a staging line", row)
		}
	}
	for _, row := range []string{"Asfericidade (Ant.) a 6mm", "0 1 2 3 4", "A0 B0"} {
		if stagingRow.MatchString(row) {
			t.Errorf("%q must not look like a staging line", row)
		}
	}
}
