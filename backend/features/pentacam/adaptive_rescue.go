package pentacam

import (
	"context"
	"regexp"
	"strconv"

	pdfutil "refratia/backend/shared/pdf"
)

type pentacamAdaptiveCrop struct {
	X int
	Y int
	W int
	H int
}

type pentacamAdaptiveSpec struct {
	Page           int
	Crops          []pentacamAdaptiveCrop
	RequireDecimal bool
	MinValue       float64
	MaxValue       float64
}

func readPentacamAdaptiveNumber(
	ctx context.Context,
	pdf []byte,
	spec pentacamAdaptiveSpec,
) (float64, bool) {
	page, err := pdfutil.RenderPageAtDPI(
		ctx,
		pdf,
		spec.Page,
		300,
	)
	if err != nil {
		return 0, false
	}
	return readPentacamAdaptiveNumberFromPage(ctx, page, spec)
}

func readPentacamAdaptiveNumberFromPage(
	ctx context.Context,
	page []byte,
	spec pentacamAdaptiveSpec,
) (float64, bool) {
	if len(page) == 0 {
		return 0, false
	}

	type evidence struct {
		thresholds map[uint8]bool
		ocrRegions map[int]bool
	}

	candidates := map[float64]*evidence{}

	for regionIndex, c := range spec.Crops {
		cell, err := extractOCRRegionPNG(
			page,
			pentacamOCRRegion{
				X: c.X,
				Y: c.Y,
				W: c.W,
				H: c.H,
			},
		)
		if err != nil {
			continue
		}

		for _, threshold := range []uint8{
			140, 150, 160, 170, 180, 190,
		} {
			img, err := pentacamBinaryUpscale(
				cell,
				4,
				threshold,
			)
			if err != nil {
				continue
			}

			// Um bitmap só pode gerar um voto.
			observed := map[float64]bool{}

			for _, psm := range []int{7, 8, 13} {
				text, err := pentacamNumericOCR(
					ctx,
					img,
					psm,
				)
				if err != nil {
					continue
				}

				value, ok := pentacamParseCellNumber(
					text,
					spec.RequireDecimal,
					spec.MinValue,
					spec.MaxValue,
				)
				if ok {
					observed[value] = true
				}
			}

			// PSMs discordaram ou nenhum conseguiu ler:
			// este bitmap não conta como evidência.
			if len(observed) != 1 {
				continue
			}

			for value := range observed {
				if candidates[value] == nil {
					candidates[value] = &evidence{
						thresholds: map[uint8]bool{},
						ocrRegions: map[int]bool{},
					}
				}

				candidates[value].thresholds[threshold] = true
				candidates[value].ocrRegions[regionIndex] = true
			}
		}
	}

	var winner float64
	found := false

	for value, evidence := range candidates {
		// Critério conservador:
		// mesmo decimal em >=3 thresholds
		// e >=2 janelas espaciais.
		if len(evidence.thresholds) < 3 ||
			len(evidence.ocrRegions) < 2 {
			continue
		}

		// Mais de um valor satisfazendo o critério:
		// ambíguo, portanto rejeitar.
		if found {
			return 0, false
		}

		winner = value
		found = true
	}

	return winner, found
}

func readPentacamK1Adaptive(
	ctx context.Context,
	pdf []byte,
) (float64, bool) {
	return readPentacamAdaptiveNumber(
		ctx,
		pdf,
		pentacamAdaptiveSpec{
			Page:           4,
			RequireDecimal: true,
			MinValue:       20,
			MaxValue:       80,
			Crops: []pentacamAdaptiveCrop{
				{X: 540, Y: 1275, W: 150, H: 60},
				{X: 525, Y: 1265, W: 175, H: 75},
				{X: 550, Y: 1265, W: 160, H: 75},
				{X: 515, Y: 1255, W: 200, H: 90},
			},
		},
	)
}

func readPentacamK1AdaptiveFromPage(
	ctx context.Context,
	page []byte,
) (float64, bool) {
	return readPentacamAdaptiveNumberFromPage(
		ctx,
		page,
		pentacamAdaptiveSpec{
			Page:           4,
			RequireDecimal: true,
			MinValue:       20,
			MaxValue:       80,
			Crops: []pentacamAdaptiveCrop{
				{X: 540, Y: 1275, W: 150, H: 60},
				{X: 525, Y: 1265, W: 175, H: 75},
				{X: 550, Y: 1265, W: 160, H: 75},
				{X: 515, Y: 1255, W: 200, H: 90},
			},
		},
	)
}

func readPentacamIHDFromRenderedPage(
	ctx context.Context,
	page []byte,
) (float64, bool) {
	// A célula vermelha do IHD é reconhecida com estabilidade em 450 DPI
	// quando a janela inclui a coluna numérica completa.
	texts, err := extractOCRTextsFromRegion(
		ctx,
		page,
		pentacamOCRRegion{X: 730, Y: 2160, W: 700, H: 330},
	)
	if err != nil {
		return 0, false
	}

	for _, text := range texts {
		if value, ok := parsePentacamFieldNumber(text, `\bihd\b`, true); ok && value <= 1 {
			return value, true
		}
	}

	return 0, false
}

func readPentacamThinnestPachymetryFromDedicatedRender(
	ctx context.Context,
	pdf []byte,
	pageNumber int,
) (float64, bool) {
	page, err := pdfutil.RenderPageAtDPI(ctx, pdf, pageNumber, 300)
	if err != nil {
		return 0, false
	}
	return readPentacamThinnestPachymetryFromRenderedPage(ctx, page)
}

func readPentacamThinnestPachymetryFromRenderedPage(
	ctx context.Context,
	page []byte,
) (float64, bool) {
	if len(page) == 0 {
		return 0, false
	}

	regions := []pentacamOCRRegion{
		{X: 1550, Y: 1400, W: 700, H: 350},
		{X: 1760, Y: 1460, W: 380, H: 180},
		{X: 1800, Y: 1500, W: 300, H: 120},
		{X: 1780, Y: 1460, W: 360, H: 220},
	}
	labelPattern := regexp.MustCompile(`(?is)(?:\bthinnest\W+pachy\b|\bponto\W+(?:mais\W+)?fino\b)(.{0,32})`)
	numberPattern := regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)
	candidates := map[float64]int{}

	for _, region := range regions {
		texts, err := extractOCRTextsFromRegion(ctx, page, region)
		if err != nil {
			continue
		}

		for _, text := range texts {
			matches := labelPattern.FindStringSubmatch(text)
			if len(matches) == 2 {
				text = matches[1]
			}

			for _, numberText := range numberPattern.FindAllString(text, -1) {
				value, parseErr := strconv.ParseFloat(numberText, 64)
				if parseErr == nil && value >= 300 && value <= 1000 {
					candidates[value]++
				}
			}
		}
	}

	var winner float64
	bestCount := 0
	for value, count := range candidates {
		if count > bestCount {
			winner = value
			bestCount = count
		}
	}

	return winner, bestCount >= 2
}

func readPentacamThinnestPachymetryAdaptiveFromPage(
	ctx context.Context,
	page []byte,
) (float64, bool) {
	return readPentacamAdaptiveNumberFromPage(
		ctx,
		page,
		pentacamAdaptiveSpec{
			RequireDecimal: false,
			MinValue:       300,
			MaxValue:       1000,
			Crops: []pentacamAdaptiveCrop{
				{X: 1800, Y: 1490, W: 280, H: 110},
				{X: 1820, Y: 1510, W: 240, H: 90},
				{X: 1780, Y: 1470, W: 320, H: 140},
				{X: 1840, Y: 1530, W: 220, H: 80},
			},
		},
	)
}

func readPentacamBADDFromDedicatedRender(
	ctx context.Context,
	pdf []byte,
	pageNumber int,
) (float64, bool) {
	page, err := pdfutil.RenderPageAtDPI(ctx, pdf, pageNumber, 300)
	if err != nil {
		return 0, false
	}
	return readPentacamBADDFromRenderedPage(ctx, page)
}

func readPentacamBADDFromRenderedPage(
	ctx context.Context,
	page []byte,
) (float64, bool) {
	if len(page) == 0 {
		return 0, false
	}

	texts, err := extractOCRTextsFromRegion(
		ctx,
		page,
		pentacamOCRRegion{X: 1550, Y: 2350, W: 850, H: 350},
	)
	if err != nil {
		return 0, false
	}

	// Em algumas versões do relatório a coluna aparece apenas como "D:",
	// sem o rótulo BAD-D. É o valor da mesma célula do índice Belin/Ambrosio.
	labelPattern := regexp.MustCompile(`(?is)(?:\bbad\W*d\b|\bd\s*:)\s*(.{0,24})`)
	numberPattern := regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)

	for _, text := range texts {
		match := labelPattern.FindStringSubmatch(text)
		if len(match) != 2 {
			continue
		}

		for _, numberText := range numberPattern.FindAllString(match[1], -1) {
			value, parseErr := strconv.ParseFloat(numberText, 64)
			if parseErr == nil && value >= 0 && value <= 20 {
				return value, true
			}
		}
	}

	return 0, false
}

func readPentacamACDFromRenderedPage(
	ctx context.Context,
	page []byte,
) (float64, bool) {
	texts, err := extractOCRTextsFromRegion(
		ctx,
		page,
		pentacamOCRRegion{X: 2000, Y: 2350, W: 600, H: 300},
	)
	if err != nil {
		return 0, false
	}

	for _, text := range texts {
		if value, ok := parsePentacamFieldNumber(
			text,
			`\bprof\W*cam\W*ant\W*int\b`,
			true,
		); ok && value >= 1 && value <= 10 {
			return value, true
		}
	}

	return 0, false
}

func readPentacamACDFromPage(
	ctx context.Context,
	page []byte,
) (float64, bool) {
	if len(page) == 0 {
		return 0, false
	}

	texts, err := extractOCRTextsFromRegion(
		ctx,
		page,
		pentacamOCRRegion{X: 2000, Y: 2350, W: 600, H: 300},
	)
	if err != nil {
		return 0, false
	}

	for _, text := range texts {
		if value, ok := parsePentacamFieldNumber(
			text,
			`\bprof\W*cam\W*ant\W*int\b`,
			true,
		); ok && value >= 1 && value <= 10 {
			return value, true
		}
	}

	return 0, false
}

func readPentacamACDAdaptive(
	ctx context.Context,
	pdf []byte,
	pageNumber int,
) (float64, bool) {
	// Cataract Pre-OP — Prof.Cam.Ant.(Int.).
	//
	// A linha Ext. fica imediatamente acima.
	// Estas janelas contêm apenas a célula numérica Int.
	return readPentacamAdaptiveNumber(
		ctx,
		pdf,
		pentacamAdaptiveSpec{
			Page:           pageNumber,
			RequireDecimal: true,
			MinValue:       1,
			MaxValue:       10,
			Crops: []pentacamAdaptiveCrop{
				{X: 2290, Y: 2460, W: 135, H: 55},
				{X: 2298, Y: 2466, W: 120, H: 46},
				{X: 2285, Y: 2456, W: 145, H: 62},
				{X: 2300, Y: 2463, W: 115, H: 50},
			},
		},
	)
}

func readPentacamACDAdaptiveFromPage(
	ctx context.Context,
	page []byte,
) (float64, bool) {
	return readPentacamAdaptiveNumberFromPage(
		ctx,
		page,
		pentacamAdaptiveSpec{
			Page:           10,
			RequireDecimal: true,
			MinValue:       1,
			MaxValue:       10,
			Crops: []pentacamAdaptiveCrop{
				{X: 2290, Y: 2460, W: 135, H: 55},
				{X: 2298, Y: 2466, W: 120, H: 46},
				{X: 2285, Y: 2456, W: 145, H: 62},
				{X: 2300, Y: 2463, W: 115, H: 50},
			},
		},
	)
}
