package main

import (
	"context"
	"fmt"

	"refratia/backend/shared/ocr"

	pdfutil "refratia/backend/shared/pdf"
)

type eyeSuiteLocalBundle struct {
	Exam     map[string]any
	Identity *eyeSuiteIdentity
}

func extractEyeSuitePDFLocal(ctx context.Context, data []byte) (map[string]any, error) {
	bundle, err := extractEyeSuitePDFLocalBundle(ctx, data)
	if err != nil {
		return nil, err
	}
	return bundle.Exam, nil
}

func extractEyeSuitePDFLocalBundle(ctx context.Context, data []byte) (eyeSuiteLocalBundle, error) {
	if _, _, err := pdfutil.Inspect(ctx, data); err != nil {
		return eyeSuiteLocalBundle{}, err
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
			return eyeSuiteLocalBundle{
				Exam:     exam,
				Identity: &identity,
			}, nil
		}

		lastErr = fmt.Errorf("%d DPI identidade: %w", dpi, identityErr)
	}

	if bestExam != nil {
		return eyeSuiteLocalBundle{Exam: bestExam}, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("extração local não produziu resultado")
	}
	return eyeSuiteLocalBundle{}, fmt.Errorf("EyeSuite: %w", lastErr)
}
