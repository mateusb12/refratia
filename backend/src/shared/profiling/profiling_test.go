package profiling

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

// Profiling off (no REFRATIA_PROFILE in tests): every call must be a no-op.
func TestDisabledIsNoOp(t *testing.T) {
	if Enabled() {
		t.Skip("REFRATIA_PROFILE is set")
	}
	ctx := WithDoc(WithRequest(context.Background(), "r", "analyze", "p1/analyze"), 0)
	span := Start(ctx, "x").Caller().Set("k", 1)
	if span != nil {
		t.Fatal("Start must return nil when disabled")
	}
	Annotate(span.Into(ctx), "wait_ms", 1.0)
	span.EndCmd(exec.Command("true"), nil)
	span.End(errors.New("e"))
	phases := StartPhases(ctx, "p")
	phases.Next("a")
	phases.End()
}

func TestOutcome(t *testing.T) {
	for err, want := range map[error]string{nil: "ok", context.Canceled: "canceled", context.DeadlineExceeded: "timeout", errors.New("x"): "error"} {
		if got := Outcome(err); got != want {
			t.Fatalf("Outcome(%v) = %s, want %s", err, got, want)
		}
	}
}
