package parallelwork

import (
	"context"
	"errors"
	"reflect"
	"testing"
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
