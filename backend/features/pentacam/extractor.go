package pentacam

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"refratia/backend/shared/parallelwork"
	pdfutil "refratia/backend/shared/pdf"

	progress "refratia/backend/shared/progress"
)

type pentacamDocumentField struct {
	Key     string
	Label   string
	Decimal bool
}

func ExtractPDF(executionContext context.Context, data []byte) (map[string]any, error) {
	return extractPDF(executionContext, data, true)
}

// extractPDF is kept behind the same parser interface so benchmarks can
// compare an experimental execution strategy without creating a second parser.
func extractPDF(executionContext context.Context, data []byte, parallelRender bool) (map[string]any, error) {
	pages := map[int][]byte{}

	progress.Report(
		executionContext,
		0,
		"pentacam",
		"Preparando páginas do Pentacam",
	)

	renderPages := []int{4, 8, 9, 10, 11, 12}

	if parallelRender {
		rendered, errorValue := parallelwork.Map(executionContext, renderPages, 3, func(executionContext context.Context, page int) ([]byte, error) {
			taskIdentifier, taskLabel := pentacamRenderTask(page)
			progress.ReportTask(
				executionContext,
				taskIdentifier,
				taskLabel,
				progress.TaskStatusRunning,
				0,
			)

			renderedPage, errorValue := pdfutil.RenderPageAtDPI(executionContext, data, page, 450)
			if errorValue != nil {
				progress.ReportTask(
					executionContext,
					taskIdentifier,
					taskLabel,
					progress.TaskStatusFailed,
					100,
				)
				return nil, errorValue
			}

			progress.ReportTask(
				executionContext,
				taskIdentifier,
				taskLabel,
				progress.TaskStatusCompleted,
				100,
			)
			return renderedPage, nil
		})
		if errorValue != nil {
			return nil, errorValue
		}
		for index, page := range renderPages {
			pages[page] = rendered[index]
		}
	} else {
		for index, page := range renderPages {
			progress.Report(
				executionContext,
				index*25/len(renderPages),
				"pentacam",
				"Renderizando páginas do Pentacam",
			)

			image, errorValue := pdfutil.RenderPageAtDPI(
				executionContext,
				data,
				page,
				450,
			)

			if errorValue != nil {
				return nil, errorValue
			}

			pages[page] = image
		}
	}

	progress.Report(
		executionContext,
		25,
		"pentacam",
		"Páginas renderizadas",
	)

	values := map[string]any{}

	read := func(page int, ocrRegion pentacamOCRRegion, documentFields []pentacamDocumentField) error {
		ocrTexts, errorValue := extractOCRTextsFromRegion(executionContext, pages[page], ocrRegion)
		if errorValue != nil {
			return errorValue
		}

		for _, field := range documentFields {
			for _, text := range ocrTexts {
				if _, fieldAlreadyExtracted := values[field.Key]; fieldAlreadyExtracted {
					break
				}

				if value, ok := parsePentacamFieldNumber(
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

	progress.Report(
		executionContext,
		30,
		"pentacam",
		"Lendo córnea anterior",
	)

	// Página 4 — córnea anterior.
	_ = read(4, pentacamOCRRegion{X: 250, Y: 1240, W: 500, H: 310}, []pentacamDocumentField{
		{"k1_d", `\b(?:k1|ki)\b`, true},
		{"k2_d", `\bk2\b`, true},
		{"km_d", `\bkm\b`, true},
		{"astigmatism_d", `\bastig\w*`, true},
	})

	progress.Report(
		executionContext,
		36,
		"pentacam",
		"Lendo paquimetria",
	)

	// Página 8 — paquimetria.
	_ = read(8, pentacamOCRRegion{X: 740, Y: 1880, W: 400, H: 150}, []pentacamDocumentField{
		{"thinnest_um", `\bthinnest\W+pachy\b`, false},
	})

	progress.Report(
		executionContext,
		42,
		"pentacam",
		"Lendo índices topométricos",
	)

	// Página 9 — índices topométricos.
	topometricOCRTexts, _ := extractOCRTextsFromRegion(
		executionContext,
		pages[9],
		pentacamOCRRegion{X: 730, Y: 2160, W: 450, H: 330},
	)

	topometricDocumentFields := []pentacamDocumentField{
		{"isv", `\b(?:isv|15v)\b`, false},
		{"iva", `\biva\b`, true},
		{"iha", `\biha\b`, true},
		{"ihd", `\bihd\b`, true},
		{"ki", `\bk[i1l]\b`, true},
		{"cki", `\bcki\b`, true},
	}

	for _, field := range topometricDocumentFields {
		for _, text := range topometricOCRTexts {
			if value, ok := parsePentacamFieldNumber(text, field.Label, field.Decimal); ok {
				values[field.Key] = value
				break
			}
		}
	}

	if tkc, ok := pentacamFocusedTKC(topometricOCRTexts); ok {
		values["tkc"] = tkc
	}

	progress.Report(
		executionContext,
		48,
		"pentacam",
		"Lendo BAD-D",
	)

	// Página 11 — BAD-D.
	_ = read(11, pentacamOCRRegion{X: 1600, Y: 2110, W: 330, H: 130}, []pentacamDocumentField{
		{"bad_d", `\bbad\W*d`, true},
	})

	progress.Report(
		executionContext,
		52,
		"pentacam",
		"Lendo Z40 6 mm",
	)

	// Página 10 — Z40 6 mm.
	_ = read(10, pentacamOCRRegion{X: 1970, Y: 2050, W: 460, H: 150}, []pentacamDocumentField{
		{"z40_6mm_um", `\bz40\b`, true},
	})

	progress.Report(
		executionContext,
		56,
		"pentacam",
		"Lendo profundidade da câmara anterior",
	)

	// Página 10 — profundidade da câmara anterior INTERNA.
	_ = read(10, pentacamOCRRegion{X: 1990, Y: 2370, W: 440, H: 180}, []pentacamDocumentField{
		{
			"acd_internal_mm",
			`(?:prof|pot)\w*\W*cam\w*\W*ant\w*\W*\(?int`,
			true,
		},
	})

	progress.Report(
		executionContext,
		60,
		"pentacam",
		"Lendo ARTmax",
	)

	// Página 11 — ARTmax.
	_ = read(11, pentacamOCRRegion{X: 1490, Y: 1420, W: 300, H: 150}, []pentacamDocumentField{
		{"art_max", `\bartmax\b`, false},
	})

	progress.Report(
		executionContext,
		64,
		"pentacam",
		"Lendo Z31 coma 5 mm",
	)

	// Página 12 — Z31 da coluna 5 mm.
	_ = read(12, pentacamOCRRegion{X: 970, Y: 1490, W: 290, H: 180}, []pentacamDocumentField{
		{"z31_5mm_um", `\bz31\W*\(?coma\w*\)?`, true},
	})

	// Segunda passada: célula numérica exata, sem inferência de decimal.
	progress.Report(
		executionContext,
		68,
		"pentacam",
		"Lendo células numéricas",
	)

	fillPentacamFocusedCells(executionContext, pages, values)

	progress.Report(
		executionContext,
		75,
		"pentacam",
		"Células numéricas processadas",
	)

	// BAD-D rescue determinístico:
	// - três dígitos estáveis em múltiplos thresholds
	// - separador decimal comprovado geometricamente nos pixels
	// - nenhuma inferência de casa decimal
	progress.Report(
		executionContext,
		78,
		"pentacam",
		"Validando BAD-D deterministicamente",
	)

	if current, hasBadDValue := values["bad_d"]; !hasBadDValue || current == nil {
		if value, ok := readPentacamBADDDeterministic(executionContext, data); ok {
			values["bad_d"] = value
		}
	}

	// K1 rescue determinístico:
	// mesmo decimal em >=3 thresholds e >=2 ocrRegions.
	progress.Report(
		executionContext,
		84,
		"pentacam",
		"Validando K1 deterministicamente",
	)

	if current, hasK1Value := values["k1_d"]; !hasK1Value || current == nil {
		if value, ok := readPentacamK1Deterministic(executionContext, data); ok {
			values["k1_d"] = value
		}
	}

	progress.Report(
		executionContext,
		90,
		"pentacam",
		"Validando variações de layout",
	)

	// Rescue determinístico tolerante a pequenas variações de layout.
	// Executa somente quando o caminho local principal ainda deixou gap.
	if current, hasK1Value := values["k1_d"]; !hasK1Value || current == nil {
		if value, ok := readPentacamK1Adaptive(executionContext, data); ok {
			values["k1_d"] = value
		}
	}

	progress.Report(
		executionContext,
		96,
		"pentacam",
		"Validando ACD interna",
	)

	if current, hasInternalChamberDepthValue := values["acd_internal_mm"]; !hasInternalChamberDepthValue || current == nil {
		if value, ok := readPentacamACDAdaptive(executionContext, data); ok {
			values["acd_internal_mm"] = value
		}
	}

	progress.Report(
		executionContext,
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
			"kmax_d":        values["kmax_d"],
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
			"ihd": values["ihd"],
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

func pentacamRenderTask(page int) (string, string) {
	labels := map[int]string{
		4:  "Página 4 — Córnea anterior",
		8:  "Página 8 — Paquimetria",
		9:  "Página 9 — Índices topométricos",
		10: "Página 10 — Cataract Pre-OP",
		11: "Página 11 — BAD-D / ARTmax",
		12: "Página 12 — Anéis corneanos",
	}

	return "pentacam-page-" + strconv.Itoa(page), labels[page]
}

func parsePentacamFieldNumber(text, label string, decimal bool) (float64, bool) {
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

	value, errorValue := strconv.ParseFloat(match[1], 64)
	return value, errorValue == nil
}

func pentacamFocusedTKC(ocrTexts []string) (string, bool) {
	for _, raw := range ocrTexts {
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
