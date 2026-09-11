package eyesuite

import (
	"context"
	"fmt"

	"refratia/backend/shared/ocr"

	pdfutil "refratia/backend/shared/pdf"
)

type Result struct {
	Exam     map[string]any
	Identity *Identity
}

func ExtractPDF(ctx context.Context, data []byte) (Result, error) {
	if _, _, err := pdfutil.Inspect(ctx, data); err != nil {
		return Result{}, err
	}

	var bestExam map[string]any
	var lastErr error

	for _, dpi := range []int{300, 450} {
		image, err := pdfutil.RenderPageAtDPI(ctx, data, 1, dpi)
		if err != nil {
			lastErr = err
			continue
		}

		tsv, err := ocr.RunTesseractTSV(ctx, image)
		if err != nil {
			lastErr = err
			continue
		}

		exam, err := parseEyeSuiteTSV(tsv)
		if err != nil {
			lastErr = fmt.Errorf("%d DPI: %w", dpi, err)
			continue
		}

		if bestExam == nil {
			bestExam = exam
		}

		identity, identityErr := parseEyeSuiteIdentityTSV(tsv)
		if identityErr == nil {
			return Result{
				Exam:     exam,
				Identity: &identity,
			}, nil
		}

		lastErr = fmt.Errorf("%d DPI identidade: %w", dpi, identityErr)
	}

	if bestExam != nil {
		return Result{Exam: bestExam}, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("extração local não produziu resultado")
	}
	return Result{}, fmt.Errorf("EyeSuite: %w", lastErr)
}
