package pentacam

import (
	"context"
	"errors"
	"image"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"refratia/backend/shared/examdate"
	"refratia/backend/shared/ocr"
	pdfutil "refratia/backend/shared/pdf"
)

// DateOrder is the day/month order the Pentacam prints (MM/DD/YYYY). It is
// device knowledge, so the consumer never has to guess it from the string.
const DateOrder = "MDY"

type Identity struct {
	FullName     string
	BirthDateRaw string
	ExamDateRaw  string
	DateOrder    string
	// BirthDateIssue is "" or "conflicting" when the birth date read from the
	// label failed the plausibility checks (not before the exam, age 18..110).
	// BirthDateRaw is then empty: a missing value, never a guessed one.
	BirthDateIssue string
}

var pentacamDatePattern = regexp.MustCompile(`\b\d{1,2}/\d{1,2}/\d{4}\b`)

// Labels of the identity panel. Every value is bound to the row band of its
// own label and stops at the next label on that row.
var (
	surnameLabel  = lbl(`sobrenome`)
	givenLabel    = lbl(`nome`)
	birthLabel    = lbl(`nasc`)
	examDateLabel = lbl(`data de exame`)
	rowStopLabels = regexp.MustCompile(`^(?:olho|hora|dt|dta)$`)
)

// ExtractIdentity reads the patient panel (upper left of the content area,
// printed on every screen) of the first page that yields a usable reading.
func ExtractIdentity(ctx context.Context, data []byte) (Identity, error) {
	pageCount, _, err := pdfutil.Inspect(ctx, data)
	if err != nil {
		return Identity{}, err
	}
	for page := 1; page <= pageCount && page <= 3; page++ {
		rendered, err := pdfutil.RenderPageAtDPI(ctx, data, page, 300)
		if err != nil {
			return Identity{}, err
		}
		img, err := ocr.DecodeImage(rendered)
		if err != nil {
			continue
		}
		b := img.Bounds()
		// The panel sits upper-left on most screens and upper-right on the
		// general one, so the whole width of the header zone is searched.
		panel := image.Rect(b.Min.X, b.Min.Y+b.Dy()*20/100, b.Max.X, b.Min.Y+b.Dy()*50/100)
		encoded, err := ocr.EncodePNG(ocr.PrepareGray(img, panel, 1, 0))
		if err != nil {
			continue
		}
		tsv, err := ocr.RunTesseractTSVWithPSM(ctx, encoded, 11)
		if err != nil {
			continue
		}
		words, _, _ := ocr.ParseTSVWords(tsv)
		identity, err := parsePentacamIdentityWords(words)
		if err != nil {
			continue
		}
		// Re-read the name boxes from pixels: the box border is read as a
		// letter ("J") by the page-level pass.
		if name := readNameBoxes(ctx, img, panel, words); name != "" {
			identity.FullName = name
		}
		return identity, nil
	}
	return Identity{}, errors.New("Pentacam: nome/nascimento não localizados")
}

// bandValue returns the words to the right of label on its own row band, up to
// (excluding) the next label on that row, in reading order.
func bandValue(words []ocr.Word, label labelSpan) []ocr.Word {
	h := max(label.height, 12)
	var in []ocr.Word
	for _, w := range words {
		wh := w.Height
		if wh == 0 {
			wh = h
		}
		cy := w.Top + wh/2
		if w.Left >= label.right-h/4 && abs(cy-label.cy) <= h*3/4 {
			in = append(in, w)
		}
	}
	sort.Slice(in, func(i, j int) bool { return in[i].Left < in[j].Left })
	for i, w := range in {
		if rowStopLabels.MatchString(normLabelWord(w.Text)) {
			return in[:i]
		}
	}
	return in
}

func parsePentacamIdentityWords(words []ocr.Word) (Identity, error) {
	for i := range words {
		if words[i].Height == 0 {
			words[i].Height = 20
		}
	}
	spans := labelSpans(append([]ocr.Word(nil), words...), 12)
	find := func(pattern *regexp.Regexp, below *labelSpan) (labelSpan, bool) {
		var best *labelSpan
		for i := range spans {
			if !pattern.MatchString(spans[i].text) || (below != nil && spans[i].top <= below.top) {
				continue
			}
			if best == nil || spans[i].top < best.top {
				best = &spans[i]
			}
		}
		if best == nil {
			return labelSpan{}, false
		}
		return *best, true
	}
	join := func(ws []ocr.Word) string {
		parts := make([]string, 0, len(ws))
		for _, w := range ws {
			parts = append(parts, w.Text)
		}
		return cleanIdentityValue(strings.Join(parts, " "))
	}

	var surname, given string
	surnameSpan, hasSurname := find(surnameLabel, nil)
	if hasSurname {
		surname = join(bandValue(words, surnameSpan))
	}
	if givenSpan, ok := find(givenLabel, ternary(hasSurname, &surnameSpan, nil)); ok {
		given = join(bandValue(words, givenSpan))
	}

	// A date is looked for only inside its own label's band: a label that was
	// not read yields no date, it never borrows the neighbouring row's.
	dateIn := func(pattern *regexp.Regexp) string {
		span, ok := find(pattern, nil)
		if !ok {
			return ""
		}
		for _, w := range bandValue(words, span) {
			if d := pentacamDatePattern.FindString(w.Text); d != "" {
				return d
			}
		}
		return ""
	}
	birth, exam := dateIn(birthLabel), dateIn(examDateLabel)

	fullName := strings.TrimSpace(given + " " + surname)
	if fullName == "" || birth == "" {
		return Identity{}, errors.New("Pentacam: nome/nascimento não localizados")
	}

	identity := Identity{FullName: fullName, BirthDateRaw: birth, ExamDateRaw: exam, DateOrder: DateOrder}
	// A birth date equal to the exam date, in the future, or giving an age
	// outside 18..110 is a misread label: drop it instead of reporting it.
	if exam != "" {
		birthISO, examISO := examdate.FromSlashOrder(birth, DateOrder), examdate.FromSlashOrder(exam, DateOrder)
		if examISO != "" && !examdate.PlausibleBirth(birthISO, examISO) {
			identity.BirthDateRaw, identity.BirthDateIssue = "", "conflicting"
		}
	}
	return identity, nil
}

func ternary[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

// cleanIdentityValue keeps what can be part of a name or date: letters,
// digits, spaces and the usual name punctuation. Box borders and stray marks
// around the value are trimmed. No token is ever rewritten.
func cleanIdentityValue(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '\'' || r == '.' || r == '/' {
			return r
		}
		return ' '
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

// readNameBoxes re-reads the surname and given-name boxes from pixels: the
// left cell border is cut off first and the interior is read at several
// scales; the text most reads agree on wins. Returns "" without a clear
// majority (the caller then keeps the page-level text).
func readNameBoxes(ctx context.Context, img image.Image, panel image.Rectangle, words []ocr.Word) string {
	spans := labelSpans(append([]ocr.Word(nil), words...), 12)
	pick := func(pattern *regexp.Regexp, below *labelSpan) (labelSpan, bool) {
		var best *labelSpan
		for i := range spans {
			if !pattern.MatchString(spans[i].text) || (below != nil && spans[i].top <= below.top) {
				continue
			}
			if best == nil || spans[i].top < best.top {
				best = &spans[i]
			}
		}
		if best == nil {
			return labelSpan{}, false
		}
		return *best, true
	}
	surname, ok := pick(surnameLabel, nil)
	if !ok {
		return ""
	}
	given, ok := pick(givenLabel, &surname)
	if !ok {
		return ""
	}
	read := func(label labelSpan) string {
		h := label.height
		rect := image.Rect(
			panel.Min.X+label.right+h/4, panel.Min.Y+label.cy-h,
			panel.Min.X+label.right+h/4+15*h, panel.Min.Y+label.cy+h,
		).Intersect(panel)
		return readTextBox(ctx, img, rect)
	}
	surnameText, givenText := read(surname), read(given)
	if surnameText == "" && givenText == "" {
		return ""
	}
	return strings.TrimSpace(givenText + " " + surnameText)
}

var nonNameRune = regexp.MustCompile(`[^\p{L}' -]`)

func readTextBox(ctx context.Context, img image.Image, rect image.Rectangle) string {
	if rect.Empty() {
		return ""
	}
	cell := frameWithMargin(stripBoxBorder(darkOnLight(ocr.PrepareGray(img, rect, 1, 0))), 12)
	cellPNG, err := ocr.EncodePNG(cell)
	if err != nil {
		return ""
	}
	votes := map[string]int{}
	reads := 0
	for _, scale := range []int{2, 3} {
		for _, threshold := range []uint8{0, 150} {
			var encoded []byte
			if threshold == 0 {
				gray := ocr.PrepareGray(cell, cell.Bounds(), scale, 0)
				ocr.AutoContrast(gray)
				encoded, err = ocr.EncodePNG(gray)
			} else {
				encoded, err = pentacamBinaryUpscale(cellPNG, scale, threshold)
			}
			if err != nil {
				continue
			}
			for _, psm := range []int{7, 13} {
				text, err := ocr.RunTesseractText(ctx, encoded, psm, "")
				if err != nil {
					continue
				}
				text = strings.ToUpper(strings.Join(strings.Fields(nonNameRune.ReplaceAllString(text, " ")), " "))
				if text != "" {
					votes[text]++
					reads++
				}
			}
		}
	}
	best, bestVotes, tie := "", 0, false
	for text, n := range votes {
		switch {
		case n > bestVotes:
			best, bestVotes, tie = text, n, false
		case n == bestVotes:
			tie = true
		}
	}
	if tie || bestVotes < 2 || bestVotes*2 <= reads {
		return ""
	}
	return best
}

// stripBoxBorder cuts the input box's left border (a vertical line that is
// dark over almost the full cell height) and everything left of it. Letter
// stems never reach that height, so real glyphs are untouched.
func stripBoxBorder(g *image.Gray) *image.Gray {
	b := g.Bounds()
	if b.Dx() < 12 || b.Dy() < 12 {
		return g
	}
	rows := b.Dy() - 2*(b.Dy()*8/100)
	cut := -1
	for x := b.Min.X; x < b.Min.X+b.Dx()*18/100; x++ {
		dark := 0
		for y := b.Min.Y + b.Dy()*8/100; y < b.Max.Y-b.Dy()*8/100; y++ {
			if g.GrayAt(x, y).Y < 120 {
				dark++
			}
		}
		if dark*100 >= rows*90 {
			cut = x
		}
	}
	if cut < 0 {
		return g
	}
	return g.SubImage(image.Rect(cut+2, b.Min.Y, b.Max.X, b.Max.Y)).(*image.Gray)
}
