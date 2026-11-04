package pentacam

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"refratia/backend/shared/ocr"
	pdfutil "refratia/backend/shared/pdf"
)

type Identity struct {
	FullName     string
	BirthDateRaw string
}

var pentacamBirthDatePattern = regexp.MustCompile(`\b\d{1,2}/\d{1,2}/\d{4}\b`)

// ExtractIdentity reads the patient header from the Pentacam general page.
// It is intentionally independent from the clinical-page extractor because
// identity is a prerequisite for presenting a case for clinical review.
func ExtractIdentity(ctx context.Context, data []byte) (Identity, error) {
	image, err := pdfutil.RenderPageAtDPI(ctx, data, 1, 300)
	if err != nil {
		return Identity{}, err
	}

	tsv, err := ocr.RunTesseractTSV(ctx, image)
	if err != nil {
		return Identity{}, err
	}

	words, _, err := ocr.ParseTSVWords(tsv)
	if err != nil {
		return Identity{}, err
	}

	return parsePentacamIdentityWords(words)
}

func parsePentacamIdentityWords(words []ocr.Word) (Identity, error) {
	rows := ocr.GroupRows(words, 12)

	firstName := valueAfterLabel(rows, `(?i)\bnome\b`)
	lastName := valueAfterLabel(rows, `(?i)sobrenome`)
	birthDate := ""

	for _, row := range rows {
		if match := pentacamBirthDatePattern.FindString(ocr.RowText(row)); match != "" {
			birthDate = match
			break
		}
	}

	fullName := strings.TrimSpace(strings.Join([]string{firstName, lastName}, " "))
	if fullName == "" || birthDate == "" {
		return Identity{}, errors.New("Pentacam: nome/nascimento não localizados")
	}

	return Identity{
		FullName:     fullName,
		BirthDateRaw: birthDate,
	}, nil
}

func valueAfterLabel(rows []ocr.Row, labelPattern string) string {
	label := regexp.MustCompile(labelPattern)

	for index, row := range rows {
		text := ocr.RowText(row)
		location := label.FindStringIndex(text)
		if location == nil {
			continue
		}

		value := strings.TrimSpace(text[location[1]:])
		value = cleanPentacamIdentityValue(value)
		if value != "" {
			return value
		}

		if index+1 < len(rows) {
			value = cleanPentacamIdentityValue(ocr.RowText(rows[index+1]))
			if value != "" {
				return value
			}
		}
	}

	return ""
}

func cleanPentacamIdentityValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, " |}[]():;,-")
	value = strings.Join(strings.Fields(value), " ")
	return value
}
