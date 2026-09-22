package main

import (
	"path/filepath"
	"strings"
)

type examSourceClassification struct {
	Exam string
	Eye  string
}

func examSourceNames(
	raw any,
) []string {
	switch values := raw.(type) {
	case []any:
		result := make(
			[]string,
			0,
			len(values),
		)

		for _, rawValue := range values {
			value, ok :=
				rawValue.(string)

			if !ok {
				continue
			}

			value =
				strings.TrimSpace(
					value,
				)

			if value != "" {
				result = append(
					result,
					value,
				)
			}
		}

		return result

	case []string:
		result := make(
			[]string,
			0,
			len(values),
		)

		for _, value := range values {
			value =
				strings.TrimSpace(
					value,
				)

			if value != "" {
				result = append(
					result,
					value,
				)
			}
		}

		return result
	}

	return nil
}

func sourceFilenameMatches(
	storedPath string,
	originalFilename string,
) bool {
	storedBase :=
		filepath.Base(
			strings.TrimSpace(
				storedPath,
			),
		)

	originalBase :=
		filepath.Base(
			strings.TrimSpace(
				originalFilename,
			),
		)

	if storedBase == "" ||
		originalBase == "" {
		return false
	}

	if storedBase ==
		originalBase {
		return true
	}

	// Arquivos confirmados recebem prefixo aleatório:
	//
	//   714e3cca-EYESUITE__AO__PACIENTE.pdf
	//
	// enquanto exam.source preserva:
	//
	//   EYESUITE__AO__PACIENTE.pdf
	return strings.HasSuffix(
		storedBase,
		"-"+originalBase,
	)
}

func classifySourceFilesFromExams(
	analysis map[string]any,
) {
	exams, _ := analysis["exams"].(map[string]any)

	if len(exams) == 0 {
		return
	}

	sourceFiles, _ := analysis["source_files"].([]any)

	if len(sourceFiles) == 0 {
		return
	}

	classifications :=
		make(
			map[string]examSourceClassification,
		)

	for examKey,
		rawExam :=
		range exams {

		exam, ok :=
			rawExam.(map[string]any)

		if !ok ||
			exam == nil {
			continue
		}

		for _, filename :=
			range examSourceNames(
				exam["source"],
			) {

			if filename == "" {
				continue
			}

			classifications[
				filename,
			] =
				examSourceClassification{
					Exam: examKey,
					Eye:
						localExamEyeFromFilename(
							filename,
						),
				}
		}
	}

	for _, rawSource :=
		range sourceFiles {

		source, ok :=
			rawSource.(map[string]any)

		if !ok ||
			source == nil {
			continue
		}

		currentExam, _ :=
			source["exam"].(string)

		currentEye, _ :=
			source["eye"].(string)

		if strings.TrimSpace(
			currentExam,
		) != "" &&
			strings.TrimSpace(
				currentEye,
			) != "" {
			continue
		}

		path, _ :=
			source["path"].(string)

		if path == "" {
			continue
		}

		for originalFilename,
			classification :=
			range classifications {

			if !sourceFilenameMatches(
				path,
				originalFilename,
			) {
				continue
			}

			if strings.TrimSpace(
				currentExam,
			) == "" {
				source["exam"] =
					classification.Exam
			}

			if strings.TrimSpace(
				currentEye,
			) == "" &&
				classification.Eye != "" {

				source["eye"] =
					classification.Eye
			}

			break
		}
	}
}
