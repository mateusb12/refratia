//go:build cgo

package ocr

import (
	"context"
	"errors"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	pdfutil "refratia/backend/src/shared/pdf"
)

// Many more callers than engines, a third of them canceled while waiting:
// every completed read equals the uncontended one, canceled callers get the
// context error, and every engine is back in the pool afterwards.
func TestEnginePoolUnderContentionAndCancel(t *testing.T) {
	if os.Getenv("REFRATIA_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("teste de integração desabilitado; use REFRATIA_RUN_INTEGRATION_TESTS=1")
	}
	pdfData, readError := os.ReadFile("../../../../frontend/data/maria-gorete/PENTACAM OD MARIA GORETE.pdf")
	if readError != nil {
		t.Skip(readError)
	}
	pagePNG, renderError := pdfutil.RenderPageAtDPI(context.Background(), pdfData, 1, 100)
	if renderError != nil {
		t.Fatal(renderError)
	}
	reference, referenceError := Recognize(context.Background(), pagePNG, 6, "", false)
	if referenceError != nil {
		t.Fatal(referenceError)
	}

	callers := 4 * runtime.GOMAXPROCS(0)
	var group sync.WaitGroup
	var mutex sync.Mutex
	done, canceled := 0, 0
	for index := range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			executionContext := context.Background()
			if index%3 == 0 {
				var cancel context.CancelFunc
				executionContext, cancel = context.WithTimeout(executionContext, 50*time.Millisecond)
				defer cancel()
			}
			got, recognizeError := Recognize(executionContext, pagePNG, 6, "", false)
			mutex.Lock()
			defer mutex.Unlock()
			switch {
			case errors.Is(recognizeError, context.DeadlineExceeded):
				canceled++
			case recognizeError != nil:
				t.Errorf("caller %d: %v", index, recognizeError)
			case got != reference:
				t.Errorf("caller %d: read differs under contention", index)
			default:
				done++
			}
		}()
	}
	group.Wait()
	if idle := len(enginePool); idle != cap(enginePool) {
		t.Fatalf("%d of %d engines back in the pool", idle, cap(enginePool))
	}
	if canceled == 0 {
		t.Fatal("no caller was canceled while waiting; the test did not exercise cancellation")
	}
	t.Logf("%d callers on %d engines: %d read, %d canceled while waiting", callers, cap(enginePool), done, canceled)
}
