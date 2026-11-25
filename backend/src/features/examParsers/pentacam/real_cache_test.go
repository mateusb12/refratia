package pentacam

import (
	"context"
	"os"
	"sync"
	"testing"
)

// Real-document tests share one ExtractPDF per PDF path per test binary run:
// each extraction takes ~25 s, and several tests read the same documents.
type extractEntry struct {
	once  sync.Once
	exam  map[string]any
	hints []FieldHint
	err   error
}

var (
	extractCacheMu sync.Mutex
	extractCache   = map[string]*extractEntry{}
)

func cachedExtractPDF(t *testing.T, path string) map[string]any {
	t.Helper()
	exam, _ := cachedExtractPDFWithHints(t, path)
	return exam
}

func cachedExtractPDFWithHints(t *testing.T, path string) (map[string]any, []FieldHint) {
	t.Helper()
	extractCacheMu.Lock()
	entry := extractCache[path]
	if entry == nil {
		entry = &extractEntry{}
		extractCache[path] = entry
	}
	extractCacheMu.Unlock()
	entry.once.Do(func() {
		data, err := os.ReadFile(path)
		if err != nil {
			entry.err = err
			return
		}
		entry.exam, entry.hints, entry.err = ExtractPDFWithHints(context.Background(), data)
	})
	if entry.err != nil {
		t.Fatalf("%s: %v", path, entry.err)
	}
	return entry.exam, entry.hints
}
