package ocr

import "context"

// tesseractDefaultPSM is the CLI's page segmentation mode when no --psm is
// given (fully automatic, no OSD).
const tesseractDefaultPSM = 3

func RunTesseractTSV(
	ctx context.Context,
	png []byte,
) (string, error) {
	return Recognize(ctx, png, tesseractDefaultPSM, "", true)
}

func RunTesseractTSVWithPSM(
	ctx context.Context,
	png []byte,
	psm int,
) (string, error) {
	return Recognize(ctx, png, psm, "", true)
}
