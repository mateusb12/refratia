package parallelwork

import (
	"context"
	"runtime"
	"sync"

	"refratia/backend/src/shared/profiling"
)

type indexed[T any] struct {
	index int
	value T
}

type indexedResult[T any] struct {
	index      int
	value      T
	errorValue error
}

// Map executes independent tasks with a bounded number of workers and
// preserves input order in the returned values. The worker limit is explicit
// so callers do not accidentally spawn one external OCR process per page.
// It is also capped at GOMAXPROCS: the tasks run CPU-bound tesseract/poppler
// processes, and more of them than cores only adds memory (each holds a
// high-DPI page) without finishing sooner. Results do not depend on the count.
func Map[T any, R any](
	executionContext context.Context,
	items []T,
	workers int,
	fn func(context.Context, T) (R, error),
) ([]R, error) {
	if len(items) == 0 {
		return []R{}, nil
	}
	if workers < 1 {
		workers = 1
	}
	if cores := runtime.GOMAXPROCS(0); workers > cores {
		workers = cores
	}
	if workers > len(items) {
		workers = len(items)
	}

	// The span ends at the join (all results in): its duration is the fork's
	// elapsed time; the items' own spans show how busy the workers were.
	span := profiling.Start(executionContext, "parallel.map").Caller("/shared/parallelwork.").Set("workers", workers).Set("items", len(items))
	defer func() { span.End(executionContext.Err()) }()

	workContext, cancel := context.WithCancel(executionContext)
	defer cancel()

	jobs := make(chan indexed[T], len(items))
	results := make(chan indexedResult[R], len(items))

	for index, item := range items {
		jobs <- indexed[T]{index: index, value: item}
	}
	close(jobs)

	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for job := range jobs {
				if errorValue := workContext.Err(); errorValue != nil {
					results <- indexedResult[R]{index: job.index, errorValue: errorValue}
					continue
				}

				value, errorValue := fn(workContext, job.value)
				results <- indexedResult[R]{index: job.index, value: value, errorValue: errorValue}
				if errorValue != nil {
					cancel()
				}
			}
		}()
	}

	go func() {
		group.Wait()
		close(results)
	}()

	values := make([]R, len(items))
	var firstError error
	for result := range results {
		if result.errorValue != nil && firstError == nil {
			firstError = result.errorValue
		}
		values[result.index] = result.value
	}

	if firstError != nil {
		return nil, firstError
	}
	return values, nil
}
