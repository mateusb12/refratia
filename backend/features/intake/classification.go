package intake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
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
	FilenameConflict   bool            `json:"filenameConflict,omitempty"`
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
		}

		// O tipo vem do conteúdo impresso (título do aparelho), não da
		// contagem de páginas nem do nome do arquivo.
		if examType, _ := ClassifyContent(
			ctx,
			file.ContentType,
			file.Data,
		); examType != "" {
			item.Candidates = append(
				item.Candidates,
				ExamCandidate{
					ExamType: examType,
					Score:    95,
					Reason:   "título do aparelho reconhecido no conteúdo do PDF",
				},
			)

			if item.RecognizedExamType != examType {
				// Conteúdo prevalece sobre o nome; o frontend vê a divergência.
				item.FilenameConflict = item.RecognizedExamType != ""
				item.RecognizedExamType = examType
				item.RecognizedEye = ""
			}
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
			item.RecognizedExamType = "specular_microscopy"
			item.RecognizedEye = "AO"
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

	if item.RecognizedExamType == "" &&
		strings.HasPrefix(strings.ToLower(file.ContentType), "image/") {
		if examType, _ := ClassifyContent(
			ctx,
			file.ContentType,
			file.Data,
		); examType != "" {
			item.RecognizedExamType = examType
			item.RecognizedEye = ""
			item.Candidates = append(
				item.Candidates,
				ExamCandidate{
					ExamType: examType,
					Score:    85,
					Reason:   "conteúdo da imagem reconhecido por OCR",
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
	filename = filepath.Base(strings.TrimSpace(filename))
	parts := strings.Split(filename, "__")

	if len(parts) >= 4 && canonicalTimestampPattern.MatchString(
		strings.TrimSuffix(parts[len(parts)-1], fileExtension(filename)),
	) && strings.TrimSpace(strings.Join(parts[2:len(parts)-1], "__")) != "" {
		if examType, ok := examTypeFromFilenameLabel(parts[0]); ok {
			if eye, ok := normalizeFilenameEye(parts[1]); ok {
				return examType, eye, true
			}
		}
	}

	stem := strings.TrimSuffix(filename, fileExtension(filename))
	examMatch := regexp.MustCompile(`(?i)(^|[[:space:]_-])(PENTACAM|EYESUITE|BIOMETRIA|AUTO|RETINA|OCT|CORNEA|ANTSEG|TOPOGRAFIA|TOPO|MICROSCOPIA([_[:space:]]+ESPECULAR)?)([[:space:]_-]|$)`).FindStringSubmatch(stem)
	if len(examMatch) != 5 {
		return "", "", false
	}

	examType, ok := examTypeFromFilenameLabel(examMatch[2])
	if !ok {
		return "", "", false
	}

	if examType == "specular_microscopy" {
		return examType, "AO", true
	}

	eyeMatch := regexp.MustCompile(`(?i)(^|[[:space:]_-])(OD|OE|OS|AO)([[:space:]_-]|$)`).FindStringSubmatch(stem)
	if len(eyeMatch) != 4 {
		return "", "", false
	}
	eye, ok := normalizeFilenameEye(eyeMatch[2])
	return examType, eye, ok
}

func examTypeFromFilenameLabel(label string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(label)) {
	case "PENTACAM":
		return "pentacam_corneal_tomography", true
	case "EYESUITE", "BIOMETRIA", "AUTO":
		if strings.EqualFold(strings.TrimSpace(label), "AUTO") {
			return "refractometry", true
		}
		return "iol_calculation", true
	case "MICROSCOPIA_ESPECULAR", "MICROSCOPIA ESPECULAR", "MICROSCOPIA":
		return "specular_microscopy", true
	case "RETINA":
		return "fundus_retinography", true
	case "OCT":
		return "oct_retina", true
	case "CORNEA", "ANTSEG":
		return "anterior_segment_photo", true
	case "TOPO", "TOPOGRAFIA":
		return "corneal_topography_other", true
	default:
		return "", false
	}
}

func normalizeFilenameEye(value string) (string, bool) {
	eye := strings.ToUpper(strings.TrimSpace(value))
	if eye == "OE" {
		eye = "OS"
	}
	return eye, validEye(eye)
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
				ContractExamType(
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

			if examType == "specular_microscopy" {
				eye = "AO"
			} else if !validEye(eye) {
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

// ContractExamType is the single exam-type vocabulary of the pipeline: the
// contract key (ground truth `exam_type`). It also accepts the legacy upper
// case labels that older callers and stored checkpoints carry, and the
// out-of-protocol types that are recognised for identity only.
func ContractExamType(value string) (string, bool) {
	value = strings.TrimSpace(value)
	switch value {
	case "pentacam_corneal_tomography", "iol_calculation",
		"refractometry", "specular_microscopy", "fundus_retinography",
		"oct_retina", "anterior_segment_photo", "corneal_topography_other":
		return value, true
	}
	return examTypeFromFilenameLabel(value)
}

// ExamTypeFromContract returns the legacy upper case label of a contract key.
//
// Deprecated: use ContractExamType; kept for callers that still switch on
// labels.
func ExamTypeFromContract(
	value string,
) (string, bool) {
	switch strings.TrimSpace(value) {
	case "pentacam_corneal_tomography":
		return "PENTACAM", true

	case "iol_calculation":
		return "EYESUITE", true

	case "refractometry":
		return "AUTO", true

	case "specular_microscopy":
		return "MICROSCOPIA_ESPECULAR", true

	case "fundus_retinography":
		return "RETINA", true

	case "oct_retina":
		return "OCT", true

	case "anterior_segment_photo":
		return "ANTSEG", true

	case "corneal_topography_other":
		return "TOPO", true

	default:
		return "", false
	}
}

func CanonicalExamFilename(
	patient string,
	examType string,
	eye string,
	extension string,
	sha256 string,
) string {
	labels :=
		map[string]string{
			"pentacam_corneal_tomography": "PENTACAM",
			"PENTACAM":                    "PENTACAM",

			"iol_calculation": "EYESUITE",
			"EYESUITE":        "EYESUITE",

			"specular_microscopy":   "MICROSCOPIA_ESPECULAR",
			"MICROSCOPIA_ESPECULAR": "MICROSCOPIA_ESPECULAR",

			"refractometry": "AUTO",
			"AUTO":          "AUTO",

			"fundus_retinography": "RETINA",
			"RETINA":              "RETINA",

			"oct_retina": "OCT",
			"OCT":        "OCT",

			"anterior_segment_photo": "ANTSEG",
			"ANTSEG":                 "ANTSEG",

			"corneal_topography_other": "TOPO",
			"TOPO":                     "TOPO",
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

	// O prefixo do SHA-256 mantém únicas duas fotos do mesmo exame+olho no
	// mesmo lote (antes sobrescreviam-se) sem quebrar o parse do nome canônico.
	if len(sha256) > 8 {
		sha256 = sha256[:8]
	}

	return fmt.Sprintf(
		"%s__%s__%s__%s__%s.%s",
		label,
		eye,
		patient,
		sha256,
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
