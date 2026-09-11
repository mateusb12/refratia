package progress

import (
	"context"
	"testing"
)

func TestRangeMapping(t *testing.T) {
	var got Event

	ctx := WithReporter(
		context.Background(),
		func(event Event) {
			got = event
		},
	)

	ctx = WithRange(
		ctx,
		20,
		60,
	)

	Report(
		ctx,
		50,
		"test",
		"half",
	)

	if got.Percent != 40 {
		t.Fatalf(
			"expected 40, got %d",
			got.Percent,
		)
	}
}

func TestNestedRangeMapping(t *testing.T) {
	var got Event

	ctx := WithReporter(
		context.Background(),
		func(event Event) {
			got = event
		},
	)

	ctx = WithRange(ctx, 20, 60)
	ctx = WithRange(ctx, 50, 100)

	Report(
		ctx,
		50,
		"nested",
		"half",
	)

	// 20..60 => width 40
	// nested 50..100 => 40..60
	// 50% => 50
	if got.Percent != 50 {
		t.Fatalf(
			"expected 50, got %d",
			got.Percent,
		)
	}
}

func TestReportWithoutReporterIsNoop(t *testing.T) {
	Report(
		context.Background(),
		50,
		"test",
		"ignored",
	)
}
