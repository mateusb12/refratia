package main

import (
	"encoding/json"
	"net/http"
	"sync"

	progressutil "refratia/backend/shared/progress"
)

type progressStream struct {
	mu      sync.Mutex
	encoder *json.Encoder
	flusher http.Flusher
	last    int
}

func newProgressStream(
	w http.ResponseWriter,
) (*progressStream, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}

	w.Header().Set(
		"Content-Type",
		"application/x-ndjson; charset=utf-8",
	)

	w.Header().Set(
		"Cache-Control",
		"no-cache, no-store",
	)

	w.Header().Set(
		"X-Accel-Buffering",
		"no",
	)

	return &progressStream{
		encoder: json.NewEncoder(w),
		flusher: flusher,
	}, true
}

func (stream *progressStream) emit(
	event progressutil.Event,
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
	event progressutil.Event,
) {
	stream.emit(event)
}

func (stream *progressStream) writeError(
	status int,
	message string,
) {
	stream.emit(progressutil.Event{
		Type:    "error",
		Status:  status,
		Message: message,
	})
}

func (stream *progressStream) writeResult(
	payload any,
) {
	stream.emit(progressutil.Event{
		Type:    "result",
		Payload: payload,
	})
}
