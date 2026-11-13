package pentacam

import (
	"regexp"
	"strconv"
	"strings"

	"refratia/backend/features/extractionfields"
)

var (
	// Everything that is not part of a number: box borders ([ ] | { } _),
	// units (D, mm, um, °) and stray letters.
	nonNumber    = regexp.MustCompile(`[^0-9.,+\-−–]`)
	numberTokens = regexp.MustCompile(`[+\-]?\d+(?:\.\d+)?`)
)

// normalizePrinted turns raw OCR text of one printed cell into a number using
// the field's printed format (extractionfields.Rule):
//
//   - the sign is preserved, never invented or dropped;
//   - a token with a decimal separator must carry exactly the printed number of
//     decimals, otherwise digits were lost and the cell is unreadable;
//   - a token without a separator (the OCR often drops the tiny dot) gets the
//     separator re-inserted at the printed decimals, but only if enough digits
//     remain and the result is inside the field's plausible range;
//   - integer fields must not carry a separator at all.
func normalizePrinted(raw string, rule extractionfields.Rule) (float64, bool) {
	text := strings.NewReplacer("−", "-", "–", "-", ",", ".").Replace(raw)
	text = nonNumber.ReplaceAllString(text, " ")
	tokens := numberTokens.FindAllString(text, -1)
	if len(tokens) != 1 {
		return 0, false // none, or several numbers in one cell
	}
	token := tokens[0]

	sign := ""
	if token[0] == '+' || token[0] == '-' {
		sign, token = token[:1], token[1:]
	}

	if dot := strings.IndexByte(token, '.'); dot >= 0 {
		if rule.Decimals < 0 || len(token)-dot-1 != rule.Decimals || rule.Decimals == 0 {
			return 0, false
		}
	} else if rule.Decimals > 0 {
		if len(token) <= rule.Decimals {
			return 0, false
		}
		cut := len(token) - rule.Decimals
		token = token[:cut] + "." + token[cut:]
	}

	value, err := strconv.ParseFloat(sign+token, 64)
	if err != nil {
		return 0, false
	}
	if rule.HasRange && (value < rule.Min || value > rule.Max) {
		return 0, false
	}
	return value, true
}
