package progress

import "context"

type Event struct {
	Type    string `json:"type"`
	Percent int    `json:"percent,omitempty"`
	Stage   string `json:"stage,omitempty"`
	Message string `json:"message,omitempty"`
	Status  int    `json:"status,omitempty"`
	Payload any    `json:"payload,omitempty"`
}

type Reporter func(Event)

type reporterKey struct{}
type rangeKey struct{}

type progressRange struct {
	Start int
	End   int
}

func WithReporter(
	ctx context.Context,
	reporter Reporter,
) context.Context {
	ctx = context.WithValue(
		ctx,
		reporterKey{},
		reporter,
	)

	return context.WithValue(
		ctx,
		rangeKey{},
		progressRange{
			Start: 0,
			End:   100,
		},
	)
}

func currentRange(
	ctx context.Context,
) progressRange {
	if value, ok :=
		ctx.Value(rangeKey{}).(progressRange); ok {

		return value
	}

	return progressRange{
		Start: 0,
		End:   100,
	}
}

func WithRange(
	ctx context.Context,
	start,
	end int,
) context.Context {
	parent := currentRange(ctx)

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
		Start: parent.Start +
			width*start/100,

		End: parent.Start +
			width*end/100,
	}

	return context.WithValue(
		ctx,
		rangeKey{},
		mapped,
	)
}

func Report(
	ctx context.Context,
	percent int,
	stage,
	message string,
) {
	reporter, ok :=
		ctx.Value(reporterKey{}).(Reporter)

	if !ok || reporter == nil {
		return
	}

	if percent < 0 {
		percent = 0
	}

	if percent > 100 {
		percent = 100
	}

	scope := currentRange(ctx)

	mapped :=
		scope.Start +
			(scope.End-scope.Start)*percent/100

	reporter(Event{
		Type:    "progress",
		Percent: mapped,
		Stage:   stage,
		Message: message,
	})
}
