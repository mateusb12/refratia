// Package document composes the shared PDF and OCR primitives into the few
// workflows that several extractors repeat. Device meaning stays in callers.
package document

import (
	"context"
	"image"
	"strings"

	"refratia/backend/src/shared/ocr"
)

// OCRWords runs Tesseract in TSV mode with the given page segmentation mode
// and returns the recognised word boxes.
func OCRWords(executionContext context.Context, pngData []byte, psm int) ([]ocr.Word, error) {
	tsv, ocrError := ocr.RunTesseractTSVWithPSM(executionContext, pngData, psm)
	if ocrError != nil {
		return nil, ocrError
	}
	words, _, parseError := ocr.ParseTSVWords(tsv)
	if parseError != nil {
		return nil, parseError
	}
	return words, nil
}

// OCRRegionWords converts region of source to grayscale (upscaled by scale,
// no threshold), encodes it as PNG and returns the OCR word boxes in the
// coordinates of that processed image. Encode and OCR errors are returned;
// a TSV parse error is ignored and the parsed words are kept, as every
// region caller did before this helper existed.
func OCRRegionWords(
	executionContext context.Context,
	source image.Image,
	region image.Rectangle,
	scale,
	psm int,
) ([]ocr.Word, error) {
	encoded, encodeError := ocr.EncodePNG(ocr.PrepareGray(source, region, scale, 0))
	if encodeError != nil {
		return nil, encodeError
	}
	tsv, ocrError := ocr.RunTesseractTSVWithPSM(executionContext, encoded, psm)
	if ocrError != nil {
		return nil, ocrError
	}
	words, _, _ := ocr.ParseTSVWords(tsv)
	return words, nil
}

// RowsText groups words into rows within tolerance pixels and returns one
// line per row. GroupRows sorts words in place.
func RowsText(words []ocr.Word, tolerance int) string {
	rows := ocr.GroupRows(words, tolerance)
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, ocr.RowText(row))
	}
	return strings.Join(lines, "\n")
}
