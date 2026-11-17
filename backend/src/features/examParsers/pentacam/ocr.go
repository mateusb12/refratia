package pentacam

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"refratia/backend/src/shared/profiling"
	"strings"

	"refratia/backend/src/shared/ocr"
)

type pentacamOCRRegion struct {
	X, Y, W, H int
}

type decodedPagesKey struct{}

// withDecodedPageImages lets the region readers crop from the pages extractPDF has
// already decoded instead of decoding the 450 dpi PNG again for every region.
// A page is matched by identity (its PNG's backing array): the readers get the
// very slices held in extractPDF's pages map. Anything else is decoded as before.
func withDecodedPageImages(ctx context.Context, pages map[int][]byte, images map[int]image.Image) context.Context {
	decodedImagesByPageData := make(map[*byte]image.Image, len(images))
	for page, data := range pages {
		if decoded := images[page]; decoded != nil && len(data) > 0 {
			decodedImagesByPageData[&data[0]] = decoded
		}
	}
	return context.WithValue(ctx, decodedPagesKey{}, decodedImagesByPageData)
}

func findDecodedPageImage(ctx context.Context, pageData []byte) image.Image {
	if decodedImagesByPageData, _ := ctx.Value(decodedPagesKey{}).(map[*byte]image.Image); len(pageData) > 0 {
		return decodedImagesByPageData[&pageData[0]]
	}
	return nil
}

func cropPentacamFixedRegionToPNG(ctx context.Context, data []byte, ocrRegion pentacamOCRRegion) ([]byte, error) {
	span := profiling.Start(context.Background(), "preprocess.pentacam_region") // ctx-less: the summary attributes it by time
	defer span.End(nil)
	img := findDecodedPageImage(ctx, data)
	span.Set("reused", img != nil)
	if img == nil {
		var err error
		if img, _, err = image.Decode(bytes.NewReader(data)); err != nil {
			return nil, err
		}
	}

	b := img.Bounds()
	sx := float64(b.Dx()) / 2480.0
	sy := float64(b.Dy()) / 3508.0

	r := image.Rect(
		int(float64(ocrRegion.X)*sx),
		int(float64(ocrRegion.Y)*sy),
		int(float64(ocrRegion.X+ocrRegion.W)*sx),
		int(float64(ocrRegion.Y+ocrRegion.H)*sy),
	).Intersect(b)

	if r.Empty() {
		return nil, fmt.Errorf("ocrRegion vazio")
	}

	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), img, r.Min, draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func runTesseractTextPSM(executionContext context.Context, pngData []byte, psm int) (string, error) {
	text, recognizeError := ocr.Recognize(executionContext, pngData, psm, "", false)
	if recognizeError != nil {
		return "", fmt.Errorf("tesseract OCR region: %w", recognizeError)
	}
	return strings.TrimSpace(text), nil
}

func extractOCRTextsFromRegion(ctx context.Context, page []byte, ocrRegion pentacamOCRRegion) ([]string, error) {
	image, err := cropPentacamFixedRegionToPNG(ctx, page, ocrRegion)
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, 3)
	for _, psm := range []int{6, 11, 12} {
		text, err := runTesseractTextPSM(ctx, image, psm)
		if err == nil && strings.TrimSpace(text) != "" {
			result = append(result, text)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("OCR focado vazio")
	}
	return result, nil
}
