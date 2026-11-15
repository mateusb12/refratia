package progressTracking

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gorm.io/gorm"

	"refratia/backend/src/shared/database"
	"refratia/backend/src/shared/fileStorage"
	"refratia/backend/src/shared/httpresponse"
	progress "refratia/backend/src/shared/progress"
)

// liveRun keeps the progress events of an analyze that is still running, so a
// browser that reconnects can replay them and follow the rest.
// ponytail: in memory, one process; a restart fails the run (see
// database.migrateIntakeRuns) and the file checkpoints make the re-run cheap.
type liveRun struct {
	mutex  sync.Mutex
	events []progress.Event
	done   bool
	wake   chan struct{} // closed and replaced on every change
	cancel func()
}

var liveRuns sync.Map // run ID -> *liveRun

func startLiveRun(runID uint, cancel func()) *liveRun {
	run := &liveRun{wake: make(chan struct{}), cancel: cancel}
	liveRuns.Store(runID, run)
	return run
}

func (run *liveRun) add(event progress.Event) {
	if run == nil {
		return
	}
	run.mutex.Lock()
	defer run.mutex.Unlock()
	run.events = append(run.events, event)
	close(run.wake)
	run.wake = make(chan struct{})
}

func (run *liveRun) since(seen int) ([]progress.Event, bool, <-chan struct{}) {
	run.mutex.Lock()
	defer run.mutex.Unlock()
	return run.events[seen:], run.done, run.wake
}

// result is the payload of the final "result" event, nil if the run failed.
func (run *liveRun) result() any {
	if run == nil {
		return nil
	}
	run.mutex.Lock()
	defer run.mutex.Unlock()
	if last := len(run.events) - 1; last >= 0 && run.events[last].Type == "result" {
		return run.events[last].Payload
	}
	return nil
}

func (run *liveRun) finish() {
	if run == nil {
		return
	}
	run.mutex.Lock()
	run.done = true
	close(run.wake)
	run.wake = make(chan struct{})
	run.mutex.Unlock()
	liveRuns.Range(func(key, value any) bool {
		if value == run {
			liveRuns.Delete(key)
		}
		return true
	})
}

// ServeRun routes /api/intakes/runs/{id}/stream (GET) and
// /api/intakes/runs/{id}/cancel (POST).
func (handler Handler) ServeRun(responseWriter http.ResponseWriter, request *http.Request) {
	parts := strings.Split(strings.TrimPrefix(request.URL.Path, "/api/intakes/runs/"), "/")
	runID, parseError := strconv.ParseUint(parts[0], 10, 64)
	if parseError != nil || len(parts) != 2 {
		httpresponse.WriteError(responseWriter, http.StatusNotFound, "execução não encontrada")
		return
	}
	switch {
	case parts[1] == "stream" && request.Method == http.MethodGet:
		handler.streamRun(responseWriter, request, uint(runID))
	case parts[1] == "cancel" && request.Method == http.MethodPost:
		if value, ok := liveRuns.Load(uint(runID)); ok {
			value.(*liveRun).cancel()
		}
		responseWriter.WriteHeader(http.StatusNoContent)
	default:
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// streamRun answers with the same NDJSON stream analyze sends: every event
// so far, then the live ones; a finished run sends its outcome from the table.
func (handler Handler) streamRun(responseWriter http.ResponseWriter, request *http.Request, runID uint) {
	value, live := liveRuns.Load(runID)
	var row database.IntakeRun
	if !live {
		if handler.Database == nil {
			httpresponse.WriteError(responseWriter, http.StatusNotFound, "execução não encontrada")
			return
		}
		findError := handler.Database.Where("kind = ?", "analyze").First(&row, runID).Error
		if errors.Is(findError, gorm.ErrRecordNotFound) {
			httpresponse.WriteError(responseWriter, http.StatusNotFound, "execução não encontrada")
			return
		}
		if findError != nil {
			httpresponse.WriteError(responseWriter, http.StatusInternalServerError, "não foi possível consultar a execução")
			return
		}
		// The run finished between the lookups: the row is final by now.
		if value, live = liveRuns.Load(runID); live {
			row = database.IntakeRun{}
		}
	}

	stream, ok := NewStream(responseWriter)
	if !ok {
		httpresponse.WriteError(responseWriter, http.StatusInternalServerError, "stream de progresso indisponível")
		return
	}
	if live {
		run, seen := value.(*liveRun), 0
		for {
			events, done, wake := run.since(seen)
			for _, event := range events {
				stream.Emit(event)
			}
			seen += len(events)
			if done {
				return
			}
			select {
			case <-wake:
			case <-request.Context().Done():
				return
			}
		}
	}

	var details struct {
		HTTPStatus int            `json:"http_status"`
		Error      string         `json:"error"`
		Result     map[string]any `json:"result"`
	}
	_ = json.Unmarshal([]byte(row.Details), &details)
	switch {
	case row.Status == database.RunDone && details.Result != nil:
		resignPreviewFiles(request, details.Result)
		stream.WriteResult(details.Result)
	case row.Status == database.RunCancelled:
		stream.WriteError(http.StatusConflict, "Análise cancelada. Os resultados já processados foram preservados.")
	default:
		message := details.Error
		if message == "" {
			message = "a análise não terminou"
		}
		status := details.HTTPStatus
		if status < 400 {
			status = http.StatusInternalServerError
		}
		stream.WriteError(status, message)
	}
}

// resignPreviewFiles refreshes the preview links of a stored result; the ones
// signed when the run finished may have expired.
func resignPreviewFiles(request *http.Request, result map[string]any) {
	files, _ := result["files"].([]any)
	presigner, presignError := fileStorage.NewPresigner(request.Context())
	if presignError != nil {
		return
	}
	for _, item := range files {
		file, _ := item.(map[string]any)
		key, _ := file["key"].(string)
		if key == "" {
			continue
		}
		if signed, signError := presigner.PresignGetObject(request.Context(), &s3.GetObjectInput{
			Bucket: aws.String(os.Getenv("BUCKET_NAME")),
			Key:    aws.String(key),
		}); signError == nil {
			file["signed_url"] = signed.URL
		}
	}
}
