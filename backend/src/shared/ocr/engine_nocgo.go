//go:build !cgo

package ocr

import "context"

// WarmUp is a no-op without cgo: every Recognize starts the tesseract CLI.
func WarmUp() error { return nil }

func recognize(executionContext context.Context, pngData []byte, psm int, whitelist string, tsv bool) (string, error) {
	return recognizeWithCLI(executionContext, pngData, psm, whitelist, tsv)
}
