// Package progressTracking records every intake request in intake_runs and
// keeps an analyze running, and watchable, after the browser leaves: the
// audit record (Run), the progress stream (Stream) and the reconnect and
// cancel endpoints (Handler.ServeRun).
package progressTracking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"gorm.io/gorm"

	"refratia/backend/src/shared/database"
	"refratia/backend/src/shared/profiling"
)

// Run collects what one intake request did; Audit writes it to
// its intake_runs row when the request ends.
type Run struct {
	mutex    sync.Mutex
	intakeID string
	details  Details
	started  time.Time
	open     map[string]*File // per-file entries by filename, while processing
	runID    uint
	live     *liveRun // analyze only: the events a reconnecting browser replays
}

type Details struct {
	StartedAt  time.Time `json:"started_at"`
	DurationMS int64     `json:"duration_ms"`
	HTTPStatus int       `json:"http_status"`
	Error      string    `json:"error,omitempty"`
	CaseID     string    `json:"case_id,omitempty"`
	Action     string    `json:"action,omitempty"`
	AppCommit  string    `json:"app_commit,omitempty"`
	Files      []*File   `json:"files,omitempty"`
	Result     any       `json:"result,omitempty"` // analyze: the payload of the final "result" event
}

type File struct {
	Filename    string         `json:"filename"`
	SHA256      string         `json:"sha256"`
	ContentType string         `json:"content_type"`
	Size        int64          `json:"size"`
	ExamType    string         `json:"exam_type,omitempty"`
	Eye         string         `json:"eye,omitempty"`
	Pages       int            `json:"pages,omitempty"`
	Reused      bool           `json:"reused_checkpoint,omitempty"`
	Outcome     string         `json:"outcome,omitempty"`
	Message     string         `json:"message,omitempty"`
	DurationMS  int64          `json:"duration_ms"`
	Fields      map[string]int `json:"fields,omitempty"`
	startedAt   time.Time
}

// Handler owns the intake_runs table.
type Handler struct {
	Database *gorm.DB
}

type contextKey struct{}

func From(executionContext context.Context) *Run {
	audit, _ := executionContext.Value(contextKey{}).(*Run)
	return audit
}

// All recording methods are no-ops on a nil audit (unaudited calls, tests).

func (audit *Run) SetIntakeID(intakeID string) {
	if audit == nil {
		return
	}
	audit.mutex.Lock()
	defer audit.mutex.Unlock()
	audit.intakeID = intakeID
}

func (audit *Run) SetCase(caseID, action string) {
	if audit == nil {
		return
	}
	audit.mutex.Lock()
	defer audit.mutex.Unlock()
	audit.details.CaseID, audit.details.Action = caseID, action
}

// fail records an error that was reported inside a progress stream (the HTTP
// status there is already 200).
func (audit *Run) Fail(status int, message string) {
	if audit == nil {
		return
	}
	audit.mutex.Lock()
	defer audit.mutex.Unlock()
	audit.details.HTTPStatus, audit.details.Error = status, message
}

// StartFile opens the per-file entry of file.Filename; FinishFile closes it.
func (audit *Run) StartFile(file File) {
	if audit == nil {
		return
	}
	audit.mutex.Lock()
	defer audit.mutex.Unlock()
	entry := &file
	entry.startedAt = time.Now()
	audit.details.Files = append(audit.details.Files, entry)
	audit.open[entry.Filename] = entry
}

func (audit *Run) FinishFile(filename string, update func(entry *File)) {
	if audit == nil {
		return
	}
	audit.mutex.Lock()
	defer audit.mutex.Unlock()
	entry := audit.open[filename]
	if entry == nil {
		return
	}
	if update != nil {
		update(entry)
	}
	entry.DurationMS = time.Since(entry.startedAt).Milliseconds()
}

// countFields tallies the per-field diagnostics of the analysis by file and
// status (valid, missing, unreadable, ...).
func (audit *Run) CountFields(analysis map[string]any) {
	if audit == nil {
		return
	}
	encoded, _ := json.Marshal(analysis["extraction_diagnostics"])
	var diagnostics []struct {
		FileSHA256 string `json:"file_sha256"`
		Status     string `json:"status"`
	}
	if json.Unmarshal(encoded, &diagnostics) != nil {
		return
	}
	audit.mutex.Lock()
	defer audit.mutex.Unlock()
	for _, entry := range audit.details.Files {
		for _, diagnostic := range diagnostics {
			if diagnostic.FileSHA256 != entry.SHA256 {
				continue
			}
			if entry.Fields == nil {
				entry.Fields = map[string]int{}
			}
			entry.Fields[diagnostic.Status]++
		}
	}
}

// auditWriter captures the response status and, for errors, the body (the
// JSON error message). It keeps Flush for the progress stream.
type auditWriter struct {
	http.ResponseWriter
	status    int
	errorBody bytes.Buffer
}

func (writer *auditWriter) WriteHeader(status int) {
	if writer.status == 0 {
		writer.status = status
	}
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *auditWriter) Write(data []byte) (int, error) {
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
	if writer.status >= 400 && writer.errorBody.Len() < 2048 {
		writer.errorBody.Write(data)
	}
	return writer.ResponseWriter.Write(data)
}

func (writer *auditWriter) Flush() {
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Audit wraps an intake route so each POST leaves one intake_runs row,
// created as "running" when the request starts and finalized when it ends.
// An analyze run is detached from the request: closing the tab does not stop
// it, the browser reconnects through StreamRun and stops it through CancelRun.
// A failed run write is logged and never fails the request.
func (handler Handler) Audit(kind string, next http.HandlerFunc) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || handler.Database == nil {
			next(responseWriter, request)
			return
		}
		audit := &Run{started: time.Now(), open: map[string]*File{}}
		requestSpan := profiling.Start(request.Context(), "request")
		row := database.IntakeRun{Kind: kind, Status: database.RunRunning, Details: "{}"}
		if createError := handler.Database.Create(&row).Error; createError != nil {
			log.Printf("intake run (%s): %v", kind, createError)
		}
		audit.runID = row.ID

		executionContext := context.WithValue(request.Context(), contextKey{}, audit)
		if profiling.Enabled() {
			executionContext = profiling.WithRequest(executionContext, fmt.Sprintf("%s-%d-%d", kind, row.ID, audit.started.UnixMicro()%1e6), kind, profileLabel(request))
			requestSpan.In(executionContext)
		}
		var cancelled func() bool
		if kind == "analyze" && row.ID != 0 {
			detached, cancel := context.WithCancel(context.WithoutCancel(executionContext))
			defer cancel()
			executionContext = detached
			cancelled = func() bool { return detached.Err() != nil }
			audit.live = startLiveRun(row.ID, cancel)
		}
		writer := &auditWriter{ResponseWriter: responseWriter}
		handlerSpan := profiling.Start(executionContext, "handler")
		next(writer, request.WithContext(executionContext))
		handlerSpan.Set("status", writer.status).End(nil)

		audit.mutex.Lock()
		details := audit.details
		details.StartedAt = audit.started.UTC()
		details.DurationMS = time.Since(audit.started).Milliseconds()
		details.AppCommit = appCommit()
		if details.HTTPStatus == 0 {
			details.HTTPStatus = writer.status
			if writer.status >= 400 {
				var errorResponse struct {
					Error string `json:"error"`
				}
				if json.Unmarshal(writer.errorBody.Bytes(), &errorResponse) == nil && errorResponse.Error != "" {
					details.Error = errorResponse.Error
				} else {
					details.Error = http.StatusText(writer.status)
				}
			}
		}
		row.Status = database.RunDone
		switch {
		case cancelled != nil && cancelled():
			row.Status = database.RunCancelled
		case details.HTTPStatus >= 400 || details.Error != "":
			row.Status = database.RunFailed
		}
		details.Result = audit.live.result()
		encoded, _ := json.Marshal(details)
		row.IntakeID, row.Details = audit.intakeID, string(encoded)
		audit.mutex.Unlock()

		// The row is final before the live run disappears, so a reconnect that
		// misses the live run always finds the outcome in the table.
		if row.ID == 0 {
			if createError := handler.Database.Create(&row).Error; createError != nil {
				log.Printf("intake run (%s): %v", kind, createError)
			}
		} else if saveError := handler.Database.Save(&row).Error; saveError != nil {
			log.Printf("intake run %d (%s): %v", row.ID, kind, saveError)
		}
		requestSpan.Set("status", details.HTTPStatus).Set("run_status", row.Status).Set("files", len(details.Files))
		requestSpan.End(nil)
		audit.live.finish()
	}
}

// appCommit is the deployed commit: Coolify's SOURCE_COMMIT, else the VCS
// revision Go embedded at build time.
func appCommit() string {
	if commit := os.Getenv("SOURCE_COMMIT"); commit != "" {
		return commit
	}
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range buildInfo.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}
	return ""
}

// profileLabel is the caller's anonymous tag for profiling (the benchmark
// sends "p1/analyze"); anything but [a-z0-9/_-] is dropped so it can never
// carry a name.
func profileLabel(request *http.Request) string {
	label := []byte{}
	for _, character := range []byte(request.Header.Get("X-Refratia-Profile-Label")) {
		if len(label) < 32 && (character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '/' || character == '_' || character == '-') {
			label = append(label, character)
		}
	}
	return string(label)
}
