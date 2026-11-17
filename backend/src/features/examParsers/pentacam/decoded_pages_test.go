package pentacam

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"testing"
)

// Cropping from the page extractPDF already decoded must give the same PNG,
// byte for byte, as decoding the page again; an unknown page still decodes.
func TestRegionCropFromDecodedPageMatchesDecodePath(t *testing.T) {
	random := rand.New(rand.NewSource(1))
	page := image.NewRGBA(image.Rect(0, 0, 1240, 1754)) // half-scale A4, the region math is relative
	for index := range page.Pix {
		page.Pix[index] = uint8(random.Intn(256))
	}
	for y := range 1754 {
		for x := range 1240 {
			page.Pix[page.PixOffset(x, y)+3] = 255
		}
	}
	page.Set(0, 0, color.RGBA{1, 2, 3, 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, page); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	reuse := withDecodedPageImages(context.Background(), map[int][]byte{4: data}, map[int]image.Image{4: decoded})

	for _, region := range []pentacamOCRRegion{{250, 1240, 500, 310}, {1745, 2137, 100, 62}, {2400, 3400, 200, 200}} {
		fresh, freshErr := cropPentacamFixedRegionToPNG(context.Background(), data, region)
		reused, reusedErr := cropPentacamFixedRegionToPNG(reuse, data, region)
		if (freshErr == nil) != (reusedErr == nil) || !bytes.Equal(fresh, reused) {
			t.Fatalf("region %+v: reused crop differs from decoded crop (errors %v / %v)", region, freshErr, reusedErr)
		}
	}
	// A copy of the bytes is not the page extractPDF holds: it is decoded again.
	other := append([]byte(nil), data...)
	if got, err := cropPentacamFixedRegionToPNG(reuse, other, pentacamOCRRegion{250, 1240, 500, 310}); err != nil || len(got) == 0 {
		t.Fatalf("fallback decode failed: %v", err)
	}
}
