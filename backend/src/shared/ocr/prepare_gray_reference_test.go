package ocr

import (
	"bytes"
	"image"
	"image/color"
	"math/rand"
	"testing"
)

// referencePrepareGrayMode is PrepareGrayMode before the per-pixel buffer
// optimisation; the optimised version must produce the same bytes.
func referencePrepareGrayMode(src image.Image, r image.Rectangle, scale int, threshold uint8, luma bool) *image.Gray {
	r = r.Intersect(src.Bounds())
	if r.Empty() || scale < 1 {
		return image.NewGray(image.Rect(0, 0, 1, 1))
	}
	w, h := r.Dx(), r.Dy()
	dst := image.NewGray(image.Rect(0, 0, w*scale, h*scale))
	at := func(x, y int) (float64, float64, float64) {
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		if x >= w {
			x = w - 1
		}
		if y >= h {
			y = h - 1
		}
		cr, cg, cb, _ := src.At(r.Min.X+x, r.Min.Y+y).RGBA()
		return float64(cr >> 8), float64(cg >> 8), float64(cb >> 8)
	}
	for y := 0; y < h*scale; y++ {
		fy := (float64(y)+0.5)/float64(scale) - 0.5
		y0 := int(fy)
		if fy < 0 {
			y0 = -1
		}
		ty := fy - float64(y0)
		for x := 0; x < w*scale; x++ {
			fx := (float64(x)+0.5)/float64(scale) - 0.5
			x0 := int(fx)
			if fx < 0 {
				x0 = -1
			}
			tx := fx - float64(x0)
			var ch [3]float64
			r00, g00, b00 := at(x0, y0)
			r10, g10, b10 := at(x0+1, y0)
			r01, g01, b01 := at(x0, y0+1)
			r11, g11, b11 := at(x0+1, y0+1)
			ch[0] = (r00*(1-tx)+r10*tx)*(1-ty) + (r01*(1-tx)+r11*tx)*ty
			ch[1] = (g00*(1-tx)+g10*tx)*(1-ty) + (g01*(1-tx)+g11*tx)*ty
			ch[2] = (b00*(1-tx)+b10*tx)*(1-ty) + (b01*(1-tx)+b11*tx)*ty
			g := ch[0]
			if luma {
				g = 0.299*ch[0] + 0.587*ch[1] + 0.114*ch[2]
			} else {
				if ch[1] < g {
					g = ch[1]
				}
				if ch[2] < g {
					g = ch[2]
				}
			}
			v := uint8(g + 0.5)
			if threshold > 0 {
				if v < threshold {
					v = 0
				} else {
					v = 255
				}
			}
			dst.SetGray(x, y, color.Gray{Y: v})
		}
	}
	return dst
}

func TestPrepareGrayModeMatchesReference(t *testing.T) {
	random := rand.New(rand.NewSource(1))
	bounds := image.Rect(3, 5, 63, 45) // non-zero origin like a SubImage
	rgba := image.NewRGBA(bounds)
	nrgba := image.NewNRGBA(bounds)
	gray := image.NewGray(bounds)
	ycbcr := image.NewYCbCr(bounds, image.YCbCrSubsampleRatio420)
	random.Read(rgba.Pix)
	random.Read(nrgba.Pix)
	random.Read(gray.Pix)
	random.Read(ycbcr.Y)
	random.Read(ycbcr.Cb)
	random.Read(ycbcr.Cr)
	sources := map[string]image.Image{
		"rgba": rgba, "nrgba": nrgba, "gray": gray, "ycbcr": ycbcr,
		"rgba-subimage": rgba.SubImage(image.Rect(10, 12, 50, 40)),
	}
	regions := []image.Rectangle{
		image.Rect(3, 5, 63, 45),   // whole image
		image.Rect(15, 9, 41, 22),  // inside
		image.Rect(0, 0, 20, 20),   // clipped at the origin
		image.Rect(50, 30, 90, 80), // clipped at the far edge
		image.Rect(20, 20, 21, 21), // single pixel
	}
	for name, source := range sources {
		for _, region := range regions {
			for _, scale := range []int{1, 2, 3, 4} {
				for _, threshold := range []uint8{0, 128, 200} {
					for _, luma := range []bool{false, true} {
						want := referencePrepareGrayMode(source, region, scale, threshold, luma)
						got := PrepareGrayMode(source, region, scale, threshold, luma)
						if got.Rect != want.Rect || !bytes.Equal(got.Pix, want.Pix) {
							t.Fatalf("%s region %v scale %d threshold %d luma %v differs", name, region, scale, threshold, luma)
						}
					}
				}
			}
		}
	}
}
