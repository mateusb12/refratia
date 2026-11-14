package pdf

import (
	"context"
	"os/exec"
	"testing"
)

// A PDF with 10+ pages makes pdftoppm zero-pad the page numbers; every page
// must still be returned.
func TestRenderAllPagesAtDPIHandlesZeroPaddedNames(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}
	data := blankPDF(12)
	pages, err := RenderAllPagesAtDPI(context.Background(), data, 12, 20)
	if err != nil || len(pages) != 12 {
		t.Fatalf("pages=%d err=%v", len(pages), err)
	}
}

// blankPDF builds a minimal n-page PDF.
func blankPDF(n int) []byte {
	var objs []string
	kids := ""
	for i := 0; i < n; i++ {
		kids += " " + itoa(3+i) + " 0 R"
	}
	objs = append(objs, "<< /Type /Catalog /Pages 2 0 R >>")
	objs = append(objs, "<< /Type /Pages /Kids ["+kids+" ] /Count "+itoa(n)+" >>")
	for i := 0; i < n; i++ {
		objs = append(objs, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 50 50] >>")
	}
	out := "%PDF-1.4\n"
	offsets := []int{}
	for i, o := range objs {
		offsets = append(offsets, len(out))
		out += itoa(i+1) + " 0 obj\n" + o + "\nendobj\n"
	}
	xref := len(out)
	out += "xref\n0 " + itoa(len(objs)+1) + "\n0000000000 65535 f \n"
	for _, off := range offsets {
		out += pad10(off) + " 00000 n \n"
	}
	out += "trailer\n<< /Size " + itoa(len(objs)+1) + " /Root 1 0 R >>\nstartxref\n" + itoa(xref) + "\n%%EOF\n"
	return []byte(out)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	s := ""
	for v > 0 {
		s = string(rune('0'+v%10)) + s
		v /= 10
	}
	return s
}

func pad10(v int) string {
	s := itoa(v)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}
