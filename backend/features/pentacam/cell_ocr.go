package pentacam

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type pentacamCellSpec struct {
	Key     string
	Page    int
	Crop    referenceCrop
	Decimal bool
	Min     float64
	Max     float64
}

func fillPentacamFocusedCells(
	ctx context.Context,
	pages map[int][]byte,
	values map[string]any,
) {
	specs := []pentacamCellSpec{
		{"k1_d", 4, referenceCrop{535, 1270, 180, 70}, true, 20, 70},
		{"k2_d", 4, referenceCrop{535, 1335, 180, 70}, true, 20, 70},
		{"km_d", 4, referenceCrop{535, 1385, 155, 85}, true, 20, 70},
		{"astigmatism_d", 4, referenceCrop{535, 1455, 180, 70}, true, 0, 15},

		// Página 6 — Topométrico / Estadiamento KC.
		// Célula K Máx. (ant.), sem incluir as bordas horizontais.
		// Fixtures reais Rolmey: OD = 44.2 D; OS = 45.5 D.
		{"kmax_d", 6, referenceCrop{335, 2250, 155, 55}, true, 20, 70},

		{"iva", 6, referenceCrop{842, 2293, 52, 31}, true, 0, 5},
		{"ki", 6, referenceCrop{830, 2315, 150, 65}, true, 0, 5},

		{"bad_d", 7, referenceCrop{1765, 2120, 130, 100}, true, 0, 20},
		{"z40_6mm_um", 7, referenceCrop{2298, 2095, 62, 45}, true, -5, 5},
		{"acd_internal_mm", 7, referenceCrop{2280, 2415, 145, 120}, true, 0.5, 10},

		{"art_max", 8, referenceCrop{1648, 1490, 42, 31}, false, 100, 1000},
		{"z31_5mm_um", 9, referenceCrop{1000, 1545, 180, 90}, true, -5, 5},
	}

	for _, spec := range specs {
		if _, exists := values[spec.Key]; exists {
			continue
		}

		page := pages[spec.Page]
		if len(page) == 0 {
			continue
		}

		if value, ok := pentacamReadNumericCell(ctx, page, spec); ok {
			values[spec.Key] = value
		}
	}
}

func pentacamReadNumericCell(
	ctx context.Context,
	page []byte,
	spec pentacamCellSpec,
) (float64, bool) {
	crop, err := cropReferencePNG(page, spec.Crop)
	if err != nil {
		return 0, false
	}

	variants := [][]byte{crop}

	// O ponto decimal dessas células é muito pequeno.
	// Binarizamos com vários limiares, mas SEM reconstruir dígitos.
	for _, threshold := range []uint8{130, 160, 190, 220} {
		v, err := pentacamBinaryUpscale(crop, 3, threshold)
		if err == nil {
			variants = append(variants, v)
		}
	}

	votes := map[string]int{}
	values := map[string]float64{}

	psms := []int{7, 8, 13}

	// Kmax fica em uma célula horizontal curta.
	//
	// PSM 7 foi estável semanticamente em host e container.
	// PSM 13 confundiu 45.5 com 41.5 no ambiente Alpine,
	// portanto não participa da votação deste campo.
	if spec.Key == "kmax_d" {
		psms = []int{7}
	}

	for _, img := range variants {
		for _, psm := range psms {
			text, err := pentacamNumericOCR(ctx, img, psm)
			if err != nil {
				continue
			}

			value, ok := pentacamParseCellNumber(
				text,
				spec.Decimal,
				spec.Min,
				spec.Max,
			)

			// No container Alpine, o Tesseract/Leptonica pode
			// remover apenas o separador decimal do Kmax:
			//
			//   44.20 -> "4420"
			//   45.50 -> "4550"
			//
			// Só aplicamos esta recuperação ao Kmax e somente
			// quando o parser normal falhou.
			if !ok && spec.Key == "kmax_d" {
				value, ok = pentacamRecoverKMaxCompactDecimal(
					text,
					spec.Min,
					spec.Max,
				)
			}

			if !ok {
				continue
			}

			key := fmt.Sprintf("%.6f", value)
			votes[key]++
			values[key] = value
		}
	}

	bestKey := ""
	bestVotes := 0
	tie := false

	for key, count := range votes {
		if count > bestVotes {
			bestKey = key
			bestVotes = count
			tie = false
		} else if count == bestVotes {
			tie = true
		}
	}

	// Um único palpite não basta.
	// O mesmo valor precisa ser reconhecido por pelo menos 2 variantes.
	if bestVotes < 2 || tie {
		return 0, false
	}

	return values[bestKey], true
}

func pentacamNumericOCR(
	ctx context.Context,
	data []byte,
	psm int,
) (string, error) {
	f, err := os.CreateTemp("", "refratia-pentacam-cell-*.png")
	if err != nil {
		return "", err
	}

	path := f.Name()
	defer os.Remove(path)

	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(
		ctx,
		"tesseract",
		path,
		"stdout",
		"--psm",
		strconv.Itoa(psm),
		"-c",
		"tessedit_char_whitelist=0123456789.,+-",
	)

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

func pentacamParseCellNumber(
	text string,
	requireDecimal bool,
	minValue,
	maxValue float64,
) (float64, bool) {
	text = strings.ReplaceAll(text, ",", ".")

	pattern := `[+-]?\d+(?:\.\d+)?`
	if requireDecimal {
		pattern = `[+-]?\d+\.\d+`
	}

	match := regexp.MustCompile(pattern).FindString(text)
	if match == "" {
		return 0, false
	}

	value, err := strconv.ParseFloat(match, 64)
	if err != nil || value < minValue || value > maxValue {
		return 0, false
	}

	return value, true
}

func pentacamBinaryUpscale(
	data []byte,
	scale int,
	threshold uint8,
) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	b := src.Bounds()
	dst := image.NewGray(
		image.Rect(0, 0, b.Dx()*scale, b.Dy()*scale),
	)

	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			sx := b.Min.X + x/scale
			sy := b.Min.Y + y/scale

			g := color.GrayModel.Convert(src.At(sx, sy)).(color.Gray).Y

			if g < threshold {
				dst.SetGray(x, y, color.Gray{Y: 0})
			} else {
				dst.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// pentacamRecoverKMaxCompactDecimal recupera apenas o caso observado
// em que o OCR do Kmax remove o separador decimal.
//
// Exemplos válidos:
//
//	"4420" -> 44.20
//	"4550" -> 45.50
//
// Deliberadamente não recuperamos três dígitos:
//
//	"550" != 55.0
//
// porque esse padrão também apareceu como falso positivo em outros PSMs.
func pentacamRecoverKMaxCompactDecimal(
	text string,
	min float64,
	max float64,
) (float64, bool) {
	digits := make([]byte, 0, 4)

	for i := 0; i < len(text); i++ {
		ch := text[i]

		if ch < '0' || ch > '9' {
			continue
		}

		digits = append(
			digits,
			ch,
		)

		if len(digits) > 4 {
			return 0, false
		}
	}

	if len(digits) != 4 {
		return 0, false
	}

	number := 0

	for _, digit := range digits {
		number =
			number*10 +
				int(digit-'0')
	}

	value :=
		float64(number) / 100.0

	if value < min || value > max {
		return 0, false
	}

	return value, true
}
