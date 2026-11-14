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

	"refratia/backend/src/shared/document"
	"refratia/backend/src/shared/examdate"
	"refratia/backend/src/shared/ocr"
	pdfutil "refratia/backend/src/shared/pdf"
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
func ExtractImage(executionContext context.Context, data []byte) (map[string]any, error) {
	sourceImage, decodeError := ocr.DecodeImage(data)
	if decodeError != nil {
		return nil, fmt.Errorf("refratometria: imagem inválida")
	}

	words, ocrError := document.OCRWords(executionContext, data, 4)
	if ocrError != nil {
		return nil, ocrError
	}

	eyes := map[string]any{}
	examTimestamp := ""
	pupillaryDistance, hasPD := 0.0, false
	isAutoRefractor := false
	// Row tolerance scales with the image (about 0.6% of its height).
	tolerance := max(8, sourceImage.Bounds().Dy()*6/1000)
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
		if pdMatch := pupillaryDistancePattern.FindStringSubmatch(text); pdMatch != nil && !hasPD {
			if distance, parseError := strconv.ParseFloat(pdMatch[1], 64); parseError == nil && distance >= 40 && distance <= 80 {
				pupillaryDistance, hasPD = distance, true
			}
		}
		if autoRefractorPattern.MatchString(text) {
			isAutoRefractor = true
		}
		if eyeMatch := eyeMarkerPattern.FindStringSubmatch(text); len(eyeMatch) == 2 {
			eye := "OS"
			if eyeMatch[1] == "R" || eyeMatch[1] == "r" {
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
		activeEye, markerTop := "", 0
		for _, eyeMarker := range markers {
			if eyeMarker.top <= anchor.Top {
				activeEye, markerTop = eyeMarker.eye, eyeMarker.top
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
		prior := readingsPrior(executionContext, sourceImage, words, anchor, line, markerTop)
		values, ok := parseAverageRowCrop(executionContext, sourceImage, anchor, line, prior)
		if !ok && !(prior.usable[0] || prior.usable[1] || prior.usable[2]) {
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
	for _, eye := range eyes {
		payload := eye.(map[string]any)
		if hasPD {
			payload["pd_mm"] = pupillaryDistance // one binocular value printed once on the ticket
		}
		if isAutoRefractor {
			payload["refraction_type"] = "autorrefração"
		}
	}

	result := map[string]any{"eyes": eyes}
	if examTimestamp != "" {
		result["exam_datetime"] = examTimestamp
	}
	return result, nil
}

var (
	pupillaryDistancePattern = regexp.MustCompile(`(?i)\bPD\s*[=:]?\s*(\d{2})\s*mm`)
	// "[REF]" is the autorefractor's measurement-mode tag on the ticket.
	autoRefractorPattern = regexp.MustCompile(`(?i)\[\s*REF\s*\]`)
)

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

// strictReading is a printed measurement row (no AVE label).
var strictReading = regexp.MustCompile(`([+-]?\d{1,2}\.\d{2})\s*([+-]?\d{1,2}\.\d{2})\s*(\d{1,3})`)

func rowRect(anchor ocr.Word, line ocr.Row) image.Rectangle {
	top, bottom := anchor.Top, anchor.Top+anchor.Height
	right := anchor.Left + anchor.Width
	for _, w := range line.Words {
		top = min(top, w.Top)
		bottom = max(bottom, w.Top+w.Height)
		right = max(right, w.Left+w.Width)
	}
	pad := (bottom - top) / 4
	return image.Rect(anchor.Left-pad, top-pad, right+pad, bottom+pad)
}

// The printer font slashes its zeros and the OCR reads them as 8 (or 6):
// "+5.08" for +5.00, "-1.58" for -1.50, "188" for 100. A token is therefore
// expanded into the values reachable by turning some 8/6 glyphs into 0, and
// only candidates that are valid on the printed grid (0.25 D steps, axis
// 0..180) are kept.
func zeroConfusions(token string) []candidate {
	positions := []int{}
	for i, r := range token {
		if r == '8' || r == '6' {
			positions = append(positions, i)
		}
	}
	var out []candidate
	for mask := 0; mask < 1<<len(positions) && len(positions) <= 8; mask++ {
		b := []byte(token)
		subs := 0
		for k, pos := range positions {
			if mask&(1<<k) != 0 {
				b[pos] = '0'
				subs++
			}
		}
		if subs > 3 {
			continue
		}
		if f, err := strconv.ParseFloat(string(b), 64); err == nil {
			out = append(out, candidate{value: f, subs: subs})
		}
	}
	return out
}

type candidate struct {
	value float64
	subs  int
}

func validOnGrid(column int, v float64) bool {
	if column == 2 {
		return v >= 0 && v <= 180 && v == math.Trunc(v)
	}
	return onQuarterGrid(v)
}

// rowVotes keeps, per column, the strong votes (a unique closest-to-printed
// candidate) and the weak ones (every grid-valid candidate of an ambiguous
// token, only usable together with an independent prior).
type rowVotes struct {
	strong, weak [3]map[float64]int
	reads        int
}

// voteRow re-OCRs one printed row under several preprocessings.
func voteRow(ctx context.Context, img image.Image, rect image.Rectangle, format *regexp.Regexp) rowVotes {
	var rv rowVotes
	for i := range rv.strong {
		rv.strong[i], rv.weak[i] = map[float64]int{}, map[float64]int{}
	}
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
		m := format.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		var perColumn [3][]candidate
		usable := true
		for i := 0; i < 3; i++ {
			for _, c := range zeroConfusions(m[i+1]) {
				if validOnGrid(i, c.value) {
					perColumn[i] = append(perColumn[i], c)
				}
			}
			if len(perColumn[i]) == 0 {
				usable = false
			}
		}
		if !usable {
			continue
		}
		rv.reads++
		for i := 0; i < 3; i++ {
			minSubs := 99
			for _, c := range perColumn[i] {
				minSubs = min(minSubs, c.subs)
			}
			var closest []candidate
			for _, c := range perColumn[i] {
				rv.weak[i][c.value]++
				if c.subs == minSubs {
					closest = append(closest, c)
				}
			}
			if len(closest) == 1 {
				rv.strong[i][closest[0].value]++
			}
		}
	}
	return rv
}

// majority returns the value a strict majority of the reads agree on.
func majority(votes map[float64]int, reads int) (float64, bool) {
	best, bestN := 0.0, 0
	for value, n := range votes {
		if n > bestN {
			best, bestN = value, n
		}
	}
	return best, bestN*2 > reads
}

// readingsPrior reads the individual measurements printed above the AVE row of
// the same eye. The device prints AVE as their average, so the three readings
// are an independent estimate of what AVE must be near (the slashed-zero scan
// makes 5/0 reads disagree; the readings narrow the choice).
// rowPrior is what the printed readings say AVE must be near. A column is
// usable to rescue a row without a clear majority when two readings agree on it,
// and may veto a clear majority only when every one of at least three agrees.
type rowPrior struct {
	value        [3]float64
	usable, veto [3]bool
}

func readingsPrior(ctx context.Context, img image.Image, words []ocr.Word, anchor ocr.Word, line ocr.Row, markerTop int) rowPrior {
	var p rowPrior
	prior, valid := &p.value, &p.usable
	h := anchor.Height
	left, right := anchor.Left, anchor.Left+anchor.Width
	for _, w := range line.Words {
		right = max(right, w.Left+w.Width)
	}
	// Cluster the numeric words between the eye marker and the AVE row into
	// rows by vertical centre.
	type cluster struct {
		cy    int
		words []ocr.Word
	}
	var clusters []cluster
	for _, w := range words {
		cy := w.Top + w.Height/2
		if cy >= anchor.Top-h/2 || cy <= markerTop+h || w.Left+w.Width < left || w.Left > right ||
			!strings.ContainsAny(w.Text, "0123456789") || !strings.ContainsAny(w.Text, ".,") {
			continue
		}
		placed := false
		for i := range clusters {
			if abs(clusters[i].cy-cy) <= h*7/10 {
				clusters[i].words = append(clusters[i].words, w)
				placed = true
				break
			}
		}
		if !placed {
			clusters = append(clusters, cluster{cy: cy, words: []ocr.Word{w}})
		}
	}
	sort.Slice(clusters, func(i, j int) bool { return clusters[i].cy > clusters[j].cy }) // nearest to AVE first
	var rows [][3]float64
	for _, c := range clusters {
		if len(c.words) < 2 || len(rows) == 3 {
			continue
		}
		r := ocr.Row{Words: c.words}
		first := c.words[0]
		for _, w := range c.words {
			if w.Left < first.Left {
				first = w
			}
		}
		rect := rowRect(first, r)
		rect.Max.X = max(rect.Max.X, right+h/4) // the axis column has no decimal point and is not among the clustered words
		rv := voteRow(ctx, img, rect, strictReading)
		if rv.reads < 2 {
			continue
		}
		var row [3]float64
		ok := true
		for i := range rv.strong {
			v, good := majority(rv.strong[i], rv.reads)
			if !good {
				ok = false
				break
			}
			row[i] = v
		}
		if ok {
			rows = append(rows, row)
		}
	}
	if len(rows) < 2 {
		return p
	}
	// Median (medoid for the axis), not mean: one misread row must not drag the
	// prior away from the two that agree.
	for i := 0; i < 2; i++ {
		vals := make([]float64, 0, len(rows))
		for _, r := range rows {
			vals = append(vals, r[i])
		}
		sort.Float64s(vals)
		prior[i] = vals[len(vals)/2]
	}
	prior[2] = medoidAxis(rows)
	// A column's prior is only used when at least two readings agree on it
	// (within the tolerance); a lone or scattered column says nothing.
	for i := range valid {
		agreeing := 0
		for _, r := range rows {
			d := math.Abs(r[i] - prior[i])
			if i == 2 {
				d = axisDistance(r[i], prior[2])
			}
			if d <= priorTolerance[i] {
				agreeing++
			}
		}
		valid[i] = agreeing >= 2
		p.veto[i] = agreeing == len(rows) && len(rows) >= 3
	}
	return p
}

// medoidAxis returns the row axis with the smallest total circular distance
// to the others (axes wrap at 180).
func medoidAxis(rows [][3]float64) float64 {
	best, bestSum := rows[0][2], math.MaxFloat64
	for _, a := range rows {
		sum := 0.0
		for _, b := range rows {
			sum += axisDistance(a[2], b[2])
		}
		if sum < bestSum {
			best, bestSum = a[2], sum
		}
	}
	return best
}

func normalizeAxis(v float64) float64 {
	for v < 0 {
		v += 180
	}
	for v >= 180 {
		v -= 180
	}
	return v
}

func axisDistance(a, b float64) float64 {
	d := math.Abs(normalizeAxis(a) - normalizeAxis(b))
	return math.Min(d, 180-d)
}

// priorTolerance is how far AVE may sit from the mean of the printed readings
// (the device rounds and averages in vector space): 0.25 D, 5 degrees.
var priorTolerance = [3]float64{0.25, 0.25, 5}

// parseAverageRowCrop re-OCRs the AVE row and returns, per column, the reading
// a clear majority of the usable reads agree on. When the printed readings
// above it are known, a column's value must also lie within tolerance of their
// mean: a majority outside it is rejected, and with no clear majority the one
// reading within tolerance that was read at least twice and unambiguously
// wins. Otherwise the row is unreadable: a missing value beats a guess.
func parseAverageRowCrop(ctx context.Context, img image.Image, anchor ocr.Word, line ocr.Row, prior rowPrior) ([3]float64, bool) {
	var result [3]float64
	rv := voteRow(ctx, img, rowRect(anchor, line), strictAverage)
	if rv.reads < 2 {
		return result, false
	}
	for i := range rv.strong {
		near := func(v float64) bool {
			if i == 2 {
				return axisDistance(v, prior.value[2]) <= priorTolerance[2]
			}
			return math.Abs(v-prior.value[i]) <= priorTolerance[i]
		}
		best, ok := majority(rv.strong[i], rv.reads)
		if ok && (!prior.veto[i] || near(best)) {
			result[i] = best
			continue
		}
		if !prior.usable[i] {
			return result, false
		}
		// The in-tolerance value nearest to the prior among those at least two
		// reads support (strongly or as one candidate of an ambiguous token);
		// a tie for nearest is no answer.
		pick, found, bestDist, tie := 0.0, false, math.MaxFloat64, false
		for v, n := range rv.weak[i] {
			if n < 2 || !near(v) {
				continue
			}
			d := math.Abs(v - prior.value[i])
			if i == 2 {
				d = axisDistance(v, prior.value[2])
			}
			switch {
			case d < bestDist-1e-9:
				pick, found, bestDist, tie = v, true, d, false
			case math.Abs(d-bestDist) <= 1e-9:
				tie = true
			}
		}
		if !found || tie {
			return result, false
		}
		result[i] = pick
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
