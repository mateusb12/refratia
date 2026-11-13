package main

import (
	"bytes"
	"context"
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

	"refratia/backend/database"
	intakefeature "refratia/backend/features/intake"
	patientfeature "refratia/backend/features/patient"
	progress "refratia/backend/shared/progress"
)

const (
	maxFileSize   = 20 << 20
	maxIntakeSize = 50 << 20
)

type storedIntake struct {
	CreatedAt time.Time                    `json:"createdAt"`
	Files     []intakefeature.FileMetadata `json:"files"`
	Analysis  map[string]any               `json:"analysis"`
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

func inspectIntakeHandler(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "não foi possível ler um dos arquivos")
		return
	}

	result := make([]intakefeature.FileInspection, 0, len(files))
	for _, file := range files {
		result = append(result, intakefeature.InspectFile(r.Context(), intakefeature.File{
			Filename:    file.Metadata.Filename,
			SHA256:      file.Metadata.SHA256,
			ContentType: file.Metadata.ContentType,
			Size:        file.Metadata.Size,
			Data:        file.Data,
		}))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"files": result})
}

func analyzeIntakeHandler(w http.ResponseWriter, r *http.Request) {
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
			"não foi possível ler um dos arquivos",
		)
		return
	}

	classifications, classificationErr := intakefeature.ParseClassifications(
		r.MultipartForm.Value["classifications"],
	)
	if classificationErr != nil {
		writeError(
			w,
			http.StatusBadRequest,
			"classificação de arquivos inválida",
		)
		return
	}

	var processFiles map[string]bool
	if rawProcess := r.MultipartForm.Value["processFiles"]; len(rawProcess) > 0 {
		var sha256Values []string
		if err := json.Unmarshal([]byte(rawProcess[0]), &sha256Values); err != nil {
			writeError(w, http.StatusBadRequest, "seleção de processamento inválida")
			return
		}

		processFiles = make(map[string]bool, len(sha256Values))
		for _, sha256Value := range sha256Values {
			processFiles[sha256Value] = true
		}
	}

	for index := range files {
		classification, ok := classifications[files[index].Metadata.SHA256]
		if ok {
			files[index].Metadata.ExamType = classification.ExamType
			files[index].Metadata.Eye = classification.Eye
			continue
		}

		// O filename é um fallback seguro para o novo padrão de nomenclatura.
		// Uma classificação manual enviada pelo frontend sempre tem prioridade.
		if examType, eye, filenameOK := intakefeature.CanonicalClassificationFromFilename(
			files[index].Metadata.Filename,
		); filenameOK {
			// Metadata.ExamType usa o mesmo vocabulário (rótulo) da classificação
			// explícita do frontend; o fallback devolvia a chave do contrato (F14).
			if label, labelOK := intakefeature.ExamTypeFromContract(examType); labelOK {
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
		writeError(
			w,
			http.StatusUnprocessableEntity,
			"classifique exame e lateralidade de todos os arquivos: "+strings.Join(unclassified, ", "),
		)
		return
	}

	ctx := r.Context()
	var stream *progressStream

	if r.URL.Query().Get("stream") == "1" {
		var ok bool

		stream, ok = newProgressStream(w)
		if !ok {
			writeError(
				w,
				http.StatusInternalServerError,
				"stream de progresso indisponível",
			)
			return
		}

		ctx = progress.WithReporter(
			ctx,
			stream.reporter,
		)

		progress.Report(
			ctx,
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

		writeError(w, status, message)
	}

	client, storageClientError :=
		storageClient(ctx)

	if storageClientError != nil {
		fail(
			http.StatusInternalServerError,
			"storage indisponível",
		)
		return
	}

	cleanupExpiredDrafts(
		ctx,
		client,
	)

	ctx = withFileCheckpointStorage(
		ctx,
		client,
		os.Getenv("BUCKET_NAME"),
		r.URL.Query().Get("force") == "1",
		processFiles,
	)

	progress.Report(
		ctx,
		8,
		"ocr",
		"Iniciando extração local",
	)

	analysis, extractionErr := extractPatient(
		progress.WithRange(ctx, 8, 88),
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
	classifySourceFilesFromExams(analysis)

	// O nascimento canônico final (leitura do Pentacam em caso de inversão)
	// precisa estar em patient.birth_date: a extração incremental pode tê-lo
	// fixado só com o primeiro documento lido.
	patientfeature.NormalizeIdentityFields(analysis)

	progress.Report(
		ctx,
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
			findErr = findExistingPatientCase(
			ctx,
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
		similar, similarErr := similarPatientsByBirthDate(appDatabase.Gorm, key, birth)
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
		ctx,
		93,
		"storage",
		"Preparando rascunho",
	)

	storedFiles := make([]intakefeature.FileMetadata, 0, len(files))
	keys := make([]string, 0, len(files)+1)

	for index, uploaded := range files {
		percent := 94

		if len(files) > 0 {
			percent += index * 4 / len(files)
		}

		progress.Report(
			ctx,
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
		metadata.CanonicalFilename = intakefeature.CanonicalExamFilename(patientName, metadata.ExamType, metadata.Eye, extension, metadata.SHA256)
		storageName := metadata.CanonicalFilename
		key := fmt.Sprintf(refratiaDraftsPrefix+"%s/%s", intakeID, storageName)

		_, putErr := client.PutObject(
			ctx,
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
			cleanupObjects(ctx, client, keys)

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
		cleanupObjects(ctx, client, keys)

		fail(
			http.StatusInternalServerError,
			"não foi possível montar o rascunho",
		)
		return
	}

	draftKey := fmt.Sprintf(
		refratiaDraftsPrefix+"%s/intake.json",
		intakeID,
	)

	if _, putErr := client.PutObject(
		ctx,
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
		cleanupObjects(ctx, client, keys)

		fail(
			http.StatusBadGateway,
			"não foi possível armazenar o rascunho",
		)
		return
	}

	progress.Report(
		ctx,
		99,
		"preview",
		"Gerando pré-visualização",
	)

	presigner, presignErr := storagePresigner(ctx)
	if presignErr != nil {
		fail(
			http.StatusInternalServerError,
			"storage indisponível",
		)
		return
	}

	previewFiles := intakeMetadata(files)

	for index := range previewFiles {
		request, err := presigner.PresignGetObject(
			ctx,
			&s3.GetObjectInput{
				Bucket: aws.String(
					os.Getenv("BUCKET_NAME"),
				),
				Key: aws.String(
					storedFiles[index].Key,
				),
			},
		)

		if err == nil {
			previewFiles[index].SignedURL = request.URL
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
		ctx,
		100,
		"complete",
		"Análise concluída",
	)

	if stream != nil {
		stream.writeResult(payload)
		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(w).Encode(payload)
}

func confirmIntakeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var request struct {
		IntakeID         string            `json:"intakeId"`
		IdentityDecision *identityDecision `json:"identityDecision"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil || !intakefeature.ValidID(request.IntakeID) {
		writeError(w, http.StatusBadRequest, "rascunho inválido")
		return
	}

	client, storageErr := storageClient(r.Context())
	if storageErr != nil {
		writeError(w, http.StatusInternalServerError, "storage indisponível")
		return
	}

	bucket := os.Getenv("BUCKET_NAME")

	// Retry da mesma confirmação deve devolver exatamente o case já resolvido,
	// inclusive quando aquele intake foi incorporado em um paciente preexistente.
	if receipt, ok := loadConfirmationReceipt(r.Context(), client, bucket, request.IntakeID); ok {
		writeConfirmation(w, receipt.CaseID, receipt.AnalysisKey, receipt.Action)
		return
	}

	defaultCaseID := "case-" + strings.TrimPrefix(request.IntakeID, "intake-")
	defaultAnalysisKey := fmt.Sprintf(refratiaCasesPrefix+"%s/paciente_compilado.json", defaultCaseID)
	draftKey := fmt.Sprintf(refratiaDraftsPrefix+"%s/intake.json", request.IntakeID)

	object, getErr := client.GetObject(r.Context(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(draftKey),
	})
	if getErr != nil {
		// Compatibilidade com confirmações criadas antes do receipt.
		if _, headErr := client.HeadObject(r.Context(), &s3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(defaultAnalysisKey),
		}); headErr == nil {
			writeConfirmation(w, defaultCaseID, defaultAnalysisKey, "created")
			return
		}

		writeError(w, http.StatusNotFound, "rascunho não encontrado")
		return
	}

	var draft storedIntake
	decodeErr := json.NewDecoder(
		io.LimitReader(object.Body, maxAnalysisSize+(1<<20)),
	).Decode(&draft)
	object.Body.Close()

	if decodeErr != nil ||
		time.Since(draft.CreatedAt) > 24*time.Hour ||
		validateStoredAnalysis(draft.Analysis, draft.Files) != nil {
		writeError(w, http.StatusBadRequest, "rascunho inválido ou expirado")
		return
	}

	// C2: identidade validada ANTES de qualquer escrita no Garage.
	if _, _, _, identityErr := patientIdentity(draft.Analysis); identityErr != nil {
		writeError(w, http.StatusUnprocessableEntity, "não foi possível salvar o paciente: nome ou data de nascimento estão incompletos ou conflitantes")
		return
	}
	for _, file := range draft.Files {
		if file.ExamType == "" || file.Eye == "" {
			writeError(w, http.StatusUnprocessableEntity, "arquivo sem exame e lateralidade classificados")
			return
		}
	}

	receipt, commitErr := commitIntake(r.Context(), client, bucket, draft, defaultCaseID, request.IdentityDecision)
	if commitErr != nil {
		var httpErr *intakeError
		switch {
		case errors.As(commitErr, &httpErr) && httpErr.body != nil:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(httpErr.status)
			json.NewEncoder(w).Encode(httpErr.body)
		case errors.As(commitErr, &httpErr):
			writeError(w, httpErr.status, httpErr.message)
		case errors.Is(commitErr, errPatientIdentity):
			writeError(w, http.StatusUnprocessableEntity, "não foi possível salvar o paciente: nome ou data de nascimento estão incompletos ou conflitantes")
		default:
			log.Printf("confirm %s: %v", request.IntakeID, commitErr)
			writeError(w, http.StatusInternalServerError, "não foi possível salvar o paciente")
		}
		return
	}
	caseID, analysisKey, action := receipt.CaseID, receipt.AnalysisKey, receipt.Action

	// O receipt precisa existir antes de consumirmos o draft.
	// Se sua gravação falhar, o case já pode ter sido atualizado, mas o draft
	// permanece disponível para um retry idempotente da mesma confirmação.
	if receiptErr := storeConfirmationReceipt(
		r.Context(),
		client,
		bucket,
		request.IntakeID,
		receipt,
	); receiptErr != nil {
		writeError(w, http.StatusBadGateway, "não foi possível finalizar a confirmação")
		return
	}

	draftKeys := make([]string, 0, len(draft.Files)+1)
	for _, file := range draft.Files {
		draftKeys = append(draftKeys, file.Key)
	}
	cleanupObjects(r.Context(), client, append(draftKeys, draftKey))

	writeConfirmation(w, caseID, analysisKey, action)
}

func (e *intakeError) Error() string { return e.message }

// afterGarageWrites is a test seam: it runs inside the commit transaction after
// the Garage writes and before the SQL rows, so tests can fail that step.
var afterGarageWrites func() error

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
func commitIntake(
	ctx context.Context,
	client fileRepository,
	bucket string,
	draft storedIntake,
	defaultCaseID string,
	decision *identityDecision,
) (confirmationReceipt, error) {
	identityKey, fullName, birthDate, err := patientIdentity(draft.Analysis)
	if err != nil {
		return confirmationReceipt{}, err
	}
	if appDatabase == nil {
		return confirmationReceipt{}, errors.New("database is not initialized")
	}
	patientfeature.NormalizeIdentityFields(draft.Analysis)

	// C2: same birth date + similar-but-different name and no exact match is a
	// possible OCR typo: ask for a decision, write nothing (no Garage, no SQL).
	var exact int64
	if err := appDatabase.Gorm.Model(&database.Patient{}).Where("identity_key = ?", identityKey).Count(&exact).Error; err != nil {
		return confirmationReceipt{}, err
	}
	if exact == 0 {
		similar, err := similarPatientsByBirthDate(appDatabase.Gorm, identityKey, birthDate)
		if err != nil {
			return confirmationReceipt{}, err
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
				for i := range candidates {
					if candidates[i].CaseID == decision.CaseID {
						chosen = &candidates[i]
					}
				}
				if chosen == nil {
					return confirmationReceipt{}, &intakeError{status: http.StatusBadRequest, message: "caseId da decisão não é um candidato"}
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
				for _, c := range candidates {
					list = append(list, map[string]any{"caseId": c.CaseID, "name": c.FullName})
				}
				return confirmationReceipt{}, &intakeError{
					status:  http.StatusConflict,
					message: "identity_review_required",
					body:    map[string]any{"error": "identity_review_required", "candidates": list},
				}
			}
		}
	}

	var receipt confirmationReceipt
	err = appDatabase.Gorm.Transaction(func(tx *gorm.DB) error {
		patient, err := lockPatient(tx, identityKey, fullName, birthDate)
		if err != nil {
			return err
		}

		caseID := defaultCaseID
		action := "created"
		var existingAnalysis map[string]any
		if patient.CaseID != "" {
			stored, loadErr := loadStoredCaseAnalysis(ctx, client, bucket, patient.CaseID)
			switch {
			case loadErr == nil:
				caseID, existingAnalysis, action = patient.CaseID, stored, "updated"
			case !isNotFound(loadErr):
				return &intakeError{status: http.StatusBadGateway, message: "não foi possível verificar pacientes existentes"}
			}
		}
		if existingAnalysis == nil {
			// Fallback legado: case sem linha/case_id em SQL.
			legacyID, legacy, found, findErr := findExistingPatientCase(ctx, client, bucket, draft.Analysis)
			if findErr != nil {
				return &intakeError{status: http.StatusBadGateway, message: "não foi possível verificar pacientes existentes"}
			}
			if found {
				caseID, existingAnalysis, action = legacyID, legacy, "updated"
			}
		}
		analysisKey := fmt.Sprintf(refratiaCasesPrefix+"%s/paciente_compilado.json", caseID)

		result := make([]intakefeature.FileMetadata, 0, len(draft.Files))
		copiedKeys := make([]string, 0, len(draft.Files))
		for _, file := range draft.Files {
			key := fmt.Sprintf(refratiaCasesPrefix+"%s/%s", caseID, filepath.Base(file.Key))
			_, copyErr := client.CopyObject(ctx, &s3.CopyObjectInput{
				Bucket:            aws.String(bucket),
				Key:               aws.String(key),
				CopySource:        aws.String(url.PathEscape(bucket + "/" + file.Key)),
				ContentType:       aws.String(file.ContentType),
				MetadataDirective: "REPLACE",
			})
			if copyErr != nil {
				cleanupObjects(ctx, client, copiedKeys)
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
		classifySourceFilesFromExams(finalAnalysis)

		compiled, marshalErr := json.MarshalIndent(finalAnalysis, "", "  ")
		if marshalErr != nil {
			cleanupObjects(ctx, client, copiedKeys)
			return &intakeError{status: http.StatusInternalServerError, message: "não foi possível montar o JSON compilado"}
		}
		if _, putErr := client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      aws.String(bucket),
			Key:         aws.String(analysisKey),
			Body:        bytes.NewReader(compiled),
			ContentType: aws.String("application/json"),
		}); putErr != nil {
			cleanupObjects(ctx, client, copiedKeys)
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
			for _, eye := range examEyes(file.Eye) {
				rows = append(rows, database.Exam{
					CaseID:      caseID,
					PatientID:   &patient.ID,
					PatientName: fullName,
					Filename:    filename,
					ExamType:    examTypeKey(file.ExamType),
					Eye:         eye,
					ContentType: file.ContentType,
					Bucket:      bucket,
					ObjectKey:   file.Key,
					Size:        file.Size,
					Referenced:  true,
				})
			}
		}
		cleanupObjects(ctx, client, orphanedCopies)

		if afterGarageWrites != nil {
			if err := afterGarageWrites(); err != nil {
				return err
			}
		}
		if err := savePatient(tx, patient, fullName, birthDate, caseID); err != nil {
			return fmt.Errorf("update patient: %w", err)
		}
		if err := upsertExamRows(tx, rows); err != nil {
			return fmt.Errorf("upsert exams: %w", err)
		}
		// Documentos substituídos no merge (mesmo exame+olho) saem do SQL
		// junto com o manifesto; arquivos manuais não referenciados ficam.
		stale := tx.Where("case_id = ? AND referenced = ?", caseID, true)
		if len(referenced) > 0 {
			keys := make([]string, 0, len(referenced))
			for key := range referenced {
				keys = append(keys, key)
			}
			stale = stale.Where("object_key NOT IN ?", keys)
		}
		if err := stale.Delete(&database.Exam{}).Error; err != nil {
			return fmt.Errorf("prune exams: %w", err)
		}

		receipt = confirmationReceipt{CaseID: caseID, AnalysisKey: analysisKey, Action: action}
		return nil
	})
	return receipt, err
}

func intakeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	intakeID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/intakes/"), "/")
	if !intakefeature.ValidID(intakeID) {
		writeError(w, http.StatusBadRequest, "rascunho inválido")
		return
	}
	client, err := storageClient(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage indisponível")
		return
	}
	bucket := os.Getenv("BUCKET_NAME")
	draftKey := fmt.Sprintf(refratiaDraftsPrefix+"%s/intake.json", intakeID)
	object, err := client.GetObject(r.Context(), &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(draftKey)})
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var draft storedIntake
	decodeErr := json.NewDecoder(io.LimitReader(object.Body, maxAnalysisSize+(1<<20))).Decode(&draft)
	object.Body.Close()
	if decodeErr != nil {
		writeError(w, http.StatusBadGateway, "não foi possível ler o rascunho")
		return
	}
	keys := []string{draftKey}
	prefix := fmt.Sprintf(refratiaDraftsPrefix+"%s/", intakeID)
	for _, file := range draft.Files {
		if strings.HasPrefix(file.Key, prefix) {
			keys = append(keys, file.Key)
		}
	}
	cleanupObjects(r.Context(), client, keys)
	w.WriteHeader(http.StatusNoContent)
}

func writeConfirmation(w http.ResponseWriter, caseID, analysisKey, action string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"caseId":      caseID,
		"analysisKey": analysisKey,
		"action":      action,
	})
}

func intakeHeaders(w http.ResponseWriter, r *http.Request) ([]*multipart.FileHeader, *intakeError) {
	r.Body = http.MaxBytesReader(w, r.Body, maxIntakeSize+maxAnalysisSize+(1<<20))
	if err := r.ParseMultipartForm(maxFileSize); err != nil {
		return nil, &intakeError{status: http.StatusBadRequest, message: "arquivos excedem o limite ou são inválidos"}
	}
	headers := r.MultipartForm.File["files"]
	if len(headers) == 0 {
		return nil, &intakeError{status: http.StatusBadRequest, message: "envie ao menos um arquivo no campo files"}
	}
	var totalSize int64
	for _, header := range headers {
		contentType := header.Header.Get("Content-Type")
		if !allowedTypes[contentType] {
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

func cleanupExpiredDrafts(ctx context.Context, client fileRepository) {
	// ponytail: one-page cleanup; use a bucket lifecycle rule if drafts can exceed 1000 objects.
	listed, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(os.Getenv("BUCKET_NAME")), Prefix: aws.String(refratiaDraftsPrefix)})
	if err != nil {
		return
	}
	keys := make([]string, 0)
	for _, object := range listed.Contents {
		if object.LastModified != nil && time.Since(*object.LastModified) > 24*time.Hour {
			keys = append(keys, aws.ToString(object.Key))
		}
	}
	cleanupObjects(ctx, client, keys)
}
