package intake

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gorm.io/gorm"

	"refratia/backend/src/features/fileIdentification/classification"
	"refratia/backend/src/features/persistence/cases"
	"refratia/backend/src/features/persistence/exams"
	patientfeature "refratia/backend/src/features/persistence/patient"
	"refratia/backend/src/shared/database"
	"refratia/backend/src/shared/fileStorage"
	"refratia/backend/src/shared/httpresponse"
	progress "refratia/backend/src/shared/progress"
)

const (
	maxFileSize   = 20 << 20
	maxIntakeSize = 50 << 20
)

type storedIntake struct {
	CreatedAt time.Time      `json:"createdAt"`
	Files     []FileMetadata `json:"files"`
	Analysis  map[string]any `json:"analysis"`
}

// Handler serves the intake routes that need SQL.
type Handler struct {
	Database *gorm.DB
	// afterGarageWrites is a test seam: it runs inside the commit transaction after
	// the Garage writes and before the SQL rows, so tests can fail that step.
	afterGarageWrites func() error
}

type intakeError struct {
	status  int
	message string
	body    map[string]any // optional structured response (409 identity_review_required)
}

// identityDecision answers an identity_review_required (409): merge into the
// candidate patient ("existing" + caseId) or knowingly create another ("new").
type identityDecision struct {
	Action string `json:"action"`
	CaseID string `json:"caseId"`
}

func InspectIntakeHandler(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	headers, requestError := intakeHeaders(responseWriter, request)
	if requestError != nil {
		httpresponse.WriteError(responseWriter, requestError.status, requestError.message)
		return
	}
	defer request.MultipartForm.RemoveAll()

	files, readErr := readIntakeFiles(headers)
	if readErr != nil {
		httpresponse.WriteError(responseWriter, http.StatusBadRequest, "não foi possível ler um dos arquivos")
		return
	}

	result := make([]classification.FileInspection, 0, len(files))
	for _, file := range files {
		result = append(result, classification.InspectFile(request.Context(), classification.File{
			Filename:    file.Metadata.Filename,
			SHA256:      file.Metadata.SHA256,
			ContentType: file.Metadata.ContentType,
			Size:        file.Metadata.Size,
			Data:        file.Data,
		}))
	}

	responseWriter.Header().Set("Content-Type", "application/json")
	json.NewEncoder(responseWriter).Encode(map[string]any{"files": result})
}

func (handler Handler) AnalyzeIntake(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	headers, requestError := intakeHeaders(responseWriter, request)
	if requestError != nil {
		httpresponse.WriteError(responseWriter, requestError.status, requestError.message)
		return
	}

	defer request.MultipartForm.RemoveAll()

	files, readErr := readIntakeFiles(headers)
	if readErr != nil {
		httpresponse.WriteError(
			responseWriter,
			http.StatusBadRequest,
			"não foi possível ler um dos arquivos",
		)
		return
	}

	classifications, classificationErr := classification.ParseClassifications(
		request.MultipartForm.Value["classifications"],
	)
	if classificationErr != nil {
		httpresponse.WriteError(
			responseWriter,
			http.StatusBadRequest,
			"classificação de arquivos inválida",
		)
		return
	}

	var processFiles map[string]bool
	if rawProcess := request.MultipartForm.Value["processFiles"]; len(rawProcess) > 0 {
		var sha256Values []string
		if errorValue := json.Unmarshal([]byte(rawProcess[0]), &sha256Values); errorValue != nil {
			httpresponse.WriteError(responseWriter, http.StatusBadRequest, "seleção de processamento inválida")
			return
		}

		processFiles = make(map[string]bool, len(sha256Values))
		for _, sha256Value := range sha256Values {
			processFiles[sha256Value] = true
		}
	}

	for index := range files {
		fileClassification, ok := classifications[files[index].Metadata.SHA256]
		if ok {
			files[index].Metadata.ExamType = fileClassification.ExamType
			files[index].Metadata.Eye = fileClassification.Eye
			continue
		}

		// O filename é um fallback seguro para o novo padrão de nomenclatura.
		// Uma classificação manual enviada pelo frontend sempre tem prioridade.
		if examType, eye, filenameOK := classification.CanonicalClassificationFromFilename(
			files[index].Metadata.Filename,
		); filenameOK {
			// Metadata.ExamType usa o mesmo vocabulário (rótulo) da classificação
			// explícita do frontend; o fallback devolvia a chave do contrato (F14).
			if label, labelOK := classification.ExamTypeFromContract(examType); labelOK {
				examType = label
			}
			files[index].Metadata.ExamType = examType
			files[index].Metadata.Eye = eye
		}
	}

	// D11: o backend não persiste arquivo sem exame+olho (antes só a UI exigia).
	unclassified := make([]string, 0)
	for _, file := range files {
		if file.Metadata.ExamType == "" || file.Metadata.Eye == "" {
			unclassified = append(unclassified, file.Metadata.Filename)
		}
	}
	if len(unclassified) > 0 {
		httpresponse.WriteError(
			responseWriter,
			http.StatusUnprocessableEntity,
			"classifique exame e lateralidade de todos os arquivos: "+strings.Join(unclassified, ", "),
		)
		return
	}

	requestContext := request.Context()
	var stream *progressStream

	if request.URL.Query().Get("stream") == "1" {
		var ok bool

		stream, ok = newProgressStream(responseWriter)
		if !ok {
			httpresponse.WriteError(
				responseWriter,
				http.StatusInternalServerError,
				"stream de progresso indisponível",
			)
			return
		}

		requestContext = progress.WithReporter(
			requestContext,
			stream.reporter,
		)

		progress.Report(
			requestContext,
			4,
			"upload",
			"Arquivos recebidos",
		)
	}

	fail := func(status int, message string) {
		if stream != nil {
			stream.writeError(status, message)
			return
		}

		httpresponse.WriteError(responseWriter, status, message)
	}

	client, storageClientError :=
		fileStorage.New(requestContext)

	if storageClientError != nil {
		fail(
			http.StatusInternalServerError,
			"storage indisponível",
		)
		return
	}

	cleanupExpiredDrafts(
		requestContext,
		client,
	)

	requestContext = withFileCheckpointStorage(
		requestContext,
		client,
		os.Getenv("BUCKET_NAME"),
		request.URL.Query().Get("force") == "1",
		processFiles,
	)

	progress.Report(
		requestContext,
		8,
		"ocr",
		"Iniciando extração local",
	)

	analysis, extractionErr := extractPatient(
		progress.WithRange(requestContext, 8, 88),
		files,
	)

	if extractionErr != nil {
		fail(
			http.StatusBadGateway,
			extractionErr.Error(),
		)
		return
	}

	// O preview também precisa saber qual exame/lateralidade cada arquivo
	// alimentou. Antes isso só era feito na confirmação, fazendo a tela de
	// debug exibir "sem campos" mesmo quando os valores estavam em analysis.exams.
	exams.ClassifySourceFiles(analysis)

	// O nascimento canônico final (leitura do Pentacam em caso de inversão)
	// precisa estar em patient.birth_date: a extração incremental pode tê-lo
	// fixado só com o primeiro documento lido.
	patientfeature.NormalizeIdentityFields(analysis)

	progress.Report(
		requestContext,
		90,
		"identity",
		"Verificando identidade do paciente",
	)

	intakeID := fmt.Sprintf(
		"intake-%s-%s",
		time.Now().UTC().Format("20060102T150405Z"),
		randomToken(),
	)

	patientMatch := map[string]any{
		"status": "unresolved",
	}

	var changePreview any

	_, identifiable := patientfeature.Identity(analysis)

	var existingCaseID string
	var existingAnalysis map[string]any
	var foundExisting bool

	if warnings := patientfeature.IdentityWarnings(analysis); len(warnings) > 0 {
		list := make([]any, 0, len(warnings))
		for _, warning := range warnings {
			list = append(list, map[string]any(warning))
		}
		analysis["identity_warnings"] = list
	}

	if identifiable {
		patientMatch["status"] = "new"

		var findErr error

		existingCaseID,
			existingAnalysis,
			foundExisting,
			findErr = cases.FindExistingPatientCase(
			requestContext,
			client,
			os.Getenv("BUCKET_NAME"),
			analysis,
		)

		if findErr != nil {
			fail(
				http.StatusBadGateway,
				"não foi possível verificar pacientes existentes",
			)
			return
		}
	}

	if identifiable && !foundExisting {
		// Mesmo nascimento, nome diferente: possível erro de OCR ou homônimo.
		// Sinalizado para revisão; nunca fundido nem "adivinhado".
		key, _ := patientfeature.Identity(analysis)
		birth, _ := patientfeature.CanonicalBirthDate(analysis)
		similar, similarErr := patientfeature.SimilarByBirthDate(handler.Database, key, birth)
		if similarErr != nil {
			fail(http.StatusInternalServerError, "não foi possível verificar pacientes existentes")
			return
		}
		for _, other := range similar {
			warnings, _ := analysis["identity_warnings"].([]any)
			analysis["identity_warnings"] = append(warnings, map[string]any{
				"code":   "nome_divergente_paciente_existente",
				"caseId": other.CaseID,
				"name":   other.FullName,
				"detail": "já existe paciente com a mesma data de nascimento e nome diferente; revisar antes de confirmar",
			})
		}
	}

	if foundExisting {
		patientMatch["status"] = "existing"
		patientMatch["caseId"] = existingCaseID

		changePreview = patientfeature.BuildChangePreview(
			existingAnalysis,
			analysis,
		)

		if patient, ok := existingAnalysis["patient"].(map[string]any); ok {
			if name, ok := patient["full_name"].(string); ok {
				patientMatch["patientName"] = name
			}
		}
	}

	progress.Report(
		requestContext,
		93,
		"storage",
		"Preparando rascunho",
	)

	storedFiles := make([]FileMetadata, 0, len(files))
	keys := make([]string, 0, len(files)+1)

	for index, uploaded := range files {
		percent := 94

		if len(files) > 0 {
			percent += index * 4 / len(files)
		}

		progress.Report(
			requestContext,
			percent,
			"storage",
			fmt.Sprintf(
				"Armazenando documento %d/%d",
				index+1,
				len(files),
			),
		)

		metadata := uploaded.Metadata
		patientName := ""
		if patient, ok := analysis["patient"].(map[string]any); ok {
			patientName, _ = patient["full_name"].(string)
		}
		extension := strings.TrimPrefix(filepath.Ext(metadata.Filename), ".")
		if extension == "" {
			extension = "bin"
		}
		metadata.CanonicalFilename = classification.CanonicalExamFilename(patientName, metadata.ExamType, metadata.Eye, extension, metadata.SHA256)
		storageName := metadata.CanonicalFilename
		key := fmt.Sprintf(fileStorage.DraftsPrefix+"%s/%s", intakeID, storageName)

		_, putErr := client.PutObject(
			requestContext,
			&s3.PutObjectInput{
				Bucket: aws.String(os.Getenv("BUCKET_NAME")),
				Key:    aws.String(key),
				Body: bytes.NewReader(
					uploaded.Data,
				),
				ContentType: aws.String(
					uploaded.Metadata.ContentType,
				),
			},
		)

		if putErr != nil {
			fileStorage.CleanupObjects(requestContext, client, keys)

			fail(
				http.StatusBadGateway,
				"não foi possível armazenar o rascunho",
			)
			return
		}

		keys = append(keys, key)

		metadata.Key = key

		storedFiles = append(
			storedFiles,
			metadata,
		)
	}

	draft, marshalErr := json.Marshal(
		storedIntake{
			CreatedAt: time.Now().UTC(),
			Files:     storedFiles,
			Analysis:  analysis,
		},
	)

	if marshalErr != nil {
		fileStorage.CleanupObjects(requestContext, client, keys)

		fail(
			http.StatusInternalServerError,
			"não foi possível montar o rascunho",
		)
		return
	}

	draftKey := fmt.Sprintf(
		fileStorage.DraftsPrefix+"%s/intake.json",
		intakeID,
	)

	if _, putErr := client.PutObject(
		requestContext,
		&s3.PutObjectInput{
			Bucket: aws.String(
				os.Getenv("BUCKET_NAME"),
			),
			Key: aws.String(draftKey),
			Body: bytes.NewReader(
				draft,
			),
			ContentType: aws.String(
				"application/json",
			),
		},
	); putErr != nil {
		fileStorage.CleanupObjects(requestContext, client, keys)

		fail(
			http.StatusBadGateway,
			"não foi possível armazenar o rascunho",
		)
		return
	}

	progress.Report(
		requestContext,
		99,
		"preview",
		"Gerando pré-visualização",
	)

	presigner, presignErr := fileStorage.NewPresigner(requestContext)
	if presignErr != nil {
		fail(
			http.StatusInternalServerError,
			"storage indisponível",
		)
		return
	}

	previewFiles := intakeMetadata(files)

	for index := range previewFiles {
		presignedRequest, errorValue := presigner.PresignGetObject(
			requestContext,
			&s3.GetObjectInput{
				Bucket: aws.String(
					os.Getenv("BUCKET_NAME"),
				),
				Key: aws.String(
					storedFiles[index].Key,
				),
			},
		)

		if errorValue == nil {
			previewFiles[index].SignedURL = presignedRequest.URL
		}
	}

	payload := map[string]any{
		"intakeId":      intakeID,
		"files":         previewFiles,
		"analysis":      analysis,
		"patientMatch":  patientMatch,
		"changePreview": changePreview,
		"message":       "Documentos e análise armazenados. Confira a extração antes de confirmar.",
	}

	progress.Report(
		requestContext,
		100,
		"complete",
		"Análise concluída",
	)

	if stream != nil {
		stream.writeResult(payload)
		return
	}

	responseWriter.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(responseWriter).Encode(payload)
}

func (handler Handler) ConfirmIntake(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	request.Body = http.MaxBytesReader(responseWriter, request.Body, 1<<20)
	var confirmRequest struct {
		IntakeID         string            `json:"intakeId"`
		IdentityDecision *identityDecision `json:"identityDecision"`
	}
	if json.NewDecoder(request.Body).Decode(&confirmRequest) != nil || !ValidID(confirmRequest.IntakeID) {
		httpresponse.WriteError(responseWriter, http.StatusBadRequest, "rascunho inválido")
		return
	}

	client, storageErr := fileStorage.New(request.Context())
	if storageErr != nil {
		httpresponse.WriteError(responseWriter, http.StatusInternalServerError, "storage indisponível")
		return
	}

	bucket := os.Getenv("BUCKET_NAME")

	// Retry da mesma confirmação deve devolver exatamente o case já resolvido,
	// inclusive quando aquele intake foi incorporado em um paciente preexistente.
	if receipt, ok := cases.LoadConfirmationReceipt(request.Context(), client, bucket, confirmRequest.IntakeID); ok {
		writeConfirmation(responseWriter, receipt.CaseID, receipt.AnalysisKey, receipt.Action)
		return
	}

	defaultCaseID := "case-" + strings.TrimPrefix(confirmRequest.IntakeID, "intake-")
	defaultAnalysisKey := fmt.Sprintf(fileStorage.CasesPrefix+"%s/paciente_compilado.json", defaultCaseID)
	draftKey := fmt.Sprintf(fileStorage.DraftsPrefix+"%s/intake.json", confirmRequest.IntakeID)

	object, getErr := client.GetObject(request.Context(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(draftKey),
	})
	if getErr != nil {
		// Compatibilidade com confirmações criadas antes do receipt.
		if _, headErr := client.HeadObject(request.Context(), &s3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(defaultAnalysisKey),
		}); headErr == nil {
			writeConfirmation(responseWriter, defaultCaseID, defaultAnalysisKey, "created")
			return
		}

		httpresponse.WriteError(responseWriter, http.StatusNotFound, "rascunho não encontrado")
		return
	}

	var draft storedIntake
	decodeErr := json.NewDecoder(
		io.LimitReader(object.Body, fileStorage.MaxAnalysisSize+(1<<20)),
	).Decode(&draft)
	object.Body.Close()

	if decodeErr != nil ||
		time.Since(draft.CreatedAt) > 24*time.Hour ||
		validateStoredAnalysis(draft.Analysis, draft.Files) != nil {
		httpresponse.WriteError(responseWriter, http.StatusBadRequest, "rascunho inválido ou expirado")
		return
	}

	// C2: identidade validada ANTES de qualquer escrita no Garage.
	if _, _, _, identityErr := patientfeature.IdentityFromAnalysis(draft.Analysis); identityErr != nil {
		httpresponse.WriteError(responseWriter, http.StatusUnprocessableEntity, "não foi possível salvar o paciente: nome ou data de nascimento estão incompletos ou conflitantes")
		return
	}
	for _, file := range draft.Files {
		if file.ExamType == "" || file.Eye == "" {
			httpresponse.WriteError(responseWriter, http.StatusUnprocessableEntity, "arquivo sem exame e lateralidade classificados")
			return
		}
	}

	receipt, commitErr := handler.commitIntake(request.Context(), client, bucket, draft, defaultCaseID, confirmRequest.IdentityDecision)
	if commitErr != nil {
		var httpErr *intakeError
		switch {
		case errors.As(commitErr, &httpErr) && httpErr.body != nil:
			responseWriter.Header().Set("Content-Type", "application/json")
			responseWriter.WriteHeader(httpErr.status)
			json.NewEncoder(responseWriter).Encode(httpErr.body)
		case errors.As(commitErr, &httpErr):
			httpresponse.WriteError(responseWriter, httpErr.status, httpErr.message)
		case errors.Is(commitErr, patientfeature.ErrIdentity):
			httpresponse.WriteError(responseWriter, http.StatusUnprocessableEntity, "não foi possível salvar o paciente: nome ou data de nascimento estão incompletos ou conflitantes")
		default:
			log.Printf("confirm %s: %v", confirmRequest.IntakeID, commitErr)
			httpresponse.WriteError(responseWriter, http.StatusInternalServerError, "não foi possível salvar o paciente")
		}
		return
	}
	caseID, analysisKey, action := receipt.CaseID, receipt.AnalysisKey, receipt.Action

	// O receipt precisa existir antes de consumirmos o draft.
	// Se sua gravação falhar, o case já pode ter sido atualizado, mas o draft
	// permanece disponível para um retry idempotente da mesma confirmação.
	if receiptErr := cases.StoreConfirmationReceipt(
		request.Context(),
		client,
		bucket,
		confirmRequest.IntakeID,
		receipt,
	); receiptErr != nil {
		httpresponse.WriteError(responseWriter, http.StatusBadGateway, "não foi possível finalizar a confirmação")
		return
	}

	draftKeys := make([]string, 0, len(draft.Files)+1)
	for _, file := range draft.Files {
		draftKeys = append(draftKeys, file.Key)
	}
	fileStorage.CleanupObjects(request.Context(), client, append(draftKeys, draftKey))

	writeConfirmation(responseWriter, caseID, analysisKey, action)
}

func (failure *intakeError) Error() string { return failure.message }

// commitIntake is the commit protocol (C3). Garage objects live at keys that
// are deterministic per intake; the case marker (paciente_compilado.json) is
// written next; everything in SQL (patient + exam rows) commits in ONE
// transaction. The patient row lock (SELECT ... FOR UPDATE) is the mutex for
// the read-merge-write of the compiled JSON, so concurrent confirms of the same
// patient serialize instead of creating two cases or losing an update. A
// failure after Garage writes rolls SQL back; the objects are then garbage that
// a retry of the same intake overwrites (cmd/reconcile lists the rest).
// ponytail: the row lock is held across Garage I/O (fine at clinic scale);
// switch to If-Match writes if contention ever shows.
func (handler Handler) commitIntake(
	executionContext context.Context,
	client fileStorage.Repository,
	bucket string,
	draft storedIntake,
	defaultCaseID string,
	decision *identityDecision,
) (cases.ConfirmationReceipt, error) {
	identityKey, fullName, birthDate, errorValue := patientfeature.IdentityFromAnalysis(draft.Analysis)
	if errorValue != nil {
		return cases.ConfirmationReceipt{}, errorValue
	}
	if handler.Database == nil {
		return cases.ConfirmationReceipt{}, errors.New("database is not initialized")
	}
	patientfeature.NormalizeIdentityFields(draft.Analysis)

	// C2: same birth date + similar-but-different name and no exact match is a
	// possible OCR typo: ask for a decision, write nothing (no Garage, no SQL).
	var exact int64
	if errorValue := handler.Database.Model(&database.Patient{}).Where("identity_key = ?", identityKey).Count(&exact).Error; errorValue != nil {
		return cases.ConfirmationReceipt{}, errorValue
	}
	if exact == 0 {
		similar, errorValue := patientfeature.SimilarByBirthDate(handler.Database, identityKey, birthDate)
		if errorValue != nil {
			return cases.ConfirmationReceipt{}, errorValue
		}
		candidates := make([]database.Patient, 0)
		for _, other := range similar {
			if patientfeature.NamesSimilar(other.FullName, fullName) {
				candidates = append(candidates, other)
			}
		}
		if len(candidates) > 0 {
			switch {
			case decision != nil && decision.Action == "new":
			case decision != nil && decision.Action == "existing":
				var chosen *database.Patient
				for index := range candidates {
					if candidates[index].CaseID == decision.CaseID {
						chosen = &candidates[index]
					}
				}
				if chosen == nil {
					return cases.ConfirmationReceipt{}, &intakeError{status: http.StatusBadRequest, message: "caseId da decisão não é um candidato"}
				}
				// Keeps the existing identity_key/name; records the new reading.
				warnings, _ := draft.Analysis["identity_warnings"].([]any)
				draft.Analysis["identity_warnings"] = append(warnings, map[string]any{
					"code":   "nome_variante",
					"name":   fullName,
					"detail": "leitura de nome diferente mesclada ao paciente existente por decisão do revisor",
				})
				identityKey, fullName = chosen.IdentityKey, chosen.FullName
			default:
				list := make([]map[string]any, 0, len(candidates))
				for _, candidate := range candidates {
					list = append(list, map[string]any{"caseId": candidate.CaseID, "name": candidate.FullName})
				}
				return cases.ConfirmationReceipt{}, &intakeError{
					status:  http.StatusConflict,
					message: "identity_review_required",
					body:    map[string]any{"error": "identity_review_required", "candidates": list},
				}
			}
		}
	}

	var receipt cases.ConfirmationReceipt
	errorValue = handler.Database.Transaction(func(transaction *gorm.DB) error {
		patient, errorValue := patientfeature.Lock(transaction, identityKey, fullName, birthDate)
		if errorValue != nil {
			return errorValue
		}

		caseID := defaultCaseID
		action := "created"
		var existingAnalysis map[string]any
		if patient.CaseID != "" {
			stored, loadErr := cases.LoadStoredCaseAnalysis(executionContext, client, bucket, patient.CaseID)
			switch {
			case loadErr == nil:
				caseID, existingAnalysis, action = patient.CaseID, stored, "updated"
			case !fileStorage.IsNotFound(loadErr):
				return &intakeError{status: http.StatusBadGateway, message: "não foi possível verificar pacientes existentes"}
			}
		}
		if existingAnalysis == nil {
			// Fallback legado: case sem linha/case_id em SQL.
			legacyID, legacy, found, findErr := cases.FindExistingPatientCase(executionContext, client, bucket, draft.Analysis)
			if findErr != nil {
				return &intakeError{status: http.StatusBadGateway, message: "não foi possível verificar pacientes existentes"}
			}
			if found {
				caseID, existingAnalysis, action = legacyID, legacy, "updated"
			}
		}
		analysisKey := fmt.Sprintf(fileStorage.CasesPrefix+"%s/paciente_compilado.json", caseID)

		result := make([]FileMetadata, 0, len(draft.Files))
		copiedKeys := make([]string, 0, len(draft.Files))
		for _, file := range draft.Files {
			key := fmt.Sprintf(fileStorage.CasesPrefix+"%s/%s", caseID, filepath.Base(file.Key))
			_, copyErr := client.CopyObject(executionContext, &s3.CopyObjectInput{
				Bucket:            aws.String(bucket),
				Key:               aws.String(key),
				CopySource:        aws.String(url.PathEscape(bucket + "/" + file.Key)),
				ContentType:       aws.String(file.ContentType),
				MetadataDirective: "REPLACE",
			})
			if copyErr != nil {
				fileStorage.CleanupObjects(executionContext, client, copiedKeys)
				return &intakeError{status: http.StatusBadGateway, message: "não foi possível confirmar os arquivos"}
			}
			copiedKeys = append(copiedKeys, key)
			file.Key = key
			result = append(result, file)
		}

		setStoredPaths(draft.Analysis, result)
		finalAnalysis := draft.Analysis
		if existingAnalysis != nil {
			finalAnalysis = patientfeature.Merge(existingAnalysis, draft.Analysis)
		}
		exams.ClassifySourceFiles(finalAnalysis)

		compiled, marshalErr := json.MarshalIndent(finalAnalysis, "", "  ")
		if marshalErr != nil {
			fileStorage.CleanupObjects(executionContext, client, copiedKeys)
			return &intakeError{status: http.StatusInternalServerError, message: "não foi possível montar o JSON compilado"}
		}
		if _, putErr := client.PutObject(executionContext, &s3.PutObjectInput{
			Bucket:      aws.String(bucket),
			Key:         aws.String(analysisKey),
			Body:        bytes.NewReader(compiled),
			ContentType: aws.String("application/json"),
		}); putErr != nil {
			fileStorage.CleanupObjects(executionContext, client, copiedKeys)
			return &intakeError{status: http.StatusBadGateway, message: "não foi possível armazenar o JSON compilado"}
		}

		// Se exatamente o mesmo arquivo for reenviado, source_files é deduplicado
		// por SHA-256. Nesse caso não deixamos a cópia redundante órfã no bucket.
		referenced := patientfeature.ReferencedSourcePaths(finalAnalysis)
		orphanedCopies := make([]string, 0)
		rows := make([]database.Exam, 0, len(result))
		for _, file := range result {
			if !referenced[file.Key] {
				orphanedCopies = append(orphanedCopies, file.Key)
				continue
			}
			filename := file.OriginalFilename
			if filename == "" {
				filename = file.Filename
			}
			for _, eye := range exams.Eyes(file.Eye) {
				rows = append(rows, database.Exam{
					CaseID:      caseID,
					PatientID:   &patient.ID,
					PatientName: fullName,
					Filename:    filename,
					ExamType:    exams.TypeKey(file.ExamType),
					Eye:         eye,
					ContentType: file.ContentType,
					Bucket:      bucket,
					ObjectKey:   file.Key,
					Size:        file.Size,
					Referenced:  true,
				})
			}
		}
		fileStorage.CleanupObjects(executionContext, client, orphanedCopies)

		if handler.afterGarageWrites != nil {
			if errorValue := handler.afterGarageWrites(); errorValue != nil {
				return errorValue
			}
		}
		if errorValue := patientfeature.Save(transaction, patient, fullName, birthDate, caseID); errorValue != nil {
			return fmt.Errorf("update patient: %w", errorValue)
		}
		if errorValue := exams.UpsertRows(transaction, rows); errorValue != nil {
			return fmt.Errorf("upsert exams: %w", errorValue)
		}
		// Documentos substituídos no merge (mesmo exame+olho) saem do SQL
		// junto com o manifesto; arquivos manuais não referenciados ficam.
		stale := transaction.Where("case_id = ? AND referenced = ?", caseID, true)
		if len(referenced) > 0 {
			keys := make([]string, 0, len(referenced))
			for key := range referenced {
				keys = append(keys, key)
			}
			stale = stale.Where("object_key NOT IN ?", keys)
		}
		if errorValue := stale.Delete(&database.Exam{}).Error; errorValue != nil {
			return fmt.Errorf("prune exams: %w", errorValue)
		}

		receipt = cases.ConfirmationReceipt{CaseID: caseID, AnalysisKey: analysisKey, Action: action}
		return nil
	})
	return receipt, errorValue
}

func DeleteIntakeHandler(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodDelete {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	intakeID := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/intakes/"), "/")
	if !ValidID(intakeID) {
		httpresponse.WriteError(responseWriter, http.StatusBadRequest, "rascunho inválido")
		return
	}
	client, errorValue := fileStorage.New(request.Context())
	if errorValue != nil {
		httpresponse.WriteError(responseWriter, http.StatusInternalServerError, "storage indisponível")
		return
	}
	bucket := os.Getenv("BUCKET_NAME")
	draftKey := fmt.Sprintf(fileStorage.DraftsPrefix+"%s/intake.json", intakeID)
	object, errorValue := client.GetObject(request.Context(), &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(draftKey)})
	if errorValue != nil {
		responseWriter.WriteHeader(http.StatusNoContent)
		return
	}
	var draft storedIntake
	decodeErr := json.NewDecoder(io.LimitReader(object.Body, fileStorage.MaxAnalysisSize+(1<<20))).Decode(&draft)
	object.Body.Close()
	if decodeErr != nil {
		httpresponse.WriteError(responseWriter, http.StatusBadGateway, "não foi possível ler o rascunho")
		return
	}
	keys := []string{draftKey}
	prefix := fmt.Sprintf(fileStorage.DraftsPrefix+"%s/", intakeID)
	for _, file := range draft.Files {
		if strings.HasPrefix(file.Key, prefix) {
			keys = append(keys, file.Key)
		}
	}
	fileStorage.CleanupObjects(request.Context(), client, keys)
	responseWriter.WriteHeader(http.StatusNoContent)
}

func writeConfirmation(responseWriter http.ResponseWriter, caseID, analysisKey, action string) {
	responseWriter.Header().Set("Content-Type", "application/json")
	json.NewEncoder(responseWriter).Encode(map[string]any{
		"caseId":      caseID,
		"analysisKey": analysisKey,
		"action":      action,
	})
}

func intakeHeaders(responseWriter http.ResponseWriter, request *http.Request) ([]*multipart.FileHeader, *intakeError) {
	request.Body = http.MaxBytesReader(responseWriter, request.Body, maxIntakeSize+fileStorage.MaxAnalysisSize+(1<<20))
	if errorValue := request.ParseMultipartForm(maxFileSize); errorValue != nil {
		return nil, &intakeError{status: http.StatusBadRequest, message: "arquivos excedem o limite ou são inválidos"}
	}
	headers := request.MultipartForm.File["files"]
	if len(headers) == 0 {
		return nil, &intakeError{status: http.StatusBadRequest, message: "envie ao menos um arquivo no campo files"}
	}
	var totalSize int64
	for _, header := range headers {
		contentType := header.Header.Get("Content-Type")
		if !exams.AllowedUploadTypes[contentType] {
			return nil, &intakeError{status: http.StatusBadRequest, message: "tipo de arquivo não permitido: " + header.Filename}
		}
		if header.Size > maxFileSize {
			return nil, &intakeError{status: http.StatusRequestEntityTooLarge, message: "arquivo excede o limite de 20 MB: " + header.Filename}
		}
		totalSize += header.Size
	}
	if totalSize > maxIntakeSize {
		return nil, &intakeError{status: http.StatusRequestEntityTooLarge, message: "o conjunto de arquivos excede o limite de 50 MB"}
	}
	return headers, nil
}

func cleanupExpiredDrafts(executionContext context.Context, client fileStorage.Repository) {
	// ponytail: one-page cleanup; use a bucket lifecycle rule if drafts can exceed 1000 objects.
	listed, errorValue := client.ListObjectsV2(executionContext, &s3.ListObjectsV2Input{Bucket: aws.String(os.Getenv("BUCKET_NAME")), Prefix: aws.String(fileStorage.DraftsPrefix)})
	if errorValue != nil {
		return
	}
	keys := make([]string, 0)
	for _, object := range listed.Contents {
		if object.LastModified != nil && time.Since(*object.LastModified) > 24*time.Hour {
			keys = append(keys, aws.ToString(object.Key))
		}
	}
	fileStorage.CleanupObjects(executionContext, client, keys)
}

func randomToken() string {
	var token [4]byte
	if _, randomError := rand.Read(token[:]); randomError != nil {
		return "file"
	}
	return hex.EncodeToString(token[:])
}
