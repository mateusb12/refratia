package intake

import (
	"context"
	"regexp"
	"strings"

	"refratia/backend/features/retinography"
	"refratia/backend/shared/ocr"
	pdfutil "refratia/backend/shared/pdf"
)

// Printed-title signatures per device. They identify the document by what is
// on the page, independently of the file name.
var contentSignatures = []struct {
	examType string
	pattern  *regexp.Regexp
}{
	{"pentacam_corneal_tomography", regexp.MustCompile(`(?i)pentacam|oculus\s*optik`)},
	{"iol_calculation", regexp.MustCompile(`(?i)eyesuite|calculation\s*id`)},
	{"corneal_topography_other", regexp.MustCompile(`(?i)\bT[CP]-?15\b|topograph`)},
	{"refractometry", regexp.MustCompile(`(?i)\bPRK-?\d|\[REF\]|auto\s*ref`)},
}

// ClassifyText returns the exam type whose device signature appears in text,
// or "" when none (or more than one) does.
func ClassifyText(text string) string {
	found := ""
	for _, signature := range contentSignatures {
		if signature.pattern.MatchString(text) {
			if found != "" {
				return ""
			}
			found = signature.examType
		}
	}
	return found
}

// ClassifyContent identifies a document from its content: the PDF text layer
// when it has one, otherwise an OCR of the first page at low resolution
// (enough for the title band); for images, the retina-camera legend mode.
// "" means unknown. Errors are not fatal: classification then falls back to
// the file name.
func ClassifyContent(ctx context.Context, contentType string, data []byte) (string, error) {
	if contentType == "application/pdf" {
		_, layer, err := pdfutil.Inspect(ctx, data)
		if err != nil {
			return "", err
		}
		if len(strings.Fields(layer)) >= 5 {
			if t := ClassifyText(layer); t != "" {
				return t, nil
			}
		}
		page, err := pdfutil.RenderPageAtDPI(ctx, data, 1, 100)
		if err != nil {
			return "", err
		}
		text, err := ocr.RunTesseractText(ctx, page, 11, "")
		if err != nil {
			return "", err
		}
		return ClassifyText(text), nil
	}

	if strings.HasPrefix(strings.ToLower(contentType), "image/") {
		// A phone photo of a refractometer ticket.
		if img, err := ocr.DecodeImage(data); err == nil {
			b := img.Bounds()
			gray := ocr.PrepareGray(img, b, 1, 0)
			if png, err := ocr.EncodePNG(gray); err == nil {
				if text, err := ocr.RunTesseractText(ctx, png, 11, ""); err == nil {
					if t := ClassifyText(text); t == "refractometry" {
						return t, nil
					}
				}
			}
		}
		if result, err := retinography.Extract(ctx, data); err == nil && result.Mode == "Cornea" {
			return "anterior_segment_photo", nil
		}
	}
	return "", nil
}
