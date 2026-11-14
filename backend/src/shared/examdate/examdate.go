package examdate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var timestampPattern = regexp.MustCompile(`(?i)(\d{1,2})[/-](\d{1,2}|[a-z]{3})[/-](\d{2,4})(?:\s+(\d{1,2}):(\d{2})(?::(\d{2}))?)?`)

var monthNumbers = map[string]int{
	"jan": 1, "fev": 2, "feb": 2, "mar": 3, "abr": 4, "apr": 4,
	"mai": 5, "may": 5, "jun": 6, "jul": 7, "ago": 8, "aug": 8,
	"set": 9, "sep": 9, "out": 10, "oct": 10, "nov": 11, "dez": 12, "dec": 12,
}

func FromOCR(text string) string {
	match := timestampPattern.FindStringSubmatch(strings.ReplaceAll(text, "0CT", "OCT"))
	if len(match) == 0 {
		return ""
	}

	first, _ := strconv.Atoi(match[1])
	year, _ := strconv.Atoi(match[3])
	if year < 100 {
		year += 2000
	}

	month := 0
	if numeric, err := strconv.Atoi(match[2]); err == nil {
		month = numeric
	} else {
		month = monthNumbers[strings.ToLower(match[2])]
	}
	day := first
	if numericMonth, err := strconv.Atoi(match[2]); err == nil && numericMonth > 12 {
		month = first
		day = numericMonth
	}
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return ""
	}

	timestamp := fmt.Sprintf("%04d-%02d-%02d", year, month, day)
	if match[4] != "" {
		timestamp += fmt.Sprintf(" %02s:%02s", match[4], match[5])
		if match[6] != "" {
			timestamp += ":" + match[6]
		}
	}
	return timestamp
}
