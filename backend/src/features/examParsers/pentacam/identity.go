package pentacam

import (
	"context"
	"errors"
	"image"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"refratia/backend/src/shared/document"
	"refratia/backend/src/shared/examdate"
	"refratia/backend/src/shared/ocr"
	pdfutil "refratia/backend/src/shared/pdf"
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
func ExtractIdentity(executionContext context.Context, data []byte) (Identity, error) {
	pageCount, _, inspectError := pdfutil.Inspect(executionContext, data)
	if inspectError != nil {
		return Identity{}, inspectError
	}
	for page := 1; page <= pageCount && page <= 3; page++ {
		rendered, renderError := pdfutil.RenderPageAtDPI(executionContext, data, page, 300)
		if renderError != nil {
			return Identity{}, renderError
		}
		pageImage, decodeError := ocr.DecodeImage(rendered)
		if decodeError != nil {
			continue
		}
		bounds := pageImage.Bounds()
		// The panel sits upper-left on most screens and upper-right on the
		// general one, so the whole width of the header zone is searched.
		panel := image.Rect(bounds.Min.X, bounds.Min.Y+bounds.Dy()*20/100, bounds.Max.X, bounds.Min.Y+bounds.Dy()*50/100)
		words, ocrError := document.OCRRegionWords(executionContext, pageImage, panel, 1, 11)
		if ocrError != nil {
			continue
		}
		identity, parseError := parsePentacamIdentityWords(words)
		if parseError != nil {
			continue
		}
		// Re-read the name boxes from pixels: the box border is read as a
		// letter ("J") by the page-level pass.
		identity = refineFromPixels(executionContext, pageImage, panel, words, identity)
		return identity, nil
	}
	return Identity{}, errors.New("Pentacam: nome/nascimento não localizados")
}

// bandValue returns the words to the right of label on its own row band, up to
// (excluding) the next label on that row, in reading order.
func bandValue(words []ocr.Word, label labelSpan) []ocr.Word {
	return wordsInBand(words, label.right-max(label.height, 12)/4, label.cy, max(label.height, 12))
}

// wordsInBand returns the contiguous run of words whose vertical centre lies
// within 0.75h of cy, starting at x >= fromX. Text further right across a wide
// gap belongs to another element of the screen (the Scheimpflug segment list,
// a colour scale), and an input box is not wider than ~22 label heights.
func wordsInBand(words []ocr.Word, fromX, cy, h int) []ocr.Word {
	var in []ocr.Word
	for _, w := range words {
		wh := w.Height
		if wh == 0 {
			wh = h
		}
		if w.Left >= fromX && abs(w.Top+wh/2-cy) <= h*3/4 {
			in = append(in, w)
		}
	}
	sort.Slice(in, func(i, j int) bool { return in[i].Left < in[j].Left })
	end := len(in)
	prevRight := fromX
	for i, w := range in {
		if rowStopLabels.MatchString(normLabelWord(w.Text)) ||
			w.Left-prevRight > 6*h || w.Left > fromX+22*h {
			end = i
			break
		}
		prevRight = max(prevRight, w.Left+w.Width)
	}
	return in[:end]
}

var looseDate = regexp.MustCompile(`\d{1,2}/\d{1,2}/\d{2,4}`)

// identityRows is where the panel's rows are, found from the dates' own
// geometry: the birth and exam date boxes are stacked in one column, one row
// pitch apart, and the name boxes sit above them at the same pitch. The label
// text is only a cross-check, because the OCR frequently drops it.
type identityRows struct {
	birth, exam       ocr.Word
	hasBirth, hasExam bool
	pitch, h          int
}

func locateIdentityRows(words []ocr.Word, spans []labelSpan) identityRows {
	var dates []ocr.Word
	for _, w := range words {
		if looseDate.MatchString(w.Text) {
			dates = append(dates, w)
		}
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Top < dates[j].Top })
	var rows identityRows
	for i := range dates {
		for j := i + 1; j < len(dates); j++ {
			a, b := dates[i], dates[j]
			h := max(a.Height, 12)
			pitch := (b.Top + b.Height/2) - (a.Top + a.Height/2)
			if abs(a.Left-b.Left) <= 2*h && pitch >= 3*h/2 && pitch <= 4*h {
				return identityRows{birth: a, exam: b, hasBirth: true, hasExam: true, pitch: pitch, h: h}
			}
		}
	}
	// One date only: its own label decides what it is; without a readable
	// label it is not guessed.
	for _, d := range dates {
		h := max(d.Height, 12)
		cy := d.Top + d.Height/2
		for _, s := range spans {
			if s.right <= d.Left+h && abs(s.cy-cy) <= h*3/4 {
				switch {
				case birthLabel.MatchString(s.text):
					rows.birth, rows.hasBirth, rows.h = d, true, h
				case regexp.MustCompile(`exame`).MatchString(s.text):
					rows.exam, rows.hasExam, rows.h = d, true, h
				}
			}
		}
	}
	return rows
}

func parsePentacamIdentityWords(words []ocr.Word) (Identity, error) {
	for i := range words {
		if words[i].Height == 0 {
			words[i].Height = 20
		}
	}
	spans := labelSpans(append([]ocr.Word(nil), words...), 12)
	rows := locateIdentityRows(words, spans)

	// Names are read from the rows above the dates when the date geometry is
	// known (labels are often unreadable); otherwise from their own labels.
	join := func(ws []ocr.Word) string {
		parts := make([]string, 0, len(ws))
		for _, w := range ws {
			// Names contain no digits: a word with a digit or a degree sign is
			// screen text that leaked into the band.
			if strings.IndexFunc(w.Text, func(r rune) bool { return unicode.IsDigit(r) || r == '°' }) >= 0 {
				continue
			}
			parts = append(parts, w.Text)
		}
		return cleanIdentityValue(strings.Join(parts, " "))
	}
	var surname, given string
	if rows.hasBirth && rows.hasExam {
		cy := rows.birth.Top + rows.birth.Height/2
		from := rows.birth.Left - rows.h/2
		surname = join(wordsInBand(words, from, cy-3*rows.pitch, rows.h))
		given = join(wordsInBand(words, from, cy-2*rows.pitch, rows.h))
	} else {
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
		surnameSpan, hasSurname := find(surnameLabel, nil)
		if hasSurname {
			surname = join(bandValue(words, surnameSpan))
		}
		if givenSpan, ok := find(givenLabel, ternary(hasSurname, &surnameSpan, nil)); ok {
			given = join(bandValue(words, givenSpan))
		}
	}

	dateText := func(w ocr.Word) string { return looseDate.FindString(w.Text) }
	var birth, exam string
	if rows.hasBirth {
		birth = dateText(rows.birth)
	}
	if rows.hasExam {
		exam = dateText(rows.exam)
	}

	fullName := strings.TrimSpace(given + " " + surname)
	if fullName == "" || birth == "" {
		return Identity{}, errors.New("Pentacam: nome/nascimento não localizados")
	}

	identity := Identity{FullName: fullName, BirthDateRaw: birth, ExamDateRaw: exam, DateOrder: DateOrder}
	// A birth date equal to the exam date, in the future, or giving an age
	// outside 18..110 is a misread: it is reported as conflicting, never
	// persisted and never silently dropped.
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

// refineFromPixels re-reads the name and date boxes from pixels, located by
// the date geometry. The page-level text carries the box border as a letter
// ("J"), merges words and truncates dates. Each box is cut at its border, read
// at several scales and the text most reads agree on wins; with no majority the
// page-level reading stays.
func refineFromPixels(ctx context.Context, img image.Image, panel image.Rectangle, words []ocr.Word, identity Identity) Identity {
	spans := labelSpans(append([]ocr.Word(nil), words...), 12)
	rows := locateIdentityRows(words, spans)
	if !rows.hasBirth || !rows.hasExam {
		return identity
	}
	h := rows.h
	box := func(w ocr.Word, cy, width int) image.Rectangle {
		return image.Rect(
			panel.Min.X+w.Left-h/2, panel.Min.Y+cy-h,
			panel.Min.X+w.Left-h/2+width, panel.Min.Y+cy+h,
		).Intersect(panel)
	}
	cy := rows.birth.Top + rows.birth.Height/2
	fullDate := regexp.MustCompile(`^\d{1,2}/\d{1,2}/\d{4}$`)
	digits := func(t string) string {
		return strings.Join(strings.Fields(regexp.MustCompile(`[^0-9/]`).ReplaceAllString(t, " ")), "")
	}

	refined := false
	if d := readTextBox(ctx, img, box(rows.birth, cy, 12*h), "0123456789/", digits, ""); fullDate.MatchString(d) && examdate.FromSlashOrder(d, DateOrder) != "" {
		identity.BirthDateRaw, refined = d, true
	}
	examCY := rows.exam.Top + rows.exam.Height/2
	if d := readTextBox(ctx, img, box(rows.exam, examCY, 12*h), "0123456789/", digits, ""); fullDate.MatchString(d) && examdate.FromSlashOrder(d, DateOrder) != "" {
		identity.ExamDateRaw, refined = d, true
	}
	letters := func(t string) string {
		return strings.ToUpper(strings.Join(strings.Fields(nonNameRune.ReplaceAllString(foldAccents(t), " ")), " "))
	}
	tsvSurname, tsvGiven := rowNames(words, rows)
	surname := readTextBox(ctx, img, box(rows.birth, cy-3*rows.pitch, 22*h), "", letters, tsvSurname)
	given := readTextBox(ctx, img, box(rows.birth, cy-2*rows.pitch, 22*h), "", letters, tsvGiven)
	// Each box is refined on its own; a box without a clear majority keeps the
	// page-level text of that row.
	if surname != "" || given != "" {
		if surname == "" {
			surname = tsvSurname
		}
		if given == "" {
			given = tsvGiven
		}
		identity.FullName = strings.TrimSpace(given + " " + surname)
	}

	// Re-run the plausibility check on the refined dates.
	if refined {
		identity.BirthDateIssue = ""
	}
	if refined && identity.BirthDateRaw != "" && identity.ExamDateRaw != "" {
		b, e := examdate.FromSlashOrder(identity.BirthDateRaw, DateOrder), examdate.FromSlashOrder(identity.ExamDateRaw, DateOrder)
		if e != "" && !examdate.PlausibleBirth(b, e) {
			identity.BirthDateRaw, identity.BirthDateIssue = "", "conflicting"
		}
	}
	return identity
}

var nonNameRune = regexp.MustCompile(`[^\p{L}' -]`)

func readTextBox(ctx context.Context, img image.Image, rect image.Rectangle, whitelist string, normalize func(string) string, hint string) string {
	if rect.Empty() {
		return ""
	}
	cell := frameWithMargin(stripBoxBorder(darkOnLight(ocr.PrepareGray(img, rect, 1, 0))), 12)
	cellPNG, err := ocr.EncodePNG(cell)
	if err != nil {
		return ""
	}
	votes := map[string]int{}
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
				text, err := ocr.RunTesseractText(ctx, encoded, psm, whitelist)
				if err != nil {
					continue
				}
				text = normalize(text)
				if text != "" {
					votes[text]++
				}
			}
		}
	}
	// Readings differ only in where the OCR puts the spaces; they vote as one
	// text (spaces removed) and the commonest spelling, preferring more spaces,
	// is reported.
	groups := map[string]map[string]int{}
	groupVotes := map[string]int{}
	for text, n := range votes {
		key := strings.ReplaceAll(text, " ", "")
		if groups[key] == nil {
			groups[key] = map[string]int{}
		}
		groups[key][text] += n
		groupVotes[key] += n
	}
	bestKey, bestVotes, tie := "", 0, false
	for key, n := range groupVotes {
		switch {
		case n > bestVotes:
			bestKey, bestVotes, tie = key, n, false
		case n == bestVotes:
			tie = true
		}
	}
	if bestVotes < 2 {
		return ""
	}
	if tie {
		// Equal votes: the page-level text of the same row is one more,
		// independent reading; the candidate sharing the most words with it
		// wins, otherwise there is no value.
		hintWords := map[string]bool{}
		for _, w := range strings.Fields(normalize(hint)) {
			hintWords[w] = true
		}
		bestKey, tie = "", false
		top := -1
		for key, n := range groupVotes {
			if n != bestVotes {
				continue
			}
			shared := 0
			for text := range groups[key] {
				for _, w := range strings.Fields(text) {
					if hintWords[w] {
						shared++
					}
				}
			}
			switch {
			case shared > top:
				bestKey, top, tie = key, shared, false
			case shared == top:
				tie = true
			}
		}
		if tie || top <= 0 {
			return ""
		}
	}
	// Prefer the spelling with the most spaces among those read at least twice
	// (the OCR often drops a space, never invents one), else the commonest.
	best, bestSpelling := "", 0
	for text, n := range groups[bestKey] {
		if n < 2 {
			continue
		}
		if best == "" || strings.Count(text, " ") > strings.Count(best, " ") ||
			(strings.Count(text, " ") == strings.Count(best, " ") && n > bestSpelling) {
			best, bestSpelling = text, n
		}
	}
	if best == "" {
		for text, n := range groups[bestKey] {
			if n > bestSpelling {
				best, bestSpelling = text, n
			}
		}
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
			if g.GrayAt(x, y).Y < 170 {
				dark++
			}
		}
		if dark*100 >= rows*80 {
			cut = x
		}
	}
	if cut < 0 {
		return g
	}
	return g.SubImage(image.Rect(cut+2, b.Min.Y, b.Max.X, b.Max.Y)).(*image.Gray)
}

var accentFolder = strings.NewReplacer(
	"Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A", "É", "E", "È", "E", "Ê", "E", "Ë", "E",
	"Í", "I", "Ì", "I", "Î", "I", "Ï", "I", "Ó", "O", "Ò", "O", "Ô", "O", "Õ", "O", "Ö", "O",
	"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U", "Ç", "C", "Ñ", "N",
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i", "ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u", "ç", "c", "ñ", "n",
)

// foldAccents makes the readings of one name comparable: the OCR flips between
// "Ã" and "A" from read to read.
func foldAccents(s string) string { return accentFolder.Replace(s) }

// rowNames is the page-level text of the surname and given-name rows.
func rowNames(words []ocr.Word, rows identityRows) (surname, given string) {
	cy := rows.birth.Top + rows.birth.Height/2
	from := rows.birth.Left - rows.h/2
	text := func(ws []ocr.Word) string {
		parts := []string{}
		for _, w := range ws {
			if strings.IndexFunc(w.Text, func(r rune) bool { return unicode.IsDigit(r) || r == '°' }) >= 0 {
				continue
			}
			parts = append(parts, w.Text)
		}
		return cleanIdentityValue(strings.Join(parts, " "))
	}
	return text(wordsInBand(words, from, cy-3*rows.pitch, rows.h)), text(wordsInBand(words, from, cy-2*rows.pitch, rows.h))
}
