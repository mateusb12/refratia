package pentacam

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func encode(t *testing.T, img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Digits must come out dark on light for black-on-white, red-on-white and
// white-on-red cells alike.
func TestBinaryUpscalePolarityAndColour(t *testing.T) {
	cases := map[string][2]color.RGBA{ // background, text
		"black on white": {{255, 255, 255, 255}, {0, 0, 0, 255}},
		"red on white":   {{255, 255, 255, 255}, {210, 20, 20, 255}},
		"white on red":   {{200, 20, 20, 255}, {255, 255, 255, 255}},
	}
	for name, c := range cases {
		img := image.NewRGBA(image.Rect(0, 0, 20, 20))
		for y := 0; y < 20; y++ {
			for x := 0; x < 20; x++ {
				img.Set(x, y, c[0])
			}
		}
		for y := 8; y < 12; y++ { // 4x4 "glyph"
			for x := 8; x < 12; x++ {
				img.Set(x, y, c[1])
			}
		}
		out, err := pentacamBinaryUpscale(encode(t, img), 1, 130)
		if err != nil {
			t.Fatal(err)
		}
		bin, _ := png.Decode(bytes.NewReader(out))
		if r, _, _, _ := bin.At(10, 10).RGBA(); r != 0 {
			t.Errorf("%s: glyph pixel not dark", name)
		}
		if r, _, _, _ := bin.At(1, 1).RGBA(); r == 0 {
			t.Errorf("%s: background pixel not light", name)
		}
	}
}

var black = color.Gray{Y: 0}

func newGray(w, h int) *image.Gray {
	g := image.NewGray(image.Rect(0, 0, w, h))
	for i := range g.Pix {
		g.Pix[i] = 255
	}
	return g
}
