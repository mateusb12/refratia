package pentacam

import (
	"context"
	"image"
	"regexp"
	"strings"

	"refratia/backend/shared/ocr"
	pdfutil "refratia/backend/shared/pdf"
)

// Screen identifies one printed Pentacam screen. Reports differ in which
// screens they contain and in the page each sits on (9, 11 and 12 page
// reports exist), so a field is always located through its screen, never
// through a page number.
type Screen string

const (
	ScreenGeneral     Screen = "general"
	ScreenScheimpflug Screen = "scheimpflug"
	ScreenRefractive  Screen = "refractive_maps" // "4 Mapas Refrativos": anterior cornea summary
	ScreenAberrometry Screen = "aberrometry"
	ScreenPachymetry  Screen = "pachymetry"
	ScreenTopometric  Screen = "topometric"
	ScreenCataract    Screen = "cataract_preop"
	ScreenBelin       Screen = "belin_ambrosio"
	ScreenRings       Screen = "corneal_rings"
)

// Stems are matched case-insensitively against the words printed after the
// "PENTACAM" header. Order matters only when a title could match two stems.
var screenTitles = []struct {
	screen  Screen
	pattern *regexp.Regexp
}{
	{ScreenRings, regexp.MustCompile(`(?i)an[eéê]is|corneano|ring`)},
	{ScreenBelin, regexp.MustCompile(`(?i)belin|ambr|ectasia`)},
	{ScreenCataract, regexp.MustCompile(`(?i)cataract|pre-?op`)},
	{ScreenTopometric, regexp.MustCompile(`(?i)topom|estadiamento`)},
	{ScreenPachymetry, regexp.MustCompile(`(?i)paquim|pachym`)},
	{ScreenAberrometry, regexp.MustCompile(`(?i)aberr`)},
	{ScreenRefractive, regexp.MustCompile(`(?i)refrativ`)},
	{ScreenScheimpflug, regexp.MustCompile(`(?i)scheimpf`)},
	{ScreenGeneral, regexp.MustCompile(`(?i)geral|overview`)},
}

// ScreenForTitle maps the printed title text to a screen ("" when unknown).
func ScreenForTitle(title string) Screen {
	for _, candidate := range screenTitles {
		if candidate.pattern.MatchString(title) {
			return candidate.screen
		}
	}
	return ""
}

// titleAfterHeader returns the words printed after the "PENTACAM" header word
// on the page's title row, or "" when the page has no such header.
func titleAfterHeader(words []ocr.Word, tolerance int) string {
	for _, row := range ocr.GroupRows(words, tolerance) {
		for index, word := range row.Words {
			if strings.EqualFold(strings.Trim(word.Text, " .,:;-–—"), "pentacam") {
				parts := make([]string, 0, len(row.Words))
				for _, following := range row.Words[index+1:] {
					parts = append(parts, following.Text)
				}
				return strings.Join(parts, " ")
			}
		}
	}
	return ""
}

// DetectScreens renders every page once at low resolution, reads the title
// band and returns screen -> 1-based page. A screen that is absent from the
// report is simply not in the map. When a screen appears on several pages
// (aberrometry) the first page wins.
func DetectScreens(ctx context.Context, data []byte, pageCount int) (map[Screen]int, error) {
	pages, err := pdfutil.RenderAllPagesAtDPI(ctx, data, pageCount, 100)
	if err != nil {
		return nil, err
	}

	screens := map[Screen]int{}
	for index, png := range pages {
		img, err := ocr.DecodeImage(png)
		if err != nil {
			continue
		}
		b := img.Bounds()
		// The header sits in the upper half; the lower half only holds data.
		band := image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+b.Dy()*45/100)
		gray := ocr.PrepareGray(img, band, 2, 0)
		encoded, err := ocr.EncodePNG(gray)
		if err != nil {
			continue
		}
		tsv, err := ocr.RunTesseractTSVWithPSM(ctx, encoded, 6)
		if err != nil {
			continue
		}
		words, _, _ := ocr.ParseTSVWords(tsv)
		title := titleAfterHeader(words, 12)
		screen := ScreenForTitle(title)
		if screen == "" {
			continue
		}
		if _, seen := screens[screen]; !seen {
			screens[screen] = index + 1
		}
	}
	return screens, nil
}
