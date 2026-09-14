package retinography

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"regexp"
	"strings"

	progressutil "refratia/backend/shared/progress"
)

const (
	roiWidthPercent  = 45
	roiHeightPercent = 40
	upscaleFactor    = 4
	tesseractPSM     = "6"
)

type Record struct {
	PatientID    string
	Eye          string
	Mode         string
	ExamDateTime string
}

type Result struct {
	Exam         map[string]any
	PatientID    string
	Eye          string
	Mode         string
	ExamDateTime string
	RawText      string
}

var (
	idPattern = regexp.MustCompile(
		`(?mi)^[ \t]*ID[ \t]+([^\r\n]+?)[ \t]*$`,
	)

	eyePattern = regexp.MustCompile(
		`(?mi)^[ \t]*Eye[ \t]+(OD|OS)[ \t]*$`,
	)

	modePattern = regexp.MustCompile(
		`(?mi)^[ \t]*Mode[ \t]+([^\r\n]+?)[ \t]*$`,
	)

	timePattern = regexp.MustCompile(
		`(?mi)^[ \t]*Time[ \t]+([0-9]{4}-[0-9]{2}-[0-9]{2}[ \t]+[0-9]{2}:[0-9]{2}:[0-9]{2})[ \t]*$`,
	)
)

func ParseText(text string) (Record, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")

	record := Record{
		PatientID:    firstGroup(idPattern, text),
		Eye:          strings.ToUpper(firstGroup(eyePattern, text)),
		Mode:         firstGroup(modePattern, text),
		ExamDateTime: firstGroup(timePattern, text),
	}

	record.PatientID = strings.Join(
		strings.Fields(record.PatientID),
		" ",
	)

	missing := make([]string, 0, 4)

	if record.PatientID == "" {
		missing = append(missing, "ID")
	}

	if record.Eye != "OD" && record.Eye != "OS" {
		missing = append(missing, "Eye")
	}

	if !strings.EqualFold(record.Mode, "Retina") {
		missing = append(missing, "Mode Retina")
	}

	if record.ExamDateTime == "" {
		missing = append(missing, "Time")
	}

	if len(missing) > 0 {
		return Record{}, fmt.Errorf(
			"retinografia: OCR incompleto: %s",
			strings.Join(missing, ", "),
		)
	}

	record.Mode = "Retina"

	return record, nil
}

func firstGroup(
	pattern *regexp.Regexp,
	text string,
) string {
	match := pattern.FindStringSubmatch(text)

	if len(match) != 2 {
		return ""
	}

	return strings.TrimSpace(match[1])
}

func Extract(
	ctx context.Context,
	data []byte,
) (Result, error) {
	progressutil.Report(
		ctx,
		5,
		"retinography_preprocess",
		"Decodificando retinografia",
	)

	src, _, err := image.Decode(
		bytes.NewReader(data),
	)
	if err != nil {
		return Result{}, fmt.Errorf(
			"retinografia: imagem inválida: %w",
			err,
		)
	}

	progressutil.Report(
		ctx,
		20,
		"retinography_preprocess",
		"Preparando região textual",
	)

	processed, err := preprocess(src)
	if err != nil {
		return Result{}, err
	}

	progressutil.Report(
		ctx,
		55,
		"retinography_ocr",
		"Executando OCR da legenda",
	)

	text, err := runTesseractText(
		ctx,
		processed,
	)
	if err != nil {
		return Result{}, err
	}

	progressutil.Report(
		ctx,
		80,
		"retinography_parse",
		"Interpretando ID, olho e horário",
	)

	record, err := ParseText(text)
	if err != nil {
		return Result{}, fmt.Errorf(
			"%w; OCR=%q",
			err,
			compactText(text),
		)
	}

	eyePayload := map[string]any{
		"patient_id":    record.PatientID,
		"eye":           record.Eye,
		"exam_datetime": record.ExamDateTime,
		"mode":          record.Mode,
	}

	exam := map[string]any{
		"id":             record.PatientID,
		"device_or_mode": record.Mode,
		"eyes": map[string]any{
			record.Eye: eyePayload,
		},
	}

	progressutil.Report(
		ctx,
		100,
		"retinography_parse",
		"Retinografia extraída localmente",
	)

	return Result{
		Exam:         exam,
		PatientID:    record.PatientID,
		Eye:          record.Eye,
		Mode:         record.Mode,
		ExamDateTime: record.ExamDateTime,
		RawText:      text,
	}, nil
}

func preprocess(
	src image.Image,
) ([]byte, error) {
	bounds := src.Bounds()

	width := bounds.Dx()
	height := bounds.Dy()

	if width <= 0 || height <= 0 {
		return nil, errors.New(
			"retinografia: dimensões inválidas",
		)
	}

	cropWidth := width * roiWidthPercent / 100
	cropHeight := height * roiHeightPercent / 100

	left := bounds.Min.X + width - cropWidth

	roi := image.Rect(
		left,
		bounds.Min.Y,
		left+cropWidth,
		bounds.Min.Y+cropHeight,
	).Intersect(bounds)

	if roi.Empty() {
		return nil, errors.New(
			"retinografia: ROI textual vazia",
		)
	}

	base := image.NewGray(
		image.Rect(
			0,
			0,
			roi.Dx(),
			roi.Dy(),
		),
	)

	minValue := uint8(255)
	maxValue := uint8(0)

	for y := 0; y < roi.Dy(); y++ {
		for x := 0; x < roi.Dx(); x++ {
			gray := color.GrayModel.Convert(
				src.At(
					roi.Min.X+x,
					roi.Min.Y+y,
				),
			).(color.Gray).Y

			base.SetGray(
				x,
				y,
				color.Gray{Y: gray},
			)

			if gray < minValue {
				minValue = gray
			}

			if gray > maxValue {
				maxValue = gray
			}
		}
	}

	if maxValue <= minValue {
		return nil, errors.New(
			"retinografia: ROI sem contraste",
		)
	}

	// grayscale -> auto-level -> negate
	for y := 0; y < base.Bounds().Dy(); y++ {
		for x := 0; x < base.Bounds().Dx(); x++ {
			raw := base.GrayAt(x, y).Y

			leveled := uint8(
				uint32(raw-minValue) *
					255 /
					uint32(maxValue-minValue),
			)

			base.SetGray(
				x,
				y,
				color.Gray{
					Y: 255 - leveled,
				},
			)
		}
	}

	// Upscale 4x.
	//
	// O experimento externo usou Lanczos. Aqui usamos interpolação
	// bilinear em Go puro para não introduzir ImageMagick no runtime.
	scaled := resizeGrayBilinear(
		base,
		upscaleFactor,
	)

	var output bytes.Buffer

	if err := png.Encode(
		&output,
		scaled,
	); err != nil {
		return nil, fmt.Errorf(
			"retinografia: encode do preprocessing: %w",
			err,
		)
	}

	return output.Bytes(), nil
}

func resizeGrayBilinear(
	src *image.Gray,
	scale int,
) *image.Gray {
	if scale <= 1 {
		return src
	}

	srcWidth := src.Bounds().Dx()
	srcHeight := src.Bounds().Dy()

	dstWidth := srcWidth * scale
	dstHeight := srcHeight * scale

	dst := image.NewGray(
		image.Rect(
			0,
			0,
			dstWidth,
			dstHeight,
		),
	)

	for y := 0; y < dstHeight; y++ {
		srcY1000 := y * 1000 / scale

		y0 := srcY1000 / 1000
		fy := srcY1000 % 1000

		if y0 >= srcHeight {
			y0 = srcHeight - 1
		}

		y1 := y0 + 1

		if y1 >= srcHeight {
			y1 = srcHeight - 1
		}

		for x := 0; x < dstWidth; x++ {
			srcX1000 := x * 1000 / scale

			x0 := srcX1000 / 1000
			fx := srcX1000 % 1000

			if x0 >= srcWidth {
				x0 = srcWidth - 1
			}

			x1 := x0 + 1

			if x1 >= srcWidth {
				x1 = srcWidth - 1
			}

			v00 := int(src.GrayAt(x0, y0).Y)
			v10 := int(src.GrayAt(x1, y0).Y)
			v01 := int(src.GrayAt(x0, y1).Y)
			v11 := int(src.GrayAt(x1, y1).Y)

			top :=
				v00*(1000-fx) +
					v10*fx

			bottom :=
				v01*(1000-fx) +
					v11*fx

			value :=
				(top*(1000-fy) +
					bottom*fy) /
					1000000

			if value < 0 {
				value = 0
			}

			if value > 255 {
				value = 255
			}

			dst.SetGray(
				x,
				y,
				color.Gray{
					Y: uint8(value),
				},
			)
		}
	}

	return dst
}

func runTesseractText(
	ctx context.Context,
	pngData []byte,
) (string, error) {
	input, err := os.CreateTemp(
		"",
		"refratia-retinography-*.png",
	)
	if err != nil {
		return "", err
	}

	path := input.Name()
	defer os.Remove(path)

	if _, err := input.Write(pngData); err != nil {
		_ = input.Close()
		return "", err
	}

	if err := input.Close(); err != nil {
		return "", err
	}

	command := exec.CommandContext(
		ctx,
		"tesseract",
		path,
		"stdout",
		"-l",
		"eng",
		"--psm",
		tesseractPSM,
	)

	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf(
			"retinografia: tesseract: %s: %w",
			strings.TrimSpace(string(output)),
			err,
		)
	}

	text := strings.TrimSpace(
		string(output),
	)

	if text == "" {
		return "", errors.New(
			"retinografia: OCR vazio",
		)
	}

	return text, nil
}

func compactText(
	value string,
) string {
	return strings.Join(
		strings.Fields(value),
		" ",
	)
}
