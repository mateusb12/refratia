package parallelwork

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestMapPreservesInputOrder(t *testing.T) {
	got, errorValue := Map(context.Background(), []int{1, 2, 3, 4}, 2, func(_ context.Context, value int) (int, error) {
		return value * 10, nil
	})
	if errorValue != nil {
		t.Fatal(errorValue)
	}
	if want := []int{10, 20, 30, 40}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMapReturnsTaskError(t *testing.T) {
	expectedError := errors.New("task failed")
	_, errorValue := Map(context.Background(), []int{1, 2}, 2, func(_ context.Context, value int) (int, error) {
		if value == 2 {
			return 0, expectedError
		}
		return value, nil
	})
	if !errors.Is(errorValue, expectedError) {
		t.Fatalf("got error %v, want %v", errorValue, expectedError)
	}
}

func TestMapRunsNoMoreWorkersThanCores(t *testing.T) {
	previousCores := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previousCores)

	var running, maximumRunning atomic.Int32
	_, errorValue := Map(context.Background(), []int{1, 2, 3, 4}, 4, func(_ context.Context, value int) (int, error) {
		current := running.Add(1)
		for {
			observed := maximumRunning.Load()
			if current <= observed || maximumRunning.CompareAndSwap(observed, current) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		running.Add(-1)
		return value, nil
	})
	if errorValue != nil {
		t.Fatal(errorValue)
	}
	if maximumRunning.Load() != 1 {
		t.Fatalf("ran %d tasks at once with GOMAXPROCS=1, want 1", maximumRunning.Load())
	}
}
