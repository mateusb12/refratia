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
	examType, _, err := ClassifyContentEye(ctx, contentType, data)
	return examType, err
}

var topographyExamHeader = regexp.MustCompile(`#\s*\d+\s*-\s*\d{1,2}/\d{1,2}/\d{4}`)

// TopographyEye: a TP-15 report prints one result column per exam, each headed
// "#<id> - <date>". Two columns hold both eyes (AO); a single one does not say
// which eye it is, so the eye stays unknown.
func TopographyEye(text string) string {
	if len(topographyExamHeader.FindAllString(text, -1)) >= 2 {
		return "AO"
	}
	return ""
}

// ClassifyContentEye is ClassifyContent plus the eye when the document itself
// states it (never taken from the file name).
func ClassifyContentEye(ctx context.Context, contentType string, data []byte) (string, string, error) {
	if contentType == "application/pdf" {
		_, layer, err := pdfutil.Inspect(ctx, data)
		if err != nil {
			return "", "", err
		}
		if len(strings.Fields(layer)) >= 5 {
			if t := ClassifyText(layer); t != "" {
				if t == "corneal_topography_other" {
					return t, TopographyEye(layer), nil
				}
				return t, "", nil
			}
		}
		page, err := pdfutil.RenderPageAtDPI(ctx, data, 1, 100)
		if err != nil {
			return "", "", err
		}
		text, err := ocr.RunTesseractText(ctx, page, 11, "")
		if err != nil {
			return "", "", err
		}
		return ClassifyText(text), "", nil
	}

	if strings.HasPrefix(strings.ToLower(contentType), "image/") {
		// A phone photo of a refractometer ticket.
		if img, err := ocr.DecodeImage(data); err == nil {
			b := img.Bounds()
			gray := ocr.PrepareGray(img, b, 1, 0)
			if png, err := ocr.EncodePNG(gray); err == nil {
				if text, err := ocr.RunTesseractText(ctx, png, 11, ""); err == nil {
					if t := ClassifyText(text); t == "refractometry" {
						return t, "", nil
					}
				}
			}
		}
		if result, err := retinography.Extract(ctx, data); err == nil && result.Mode == "Cornea" {
			return "anterior_segment_photo", "", nil
		}
	}
	return "", "", nil
}
