package pentacam

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	pdfutil "refratia/backend/shared/pdf"

	progressutil "refratia/backend/shared/progress"
)

type pentacamFocusedField struct {
	Key     string
	Label   string
	Decimal bool
}

func ExtractPDF(ctx context.Context, data []byte) (map[string]any, error) {
	pages := map[int][]byte{}

	progressutil.Report(
		ctx,
		0,
		"pentacam",
		"Preparando páginas do Pentacam",
	)

	renderPages := []int{4, 6, 7, 8, 9}

	for index, page := range renderPages {
		progressutil.Report(
			ctx,
			index*25/len(renderPages),
			"pentacam",
			"Renderizando páginas do Pentacam",
		)

		image, err := pdfutil.RenderPageAtDPI(
			ctx,
			data,
			page,
			450,
		)

		if err != nil {
			return nil, err
		}

		pages[page] = image
	}

	progressutil.Report(
		ctx,
		25,
		"pentacam",
		"Páginas renderizadas",
	)

	values := map[string]any{}

	read := func(page int, crop referenceCrop, fields []pentacamFocusedField) error {
		variants, err := focusedOCRVariants(ctx, pages[page], crop)
		if err != nil {
			return err
		}

		for _, field := range fields {
			for _, text := range variants {
				if _, exists := values[field.Key]; exists {
					break
				}

				if value, ok := pentacamFocusedNumber(
					text,
					field.Label,
					field.Decimal,
				); ok {
					values[field.Key] = value
				}
			}
		}
		return nil
	}

	progressutil.Report(
		ctx,
		30,
		"pentacam",
		"Lendo córnea anterior",
	)

	// Página 4 — córnea anterior.
	_ = read(4, referenceCrop{X: 250, Y: 1240, W: 500, H: 310}, []pentacamFocusedField{
		{"k1_d", `\b(?:k1|ki)\b`, true},
		{"k2_d", `\bk2\b`, true},
		{"km_d", `\bkm\b`, true},
		{"astigmatism_d", `\bastig\w*`, true},
	})

	progressutil.Report(
		ctx,
		36,
		"pentacam",
		"Lendo paquimetria",
	)

	// Página 6 — paquimetria.
	_ = read(6, referenceCrop{X: 740, Y: 1880, W: 400, H: 150}, []pentacamFocusedField{
		{"thinnest_um", `\bthinnest\W+pachy\b`, false},
	})

	progressutil.Report(
		ctx,
		42,
		"pentacam",
		"Lendo índices topométricos",
	)

	// Página 6 — índices topométricos.
	indexVariants, _ := focusedOCRVariants(
		ctx,
		pages[6],
		referenceCrop{X: 730, Y: 2160, W: 450, H: 330},
	)

	indexFields := []pentacamFocusedField{
		{"isv", `\b(?:isv|15v)\b`, false},
		{"iva", `\biva\b`, true},
		{"iha", `\biha\b`, true},
		{"ki", `\bk[i1l]\b`, true},
		{"cki", `\bcki\b`, true},
	}

	for _, field := range indexFields {
		for _, text := range indexVariants {
			if value, ok := pentacamFocusedNumber(text, field.Label, field.Decimal); ok {
				values[field.Key] = value
				break
			}
		}
	}

	if tkc, ok := pentacamFocusedTKC(indexVariants); ok {
		values["tkc"] = tkc
	}

	progressutil.Report(
		ctx,
		48,
		"pentacam",
		"Lendo BAD-D",
	)

	// Página 7 — BAD-D.
	_ = read(7, referenceCrop{X: 1600, Y: 2110, W: 330, H: 130}, []pentacamFocusedField{
		{"bad_d", `\bbad\W*d`, true},
	})

	progressutil.Report(
		ctx,
		52,
		"pentacam",
		"Lendo Z40 6 mm",
	)

	// Página 7 — Z40 6 mm.
	_ = read(7, referenceCrop{X: 1970, Y: 2050, W: 460, H: 150}, []pentacamFocusedField{
		{"z40_6mm_um", `\bz40\b`, true},
	})

	progressutil.Report(
		ctx,
		56,
		"pentacam",
		"Lendo profundidade da câmara anterior",
	)

	// Página 7 — profundidade da câmara anterior INTERNA.
	_ = read(7, referenceCrop{X: 1990, Y: 2370, W: 440, H: 180}, []pentacamFocusedField{
		{
			"acd_internal_mm",
			`(?:prof|pot)\w*\W*cam\w*\W*ant\w*\W*\(?int`,
			true,
		},
	})

	progressutil.Report(
		ctx,
		60,
		"pentacam",
		"Lendo ARTmax",
	)

	// Página 8 — ARTmax.
	_ = read(8, referenceCrop{X: 1490, Y: 1420, W: 300, H: 150}, []pentacamFocusedField{
		{"art_max", `\bartmax\b`, false},
	})

	progressutil.Report(
		ctx,
		64,
		"pentacam",
		"Lendo Z31 coma 5 mm",
	)

	// Página 9 — Z31 da coluna 5 mm.
	_ = read(9, referenceCrop{X: 970, Y: 1490, W: 290, H: 180}, []pentacamFocusedField{
		{"z31_5mm_um", `\bz31\W*\(?coma\w*\)?`, true},
	})

	// Segunda passada: célula numérica exata, sem inferência de decimal.
	progressutil.Report(
		ctx,
		68,
		"pentacam",
		"Lendo células numéricas",
	)

	fillPentacamFocusedCells(ctx, pages, values)

	progressutil.Report(
		ctx,
		75,
		"pentacam",
		"Células numéricas processadas",
	)

	// BAD-D rescue determinístico:
	// - três dígitos estáveis em múltiplos thresholds
	// - separador decimal comprovado geometricamente nos pixels
	// - nenhuma inferência de casa decimal
	progressutil.Report(
		ctx,
		78,
		"pentacam",
		"Validando BAD-D deterministicamente",
	)

	if current, exists := values["bad_d"]; !exists || current == nil {
		if value, ok := readPentacamBADDDeterministic(ctx, data); ok {
			values["bad_d"] = value
		}
	}

	// K1 rescue determinístico:
	// mesmo decimal em >=3 thresholds e >=2 crops.
	progressutil.Report(
		ctx,
		84,
		"pentacam",
		"Validando K1 deterministicamente",
	)

	if current, exists := values["k1_d"]; !exists || current == nil {
		if value, ok := readPentacamK1Deterministic(ctx, data); ok {
			values["k1_d"] = value
		}
	}

	progressutil.Report(
		ctx,
		90,
		"pentacam",
		"Validando variações de layout",
	)

	// Rescue determinístico tolerante a pequenas variações de layout.
	// Executa somente quando o caminho local principal ainda deixou gap.
	if current, exists := values["k1_d"]; !exists || current == nil {
		if value, ok := readPentacamK1Adaptive(ctx, data); ok {
			values["k1_d"] = value
		}
	}

	progressutil.Report(
		ctx,
		96,
		"pentacam",
		"Validando ACD interna",
	)

	if current, exists := values["acd_internal_mm"]; !exists || current == nil {
		if value, ok := readPentacamACDAdaptive(ctx, data); ok {
			values["acd_internal_mm"] = value
		}
	}

	progressutil.Report(
		ctx,
		100,
		"pentacam",
		"Pentacam validado",
	)

	return map[string]any{
		"anterior_cornea": map[string]any{
			"k1_d":          values["k1_d"],
			"k2_d":          values["k2_d"],
			"km_d":          values["km_d"],
			"astigmatism_d": values["astigmatism_d"],
		},
		"pachymetry": map[string]any{
			"thinnest_um": values["thinnest_um"],
		},
		"belin_ambrosio": map[string]any{
			"d":       values["bad_d"],
			"art_max": values["art_max"],
		},
		"topometric_indices_8mm": map[string]any{
			"isv": values["isv"],
			"iva": values["iva"],
			"iha": values["iha"],
			"ki":  values["ki"],
			"cki": values["cki"],
			"tkc": values["tkc"],
		},
		"corneal_rings": map[string]any{
			"zernike": map[string]any{
				"5mm": map[string]any{
					"z31_coma": values["z31_5mm_um"],
				},
			},
		},
		"anterior_segment": map[string]any{
			"internal_anterior_chamber_depth_mm": values["acd_internal_mm"],
		},
		"cataract_preop": map[string]any{
			"total_corneal_z40_6mm_um": values["z40_6mm_um"],
		},
	}, nil
}

func pentacamFocusedNumber(text, label string, decimal bool) (float64, bool) {
	text = normalizePentacamText(text)

	number := `[+-]?\d+(?:\.\d+)?`
	if decimal {
		number = `[+-]?\d+\.\d+`
	}

	re := regexp.MustCompile(
		`(?is)` + label + `[^0-9+\-]{0,40}(` + number + `)`,
	)
	match := re.FindStringSubmatch(text)
	if len(match) != 2 {
		return 0, false
	}

	value, err := strconv.ParseFloat(match[1], 64)
	return value, err == nil
}

func pentacamFocusedTKC(variants []string) (string, bool) {
	for _, raw := range variants {
		text := normalizePentacamText(raw)

		re := regexp.MustCompile(`(?is)\btkc\b(.{0,20})`)
		match := re.FindStringSubmatch(text)
		if len(match) != 2 {
			continue
		}

		tail := strings.TrimSpace(match[1])

		if strings.ContainsAny(tail, "-—–") {
			return "—", true
		}

		token := regexp.MustCompile(`[a-z]*\d+[a-z0-9.-]*`).FindString(tail)
		if token != "" {
			return strings.ToUpper(token), true
		}
	}

	return "", false
}
