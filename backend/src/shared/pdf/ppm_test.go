package pdf

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func decodeToRGBA(t *testing.T, pngData []byte) *image.RGBA {
	t.Helper()
	decoded, decodeError := png.Decode(bytes.NewReader(pngData))
	if decodeError != nil {
		t.Fatal(decodeError)
	}
	rgba := image.NewRGBA(decoded.Bounds())
	draw.Draw(rgba, rgba.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	return rgba
}

// RenderPageAtDPI must decode to exactly the pixels of `pdftoppm -png`.
func TestRenderPageAtDPIMatchesPdftoppmPNG(t *testing.T) {
	if os.Getenv("REFRATIA_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("teste de integração desabilitado; use REFRATIA_RUN_INTEGRATION_TESTS=1")
	}
	pdfPath := "../../../../frontend/data/maria-gorete/PENTACAM OD MARIA GORETE.pdf"
	pdfData, readError := os.ReadFile(pdfPath)
	if readError != nil {
		t.Skip(readError)
	}
	for _, render := range []struct{ page, dpi int }{{1, 100}, {8, 450}} {
		prefix := filepath.Join(t.TempDir(), "page")
		command := exec.Command("pdftoppm", "-f", strconv.Itoa(render.page), "-singlefile", "-png", "-r", strconv.Itoa(render.dpi), pdfPath, prefix)
		if output, runError := command.CombinedOutput(); runError != nil {
			t.Fatalf("pdftoppm: %v %s", runError, output)
		}
		expectedPNG, _ := os.ReadFile(prefix + ".png")
		gotPNG, renderError := RenderPageAtDPI(context.Background(), pdfData, render.page, render.dpi)
		if renderError != nil {
			t.Fatal(renderError)
		}
		expected, got := decodeToRGBA(t, expectedPNG), decodeToRGBA(t, gotPNG)
		if expected.Rect != got.Rect || !bytes.Equal(expected.Pix, got.Pix) {
			t.Fatalf("page %d at %d DPI: pixels differ from pdftoppm -png", render.page, render.dpi)
		}
	}
}

func TestRenderAllPagesAtDPIMatchesPdftoppmPNG(t *testing.T) {
	if os.Getenv("REFRATIA_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("teste de integração desabilitado; use REFRATIA_RUN_INTEGRATION_TESTS=1")
	}
	pdfPath := "../../../../frontend/data/maria-gorete/PENTACAM OD MARIA GORETE.pdf"
	pdfData, readError := os.ReadFile(pdfPath)
	if readError != nil {
		t.Skip(readError)
	}
	pageCount, _, inspectError := Inspect(context.Background(), pdfData)
	if inspectError != nil {
		t.Fatal(inspectError)
	}
	prefix := filepath.Join(t.TempDir(), "page")
	if output, runError := exec.Command("pdftoppm", "-png", "-r", "100", pdfPath, prefix).CombinedOutput(); runError != nil {
		t.Fatalf("pdftoppm: %v %s", runError, output)
	}
	gotPages, renderError := RenderAllPagesAtDPI(context.Background(), pdfData, pageCount, 100)
	if renderError != nil {
		t.Fatal(renderError)
	}
	for pageIndex, gotPNG := range gotPages {
		expectedPNG, _ := os.ReadFile(prefix + "-" + fmt.Sprintf("%0*d", len(strconv.Itoa(pageCount)), pageIndex+1) + ".png")
		expected, got := decodeToRGBA(t, expectedPNG), decodeToRGBA(t, gotPNG)
		if expected.Rect != got.Rect || !bytes.Equal(expected.Pix, got.Pix) {
			t.Fatalf("page %d: pixels differ from pdftoppm -png", pageIndex+1)
		}
	}
}

func TestPNGFromPPMRejectsGarbage(t *testing.T) {
	if _, convertError := pngFromPPM([]byte("P5\n2 2\n255\n....")); convertError == nil {
		t.Fatal("expected an error for a non-P6 image")
	}
	if _, convertError := pngFromPPM([]byte("P6\n2 2\n255\nabc")); convertError == nil {
		t.Fatal("expected an error for a truncated raster")
	}
}
