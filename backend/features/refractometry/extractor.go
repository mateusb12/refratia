package refractometry

import (
	"context"
	"fmt"
	"image"
	"math"
	"regexp"
	"sort"
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

	page, err := pdfutil.RenderPageAtDPI(ctx, data, 1, 600)
	if err != nil {
		return nil, err
	}
	return ExtractImage(ctx, page)
}

// ExtractImage reads the same ticket from a PNG/JPEG (scan or phone photo).
func ExtractImage(ctx context.Context, data []byte) (map[string]any, error) {
	img, err := ocr.DecodeImage(data)
	if err != nil {
		return nil, fmt.Errorf("refratometria: imagem inválida")
	}

	tsv, err := ocr.RunTesseractTSVWithPSM(ctx, data, 4)
	if err != nil {
		return nil, err
	}

	words, _, err := ocr.ParseTSVWords(tsv)
	if err != nil {
		return nil, err
	}

	eyes := map[string]any{}
	examTimestamp := ""
	// Row tolerance scales with the image (about 0.6% of its height).
	tolerance := max(8, img.Bounds().Dy()*6/1000)
	type marker struct {
		top int
		eye string
	}
	var markers []marker
	for _, row := range ocr.GroupRows(words, tolerance) {
		text := ocr.RowText(row)
		if examTimestamp == "" {
			examTimestamp = examdate.FromOCR(text)
		}
		if m := eyeMarkerPattern.FindStringSubmatch(text); len(m) == 2 {
			eye := "OS"
			if m[1] == "R" || m[1] == "r" {
				eye = "OD"
			}
			markers = append(markers, marker{top: row.Y, eye: eye})
		}
	}

	// AVE rows are located by their own label word, not by row grouping: a
	// photographed (slightly rotated) ticket does not keep a row's words at
	// one baseline. The row is every word within one line height of the label.
	for _, anchor := range words {
		if !averagePattern.MatchString(anchor.Text) {
			continue
		}
		activeEye := ""
		for _, m := range markers {
			if m.top <= anchor.Top {
				activeEye = m.eye
			}
		}
		if activeEye == "" {
			continue
		}
		center := anchor.Top + anchor.Height/2
		line := ocr.Row{Y: anchor.Top}
		for _, w := range words {
			if w.Left+w.Width >= anchor.Left && abs(w.Top+w.Height/2-center) <= anchor.Height {
				line.Words = append(line.Words, w)
			}
		}
		sort.Slice(line.Words, func(i, j int) bool { return line.Words[i].Left < line.Words[j].Left })

		// Preferred reading: re-OCR just this row, upscaled, with a digit
		// whitelist, and parse it by its printed format (two decimals for
		// SPH/CYL, integer axis). This fixes the slashed zero of the printer
		// font that the page-level pass reads as 8/@/6.
		values, ok := parseAverageRowCrop(ctx, img, anchor, line)
		if !ok {
			values, ok = parseAverageRow(ocr.RowText(line))
		}
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

// strictAverage matches the printed AVE row: label, SPH and CYL with exactly
// two decimals and a signed value each, then the axis. The sign is preserved.
var strictAverage = regexp.MustCompile(`AVE\s*([+-]?\d{1,2}\.\d{2})\s*([+-]?\d{1,2}\.\d{2})\s*(\d{1,3})`)

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// readVariants are the independent preprocessing/PSM combinations whose
// readings are voted for one AVE row.
var readVariants = []struct {
	scale, binarize, psm int
	thicken              bool
}{
	{2, 0, 7, false}, {3, 0, 7, false}, {3, 0, 13, false},
	{2, 55, 7, true}, {3, 55, 7, true}, {3, 65, 7, true}, {3, 65, 13, true},
	{2, 60, 7, false}, {3, 60, 7, false},
}

// parseAverageRowCrop re-OCRs the AVE row under several preprocessings and
// returns, per column, the reading that a clear majority of the parseable
// reads agree on. Reads that are not on the printed grid (0.25 D for SPH/CYL,
// axis 0..180) are discarded before voting. A column without a clear majority
// makes the whole row unreadable: a missing value is better than a guess.
func parseAverageRowCrop(ctx context.Context, img image.Image, anchor ocr.Word, line ocr.Row) ([3]float64, bool) {
	var result [3]float64
	top, bottom := anchor.Top, anchor.Top+anchor.Height
	right := anchor.Left + anchor.Width
	for _, w := range line.Words {
		top = min(top, w.Top)
		bottom = max(bottom, w.Top+w.Height)
		right = max(right, w.Left+w.Width)
	}
	pad := (bottom - top) / 4
	rect := image.Rect(anchor.Left-pad, top-pad, right+pad, bottom+pad)

	var votes [3]map[float64]int
	for i := range votes {
		votes[i] = map[float64]int{}
	}
	reads := 0
	for _, v := range readVariants {
		gray := ocr.PrepareGray(img, rect, v.scale, 0)
		ocr.AutoContrast(gray)
		if v.binarize > 0 {
			gray = ocr.Binarize(gray, v.binarize)
		}
		if v.thicken {
			gray = ocr.ThickenDark(gray)
		}
		encoded, err := ocr.EncodePNG(gray)
		if err != nil {
			continue
		}
		text, err := ocr.RunTesseractText(ctx, encoded, v.psm, "0123456789.+-AVE")
		if err != nil {
			continue
		}
		m := strictAverage.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		var parsed [3]float64
		ok := true
		for i := range parsed {
			f, err := strconv.ParseFloat(m[i+1], 64)
			if err != nil {
				ok = false
				break
			}
			parsed[i] = f
		}
		if !ok || !onQuarterGrid(parsed[0]) || !onQuarterGrid(parsed[1]) || parsed[2] < 0 || parsed[2] > 180 {
			continue
		}
		reads++
		for i := range parsed {
			votes[i][parsed[i]]++
		}
	}
	if reads < 2 {
		return result, false
	}
	for i := range votes {
		best, bestN := 0.0, 0
		for value, n := range votes[i] {
			if n > bestN {
				best, bestN = value, n
			}
		}
		if bestN*2 <= reads { // need a strict majority of the usable reads
			return result, false
		}
		result[i] = best
	}
	return result, true
}

func onQuarterGrid(v float64) bool {
	q := v * 4
	return q-math.Round(q) < 1e-6 && math.Round(q)-q < 1e-6
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
