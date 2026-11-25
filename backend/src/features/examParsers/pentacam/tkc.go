package pentacam

import (
	"regexp"
	"strings"
)

var (
	tkcLabelLine = regexp.MustCompile(`(?i)\btkc\b([^\r\n]*)`)
	tkcDashOnly  = regexp.MustCompile(`^[-—–]+$`)
	tkcClass     = regexp.MustCompile(`^K[CK]\s*([1-4])(?:\s*[-—–]\s*([1-4]))?$`)
	tkcPossible  = regexp.MustCompile(`^POSS(?:\.|[IÍ]VEL|IBLE)?$`)
)

// parseTKCCell reads the text of ONE TKC value cell. Protocol rule 2.1:
// classified = Poss./KC1..KC4 (and adjacent intermediates), dash = not
// classified, anything else = no reading. A bare number is never a class.
func parseTKCCell(cell string) (string, bool) {
	c := strings.ToUpper(strings.Trim(cell, "[]|(){}!:; \t"))
	if c == "" {
		return "", false
	}
	if tkcDashOnly.MatchString(c) {
		return "—", true
	}
	if m := tkcClass.FindStringSubmatch(c); m != nil {
		if m[2] == "" {
			return "KC" + m[1], true
		}
		if m[2][0] == m[1][0]+1 {
			return "KC" + m[1] + "-" + m[2], true
		}
		return "", false
	}
	if tkcPossible.MatchString(c) {
		return "Poss.", true
	}
	return "", false
}

// combineTKC merges the focused reader (value + status) with the anchored cell
// reading ("" when it read nothing). Any disagreement leaves TKC unknown
// ("conflicting"); a single strict class with no rival is accepted. A dash (the
// "not classified" state, the unsafe direction) needs the focused reader: the
// anchored cell alone never establishes it.
func combineTKC(focused, status, anchored string) (value, outStatus, strategy string) {
	strategy = "focused_tkc"
	if anchored == "" || status == "conflicting" {
		return focused, status, strategy
	}
	switch {
	case focused == anchored:
		return focused, status, "focused_tkc+label_anchor"
	case focused != "":
		return "", "conflicting", "focused_tkc+label_anchor"
	case anchored != "—":
		return anchored, "valid", "label_anchor"
	}
	return "", status, strategy
}

// tkcFromText looks only at the rest of the TKC label's own line, so the
// neighbouring cell on the next line can never leak in.
func tkcFromText(text string) (reading string, labelled bool) {
	m := tkcLabelLine.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	v, ok := parseTKCCell(m[1])
	if !ok {
		return "", true
	}
	return v, true
}
