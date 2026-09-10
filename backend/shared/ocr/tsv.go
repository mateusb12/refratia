package ocr

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

type Word struct {
	Text          string
	Left, Top     int
	Width, Height int
}

type Row struct {
	Y     int
	Words []Word
}

func ParseTSVWords(tsv string) ([]Word, int, error) {
	lines := strings.Split(tsv, "\n")
	words := make([]Word, 0)
	pageWidth := 0

	for _, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) < 12 {
			continue
		}

		level, _ := strconv.Atoi(fields[0])
		left, _ := strconv.Atoi(fields[6])
		top, _ := strconv.Atoi(fields[7])
		width, _ := strconv.Atoi(fields[8])
		height, _ := strconv.Atoi(fields[9])

		if level == 1 && width > pageWidth {
			pageWidth = width
		}

		if level != 5 || strings.TrimSpace(fields[11]) == "" {
			continue
		}

		words = append(words, Word{
			Text:   strings.TrimSpace(fields[11]),
			Left:   left,
			Top:    top,
			Width:  width,
			Height: height,
		})
	}

	return words, pageWidth, nil
}

func GroupRows(words []Word, tolerance int) []Row {
	sort.Slice(words, func(i, j int) bool {
		if words[i].Top == words[j].Top {
			return words[i].Left < words[j].Left
		}

		return words[i].Top < words[j].Top
	})

	rows := make([]Row, 0)

	for _, word := range words {
		if len(rows) == 0 ||
			int(math.Abs(float64(rows[len(rows)-1].Y-word.Top))) > tolerance {

			rows = append(rows, Row{
				Y:     word.Top,
				Words: []Word{word},
			})

			continue
		}

		row := &rows[len(rows)-1]
		row.Words = append(row.Words, word)
	}

	for index := range rows {
		sort.Slice(rows[index].Words, func(i, j int) bool {
			return rows[index].Words[i].Left <
				rows[index].Words[j].Left
		})
	}

	return rows
}

func RowText(row Row) string {
	parts := make([]string, len(row.Words))

	for index, word := range row.Words {
		parts[index] = word.Text
	}

	return strings.Join(parts, " ")
}
