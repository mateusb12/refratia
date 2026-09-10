package main

import (
	"context"
	"testing"
)

func TestProgressRangeMapping(t *testing.T) {
	var got progressEvent

	ctx := withProgressReporter(
		context.Background(),
		func(event progressEvent) {
			got = event
		},
	)

	ctx = withProgressRange(ctx, 20, 60)

	reportProgress(
		ctx,
		50,
		"test",
		"half",
	)

	if got.Percent != 40 {
		t.Fatalf(
			"esperado 40, recebido %d",
			got.Percent,
		)
	}
}
