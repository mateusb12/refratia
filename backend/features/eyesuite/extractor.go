package eyesuite

import (
	"context"
	"fmt"

	"refratia/backend/shared/ocr"
	"refratia/backend/shared/parallelwork"
	"refratia/backend/shared/progress"

	pdfutil "refratia/backend/shared/pdf"
)

type Result struct {
	Exam     map[string]any
	Identity *Identity
}

func ExtractPDF(executionContext context.Context, data []byte) (Result, error) {
	if _, _, errorValue := pdfutil.Inspect(executionContext, data); errorValue != nil {
		return Result{}, errorValue
	}
	return extractPDF(executionContext, data, true)
}

type dpiAttempt struct {
	exam       map[string]any
	identity   *Identity
	errorValue error
}

func extractPDF(executionContext context.Context, data []byte, parallelDPI bool) (Result, error) {
	dpis := []int{300, 450}
	attempt := func(executionContext context.Context, dpi int) (dpiAttempt, error) {
		taskIdentifier := fmt.Sprintf("eyesuite-dpi-%d", dpi)
		taskLabel := fmt.Sprintf("OCR da biometria — %d DPI", dpi)
		progress.ReportTask(executionContext, taskIdentifier, taskLabel, progress.TaskStatusRunning, 0)

		image, errorValue := pdfutil.RenderPageAtDPI(executionContext, data, 1, dpi)
		if errorValue != nil {
			progress.ReportTask(executionContext, taskIdentifier, taskLabel, progress.TaskStatusFailed, 100)
			return dpiAttempt{errorValue: errorValue}, nil
		}

		tsv, errorValue := ocr.RunTesseractTSV(executionContext, image)
		if errorValue != nil {
			progress.ReportTask(executionContext, taskIdentifier, taskLabel, progress.TaskStatusFailed, 100)
			return dpiAttempt{errorValue: errorValue}, nil
		}

		exam, errorValue := parseEyeSuiteTSV(tsv)
		if errorValue != nil {
			progress.ReportTask(executionContext, taskIdentifier, taskLabel, progress.TaskStatusFailed, 100)
			return dpiAttempt{errorValue: fmt.Errorf("%d DPI: %w", dpi, errorValue)}, nil
		}

		identity, identityError := parseEyeSuiteIdentityTSV(tsv)
		if identityError != nil {
			progress.ReportTask(executionContext, taskIdentifier, taskLabel, progress.TaskStatusFailed, 100)
			return dpiAttempt{
				exam:       exam,
				errorValue: fmt.Errorf("%d DPI identidade: %w", dpi, identityError),
			}, nil
		}

		progress.ReportTask(executionContext, taskIdentifier, taskLabel, progress.TaskStatusCompleted, 100)
		return dpiAttempt{exam: exam, identity: &identity}, nil
	}

	attempts := make([]dpiAttempt, len(dpis))
	if parallelDPI {
		results, errorValue := parallelwork.Map(executionContext, dpis, 2, attempt)
		if errorValue != nil {
			return Result{}, errorValue
		}
		attempts = results
	} else {
		for index, dpi := range dpis {
			result, _ := attempt(executionContext, dpi)
			attempts[index] = result
			if result.identity != nil {
				break
			}
		}
	}

	var bestExam map[string]any
	var lastError error
	for _, result := range attempts {
		if result.errorValue != nil {
			lastError = result.errorValue
			if result.exam == nil {
				continue
			}
		}

		if bestExam == nil && result.exam != nil {
			bestExam = result.exam
		}

		if result.identity != nil {
			return Result{
				Exam:     result.exam,
				Identity: result.identity,
			}, nil
		}
	}

	if bestExam != nil {
		return Result{Exam: bestExam}, nil
	}

	if lastError == nil {
		lastError = fmt.Errorf("extração local não produziu resultado")
	}
	return Result{}, fmt.Errorf("EyeSuite: %w", lastError)
}
