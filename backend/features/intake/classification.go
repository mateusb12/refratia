package intake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"regexp"
	"strings"
	"time"

	specularmicroscopy "refratia/backend/features/specularmicroscopy"
	pdfutil "refratia/backend/shared/pdf"
)

type File struct {
	Filename    string
	SHA256      string
	ContentType string
	Size        int64
	Data        []byte
}

type FileInspection struct {
	Filename           string          `json:"filename"`
	SHA256             string          `json:"sha256"`
	ContentType        string          `json:"contentType"`
	Size               int64           `json:"size"`
	Pages              int             `json:"pages,omitempty"`
	Width              int             `json:"width,omitempty"`
	Height             int             `json:"height,omitempty"`
	RecognizedExamType string          `json:"recognizedExamType,omitempty"`
	RecognizedEye      string          `json:"recognizedEye,omitempty"`
	Candidates         []ExamCandidate `json:"candidates"`
}

type ExamCandidate struct {
	ExamType string `json:"examType"`
	Score    int    `json:"score"`
	Reason   string `json:"reason"`
}

type Classification struct {
	SHA256   string `json:"sha256"`
	ExamType string `json:"examType"`
	Eye      string `json:"eye"`
}

func InspectFile(
	ctx context.Context,
	file File,
) FileInspection {
	item := FileInspection{
		Filename:    file.Filename,
		SHA256:      file.SHA256,
		ContentType: file.ContentType,
		Size:        file.Size,
		Candidates:  []ExamCandidate{},
	}

	if examType, eye, ok :=
		CanonicalClassificationFromFilename(file.Filename); ok {
		item.RecognizedExamType = examType
		item.RecognizedEye = eye
	}

	if file.ContentType == "application/pdf" {
		pages, _, err := pdfutil.Inspect(ctx, file.Data)

		if err == nil {
			item.Pages = pages
			item.Candidates =
				prefilterPDFCandidates(pages)
		}

		return item
	}

	// Para imagens, dimensão continua sendo metadata útil para a UI,
	// mas NÃO é usada como evidência do tipo de exame.
	if config, _, err :=
		image.DecodeConfig(bytes.NewReader(file.Data)); err == nil {
		item.Width = config.Width
		item.Height = config.Height
	}

	// Para imagens, o tipo NÃO é inferido por resolução,
	// dimensão ou aspect ratio.
	//
	// Microscopia especular só vira candidata quando o
	// detector da própria feature encontra a mesma evidência
	// semântica exigida pelo extrator determinístico.
	if strings.HasPrefix(
		strings.ToLower(file.ContentType),
		"image/",
	) {
		detected, detectErr :=
			specularmicroscopy.Detect(
				ctx,
				file.Data,
			)

		// Falha de OCR no prefiltro não deve impedir upload.
		// Nesse caso o usuário simplesmente classifica manualmente.
		if detectErr == nil && detected {
			item.Candidates =
				append(
					item.Candidates,
					ExamCandidate{
						ExamType: "specular_microscopy",
						Score:    90,
						Reason: "OCR reconheceu Cell Density " +
							"com valor plausível",
					},
				)
		}
	}

	return item
}

var canonicalTimestampPattern = regexp.MustCompile(`^\d{8}_\d{4,6}$`)

func CanonicalClassificationFromFilename(
	filename string,
) (string, string, bool) {
	parts := strings.Split(filename, "__")

	if len(parts) < 4 ||
		!canonicalTimestampPattern.MatchString(
			strings.TrimSuffix(
				parts[len(parts)-1],
				fileExtension(filename),
			),
		) {
		return "", "", false
	}

	eye :=
		strings.ToUpper(
			strings.TrimSpace(parts[1]),
		)

	if !validEye(eye) {
		return "", "", false
	}

	var examType string

	switch strings.ToUpper(
		strings.TrimSpace(parts[0]),
	) {
	case "PENTACAM":
		examType = "pentacam_corneal_tomography"

	case "EYESUITE":
		examType = "iol_calculation"

	case "MICROSCOPIA_ESPECULAR":
		examType = "specular_microscopy"

	case "RETINA":
		examType = "fundus_retinography"

	case "OCT":
		examType = "oct_retina"

	default:
		return "", "", false
	}

	if strings.TrimSpace(
		strings.Join(
			parts[2:len(parts)-1],
			"__",
		),
	) == "" {
		return "", "", false
	}

	return examType, eye, true
}

func ParseClassifications(
	values []string,
) (map[string]Classification, error) {
	result :=
		make(map[string]Classification)

	for _, raw := range values {
		if strings.TrimSpace(raw) == "" {
			continue
		}

		var classifications []Classification

		if err :=
			json.Unmarshal(
				[]byte(raw),
				&classifications,
			); err != nil {
			return nil,
				fmt.Errorf(
					"classifications JSON inválido: %w",
					err,
				)
		}

		for _, classification := range classifications {
			sha :=
				strings.TrimSpace(
					classification.SHA256,
				)

			if sha == "" {
				return nil,
					fmt.Errorf(
						"classification sem sha256",
					)
			}

			examType, ok :=
				ExamTypeFromContract(
					classification.ExamType,
				)

			if !ok {
				return nil,
					fmt.Errorf(
						"examType inválido: %q",
						classification.ExamType,
					)
			}

			eye :=
				strings.ToUpper(
					strings.TrimSpace(
						classification.Eye,
					),
				)

			if !validEye(eye) {
				return nil,
					fmt.Errorf(
						"eye inválido: %q",
						classification.Eye,
					)
			}

			result[sha] =
				Classification{
					SHA256:   sha,
					ExamType: examType,
					Eye:      eye,
				}
		}
	}

	return result, nil
}

func ExamTypeFromContract(
	value string,
) (string, bool) {
	switch strings.TrimSpace(value) {
	case "pentacam_corneal_tomography":
		return "PENTACAM", true

	case "iol_calculation":
		return "EYESUITE", true

	case "specular_microscopy":
		return "MICROSCOPIA_ESPECULAR", true

	case "fundus_retinography":
		return "RETINA", true

	case "oct_retina":
		return "OCT", true

	default:
		return "", false
	}
}

func CanonicalExamFilename(
	patient string,
	examType string,
	eye string,
	extension string,
) string {
	labels :=
		map[string]string{
			"pentacam_corneal_tomography": "PENTACAM",
			"PENTACAM":                    "PENTACAM",

			"iol_calculation": "EYESUITE",
			"EYESUITE":        "EYESUITE",

			"specular_microscopy":   "MICROSCOPIA_ESPECULAR",
			"MICROSCOPIA_ESPECULAR": "MICROSCOPIA_ESPECULAR",

			"fundus_retinography": "RETINA",
			"RETINA":              "RETINA",

			"oct_retina": "OCT",
			"OCT":        "OCT",
		}

	label := labels[examType]

	if label == "" {
		label = "EXAME"
	}

	patient =
		strings.ToUpper(
			strings.TrimSpace(patient),
		)

	patient =
		strings.Join(
			strings.Fields(patient),
			"_",
		)

	if patient == "" {
		patient = "PACIENTE"
	}

	eye =
		strings.ToUpper(
			strings.TrimSpace(eye),
		)

	if eye == "" {
		eye = "AO"
	}

	return fmt.Sprintf(
		"%s__%s__%s__%s.%s",
		label,
		eye,
		patient,
		time.Now().UTC().
			Format("20060102_150405"),
		extension,
	)
}

func validEye(
	eye string,
) bool {
	switch eye {
	case "OD", "OS", "AO":
		return true

	default:
		return false
	}
}

func fileExtension(
	filename string,
) string {
	index :=
		strings.LastIndex(
			filename,
			".",
		)

	if index < 0 {
		return ""
	}

	return filename[index:]
}

// Regra operacional validada com os Pentacam reais recebidos:
//
//   - os relatórios completos observados chegam com 9 páginas;
//   - usamos 8–10 páginas como faixa de pré-filtro;
//   - isto é somente uma sugestão inicial e pode ser corrigido pelo usuário.
//
// Não inferimos exames de imagem por resolução ou aspect ratio.
func prefilterPDFCandidates(
	pages int,
) []ExamCandidate {
	if pages < 8 || pages > 10 {
		return []ExamCandidate{}
	}

	return []ExamCandidate{
		{
			ExamType: "pentacam_corneal_tomography",
			Score:    75,
			Reason: "PDF com 8–10 páginas, " +
				"padrão compatível com Pentacam",
		},
	}
}
