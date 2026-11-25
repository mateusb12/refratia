package intake

// Claude Vision: an optional, alternative extraction engine. It reads only
// anonymized PDFs released by frontend/scripts/anonymize_exams.py, asks Claude
// for the spec fields the app can store, validates the answer and writes it
// into the same analysis shape the OCR engine produces. Identity is never sent
// to Claude: it is attached afterwards from the operator's local case context.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"

	"refratia/backend/src/features/fileIdentification/extractionfields"
	"refratia/backend/src/shared/parallelwork"
	progress "refratia/backend/src/shared/progress"
)

const claudeVisionModel = "claude-opus-5-5"

// claudeVisionExams are the exams Claude may extract (the protocol exams with
// numeric spec fields). Others stay with the OCR engine.
var claudeVisionExams = []string{"iol_calculation", "pentacam_corneal_tomography", "specular_microscopy", "refractometry"}

// claudeDateOrder is the device convention for printed slash dates. Only the
// Pentacam prints MM/DD/YYYY; EyeSuite, NIDEK and the BR refractometers DMY.
func claudeDateOrder(examKey string) string {
	if examKey == "pentacam_corneal_tomography" {
		return "MDY"
	}
	return "DMY"
}

// ClaudeVisionStatus is what the upload UI needs to enable the engine.
type ClaudeVisionStatus struct {
	Available bool   `json:"available"`
	Mode      string `json:"mode,omitempty"` // "live" | "fixture"
	Model     string `json:"model,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

func claudeVisionStatus() ClaudeVisionStatus {
	approved, err := approvedAnonymizedFiles()
	if err != nil || len(approved) == 0 {
		return ClaudeVisionStatus{Reason: "nenhum documento anonimizado liberado encontrado em " + claudeApprovedDir() + " (rode frontend/scripts/anonymize_exams.py)"}
	}
	if path := os.Getenv("CLAUDE_VISION_FIXTURE"); path != "" {
		if _, err := os.Stat(path); err != nil {
			return ClaudeVisionStatus{Reason: "CLAUDE_VISION_FIXTURE aponta para um arquivo inexistente"}
		}
		return ClaudeVisionStatus{Available: true, Mode: "fixture", Model: claudeVisionModel}
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return ClaudeVisionStatus{Reason: "ANTHROPIC_API_KEY não configurada no backend"}
	}
	return ClaudeVisionStatus{Available: true, Mode: "live", Model: claudeVisionModel}
}

// ExtractionEnginesHandler: GET /api/intakes/engines.
func ExtractionEnginesHandler(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	responseWriter.Header().Set("Content-Type", "application/json")
	json.NewEncoder(responseWriter).Encode(map[string]any{
		"ocr":    map[string]any{"available": true},
		"claude": claudeVisionStatus(),
	})
}

func claudeApprovedDir() string {
	if dir := os.Getenv("CLAUDE_VISION_APPROVED_DIR"); dir != "" {
		return dir
	}
	return "../frontend/data"
}

// approvedAnonymizedFiles maps the SHA-256 of every anonymizer output whose
// review status is APPROVED (a human checked the preview; the automatic OK is not
// enough) to its path. Only these exact bytes may be sent to Claude.
// ponytail: re-hashes on every call (a handful of PDFs); cache by mtime if the corpus grows.
func approvedAnonymizedFiles() (map[string]string, error) {
	dir := claudeApprovedDir()
	reports, err := filepath.Glob(filepath.Join(dir, "_anon-review", "*", "report.json"))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, report := range reports {
		raw, err := os.ReadFile(report)
		if err != nil {
			continue
		}
		var entries []struct {
			Output string `json:"output"`
			Status string `json:"status"`
		}
		if json.Unmarshal(raw, &entries) != nil {
			continue
		}
		patient := filepath.Base(filepath.Dir(report))
		for _, entry := range entries {
			if entry.Status != "APPROVED" {
				continue
			}
			path := filepath.Join(dir, patient, "anonymized", filepath.Base(entry.Output))
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			out[sha256Hex(data)] = path
		}
	}
	return out, nil
}

// LocalIdentity is the patient identity typed by the operator. It is attached
// to the analysis after extraction and never sent to Claude.
type LocalIdentity struct {
	FullName  string
	BirthDate string // YYYY-MM-DD
}

// claudeMeasurement / claudeFileResult are the structured output Claude must return.
type claudeMeasurement struct {
	Eye     string   `json:"eye"`
	Field   string   `json:"field"`
	Status  string   `json:"status"` // read | printed_dash | unreadable
	Value   *float64 `json:"value"`
	Text    *string  `json:"text"`
	Printed string   `json:"printed"`
	Unit    *string  `json:"unit"`
	Page    int      `json:"page"`
}

type claudeFileResult struct {
	ExamType        string              `json:"exam_type"`
	Device          *string             `json:"device"`
	PrintedExamDate *string             `json:"printed_exam_date"`
	Measurements    []claudeMeasurement `json:"measurements"`
}

func nullable(schema map[string]any) map[string]any {
	return map[string]any{"anyOf": []any{schema, map[string]any{"type": "null"}}}
}

func claudeResultSchema(examKey string) map[string]any {
	names := []any{}
	for _, field := range extractionfields.Fields(examKey) {
		names = append(names, field.Name)
	}
	examTypes := []any{"other"}
	for _, key := range claudeVisionExams {
		examTypes = append(examTypes, key)
	}
	measurement := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"eye", "field", "status", "value", "text", "printed", "unit", "page"},
		"properties": map[string]any{
			"eye":     map[string]any{"type": "string", "enum": []any{"OD", "OS"}},
			"field":   map[string]any{"type": "string", "enum": names},
			"status":  map[string]any{"type": "string", "enum": []any{"read", "printed_dash", "unreadable"}},
			"value":   nullable(map[string]any{"type": "number"}),
			"text":    nullable(map[string]any{"type": "string"}),
			"printed": map[string]any{"type": "string"},
			"unit":    nullable(map[string]any{"type": "string"}),
			"page":    map[string]any{"type": "integer"},
		},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"exam_type", "device", "printed_exam_date", "measurements"},
		"properties": map[string]any{
			"exam_type":         map[string]any{"type": "string", "enum": examTypes},
			"device":            nullable(map[string]any{"type": "string"}),
			"printed_exam_date": nullable(map[string]any{"type": "string"}),
			"measurements":      map[string]any{"type": "array", "items": measurement},
		},
	}
}

// claudeFieldHints tells Claude where each spec field is printed.
var claudeFieldHints = map[string]string{
	// Pentacam
	"qi": "QI quality label (e.g. OK)", "k1_d": "front cornea K1", "k2_d": "front cornea K2", "km_d": "front cornea Km",
	"astig_ant_d": "front cornea Astig.", "axis_flat_deg": "front cornea Axis (flat)", "kmax_ant_d": "K Max (Front)",
	"pachy_thinnest_um": "Pachy Thinnest Location", "pachy_pupil_center_um": "Pachy Pupil Center", "pachy_vertex_um": "Pachy Apex / vertex (normal)",
	"acd_ext_mm": "ACD (External)", "acd_int_mm": "ACD (Internal) from Cataract Pre-OP", "pns": "PNS (Pentacam Nucleus Staging) from Cataract Pre-OP",
	"z40_6mm_um": "Cataract Pre-OP: total corneal Z40 at 6 mm zone", "bad_d": "Belin/Ambrósio D (final D)", "art_max_um": "Belin/Ambrósio ART max",
	"isv": "Topometric ISV", "iva": "Topometric IVA", "iha": "Topometric IHA", "ihd": "Topometric IHD", "ki": "Topometric KI", "cki": "Topometric CKI",
	"tkc": "Topometric TKC classification text; a printed dash is status printed_dash", "abcd_staging": "ABCD staging text (e.g. A0 B0 C0 D)",
	"coma_z31_5mm_um": "corneal Zernike Z 3,1 coma magnitude at 5 mm zone", "coma_z31_5mm_axis_deg": "axis of that 5 mm coma", "coma_z31_6mm_um": "corneal Zernike Z 3,1 coma magnitude at 6 mm zone",
	// EyeSuite biometry
	"lens_status": "lens status (Phakic, Pseudophakic...)", "measurement_date": "measurement date of this eye", "al_mm": "AL", "cct_um": "CCT", "ad_mm": "AD (aqueous depth)",
	"acd_mm": "ACD", "lt_mm": "LT", "k1_radius_mm": "K1 radius mm", "k1_axis_deg": "K1 axis", "k2_radius_mm": "K2 radius mm", "k2_axis_deg": "K2 axis",
	"k_mean_d": "K (mean) D", "k_mean_radius_mm": "K (mean) radius mm", "astig_d": "-AST value with its sign", "astig_axis_deg": "-AST axis",
	"wtw_mm": "WTW", "target_refraction_d": "Target refraction",
	// Specular microscopy (NIDEK: R = OD, L = OS)
	"cell_density_cells_mm2": "CD (cells/mm2)", "num_cells": "NUM", "avg_area_um2": "AVG", "sd_um2": "SD", "cv_pct": "CV", "max_area_um2": "MAX",
	"min_area_um2": "MIN", "hex_pct": "HEX", "ct_um": "CT (corneal thickness)",
	// Refractometry
	"refraction_type": "refraction type", "sphere_d": "sphere (S)", "cylinder_d": "cylinder (C)", "axis_deg": "axis (A)", "vertex_distance_mm": "VD", "pd_mm": "PD",
}

func claudePrompt(examKey string) string {
	var catalog strings.Builder
	for _, field := range extractionfields.Fields(examKey) {
		unit := field.Unit
		if unit == "" {
			unit = "-"
		}
		fmt.Fprintf(&catalog, "- %s [%s, unit %s]: %s\n", field.Name, field.Kind, unit, claudeFieldHints[field.Name])
	}
	return fmt.Sprintf(`You are transcribing an anonymized ophthalmology report. The operator classified it as %q.

Return exam_type (what the document really is), device, printed_exam_date (the exam date and time EXACTLY as printed, no reformatting) and one measurement per field and eye listed below. Nothing else.

Fields:
%s
Rules:
- Transcribe, never compute, infer or copy a value from another field, eye or document.
- eye: OD = right / Direito / R, OS = left / Esquerdo / L / OE, as printed on the page.
- read: value = the printed number as a JSON number with its sign and every printed decimal (decimal comma becomes a dot); for text fields put the text in text and value null.
- printed_dash: the field is printed as a dash or blank placeholder (e.g. TKC "-"). value and text null.
- unreadable: the field exists but cannot be read with certainty. value and text null.
- A field not present in the document: omit it.
- printed: the raw printed characters of that cell; unit: the printed unit or null; page: 1-based page number.`, examKey, catalog.String())
}

func requestClaudeFile(ctx context.Context, file UploadedFile, examKey string) (claudeFileResult, error) {
	client := anthropic.NewClient() // ANTHROPIC_API_KEY
	stream := client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     claudeVisionModel,
		MaxTokens: 32000,
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffortHigh,
			Format: anthropic.JSONOutputFormatParam{Schema: claudeResultSchema(examKey)},
		},
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(
			anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{Data: base64.StdEncoding.EncodeToString(file.Data)}),
			anthropic.NewTextBlock(claudePrompt(examKey)),
		)},
	})
	message := anthropic.Message{}
	for stream.Next() {
		message.Accumulate(stream.Current())
	}
	if err := stream.Err(); err != nil {
		return claudeFileResult{}, fmt.Errorf("Claude Vision falhou em %s: %w", file.Metadata.Filename, err)
	}
	if message.StopReason != anthropic.StopReasonEndTurn {
		return claudeFileResult{}, fmt.Errorf("Claude Vision não concluiu %s (stop_reason=%s)", file.Metadata.Filename, message.StopReason)
	}
	var text strings.Builder
	for _, block := range message.Content {
		if b, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(b.Text)
		}
	}
	var result claudeFileResult
	if err := json.Unmarshal([]byte(text.String()), &result); err != nil {
		return claudeFileResult{}, fmt.Errorf("Claude Vision devolveu JSON inválido para %s", file.Metadata.Filename)
	}
	return result, nil
}

// claudeFixture: {"files": {"<sha256>": claudeFileResult}}, a recorded answer
// replayed without calling the API (CLAUDE_VISION_FIXTURE).
func claudeFixtureFile(file UploadedFile) (claudeFileResult, error) {
	raw, err := os.ReadFile(os.Getenv("CLAUDE_VISION_FIXTURE"))
	if err != nil {
		return claudeFileResult{}, err
	}
	var fixture struct {
		Files map[string]claudeFileResult `json:"files"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		return claudeFileResult{}, fmt.Errorf("CLAUDE_VISION_FIXTURE inválido: %w", err)
	}
	result, ok := fixture.Files[file.Metadata.SHA256]
	if !ok {
		return claudeFileResult{}, fmt.Errorf("CLAUDE_VISION_FIXTURE não tem resposta para %s", file.Metadata.Filename)
	}
	return result, nil
}

func extractPatientClaude(ctx context.Context, files []UploadedFile, identity LocalIdentity) (map[string]any, error) {
	status := claudeVisionStatus()
	if !status.Available {
		return nil, errors.New("Claude Vision indisponível: " + status.Reason)
	}
	// Gate before any network call: every byte sent must be an approved anonymizer output.
	approved, err := approvedAnonymizedFiles()
	if err != nil {
		return nil, err
	}
	examKeys := make([]string, len(files))
	for index, file := range files {
		if _, ok := approved[file.Metadata.SHA256]; !ok {
			return nil, fmt.Errorf("%s não é um PDF anonimizado liberado por anonymize_exams.py; o Claude Vision não aceita documentos originais", file.Metadata.Filename)
		}
		examKeys[index] = checkpointExamKey(file.Metadata.ExamType)
		if !claudeVisionSupports(examKeys[index]) {
			return nil, fmt.Errorf("%s: exame %q não é suportado pelo Claude Vision", file.Metadata.Filename, file.Metadata.ExamType)
		}
	}

	progress.Report(ctx, 5, "claude", fmt.Sprintf("Claude Vision (%s) — enviando %d documento(s) anonimizado(s)", status.Mode, len(files)))
	indexes := make([]int, len(files))
	for index := range files {
		indexes[index] = index
	}
	results, err := parallelwork.Map(ctx, indexes, len(files), func(workContext context.Context, index int) (claudeFileResult, error) {
		file := files[index]
		fileContext := progress.WithFilename(workContext, file.Metadata.Filename)
		progress.Report(fileContext, 10, "claude", "Claude Vision lendo "+file.Metadata.Filename)
		var result claudeFileResult
		var err error
		if status.Mode == "fixture" {
			result, err = claudeFixtureFile(file)
		} else {
			result, err = requestClaudeFile(fileContext, file, examKeys[index])
		}
		if err == nil {
			progress.Report(fileContext, 100, "claude", "Claude Vision concluiu "+file.Metadata.Filename)
		}
		return result, err
	})
	if err != nil {
		return nil, err
	}

	progress.Report(ctx, 85, "validation", "Validando a resposta do Claude contra a especificação dos campos")
	analysis := map[string]any{}
	evidence := []any{}
	for index, file := range files {
		fileEvidence, err := mergeClaudeResult(analysis, file, examKeys[index], results[index])
		if err != nil {
			return nil, err
		}
		evidence = append(evidence, fileEvidence)
	}
	analysis["extraction_notes"] = map[string]any{
		"method":               "claude_vision",
		"engine":               "claude_vision",
		"model":                claudeVisionModel,
		"mode":                 status.Mode,
		"scope":                "campos de _field_spec.json com caminho no app; PDFs anonimizados por anonymize_exams.py",
		"identity_source":      "contexto local do operador (não enviado ao Claude)",
		"clinical_use_warning": "Extração demonstrativa: confira os valores antes de confirmar.",
	}
	analysis["claude_vision_evidence"] = evidence
	analysis["patient"] = map[string]any{"full_name": identity.FullName, "birth_date": identity.BirthDate}

	enrichSourceFiles(analysis, files)
	progress.Report(ctx, 100, "consolidation", "Extração Claude Vision concluída")
	return analysis, nil
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func claudeVisionSupports(examKey string) bool {
	for _, key := range claudeVisionExams {
		if key == examKey {
			return true
		}
	}
	return false
}

func fileEyes(eye string) map[string]bool {
	if eye == "AO" {
		return map[string]bool{"OD": true, "OS": true}
	}
	return map[string]bool{eye: true}
}

// mergeClaudeResult validates one file's answer and writes it into analysis
// exactly like the OCR writers: values at the spec app path, diagnostics via
// extractionfields (invalid values removed, never coerced), TKC dash as the
// same not_applicable/recognition hint, an identity entry with an ISO timestamp.
func mergeClaudeResult(analysis map[string]any, file UploadedFile, examKey string, result claudeFileResult) (map[string]any, error) {
	if result.ExamType != examKey {
		return nil, fmt.Errorf("%s: classificado como %s, mas o Claude identificou %s; revise a classificação", file.Metadata.Filename, examKey, result.ExamType)
	}
	defer validateTouchedEyes(analysis, file, snapshotEyes(analysis))

	fields := map[string]extractionfields.Field{}
	for _, field := range extractionfields.Fields(examKey) {
		fields[field.Name] = field
	}
	allowedEyes := fileEyes(file.Metadata.Eye)
	order := claudeDateOrder(examKey)

	exams, _ := analysis["exams"].(map[string]any)
	if exams == nil {
		exams = map[string]any{}
		analysis["exams"] = exams
	}
	exam, _ := exams[examKey].(map[string]any)
	if exam == nil {
		exam = map[string]any{"eyes": map[string]any{}}
		exams[examKey] = exam
	}
	eyes := exam["eyes"].(map[string]any)
	sources, _ := exam["source"].([]any)
	exam["source"] = append(sources, file.Metadata.Filename)

	rejected := []any{}
	hints := map[string][]extractionfields.Hint{}
	seen := map[string]bool{}
	for _, m := range result.Measurements {
		field, known := fields[m.Field]
		key := m.Eye + "|" + m.Field
		switch {
		case !known:
			rejected = append(rejected, map[string]any{"eye": m.Eye, "field": m.Field, "reason": "campo fora da especificação"})
			continue
		case !allowedEyes[m.Eye]:
			rejected = append(rejected, map[string]any{"eye": m.Eye, "field": m.Field, "reason": "olho diferente da classificação do arquivo"})
			continue
		case seen[key]:
			rejected = append(rejected, map[string]any{"eye": m.Eye, "field": m.Field, "reason": "campo repetido"})
			continue
		}
		seen[key] = true
		eye, _ := eyes[m.Eye].(map[string]any)
		if eye == nil {
			eye = map[string]any{}
			eyes[m.Eye] = eye
		}
		hint := extractionfields.Hint{Spec: m.Field, Page: m.Page, Strategy: "claude_vision"}
		status := m.Status
		if status == "read" && field.Kind == "str" && m.Text != nil && isPrintedDash(*m.Text) {
			status = "printed_dash"
		}
		switch status {
		case "printed_dash":
			// A printed dash is an observed "no value" (TKC: no classification), not a missing read.
			hint.Status, hint.Stage, hint.Strategy = "not_applicable", "recognition", "printed_dash"
		case "unreadable":
			hint.Status, hint.Stage = "unreadable", "recognition"
		case "read":
			if value, ok := claudeValue(field, m, order); ok {
				setPath(eye, field.Path, value)
			} else {
				hint.Status, hint.Stage = "unreadable", "mapping"
			}
		default:
			rejected = append(rejected, map[string]any{"eye": m.Eye, "field": m.Field, "reason": "status desconhecido"})
			continue
		}
		hints[m.Eye] = append(hints[m.Eye], hint)
	}
	for eyeName, eyeHints := range hints {
		setFieldHints(analysis, examKey, eyeName, eyeHints)
	}

	printed := ""
	if result.PrintedExamDate != nil {
		printed = strings.TrimSpace(*result.PrintedExamDate)
	}
	entry := map[string]any{
		"source":             file.Metadata.Filename,
		"timestamp_impresso": printed,
		"date_order":         order,
		"confidence":         "claude_vision",
		"method":             "claude_vision",
	}
	if iso, ok := parsePrintedTimestamp(printed, order); ok {
		entry["timestamp_lido"] = iso
	} else if printed != "" {
		entry["timestamp_lido"] = printed // ambiguous: the evaluator leaves it unread
	}
	verification, _ := analysis["verificacao_identidade"].([]any)
	analysis["verificacao_identidade"] = append(verification, entry)

	device := ""
	if result.Device != nil {
		device = *result.Device
	}
	measurements := make([]any, 0, len(result.Measurements))
	for _, m := range result.Measurements {
		item := map[string]any{"eye": m.Eye, "field": m.Field, "status": m.Status, "printed": m.Printed, "page": m.Page}
		if m.Unit != nil {
			item["unit"] = *m.Unit
		}
		measurements = append(measurements, item)
	}
	return map[string]any{
		"file": file.Metadata.Filename, "sha256": file.Metadata.SHA256, "exam_type": result.ExamType,
		"device": device, "printed_exam_date": printed, "date_order": order,
		"measurements": measurements, "rejected": rejected,
	}, nil
}

func isPrintedDash(text string) bool {
	switch strings.TrimSpace(text) {
	case "-", "—", "–", "--":
		return true
	}
	return false
}

// claudeValue converts one read measurement to the app value of its spec kind.
// Numbers are kept as Claude printed them (sign and decimals); range, decimals
// and integer-ness are checked afterwards by extractionfields.
func claudeValue(field extractionfields.Field, m claudeMeasurement, order string) (any, bool) {
	switch field.Kind {
	case "num", "int":
		if m.Value != nil {
			return *m.Value, true
		}
		if m.Text != nil { // tolerate a number sent as text ("-1,00")
			if n, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(*m.Text), ",", "."), 64); err == nil {
				return n, true
			}
		}
	case "str":
		if m.Text != nil && strings.TrimSpace(*m.Text) != "" {
			return strings.TrimSpace(*m.Text), true
		}
	case "date":
		raw := m.Printed
		if m.Text != nil && *m.Text != "" {
			raw = *m.Text
		}
		if iso, ok := parsePrintedTimestamp(raw, order); ok {
			return iso[:10], true
		}
	}
	return nil, false
}

func setPath(root map[string]any, path []string, value any) {
	current := root
	for _, key := range path[:len(path)-1] {
		next, _ := current[key].(map[string]any)
		if next == nil {
			next = map[string]any{}
			current[key] = next
		}
		current = next
	}
	current[path[len(path)-1]] = value
}

var (
	printedDatePattern = regexp.MustCompile(`(\d{1,2})[/.-]([A-Za-z]{3}|\d{1,2})[/.-](\d{4}|\d{2})\b`)
	printedTimePattern = regexp.MustCompile(`\b(\d{1,2}):(\d{2})(?::(\d{2}))?`)
	printedMonths      = map[string]int{"JAN": 1, "FEB": 2, "FEV": 2, "MAR": 3, "APR": 4, "ABR": 4, "MAY": 5, "MAI": 5, "JUN": 6, "JUL": 7, "AUG": 8, "AGO": 8, "SEP": 9, "SET": 9, "OCT": 10, "OUT": 10, "NOV": 11, "DEC": 12, "DEZ": 12}
)

// parsePrintedTimestamp turns a printed device date (and optional time) into
// "YYYY-MM-DD[ HH:MM[:SS]]" using the device's day/month order. A month name
// is unambiguous; otherwise order decides (MDY Pentacam, DMY the others).
func parsePrintedTimestamp(raw, order string) (string, bool) {
	match := printedDatePattern.FindStringSubmatch(raw)
	if match == nil {
		return "", false
	}
	first, _ := strconv.Atoi(match[1])
	year, _ := strconv.Atoi(match[3])
	if len(match[3]) == 2 {
		year += 2000
	}
	var day, month int
	if named, ok := printedMonths[strings.ToUpper(match[2])]; ok {
		day, month = first, named
	} else {
		second, err := strconv.Atoi(match[2])
		if err != nil {
			return "", false
		}
		day, month = first, second
		if order == "MDY" {
			day, month = second, first
		}
	}
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if date.Day() != day || int(date.Month()) != month {
		return "", false
	}
	out := date.Format("2006-01-02")
	rest := strings.Replace(raw, match[0], "", 1)
	if t := printedTimePattern.FindStringSubmatch(rest); t != nil {
		hour, _ := strconv.Atoi(t[1])
		out += fmt.Sprintf(" %02d:%s", hour, t[2])
		if t[3] != "" {
			out += ":" + t[3]
		}
	}
	return out, true
}
