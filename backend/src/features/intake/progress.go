package intake

import (
	"encoding/json"
	"net/http"
	"sync"

	progress "refratia/backend/src/shared/progress"
)

type progressStream struct {
	mu      sync.Mutex
	encoder *json.Encoder
	flusher http.Flusher
	last    int
}

func newProgressStream(
	responseWriter http.ResponseWriter,
) (*progressStream, bool) {
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

	return &progressStream{
		encoder: json.NewEncoder(responseWriter),
		flusher: flusher,
	}, true
}

func (stream *progressStream) emit(
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

	_ = stream.encoder.Encode(event)
	stream.flusher.Flush()
}

func (stream *progressStream) reporter(
	event progress.Event,
) {
	stream.emit(event)
}

func (stream *progressStream) writeError(
	status int,
	message string,
) {
	stream.emit(progress.Event{
		Type:    "error",
		Status:  status,
		Message: message,
	})
}

func (stream *progressStream) writeResult(
	payload any,
) {
	stream.emit(progress.Event{
		Type:    "result",
		Payload: payload,
	})
}
