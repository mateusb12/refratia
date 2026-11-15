//go:build cgo

package ocr_test

import (
	"context"
	"image"
	"os"
	"os/exec"
	"testing"

	"refratia/backend/src/shared/ocr"
	pdfutil "refratia/backend/src/shared/pdf"
)

// The resident engine must print byte-for-byte what the CLI printed, for every
// mode the extractors use, on crops of a real report.
func TestResidentEngineMatchesCLI(t *testing.T) {
	if os.Getenv("REFRATIA_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("teste de integração desabilitado; use REFRATIA_RUN_INTEGRATION_TESTS=1")
	}
	pdfData, readError := os.ReadFile("../../../../frontend/data/maria-gorete/PENTACAM OD MARIA GORETE.pdf")
	if readError != nil {
		t.Skip(readError)
	}
	executionContext := context.Background()
	pagePNG, renderError := pdfutil.RenderPageAtDPI(executionContext, pdfData, 1, 150)
	if renderError != nil {
		t.Fatal(renderError)
	}
	page, decodeError := ocr.DecodeImage(pagePNG)
	if decodeError != nil {
		t.Fatal(decodeError)
	}

	bounds := page.Bounds()
	crops := [][]byte{pagePNG}
	for _, region := range []image.Rectangle{
		image.Rect(0, 0, bounds.Dx()/3, bounds.Dy()/8),
		image.Rect(bounds.Dx()/3, bounds.Dy()/4, bounds.Dx()*2/3, bounds.Dy()/4+40),
		image.Rect(bounds.Dx()/2, bounds.Dy()/2, bounds.Dx()/2+120, bounds.Dy()/2+30),
	} {
		for _, threshold := range []uint8{0, 140} {
			cropPNG, encodeError := ocr.EncodePNG(ocr.PrepareGray(page, region, 3, threshold))
			if encodeError != nil {
				t.Fatal(encodeError)
			}
			crops = append(crops, cropPNG)
		}
	}

	// RunTesseractTSV used to pass no --psm at all.
	pageFile := t.TempDir() + "/page.png"
	if writeError := os.WriteFile(pageFile, pagePNG, 0o600); writeError != nil {
		t.Fatal(writeError)
	}
	defaultOutput, defaultError := exec.Command("tesseract", pageFile, "stdout", "tsv").Output()
	if defaultError != nil {
		t.Fatal(defaultError)
	}
	if got, _ := ocr.RunTesseractTSV(executionContext, pagePNG); got != string(defaultOutput) {
		t.Fatal("RunTesseractTSV differs from the CLI without --psm")
	}

	type mode struct {
		psm       int
		whitelist string
		tsv       bool
	}
	numeric := "0123456789.,+-"
	modes := []mode{
		{3, "", true}, {6, "", true}, {11, "", true},
		{6, "", false}, {7, "", false}, {11, "", false}, {12, "", false},
		{6, numeric, false}, {7, numeric, false}, {8, numeric, false}, {11, numeric, false}, {13, numeric, false},
	}
	for cropIndex, cropPNG := range crops {
		for _, current := range modes {
			if cropIndex == 0 && !current.tsv && current.psm != 6 {
				continue // the full page is only read as TSV / block text
			}
			expected, cliError := ocr.RecognizeWithCLIForTest(executionContext, cropPNG, current.psm, current.whitelist, current.tsv)
			if cliError != nil {
				t.Fatal(cliError)
			}
			got, engineError := ocr.Recognize(executionContext, cropPNG, current.psm, current.whitelist, current.tsv)
			if engineError != nil {
				t.Fatal(engineError)
			}
			if got != expected {
				t.Fatalf("crop %d mode %+v:\nengine %q\ncli    %q", cropIndex, current, got, expected)
			}
		}
	}
}
