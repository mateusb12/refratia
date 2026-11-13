package pentacam

import (
	"context"
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

// FieldHint is the extractor-side half of a C4 diagnostic; the app merges it
// with the validation result. Spec is the _field_spec.json field name.
type FieldHint struct {
	Spec       string
	Status     string // "valid" | "missing" | "unreadable" | "not_applicable"
	Stage      string
	Screen     Screen
	Page       int
	Strategy   string
	BBox       [4]int // x, y, w, h in px at 300 dpi (A4 width 2480)
	Votes      int
	Candidates int
}

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
func labelSpans(words []ocr.Word, rowTolerance int) []labelSpan {
	var spans []labelSpan
	for _, row := range ocr.GroupRows(words, rowTolerance) {
		for i := range row.Words {
			parts := []string{}
			first := row.Words[i]
			left, right := first.Left, first.Left+first.Width
			top, bottom := first.Top, first.Top+first.Height
			for j := i; j < len(row.Words) && j < i+5; j++ {
				w := row.Words[j]
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
			if !found {
				continue
			}
			located[field.Key] = span
			value, votes, candidates, rect, ok := readValueBox(ctx, img, sw.words, span, rule)
			if !ok {
				continue
			}
			out[field.Key] = anchoredValue{Value: value, Hint: FieldHint{
				Spec: field.Spec, Status: "valid", Stage: "recognition", Screen: screen, Page: page,
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

	// Use the page-OCR word to the right, when close, to size the box.
	var tokenValue *float64
	best := -1
	for i, w := range words {
		cy := w.Top + w.Height/2
		if w.Left >= label.right-h/4 && w.Left <= label.right+5*h/2 && abs(cy-label.cy) <= max(h, w.Height) &&
			(best < 0 || w.Left < words[best].Left) {
			best = i
		}
	}
	if best >= 0 {
		w := words[best]
		rect = image.Rect(w.Left-h/8, min(w.Top, label.top)-h/4, w.Left+w.Width+h/8, max(w.Top+w.Height, label.bottom)+h/4)
		if v, ok := normalizePrinted(w.Text, rule); ok {
			tokenValue = &v
		}
	}
	rect = rect.Intersect(img.Bounds())
	if rect.Empty() {
		return 0, 0, 0, rect, false
	}

	votes := map[float64]int{}
	reads := 0
	if tokenValue != nil {
		votes[*tokenValue]++
		reads++
	}
	for _, threshold := range []uint8{0, 130, 160, 190} {
		gray := ocr.PrepareGray(img, rect, 3, 0)
		ocr.AutoContrast(gray)
		encoded, err := ocr.EncodePNG(gray)
		if err != nil {
			continue
		}
		if threshold > 0 {
			encoded, err = pentacamBinaryUpscale(mustPNG(img, rect), 3, threshold)
			if err != nil {
				continue
			}
		}
		for _, psm := range []int{7, 13} {
			text, err := ocr.RunTesseractText(ctx, encoded, psm, "0123456789.,+-")
			if err != nil {
				continue
			}
			if v, ok := normalizePrinted(text, rule); ok {
				votes[v]++
				reads++
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
	if bestVotes < 2 || rival {
		return 0, bestVotes, len(votes), rect, false
	}
	return bestValue, bestVotes, len(votes), rect, true
}

func mustPNG(img image.Image, r image.Rectangle) []byte {
	encoded, _ := ocr.EncodePNG(ocr.PrepareGray(img, r, 1, 0))
	return encoded
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

