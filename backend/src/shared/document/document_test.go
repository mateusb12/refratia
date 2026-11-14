package document

import (
	"testing"

	"refratia/backend/src/shared/ocr"
)

func TestRowsTextGroupsByTopThenLeft(t *testing.T) {
	words := []ocr.Word{
		{Text: "world", Left: 50, Top: 12},
		{Text: "second", Left: 0, Top: 40},
		{Text: "hello", Left: 0, Top: 10},
	}
	got := RowsText(words, 5)
	if got != "hello world\nsecond" {
		t.Fatalf("RowsText = %q", got)
	}
}
