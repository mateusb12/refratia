package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"refratia/backend/features/eyesuite"
	intakefeature "refratia/backend/features/intake"
	"refratia/backend/features/pentacam"
	"refratia/backend/features/specularmicroscopy"
	"refratia/backend/shared/progress"
)

type benchmarkField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Unit  string `json:"unit,omitempty"`
	Found bool   `json:"found"`
	Value string `json:"value,omitempty"`
}

func benchmarkValueText(value any) string {
	switch typed := value.(type) {
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	default:
		return fmt.Sprint(value)
	}
}

type benchmarkFieldsEvent struct {
	Type           string           `json:"type"`
	Percent        int              `json:"percent,omitempty"`
	Message        string           `json:"message,omitempty"`
	ExamType       string           `json:"examType,omitempty"`
	Eye            string           `json:"eye,omitempty"`
	ElapsedMS      int64            `json:"elapsedMs,omitempty"`
	Fields         []benchmarkField `json:"fields,omitempty"`
	Extracted      int              `json:"extracted,omitempty"`
	Total          int              `json:"total,omitempty"`
	Supported      bool             `json:"supported,omitempty"`
	Error          string           `json:"error,omitempty"`
	TaskIdentifier string           `json:"taskId,omitempty"`
	TaskLabel      string           `json:"taskLabel,omitempty"`
	TaskStatus     string           `json:"taskStatus,omitempty"`
	TaskPercent    int              `json:"taskPercent,omitempty"`
}

type benchmarkFilename struct {
	ExamType string
	Eye      string
}

func parseBenchmarkFilename(name string) (benchmarkFilename, error) {
	if examType, eye, ok := intakefeature.CanonicalClassificationFromFilename(name); ok {
		mappedType := map[string]string{
			"pentacam_corneal_tomography": "PENTACAM",
			"refractometry":               "AUTO",
			"iol_calculation":             "EYESUITE",
			"fundus_retinography":         "RETINA",
			"specular_microscopy":         "MICROSCOPIA_ESPECULAR",
		}[examType]

		if mappedType != "" {
			return benchmarkFilename{ExamType: mappedType, Eye: eye}, nil
		}
	}

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
		"AUTO",
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

var refractometryBenchmarkFields = []struct {
	key   string
	label string
	path  []string
}{
	{key: "sphere", label: "Esfera", path: []string{"sphere_d"}},
	{key: "cylinder", label: "Cilindro", path: []string{"cylinder_d"}},
	{key: "axis", label: "Eixo", path: []string{"axis_deg"}},
}

func benchmarkRefractometryResult(
	exam map[string]any,
	expectedEye string,
) ([]benchmarkField, int) {
	eyes := []string{expectedEye}
	if expectedEye == "AO" {
		eyes = []string{"OD", "OS"}
	}

	fields := make([]benchmarkField, 0, len(eyes)*len(refractometryBenchmarkFields))
	extracted := 0
	for _, eye := range eyes {
		eyePayload, _ := exam["eyes"].(map[string]any)
		currentEye, _ := eyePayload[eye].(map[string]any)
		for _, spec := range refractometryBenchmarkFields {
			value, found := benchmarkPathValue(currentEye, spec.path)
			if found {
				extracted++
			}
			fields = append(fields, benchmarkField{
				Key:   spec.key + "_" + strings.ToLower(eye),
				Label: spec.label + " · " + eye,
				Found: found,
				Value: func() string {
					if !found {
						return ""
					}
					return benchmarkValueText(value)
				}(),
			})
		}
	}
	return fields, extracted
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
		Key:   "ihd",
		Label: "IHD",
		Path:  []string{"topometric_indices_8mm", "ihd"},
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
	Path  []string
}

var specularMicroscopyBenchmarkFields = []specularMicroscopyBenchmarkSpec{
	{
		Key:   "cell_density",
		Label: "Densidade endotelial",
		Unit:  "células/mm²",
		Path:  []string{"cell_density_cells_per_mm2"},
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
		value, found := benchmarkPathValue(
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
				Value: func() string {
					if !found {
						return ""
					}
					return benchmarkValueText(value)
				}(),
			},
		)
	}

	return fields, extracted
}

func benchmarkSpecularMicroscopyResult(exam map[string]any, expectedEye string) ([]benchmarkField, int) {
	eyes := []string{expectedEye}
	if expectedEye == "AO" {
		eyes = []string{"OD", "OS"}
	}

	fields := make([]benchmarkField, 0, len(eyes))
	extracted := 0
	for _, eye := range eyes {
		spec := specularMicroscopyBenchmarkFields[0]
		path := append([]string{"eyes", eye}, spec.Path...)
		value, found := benchmarkPathValue(exam, path)
		fields = append(fields, benchmarkField{
			Key:   spec.Key + "_" + strings.ToLower(eye),
			Label: spec.Label + " · " + eye,
			Unit:  spec.Unit,
			Found: found,
			Value: func() string {
				if !found {
					return ""
				}
				return benchmarkValueText(value)
			}(),
		})
		if found {
			extracted++
		}
	}
	return fields, extracted
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
				Value: benchmarkValueText(value),
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

	writeEvent := func(event benchmarkFieldsEvent) {
		_ = encoder.Encode(event)
		flusher.Flush()
	}

	activeTaskIdentifier := ""
	activeTaskLabel := ""
	emitTask := func(taskIdentifier, taskLabel, taskStatus string, taskPercent int) {
		writeEvent(
			benchmarkFieldsEvent{
				Type:           "task_progress",
				TaskIdentifier: taskIdentifier,
				TaskLabel:      taskLabel,
				TaskStatus:     taskStatus,
				TaskPercent:    taskPercent,
				ExamType:       meta.ExamType,
				Eye:            meta.Eye,
			},
		)
	}

	emit := func(event benchmarkFieldsEvent) {
		if event.Type == "result" && activeTaskIdentifier != "" {
			emitTask(
				activeTaskIdentifier,
				activeTaskLabel,
				progress.TaskStatusCompleted,
				100,
			)
			activeTaskIdentifier = ""
			activeTaskLabel = ""
		}

		if event.Type == "error" && activeTaskIdentifier != "" {
			emitTask(
				activeTaskIdentifier,
				activeTaskLabel,
				progress.TaskStatusFailed,
				100,
			)
			activeTaskIdentifier = ""
			activeTaskLabel = ""
		}

		writeEvent(event)
	}

	completeCurrentTask := func() {
		if activeTaskIdentifier == "" {
			return
		}

		emitTask(
			activeTaskIdentifier,
			activeTaskLabel,
			progress.TaskStatusCompleted,
			100,
		)
		activeTaskIdentifier = ""
		activeTaskLabel = ""
	}

	// The parser can report progress from concurrent page workers. The
	// ResponseWriter, json.Encoder, and active task state must be updated as a
	// single serialized stream; concurrent writes corrupt HTTP/1.1 chunking.
	var progressEventMutex sync.Mutex

	ctx := progress.WithReporter(
		r.Context(),
		func(event progress.Event) {
			progressEventMutex.Lock()
			defer progressEventMutex.Unlock()

			if event.Type == "task_progress" {
				emit(
					benchmarkFieldsEvent{
						Type:           event.Type,
						Percent:        event.Percent,
						Message:        event.Message,
						ExamType:       meta.ExamType,
						Eye:            meta.Eye,
						TaskIdentifier: event.TaskIdentifier,
						TaskLabel:      event.TaskLabel,
						TaskStatus:     event.TaskStatus,
						TaskPercent:    event.TaskPercent,
					},
				)
				return
			}

			if event.Type == "progress" && event.Message != "" {
				stageTaskIdentifier := "stage:" + event.Stage + ":" + event.Message
				if stageTaskIdentifier != activeTaskIdentifier {
					completeCurrentTask()
					activeTaskIdentifier = stageTaskIdentifier
					activeTaskLabel = event.Message
					emitTask(
						activeTaskIdentifier,
						activeTaskLabel,
						progress.TaskStatusRunning,
						0,
					)
				}
			}

			emit(
				benchmarkFieldsEvent{
					Type:           event.Type,
					Percent:        event.Percent,
					Message:        event.Message,
					ExamType:       meta.ExamType,
					Eye:            meta.Eye,
					TaskIdentifier: event.TaskIdentifier,
					TaskLabel:      event.TaskLabel,
					TaskStatus:     event.TaskStatus,
					TaskPercent:    event.TaskPercent,
				},
			)
		},
	)
	ctx = progress.WithFilename(ctx, file.Metadata.Filename)

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

	case "AUTO":
		if meta.Eye != "OD" && meta.Eye != "OS" && meta.Eye != "AO" {
			emit(benchmarkFieldsEvent{Type: "error", Error: "AUTO precisa usar lateralidade OD, OS ou AO"})
			return
		}

		emit(benchmarkFieldsEvent{
			Type: "progress", Percent: 7,
			Message:  "Executando leitura limpa de refratometria (sem cache)",
			ExamType: meta.ExamType, Eye: meta.Eye,
		})

		// O benchmark chama a leitura diretamente, sem contexto de checkpoint,
		// para medir esta execução como se fosse a primeira.
		prompt := fmt.Sprintf(`Leia somente o arquivo de refratometria %q.
Retorne somente JSON neste formato:
{"exams":{"refractometry":{"eyes":{"OD":{"sphere_d":null,"cylinder_d":null,"axis_deg":null},"OS":{"sphere_d":null,"cylinder_d":null,"axis_deg":null}}}}}
Extraia apenas valores realmente legíveis. Preserve sinais. Se um olho não estiver presente, mantenha seus campos como null. Não faça cálculos nem use qualquer resultado anterior.`, file.Metadata.Filename)

		raw, extractErr := requestOpenAIJSON(ctx, []uploadedFile{file}, prompt, 2000)
		if extractErr != nil {
			emit(benchmarkFieldsEvent{Type: "error", Error: extractErr.Error()})
			return
		}

		var analysis map[string]any
		if err := json.Unmarshal([]byte(raw), &analysis); err != nil {
			emit(benchmarkFieldsEvent{Type: "error", Error: "o benchmark retornou JSON inválido"})
			return
		}

		exams, _ := analysis["exams"].(map[string]any)
		exam, _ := exams["refractometry"].(map[string]any)
		fields, extracted := benchmarkRefractometryResult(exam, meta.Eye)
		emit(benchmarkFieldsEvent{
			Type: "result", Percent: 100,
			Message:  fmt.Sprintf("%d/%d campos extraídos", extracted, len(fields)),
			ExamType: meta.ExamType, Eye: meta.Eye,
			ElapsedMS: time.Since(started).Milliseconds(),
			Fields:    fields, Extracted: extracted, Total: len(fields), Supported: true,
		})

	case "MICROSCOPIA_ESPECULAR":
		if meta.Eye != "OD" && meta.Eye != "OS" && meta.Eye != "AO" {
			emit(benchmarkFieldsEvent{Type: "error", Error: "Microscopia especular precisa usar lateralidade OD, OS ou AO"})
			return
		}
		emit(benchmarkFieldsEvent{Type: "progress", Percent: 7, Message: "Executando extrator local de microscopia especular", ExamType: meta.ExamType, Eye: meta.Eye})
		result, extractErr := specularmicroscopy.Extract(progress.WithRange(ctx, 7, 96), file.Data)
		if extractErr != nil {
			if errors.Is(extractErr, specularmicroscopy.ErrNoClinicalData) {
				fields, extracted := benchmarkSpecularMicroscopyResult(map[string]any{}, meta.Eye)
				emit(benchmarkFieldsEvent{
					Type: "result", Percent: 100,
					Message:  fmt.Sprintf("%d/%d campos extraídos; conteúdo clínico não encontrado", extracted, len(fields)),
					ExamType: meta.ExamType, Eye: meta.Eye, ElapsedMS: time.Since(started).Milliseconds(),
					Fields: fields, Extracted: extracted, Total: len(fields), Supported: true,
				})
				return
			}
			emit(benchmarkFieldsEvent{Type: "error", Error: extractErr.Error()})
			return
		}
		fields, extracted := benchmarkSpecularMicroscopyResult(result.Exam, meta.Eye)

		emit(
			benchmarkFieldsEvent{
				Type:    "result",
				Percent: 100,
				Message: fmt.Sprintf(
					"%d/%d campos extraídos; conteúdo clínico lido do JPEG",
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
