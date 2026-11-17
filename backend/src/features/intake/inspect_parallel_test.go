package intake

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"refratia/backend/src/features/intake/progressTracking"
	"refratia/backend/src/shared/database"
)

type upload struct {
	name, contentType string
	data              []byte
}

func inspectRequest(executionContext context.Context, files []upload) *http.Request {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, file := range files {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="files"; filename="`+file.name+`"`)
		header.Set("Content-Type", file.contentType)
		part, _ := form.CreatePart(header)
		part.Write(file.data)
	}
	form.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/intakes/inspect", &body).WithContext(executionContext)
	request.Header.Set("Content-Type", form.FormDataContentType())
	return request
}

// francisco's folder: 5 photos (specular detection with retries, ~7 s each
// serially) and 3 PDFs. Names are replaced by their position.
func corpusUploads(t *testing.T) []upload {
	t.Helper()
	if os.Getenv("REFRATIA_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("teste de integração desabilitado; use REFRATIA_RUN_INTEGRATION_TESTS=1")
	}
	paths, _ := filepath.Glob("../../../../frontend/data/francisco/*")
	sort.Strings(paths)
	var files []upload
	for index, path := range paths {
		contentType := map[string]string{".pdf": "application/pdf", ".jpeg": "image/jpeg", ".jpg": "image/jpeg"}[strings.ToLower(filepath.Ext(path))]
		if contentType == "" {
			continue
		}
		data, readError := os.ReadFile(path)
		if readError != nil {
			t.Fatal(readError)
		}
		files = append(files, upload{name: string(rune('a'+index)) + filepath.Ext(path), contentType: contentType, data: data})
	}
	if len(files) < 4 {
		t.Skip("corpus not available")
	}
	return files
}

// A request canceled before inspection starts inspects nothing and writes
// nothing (the client is gone).
func TestInspectCanceledBeforeStartWritesNothing(t *testing.T) {
	var blank bytes.Buffer
	png.Encode(&blank, image.NewGray(image.Rect(0, 0, 40, 20)))
	executionContext, cancel := context.WithCancel(context.Background())
	cancel()
	recorder := httptest.NewRecorder()
	InspectIntakeHandler(recorder, inspectRequest(executionContext, []upload{
		{"a.png", "image/png", blank.Bytes()}, {"b.png", "image/png", blank.Bytes()},
	}))
	if recorder.Body.Len() != 0 {
		t.Fatalf("canceled inspect wrote a body: %s", recorder.Body.String())
	}
}

// Canceling mid-flight: files not yet started are skipped, the handler
// returns once the in-flight files finish (a running recognition is not
// interrupted), and the audit records the cancellation instead of a 200.
func TestInspectCanceledMidFlightSkipsQueuedFilesAndIsAudited(t *testing.T) {
	files := corpusUploads(t)
	integrationSetup(t)
	executionContext, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)
	audited := progressTracking.Handler{Database: testHandler.Database}.Audit("inspect", InspectIntakeHandler)
	recorder := httptest.NewRecorder()
	started := time.Now()
	audited(recorder, inspectRequest(executionContext, files))
	elapsed := time.Since(started)

	if recorder.Body.Len() != 0 {
		t.Fatalf("canceled inspect wrote a body (%d bytes)", recorder.Body.Len())
	}
	row, details := latestAudit(t, "inspect")
	if row.Status != database.RunFailed || !strings.Contains(details.Error, "canceled") {
		t.Fatalf("audit status %q error %q, want failed / context canceled", row.Status, details.Error)
	}
	unprocessed := 0
	for _, file := range details.Files {
		if file.Outcome == "" {
			unprocessed++
		}
	}
	if len(details.Files) != len(files) || unprocessed == 0 {
		t.Fatalf("audit has %d files, %d unprocessed; want %d files and some skipped", len(details.Files), unprocessed, len(files))
	}
	t.Logf("returned %v after cancel at 300ms; %d of %d files skipped", elapsed.Round(time.Millisecond), unprocessed, len(files))
	if elapsed > 15*time.Second {
		t.Fatalf("canceled inspect took %v; the serial inspect of this folder is ~35 s", elapsed)
	}
}

// Several inspects at once ask for 3 workers each, more than the engine pool
// holds (GOMAXPROCS). Each answer must equal the uncontended one byte for byte.
func TestParallelInspectUnderEnginePoolContention(t *testing.T) {
	files := corpusUploads(t)
	reference := httptest.NewRecorder()
	InspectIntakeHandler(reference, inspectRequest(context.Background(), files))
	if reference.Code != http.StatusOK || reference.Body.Len() == 0 {
		t.Fatalf("reference inspect: %d", reference.Code)
	}

	const concurrent = 3
	var group sync.WaitGroup
	bodies := make([][]byte, concurrent)
	for index := range concurrent {
		group.Add(1)
		go func() {
			defer group.Done()
			recorder := httptest.NewRecorder()
			InspectIntakeHandler(recorder, inspectRequest(context.Background(), files))
			bodies[index] = recorder.Body.Bytes()
		}()
	}
	group.Wait()
	for index, body := range bodies {
		if !bytes.Equal(body, reference.Body.Bytes()) {
			t.Fatalf("inspect %d under contention differs from the uncontended answer", index)
		}
	}
}
