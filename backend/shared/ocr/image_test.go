package ocr

import (
	"image"
	"image/color"
	"testing"
)

// Saturated coloured text must stay as dark as black text after the grey
// conversion, otherwise out-of-norm (red/blue) values disappear on binarize.
func TestPrepareGrayKeepsColouredTextDark(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 1))
	img.Set(0, 0, color.RGBA{255, 255, 255, 255})
	img.Set(1, 0, color.RGBA{220, 20, 20, 255}) // red
	img.Set(2, 0, color.RGBA{20, 20, 220, 255}) // blue
	g := PrepareGray(img, img.Bounds(), 1, 128)
	if g.GrayAt(0, 0).Y != 255 || g.GrayAt(1, 0).Y != 0 || g.GrayAt(2, 0).Y != 0 {
		t.Fatalf("unexpected binarization: %v", g.Pix)
	}
}
