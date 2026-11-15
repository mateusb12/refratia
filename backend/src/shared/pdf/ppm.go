package pdf

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
)

// pngFromPPM re-encodes pdftoppm's raw P6 output as a PNG. pdftoppm's own PNG
// writer spends ~90% of a 450 DPI render compressing (3.6 s vs 0.3 s per
// page); Go's BestSpeed deflate is far cheaper. Both are lossless, so the
// decoded pixels are the same as pdftoppm -png.
func pngFromPPM(ppmData []byte) ([]byte, error) {
	reader := bufio.NewReader(bytes.NewReader(ppmData))
	var magic string
	var width, height, maxValue int
	if _, scanError := fmt.Fscan(reader, &magic, &width, &height, &maxValue); scanError != nil {
		return nil, fmt.Errorf("ppm header: %w", scanError)
	}
	if magic != "P6" || maxValue != 255 || width <= 0 || height <= 0 {
		return nil, fmt.Errorf("ppm: unsupported %s %dx%d max %d", magic, width, height, maxValue)
	}
	if _, separatorError := reader.ReadByte(); separatorError != nil { // one whitespace byte ends the header
		return nil, fmt.Errorf("ppm header: %w", separatorError)
	}
	raster := make([]byte, width*height*3)
	if _, readError := io.ReadFull(reader, raster); readError != nil {
		return nil, fmt.Errorf("ppm raster: %w", readError)
	}

	page := image.NewRGBA(image.Rect(0, 0, width, height))
	for pixelIndex := 0; pixelIndex < width*height; pixelIndex++ {
		copy(page.Pix[pixelIndex*4:pixelIndex*4+3], raster[pixelIndex*3:pixelIndex*3+3])
		page.Pix[pixelIndex*4+3] = 255
	}
	var encoded bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if encodeError := encoder.Encode(&encoded, page); encodeError != nil {
		return nil, encodeError
	}
	return encoded.Bytes(), nil
}
