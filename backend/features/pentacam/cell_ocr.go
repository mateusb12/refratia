package pentacam

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"refratia/backend/shared/ocr"
	"regexp"
	"strconv"
	"strings"
)

type pentacamCellSpec struct {
	Key       string
	Page      int
	OCRRegion pentacamOCRRegion
	Decimal   bool
	Min       float64
	Max       float64
}

func fillPentacamFocusedCells(
	ctx context.Context,
	pages map[int][]byte,
	values map[string]any,
	layout pentacamPageLayout,
	anchored map[string]anchoredValue,
) {
	specs := []pentacamCellSpec{
		{"k1_d", 4, pentacamOCRRegion{535, 1270, 180, 70}, true, 20, 70},
		// The Anéis Corneanos page repeats the labeled K1 value at a stable
		// location. It prevents the anterior-cornea map from selecting the
		// nearby 43.3 map annotation as K1.
		{"k1_d", layout.z31, pentacamOCRRegion{500, 1800, 300, 140}, true, 20, 70},
		{"k2_d", 4, pentacamOCRRegion{535, 1335, 180, 70}, true, 20, 70},
		{"km_d", 4, pentacamOCRRegion{535, 1385, 155, 85}, true, 20, 70},
		{"astigmatism_d", 4, pentacamOCRRegion{535, 1455, 180, 70}, true, 0, 15},
		{"thinnest_um", layout.pachymetry, pentacamOCRRegion{1840, 1540, 200, 60}, false, 300, 1000},

		// Página 9 — Topométrico / Estadiamento KC.
		// Célula K Máx. (ant.), sem incluir as bordas horizontais.
		// Fixtures reais Rolmey: OD = 44.2 D; OS = 45.5 D.
		{"kmax_d", layout.topometric, pentacamOCRRegion{335, 2250, 155, 55}, true, 20, 70},

		// Índices: use células um pouco mais largas para tolerar o OCR do
		// container sem misturar os valores vizinhos.
		{"isv", layout.topometric, pentacamOCRRegion{790, 2200, 170, 100}, false, 0, 200},
		{"iha", layout.topometric, pentacamOCRRegion{1000, 2200, 140, 100}, true, 0, 20},
		{"iva", layout.topometric, pentacamOCRRegion{842, 2293, 52, 31}, true, 0, 5},
		{"ihd", layout.topometric, pentacamOCRRegion{1000, 2260, 160, 90}, true, 0, 1},
		{"ki", layout.topometric, pentacamOCRRegion{820, 2310, 180, 90}, true, 0, 5},

		{"z40_6mm_um", layout.z40, pentacamOCRRegion{2298, 2095, 62, 45}, true, -5, 5},
		{"acd_internal_mm", layout.acd, pentacamOCRRegion{2250, 2400, 250, 150}, true, 0.5, 10},

		{"art_max", layout.badd, pentacamOCRRegion{1648, 1490, 42, 31}, false, 100, 1000},
		{"z31_5mm_um", layout.z31, pentacamOCRRegion{1000, 1545, 180, 90}, true, -5, 5},
	}

	for _, spec := range specs {
		if _, byAnchor := anchored[spec.Key]; byAnchor {
			continue // read by its label; the fixed box is only a fallback
		}
		_, hasExtractedDocumentValue := values[spec.Key]
		focusedValueShouldOverride := spec.Key == "k1_d" ||
			spec.Key == "thinnest_um" || spec.Key == "iha" ||
			spec.Key == "z40_6mm_um" || spec.Key == "z31_5mm_um"
		if hasExtractedDocumentValue && !focusedValueShouldOverride {
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
	ocrRegion, err := extractOCRRegionPNG(page, spec.OCRRegion)
	if err != nil {
		return 0, false
	}

	ocrInputImages := [][]byte{ocrRegion}

	// O ponto decimal dessas células é muito pequeno.
	// Binarizamos com vários limiares, mas SEM reconstruir dígitos.
	for _, threshold := range []uint8{130, 160, 190, 220} {
		v, err := pentacamBinaryUpscale(ocrRegion, 3, threshold)
		if err == nil {
			ocrInputImages = append(ocrInputImages, v)
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
	if spec.Key == "iha" || spec.Key == "ihd" {
		psms = []int{11}
	}
	if spec.Key == "ki" {
		psms = []int{6, 11}
	}

	for _, img := range ocrInputImages {
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

			// O Tesseract pode remover apenas o separador decimal do IHA:
			// "5.5"/"7.9" aparecem como "55"/"79". Só aceitamos
			// exatamente dois dígitos nesta célula e dividimos por dez.
			if !ok && spec.Key == "iha" {
				compact := regexp.MustCompile(`^\s*([0-9]{2})\s*$`).FindStringSubmatch(text)
				if len(compact) == 2 {
					compactValue, parseErr := strconv.ParseFloat(compact[1], 64)
					value = compactValue / 10
					ok = parseErr == nil && value >= spec.Min && value <= spec.Max
				}
			}

			// Z40 is a small isolated decimal cell. Some OCR runtimes
			// return 0173 instead of 0.173 after removing the decimal.
			if !ok && spec.Key == "z40_6mm_um" {
				digits := regexp.MustCompile(`^\s*0?([0-9]{3})\s*$`).FindStringSubmatch(text)
				if len(digits) == 2 {
					compactValue, parseErr := strconv.ParseFloat("0."+digits[1], 64)
					ok = parseErr == nil && compactValue >= spec.Min && compactValue <= spec.Max
					value = compactValue
				}
			}

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

	// Um único palpite normalmente não basta. However, these three cells are
	// isolated, field-specific crops; on different Leptonica versions the
	// same printed value can survive only one threshold/PSM combination.
	// Accept that unambiguous result for those crops instead of losing a
	// clearly visible value solely because the OCR runtime changed.
	allowSingleVote := len(values) == 1 && (spec.Key == "thinnest_um" ||
		spec.Key == "iha" || spec.Key == "z40_6mm_um")
	if (bestVotes < 2 && !allowSingleVote) || tie {
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

// pentacamBinaryUpscale upscales a cell and binarizes it. Grey is the darkest
// colour channel, so red/blue (out-of-norm) text keeps its contrast, and a
// cell whose background is darker than its text (white digits on a red box)
// is inverted so the digits are always dark on light.
func pentacamBinaryUpscale(
	data []byte,
	scale int,
	threshold uint8,
) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	gray := ocr.PrepareGray(src, src.Bounds(), scale, 0)

	// Polarity: most pixels are background. A dark majority means light text
	// on a dark fill.
	var dark int
	for _, v := range gray.Pix {
		if v < 128 {
			dark++
		}
	}
	invert := dark*2 > len(gray.Pix)

	for i, v := range gray.Pix {
		if invert {
			v = 255 - v
		}
		if v < threshold {
			gray.Pix[i] = 0
		} else {
			gray.Pix[i] = 255
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, gray); err != nil {
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
