package eyesuite

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"refratia/backend/shared/examdate"
	"refratia/backend/shared/ocr"
)

var decimalPattern = regexp.MustCompile(`[+-]?\d+[,.]\d+`)

func parseEyeSuiteTSV(tsv string) (map[string]any, error) {
	words, pageWidth, err := ocr.ParseTSVWords(tsv)
	if err != nil {
		return nil, err
	}
	if len(words) == 0 {
		return nil, errors.New("EyeSuite OCR vazio")
	}

	if pageWidth <= 0 {
		for _, word := range words {
			if right := word.Left + word.Width; right > pageWidth {
				pageWidth = right
			}
		}
	}
	if pageWidth <= 0 {
		return nil, errors.New("largura da página OCR inválida")
	}

	mid := pageWidth / 2
	eyes := map[string]any{}

	for _, eye := range []struct {
		Name       string
		MinX, MaxX int
	}{
		{Name: "OD", MinX: 0, MaxX: mid},
		{Name: "OS", MinX: mid, MaxX: pageWidth + 1},
	} {
		selected := make([]ocr.Word, 0)
		for _, word := range words {
			center := word.Left + word.Width/2
			if center >= eye.MinX && center < eye.MaxX {
				selected = append(selected, word)
			}
		}

		payload, err := parseEyeSuiteEye(selected)
		if err != nil {
			continue // that eye yielded nothing; the other one may still be good
		}
		eyes[eye.Name] = payload
	}
	if len(eyes) == 0 {
		return nil, errors.New("nenhum olho EyeSuite lido")
	}

	return map[string]any{"eyes": eyes}, nil
}

func parseEyeSuiteEye(words []ocr.Word) (map[string]any, error) {
	rows := ocr.GroupRows(words, 10)

	values := map[string]float64{}
	var astigAxis float64
	axisFound := false

	for _, row := range rows {
		text := ocr.RowText(row)

		if value, ok := decimalAfter(text, `\bAL\b`); ok {
			values["axial_length_mm"] = value
		}
		if value, ok := decimalAfter(text, `\bK1\b`); ok {
			values["k1_d"] = value
		}
		if value, ok := decimalAfter(text, `\bK2\b`); ok {
			values["k2_d"] = value
		}
		if value, ok := decimalAfter(text, `(?:^|\s)K(?:\s|\[)`); ok {
			values["mean_k_d"] = value
		}
		if value, ok := decimalAfter(text, `-?\s*AST\b`); ok {
			values["astigmatism_d"] = value
			if axis, found := axisAfter(text); found {
				astigAxis = axis
				axisFound = true
			}
		}
		if value, ok := decimalAfter(text, `\bACD\b`); ok {
			values["anterior_chamber_depth_mm"] = value
		}
		if value, ok := decimalAfter(text, `\bLT\b`); ok {
			values["lens_thickness_mm"] = value
		}
		if value, ok := decimalAfter(text, `\bWTW\b`); ok {
			values["white_to_white_mm"] = value
		}
		if value, ok := decimalAfter(text, `Target\s+Refraction`); ok {
			values["target_refraction_d"] = value
		}
	}

	// Partial results: a field the page did not yield stays absent (the
	// diagnostics report it as missing) instead of discarding the fields that
	// were read. Only an eye with nothing at all is an error.
	keratometry := map[string]any{}
	payload := map[string]any{}
	put := func(into map[string]any, key, valueKey string) {
		if value, ok := values[valueKey]; ok {
			into[key] = value
		}
	}
	put(payload, "axial_length_mm", "axial_length_mm")
	put(keratometry, "k1_d", "k1_d")
	put(keratometry, "k2_d", "k2_d")
	put(keratometry, "mean_k_d", "mean_k_d")
	put(keratometry, "astigmatism_d", "astigmatism_d")
	if axisFound {
		keratometry["astigmatism_axis_deg"] = astigAxis
	}
	if len(keratometry) > 0 {
		payload["keratometry"] = keratometry
	}
	put(payload, "anterior_chamber_depth_mm", "anterior_chamber_depth_mm")
	put(payload, "lens_thickness_mm", "lens_thickness_mm")
	put(payload, "white_to_white_mm", "white_to_white_mm")
	put(payload, "target_refraction_d", "target_refraction_d")

	if len(payload) == 0 {
		return nil, errors.New("nenhum campo EyeSuite lido")
	}
	return payload, nil
}

func decimalAfter(text, labelPattern string) (float64, bool) {
	label := regexp.MustCompile(`(?i)` + labelPattern)
	location := label.FindStringIndex(text)
	if location == nil {
		return 0, false
	}

	match := decimalPattern.FindString(text[location[1]:])
	if match == "" {
		return 0, false
	}

	value, err := strconv.ParseFloat(strings.ReplaceAll(match, ",", "."), 64)
	return value, err == nil
}

func axisAfter(text string) (float64, bool) {
	match := regexp.MustCompile(`@\s*(\d{1,3})`).FindStringSubmatch(text)
	if len(match) != 2 {
		return 0, false
	}
	value, err := strconv.ParseFloat(match[1], 64)
	return value, err == nil
}

type Identity struct {
	FullName     string
	BirthDateRaw string
	TimestampRaw string
}

var (
	eyeSuiteBirthDatePattern = regexp.MustCompile(`^\d{1,2}/\d{1,2}/\d{4}$`)
	eyeSuiteExamDatePattern  = regexp.MustCompile(`^\d{1,2}/\d{1,2}/(?:\d{2}|\d{4})$`)
	eyeSuiteTimePattern      = regexp.MustCompile(`^\d{1,2}:\d{2}(?::\d{2})?$`)
	eyeSuiteIDPattern        = regexp.MustCompile(`(?i)\bID\b.*\bCID\b`)
	eyeSuiteLetterPattern    = regexp.MustCompile(`\p{L}`)
)

func parseEyeSuiteIdentityTSV(tsv string) (Identity, error) {
	words, _, err := ocr.ParseTSVWords(tsv)
	if err != nil {
		return Identity{}, err
	}
	return parseEyeSuiteIdentityWords(words)
}

func parseEyeSuiteIdentityWords(words []ocr.Word) (Identity, error) {
	rows := ocr.GroupRows(words, 10)

	anchor := -1
	for i, row := range rows {
		rowText := ocr.RowText(row)
		if eyeSuiteIDPattern.MatchString(rowText) ||
			(strings.Contains(strings.ToUpper(rowText), "CALCULATION") &&
				strings.Contains(strings.ToUpper(rowText), "CID")) {
			anchor = i
			break
		}
	}
	if anchor < 0 {
		return Identity{}, errors.New("EyeSuite: âncora ID (CID) ausente")
	}

	var fullName, birthDate string

	for i := anchor; i >= 0 && i >= anchor-4; i-- {
		dateIndex := -1

		for j, word := range rows[i].Words {
			clean := strings.Trim(word.Text, " ,;:")
			if eyeSuiteBirthDatePattern.MatchString(clean) {
				dateIndex = j
				birthDate = clean
				break
			}
		}
		if dateIndex < 0 {
			continue
		}

		parts := make([]string, 0, dateIndex)
		for _, word := range rows[i].Words[:dateIndex] {
			clean := strings.Trim(word.Text, " ,;:")
			if clean != "" && eyeSuiteLetterPattern.MatchString(clean) {
				parts = append(parts, clean)
			}
		}

		if len(parts) >= 2 {
			fullName = strings.Join(parts, " ")
			break
		}
	}

	if fullName == "" || birthDate == "" {
		return Identity{}, errors.New("EyeSuite: nome/nascimento não localizados")
	}

	var examDate, examTime string

	// Alguns relatórios imprimem a data/hora imediatamente antes da linha
	// "Calculation ID", enquanto outros a imprimem depois. O fluxo normal
	// precisa reconhecer os dois layouts.
	for i := anchor - 2; i < len(rows) && i <= anchor+5; i++ {
		if i < 0 {
			continue
		}
		rowDate := ""
		rowTime := ""
		for _, word := range rows[i].Words {
			clean := strings.Trim(word.Text, " ,;:")

			if rowDate == "" && eyeSuiteExamDatePattern.MatchString(clean) {
				rowDate = clean
			}
			if rowTime == "" && eyeSuiteTimePattern.MatchString(clean) {
				rowTime = clean
			}
		}

		if rowDate != "" && rowTime != "" {
			examDate = rowDate
			examTime = rowTime
			break
		}
	}

	// The timestamp is optional: without it the name and birth date are still
	// valid identity evidence (the declared DMY order does not depend on it).
	timestamp := ""
	if examDate != "" && examTime != "" {
		timestamp = examdate.FromOCR(examDate + " " + examTime)
		if timestamp == "" {
			timestamp = examDate + " " + examTime
		}
	}

	return Identity{
		FullName:     fullName,
		BirthDateRaw: birthDate,
		TimestampRaw: timestamp,
	}, nil
}
