package pentacam

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"refratia/backend/src/shared/pdf"
)

// Invented 5x9 / 3x9 test shapes (not any real report's font). Every column
// carries ink, so each shape is one column group.
var testFont = map[rune][]string{
	'4': {"#...#", "#.#.#", "##..#", ".#.##", "#..#.", "#.#..", "..###", ".#...", "#...#"},
	'2': {"##.##", "#####", "..#..", "#...#", "##.#.", ".#.#.", "###..", "#..##", "..###"},
	'1': {"#.#", "###", ".#.", "#..", "..#", ".##", "#.#", "##.", "#.."},
	'5': {".#.#.", "#####", "#...#", "..#..", ".###.", "#.#.#", "..#..", "##.##", "#...#"},
}

var (
	tWhite  = color.RGBA{255, 255, 255, 255}
	tBlack  = color.RGBA{0, 0, 0, 255}
	redC    = color.RGBA{220, 20, 20, 255}
	tYellow = color.RGBA{240, 230, 60, 255}
)

func tFill(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

// drawText draws s at (x,y) with the given gap rules: 1 px between digits
// (3 after '1'), '.' = one pixel on the baseline with 2 px each side.
func drawText(img *image.RGBA, x, y int, s string, c color.RGBA) int {
	prev := ' '
	for _, ch := range s {
		if ch == '.' {
			x += 2
			img.SetRGBA(x, y+8, c)
			x += 1 + 1
			prev = ch
			continue
		}
		if prev == '1' {
			x += 2
		}
		g := testFont[ch]
		for j, row := range g {
			for i, v := range row {
				if v == '#' {
					img.SetRGBA(x+i, y+j, c)
				}
			}
		}
		x += len(g[0]) + 1
		prev = ch
	}
	return x
}

// cellImage: 120x50 tWhite page with a 60x24 box at (30,13), 1 px tBlack border.
func cellImage(bg color.RGBA) (*image.RGBA, image.Rectangle) {
	img := image.NewRGBA(image.Rect(0, 0, 120, 50))
	tFill(img, img.Bounds(), tWhite)
	box := image.Rect(30, 13, 90, 37)
	tFill(img, box, tBlack)
	tFill(img, box.Inset(1), bg)
	return img, box
}

func tKeys(gs []glyph) []string {
	var o []string
	for _, g := range gs {
		o = append(o, g.key())
	}
	return o
}

func TestNativeSegmentBorderAdjacentWithUnit(t *testing.T) {
	img, box := cellImage(tWhite)
	// glyphs start 1 px from the inner border (x = box.Min.X+2), then a unit after a gap.
	end := drawText(img, box.Min.X+2, box.Min.Y+7, "42.1", tBlack)
	drawText(img, end+5, box.Min.Y+7, "5", tBlack) // 6 blank columns after the narrow 1: "unit"
	gs := segmentNumber(img, box.Inset(3))
	if len(gs) != 4 {
		t.Fatalf("got %d glyphs, want 4 (%v)", len(gs), tKeys(gs))
	}
	if gs[2].W != 1 || gs[2].H != 9 || strings.Count(gs[2].key()[4:], "1") != 1 {
		t.Errorf("decimal point not a single pixel: %q", gs[2].key())
	}
	if gs[0].key() != "5x9:"+strings.Join(testFont['4'], "")[:0]+bitsOf(testFont['4']) {
		t.Errorf("key of '4' = %q", gs[0].key())
	}
	if gs[0].Box.Min.X != box.Min.X+2 {
		t.Errorf("border-adjacent glyph at %d", gs[0].Box.Min.X)
	}
}

func bitsOf(rows []string) string {
	return strings.NewReplacer("#", "1", ".", "0").Replace(strings.Join(rows, ""))
}

func TestNativeSegmentRedFillWhiteInk(t *testing.T) {
	img, box := cellImage(redC)
	drawText(img, box.Min.X+4, box.Min.Y+7, "55", tWhite)
	gs := segmentNumber(img, box.Inset(3))
	if len(gs) != 2 || gs[0].key() != "5x9:"+bitsOf(testFont['5']) {
		t.Fatalf("red cell: %v", tKeys(gs))
	}
}

func TestNativeSegmentYellowFillAndRedDigits(t *testing.T) {
	img, box := cellImage(tYellow)
	drawText(img, box.Min.X+4, box.Min.Y+7, "21", tBlack)
	if gs := segmentNumber(img, box.Inset(3)); len(gs) != 2 {
		t.Fatalf("tYellow cell: %v", tKeys(gs))
	}
	img, box = cellImage(tWhite)
	drawText(img, box.Min.X+4, box.Min.Y+7, "21", redC) // red digits on tWhite are not a fill
	if gs := segmentNumber(img, box.Inset(3)); len(gs) != 2 {
		t.Fatalf("red digits: %v", tKeys(gs))
	}
}

func TestNativeSegmentFailsClosed(t *testing.T) {
	img, box := cellImage(tWhite)
	if gs := segmentNumber(img, box.Inset(3)); gs != nil {
		t.Errorf("empty cell: %v", tKeys(gs))
	}
	img, box = cellImage(tWhite)
	drawText(img, box.Min.X+4, box.Min.Y+7, "42", tBlack)
	img.SetRGBA(box.Min.X+9, box.Min.Y+9, tBlack) // bridges the two glyphs
	img.SetRGBA(box.Min.X+10, box.Min.Y+9, tBlack)
	if gs := segmentNumber(img, box.Inset(3)); gs != nil {
		t.Errorf("touching glyphs: %v", tKeys(gs))
	}
	img, box = cellImage(tWhite)
	drawText(img, box.Min.X+4, box.Min.Y+7, "4", tBlack)
	drawText(img, box.Min.X+11, box.Min.Y+9, "2", tBlack) // different row band
	if gs := segmentNumber(img, box.Inset(3)); gs != nil {
		t.Errorf("inconsistent height: %v", tKeys(gs))
	}
	noBox := image.NewRGBA(image.Rect(0, 0, 120, 50))
	tFill(noBox, noBox.Bounds(), tWhite)
	drawText(noBox, 40, 20, "42", tBlack)
	if gs := segmentNumber(noBox, image.Rect(35, 15, 70, 35)); gs != nil {
		t.Errorf("no box: %v", tKeys(gs))
	}
}

func TestNativePagesAndMapSynthetic(t *testing.T) {
	var jp bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 40, 30))
	tFill(im, im.Bounds(), tWhite)
	jpeg.Encode(&jp, im, nil)
	content := "0.5 0 0 -0.5 10 100 cm 1 0 0 1 0 0 cm 40 0 0 -30 0 30 cm /Im Do"
	_ = content
	pdfBytes := append([]byte("%PDF obj stream\r\n"), jp.Bytes()...)
	pdfBytes = append(pdfBytes, []byte("\r\nendstream stream\n")...)
	pdfBytes = append(pdfBytes, jp.Bytes()...)
	pages, err := nativePages(pdfBytes)
	if err != nil || len(pages) != 2 || pages[2].Bounds().Dx() != 40 {
		t.Fatalf("pages %d err %v", len(pages), err)
	}
	if p, err := nativePages([]byte("%PDF no images")); p != nil || err == nil {
		t.Errorf("no streams must fail closed")
	}
}

// registration recovers the placement: the render is a bilinear x3 upsample
// of a smooth native image whose true origin is 2 and 3 render px
// away from the nominal map.
func TestNativeRegisterFindsOffset(t *testing.T) {
	nat := image.NewRGBA(image.Rect(0, 0, 100, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 100; x++ {
			v := uint8(128 + 100*math.Sin(float64(x)/3)*math.Cos(float64(y)/2.5))
			nat.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	truth := nativeMap{2, -3, 3, 3}
	render := image.NewRGBA(image.Rect(0, 0, 330, 270))
	at := func(x, y int) float64 {
		return float64(nat.RGBAAt(min(max(x, 0), 99), min(max(y, 0), 79)).R)
	}
	for y := 0; y < 270; y++ {
		for x := 0; x < 330; x++ {
			u, v := (float64(x)+.5-truth.ox)/3-.5, (float64(y)+.5-truth.oy)/3-.5
			u0, v0 := int(math.Floor(u)), int(math.Floor(v))
			fx, fy := u-float64(u0), v-float64(v0)
			p := (at(u0, v0)*(1-fx)+at(u0+1, v0)*fx)*(1-fy) + (at(u0, v0+1)*(1-fx)+at(u0+1, v0+1)*fx)*fy
			render.SetRGBA(x, y, color.RGBA{uint8(p), uint8(p), uint8(p), 255})
		}
	}
	m := nativeMap{0, 0, 3, 3} // nominal, 2 and 3 render px off
	rc := image.Rect(90, 80, 210, 140)
	r, mad, ok := m.register(render, nat, rc)
	want := truth.toNative(rc)
	if !ok || r != want || mad > 1 {
		t.Fatalf("register = %v mad %.2f ok %v, want %v", r, mad, ok, want)
	}
	// a render that does not match the native image must be rejected
	other := image.NewRGBA(render.Bounds())
	tFill(other, other.Bounds(), tBlack)
	if _, _, ok := m.register(other, nat, rc); ok {
		t.Errorf("mismatched content accepted")
	}
}

// ---- glyph bank (harvest / read) ----

func glyphOf(ch rune) glyph {
	if ch == '.' {
		return glyph{Box: image.Rect(0, 8, 1, 9), W: 1, H: 9, Bits: []bool{false, false, false, false, false, false, false, false, true}}
	}
	rows := testFont[ch]
	g := glyph{Box: image.Rect(0, 0, len(rows[0]), 9), W: len(rows[0]), H: 9}
	for _, row := range rows {
		for _, v := range row {
			g.Bits = append(g.Bits, v == '#')
		}
	}
	return g
}

func glyphsOf(s string) []glyph {
	var gs []glyph
	for _, ch := range s {
		gs = append(gs, glyphOf(ch))
	}
	return gs
}

func TestNativeHarvestRequiresExactCountAndDot(t *testing.T) {
	b := &glyphBank{}
	if !b.harvest("a", glyphsOf("42.1"), "42.1") {
		t.Fatal("exact match not harvested")
	}
	for name, c := range map[string]struct{ glyphs, expected string }{
		"count":    {"42.1", "42.10"},
		"fewer":    {"42.1", "421"},
		"dot":      {"42.1", "4.21"},
		"not dot":  {"4215", "42.1"},
		"sign":     {"42.1", "-4.1"},
		"no digit": {"42.1", "42.x"},
	} {
		if b.harvest("b-"+name, glyphsOf(c.glyphs), c.expected) {
			t.Errorf("%s mismatch harvested", name)
		}
	}
	if len(b.obs) != 3 {
		t.Errorf("observations = %d, want the 3 digits of the one good cell", len(b.obs))
	}
}

func TestNativeBankReadAndLeaveOneCellOut(t *testing.T) {
	b := &glyphBank{}
	b.harvest("A", glyphsOf("42.1"), "42.1")
	b.harvest("B", glyphsOf("2.1"), "2.1")
	if text, why := b.read(glyphsOf("42.1"), ""); text != "42.1" || why != "" {
		t.Fatalf("read = %q %q", text, why)
	}
	// without cell A nobody else has seen the 4: unknown, so no veto is possible
	if _, why := b.read(glyphsOf("42.1"), "A"); why != "unknown_digit" {
		t.Errorf("leave-one-out kept the cell's own glyphs: %q", why)
	}
	// the 2 and the 1 are vouched for by cell B alone, still known without A
	if text, why := b.read(glyphsOf("2.1"), "A"); text != "2.1" || why != "" {
		t.Errorf("read without A = %q %q", text, why)
	}
	if _, why := b.read(glyphsOf("5"), ""); why != "unknown_digit" {
		t.Errorf("unknown glyph accepted: %q", why)
	}
}

func TestNativeBankDropsAmbiguousKeysAndChars(t *testing.T) {
	// one bitmap labelled with two chars: the key is dropped
	b := &glyphBank{}
	b.harvest("A", glyphsOf("4"), "4")
	b.harvest("B", glyphsOf("4"), "2")
	if _, why := b.read(glyphsOf("4"), ""); why != "ambiguous" {
		t.Errorf("key with two chars: %q", why)
	}
	// one char with two bitmaps of the same height: the char is dropped
	b = &glyphBank{}
	b.harvest("A", glyphsOf("4"), "4")
	b.harvest("B", glyphsOf("2"), "4")
	for _, s := range []string{"4", "2"} {
		if _, why := b.read(glyphsOf(s), ""); why != "ambiguous" {
			t.Errorf("char with two keys (%s): %q", s, why)
		}
	}
	// ... but a different glyph height is another size class
	b = &glyphBank{}
	b.harvest("A", glyphsOf("4"), "4")
	big := glyphOf('2')
	big.H = 10
	b.harvest("B", []glyph{big}, "4")
	if text, why := b.read(glyphsOf("4"), ""); text != "4" || why != "" {
		t.Errorf("size classes mixed: %q %q", text, why)
	}
	// a vetoed cell no longer teaches
	b.drop("A")
	if _, why := b.read(glyphsOf("4"), ""); why != "unknown_digit" {
		t.Errorf("dropped cell still known: %q", why)
	}
}

// ---- "second" fields: the digits inside the parentheses ----

func paren(img *image.RGBA, x, y int) {
	for j := -1; j < 10; j++ {
		img.SetRGBA(x, y+j, tBlack)
		img.SetRGBA(x+1, y+j, tBlack)
	}
}

func TestNativeSegmentSecond(t *testing.T) {
	build := func(open, closing, degree bool) (*image.RGBA, image.Rectangle) {
		img, box := cellImage(tWhite)
		y := box.Min.Y + 7
		x := drawText(img, box.Min.X+2, y, "5", tBlack) + 5 // "0.248 um" stand-in, then a gap
		if open {
			paren(img, x, y)
		}
		x = drawText(img, x+4, y, "421", tBlack)
		if closing {
			paren(img, x, y)
		}
		if degree {
			tFill(img, image.Rect(x, y, x+3, y+3), tBlack)
		}
		return img, box.Inset(3)
	}
	img, rect := build(true, true, false)
	gs := segmentSecond(img, rect)
	if len(gs) != 3 || gs[0].key() != segmentNumberKey('4') {
		t.Fatalf("second = %v", tKeys(gs))
	}
	if gs = segmentSecond(build(true, false, true)); len(gs) != 3 {
		t.Errorf("degree sign terminator: %v", tKeys(gs))
	}
	if gs = segmentSecond(build(true, false, false)); gs != nil {
		t.Errorf("truncated window (no closing mark) accepted: %v", tKeys(gs))
	}
	if gs = segmentSecond(build(false, true, false)); gs != nil {
		t.Errorf("missing opening parenthesis accepted: %v", tKeys(gs))
	}
}

func segmentNumberKey(ch rune) string { return glyphOf(ch).key() }

// ---- real documents (gitignored frontend/data; absent -> skip) ----

type realCell struct {
	folder, eye string
	page        int
	rect        image.Rectangle // 450-dpi render
	printed     string
}

// Ground-truth-backed cells (printed values verified against the documents):
// red text, red fill + white ink touching the border, rect left of the box,
// yellow fill, plain text.
var realCells = []realCell{
	{"ademildo", "OD", 8, image.Rect(807, 2014, 905, 2070), "42.1"},
	{"ademildo", "OD", 8, image.Rect(1538, 3433, 1636, 3489), "0.022"},
	{"ademildo", "OD", 8, image.Rect(1211, 3505, 1393, 3561), "1.08"},
	{"ademildo", "OD", 8, image.Rect(322, 3136, 441, 3204), "585"},
	{"ademildo", "OD", 8, image.Rect(444, 3605, 587, 3687), "3.26"},
	{"ademildo", "OD", 8, image.Rect(1232, 3361, 1330, 3417), "23"},
	{"ademildo", "OD", 9, image.Rect(2565, 3676, 2943, 3730), "1"},
	{"daniela-nogueira", "OD", 6, image.Rect(1232, 3361, 1330, 3417), "42"},
}

func realPDF(t *testing.T, folder, eye string) []byte {
	b, err := os.ReadFile(realPath(t, folder, eye))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func realPath(t *testing.T, folder, eye string) string {
	files, _ := filepath.Glob("../../../../../frontend/data/" + folder + "/PENTACAM*.pdf")
	tok := regexp.MustCompile("[^A-Z0-9]+")
	for _, f := range files {
		got := "OS"
		for _, w := range tok.Split(strings.ToUpper(filepath.Base(f)), -1) {
			if w == "OD" {
				got = "OD"
			}
		}
		if got == eye {
			return f
		}
	}
	t.Skip("no real Pentacam documents available")
	return ""
}

func TestNativeRealDocumentCells(t *testing.T) {
	type doc struct {
		data   []byte
		native map[int]image.Image
		render map[int]image.Image
		m      nativeMap
	}
	docs := map[string]*doc{}
	for _, c := range realCells {
		d := docs[c.folder+c.eye]
		if d == nil {
			data := realPDF(t, c.folder, c.eye)
			native, err := nativePages(data)
			if err != nil {
				t.Fatal(err)
			}
			d = &doc{data: data, native: native, render: map[int]image.Image{}}
			docs[c.folder+c.eye] = d
		}
		if d.render[c.page] == nil {
			png, err := pdf.RenderPageAtDPI(context.Background(), d.data, c.page, 450)
			if err != nil {
				t.Skip("pdftoppm unavailable: ", err)
			}
			img, _, err := image.Decode(bytes.NewReader(png))
			if err != nil {
				t.Fatal(err)
			}
			d.render[c.page] = img
			m, ok := newNativeMap(d.data, d.native[c.page], 450, img.Bounds().Dy())
			if !ok {
				t.Fatal("no nominal map")
			}
			d.m = m
		}
		nat, render := d.native[c.page], d.render[c.page]
		nr, mad, ok := d.m.register(render, nat, c.rect)
		t.Logf("%s %s: MAD %.2f native rect %v", c.folder, c.printed, mad, nr)
		if !ok {
			t.Errorf("%s: registration MAD %.2f ok=%v", c.printed, mad, ok)
			continue
		}
		// negative control: the same rect against another page must be rejected
		if _, other, ok := d.m.register(render, d.native[c.page-1], c.rect); ok {
			t.Logf("%s: control page matched anyway (MAD %.2f): flat area", c.printed, other)
		}
		gs := segmentNumber(nat, nr)
		if len(gs) != len(c.printed) {
			t.Errorf("%s: %d glyphs", c.printed, len(gs))
			continue
		}
		// equal printed digits must give equal keys, different digits different keys
		seen := map[rune]string{}
		for i, ch := range c.printed {
			if ch == '.' {
				if gs[i].W != 1 {
					t.Errorf("%s: '.' is %dx%d", c.printed, gs[i].W, gs[i].H)
				}
				continue
			}
			if k, dup := seen[ch]; dup && k != gs[i].key() {
				t.Errorf("%s: digit %c has two keys", c.printed, ch)
			}
			seen[ch] = gs[i].key()
		}
	}
}

// nestedFloat reads exam[path...] as a float64 (absent / nil -> ok=false).
func nestedFloat(exam map[string]any, path ...string) (float64, bool) {
	var cur any = exam
	for _, p := range path {
		m, _ := cur.(map[string]any)
		cur = m[p]
	}
	v, ok := cur.(float64)
	return v, ok
}

// The cells Tesseract alone could not read (lost glyph, colour fill, border
// glued to the digit) must now come out of the embedded pixel glyphs. Expected
// values are printed ground truth; an optional cell may be absent but never
// another value.
func TestNativeTargetRecoveries(t *testing.T) {
	type want struct {
		folder, eye string
		path        []string
		value       float64
		optional    bool
	}
	pupil, vertex := []string{"pachymetry", "pupil_center_um"}, []string{"pachymetry", "vertex_normal_um"}
	z5, z5axis := []string{"corneal_rings", "zernike", "5mm", "z31_coma"}, []string{"corneal_rings", "zernike", "5mm", "z31_coma_axis_deg"}
	cases := []want{
		{"ademildo", "OD", pupil, 585, false},
		{"maria-gorete", "OS", pupil, 545, false},
		{"rolmey", "OD", pupil, 534, false},
		{"rolmey", "OD", vertex, 534, false},
		{"ademildo", "OD", z5, 0.248, false},
		{"ademildo", "OD", z5axis, 281, false},
		{"ademildo", "OS", z5, 0.078, true},
		{"ademildo", "OS", z5axis, 288, true},
		{"creso-aversa", "OS", []string{"topometric_indices_8mm", "isv"}, 17, true},
	}
	exams := map[string]map[string]any{}
	for _, c := range cases {
		id := c.folder + c.eye
		if exams[id] == nil {
			exams[id] = cachedExtractPDF(t, realPath(t, c.folder, c.eye))
		}
		got, present := nestedFloat(exams[id], c.path...)
		switch {
		case !present && c.optional:
		case !present || got != c.value:
			t.Errorf("%s %s %v = %v (present=%v), want %v", c.folder, c.eye, c.path, got, present, c.value)
		}
	}
}

func TestProvenanceHintsNeverOverrideAnchoredOrConflicts(t *testing.T) {
	prov := map[string]FieldHint{
		"k1_d":  {Spec: "k1_d", Status: "valid", Strategy: "region_text"},
		"k2_d":  {Spec: "k2_d", Status: "valid", Strategy: "cell_ocr"},
		"isv":   {Spec: "isv", Status: "valid", Strategy: "region_text"},
		"alien": {Status: "valid", Strategy: "region_text"}, // unknown spec: skipped
	}
	anchored := map[string]anchoredValue{"k1_d": {Hint: FieldHint{Spec: "k1_d"}}}
	conflicts := []FieldHint{{Spec: "k2_d", Status: "conflicting"}}
	got := provenanceHints(prov, anchored, conflicts)
	if len(got) != 1 || got[0].Spec != "isv" || got[0].Strategy != "region_text" {
		t.Fatalf("hints = %+v", got)
	}
}

func TestVetoedKeyAcceptsOnlyTheNativeReading(t *testing.T) {
	veto := vetoInfo{ocr: 42.7, native: 42.1}
	if s, ok := resolvedStrategy(42.7, veto); ok {
		t.Errorf("the vetoed value was re-introduced (%q)", s)
	}
	if s, ok := resolvedStrategy(41.9, veto); ok {
		t.Errorf("a third value was accepted (%q)", s)
	}
	if s, ok := resolvedStrategy(42.1, veto); !ok || s != "label_anchor+consistency+native" {
		t.Errorf("native reading = %q %v", s, ok)
	}
}
