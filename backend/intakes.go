package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	patientfeature "refratia/backend/features/patient"
	progressutil "refratia/backend/shared/progress"
)

const (
	maxFileSize   = 20 << 20
	maxIntakeSize = 50 << 20
)

type intakeFile struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	Key         string `json:"key,omitempty"`
	SignedURL   string `json:"signed_url,omitempty"`
}

type storedIntake struct {
	CreatedAt time.Time      `json:"createdAt"`
	Files     []intakeFile   `json:"files"`
	Analysis  map[string]any `json:"analysis"`
}

type intakeError struct {
	status  int
	message string
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

		ctx = progressutil.WithReporter(
			ctx,
			stream.reporter,
		)

		progressutil.Report(
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
	)

	progressutil.Report(
		ctx,
		8,
		"ocr",
		"Iniciando extração local",
	)

	analysis, extractionErr := extractPatient(
		progressutil.WithRange(ctx, 8, 88),
		files,
	)

	if extractionErr != nil {
		fail(
			http.StatusBadGateway,
			extractionErr.Error(),
		)
		return
	}

	progressutil.Report(
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

	progressutil.Report(
		ctx,
		93,
		"storage",
		"Preparando rascunho",
	)

	storedFiles := make([]intakeFile, 0, len(files))
	keys := make([]string, 0, len(files)+1)

	for index, uploaded := range files {
		percent := 94

		if len(files) > 0 {
			percent += index * 4 / len(files)
		}

		progressutil.Report(
			ctx,
			percent,
			"storage",
			fmt.Sprintf(
				"Armazenando documento %d/%d",
				index+1,
				len(files),
			),
		)

		key := fmt.Sprintf(
			"drafts/%s/%s",
			intakeID,
			safeName(uploaded.Metadata.Filename),
		)

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

		metadata := uploaded.Metadata
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
		"drafts/%s/intake.json",
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

	progressutil.Report(
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

	progressutil.Report(
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
		IntakeID string `json:"intakeId"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil || !validIntakeID(request.IntakeID) {
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
	defaultAnalysisKey := fmt.Sprintf("cases/%s/paciente_compilado.json", defaultCaseID)
	draftKey := fmt.Sprintf("drafts/%s/intake.json", request.IntakeID)

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

	caseID := defaultCaseID
	analysisKey := defaultAnalysisKey
	action := "created"

	var existingAnalysis map[string]any

	existingCaseID, existing, found, findErr := findExistingPatientCase(
		r.Context(),
		client,
		bucket,
		draft.Analysis,
	)
	if findErr != nil {
		writeError(w, http.StatusBadGateway, "não foi possível verificar pacientes existentes")
		return
	}

	if found {
		caseID = existingCaseID
		analysisKey = fmt.Sprintf("cases/%s/paciente_compilado.json", caseID)
		existingAnalysis = existing
		action = "updated"
	}

	result := make([]intakeFile, 0, len(draft.Files))
	copiedKeys := make([]string, 0, len(draft.Files))

	for _, file := range draft.Files {
		key := fmt.Sprintf("cases/%s/%s", caseID, filepath.Base(file.Key))

		_, copyErr := client.CopyObject(r.Context(), &s3.CopyObjectInput{
			Bucket:            aws.String(bucket),
			Key:               aws.String(key),
			CopySource:        aws.String(url.PathEscape(bucket + "/" + file.Key)),
			ContentType:       aws.String(file.ContentType),
			MetadataDirective: "REPLACE",
		})
		if copyErr != nil {
			cleanupObjects(r.Context(), client, copiedKeys)
			writeError(w, http.StatusBadGateway, "não foi possível confirmar os arquivos")
			return
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

	compiled, marshalErr := json.MarshalIndent(finalAnalysis, "", "  ")
	if marshalErr != nil {
		cleanupObjects(r.Context(), client, copiedKeys)
		writeError(w, http.StatusInternalServerError, "não foi possível montar o JSON compilado")
		return
	}

	_, putErr := client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(analysisKey),
		Body:        bytes.NewReader(compiled),
		ContentType: aws.String("application/json"),
	})
	if putErr != nil {
		cleanupObjects(r.Context(), client, copiedKeys)
		writeError(w, http.StatusBadGateway, "não foi possível armazenar o JSON compilado")
		return
	}

	// Se exatamente o mesmo arquivo for reenviado, source_files é deduplicado
	// por SHA-256. Nesse caso não deixamos a cópia redundante órfã no bucket.
	referenced := patientfeature.ReferencedSourcePaths(finalAnalysis)
	orphanedCopies := make([]string, 0)

	for _, key := range copiedKeys {
		if !referenced[key] {
			orphanedCopies = append(orphanedCopies, key)
		}
	}
	cleanupObjects(r.Context(), client, orphanedCopies)

	// O receipt precisa existir antes de consumirmos o draft.
	// Se sua gravação falhar, o case já pode ter sido atualizado, mas o draft
	// permanece disponível para um retry idempotente da mesma confirmação.
	if receiptErr := storeConfirmationReceipt(
		r.Context(),
		client,
		bucket,
		request.IntakeID,
		confirmationReceipt{
			CaseID:      caseID,
			AnalysisKey: analysisKey,
			Action:      action,
		},
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

func intakeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	intakeID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/intakes/"), "/")
	if !validIntakeID(intakeID) {
		writeError(w, http.StatusBadRequest, "rascunho inválido")
		return
	}
	client, err := storageClient(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage indisponível")
		return
	}
	bucket := os.Getenv("BUCKET_NAME")
	draftKey := fmt.Sprintf("drafts/%s/intake.json", intakeID)
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
	prefix := fmt.Sprintf("drafts/%s/", intakeID)
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

func validIntakeID(id string) bool {
	parts := strings.Split(strings.TrimPrefix(id, "intake-"), "-")
	if !strings.HasPrefix(id, "intake-") || len(parts) != 2 || len(parts[1]) != 8 {
		return false
	}
	_, timeErr := time.Parse("20060102T150405Z", parts[0])
	_, tokenErr := hex.DecodeString(parts[1])
	return timeErr == nil && tokenErr == nil
}

func intakeHeaders(w http.ResponseWriter, r *http.Request) ([]*multipart.FileHeader, *intakeError) {
	r.Body = http.MaxBytesReader(w, r.Body, maxIntakeSize+maxAnalysisSize+(1<<20))
	if err := r.ParseMultipartForm(maxFileSize); err != nil {
		return nil, &intakeError{http.StatusBadRequest, "arquivos excedem o limite ou são inválidos"}
	}
	headers := r.MultipartForm.File["files"]
	if len(headers) == 0 {
		return nil, &intakeError{http.StatusBadRequest, "envie ao menos um arquivo no campo files"}
	}
	var totalSize int64
	for _, header := range headers {
		contentType := header.Header.Get("Content-Type")
		if !allowedTypes[contentType] {
			return nil, &intakeError{http.StatusBadRequest, "tipo de arquivo não permitido: " + header.Filename}
		}
		if header.Size > maxFileSize {
			return nil, &intakeError{http.StatusRequestEntityTooLarge, "arquivo excede o limite de 20 MB: " + header.Filename}
		}
		totalSize += header.Size
	}
	if totalSize > maxIntakeSize {
		return nil, &intakeError{http.StatusRequestEntityTooLarge, "o conjunto de arquivos excede o limite de 50 MB"}
	}
	return headers, nil
}

func cleanupExpiredDrafts(ctx context.Context, client *s3.Client) {
	// ponytail: one-page cleanup; use a bucket lifecycle rule if drafts can exceed 1000 objects.
	listed, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(os.Getenv("BUCKET_NAME")), Prefix: aws.String("drafts/")})
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
