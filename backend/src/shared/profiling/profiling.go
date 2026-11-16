// Package profiling writes opt-in timing spans, one JSON object per line.
//
// It is off unless REFRATIA_PROFILE names an output file; then every Start
// returns nil and every method on a nil *Span is a no-op, so call sites cost
// a nil check. Spans carry timings, counts and outcome classes only: never
// filenames, patient data, OCR text, extracted values or error messages.
//
// Each line: ts_us (start, Unix microseconds), dur_us, src, run, req, kind,
// label, doc, stage, outcome and optional attrs. ts_us + dur_us keep the
// timeline, so overlapping (parallel) spans stay distinguishable from
// sequential ones. cmd/benchmark -profile reads the file and summarizes it.
package profiling

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	output  *os.File
	writeMu sync.Mutex
	runID   string
)

func init() {
	path := os.Getenv("REFRATIA_PROFILE")
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		os.Stderr.WriteString("profiling disabled: " + err.Error() + "\n")
		return
	}
	output = file
	runID = os.Getenv("REFRATIA_PROFILE_RUN")
	if runID == "" {
		random := make([]byte, 4)
		rand.Read(random)
		runID = hex.EncodeToString(random)
	}
}

// Enabled reports whether spans are being written.
func Enabled() bool { return output != nil }

type request struct{ id, kind, label string }

type (
	requestKey struct{}
	docKey     struct{}
	spanKey    struct{}
)

// WithRequest tags ctx with the request every span below it belongs to.
// label is an optional anonymous caller tag (the benchmark sends "p1/analyze").
func WithRequest(ctx context.Context, id, kind, label string) context.Context {
	if output == nil {
		return ctx
	}
	return context.WithValue(ctx, requestKey{}, request{id, kind, label})
}

// WithDoc tags ctx with the anonymous index of the uploaded document (0-based).
func WithDoc(ctx context.Context, index int) context.Context {
	if output == nil {
		return ctx
	}
	return context.WithValue(ctx, docKey{}, index)
}

// Span is one timed stage. A nil *Span (profiling off) ignores every call.
type Span struct {
	ctx   context.Context
	stage string
	start time.Time
	mu    sync.Mutex
	attrs map[string]any
}

// Start opens a span; End (or EndCmd) writes it.
func Start(ctx context.Context, stage string) *Span {
	if output == nil {
		return nil
	}
	return &Span{ctx: ctx, stage: stage, start: time.Now()}
}

// In moves the span under ctx (e.g. once the request ID is known).
func (span *Span) In(ctx context.Context) *Span {
	if span != nil {
		span.ctx = ctx
	}
	return span
}

// Into returns ctx carrying the span, so code below can Annotate it.
func (span *Span) Into(ctx context.Context) context.Context {
	if span == nil {
		return ctx
	}
	return context.WithValue(ctx, spanKey{}, span)
}

// Annotate sets an attribute on the innermost span carried by ctx.
func Annotate(ctx context.Context, key string, value any) {
	if output == nil {
		return
	}
	if span, _ := ctx.Value(spanKey{}).(*Span); span != nil {
		span.Set(key, value)
	}
}

func (span *Span) Set(key string, value any) *Span {
	if span == nil {
		return nil
	}
	span.mu.Lock()
	if span.attrs == nil {
		span.attrs = map[string]any{}
	}
	span.attrs[key] = value
	span.mu.Unlock()
	return span
}

// Caller records the first two functions outside the given package path
// fragments ("wrapper<site"), i.e. which parser code asked for this OCR call
// or render. Two frames, because parsers wrap OCR in their own helpers.
func (span *Span) Caller(skipPackages ...string) *Span {
	if span == nil {
		return nil
	}
	pcs := make([]uintptr, 24)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	var names []string
	for len(names) < 2 {
		frame, more := frames.Next()
		name := frame.Function
		inside := strings.Contains(name, "/shared/profiling.")
		for _, fragment := range skipPackages {
			inside = inside || strings.Contains(name, fragment)
		}
		if !inside && name != "" {
			if slash := strings.LastIndex(name, "/"); slash >= 0 {
				name = name[slash+1:]
			}
			names = append(names, name)
		}
		if !more {
			break
		}
	}
	return span.Set("caller", strings.Join(names, "<"))
}

// Outcome classes an error without recording its text (which may contain paths).
func Outcome(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "error"
	}
}

func (span *Span) End(err error) {
	if span == nil {
		return
	}
	span.write(time.Since(span.start), Outcome(err))
}

// EndCmd ends a subprocess span, adding the child's CPU time when it ran.
func (span *Span) EndCmd(command *exec.Cmd, err error) {
	if span == nil {
		return
	}
	if state := command.ProcessState; state != nil {
		span.Set("user_ms", float64(state.UserTime().Microseconds())/1000)
		span.Set("sys_ms", float64(state.SystemTime().Microseconds())/1000)
		span.Set("exit", state.ExitCode())
	} else {
		span.Set("exit", -1) // never started (missing binary, cancelled before start)
	}
	span.End(err)
}

type record struct {
	TS      int64          `json:"ts_us"`
	Dur     int64          `json:"dur_us"`
	Src     string         `json:"src"`
	Run     string         `json:"run"`
	Req     string         `json:"req,omitempty"`
	Kind    string         `json:"kind,omitempty"`
	Label   string         `json:"label,omitempty"`
	Doc     *int           `json:"doc,omitempty"`
	Stage   string         `json:"stage"`
	Outcome string         `json:"outcome"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

func (span *Span) write(duration time.Duration, outcome string) {
	line := record{
		TS: span.start.UnixMicro(), Dur: duration.Microseconds(),
		Src: "server", Run: runID, Stage: span.stage, Outcome: outcome,
	}
	if req, ok := span.ctx.Value(requestKey{}).(request); ok {
		line.Req, line.Kind, line.Label = req.id, req.kind, req.label
	}
	if doc, ok := span.ctx.Value(docKey{}).(int); ok {
		line.Doc = &doc
	}
	span.mu.Lock()
	line.Attrs = span.attrs
	encoded, _ := json.Marshal(line)
	span.mu.Unlock()
	encoded = append(encoded, '\n')
	writeMu.Lock()
	output.Write(encoded) // O_APPEND: one write per line, unbuffered so a killed server loses nothing
	writeMu.Unlock()
}

// Phases times consecutive, non-overlapping steps of one handler: each Next
// ends the previous step. Nil (profiling off) ignores every call.
type Phases struct {
	ctx     context.Context
	prefix  string
	current *Span
}

func StartPhases(ctx context.Context, prefix string) *Phases {
	if output == nil {
		return nil
	}
	return &Phases{ctx: ctx, prefix: prefix}
}

func (phases *Phases) Next(name string) {
	if phases == nil {
		return
	}
	phases.current.End(nil)
	phases.current = Start(phases.ctx, phases.prefix+"."+name)
}

func (phases *Phases) End() {
	if phases == nil {
		return
	}
	phases.current.End(nil)
	phases.current = nil
}
