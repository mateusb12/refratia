package intake

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"refratia/backend/src/shared/database"
)

// processingAudit collects what one intake request did; Audit writes it to
// processing_audits when the request ends.
type processingAudit struct {
	mutex    sync.Mutex
	intakeID string
	details  auditDetails
	started  time.Time
	open     map[string]*auditFile // per-file entries by filename, while processing
}

type auditDetails struct {
	StartedAt  time.Time    `json:"started_at"`
	DurationMS int64        `json:"duration_ms"`
	HTTPStatus int          `json:"http_status"`
	Error      string       `json:"error,omitempty"`
	CaseID     string       `json:"case_id,omitempty"`
	Action     string       `json:"action,omitempty"`
	AppCommit  string       `json:"app_commit,omitempty"`
	Files      []*auditFile `json:"files,omitempty"`
}

type auditFile struct {
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

type auditContextKey struct{}

func auditFrom(executionContext context.Context) *processingAudit {
	audit, _ := executionContext.Value(auditContextKey{}).(*processingAudit)
	return audit
}

// All recording methods are no-ops on a nil audit (unaudited calls, tests).

func (audit *processingAudit) setIntakeID(intakeID string) {
	if audit == nil {
		return
	}
	audit.mutex.Lock()
	defer audit.mutex.Unlock()
	audit.intakeID = intakeID
}

func (audit *processingAudit) setCase(caseID, action string) {
	if audit == nil {
		return
	}
	audit.mutex.Lock()
	defer audit.mutex.Unlock()
	audit.details.CaseID, audit.details.Action = caseID, action
}

// fail records an error that was reported inside a progress stream (the HTTP
// status there is already 200).
func (audit *processingAudit) fail(status int, message string) {
	if audit == nil {
		return
	}
	audit.mutex.Lock()
	defer audit.mutex.Unlock()
	audit.details.HTTPStatus, audit.details.Error = status, message
}

func (audit *processingAudit) startFile(file uploadedFile) {
	if audit == nil {
		return
	}
	audit.mutex.Lock()
	defer audit.mutex.Unlock()
	entry := &auditFile{
		Filename:    file.Metadata.Filename,
		SHA256:      file.Metadata.SHA256,
		ContentType: file.Metadata.ContentType,
		Size:        file.Metadata.Size,
		ExamType:    file.Metadata.ExamType,
		Eye:         file.Metadata.Eye,
		startedAt:   time.Now(),
	}
	audit.details.Files = append(audit.details.Files, entry)
	audit.open[entry.Filename] = entry
}

func (audit *processingAudit) finishFile(filename string, update func(entry *auditFile)) {
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
func (audit *processingAudit) countFields(analysis map[string]any) {
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

// Audit wraps an intake route so each POST leaves one processing_audits row.
// A failed audit write is logged and never fails the request.
func (handler Handler) Audit(kind string, next http.HandlerFunc) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || handler.Database == nil {
			next(responseWriter, request)
			return
		}
		audit := &processingAudit{started: time.Now(), open: map[string]*auditFile{}}
		writer := &auditWriter{ResponseWriter: responseWriter}
		next(writer, request.WithContext(context.WithValue(request.Context(), auditContextKey{}, audit)))

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
		encoded, _ := json.Marshal(details)
		row := database.ProcessingAudit{Kind: kind, IntakeID: audit.intakeID, Details: string(encoded)}
		audit.mutex.Unlock()

		if createError := handler.Database.Create(&row).Error; createError != nil {
			log.Printf("processing audit (%s): %v", kind, createError)
		}
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
