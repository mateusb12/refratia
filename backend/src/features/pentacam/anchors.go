package pentacam

import (
	"context"
	"fmt"
	"image"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"

	"refratia/backend/src/features/extractionfields"
	"refratia/backend/src/shared/document"
	"refratia/backend/src/shared/ocr"
	"refratia/backend/src/shared/parallelwork"
)

const examKey = "pentacam_corneal_tomography"

// anchorField locates one printed value through the text of its label, not
// through pixel coordinates: the value cell is the box to the right of the
// label on the same row band, wherever the template places it.
type anchorField struct {
	Key     string         // key in the extractor's value map
	Spec    string         // field name in _field_spec.json (printed format, range)
	Screens []Screen       // screens tried in order (first that yields a value)
	Label   *regexp.Regexp // matched against the normalised label words
	// (IVA's label is often garbled by the OCR; it is the nearest short word
	// below ISV in the same column.)
	// ColumnOf, when set, restricts candidates to labels below the label of
	// that other anchor and in the same column (the "Índices" box repeats
	// labels such as KI that also exist in the cornea boxes).
	ColumnOf string
	// Bottom picks the bottom-most match instead of the topmost.
	Bottom bool
	// Rank picks the n-th match from the left on the topmost row (the Z31 label
	// repeats for the 5 mm and 6 mm columns).
	Rank int
	// ValueBelow: the value is printed on the line under its label.
	ValueBelow bool
	// Second: the value is the second numeric word of the row (the axis printed
	// after the magnitude, "0.081 µm (199°)").
	Second bool
	// Wide: the value box is far from its label (a gap of ~12 label heights).
	Wide bool
}

func lbl(pattern string) *regexp.Regexp { return regexp.MustCompile(`^(?:` + pattern + `)$`) }

var anchorFields = []anchorField{
	// Córnea Ant. box (identical on the 4-maps and topometric screens).
	{Key: "k1_d", Spec: "k1_d", Screens: []Screen{ScreenTopometric, ScreenRefractive}, Label: lbl(`k[1il]`)},
	{Key: "k2_d", Spec: "k2_d", Screens: []Screen{ScreenTopometric, ScreenRefractive}, Label: lbl(`k2`)},
	{Key: "km_d", Spec: "km_d", Screens: []Screen{ScreenTopometric, ScreenRefractive}, Label: lbl(`km`)},
	{Key: "astigmatism_d", Spec: "astig_ant_d", Screens: []Screen{ScreenTopometric, ScreenRefractive}, Label: lbl(`astig`)},
	{Key: "axis_deg", Spec: "axis_flat_deg", Screens: []Screen{ScreenTopometric, ScreenRefractive}, Label: lbl(`eixo`)},

	// Topometric screen.
	{Key: "kmax_d", Spec: "kmax_ant_d", Screens: []Screen{ScreenTopometric}, Label: lbl(`k m[aá]x ant`)},
	{Key: "thinnest_um", Spec: "pachy_thinnest_um", Screens: []Screen{ScreenTopometric}, Label: lbl(`ponto fino`)},
	{Key: "pupil_center_um", Spec: "pachy_pupil_center_um", Screens: []Screen{ScreenTopometric}, Label: lbl(`centro pupilar`)},
	{Key: "vertex_normal_um", Spec: "pachy_vertex_um", Screens: []Screen{ScreenTopometric, ScreenCataract}, Label: lbl(`pachy vertex n`)},
	{Key: "acd_external_mm", Spec: "acd_ext_mm", Screens: []Screen{ScreenTopometric, ScreenCataract}, Label: lbl(`prof c\S+ ant ext`)},
	{Key: "isv", Spec: "isv", Screens: []Screen{ScreenTopometric}, Label: lbl(`[i1l][s5][vy]?`)},
	{Key: "iha", Spec: "iha", Screens: []Screen{ScreenTopometric}, Label: lbl(`iha`)},
	{Key: "ihd", Spec: "ihd", Screens: []Screen{ScreenTopometric}, Label: lbl(`ihd`)},
	{Key: "iva", Spec: "iva", Screens: []Screen{ScreenTopometric}, Label: lbl(`iva|[a-z0-9]{2,4}`), ColumnOf: "isv"},
	{Key: "ki", Spec: "ki", Screens: []Screen{ScreenTopometric}, Label: lbl(`k[i1l]`), ColumnOf: "isv"},
	{Key: "cki", Spec: "cki", Screens: []Screen{ScreenTopometric}, Label: lbl(`cki`)},

	// Belin/Ambrósio screen: "D:" is the bottom-most single-letter label of
	// the page (the deviation-index row); ARTmax sits in the progression box.
	{Key: "bad_d", Spec: "bad_d", Screens: []Screen{ScreenBelin}, Label: lbl(`d`), Bottom: true},
	{Key: "art_max", Spec: "art_max_um", Screens: []Screen{ScreenBelin}, Label: lbl(`art\s?max`)},

	// Anéis Corneanos screen: Z31 (Coma) for the 5 mm and 6 mm zones, the value
	// printed on the line under each label.
	{Key: "z31_5mm_um", Spec: "coma_z31_5mm_um", Screens: []Screen{ScreenRings}, Label: lbl(`(?:z31 )?coma\w?`), ValueBelow: true},
	{Key: "z31_5mm_axis_deg", Spec: "coma_z31_5mm_axis_deg", Screens: []Screen{ScreenRings}, Label: lbl(`(?:z31 )?coma\w?`), ValueBelow: true, Second: true},
	{Key: "z31_6mm_axis_deg", Spec: "coma_z31_5mm_axis_deg", Screens: []Screen{ScreenRings}, Label: lbl(`(?:z31 )?coma\w?`), ValueBelow: true, Rank: 1, Second: true},
	{Key: "z31_6mm_um", Spec: "coma_z31_6mm_um", Screens: []Screen{ScreenRings}, Label: lbl(`(?:z31 )?coma\w?`), ValueBelow: true, Rank: 1},

	// Cataract Pre-OP screen.
	{Key: "pns", Spec: "pns", Screens: []Screen{ScreenCataract}, Label: lbl(`pns`), Bottom: true, Wide: true},
	{Key: "bad_d", Spec: "bad_d", Screens: []Screen{ScreenCataract}, Label: lbl(`bad d`)},
	{Key: "z40_6mm_um", Spec: "z40_6mm_um", Screens: []Screen{ScreenCataract}, Label: lbl(`z40`)},
	{Key: "acd_internal_mm", Spec: "acd_int_mm", Screens: []Screen{ScreenCataract}, Label: lbl(`prof c\S+ ant int`)},
}

// screenSpecFields lists which spec fields live on which screen, including the
// fields still read by the fixed-box path. A screen that is absent from a
// report makes its fields not_applicable (never read from another page).
var screenSpecFields = map[Screen][]string{
	ScreenRefractive: {"k1_d", "k2_d", "km_d", "astig_ant_d", "axis_flat_deg"},
	ScreenTopometric: {"kmax_ant_d", "pachy_thinnest_um", "pachy_pupil_center_um", "pachy_vertex_um",
		"acd_ext_mm", "isv", "iva", "iha", "ihd", "ki", "cki", "tkc"},
	ScreenCataract: {"z40_6mm_um", "acd_int_mm", "pns"},
	ScreenBelin:    {"bad_d", "art_max_um"},
	ScreenRings:    {"coma_z31_5mm_um", "coma_z31_6mm_um"},
}

// FieldHint is the extractor-side half of a C4 diagnostic.
type FieldHint = extractionfields.Hint

type anchoredValue struct {
	Text  string // text cells (quality indicator); Value is unused then
	Value float64
	Hint  FieldHint
}

var punct = regexp.MustCompile(`[\[\]{}()|:;.,'"_*<>!?=~^&@#$%/\\]`)

func normLabelWord(s string) string {
	return strings.ToLower(strings.TrimSpace(punct.ReplaceAllString(s, " ")))
}

type labelSpan struct {
	wide, second       bool
	text               string
	left, right, top   int
	bottom, cy, height int
}

// labelSpans returns, per row, every run of horizontally adjacent words (up to
// five) as one candidate label.
func hasAlnum(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 127 {
			return true
		}
	}
	return false
}

func labelSpans(words []ocr.Word, rowTolerance int) []labelSpan {
	var spans []labelSpan
	for _, row := range ocr.GroupRows(words, rowTolerance) {
		for i := range row.Words {
			parts := []string{}
			first := row.Words[i]
			left, right := first.Left, first.Left+first.Width
			top, bottom := first.Top, first.Top+first.Height
			if !hasAlnum(first.Text) {
				continue
			}
			for j := i; j < len(row.Words) && j < i+5; j++ {
				w := row.Words[j]
				if !hasAlnum(w.Text) {
					continue // icons and box borders between label words
				}
				if j > i && w.Left-right > 2*first.Height {
					break
				}
				right = w.Left + w.Width
				top, bottom = min(top, w.Top), max(bottom, w.Top+w.Height)
				parts = append(parts, normLabelWord(w.Text))
				spans = append(spans, labelSpan{
					text: strings.Join(strings.Fields(strings.Join(parts, " ")), " "),
					left: left, right: right, top: top, bottom: bottom,
					cy: (top + bottom) / 2, height: bottom - top,
				})
			}
		}
	}
	return spans
}

type screenWords struct {
	words []ocr.Word
	spans []labelSpan
}

// readAnchored reads every anchorField whose screen is present. pageImages maps
// 1-based page -> decoded 450-dpi render; pageOf maps screen -> page.
func readAnchored(
	ctx context.Context,
	pageOf map[Screen]int,
	pageImages map[int]image.Image,
) (map[string]anchoredValue, []FieldHint) {
	cache := map[Screen]*screenWords{}
	load := func(screen Screen) *screenWords {
		pageWords := &screenWords{}
		pageImage := pageImages[pageOf[screen]]
		if pageImage == nil {
			return pageWords
		}
		words, ocrError := document.OCRRegionWords(ctx, pageImage, pageImage.Bounds(), 1, 11)
		if ocrError != nil {
			return pageWords
		}
		pageWords.words = words
		pageWords.spans = labelSpans(append([]ocr.Word(nil), pageWords.words...), max(10, pageImage.Bounds().Dx()*5/1000))
		return pageWords
	}

	// Page OCR of each needed screen is independent: run them in parallel.
	var screensNeeded []Screen
	seen := map[Screen]bool{}
	for _, field := range anchorFields {
		for _, screen := range field.Screens {
			if pageImages[pageOf[screen]] != nil && !seen[screen] {
				seen[screen] = true
				screensNeeded = append(screensNeeded, screen)
			}
		}
	}
	loaded, _ := parallelwork.Map(ctx, screensNeeded, readerConcurrency, func(_ context.Context, screen Screen) (*screenWords, error) {
		return load(screen), nil
	})
	for i, screen := range screensNeeded {
		cache[screen] = loaded[i]
	}

	// Phase 1 (cheap, sequential): locate every label. Phase 2 (the expensive
	// OCR voting): read all value boxes in parallel, they are independent.
	type job struct {
		field  anchorField
		rule   extractionfields.Rule
		screen Screen
		span   labelSpan
		// fallback jobs (a field's second and later screens) only run for a
		// field its first screen did not yield.
		fallback bool
	}
	var jobs []job
	located := map[string]labelSpan{} // Key -> label span, for ColumnOf
	for _, field := range anchorFields {
		rule, ok := extractionfields.RuleFor(examKey, field.Spec)
		if !ok {
			continue
		}
		for index, screen := range field.Screens {
			if pageImages[pageOf[screen]] == nil {
				continue
			}
			span, found := pickLabel(cache[screen].spans, field, located)
			if !found {
				continue
			}
			located[field.Key] = span
			jobs = append(jobs, job{field, rule, screen, span, index > 0})
		}
	}
	type result struct {
		reading anchoredValue
		ok      bool
		suspect bool
		votes   map[float64]int // all readings, kept for the prior-based tie-break
	}
	runJobs := func(batch []job) []result {
		res, _ := parallelwork.Map(ctx, batch, readerConcurrency, func(c context.Context, j job) (result, error) {
			page := pageOf[j.screen]
			img := pageImages[page]
			span := j.span
			if j.field.ValueBelow {
				// The value sits on the next line, starting under the label.
				dy := span.height * 17 / 10
				span.cy += dy
				span.top += dy
				span.bottom += dy
				span.right = span.left - 3*span.height // value starts left of a partly read label
			}
			if j.field.Wide {
				span.wide = true
			}
			span.second = j.field.Second
			var allVotes map[float64]int
			value, votes, candidates, rect, ok := readValueBox(c, img, cache[j.screen].words, span, j.rule, &allVotes)
			if !ok {
				return result{suspect: votes == suspectCell, votes: allVotes}, nil
			}
			return result{anchoredValue{Value: value, Hint: FieldHint{
				Spec: j.field.Spec, Status: "valid", Stage: "recognition", Screen: string(j.screen), Page: page,
				Strategy: "label_anchor", BBox: to300dpi(rect, img.Bounds()), Votes: votes, Candidates: candidates,
			}}, true, false, allVotes}, nil
		})
		return res
	}

	// The first screen of a field is read first. The later screens repeat the
	// same box inside a busier layout (the 4-map screen puts the maps right next
	// to it and misreads red cells), so they are only a fallback for a field the
	// first screen did not yield, never a second opinion that can overrule it.
	var primary, fallbacks []job
	for _, j := range jobs {
		if j.fallback {
			fallbacks = append(fallbacks, j)
		} else {
			primary = append(primary, j)
		}
	}
	results := runJobs(primary)
	have := map[string]bool{}
	for i, j := range primary {
		if results[i].ok || results[i].suspect {
			have[j.field.Key] = true
		}
	}
	var pending []job
	for _, j := range fallbacks {
		if !have[j.field.Key] {
			pending = append(pending, j)
		}
	}
	results = append(results, runJobs(pending)...)
	jobs = append(primary, pending...)

	// Fields read on more than one screen independently (BAD-D on the Belin and
	// Cataract screens) must agree, otherwise the field is conflicting.
	out := map[string]anchoredValue{}
	var conflicts []FieldHint
	readings := map[string][]anchoredValue{}
	suspect := map[string]FieldHint{}
	var order []string
	for i, j := range jobs {
		if _, ok := readings[j.field.Key]; !ok {
			order = append(order, j.field.Key)
		}
		if results[i].suspect {
			suspect[j.field.Key] = FieldHint{Spec: j.field.Spec, Status: "unreadable", Stage: "recognition",
				Strategy: "label_anchor", Screen: string(j.screen), Page: pageOf[j.screen]}
		}
		readings[j.field.Key] = readings[j.field.Key]
		if results[i].ok {
			readings[j.field.Key] = append(readings[j.field.Key], results[i].reading)
		}
	}
	// The keratometry cells obey Km = (K1+K2)/2 and astigmatism = K2-K1: a
	// misread glyph (1 vs 7, a lost "1.") shows as a broken identity. Resolve
	// toward the one reading assignment that satisfies them, else drop the
	// values that cannot be trusted (never keep one side arbitrarily).
	accepted := map[string]float64{}
	candidates := map[string]map[float64]int{}
	firstJob := map[string]int{}
	for i, j := range jobs {
		if !slices.Contains(keratoKeys, j.field.Key) && !slices.Contains(depthKeys, j.field.Key) {
			continue
		}
		if _, ok := firstJob[j.field.Key]; !ok {
			firstJob[j.field.Key] = i
		}
		if candidates[j.field.Key] == nil {
			candidates[j.field.Key] = map[float64]int{}
		}
		for v, n := range results[i].votes {
			candidates[j.field.Key][v] += n
		}
	}
	for _, key := range keratoKeys {
		if rs := readings[key]; len(rs) > 0 && allEqual(rs) {
			accepted[key] = rs[0].Value
		}
	}
	acceptedDepth := map[string]float64{}
	for _, key := range depthKeys {
		if rs := readings[key]; len(rs) > 0 && allEqual(rs) {
			acceptedDepth[key] = rs[0].Value
		}
	}
	resolved := map[string]float64{}
	for key, v := range keratoResolve(accepted, candidates) {
		resolved[key] = v
	}
	for key, v := range depthResolve(acceptedDepth, candidates) {
		resolved[key] = v
	}
	for key, v := range resolved {
		if a, ok := accepted[key]; ok && a == v {
			continue
		}
		if a, ok := acceptedDepth[key]; ok && a == v {
			continue
		}
		j := jobs[firstJob[key]]
		readings[key] = []anchoredValue{{Value: v, Hint: FieldHint{Spec: j.field.Spec, Status: "valid", Stage: "recognition",
			Screen: string(j.screen), Page: pageOf[j.screen], Strategy: "label_anchor+consistency",
			Votes: candidates[key][v], Candidates: len(candidates[key])}}}
		accepted[key] = v
	}
	for _, key := range keratoDrop(accepted) {
		j := jobs[firstJob[key]]
		readings[key] = nil
		conflicts = append(conflicts, FieldHint{Spec: j.field.Spec, Status: "conflicting", Stage: "recognition",
			Strategy: "cross_check", Screen: string(j.screen), Page: pageOf[j.screen], Candidates: len(candidates[key])})
	}
	for _, key := range order {
		rs := readings[key]
		switch {
		case len(rs) == 0:
			if hint, ok := suspect[key]; ok {
				// Found but not trustworthy: keep the fixed-box fallback from
				// filling it with the same shortened number.
				conflicts = append(conflicts, hint)
			}
		case allEqual(rs):
			out[key] = rs[0]
		default:
			if os.Getenv("REFRATIA_OCR_DEBUG") == "1" {
				for _, r := range rs {
					fmt.Fprintf(os.Stderr, "pentacam conflict %s screen=%s value=%v votes=%d candidates=%d\n", key, r.Hint.Screen, r.Value, r.Hint.Votes, r.Hint.Candidates)
				}
			}
			conflicts = append(conflicts, FieldHint{
				Spec: rs[0].Hint.Spec, Status: "conflicting", Stage: "recognition", Strategy: "label_anchor",
				Screen: rs[0].Hint.Screen, Page: rs[0].Hint.Page, Candidates: len(rs),
			})
		}
	}
	if qi, hint, ok := readQuality(ctx, pageOf, pageImages, cache); ok {
		out["qi"] = anchoredValue{Hint: hint, Text: qi}
	}
	return out, conflicts
}

// The "QI:" label itself is routinely garbled; the quality cell is the box to the
// left of the "Eixo:" label on the same row.
var qualityLabel = lbl(`eixo`)
var qualityWord = regexp.MustCompile(`^[A-Z]{2,8}$`)

// readQuality reads the quality indicator text cell of the anterior cornea box
// on the first screen that carries it: its first word, read as letters by vote.
// Only a clear majority is accepted.
func readQuality(
	ctx context.Context,
	pageOf map[Screen]int,
	pageImages map[int]image.Image,
	cache map[Screen]*screenWords,
) (string, FieldHint, bool) {
	for _, screen := range []Screen{ScreenTopometric, ScreenRefractive} {
		page := pageOf[screen]
		img, sw := pageImages[page], cache[screen]
		if img == nil || sw == nil {
			continue
		}
		span, found := pickLabel(sw.spans, anchorField{Label: qualityLabel}, nil)
		if !found {
			continue
		}
		h := span.height
		// "Eixo:" is the first line of a two-line label; the box spans both lines.
		rect := image.Rect(span.left-6*h, span.top, span.left-h/4, span.top+22*h/10).Intersect(img.Bounds())
		letters := func(t string) string {
			words := strings.Fields(strings.ToUpper(regexp.MustCompile(`[^A-Za-z ]`).ReplaceAllString(foldAccents(t), " ")))
			if len(words) == 0 {
				return ""
			}
			return words[0]
		}
		text := readTextBox(ctx, img, rect, "", letters, "")
		if qualityWord.MatchString(text) {
			return text, FieldHint{Spec: "qi", Status: "valid", Stage: "recognition", Screen: string(screen), Page: page,
				Strategy: "label_anchor", BBox: to300dpi(rect, img.Bounds())}, true
		}
	}
	return "", FieldHint{}, false
}

// crossCheckAstigmatism applies the keratometry identities (keratoDrop) to the
// final values, whichever strategy read them: values that break them are
// removed and reported conflicting.
func crossCheckAstigmatism(values map[string]any) []FieldHint {
	num := map[string]float64{}
	for _, key := range keratoKeys {
		if v, ok := values[key].(float64); ok {
			num[key] = v
		}
	}
	var hints []FieldHint
	for _, key := range keratoDrop(num) {
		values[key] = nil
		hints = append(hints, FieldHint{Spec: valueKeySpec[key], Status: "conflicting", Stage: "recognition", Strategy: "cross_check", Candidates: 2})
	}
	return hints
}

// suspectCell is returned as the vote count of a cell that was deliberately not
// read because its OCR word shows a probable lost glyph.
const suspectCell = -1

// readerConcurrency bounds parallel tesseract processes (they are
// single-threaded under OMP_THREAD_LIMIT=1).
const readerConcurrency = 4

func allEqual(readings []anchoredValue) bool {
	for _, r := range readings[1:] {
		if r.Value != readings[0].Value {
			return false
		}
	}
	return true
}

func to300dpi(r image.Rectangle, page image.Rectangle) [4]int {
	k := 2480.0 / float64(page.Dx())
	return [4]int{int(float64(r.Min.X) * k), int(float64(r.Min.Y) * k), int(float64(r.Dx()) * k), int(float64(r.Dy()) * k)}
}

// pickLabel returns the topmost span matching the field's label (the Córnea
// Ant. box precedes the Post. and Real boxes), or, with ColumnOf, the topmost
// match below and in the column of that anchor's label.
func pickLabel(spans []labelSpan, field anchorField, located map[string]labelSpan) (labelSpan, bool) {
	var ref *labelSpan
	if field.ColumnOf != "" {
		r, ok := located[field.ColumnOf]
		if !ok {
			return labelSpan{}, false
		}
		ref = &r
	}
	var best *labelSpan
	for i := range spans {
		s := spans[i]
		if !field.Label.MatchString(s.text) {
			continue
		}
		if ref != nil && (s.top <= ref.top+ref.height/2 || s.left < ref.left-ref.height || s.left > ref.left+2*ref.height) {
			continue
		}
		if best == nil || (!field.Bottom && s.top < best.top) || (field.Bottom && s.top > best.top) {
			best = &spans[i]
		}
	}
	if best == nil {
		return labelSpan{}, false
	}
	if field.Rank > 0 {
		// n-th match from the left among those on the same row as the best one.
		var row []labelSpan
		for _, s := range spans {
			if field.Label.MatchString(s.text) && abs(s.cy-best.cy) <= best.height {
				row = append(row, s)
			}
		}
		sort.Slice(row, func(i, j int) bool { return row[i].left < row[j].left })
		if field.Rank >= len(row) {
			return labelSpan{}, false
		}
		return row[field.Rank], true
	}
	return *best, true
}

// readValueBox finds the value cell to the right of label and reads it by
// voting across preprocessings and PSMs (plus the whole-page OCR token, when
// there is one). A value needs two agreeing reads and no equal rival.
func readValueBox(
	ctx context.Context,
	img image.Image,
	words []ocr.Word,
	label labelSpan,
	rule extractionfields.Rule,
	keep *map[float64]int,
) (float64, int, int, image.Rectangle, bool) {
	h := label.height
	boxWidth := 7 * h / 2
	searchReach := 8 * h
	if label.wide {
		boxWidth, searchReach = 14*h, 14*h
	}
	rect := image.Rect(label.right+h/4, label.cy-h, label.right+h/4+boxWidth, label.cy+h)

	// The value cell is the first numeric word to the right of the label within
	// the row band; a word with letters in between is the next label, so stop.
	var tokenValue *float64
	variants := []image.Rectangle{rect}
	best := -1
	var candidates []int
	for i, w := range words {
		cy := w.Top + w.Height/2
		if w.Left < label.right-h/4 || w.Left > label.right+searchReach || abs(cy-label.cy) > h*3/4 ||
			!strings.ContainsAny(w.Text, "0123456789") {
			continue
		}
		candidates = append(candidates, i)
	}
	sort.Slice(candidates, func(a, b int) bool { return words[candidates[a]].Left < words[candidates[b]].Left })
	pick := 0
	if label.second {
		pick = 1
	}
	if pick < len(candidates) {
		best = candidates[pick]
	} else if label.second {
		return 0, 0, 0, rect, false // no second number on the row
	}
	if best < 0 {
		// No word with a digit: a fully garbled cell ("[aan" for "44.4 D") still
		// marks where the cell is. Take the nearest word on the row that is
		// beyond the label and starts a cell, if any.
		for i, w := range words {
			cy := w.Top + w.Height/2
			if w.Left < label.right+h/2 || w.Left > label.right+searchReach || abs(cy-label.cy) > h*3/4 || w.Width < h {
				continue
			}
			if best < 0 || w.Left < words[best].Left {
				best = i
			}
		}
	}
	if best >= 0 {
		w := words[best]
		// The OCR word still carries the cell border on its left and the unit
		// on its right; both are cut off by character proportion.
		// A leading digit stuck to the cell border is often misread as a letter
		// ("|12" -> "fi2") or lost, and the crops then agree on the shortened
		// number ("2"). For integer cells two or more non-digit glyphs before
		// the first digit therefore mean the value cannot be trusted.
		if rule.Kind == "int" && leadingNonDigits(w.Text) >= 2 && strings.ContainsAny(w.Text, "0123456789") {
			return 0, suspectCell, 0, rect, false
		}
		extents := trimWordBox(w)
		if _, parses := normalizePrinted(w.Text, rule); !parses {
			// A garbled word ("[4a" for 44.1) says little about how wide the
			// cell is: read the whole cell-sized window starting at the word.
			extents = [2][2]int{{w.Left, w.Left + 3*h}, {w.Left, w.Left + 7*h/2}}
		}
		for _, e := range extents {
			variants = append(variants, image.Rect(e[0]-h/6, min(w.Top, label.top)-h/4, e[1]+h/6, max(w.Top+w.Height, label.bottom)+h/4).Intersect(img.Bounds()))
		}
		rect = variants[0]
		if v, ok := normalizePrinted(w.Text, rule); ok {
			tokenValue = &v
		}
	}
	rect = rect.Intersect(img.Bounds())
	if rect.Empty() {
		return 0, 0, 0, rect, false
	}

	votes := map[float64]int{}
	// Votes per binarization family (grey vs fixed threshold), to tell a
	// glyph distorted by one preprocessing from a value the cell really shows.
	var family [2]map[float64]int
	family[0], family[1] = map[float64]int{}, map[float64]int{}
	if tokenValue != nil {
		votes[*tokenValue]++
	}
	for _, variant := range variants {
		if variant.Empty() {
			continue
		}
		for _, luma := range []bool{false, true} {
			if luma && len(votes) > 0 {
				break // luminance is only a fallback when darkest-channel reads gave nothing
			}
			cell := frameWithMargin(darkOnLight(ocr.PrepareGrayMode(img, variant, 1, 0, luma)), 10)
			cellPNG, err := ocr.EncodePNG(cell)
			if err != nil {
				continue
			}
			// Scale and threshold are varied on purpose: pixel-font digits such as
			// 3/9 flip with the geometry, and a value is only accepted when the
			// reads agree across those variations.
			for _, scale := range []int{2, 3, 4} {
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
					// Without the whitelist a unit glued to the cell ("D", "mm")
					// stays letters and is dropped by normalizePrinted instead
					// of becoming "0".
					for _, mode := range []struct {
						psm       int
						whitelist string
					}{{7, ""}, {7, "0123456789.,+-"}, {13, "0123456789.,+-"}} {
						text, err := ocr.RunTesseractText(ctx, encoded, mode.psm, mode.whitelist)
						if err != nil {
							continue
						}
						if v, ok := normalizePrinted(text, rule); ok {
							votes[v]++
							family[min(int(threshold), 1)][v]++
						}
					}
				}
			}
		}
	}

	bestValue, bestVotes, rival := 0.0, 0, false
	for v, n := range votes {
		switch {
		case n > bestVotes:
			bestValue, bestVotes, rival = v, n, false
		case n == bestVotes:
			rival = true
		}
	}
	if keep != nil {
		*keep = votes
	}
	total := 0
	for _, n := range votes {
		total += n
	}
	if os.Getenv("REFRATIA_OCR_DEBUG") == "1" {
		// Clinical readings only (never identity fields).
		fmt.Fprintf(os.Stderr, "pentacam anchor %q votes=%v grey=%v\n", label.text, votes, family[0])
	}
	// A competing reading (3 vs 9 in a pixel font) must not be outvoted by a
	// small margin: with rivals the winner needs 80% of the valid reads.
	if bestVotes < 2 || rival || (len(votes) > 1 && bestVotes*5 < total*4) {
		return 0, bestVotes, len(votes), rect, false
	}
	return bestValue, bestVotes, len(votes), rect, true
}

func leadingNonDigits(text string) int {
	n := 0
	for _, r := range text {
		if r >= '0' && r <= '9' {
			break
		}
		n++
	}
	return n
}

// trimWordBox returns two horizontal extents of the numeric core of an OCR
// word (a tight and a loose one). Leading box-border glyphs ("[", "|", "}")
// and trailing unit letters ("D", "mm", "°") are cut proportionally to their
// share of the characters; the exact glyph widths are unknown, so both
// extents are read and voted.
func trimWordBox(w ocr.Word) [2][2]int {
	text := []rune(w.Text)
	start, end := 0, len(text)
	for start < end && !strings.ContainsRune("0123456789+-−–", text[start]) {
		start++
	}
	for end > start && !strings.ContainsRune("0123456789", text[end-1]) {
		end--
	}
	if end <= start {
		return [2][2]int{{w.Left, w.Left + w.Width}, {w.Left, w.Left + w.Width}}
	}
	char := float64(w.Width) / float64(len(text))
	at := func(chars float64) int { return w.Left + int(chars*char) }
	// The second extent is the whole OCR word: when the OCR merges the cell
	// border with a leading digit ("|12" read as "[2") the character count
	// misplaces the cut, and only the untrimmed word keeps the digit.
	return [2][2]int{
		{at(float64(start) * 0.9), at(float64(end) + 0.5)},
		{w.Left - w.Height/4, w.Left + w.Width + w.Height/8},
	}
}

// darkOnLight inverts a cell whose background is darker than its text (white
// digits on a red out-of-norm box) so every cell is dark digits on light.
func darkOnLight(g *image.Gray) *image.Gray {
	dark := 0
	for _, v := range g.Pix {
		if v < 128 {
			dark++
		}
	}
	if dark*2 <= len(g.Pix) {
		return g
	}
	out := image.NewGray(g.Bounds())
	for i, v := range g.Pix {
		out.Pix[i] = 255 - v
	}
	return trimDarkEdges(out)
}

// trimDarkEdges drops outer columns/rows that are mostly black: after the
// inversion of a coloured cell these are the page area around the cell, which
// the OCR would otherwise read as a glyph.
func trimDarkEdges(g *image.Gray) *image.Gray {
	b := g.Bounds()
	colMean := func(x int) int {
		sum := 0
		for y := b.Min.Y; y < b.Max.Y; y++ {
			sum += int(g.GrayAt(x, y).Y)
		}
		return sum / b.Dy()
	}
	left, right := b.Min.X, b.Max.X
	for left < right-4 && colMean(left) < 80 {
		left++
	}
	for right > left+4 && colMean(right-1) < 80 {
		right--
	}
	return g.SubImage(image.Rect(left, b.Min.Y, right, b.Max.Y)).(*image.Gray)
}

// frameWithMargin returns g on a white canvas with margin pixels around it.
func frameWithMargin(g *image.Gray, margin int) *image.Gray {
	b := g.Bounds()
	out := image.NewGray(image.Rect(0, 0, b.Dx()+2*margin, b.Dy()+2*margin))
	for i := range out.Pix {
		out.Pix[i] = 255
	}
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.SetGray(x+margin, y+margin, g.GrayAt(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
