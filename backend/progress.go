package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
)

type progressEvent struct {
	Type    string `json:"type"`
	Percent int    `json:"percent,omitempty"`
	Stage   string `json:"stage,omitempty"`
	Message string `json:"message,omitempty"`
	Status  int    `json:"status,omitempty"`
	Payload any    `json:"payload,omitempty"`
}

type progressReporter func(progressEvent)

type progressReporterKey struct{}
type progressRangeKey struct{}

type progressRange struct {
	Start int
	End   int
}

func withProgressReporter(
	ctx context.Context,
	reporter progressReporter,
) context.Context {
	ctx = context.WithValue(
		ctx,
		progressReporterKey{},
		reporter,
	)

	return context.WithValue(
		ctx,
		progressRangeKey{},
		progressRange{Start: 0, End: 100},
	)
}

func currentProgressRange(ctx context.Context) progressRange {
	if value, ok := ctx.Value(progressRangeKey{}).(progressRange); ok {
		return value
	}

	return progressRange{Start: 0, End: 100}
}

func withProgressRange(
	ctx context.Context,
	start,
	end int,
) context.Context {
	parent := currentProgressRange(ctx)

	if start < 0 {
		start = 0
	}
	if end > 100 {
		end = 100
	}
	if end < start {
		end = start
	}

	width := parent.End - parent.Start

	mapped := progressRange{
		Start: parent.Start + width*start/100,
		End:   parent.Start + width*end/100,
	}

	return context.WithValue(
		ctx,
		progressRangeKey{},
		mapped,
	)
}

func reportProgress(
	ctx context.Context,
	percent int,
	stage,
	message string,
) {
	reporter, ok := ctx.Value(
		progressReporterKey{},
	).(progressReporter)

	if !ok || reporter == nil {
		return
	}

	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	scope := currentProgressRange(ctx)
	mapped := scope.Start +
		(scope.End-scope.Start)*percent/100

	reporter(progressEvent{
		Type:    "progress",
		Percent: mapped,
		Stage:   stage,
		Message: message,
	})
}

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
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no")

	return &progressStream{
		encoder: json.NewEncoder(w),
		flusher: flusher,
	}, true
}

func (stream *progressStream) emit(
	event progressEvent,
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
	event progressEvent,
) {
	stream.emit(event)
}

func (stream *progressStream) writeError(
	status int,
	message string,
) {
	stream.emit(progressEvent{
		Type:    "error",
		Status:  status,
		Message: message,
	})
}

func (stream *progressStream) writeResult(
	payload any,
) {
	stream.emit(progressEvent{
		Type:    "result",
		Payload: payload,
	})
}
