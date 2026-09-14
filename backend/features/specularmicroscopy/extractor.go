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

	"refratia/backend/shared/ocr"
	progressutil "refratia/backend/shared/progress"
)

type OCRPanel struct {
	Laterality string
	Text       string
}

type Evidence struct {
	Laterality string  `json:"laterality"`
	Context    string  `json:"context"`
	Value      float64 `json:"value"`
}

type Result struct {
	Exam     map[string]any
	Evidence []Evidence
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

		eyes[eye] = map[string]any{
			"cell_density_cells_per_mm2": value,
		}
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

	labelFound := false

	// "Cell Density" é o marcador semântico.
	//
	// OCR real do equipamento demonstrou:
	//
	//   Cell Density (C0) 2403
	//
	// e também:
	//
	//   Cell Density (CO)
	//   cells/mm? 2184
	//
	// Portanto não exigimos que "CD" ou "mm"
	// estejam na MESMA linha do label.
	for index, line := range lines {
		if !strings.Contains(
			line,
			"cell density",
		) {
			continue
		}

		labelFound = true

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

		match := regexp.MustCompile(
			`(?i)cell\s+density.*?([1-9][0-9]{2,4})`,
		).FindStringSubmatch(
			contextText,
		)

		if len(match) != 2 {
			continue
		}

		value, err :=
			strconv.ParseFloat(
				match[1],
				64,
			)

		if err != nil {
			continue
		}

		// Faixa apenas como barreira contra lixo OCR.
		// Não é interpretação clínica.
		if value < 500 || value > 10000 {
			continue
		}

		return value,
			contextText,
			"",
			true
	}

	if !labelFound {
		return 0,
			"",
			"Cell Density label not found",
			false
	}

	return 0,
		"",
		"Cell Density label found but numeric value not found nearby",
		false
}

func normalizeOCRText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "²", "2")
	value = strings.Join(strings.Fields(value), " ")
	return value
}

func Extract(ctx context.Context, data []byte) (Result, error) {
	progressutil.Report(ctx, 0, "microscopy_preprocess", "Preparando painéis da microscopia especular")

	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return Result{}, fmt.Errorf("microscopia especular: JPEG inválido: %w", err)
	}

	panels := make([]OCRPanel, 0, 2)
	for index, panel := range microscopyPanels(img.Bounds()) {
		progressutil.Report(ctx, 10+index*25, "microscopy_preprocess", fmt.Sprintf("Preparando painel %d/2", index+1))
		pngData, err := preprocessPanel(img, panel)
		if err != nil {
			return Result{}, err
		}

		progressutil.Report(ctx, 20+index*25, "microscopy_ocr", fmt.Sprintf("Executando OCR do painel %d/2", index+1))
		tsv, err := ocr.RunTesseractTSVWithPSM(ctx, pngData, 6)
		if err != nil {
			return Result{}, fmt.Errorf("microscopia especular: OCR do painel %d: %w", index+1, err)
		}

		words, _, err := ocr.ParseTSVWords(tsv)
		if err != nil {
			return Result{}, fmt.Errorf("microscopia especular: TSV do painel %d: %w", index+1, err)
		}
		rows := ocr.GroupRows(words, 12)
		textRows := make([]string, 0, len(rows))
		for _, row := range rows {
			textRows = append(textRows, ocr.RowText(row))
		}
		panelText := strings.Join(textRows, "\n")
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
			img.Bounds(),
			panel,
			pngData,
			tsv,
			panelText,
			laterality,
		)

		panels = append(
			panels,
			OCRPanel{
				Laterality: laterality,
				Text:       panelText,
			},
		)
	}

	progressutil.Report(ctx, 75, "microscopy_parse", "Interpretando densidade endotelial")
	exam, evidence, err := ParseOCRPanels(panels)
	if err != nil {
		return Result{}, err
	}
	progressutil.Report(ctx, 100, "microscopy_parse", "Microscopia especular extraída localmente")
	return Result{Exam: exam, Evidence: evidence}, nil
}

func specularOCRDebugEnabled() bool {
	return os.Getenv("REFRATIA_OCR_DEBUG") == "1"
}

func debugSpecularPanel(index int, original image.Rectangle, panel panelRect, processed []byte, rawTSV, lines, laterality string) {
	if !specularOCRDebugEnabled() {
		return
	}

	panelName := []string{"R", "L"}[index]
	processedImage, err := png.Decode(bytes.NewReader(processed))
	processedSize := "unknown"
	if err == nil {
		processedSize = fmt.Sprintf("%dx%d", processedImage.Bounds().Dx(), processedImage.Bounds().Dy())
	}

	_, _, reason, ok := parseCellDensityDetailed(lines)
	if ok {
		reason = "match"
	}
	if laterality == "" {
		laterality = "<missing>"
	}

	fmt.Fprintf(os.Stderr, "=== SPECULAR OCR DEBUG ===\npanel=%s\noriginal=%dx%d\nroi=%d,%d,%d,%d\ncrop=%dx%d\nprocessed=%s\npreprocess=raw_crop psm=6\n--- RAW OCR ---\n%s\n--- RECONSTRUCTED LINES ---\n%s\n--- RELEVANT TOKENS ---\n%s\n--- PARSE ---\nlaterality=%s\ncell_density=%s\nreason=%q\n", panelName, original.Dx(), original.Dy(), panel.x, panel.y, panel.x+panel.w, panel.y+panel.h, panel.w, panel.h, processedSize, redactDebugTSV(rawTSV), redactDebugLines(lines), relevantOCRTokens(lines), laterality, densityDebugValue(lines), reason)

	if os.Getenv("REFRATIA_OCR_DEBUG_SAVE") == "1" {
		path := fmt.Sprintf("/tmp/specular-%s-preprocessed.png", panelName)
		if err := os.WriteFile(path, processed, 0600); err != nil {
			fmt.Fprintf(os.Stderr, "debug_save_error=%q\n", err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "debug_saved=%s\n", path)
		}
	}

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
