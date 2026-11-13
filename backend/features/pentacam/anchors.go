package pentacam

import (
	"context"
	"fmt"
	"image"
	"regexp"
	"strings"

	"refratia/backend/features/extractionfields"
	"refratia/backend/shared/ocr"
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
	// ColumnOf, when set, restricts candidates to labels below the label of
	// that other anchor and in the same column (the "Índices" box repeats
	// labels such as KI that also exist in the cornea boxes).
	ColumnOf string
}

func lbl(pattern string) *regexp.Regexp { return regexp.MustCompile(`^(?:` + pattern + `)$`) }

var anchorFields = []anchorField{
	// Córnea Ant. box (identical on the 4-maps and topometric screens).
	{Key: "k1_d", Spec: "k1_d", Screens: []Screen{ScreenRefractive, ScreenTopometric}, Label: lbl(`k[1il]`)},
	{Key: "k2_d", Spec: "k2_d", Screens: []Screen{ScreenRefractive, ScreenTopometric}, Label: lbl(`k2`)},
	{Key: "km_d", Spec: "km_d", Screens: []Screen{ScreenRefractive, ScreenTopometric}, Label: lbl(`km`)},
	{Key: "astigmatism_d", Spec: "astig_ant_d", Screens: []Screen{ScreenRefractive, ScreenTopometric}, Label: lbl(`astig`)},
	{Key: "axis_deg", Spec: "axis_flat_deg", Screens: []Screen{ScreenRefractive, ScreenTopometric}, Label: lbl(`eixo`)},

	// Topometric screen.
	{Key: "kmax_d", Spec: "kmax_ant_d", Screens: []Screen{ScreenTopometric}, Label: lbl(`k m[aá]x ant`)},
	{Key: "thinnest_um", Spec: "pachy_thinnest_um", Screens: []Screen{ScreenTopometric}, Label: lbl(`ponto fino`)},
	{Key: "pupil_center_um", Spec: "pachy_pupil_center_um", Screens: []Screen{ScreenTopometric}, Label: lbl(`centro pupilar`)},
	{Key: "vertex_normal_um", Spec: "pachy_vertex_um", Screens: []Screen{ScreenTopometric}, Label: lbl(`pachy vertex n`)},
	{Key: "acd_external_mm", Spec: "acd_ext_mm", Screens: []Screen{ScreenTopometric}, Label: lbl(`prof c.mara ant ext`)},
	{Key: "isv", Spec: "isv", Screens: []Screen{ScreenTopometric}, Label: lbl(`isv|15v`)},
	{Key: "iha", Spec: "iha", Screens: []Screen{ScreenTopometric}, Label: lbl(`iha`)},
	{Key: "ihd", Spec: "ihd", Screens: []Screen{ScreenTopometric}, Label: lbl(`ihd`)},
	{Key: "iva", Spec: "iva", Screens: []Screen{ScreenTopometric}, Label: lbl(`iva|vay|iv4`), ColumnOf: "isv"},
	{Key: "ki", Spec: "ki", Screens: []Screen{ScreenTopometric}, Label: lbl(`k[i1l]`), ColumnOf: "isv"},
	{Key: "cki", Spec: "cki", Screens: []Screen{ScreenTopometric}, Label: lbl(`cki`)},

	// Cataract Pre-OP screen.
	{Key: "z40_6mm_um", Spec: "z40_6mm_um", Screens: []Screen{ScreenCataract}, Label: lbl(`z40`)},
	{Key: "acd_internal_mm", Spec: "acd_int_mm", Screens: []Screen{ScreenCataract}, Label: lbl(`prof c.mara ant int`)},
}

// screenSpecFields lists which spec fields live on which screen, including the
// fields still read by the fixed-box path. A screen that is absent from a
// report makes its fields not_applicable (never read from another page).
var screenSpecFields = map[Screen][]string{
	ScreenRefractive: {"k1_d", "k2_d", "km_d", "astig_ant_d", "axis_flat_deg"},
	ScreenTopometric: {"kmax_ant_d", "pachy_thinnest_um", "pachy_pupil_center_um", "pachy_vertex_um",
		"acd_ext_mm", "isv", "iva", "iha", "ihd", "ki", "cki", "tkc"},
	ScreenCataract: {"z40_6mm_um", "acd_int_mm"},
	ScreenBelin:    {"bad_d", "art_max_um"},
	ScreenRings:    {"coma_z31_5mm_um"},
}

// FieldHint is the extractor-side half of a C4 diagnostic.
type FieldHint = extractionfields.Hint

type anchoredValue struct {
	Value float64
	Hint  FieldHint
}

var punct = regexp.MustCompile(`[\[\]{}()|:;.,'"_*<>!?=~^&@#$%/\\]`)

func normLabelWord(s string) string {
	return strings.ToLower(strings.TrimSpace(punct.ReplaceAllString(s, " ")))
}

type labelSpan struct {
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

// readAnchored reads every anchorField whose screen is present. pageImages maps
// 1-based page -> decoded 450-dpi render; pageOf maps screen -> page.
func readAnchored(
	ctx context.Context,
	pageOf map[Screen]int,
	pageImages map[int]image.Image,
) map[string]anchoredValue {
	type screenWords struct {
		words []ocr.Word
		spans []labelSpan
	}
	cache := map[Screen]*screenWords{}
	load := func(screen Screen) *screenWords {
		if sw, ok := cache[screen]; ok {
			return sw
		}
		sw := &screenWords{}
		cache[screen] = sw
		img := pageImages[pageOf[screen]]
		if img == nil {
			return sw
		}
		encoded, err := ocr.EncodePNG(ocr.PrepareGray(img, img.Bounds(), 1, 0))
		if err != nil {
			return sw
		}
		tsv, err := ocr.RunTesseractTSVWithPSM(ctx, encoded, 11)
		if err != nil {
			return sw
		}
		sw.words, _, _ = ocr.ParseTSVWords(tsv)
		sw.spans = labelSpans(append([]ocr.Word(nil), sw.words...), max(10, img.Bounds().Dx()*5/1000))
		return sw
	}

	out := map[string]anchoredValue{}
	located := map[string]labelSpan{} // Key -> label span, for ColumnOf
	for _, field := range anchorFields {
		rule, ok := extractionfields.RuleFor(examKey, field.Spec)
		if !ok {
			continue
		}
		for _, screen := range field.Screens {
			page := pageOf[screen]
			img := pageImages[page]
			if img == nil {
				continue
			}
			sw := load(screen)
			span, found := pickLabel(sw.spans, field, located)
			if debugAnchors {
				println("DBG", field.Key, found, span.text, span.left, span.top)
			}
			if !found {
				continue
			}
			located[field.Key] = span
			value, votes, candidates, rect, ok := readValueBox(ctx, img, sw.words, span, rule)
			if debugAnchors {
				println("DBG  ->", field.Key, value, votes, candidates, ok)
			}
			if !ok {
				continue
			}
			out[field.Key] = anchoredValue{Value: value, Hint: FieldHint{
				Spec: field.Spec, Status: "valid", Stage: "recognition", Screen: string(screen), Page: page,
				Strategy: "label_anchor", BBox: to300dpi(rect, img.Bounds()), Votes: votes, Candidates: candidates,
			}}
			break
		}
	}
	return out
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
		if ref != nil && (s.top < ref.top || s.left < ref.left-ref.height || s.left > ref.left+2*ref.height) {
			continue
		}
		if best == nil || s.top < best.top {
			best = &spans[i]
		}
	}
	if best == nil {
		return labelSpan{}, false
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
) (float64, int, int, image.Rectangle, bool) {
	h := label.height
	rect := image.Rect(label.right+h/4, label.cy-h, label.right+h/4+7*h/2, label.cy+h)

	// The value cell is the first numeric word to the right of the label within
	// the row band; a word with letters in between is the next label, so stop.
	var tokenValue *float64
	variants := []image.Rectangle{rect}
	best := -1
	for i, w := range words {
		cy := w.Top + w.Height/2
		if w.Left < label.right-h/4 || w.Left > label.right+8*h || abs(cy-label.cy) > h*3/4 ||
			!strings.ContainsAny(w.Text, "0123456789") {
			continue
		}
		if best >= 0 && w.Left >= words[best].Left {
			continue
		}
		best = i
	}
	if best >= 0 {
		w := words[best]
		// The OCR word still carries the cell border on its left and the unit
		// on its right; both are cut off by character proportion.
		extents := trimWordBox(w)
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
	if tokenValue != nil {
		votes[*tokenValue]++
	}
	for _, variant := range variants {
		if variant.Empty() {
			continue
		}
		cell := frameWithMargin(darkOnLight(ocr.PrepareGray(img, variant, 1, 0)), 10)
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
					}
				}
			}
		}
	}

	if debugAnchors {
		println("DBG   votes", label.text, fmt.Sprint(votes))
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
	total := 0
	for _, n := range votes {
		total += n
	}
	// A competing reading (3 vs 9 in a pixel font) must not be outvoted by a
	// small margin: with rivals the winner needs 80% of the valid reads.
	if bestVotes < 2 || rival || (len(votes) > 1 && bestVotes*5 < total*4) {
		return 0, bestVotes, len(votes), rect, false
	}
	return bestValue, bestVotes, len(votes), rect, true
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
	return [2][2]int{
		{at(float64(start) * 0.9), at(float64(end) + 0.5)},
		{at(float64(start) * 0.5), at(float64(end) + 0.1)},
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

var debugAnchors = false
