package pentacam

import (
	"context"
	"fmt"
	"image"
	"strings"

	"refratia/backend/src/shared/ocr"
	pdfutil "refratia/backend/src/shared/pdf"
)

// eyeFromText resolves laterality from the identity panel words that every
// Pentacam screen prints ("Olho: Direito|Esquerdo"). Both or neither => "".
func eyeFromText(text string) string {
	text = strings.ToLower(text)
	right, left := strings.Contains(text, "direito"), strings.Contains(text, "esquerdo")
	switch {
	case right && !left:
		return "OD"
	case left && !right:
		return "OS"
	}
	return ""
}

// DetectEye reads laterality from the identity panel (upper-left of the
// content area), which every screen prints, scanning pages until it resolves.
func DetectEye(ctx context.Context, data []byte) (string, error) {
	pageCount, _, err := pdfutil.Inspect(ctx, data)
	if err != nil {
		return "", err
	}
	isPentacam := false
	for page := 1; page <= pageCount; page++ {
		rendered, err := pdfutil.RenderPageAtDPI(ctx, data, page, 200)
		if err != nil {
			return "", err
		}
		img, err := ocr.DecodeImage(rendered)
		if err != nil {
			return "", err
		}
		b := img.Bounds()
		panel := image.Rect(b.Min.X, b.Min.Y+b.Dy()*20/100, b.Min.X+b.Dx()*45/100, b.Min.Y+b.Dy()*50/100)
		encoded, err := ocr.EncodePNG(ocr.PrepareGray(img, panel, 1, 0))
		if err != nil {
			return "", err
		}
		text, err := runTesseractTextPSM(ctx, encoded, 11)
		if err != nil {
			return "", err
		}
		lower := strings.ToLower(text)
		if strings.Contains(lower, "pentacam") || strings.Contains(lower, "oculus") {
			isPentacam = true
		}
		if eye := eyeFromText(text); eye != "" {
			return eye, nil
		}
	}
	if !isPentacam {
		return "", fmt.Errorf("documento não identificado como Pentacam")
	}
	return "", fmt.Errorf("lateralidade Pentacam não resolvida")
}
