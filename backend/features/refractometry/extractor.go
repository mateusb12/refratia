package refractometry

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"refratia/backend/shared/examdate"
	"refratia/backend/shared/ocr"
	pdfutil "refratia/backend/shared/pdf"
)

var (
	eyeMarkerPattern = regexp.MustCompile(`(?i)(?:<|\[)\s*([RL])\s*(?:>|\])`)
	numberPattern    = regexp.MustCompile(`[+-]?\d+(?:[.,]\d+)?`)
	averagePattern   = regexp.MustCompile(`(?i)\bA(?:V|U|G)E?\b`)
)

// ExtractPDF reads the average SPH/CYL/AX row printed for each eye by the
// refractometer. R/L are the device's right/left labels and become OD/OS.
func ExtractPDF(ctx context.Context, data []byte) (map[string]any, error) {
	pageCount, _, err := pdfutil.Inspect(ctx, data)
	if err != nil {
		return nil, err
	}
	if pageCount < 1 {
		return nil, fmt.Errorf("refratometria sem páginas")
	}

	image, err := pdfutil.RenderPageAtDPI(ctx, data, 1, 600)
	if err != nil {
		return nil, err
	}

	tsv, err := ocr.RunTesseractTSVWithPSM(ctx, image, 4)
	if err != nil {
		return nil, err
	}

	words, _, err := ocr.ParseTSVWords(tsv)
	if err != nil {
		return nil, err
	}

	eyes := map[string]any{}
	activeEye := ""
	examTimestamp := ""
	for _, row := range ocr.GroupRows(words, 40) {
		text := ocr.RowText(row)
		if examTimestamp == "" {
			examTimestamp = examdate.FromOCR(text)
		}
		if marker := eyeMarkerPattern.FindStringSubmatch(text); len(marker) == 2 {
			if marker[1] == "R" || marker[1] == "r" {
				activeEye = "OD"
			} else {
				activeEye = "OS"
			}
		}

		if activeEye == "" || !averagePattern.MatchString(text) {
			continue
		}

		values, ok := parseAverageRow(text)
		if !ok {
			continue
		}
		eyes[activeEye] = map[string]any{
			"sphere_d":   values[0],
			"cylinder_d": values[1],
			"axis_deg":   values[2],
		}
	}

	if len(eyes) == 0 {
		return nil, fmt.Errorf("refratometria: nenhuma média SPH/CYL/AX encontrada")
	}

	result := map[string]any{"eyes": eyes}
	if examTimestamp != "" {
		result["exam_datetime"] = examTimestamp
	}
	return result, nil
}

func parseAverageRow(text string) ([3]float64, bool) {
	var result [3]float64
	marker := averagePattern.FindStringIndex(text)
	if marker == nil {
		return result, false
	}

	text = strings.ReplaceAll(text[marker[1]:], ",", ".")
	text = strings.ReplaceAll(text, ". ", ".")
	numbers := numberPattern.FindAllString(text, -1)
	if len(numbers) < 3 {
		return result, false
	}

	for index := range result {
		value, err := strconv.ParseFloat(numbers[index], 64)
		if err != nil {
			return result, false
		}
		result[index] = value
	}

	return result, true
}
