package pentacam

import (
	"context"
	"image"
	"regexp"
	"strconv"
	"strings"

	"refratia/backend/shared/ocr"
	"refratia/backend/shared/parallelwork"
	pdfutil "refratia/backend/shared/pdf"

	progress "refratia/backend/shared/progress"
)

type pentacamDocumentField struct {
	Key     string
	Label   string
	Decimal bool
}

// pentacamPageLayout holds, per field group, the page of the screen that
// carries it. 0 means the report has no such screen: the fields stay missing
// ("not_applicable") and no other page is read in its place.
type pentacamPageLayout struct {
	anteriorCornea int
	pachymetry     int
	topometric     int
	badd           int
	z40            int
	acd            int
	z31            int
}

// pentacamPageLayoutFromScreens derives the layout from the detected screens
// instead of the page count (9, 11 and 12 page reports all exist).
func pentacamPageLayoutFromScreens(screens map[Screen]int) pentacamPageLayout {
	return pentacamPageLayout{
		anteriorCornea: screens[ScreenRefractive],
		pachymetry:     screens[ScreenPachymetry],
		topometric:     screens[ScreenTopometric],
		badd:           screens[ScreenBelin],
		z40:            screens[ScreenCataract],
		acd:            screens[ScreenCataract],
		z31:            screens[ScreenRings],
	}
}

func ExtractPDF(executionContext context.Context, data []byte) (map[string]any, error) {
	exam, _, err := ExtractPDFWithHints(executionContext, data)
	return exam, err
}

// ExtractPDFWithHints is ExtractPDF plus the per-field extractor hints that
// feed the C4 diagnostics (strategy, screen, page, bbox, not_applicable).
func ExtractPDFWithHints(executionContext context.Context, data []byte) (map[string]any, []FieldHint, error) {
	return extractPDF(executionContext, data, true)
}

// extractPDF is kept behind the same parser interface so benchmarks can
// compare an experimental execution strategy without creating a second parser.
func extractPDF(executionContext context.Context, data []byte, parallelRender bool) (map[string]any, []FieldHint, error) {
	pages := map[int][]byte{}

	progress.Report(
		executionContext,
		0,
		"pentacam",
		"Preparando páginas do Pentacam",
	)

	pageCount, _, errorValue := pdfutil.Inspect(executionContext, data)
	if errorValue != nil {
		return nil, nil, errorValue
	}

	screens, errorValue := DetectScreens(executionContext, data, pageCount)
	if errorValue != nil {
		return nil, nil, errorValue
	}
	layout := pentacamPageLayoutFromScreens(screens)
	renderPages := []int{}
	seenPage := map[int]bool{}
	for _, page := range []int{
		layout.anteriorCornea,
		layout.pachymetry,
		layout.topometric,
		layout.badd,
		layout.z40,
		layout.acd,
		layout.z31,
	} {
		if page > 0 && page <= pageCount && !seenPage[page] {
			seenPage[page] = true
			renderPages = append(renderPages, page)
		}
	}

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
			return nil, nil, errorValue
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
				return nil, nil, errorValue
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

	// Primary strategy: locate every value through its printed label.
	progress.Report(executionContext, 27, "pentacam", "Localizando campos por rótulo")
	pageImages := map[int]image.Image{}
	for page, encoded := range pages {
		if decoded, err := ocr.DecodeImage(encoded); err == nil {
			pageImages[page] = decoded
		}
	}
	anchored, conflicts := readAnchored(executionContext, screensToPages(screens), pageImages)

	values := map[string]any{}
	for key, found := range anchored {
		values[key] = found.Value
	}
	// A conflicting field (two screens disagree) must stay empty: block the
	// fixed-box readers from filling it with a third, unverified reading.
	for _, conflict := range conflicts {
		for key, spec := range valueKeySpec {
			if spec == conflict.Spec {
				values[key] = nil
			}
		}
	}
	// The fixed-box readers below only fill what the anchors did not read.
	hasAll := func(keys ...string) bool {
		for _, key := range keys {
			if _, ok := values[key]; !ok {
				return false
			}
		}
		return true
	}

	read := func(page int, ocrRegion pentacamOCRRegion, documentFields []pentacamDocumentField) error {
		if pages[page] == nil {
			return nil
		}
		allRead := true
		for _, field := range documentFields {
			allRead = allRead && hasAll(field.Key)
		}
		if allRead && len(documentFields) > 0 {
			return nil
		}

		ocrTexts, errorValue := extractOCRTextsFromRegion(executionContext, pages[page], ocrRegion)
		if errorValue != nil {
			return errorValue
		}

		for _, field := range documentFields {
			for _, text := range ocrTexts {
				if _, fieldAlreadyExtracted := values[field.Key]; fieldAlreadyExtracted {
					break
				}

				value, ok := parsePentacamFieldNumber(
					text,
					field.Label,
					field.Decimal,
				)
				if !ok && field.Key == "z31_5mm_um" {
					value, ok = parsePentacamZ31(text)
				}
				if ok {
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
	_ = read(layout.anteriorCornea, pentacamOCRRegion{X: 250, Y: 1240, W: 500, H: 310}, []pentacamDocumentField{
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
	_ = read(layout.pachymetry, pentacamOCRRegion{X: 1550, Y: 1400, W: 700, H: 350}, nil)

	progress.Report(
		executionContext,
		42,
		"pentacam",
		"Lendo índices topométricos",
	)

	// Página 9 — índices topométricos.
	var topometricOCRTexts []string
	if len(pages[layout.topometric]) > 0 && !hasAll("isv", "iva", "iha", "ki", "cki", "tkc") {
		topometricOCRTexts, _ = extractOCRTextsFromRegion(
			executionContext,
			pages[layout.topometric],
			pentacamOCRRegion{X: 730, Y: 2160, W: 450, H: 330},
		)
	}

	topometricDocumentFields := []pentacamDocumentField{
		{"isv", `\b(?:isv|15v)\b`, false},
		{"iva", `\biva\b`, true},
		{"iha", `\biha\b`, true},
		{"ki", `\bk[i1l]\b`, true},
		{"cki", `\bcki\b`, true},
	}

	for _, field := range topometricDocumentFields {
		if hasAll(field.Key) {
			continue
		}
		for _, text := range topometricOCRTexts {
			if value, ok := parsePentacamFieldNumber(text, field.Label, field.Decimal); ok {
				values[field.Key] = value
				break
			}
		}
	}

	if !hasAll("ihd") {
		if value, ok := readPentacamIHDFromRenderedPage(executionContext, pages[layout.topometric]); ok {
			values["ihd"] = value
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
	_ = read(layout.badd, pentacamOCRRegion{X: 1600, Y: 2110, W: 330, H: 130}, nil)

	progress.Report(
		executionContext,
		52,
		"pentacam",
		"Lendo Z40 6 mm",
	)

	// Página 10 — Z40 6 mm.
	_ = read(layout.z40, pentacamOCRRegion{X: 1970, Y: 2050, W: 460, H: 150}, []pentacamDocumentField{
		{"z40_6mm_um", `\bz40\b`, true},
	})

	progress.Report(
		executionContext,
		56,
		"pentacam",
		"Lendo profundidade da câmara anterior",
	)

	// Página 10 — profundidade da câmara anterior INTERNA.
	_ = read(layout.acd, pentacamOCRRegion{X: 1990, Y: 2370, W: 440, H: 180}, []pentacamDocumentField{
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
	_ = read(layout.badd, pentacamOCRRegion{X: 1490, Y: 1420, W: 300, H: 150}, []pentacamDocumentField{
		{"art_max", `\bartmax\b`, false},
	})

	progress.Report(
		executionContext,
		64,
		"pentacam",
		"Lendo Z31 coma 5 mm",
	)

	// Página 12 — Z31 da coluna 5 mm.
	_ = read(layout.z31, pentacamOCRRegion{X: 1000, Y: 1500, W: 200, H: 170}, []pentacamDocumentField{
		{"z31_5mm_um", `\bz31\W*\(?coma\w*\)?`, true},
	})

	// Segunda passada: célula numérica exata, sem inferência de decimal.
	progress.Report(
		executionContext,
		68,
		"pentacam",
		"Lendo células numéricas",
	)

	fillPentacamFocusedCells(executionContext, pages, values, layout, anchored)

	if _, hasThinnestPachymetry := values["thinnest_um"]; !hasThinnestPachymetry {
		if value, ok := readPentacamThinnestPachymetryFromRenderedPage(executionContext, pages[layout.pachymetry]); ok {
			values["thinnest_um"] = value
		}
	}

	if _, hasThinnestPachymetry := values["thinnest_um"]; !hasThinnestPachymetry {
		if value, ok := readPentacamThinnestPachymetryAdaptiveFromPage(executionContext, pages[layout.pachymetry]); ok {
			values["thinnest_um"] = value
		}
	}

	if !hasAll("acd_internal_mm") {
		if value, ok := readPentacamACDFromPage(executionContext, pages[layout.acd]); ok {
			values["acd_internal_mm"] = value
		}
	}

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

	// Prefer the deterministic reader whenever it can prove the decimal
	// separator and each digit independently. The broad OCR reader can
	// collapse values such as 0.98 into "5" or pick a nearby annotation.
	// The broad OCR reader of the "D" cell was removed: it returned
	// well-formed but wrong values (a lone digit) when the cell was clipped.
	// Only the reader that proves the decimal separator in the pixels stays.
	if !hasAll("bad_d") {
		if value, ok := readPentacamBADDDeterministicFromPage(executionContext, pages[layout.badd]); ok {
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
		if value, ok := readPentacamK1DeterministicFromPage(executionContext, pages[layout.anteriorCornea]); ok {
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
		if value, ok := readPentacamK1AdaptiveFromPage(executionContext, pages[layout.anteriorCornea]); ok {
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
		if value, ok := readPentacamACDAdaptiveFromPage(executionContext, pages[layout.acd]); ok {
			values["acd_internal_mm"] = value
		}
	}

	progress.Report(
		executionContext,
		100,
		"pentacam",
		"Pentacam validado",
	)

	conflicts = append(conflicts, crossCheckAstigmatism(values)...)

	return map[string]any{
		"anterior_cornea": map[string]any{
			"k1_d":          values["k1_d"],
			"k2_d":          values["k2_d"],
			"km_d":          values["km_d"],
			"astigmatism_d": values["astigmatism_d"],
			"axis_deg":      values["axis_deg"],
			"kmax_d":        values["kmax_d"],
		},
		"pachymetry": map[string]any{
			"thinnest_um":      values["thinnest_um"],
			"pupil_center_um":  values["pupil_center_um"],
			"vertex_normal_um": values["vertex_normal_um"],
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
			"external_anterior_chamber_depth_mm": values["acd_external_mm"],
		},
		"cataract_preop": map[string]any{
			"total_corneal_z40_6mm_um": values["z40_6mm_um"],
		},
	}, buildHints(screens, anchored, conflicts), nil
}

func screensToPages(screens map[Screen]int) map[Screen]int { return screens }

// buildHints reports, for the C4 diagnostics, which strategy read each field
// and which fields cannot exist because their screen is absent.
func buildHints(screens map[Screen]int, anchored map[string]anchoredValue, conflicts []FieldHint) []FieldHint {
	hints := append([]FieldHint(nil), conflicts...)
	for _, found := range anchored {
		hints = append(hints, found.Hint)
	}
	for screen, fields := range screenSpecFields {
		if screens[screen] != 0 {
			continue
		}
		for _, field := range fields {
			hints = append(hints, FieldHint{Spec: field, Status: "not_applicable", Stage: "region", Screen: string(screen)})
		}
	}
	return hints
}

func parsePentacamZ31(text string) (float64, bool) {
	text = normalizePentacamText(text)
	match := regexp.MustCompile(
		`(?is)\bz31\W*\(?coma\w*\)?[^0-9+\-]{0,32}(0\.\d{3}|\d{3})\b`,
	).FindStringSubmatch(text)
	if len(match) != 2 {
		// The isolated label is sometimes OCR'd as "231". In that case,
		// use only the three digits immediately followed by the unit so the
		// angle (207°) cannot be mistaken for the Z31 value.
		match = regexp.MustCompile(`(?i)\b(\d{3})\s*(?:um|pm|µm)\b`).FindStringSubmatch(text)
		if len(match) != 2 {
			return 0, false
		}
	}

	token := match[1]
	if !strings.Contains(token, ".") {
		token = "0." + token
	}

	value, err := strconv.ParseFloat(token, 64)
	return value, err == nil && value >= -5 && value <= 5
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
		`(?is)` + label + `[^0-9+\-]{0,8}(` + number + `)`,
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

		// TKC is a categorical keratoconus classification, not a decimal
		// measurement. Reject nearby numeric OCR such as "4.521".
		token := regexp.MustCompile(`(?i)\b(?:kc|tkc)?\s*[1-4](?:[a-d])?\b`).FindString(tail)
		if token != "" && !strings.Contains(token, ".") {
			return strings.ToUpper(strings.Join(strings.Fields(token), " ")), true
		}
	}

	return "", false
}
