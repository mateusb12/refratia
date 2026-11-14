package specularmicroscopy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"regexp"
	"strconv"
	"strings"

	"refratia/backend/src/shared/document"
	"refratia/backend/src/shared/examdate"
	"refratia/backend/src/shared/ocr"
	progress "refratia/backend/src/shared/progress"
)

type OCRPanel struct {
	Laterality string
	Text       string
	// ExtraTexts are further readings of the same panel (other upscale /
	// binarization / PSM); they only complete table rows the first lacks.
	ExtraTexts []string
}

type Evidence struct {
	Laterality string  `json:"laterality"`
	Context    string  `json:"context"`
	Value      float64 `json:"value"`
}

type Result struct {
	Exam         map[string]any
	Evidence     []Evidence
	ExamDateTime string
}

var ErrNoClinicalData = errors.New("microscopia especular: densidade endotelial não encontrada")

// ParseOCRPanels parses only the clinical row and its printed laterality.
// R/L are the labels printed by the NIDEK panel; they are mapped to OD/OS
// only after the textual label has been found.
func ParseOCRPanels(panels []OCRPanel) (map[string]any, []Evidence, error) {
	eyes := map[string]any{}
	evidence := make([]Evidence, 0, len(panels))

	for _, panel := range panels {
		eye, ok := normalizeLaterality(panel.Laterality)
		if !ok {
			continue
		}

		value, contextText, _, ok := parseCellDensityDetailed(panel.Text)
		if !ok {
			continue
		}

		fields := map[string]any{
			"cell_density_cells_per_mm2": value,
		}
		for key, other := range parseTableRows(value, true, append([]string{panel.Text}, panel.ExtraTexts...)...) {
			fields[key] = other
		}
		eyes[eye] = fields
		evidence = append(evidence, Evidence{
			Laterality: eye,
			Context:    contextText,
			Value:      value,
		})
	}

	if len(eyes) == 0 {
		return nil, nil, ErrNoClinicalData
	}

	return map[string]any{"eyes": eyes}, evidence, nil
}

func normalizeLaterality(value string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "R":
		return "OD", true
	case "L":
		return "OS", true
	case "OD", "OS":
		return strings.ToUpper(strings.TrimSpace(value)), true
	default:
		return "", false
	}
}

func parseCellDensity(text string) (float64, string, bool) {
	value, contextText, _, ok := parseCellDensityDetailed(text)
	return value, contextText, ok
}

func parseCellDensityDetailed(
	text string,
) (float64, string, string, bool) {
	rawLines := strings.Split(text, "\n")

	lines := make([]string, 0, len(rawLines))

	for _, raw := range rawLines {
		line := normalizeOCRText(raw)

		if line != "" {
			lines = append(lines, line)
		}
	}

	markerFound := false

	// Há dois caminhos aceitos para reconhecer a densidade:
	//
	// 1. label textual preservado:
	//
	//      Cell Density (CD) cells/mm² 2403
	//
	//    ou:
	//
	//      Cell Density (CO)
	//      cells/mm? 2184
	//
	// 2. fallback semântico para OCR real do NIDEK:
	//
	//      cece cell Dersiy (0) icells/m?| 3366 |
	//
	//    "Density" foi corrompido, mas permaneceram:
	//
	//      cell + cells/m + valor plausível
	//
	// O segundo caminho NÃO aceita apenas "Number of Cells",
	// porque contagem absoluta não possui a unidade cells/mm².
	for index, line := range lines {
		hasExactLabel :=
			strings.Contains(
				line,
				"cell density",
			)

		hasDensityUnitEvidence :=
			strings.Contains(
				line,
				"cell",
			) &&
				strings.Contains(
					line,
					"cells/m",
				)

		// Docker's Tesseract can merge and corrupt the label into text such as
		// "cllbemsity" and the unit into "cel [s/m?". Keep this fallback
		// narrow: require both a cell-like token and the distinctive tail of
		// "density", plus unit evidence, so unrelated cell counts remain out.
		hasDockerOCRDensityEvidence :=
			(strings.Contains(line, "cell") ||
				strings.Contains(line, "cll") ||
				strings.Contains(line, "cel")) &&
				strings.Contains(line, "emsity") &&
				(strings.Contains(line, "s/m") ||
					strings.Contains(line, "cells"))

		if !hasExactLabel &&
			!hasDensityUnitEvidence &&
			!hasDockerOCRDensityEvidence {
			continue
		}

		markerFound = true

		// Caminho original: permite que o valor esteja
		// na mesma linha ou em até duas linhas seguintes.
		if hasExactLabel {
			end := index + 2

			if end >= len(lines) {
				end = len(lines) - 1
			}

			contextLines :=
				lines[index : end+1]

			contextText :=
				strings.Join(
					contextLines,
					" ",
				)

			match :=
				regexp.MustCompile(
					`(?i)cell\s+density.*?([1-9][0-9]{2,4})`,
				).
					FindStringSubmatch(
						contextText,
					)

			if len(match) == 2 {
				value, err :=
					strconv.ParseFloat(
						match[1],
						64,
					)

				if err == nil &&
					value >= 500 &&
					value <= 10000 {
					return value,
						contextText,
						"",
						true
				}
			}
		}

		// Fallback para OCR em que a palavra "Density"
		// foi destruída mas a unidade específica da
		// densidade endotelial permaneceu reconhecível.
		//
		// Usamos SOMENTE a linha atual para não capturar
		// números de Average Area, SD etc. das linhas abaixo.
		if hasDensityUnitEvidence || hasDockerOCRDensityEvidence {
			matches :=
				regexp.MustCompile(
					`[1-9][0-9]{2,4}`,
				).
					FindAllString(
						line,
						-1,
					)

			for _, match := range matches {
				value, err :=
					strconv.ParseFloat(
						match,
						64,
					)

				if err != nil {
					continue
				}

				// Barreira contra lixo OCR.
				// Não representa interpretação clínica.
				if value < 500 ||
					value > 10000 {
					continue
				}

				return value,
					line,
					"",
					true
			}
		}
	}

	if !markerFound {
		return 0,
			"",
			"Cell Density semantic marker not found",
			false
	}

	return 0,
		"",
		"Cell Density marker found but numeric value not found nearby",
		false
}

func normalizeOCRText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "²", "2")
	value = strings.Join(strings.Fields(value), " ")
	return value
}

type panelOCRResult struct {
	ExtraTexts []string
	Processed  []byte
	TSV        string
	Text       string
}

// ocrMicroscopyPanel concentra o único caminho de OCR usado tanto
// pela detecção quanto pela extração clínica.
func ocrMicroscopyPanel(
	ctx context.Context,
	img image.Image,
	panel panelRect,
) (panelOCRResult, error) {
	return ocrMicroscopyPanelOpts(ctx, img, panel, true)
}

// ocrMicroscopyPanelOpts with collectExtras=false stops at the first reading
// that recognises the density row (what detection needs).
func ocrMicroscopyPanelOpts(
	ctx context.Context,
	img image.Image,
	panel panelRect,
	collectExtras bool,
) (panelOCRResult, error) {
	pngData, err := preprocessPanel(img, panel)
	if err != nil {
		return panelOCRResult{}, err
	}

	first, err := ocrPanelPNG(ctx, pngData, 6, 1)
	if err != nil {
		return panelOCRResult{}, err
	}

	// The raw crop can lose whole rows (small, low-contrast text). Re-read the
	// same panel upscaled x3 with the darkest-channel grey, auto-contrast and
	// binarized, under two page segmentation modes. The first reading in which
	// the density row is recognised is the main one; every other reading is
	// kept to complete result-table rows the main one lacks.
	crop := image.Rect(panel.x, panel.y, panel.x+panel.w, panel.y+panel.h)
	main := first
	mainOK := looksLikeSpecularMicroscopyText(first.Text)
	if mainOK && !collectExtras {
		return main, nil
	}
	var extras []string
	for _, variant := range []struct {
		binarize int
		psm      int
	}{{0, 6}, {0, 4}, {60, 6}, {60, 4}, {120, 6}, {120, 4}} {
		gray := ocr.PrepareGray(img, crop, 3, 0)
		ocr.AutoContrast(gray)
		if variant.binarize > 0 {
			gray = ocr.Binarize(gray, variant.binarize)
		}
		encoded, encodeErr := ocr.EncodePNG(gray)
		if encodeErr != nil {
			continue
		}
		result, ocrErr := ocrPanelPNG(ctx, encoded, variant.psm, 3)
		if ocrErr != nil {
			continue
		}
		if !mainOK && looksLikeSpecularMicroscopyText(result.Text) {
			main, mainOK = result, true
			if !collectExtras {
				break
			}
			continue
		}
		extras = append(extras, result.Text)
	}
	if !mainOK {
		extras = nil
	}
	main.ExtraTexts = extras
	return main, nil
}

// scale is the upscale factor of pngData: the row tolerance is in its pixels.
func ocrPanelPNG(executionContext context.Context, pngData []byte, psm, scale int) (panelOCRResult, error) {
	tsv, ocrError := ocr.RunTesseractTSVWithPSM(executionContext, pngData, psm)
	if ocrError != nil {
		return panelOCRResult{}, ocrError
	}
	words, _, parseError := ocr.ParseTSVWords(tsv)
	if parseError != nil {
		return panelOCRResult{}, parseError
	}
	return panelOCRResult{Processed: pngData, TSV: tsv, Text: document.RowsText(words, 12*scale)}, nil
}

func looksLikeSpecularMicroscopyText(
	text string,
) bool {
	_, _, _, ok :=
		parseCellDensityDetailed(text)

	return ok
}

// Detect responde somente se o arquivo parece uma microscopia
// especular compatível com o layout atualmente suportado.
//
// Não retorna valores clínicos e não inventa lateralidade.
//
// A evidência usada é a mesma do extrator:
// "Cell Density" + valor numericamente plausível.
//
// O primeiro painel positivo encerra a detecção, evitando OCR
// desnecessário do segundo painel.
func Detect(
	ctx context.Context,
	data []byte,
) (bool, error) {
	img, err :=
		jpeg.Decode(
			bytes.NewReader(data),
		)

	if err != nil {
		// O extrator atual suporta JPEG.
		// Outro formato simplesmente não é reconhecido pelo detector.
		return false, nil
	}

	for _, panel := range microscopyPanels(img.Bounds()) {
		result, err :=
			ocrMicroscopyPanelOpts(
				ctx,
				img,
				panel,
				false,
			)

		if err != nil {
			return false, err
		}

		if looksLikeSpecularMicroscopyText(
			result.Text,
		) {
			return true, nil
		}
	}

	return false, nil
}

func Extract(executionContext context.Context, data []byte) (Result, error) {
	progress.Report(executionContext, 0, "microscopy_preprocess", "Preparando painéis da microscopia especular")

	sourceImage, decodeError := jpeg.Decode(bytes.NewReader(data))
	if decodeError != nil {
		return Result{}, fmt.Errorf("microscopia especular: JPEG inválido: %w", decodeError)
	}

	var examDateTime string
	var fullImage bytes.Buffer
	if encodeError := png.Encode(&fullImage, sourceImage); encodeError == nil {
		if words, ocrError := document.OCRWords(executionContext, fullImage.Bytes(), 11); ocrError == nil {
			rows := ocr.GroupRows(words, 12)
			for _, row := range rows {
				if examDateTime = examdate.FromOCR(ocr.RowText(row)); examDateTime != "" {
					break
				}
			}
		}
	}

	panels := make([]OCRPanel, 0, 2)
	for index, panel := range microscopyPanels(sourceImage.Bounds()) {
		progress.Report(executionContext, 10+index*25, "microscopy_preprocess", fmt.Sprintf("Preparando painel %d/2", index+1))
		progress.Report(
			executionContext,
			20+index*25,
			"microscopy_ocr",
			fmt.Sprintf(
				"Executando OCR do painel %d/2",
				index+1,
			),
		)

		panelOCR, panelError :=
			ocrMicroscopyPanel(
				executionContext,
				sourceImage,
				panel,
			)
		if panelError != nil {
			return Result{},
				fmt.Errorf(
					"microscopia especular: OCR do painel %d: %w",
					index+1,
					panelError,
				)
		}

		panelText := panelOCR.Text
		// Template NIDEK AO validado nas fixtures reais:
		// painel superior = R (OD)
		// painel inferior = L (OS)
		expectedLaterality := []string{"R", "L"}[index]
		laterality := expectedLaterality

		// Se o OCR também conseguir ler a lateralidade impressa,
		// ela vira uma validação independente do layout.
		printedLaterality := findPrintedLaterality(panelText)

		if printedLaterality != "" {
			printedEye, printedOK :=
				normalizeLaterality(printedLaterality)

			expectedEye, expectedOK :=
				normalizeLaterality(expectedLaterality)

			if !printedOK ||
				!expectedOK ||
				printedEye != expectedEye {

				return Result{}, fmt.Errorf(
					"microscopia especular: lateralidade impressa %q diverge do painel %q",
					printedLaterality,
					expectedLaterality,
				)
			}

			laterality = printedLaterality
		}

		debugSpecularPanel(
			index,
			sourceImage.Bounds(),
			panel,
			panelOCR.Processed,
			panelOCR.TSV,
			panelText,
			laterality,
		)

		panels = append(
			panels,
			OCRPanel{
				Laterality: laterality,
				Text:       panelText,
				ExtraTexts: panelOCR.ExtraTexts,
			},
		)
	}

	progress.Report(executionContext, 75, "microscopy_parse", "Interpretando densidade endotelial")
	exam, evidence, parseError := ParseOCRPanels(panels)
	if parseError != nil {
		return Result{}, parseError
	}
	progress.Report(executionContext, 100, "microscopy_parse", "Microscopia especular extraída localmente")
	return Result{Exam: exam, Evidence: evidence, ExamDateTime: examDateTime}, nil
}

func specularOCRDebugEnabled() bool {
	return os.Getenv("REFRATIA_OCR_DEBUG") == "1"
}

// debugSpecularPanel prints geometry and the parse outcome only. The raw OCR
// text and the crop are never printed or saved: the panel header carries the
// patient's name and id.
func debugSpecularPanel(index int, original image.Rectangle, panel panelRect, _ []byte, _, lines, laterality string) {
	if !specularOCRDebugEnabled() {
		return
	}
	panelName := []string{"R", "L"}[index]
	_, _, reason, ok := parseCellDensityDetailed(lines)
	if ok {
		reason = "match"
	}
	if laterality == "" {
		laterality = "<missing>"
	}
	fmt.Fprintf(os.Stderr, "specular panel=%s original=%dx%d roi=%d,%d,%d,%d laterality=%s reason=%q\n",
		panelName, original.Dx(), original.Dy(), panel.x, panel.y, panel.x+panel.w, panel.y+panel.h, laterality, reason)
}

func redactDebugTSV(tsv string) string {
	rows := strings.Split(tsv, "\n")
	for index, row := range rows {
		fields := strings.Split(row, "\t")
		if len(fields) < 12 || fields[0] != "5" {
			continue
		}
		top, err := strconv.Atoi(fields[7])
		if err == nil && top < 60 {
			fields[11] = "[REDACTED_HEADER]"
			rows[index] = strings.Join(fields, "\t")
		}
	}
	return strings.Join(rows, "\n")
}

func redactDebugLines(lines string) string {
	kept := make([]string, 0)
	for _, line := range strings.Split(lines, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "name") || strings.Contains(lower, " no ") || strings.HasPrefix(lower, "id ") {
			kept = append(kept, "[REDACTED_HEADER]")
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func relevantOCRTokens(text string) string {
	tokens := make([]string, 0)
	for _, token := range strings.Fields(text) {
		lower := strings.ToLower(token)
		if strings.Contains(lower, "cell") || strings.Contains(lower, "dens") || strings.Contains(lower, "cd") || strings.Contains(lower, "mm") {
			tokens = append(tokens, token)
		}
	}
	return strings.Join(tokens, " ")
}

func densityDebugValue(text string) string {
	value, _, _, ok := parseCellDensityDetailed(text)
	if !ok {
		return "<missing>"
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

type panelRect struct{ x, y, w, h int }

func microscopyPanels(bounds image.Rectangle) []panelRect {
	half := bounds.Dy() / 2
	return []panelRect{
		{x: bounds.Min.X + bounds.Dx()*31/100, y: bounds.Min.Y, w: bounds.Dx() * 69 / 100, h: half},
		{x: bounds.Min.X + bounds.Dx()*31/100, y: bounds.Min.Y + half, w: bounds.Dx() * 69 / 100, h: bounds.Dy() - half},
	}
}

func preprocessPanel(
	src image.Image,
	panel panelRect,
) ([]byte, error) {
	r := image.Rect(
		panel.x,
		panel.y,
		panel.x+panel.w,
		panel.y+panel.h,
	).Intersect(src.Bounds())

	if r.Empty() {
		return nil, fmt.Errorf(
			"microscopia especular: painel vazio",
		)
	}

	crop := image.NewRGBA(
		image.Rect(
			0,
			0,
			r.Dx(),
			r.Dy(),
		),
	)

	draw.Draw(
		crop,
		crop.Bounds(),
		src,
		r.Min,
		draw.Src,
	)

	var output bytes.Buffer

	if err := png.Encode(
		&output,
		crop,
	); err != nil {
		return nil, err
	}

	return output.Bytes(), nil
}

func findPrintedLaterality(text string) string {
	for _, token := range strings.Fields(text) {
		switch strings.ToUpper(strings.Trim(token, "()[],:;")) {
		case "R", "L", "OD", "OS":
			return strings.ToUpper(strings.Trim(token, "()[],:;"))
		}
	}
	return ""
}
