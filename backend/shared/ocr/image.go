package ocr

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// DecodeImage decodes PNG/JPEG bytes.
func DecodeImage(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// PrepareGray crops r out of src, upscales it by an integer factor with
// bilinear interpolation and converts it to grey using the *darkest channel*:
// grey = min(R,G,B). Unlike a luminance conversion this keeps saturated
// (red / blue) text as dark as black text, so out-of-norm coloured values
// survive binarization. Pass threshold>0 to binarize at that grey level.
func PrepareGray(src image.Image, r image.Rectangle, scale int, threshold uint8) *image.Gray {
	return PrepareGrayMode(src, r, scale, threshold, false)
}

// PrepareGrayMode is PrepareGray with an optional luminance conversion. The
// darkest channel makes saturated yellow look black; luminance keeps dark text
// readable on yellow/white fills (and is what PrepareGray's red/blue-safe mode
// gives up), so cells are read in both modes.
func PrepareGrayMode(src image.Image, r image.Rectangle, scale int, threshold uint8, luma bool) *image.Gray {
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

// EncodePNG encodes any image as PNG.
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RunTesseractText runs tesseract with the given PSM and an optional character
// whitelist and returns the plain text.
func RunTesseractText(ctx context.Context, pngData []byte, psm int, whitelist string) (string, error) {
	f, err := os.CreateTemp("", "refratia-ocr-*.png")
	if err != nil {
		return "", err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err := f.Write(pngData); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	args := []string{path, "stdout", "--psm", strconv.Itoa(psm)}
	if whitelist != "" {
		args = append(args, "-c", "tessedit_char_whitelist="+whitelist)
	}
	out, err := exec.CommandContext(ctx, "tesseract", args...).Output()
	if err != nil {
		return "", fmt.Errorf("tesseract: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// AutoContrast stretches the 1st..99th grey percentile of img to 0..255.
func AutoContrast(img *image.Gray) {
	var hist [256]int
	for _, v := range img.Pix {
		hist[v]++
	}
	total := len(img.Pix)
	lo, hi, acc := 0, 255, 0
	for i := 0; i < 256; i++ {
		acc += hist[i]
		if acc*100 >= total {
			lo = i
			break
		}
	}
	acc = 0
	for i := 255; i >= 0; i-- {
		acc += hist[i]
		if acc*100 >= total {
			hi = i
			break
		}
	}
	if hi <= lo {
		return
	}
	for i, v := range img.Pix {
		n := (int(v) - lo) * 255 / (hi - lo)
		if n < 0 {
			n = 0
		}
		if n > 255 {
			n = 255
		}
		img.Pix[i] = uint8(n)
	}
}

// Binarize returns a copy of img with pixels below percent% of 255 set to
// black and the rest white.
func Binarize(img *image.Gray, percent int) *image.Gray {
	out := image.NewGray(img.Bounds())
	cut := uint8(percent * 255 / 100)
	for i, v := range img.Pix {
		if v < cut {
			out.Pix[i] = 0
		} else {
			out.Pix[i] = 255
		}
	}
	return out
}

// ThickenDark grows dark strokes by one pixel (3x3 minimum filter), which
// closes the slash of slashed-zero fonts so it reads as a clean glyph.
func ThickenDark(img *image.Gray) *image.Gray {
	b := img.Bounds()
	out := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			m := uint8(255)
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					xx, yy := x+dx, y+dy
					if xx < b.Min.X || yy < b.Min.Y || xx >= b.Max.X || yy >= b.Max.Y {
						continue
					}
					if v := img.GrayAt(xx, yy).Y; v < m {
						m = v
					}
				}
			}
			out.SetGray(x, y, color.Gray{Y: m})
		}
	}
	return out
}
