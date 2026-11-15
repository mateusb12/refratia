package progressTracking

import (
	"encoding/json"
	"net/http"
	"sync"

	progress "refratia/backend/src/shared/progress"
)

type Stream struct {
	mu      sync.Mutex
	encoder *json.Encoder
	flusher http.Flusher
	last    int
	live    *liveRun // analyze: every event is also kept for reconnects
}

func NewStream(
	responseWriter http.ResponseWriter,
) (*Stream, bool) {
	flusher, ok := responseWriter.(http.Flusher)
	if !ok {
		return nil, false
	}

	responseWriter.Header().Set(
		"Content-Type",
		"application/x-ndjson; charset=utf-8",
	)

	responseWriter.Header().Set(
		"Cache-Control",
		"no-cache, no-store",
	)

	responseWriter.Header().Set(
		"X-Accel-Buffering",
		"no",
	)

	return &Stream{
		encoder: json.NewEncoder(responseWriter),
		flusher: flusher,
	}, true
}

func (stream *Stream) Emit(
	event progress.Event,
) {
	stream.mu.Lock()
	defer stream.mu.Unlock()

	if event.Type == "progress" {
		if event.Percent < stream.last {
			event.Percent = stream.last
		} else {
			stream.last = event.Percent
		}
	}

	stream.live.add(event)
	// After the browser leaves, writes fail and the run goes on.
	_ = stream.encoder.Encode(event)
	stream.flusher.Flush()
}

// Attach makes the stream feed run's reconnects and sends the run ID first:
// the browser keeps it to reconnect. No-op for runs that are not live.
func (stream *Stream) Attach(run *Run) {
	if run == nil || run.live == nil {
		return
	}
	stream.live = run.live
	stream.Emit(progress.Event{Type: "run", Payload: map[string]any{"runId": run.runID}})
}

func (stream *Stream) Report(
	event progress.Event,
) {
	stream.Emit(event)
}

func (stream *Stream) WriteError(
	status int,
	message string,
) {
	stream.Emit(progress.Event{
		Type:    "error",
		Status:  status,
		Message: message,
	})
}

func (stream *Stream) WriteResult(
	payload any,
) {
	stream.Emit(progress.Event{
		Type:    "result",
		Payload: payload,
	})
}
