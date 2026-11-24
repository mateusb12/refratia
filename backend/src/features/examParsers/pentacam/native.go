package pentacam

import (
	"bytes"
	"compress/zlib"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"math"
	"refratia/backend/src/features/fileIdentification/extractionfields"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Native-pixel reader for image-only Pentacam PDFs: every page is a single
// embedded JPEG (/DCTDecode). Pixel-exact bitmap glyphs can be read from it
// without OCR. readAnchoredNative (anchors.go) uses it to fill cells Tesseract
// could not read and to veto values the glyphs contradict; every function
// fails closed (nil / ok=false) when the structure is not the expected one.

// nativePages decodes every embedded JPEG in file order; page N (1-based) is
// the N-th stream. The caller must check len(result) == page count.
// ponytail: raw byte scan, not a PDF parser; it assumes one JPEG per page
// (image-only layout) and the caller's page-count check is the guard.
func nativePages(pdf []byte) (map[int]image.Image, error) {
	pages := map[int]image.Image{}
	for _, body := range streamBodies(pdf) {
		if !bytes.HasPrefix(body, []byte{0xFF, 0xD8, 0xFF}) {
			continue
		}
		img, err := jpeg.Decode(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		pages[len(pages)+1] = img
	}
	if len(pages) == 0 {
		return nil, errors.New("no embedded JPEG")
	}
	return pages, nil
}

// streamBodies returns, for every "stream" keyword, the bytes after it
// (up to the end of the file; decoders stop by themselves).
func streamBodies(pdf []byte) [][]byte {
	var out [][]byte
	for pos := 0; ; {
		i := bytes.Index(pdf[pos:], []byte("stream"))
		if i < 0 {
			return out
		}
		at := pos + i
		pos = at + len("stream")
		if bytes.HasSuffix(pdf[:at], []byte("end")) {
			continue
		}
		body := pdf[pos:]
		body = bytes.TrimPrefix(bytes.TrimPrefix(body, []byte("\r")), []byte("\n"))
		out = append(out, body)
	}
}

// nativeMap maps render coordinates (any dpi) to native image pixels:
// native = (render - origin) / scale, per axis, in edge coordinates.
type nativeMap struct{ ox, oy, sx, sy float64 }

var cmRe = regexp.MustCompile(`(-?[\d.]+) 0 0 (-?[\d.]+) (-?[\d.]+) (-?[\d.]+) cm`)

// newNativeMap derives the nominal placement of the page image from the
// content-stream cm chain (scale+translate only) of the first page, then
// poppler's integer snapping (floor / ceil of the image box) at renderDPI.
// ponytail: the page-1 CTM is assumed identical on every page; the per-cell
// registration in register() is the guard.
func newNativeMap(pdf []byte, native image.Image, renderDPI, renderH int) (nativeMap, bool) {
	var ms [][][]byte
	for _, body := range streamBodies(pdf) {
		zr, err := zlib.NewReader(bytes.NewReader(body))
		if err != nil {
			continue
		}
		content, _ := io.ReadAll(io.LimitReader(zr, 1<<16))
		if ms = cmRe.FindAllSubmatch(content, 3); len(ms) == 3 {
			break
		}
	}
	if len(ms) < 3 {
		return nativeMap{}, false
	}
	var m [3][4]float64
	for i := range ms {
		for j := range 4 {
			v, err := strconv.ParseFloat(string(ms[i][j+1]), 64)
			if err != nil {
				return nativeMap{}, false
			}
			m[i][j] = v
		}
	}
	nw, nh := float64(native.Bounds().Dx()), float64(native.Bounds().Dy())
	if math.Abs(math.Abs(m[2][0])-nw) > 0.5 || math.Abs(math.Abs(m[2][1])-nh) > 0.5 {
		return nativeMap{}, false
	}
	// unit-square corners through cm3, cm2, cm1 (PDF concatenation order).
	apply := func(x, y float64) (float64, float64) {
		for i := 2; i >= 0; i-- {
			x, y = m[i][0]*x+m[i][2], m[i][1]*y+m[i][3]
		}
		return x, y
	}
	xa, ya := apply(0, 0)
	xb, yb := apply(1, 1)
	k := float64(renderDPI) / 72
	pageH := float64(renderH) / k
	x0, x1 := math.Floor(math.Min(xa, xb)*k), math.Ceil(math.Max(xa, xb)*k)
	y0, y1 := math.Floor((pageH-math.Max(ya, yb))*k), math.Ceil((pageH-math.Min(ya, yb))*k)
	return nativeMap{x0, y0, (x1 - x0) / nw, (y1 - y0) / nh}, true
}

// toNative maps a render rect to a (rounded) native rect.
func (m nativeMap) toNative(r image.Rectangle) image.Rectangle {
	return image.Rect(
		int(math.Round((float64(r.Min.X)-m.ox)/m.sx)), int(math.Round((float64(r.Min.Y)-m.oy)/m.sy)),
		int(math.Round((float64(r.Max.X)-m.ox)/m.sx)), int(math.Round((float64(r.Max.Y)-m.oy)/m.sy)))
}

func gray(img image.Image, x, y int) float64 {
	r, g, b, _ := img.At(x, y).RGBA()
	return (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 257
}

const (
	registerRadius = 4 // render px; poppler's integer snapping leaves 1-2 px
	// registerMaxMAD: see TestNativeRegistrationDistribution measurements.
	registerMaxMAD = 4.0
)

// register maps render rect r (render = the 450-dpi page) to the native rect
// of the same content. The nominal origin is refined by the render-pixel
// shift within +-registerRadius that minimises the mean absolute luminance
// difference between the render and a bilinear resampling of the native
// image (poppler upscales bilinearly; native strokes are 1 px, so a 1-2
// render px phase error alone gives a large difference).
// ok=false when even the best shift is a poor match (or leaves the image).
func (m nativeMap) register(render, native image.Image, r image.Rectangle) (image.Rectangle, float64, bool) {
	fm, best, _, ok := m.fit(render, native, r, registerRadius)
	return fm.toNative(r), best, ok && best <= registerMaxMAD
}

// fit is register's search: the shifted map, its MAD, the spread between the
// worst and best shift (small = a flat cell that cannot pin the shift down),
// and false when the rect is unusable.
func (m nativeMap) fit(render, native image.Image, r image.Rectangle, radius int) (nativeMap, float64, float64, bool) {
	r = r.Intersect(render.Bounds())
	n := m.toNative(r).Inset(-registerRadius)
	if r.Dx() < 8 || r.Dy() < 8 || !n.In(native.Bounds()) {
		return m, 0, 0, false
	}
	nat := make([]float64, n.Dx()*n.Dy())
	for y := n.Min.Y; y < n.Max.Y; y++ {
		for x := n.Min.X; x < n.Max.X; x++ {
			nat[(y-n.Min.Y)*n.Dx()+x-n.Min.X] = gray(native, x, y)
		}
	}
	ren := make([]float64, r.Dx()*r.Dy())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			ren[(y-r.Min.Y)*r.Dx()+x-r.Min.X] = gray(render, x, y)
		}
	}
	at := func(x, y int) float64 {
		return nat[(min(max(y, n.Min.Y), n.Max.Y-1)-n.Min.Y)*n.Dx()+min(max(x, n.Min.X), n.Max.X-1)-n.Min.X]
	}
	best, worst, bs := math.Inf(1), 0.0, image.Point{}
	for ay := -radius; ay <= radius; ay++ {
		for ax := -radius; ax <= radius; ax++ {
			var sum float64
			for y := r.Min.Y; y < r.Max.Y; y++ {
				v := (float64(y)+0.5-m.oy-float64(ay))/m.sy - 0.5
				v0 := int(math.Floor(v))
				fy := v - float64(v0)
				for x := r.Min.X; x < r.Max.X; x++ {
					u := (float64(x)+0.5-m.ox-float64(ax))/m.sx - 0.5
					u0 := int(math.Floor(u))
					fx := u - float64(u0)
					p := (at(u0, v0)*(1-fx)+at(u0+1, v0)*fx)*(1-fy) + (at(u0, v0+1)*(1-fx)+at(u0+1, v0+1)*fx)*fy
					sum += math.Abs(p - ren[(y-r.Min.Y)*r.Dx()+x-r.Min.X])
				}
			}
			mad := sum / float64(len(ren))
			worst = max(worst, mad)
			if mad < best {
				best, bs = mad, image.Pt(ax, ay)
			}
		}
	}
	m.ox, m.oy = m.ox+float64(bs.X), m.oy+float64(bs.Y)
	return m, best, worst - best, true
}

// glyph is one column-separated ink group of a number. Box is its tight ink
// bounding box (native px); Bits is Box's columns over the number's shared
// row band (all glyphs of one number have the same H), row-major.
type glyph struct {
	Box  image.Rectangle
	W, H int
	Bits []bool
}

// key is the canonical template key: "WxH:" followed by 0/1 per pixel.
func (g glyph) key() string {
	b := make([]byte, 0, len(g.Bits)+8)
	b = strconv.AppendInt(b, int64(g.W), 10)
	b = append(b, 'x')
	b = strconv.AppendInt(b, int64(g.H), 10)
	b = append(b, ':')
	for _, v := range g.Bits {
		if v {
			b = append(b, '1')
		} else {
			b = append(b, '0')
		}
	}
	return string(b)
}

const (
	cellMinLine  = 14 // shortest dark line taken as a cell border (digits are <= 6 wide)
	maxGlyphW    = 7  // wider groups are touching glyphs
	maxGlyphH    = 12
	spaceGap     = 4 // blank columns that end the number (unit follows)
	spaceGapOne  = 5 // ... after a narrow '1' (wider right bearing)
	redFillRows  = 11
	cropPadX     = 36  // rows are searched this far left/right of the rect
	boxPadX      = 100 // the box may extend this far (digits can be outside the rect)
	cropPadY     = 15
	borderMargin = 2
	maxReachX    = 60 // the box border must start/end this close to the rect centre
	maxBoxH      = 60
	minInnerH    = 11 // a 9 px digit plus 1 px above and below
)

type cropPix struct {
	rect    image.Rectangle
	r, g, b []int
}

func (c cropPix) at(x, y int) (int, int, int) {
	i := (y-c.rect.Min.Y)*c.rect.Dx() + x - c.rect.Min.X
	return c.r[i], c.g[i], c.b[i]
}

func (c cropPix) lum(x, y int) int {
	r, g, b := c.at(x, y)
	return (299*r + 587*g + 114*b) / 1000
}

func longestRun(n int, f func(int) bool) int {
	best, cur := 0, 0
	for i := 0; i < n; i++ {
		if f(i) {
			cur++
			best = max(best, cur)
		} else {
			cur = 0
		}
	}
	return best
}

// tight is the ink extent of one column-separated group (inclusive).
type tight struct{ x0, x1, y0, y1 int }

func (t tight) h() int { return t.y1 - t.y0 + 1 }
func (t tight) w() int { return t.x1 - t.x0 + 1 }

// cell is a located value cell: its ink groups left to right.
type cell struct {
	ts      []tight
	ink     func(x, y int) bool
	box     image.Rectangle
	clipped bool // the box runs to the edge of the analysed window
}

// segmentCell finds the value cell around rect (native px): the box whose
// borders are nearest the rect centre (or the red fill band), and its ink
// groups. nil when there is no clean box.
// ponytail: heuristics tuned on 14 documents (box = long dark lines around
// the rect centre; red fill = tall red band, white ink).
func segmentCell(img image.Image, rect image.Rectangle) *cell {
	area := image.Rect(rect.Min.X-boxPadX, rect.Min.Y-cropPadY, rect.Max.X+boxPadX, rect.Max.Y+cropPadY).Intersect(img.Bounds())
	if area.Empty() {
		return nil
	}
	c := cropPix{rect: area}
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			c.r, c.g, c.b = append(c.r, int(r>>8)), append(c.g, int(g>>8)), append(c.b, int(b>>8))
		}
	}
	cx, cy := (rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2
	if !image.Pt(cx, cy).In(area) {
		return nil
	}
	red := func(x, y int) bool { r, g, b := c.at(x, y); return r > 170 && g < 100 && b < 100 }
	// red fill: the red band on the centre row nearest the centre (white digits
	// split it into pieces, so pieces <= 8 px apart are one band)
	fx0, fx1, fd := 0, 0, maxReachX+1
	for x := area.Min.X; x < area.Max.X; {
		if !red(x, cy) {
			x++
			continue
		}
		x1 := x + 1
		for e := x1; e < area.Max.X && e-x1 <= 8; e++ {
			if red(e, cy) {
				x1 = e + 1
			}
		}
		if d := max(x-cx, cx-(x1-1), 0); d < fd && x1-x >= 20 {
			fx0, fx1, fd = x, x1, d
		}
		x = x1
	}
	redRow := func(y int) int {
		n := 0
		for x := fx0; x < fx1; x++ {
			if red(x, y) {
				n++
			}
		}
		return n
	}

	var box image.Rectangle // interior to scan, native coords
	var isInk func(x, y int) bool
	r0, r1 := cy, cy
	if fx1 > fx0 {
		for r0 > area.Min.Y && redRow(r0-1) >= 10 {
			r0--
		}
		for r1 < area.Max.Y-1 && redRow(r1+1) >= 10 {
			r1++
		}
	}
	if fx1 > fx0 && r1-r0+1 >= redFillRows { // red fill, white ink
		bb := image.Rectangle{Min: image.Pt(fx1, r0), Max: image.Pt(fx0, r1+1)}
		for y := r0; y <= r1; y++ {
			for x := fx0; x < fx1; x++ {
				if red(x, y) {
					bb.Min.X, bb.Max.X = min(bb.Min.X, x), max(bb.Max.X, x+1)
				}
			}
		}
		box = image.Rect(bb.Min.X+1, bb.Min.Y+1, bb.Max.X-1, bb.Max.Y-1)
		isInk = func(x, y int) bool {
			r, g, b := c.at(x, y)
			return c.lum(x, y) > 170 && min(r, g, b) > 150
		}
	} else { // dark ink (also yellow fill and red-on-white digits)
		line := func(x, y int) bool {
			r, g, b := c.at(x, y)
			return c.lum(x, y) < 215 && !(r > 200 && g > 170 && b < 120)
		}
		// runs(y): maximal dark runs of the row, as [start,end) pairs
		runs := func(y int) (rs [][2]int) {
			for x := area.Min.X; x < area.Max.X; {
				if !line(x, y) {
					x++
					continue
				}
				x1 := x
				for x1 < area.Max.X && line(x1, y) {
					x1++
				}
				if x1-x >= cellMinLine {
					rs = append(rs, [2]int{x, x1})
				}
				x = x1
			}
			return rs
		}
		dist := func(r [2]int) int { return max(r[0]-cx, cx-(r[1]-1), 0) }
		// box = nearest row at/above the centre whose run (near the centre) has a
		// run under it, with room for a digit between the two (inner height >= minInnerH)
		top, l, rr, bot := -1, 0, 0, -1
		for y := cy; y >= area.Min.Y && top < 0; y-- {
			bestD := maxReachX + 1
			for _, r := range runs(y) {
				d := dist(r)
				if d >= bestD {
					continue
				}
				for b := max(y+minInnerH+1, cy+1); b < min(y+maxBoxH, area.Max.Y); b++ {
					for _, br := range runs(b) {
						if min(br[1], r[1])-max(br[0], r[0]) >= cellMinLine {
							top, l, rr, bot, bestD = y, r[0], r[1]-1, b, d
							b = area.Max.Y // first bottom border wins
							break
						}
					}
				}
				if bestD != d { // open-bottomed box: the left border's own length
					e := y + 1
					for e < min(y+maxBoxH, area.Max.Y) && (line(r[0], e) || line(r[0]+1, e)) {
						e++
					}
					if e-y-1 >= minInnerH {
						top, l, rr, bot, bestD = y, r[0], r[1]-1, e, d
					}
				}
			}
		}
		if top < 0 {
			return nil
		}
		box = image.Rect(l+borderMargin, top+1, rr+1-borderMargin, bot)
		isInk = func(x, y int) bool { return c.lum(x, y) < 128 }
	}
	if box.Dx() < 4 || box.Dy() < 4 {
		return nil
	}

	// column groups
	var groups [][2]int // inclusive x ranges
	for x := box.Min.X; x < box.Max.X; x++ {
		any := false
		for y := box.Min.Y; y < box.Max.Y; y++ {
			if isInk(x, y) {
				any = true
				break
			}
		}
		if any {
			if n := len(groups); n > 0 && groups[n-1][1] == x-1 {
				groups[n-1][1] = x
			} else {
				groups = append(groups, [2]int{x, x})
			}
		}
	}
	if len(groups) == 0 {
		return nil
	}
	var ts []tight
	for _, g := range groups {
		t := tight{g[0], g[1], box.Max.Y, box.Min.Y - 1}
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := g[0]; x <= g[1]; x++ {
				if isInk(x, y) {
					t.y0, t.y1 = min(t.y0, y), max(t.y1, y)
				}
			}
		}
		ts = append(ts, t)
	}
	return &cell{ts: ts, ink: isInk, box: box, clipped: box.Max.X+borderMargin >= area.Max.X}
}

// segmentNumber returns the glyphs of the number at the start of the cell (up
// to the first wide gap, i.e. before a unit). nil when the structure is not
// clean: no cell box, touching glyphs, inconsistent glyph height, a stray mark
// off the baseline, or a number cut by the edge of the window.
func segmentNumber(img image.Image, rect image.Rectangle) []glyph {
	c := segmentCell(img, rect)
	if c == nil {
		return nil
	}
	num := c.ts[:1]
	for i := 1; i < len(c.ts); i++ {
		a, b := c.ts[i-1], c.ts[i]
		thr := spaceGap
		if a.w() <= 4 && a.h() >= 3 {
			thr = spaceGapOne
		}
		if b.x0-a.x1-1 >= thr && b.h() >= 3 {
			break
		}
		num = c.ts[:i+1]
	}
	return c.glyphs(num)
}

// segmentSecond returns the digits inside the parentheses of a cell such as
// "0.248 um (281deg)". Complete only when a "(" precedes the digits and a ")"
// or degree sign follows them (a window that cuts the text gives nil).
func segmentSecond(img image.Image, rect image.Rectangle) []glyph {
	c := segmentCell(img, rect)
	if c == nil {
		return nil
	}
	paren := func(t tight) bool { return t.h() >= 10 && t.w() <= 3 }
	for i, open := range c.ts {
		if !paren(open) {
			continue
		}
		for j := i + 1; j < len(c.ts); j++ {
			t := c.ts[j]
			if paren(t) || t.h() <= 4 && t.y0 <= open.y0+3 { // ")" or degree sign
				if j == i+1 {
					return nil
				}
				return c.glyphs(c.ts[i+1 : j])
			}
		}
		return nil // no closing mark: the window cut the number
	}
	return nil
}

// glyphs checks a selection of groups (not touching, one height, dots on the
// baseline) and returns them as glyphs over their shared row band.
func (c *cell) glyphs(sel []tight) []glyph {
	if c.clipped && sel[len(sel)-1].x1 >= c.box.Max.X-1 {
		return nil
	}
	dy0, dy1 := -1, -1
	for _, t := range sel {
		if t.w() > maxGlyphW || t.h() > maxGlyphH {
			return nil
		}
		if t.h() >= 3 {
			if dy0 >= 0 && (t.y0 != dy0 || t.y1 != dy1) {
				return nil
			}
			dy0, dy1 = t.y0, t.y1
		}
	}
	if dy0 < 0 {
		return nil
	}
	out := make([]glyph, 0, len(sel))
	for _, t := range sel {
		if t.h() < 3 && (t.y1 != dy1 || t.w() > 2) { // '.' sits on the baseline
			return nil
		}
		g := glyph{Box: image.Rect(t.x0, t.y0, t.x1+1, t.y1+1), W: t.w(), H: dy1 - dy0 + 1}
		for y := dy0; y <= dy1; y++ {
			for x := t.x0; x <= t.x1; x++ {
				g.Bits = append(g.Bits, c.ink(x, y))
			}
		}
		out = append(out, g)
	}
	return out
}

// ---- per-document native context and glyph templates ----

// nativeGlyphs switches the native-glyph fill/veto on (tests flip it to
// compare before/after).
var nativeGlyphs = true

// nativeCtx holds the embedded page images of an image-only PDF and the
// render-to-native map, refined once per page (the snapping shift is the same
// for every cell of a page).
type nativeCtx struct {
	pages map[int]image.Image
	base  nativeMap
	mu    sync.Mutex
	shift map[int]nativeMap
}

// newNativeCtx returns nil unless the PDF is image-only: exactly one embedded
// JPEG per page and a readable placement. renders are the decoded 450-dpi pages.
func newNativeCtx(pdf []byte, pageCount int, renders map[int]image.Image) *nativeCtx {
	if !nativeGlyphs {
		return nil
	}
	pages, err := nativePages(pdf)
	if err != nil || len(pages) != pageCount {
		return nil
	}
	// The reference page is the lowest-numbered render (deterministic).
	ref := 0
	for page, render := range renders {
		if render != nil && pages[page] != nil && (ref == 0 || page < ref) {
			ref = page
		}
	}
	if ref == 0 {
		return nil
	}
	base, ok := newNativeMap(pdf, pages[ref], 450, renders[ref].Bounds().Dy())
	if !ok {
		return nil
	}
	return &nativeCtx{pages: pages, base: base, shift: map[int]nativeMap{}}
}

// mapRect maps a 450-dpi render rect of page to the native page image. The
// cached page shift is verified for this cell (MAD) and refitted when it fails.
func (n *nativeCtx) mapRect(page int, render image.Image, r image.Rectangle) (image.Image, image.Rectangle, bool) {
	native := n.pages[page]
	if native == nil || render == nil {
		return nil, image.Rectangle{}, false
	}
	n.mu.Lock()
	cached, have := n.shift[page]
	n.mu.Unlock()
	if have {
		if fm, mad, _, ok := cached.fit(render, native, r, 0); ok && mad <= registerMaxMAD {
			return native, fm.toNative(r), true
		}
	}
	fm, mad, spread, ok := n.base.fit(render, native, r, registerRadius)
	if !ok || mad > registerMaxMAD {
		return nil, image.Rectangle{}, false
	}
	if spread >= 8 { // a cell with contrast pins the shift down
		n.mu.Lock()
		n.shift[page] = fm
		n.mu.Unlock()
	}
	return native, fm.toNative(r), true
}

func isDot(g glyph) bool { return g.Box.Dy() < 3 }

// glyphBank collects (glyph key -> printed char) observations from cells whose
// value is already trusted, by cell, so a read can leave its own cell out.
type glyphBank struct {
	mu  sync.Mutex
	obs []glyphObs
}

type glyphObs struct {
	cell, key string
	h         int
	ch        rune
}

// harvest records the digits of a segmented cell when its glyph count and
// decimal point position match the expected printed text exactly.
func (b *glyphBank) harvest(cell string, gs []glyph, expected string) bool {
	if len(gs) != len(expected) {
		return false
	}
	for i, ch := range expected {
		if ch != '.' && (ch < '0' || ch > '9') || (ch == '.') != isDot(gs[i]) {
			return false
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, ch := range expected {
		if ch != '.' {
			b.obs = append(b.obs, glyphObs{cell, gs[i].key(), gs[i].H, ch})
		}
	}
	return true
}

// read returns the printed text of gs from the bank leaving the cell exclude
// out. A key seen with two chars is unknown, and so is a char that has two
// different keys at the same glyph height (the bitmap<->char map must be a
// bijection per size class). outcome is "" or "unknown_digit" / "ambiguous".
func (b *glyphBank) read(gs []glyph, exclude string) (string, string) {
	b.mu.Lock()
	keyChars := map[string]map[rune]bool{}
	charKeys := map[[2]int]map[string]bool{}
	for _, o := range b.obs {
		if o.cell == exclude {
			continue
		}
		if keyChars[o.key] == nil {
			keyChars[o.key] = map[rune]bool{}
		}
		keyChars[o.key][o.ch] = true
		id := [2]int{o.h, int(o.ch)}
		if charKeys[id] == nil {
			charKeys[id] = map[string]bool{}
		}
		charKeys[id][o.key] = true
	}
	b.mu.Unlock()
	text := make([]rune, 0, len(gs))
	for _, g := range gs {
		if isDot(g) {
			text = append(text, '.')
			continue
		}
		chars := keyChars[g.key()]
		if len(chars) == 0 {
			return "", "unknown_digit"
		}
		if len(chars) > 1 {
			return "", "ambiguous"
		}
		for ch := range chars {
			if len(charKeys[[2]int{g.H, int(ch)}]) > 1 {
				return "", "ambiguous"
			}
			text = append(text, ch)
		}
	}
	return string(text), ""
}

// drop forgets a cell's observations (its value was vetoed: it is no longer trusted).
func (b *glyphBank) drop(cell string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	kept := b.obs[:0]
	for _, o := range b.obs {
		if o.cell != cell {
			kept = append(kept, o)
		}
	}
	b.obs = kept
}

// nativeCell is one anchored cell as the native pass sees it: the OCR outcome
// plus the glyphs segmented from the embedded image (nil = none / incomplete).
type nativeCell struct {
	id       string
	rule     extractionfields.Rule
	ok       bool    // Tesseract accepted a value
	strategy string  // strategy of that value
	value    float64 // that value
	mapped   bool    // the cell was located in the native image
	glyphs   []glyph
}

// nativeDecision is the outcome for one cell: action is "", "fill" (value is
// the native reading of a rejected cell) or "veto" (value is the native
// reading that contradicts the accepted OCR value).
type nativeDecision struct {
	outcome string
	action  string
	value   float64
}

// decideNative is the whole native pass over a batch of cells, free of any PDF
// or image: it harvests templates from Tesseract-accepted cells (count and dot
// position must match the printed text exactly), then reads every cell from
// the bank. A cell never vouches for itself (leave-one-cell-out), reads are
// taken against the same bank snapshot, and vetoed cells are dropped from the
// bank afterwards.
func decideNative(bank *glyphBank, cells []nativeCell) []nativeDecision {
	out := make([]nativeDecision, len(cells))
	harvested := make([]bool, len(cells))
	for i, c := range cells {
		// Only Tesseract-accepted cells teach templates: never native fills,
		// consistency resolutions or fallback readings.
		if c.ok && c.strategy == "label_anchor" && c.glyphs != nil {
			expected := strconv.FormatFloat(c.value, 'f', c.rule.Decimals, 64)
			if c.rule.Decimals < 0 {
				expected = strconv.FormatFloat(c.value, 'f', -1, 64)
			}
			harvested[i] = bank.harvest(c.id, c.glyphs, expected)
		}
	}
	var vetoed []string
	for i, c := range cells {
		d := &out[i]
		d.outcome = "noseg"
		switch {
		case !c.mapped:
		case c.glyphs == nil:
			d.outcome = "incomplete"
		case c.ok && c.strategy != "label_anchor":
			d.outcome = "skip"
		default:
			exclude := ""
			if c.ok {
				exclude = c.id // leave-one-cell-out
			}
			text, why := bank.read(c.glyphs, exclude)
			value, parses := 0.0, false
			switch {
			case why != "":
				d.outcome = why
				continue
			case c.rule.Decimals > 0 && !strings.Contains(text, "."):
				// normalizePrinted would insert the decimal point into a dotless
				// token: a native read must show the dot itself.
				d.outcome = "incomplete"
				continue
			}
			value, parses = normalizePrinted(text, c.rule)
			switch {
			case !parses:
				d.outcome = "badparse"
			case c.ok && value == c.value:
				d.outcome = "agree"
			case c.ok:
				d.outcome, d.action, d.value = "veto", "veto", value
				vetoed = append(vetoed, c.id)
			default:
				d.outcome, d.action, d.value = "fill", "fill", value
			}
		}
		if harvested[i] {
			d.outcome = "harvest+" + d.outcome
		}
	}
	for _, id := range vetoed {
		bank.drop(id)
	}
	return out
}
