//go:build !cgo

package ocr

import "context"

// WarmUp is a no-op without cgo: every Recognize starts the tesseract CLI.
func WarmUp() error { return nil }

// Backend names the OCR implementation this binary was built with.
const Backend = "tesseract-cli"

// Version is unknown without cgo: the CLI on PATH decides.
func Version() string { return "" }

func recognize(executionContext context.Context, pngData []byte, psm int, whitelist string, tsv bool) (string, error) {
	return recognizeWithCLI(executionContext, pngData, psm, whitelist, tsv)
}
