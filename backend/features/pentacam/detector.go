package pentacam

import (
	"context"
	"fmt"
	"strings"

	pdfutil "refratia/backend/shared/pdf"
)

func DetectEye(ctx context.Context, data []byte) (string, error) {
	image, err := pdfutil.RenderPageAtDPI(ctx, data, 9, 220)
	if err != nil {
		return "", err
	}

	text, err := runTesseractTextPSM(ctx, image, 11)
	if err != nil {
		return "", err
	}

	text = strings.ToLower(text)

	if !strings.Contains(text, "pentacam") &&
		!strings.Contains(text, "oculus") {
		return "", fmt.Errorf("documento não identificado como Pentacam")
	}

	hasRight := strings.Contains(text, "direito")
	hasLeft := strings.Contains(text, "esquerdo")

	switch {
	case hasRight && !hasLeft:
		return "OD", nil
	case hasLeft && !hasRight:
		return "OS", nil
	default:
		return "", fmt.Errorf("lateralidade Pentacam não resolvida")
	}
}
