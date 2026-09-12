package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"refratia/backend/features/eyesuite"
	"refratia/backend/features/pentacam"
	"refratia/backend/shared/progress"
)

type benchmarkField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Unit  string `json:"unit,omitempty"`
	Found bool   `json:"found"`
}

type benchmarkFieldsEvent struct {
	Type      string           `json:"type"`
	Percent   int              `json:"percent,omitempty"`
	Message   string           `json:"message,omitempty"`
	ExamType  string           `json:"examType,omitempty"`
	Eye       string           `json:"eye,omitempty"`
	ElapsedMS int64            `json:"elapsedMs,omitempty"`
	Fields    []benchmarkField `json:"fields,omitempty"`
	Extracted int              `json:"extracted,omitempty"`
	Total     int              `json:"total,omitempty"`
	Supported bool             `json:"supported,omitempty"`
	Error     string           `json:"error,omitempty"`
}

type benchmarkFilename struct {
	ExamType string
	Eye      string
}

func parseBenchmarkFilename(name string) (benchmarkFilename, error) {
	base := strings.TrimSuffix(
		filepath.Base(name),
		filepath.Ext(name),
	)

	parts := strings.Split(base, "__")

	if len(parts) < 3 {
		return benchmarkFilename{}, fmt.Errorf(
			"nome fora do padrão TIPO__LATERALIDADE__PACIENTE__DATAHORA",
		)
	}

	examType := strings.ToUpper(strings.TrimSpace(parts[0]))
	eye := strings.ToUpper(strings.TrimSpace(parts[1]))

	switch eye {
	case "OD", "OS", "AO":
	default:
		return benchmarkFilename{}, fmt.Errorf(
			"lateralidade inválida no filename: %s",
			eye,
		)
	}

	switch examType {
	case "PENTACAM",
		"EYESUITE",
		"RETINA",
		"CORNEA",
		"MICROSCOPIA_ESPECULAR":
	default:
		return benchmarkFilename{}, fmt.Errorf(
			"tipo de exame desconhecido no filename: %s",
			examType,
		)
	}

	return benchmarkFilename{
		ExamType: examType,
		Eye:      eye,
	}, nil
}

type pentacamBenchmarkSpec struct {
	Key   string
	Label string
	Path  []string
}

var pentacamBenchmarkFields = []pentacamBenchmarkSpec{
	{
		Key:   "k1",
		Label: "K1",
		Path:  []string{"anterior_cornea", "k1_d"},
	},
	{
		Key:   "k2",
		Label: "K2",
		Path:  []string{"anterior_cornea", "k2_d"},
	},
	{
		Key:   "km",
		Label: "Km",
		Path:  []string{"anterior_cornea", "km_d"},
	},
	{
		Key:   "astigmatism",
		Label: "Astigmatismo corneano anterior",
		Path:  []string{"anterior_cornea", "astigmatism_d"},
	},
	{
		Key:   "thinnest",
		Label: "Paquimetria — ponto mais fino",
		Path:  []string{"pachymetry", "thinnest_um"},
	},
	{
		Key:   "bad_d",
		Label: "BAD-D",
		Path:  []string{"belin_ambrosio", "d"},
	},
	{
		Key:   "art_max",
		Label: "ARTmax",
		Path:  []string{"belin_ambrosio", "art_max"},
	},
	{
		Key:   "isv",
		Label: "ISV",
		Path:  []string{"topometric_indices_8mm", "isv"},
	},
	{
		Key:   "iva",
		Label: "IVA",
		Path:  []string{"topometric_indices_8mm", "iva"},
	},
	{
		Key:   "iha",
		Label: "IHA",
		Path:  []string{"topometric_indices_8mm", "iha"},
	},
	{
		Key:   "ki",
		Label: "KI",
		Path:  []string{"topometric_indices_8mm", "ki"},
	},
	{
		Key:   "cki",
		Label: "CKI",
		Path:  []string{"topometric_indices_8mm", "cki"},
	},
	{
		Key:   "tkc",
		Label: "TKC",
		Path:  []string{"topometric_indices_8mm", "tkc"},
	},
	{
		Key:   "z31",
		Label: "Z31 Coma — 5 mm",
		Path:  []string{"corneal_rings", "zernike", "5mm", "z31_coma"},
	},
	{
		Key:   "acd_internal",
		Label: "ACD interna",
		Path: []string{
			"anterior_segment",
			"internal_anterior_chamber_depth_mm",
		},
	},
	{
		Key:   "z40",
		Label: "Total Corneal Z40 — 6 mm",
		Path: []string{
			"cataract_preop",
			"total_corneal_z40_6mm_um",
		},
	},
}

type specularMicroscopyBenchmarkSpec struct {
	Key   string
	Label string
	Unit  string
}

var specularMicroscopyBenchmarkFields = []specularMicroscopyBenchmarkSpec{
	{
		Key:   "cell_density",
		Label: "Densidade endotelial",
		Unit:  "células/mm²",
	},
}

func benchmarkPathValue(
	root map[string]any,
	path []string,
) (any, bool) {
	var current any = root

	for _, key := range path {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}

		current, ok = mapping[key]
		if !ok {
			return nil, false
		}
	}

	return current, benchmarkValuePresent(current)
}

func benchmarkValuePresent(value any) bool {
	if value == nil {
		return false
	}

	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""

	case float64:
		return !math.IsNaN(typed)

	case float32:
		return !math.IsNaN(float64(typed))

	case map[string]any:
		return len(typed) > 0

	case []any:
		return len(typed) > 0

	default:
		return true
	}
}

func benchmarkPentacamResult(
	exam map[string]any,
) ([]benchmarkField, int) {
	fields := make(
		[]benchmarkField,
		0,
		len(pentacamBenchmarkFields),
	)

	extracted := 0

	for _, spec := range pentacamBenchmarkFields {
		_, found := benchmarkPathValue(
			exam,
			spec.Path,
		)

		if found {
			extracted++
		}

		fields = append(
			fields,
			benchmarkField{
				Key:   spec.Key,
				Label: spec.Label,
				Found: found,
			},
		)
	}

	return fields, extracted
}

func benchmarkSpecularMicroscopyResult() ([]benchmarkField, int) {
	fields := make([]benchmarkField, 0, len(specularMicroscopyBenchmarkFields))

	for _, spec := range specularMicroscopyBenchmarkFields {
		// A microscopia ainda não possui extrator clínico. O filename identifica
		// o documento, mas nunca preenche um valor clínico.
		fields = append(fields, benchmarkField{
			Key:   spec.Key,
			Label: spec.Label,
			Unit:  spec.Unit,
			Found: false,
		})
	}

	return fields, 0
}

func benchmarkFlattenExtracted(
	value any,
	prefix string,
	out *[]benchmarkField,
) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))

		for key := range typed {
			keys = append(keys, key)
		}

		sort.Strings(keys)

		for _, key := range keys {
			next := key

			if prefix != "" {
				next = prefix + "." + key
			}

			benchmarkFlattenExtracted(
				typed[key],
				next,
				out,
			)
		}

	case []any:
		for index, item := range typed {
			next := fmt.Sprintf(
				"%s[%d]",
				prefix,
				index,
			)

			benchmarkFlattenExtracted(
				item,
				next,
				out,
			)
		}

	default:
		if prefix == "" || !benchmarkValuePresent(value) {
			return
		}

		*out = append(
			*out,
			benchmarkField{
				Key:   prefix,
				Label: prefix,
				Found: true,
			},
		)
	}
}

func benchmarkEyeSuiteFields(
	result eyesuite.Result,
) ([]benchmarkField, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	var generic any

	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}

	fields := []benchmarkField{}

	benchmarkFlattenExtracted(
		generic,
		"",
		&fields,
	)

	return fields, nil
}

func benchmarkExtractFieldsHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	headers, err := intakeHeaders(w, r)
	if err != nil {
		writeError(w, err.status, err.message)
		return
	}

	defer r.MultipartForm.RemoveAll()

	files, readErr := readIntakeFiles(headers)
	if readErr != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"não foi possível ler o arquivo",
		)
		return
	}

	if len(files) != 1 {
		writeError(
			w,
			http.StatusBadRequest,
			"selecione exatamente um arquivo",
		)
		return
	}

	file := files[0]

	meta, nameErr := parseBenchmarkFilename(
		file.Metadata.Filename,
	)

	if nameErr != nil {
		writeError(
			w,
			http.StatusBadRequest,
			nameErr.Error(),
		)
		return
	}

	flusher, ok := w.(http.Flusher)

	if !ok {
		writeError(
			w,
			http.StatusInternalServerError,
			"stream de progresso indisponível",
		)
		return
	}

	w.Header().Set(
		"Content-Type",
		"application/x-ndjson; charset=utf-8",
	)
	w.Header().Set(
		"Cache-Control",
		"no-cache, no-store",
	)
	w.Header().Set(
		"X-Accel-Buffering",
		"no",
	)

	encoder := json.NewEncoder(w)

	emit := func(event benchmarkFieldsEvent) {
		_ = encoder.Encode(event)
		flusher.Flush()
	}

	ctx := progress.WithReporter(
		r.Context(),
		func(event progress.Event) {
			emit(
				benchmarkFieldsEvent{
					Type:     "progress",
					Percent:  event.Percent,
					Message:  event.Message,
					ExamType: meta.ExamType,
					Eye:      meta.Eye,
				},
			)
		},
	)

	emit(
		benchmarkFieldsEvent{
			Type:    "progress",
			Percent: 3,
			Message: fmt.Sprintf(
				"Filename → %s %s",
				meta.ExamType,
				meta.Eye,
			),
			ExamType: meta.ExamType,
			Eye:      meta.Eye,
		},
	)

	started := time.Now()

	switch meta.ExamType {
	case "PENTACAM":
		if meta.Eye != "OD" && meta.Eye != "OS" {
			emit(
				benchmarkFieldsEvent{
					Type:  "error",
					Error: "Pentacam precisa usar lateralidade OD ou OS",
				},
			)
			return
		}

		if file.Metadata.ContentType != "application/pdf" {
			emit(
				benchmarkFieldsEvent{
					Type:  "error",
					Error: "Pentacam precisa ser PDF",
				},
			)
			return
		}

		emit(
			benchmarkFieldsEvent{
				Type:    "progress",
				Percent: 7,
				Message: fmt.Sprintf(
					"Executando parser Pentacam %s",
					meta.Eye,
				),
				ExamType: meta.ExamType,
				Eye:      meta.Eye,
			},
		)

		exam, extractErr := pentacam.ExtractPDF(
			progress.WithRange(ctx, 7, 96),
			file.Data,
		)

		if extractErr != nil {
			emit(
				benchmarkFieldsEvent{
					Type:  "error",
					Error: extractErr.Error(),
				},
			)
			return
		}

		fields, extracted :=
			benchmarkPentacamResult(exam)

		emit(
			benchmarkFieldsEvent{
				Type:    "result",
				Percent: 100,
				Message: fmt.Sprintf(
					"%d/%d campos extraídos",
					extracted,
					len(fields),
				),
				ExamType:  meta.ExamType,
				Eye:       meta.Eye,
				ElapsedMS: time.Since(started).Milliseconds(),
				Fields:    fields,
				Extracted: extracted,
				Total:     len(fields),
				Supported: true,
			},
		)

	case "EYESUITE":
		if file.Metadata.ContentType != "application/pdf" {
			emit(
				benchmarkFieldsEvent{
					Type:  "error",
					Error: "EyeSuite precisa ser PDF",
				},
			)
			return
		}

		emit(
			benchmarkFieldsEvent{
				Type:     "progress",
				Percent:  7,
				Message:  "Executando parser EyeSuite",
				ExamType: meta.ExamType,
				Eye:      meta.Eye,
			},
		)

		result, extractErr := eyesuite.ExtractPDF(
			progress.WithRange(ctx, 7, 96),
			file.Data,
		)

		if extractErr != nil {
			emit(
				benchmarkFieldsEvent{
					Type:  "error",
					Error: extractErr.Error(),
				},
			)
			return
		}

		fields, fieldsErr :=
			benchmarkEyeSuiteFields(result)

		if fieldsErr != nil {
			emit(
				benchmarkFieldsEvent{
					Type:  "error",
					Error: fieldsErr.Error(),
				},
			)
			return
		}

		emit(
			benchmarkFieldsEvent{
				Type:    "result",
				Percent: 100,
				Message: fmt.Sprintf(
					"%d campos retornados",
					len(fields),
				),
				ExamType:  meta.ExamType,
				Eye:       meta.Eye,
				ElapsedMS: time.Since(started).Milliseconds(),
				Fields:    fields,
				Extracted: len(fields),
				Total:     len(fields),
				Supported: true,
			},
		)

	case "MICROSCOPIA_ESPECULAR":
		fields, extracted := benchmarkSpecularMicroscopyResult()

		emit(
			benchmarkFieldsEvent{
				Type:    "result",
				Percent: 100,
				Message: fmt.Sprintf(
					"%d/%d campos extraídos; arquivo identificado pelo filename",
					extracted,
					len(fields),
				),
				ExamType:  meta.ExamType,
				Eye:       meta.Eye,
				ElapsedMS: time.Since(started).Milliseconds(),
				Fields:    fields,
				Extracted: extracted,
				Total:     len(fields),
				Supported: true,
			},
		)

	default:
		emit(
			benchmarkFieldsEvent{
				Type:      "result",
				Percent:   100,
				Message:   "Tipo identificado pelo filename; benchmark de extração ainda não configurado",
				ExamType:  meta.ExamType,
				Eye:       meta.Eye,
				ElapsedMS: time.Since(started).Milliseconds(),
				Supported: false,
			},
		)
	}
}
