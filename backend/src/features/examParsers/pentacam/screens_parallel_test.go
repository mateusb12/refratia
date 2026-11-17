package pentacam

import (
	"context"
	"errors"
	"image"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"refratia/backend/src/shared/document"
	"refratia/backend/src/shared/ocr"
	pdfutil "refratia/backend/src/shared/pdf"
)

// serialScreens is DetectScreens as it was before the page reads ran
// concurrently: one page after another, first page wins.
func serialScreens(t *testing.T, executionContext context.Context, data []byte, pageCount int) map[Screen]int {
	pages, renderError := pdfutil.RenderAllPagesAtDPI(executionContext, data, pageCount, 100)
	if renderError != nil {
		t.Fatal(renderError)
	}
	screens := map[Screen]int{}
	for index, pagePNG := range pages {
		pageImage, decodeError := ocr.DecodeImage(pagePNG)
		if decodeError != nil {
			continue
		}
		bounds := pageImage.Bounds()
		band := image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Min.Y+bounds.Dy()*45/100)
		words, ocrError := document.OCRRegionWords(executionContext, pageImage, band, 2, 6)
		if ocrError != nil {
			continue
		}
		if screen := ScreenForTitle(titleAfterHeader(words, 12)); screen != "" {
			if _, seen := screens[screen]; !seen {
				screens[screen] = index + 1
			}
		}
	}
	return screens
}

func corpusPentacamPDFs(t *testing.T) [][]byte {
	t.Helper()
	if os.Getenv("REFRATIA_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("teste de integração desabilitado; use REFRATIA_RUN_INTEGRATION_TESTS=1")
	}
	paths, _ := filepath.Glob("../../../../../frontend/data/*/*")
	var pdfs [][]byte
	for _, path := range paths {
		name := strings.ToLower(filepath.Base(path))
		if strings.Contains(name, "pentacam") && strings.HasSuffix(name, ".pdf") {
			data, readError := os.ReadFile(path)
			if readError != nil {
				t.Fatal(readError)
			}
			pdfs = append(pdfs, data)
		}
	}
	if len(pdfs) == 0 {
		t.Skip("corpus not available")
	}
	return pdfs
}

// Every corpus report maps to the same screens as the serial loop, also while
// three documents detect screens at once (more readers than half the engines).
func TestDetectScreensMatchesSerialUnderContention(t *testing.T) {
	pdfs := corpusPentacamPDFs(t)
	executionContext := context.Background()
	references := make([]map[Screen]int, len(pdfs))
	for index, data := range pdfs {
		pageCount, _, inspectError := pdfutil.Inspect(executionContext, data)
		if inspectError != nil {
			t.Fatal(inspectError)
		}
		references[index] = serialScreens(t, executionContext, data, pageCount)
		got, detectError := DetectScreens(executionContext, data, pageCount)
		if detectError != nil || !maps.Equal(got, references[index]) {
			t.Fatalf("report %d: got %v (%v), serial %v", index, got, detectError, references[index])
		}
	}

	var group sync.WaitGroup
	for range 3 {
		group.Add(1)
		go func() {
			defer group.Done()
			for index, data := range pdfs {
				pageCount, _, _ := pdfutil.Inspect(executionContext, data)
				if got, detectError := DetectScreens(executionContext, data, pageCount); detectError != nil || !maps.Equal(got, references[index]) {
					t.Errorf("report %d under contention: got %v (%v)", index, got, detectError)
				}
			}
		}()
	}
	group.Wait()
}

// A canceled request stops reading pages and reports the cancellation instead
// of a partial screen map.
func TestDetectScreensCanceled(t *testing.T) {
	data := corpusPentacamPDFs(t)[0]
	pageCount, _, _ := pdfutil.Inspect(context.Background(), data)
	pages, _ := pdfutil.RenderAllPagesAtDPI(context.Background(), data, pageCount, 100)
	if len(pages) <= screenReadWorkers {
		t.Skip("report too short to leave queued pages")
	}

	executionContext, cancel := context.WithCancel(context.Background())
	cancel()
	// The render (unchanged) fails first, with poppler's own error.
	if got, detectError := DetectScreens(executionContext, data, pageCount); detectError == nil || got != nil {
		t.Fatalf("canceled before start: got %v, %v", got, detectError)
	}

	// Cancel after the render, while the first page reads are in flight.
	executionContext, cancel = context.WithCancel(context.Background())
	time.AfterFunc(900*time.Millisecond, cancel)
	started := time.Now()
	got, detectError := DetectScreens(executionContext, data, pageCount)
	elapsed := time.Since(started)
	if detectError == nil {
		t.Skipf("finished in %v before the cancel landed", elapsed)
	}
	if !errors.Is(detectError, context.Canceled) || got != nil {
		t.Fatalf("canceled mid-flight: got %v, %v", got, detectError)
	}
	t.Logf("canceled at 900ms, returned after %v", elapsed.Round(time.Millisecond))
}
