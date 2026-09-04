package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	maxFileSize     = 20 << 20
	maxIntakeSize   = 50 << 20
	maxAnalysisSize = 5 << 20
)

var allowedTypes = map[string]bool{
	"application/pdf":    true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.ms-excel": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
	"image/jpeg": true,
	"image/png":  true,
}

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

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/api/cases", casesHandler)
	http.HandleFunc("/api/cases/", caseHandler)
	http.HandleFunc("/api/intakes/analyze", analyzeIntakeHandler)
	http.HandleFunc("/api/intakes/confirm", confirmIntakeHandler)
	http.HandleFunc("/api/intakes/", intakeHandler)

	address := ":" + port
	fmt.Printf("Backend running on http://localhost%s\n", address)
	if err := http.ListenAndServe(address, cors(http.DefaultServeMux)); err != nil {
		panic(err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "hello world")
}

func casesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	client, err := storageClient(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage indisponível")
		return
	}
	listed, err := client.ListObjectsV2(r.Context(), &s3.ListObjectsV2Input{
		Bucket:    aws.String(os.Getenv("BUCKET_NAME")),
		Prefix:    aws.String("cases/"),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "não foi possível listar os casos")
		return
	}

	type storedCase struct {
		CaseID      string `json:"caseId"`
		PatientName string `json:"patientName"`
		AnalysisKey string `json:"analysisKey"`
	}
	cases := make([]storedCase, 0, len(listed.CommonPrefixes))
	for _, prefix := range listed.CommonPrefixes {
		casePrefix := aws.ToString(prefix.Prefix)
		caseID := strings.TrimSuffix(strings.TrimPrefix(casePrefix, "cases/"), "/")
		analysisKey := casePrefix + "paciente_compilado.json"
		object, getErr := client.GetObject(r.Context(), &s3.GetObjectInput{
			Bucket: aws.String(os.Getenv("BUCKET_NAME")),
			Key:    aws.String(analysisKey),
		})
		if getErr != nil {
			continue
		}
		var analysis struct {
			Patient struct {
				FullName string `json:"full_name"`
			} `json:"patient"`
		}
		decodeErr := json.NewDecoder(io.LimitReader(object.Body, 1<<20)).Decode(&analysis)
		object.Body.Close()
		if decodeErr != nil {
			continue
		}
		cases = append(cases, storedCase{CaseID: caseID, PatientName: analysis.Patient.FullName, AnalysisKey: analysisKey})
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].CaseID > cases[j].CaseID })
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"cases": cases})
}

func caseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	caseID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/cases/"), "/")
	if !validCaseID(caseID) {
		writeError(w, http.StatusBadRequest, "caso inválido")
		return
	}
	if r.Method == http.MethodDelete {
		deleteCaseHandler(w, r, caseID)
		return
	}
	client, err := storageClient(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage indisponível")
		return
	}
	analysisKey := fmt.Sprintf("cases/%s/paciente_compilado.json", caseID)
	object, err := client.GetObject(r.Context(), &s3.GetObjectInput{
		Bucket: aws.String(os.Getenv("BUCKET_NAME")),
		Key:    aws.String(analysisKey),
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "caso não encontrado")
		return
	}
	defer object.Body.Close()
	var analysis map[string]any
	if err := json.NewDecoder(io.LimitReader(object.Body, maxAnalysisSize)).Decode(&analysis); err != nil {
		writeError(w, http.StatusBadGateway, "não foi possível ler a análise do caso")
		return
	}
	presigner, err := storagePresigner(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage indisponível")
		return
	}
	if files, ok := analysis["source_files"].([]any); ok {
		for _, rawFile := range files {
			file, ok := rawFile.(map[string]any)
			key, ok := file["path"].(string)
			if !ok || !strings.HasPrefix(key, fmt.Sprintf("cases/%s/", caseID)) {
				continue
			}
			request, err := presigner.PresignGetObject(r.Context(), &s3.GetObjectInput{
				Bucket: aws.String(os.Getenv("BUCKET_NAME")),
				Key:    aws.String(key),
			})
			if err == nil {
				file["signed_url"] = request.URL
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"caseId": caseID, "analysis": analysis})
}

func deleteCaseHandler(w http.ResponseWriter, r *http.Request, caseID string) {
	token := os.Getenv("CASE_DELETE_TOKEN")
	if token == "" {
		writeError(w, http.StatusNotFound, "exclusão de casos desabilitada")
		return
	}
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
		writeError(w, http.StatusUnauthorized, "token de exclusão inválido")
		return
	}
	client, err := storageClient(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage indisponível")
		return
	}
	bucket := os.Getenv("BUCKET_NAME")
	prefix := fmt.Sprintf("cases/%s/", caseID)
	marker := prefix + "paciente_compilado.json"
	keys := make([]string, 0)
	var continuation *string
	markerExists := false
	for {
		listed, listErr := client.ListObjectsV2(r.Context(), &s3.ListObjectsV2Input{
			Bucket: aws.String(bucket), Prefix: aws.String(prefix), ContinuationToken: continuation,
		})
		if listErr != nil {
			writeError(w, http.StatusBadGateway, "não foi possível listar os arquivos do caso")
			return
		}
		for _, object := range listed.Contents {
			key := aws.ToString(object.Key)
			if key == marker {
				markerExists = true
			} else {
				keys = append(keys, key)
			}
		}
		if !aws.ToBool(listed.IsTruncated) {
			break
		}
		continuation = listed.NextContinuationToken
	}
	if len(keys) == 0 && !markerExists {
		writeError(w, http.StatusNotFound, "caso não encontrado")
		return
	}
	deletedCount := len(keys)
	if markerExists {
		deletedCount++
	}
	for _, key := range append(keys, marker) {
		if key == marker && !markerExists {
			continue
		}
		if _, deleteErr := client.DeleteObject(r.Context(), &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}); deleteErr != nil {
			writeError(w, http.StatusBadGateway, "não foi possível apagar todos os arquivos do caso")
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"caseId": caseID, "deletedObjects": deletedCount})
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
		writeError(w, http.StatusBadRequest, "não foi possível ler um dos arquivos")
		return
	}
	analysis, extractionErr := extractPatient(r.Context(), files)
	if extractionErr != nil {
		writeError(w, http.StatusBadGateway, extractionErr.Error())
		return
	}

	intakeID := fmt.Sprintf("intake-%s-%s", time.Now().UTC().Format("20060102T150405Z"), randomToken())
	client, storageErr := storageClient(r.Context())
	if storageErr != nil {
		writeError(w, http.StatusInternalServerError, "storage indisponível")
		return
	}
	cleanupExpiredDrafts(r.Context(), client)

	patientMatch := map[string]any{
		"status": "unresolved",
	}
	var changePreview any

	_, identifiable := patientIdentity(analysis)
	var existingCaseID string
	var existingAnalysis map[string]any
	var foundExisting bool

	if identifiable {
		patientMatch["status"] = "new"

		var findErr error
		existingCaseID, existingAnalysis, foundExisting, findErr = findExistingPatientCase(
			r.Context(),
			client,
			os.Getenv("BUCKET_NAME"),
			analysis,
		)
		if findErr != nil {
			writeError(w, http.StatusBadGateway, "não foi possível verificar pacientes existentes")
			return
		}
	}

	if foundExisting {
		patientMatch["status"] = "existing"
		patientMatch["caseId"] = existingCaseID
		changePreview = buildPatientChangePreview(existingAnalysis, analysis)

		if patient, ok := existingAnalysis["patient"].(map[string]any); ok {
			if name, ok := patient["full_name"].(string); ok {
				patientMatch["patientName"] = name
			}
		}
	}

	storedFiles := make([]intakeFile, 0, len(files))
	keys := make([]string, 0, len(files)+1)
	for _, uploaded := range files {
		key := fmt.Sprintf("drafts/%s/%s", intakeID, safeName(uploaded.Metadata.Filename))
		_, putErr := client.PutObject(r.Context(), &s3.PutObjectInput{
			Bucket: aws.String(os.Getenv("BUCKET_NAME")), Key: aws.String(key), Body: bytes.NewReader(uploaded.Data), ContentType: aws.String(uploaded.Metadata.ContentType),
		})
		if putErr != nil {
			cleanupObjects(r.Context(), client, keys)
			writeError(w, http.StatusBadGateway, "não foi possível armazenar o rascunho")
			return
		}
		keys = append(keys, key)
		metadata := uploaded.Metadata
		metadata.Key = key
		storedFiles = append(storedFiles, metadata)
	}
	draft, marshalErr := json.Marshal(storedIntake{CreatedAt: time.Now().UTC(), Files: storedFiles, Analysis: analysis})
	if marshalErr != nil {
		cleanupObjects(r.Context(), client, keys)
		writeError(w, http.StatusInternalServerError, "não foi possível montar o rascunho")
		return
	}
	draftKey := fmt.Sprintf("drafts/%s/intake.json", intakeID)
	if _, putErr := client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket: aws.String(os.Getenv("BUCKET_NAME")), Key: aws.String(draftKey), Body: bytes.NewReader(draft), ContentType: aws.String("application/json"),
	}); putErr != nil {
		cleanupObjects(r.Context(), client, keys)
		writeError(w, http.StatusBadGateway, "não foi possível armazenar o rascunho")
		return
	}
	presigner, presignErr := storagePresigner(r.Context())
	if presignErr != nil {
		writeError(w, http.StatusInternalServerError, "storage indisponível")
		return
	}
	previewFiles := intakeMetadata(files)
	for index := range previewFiles {
		request, err := presigner.PresignGetObject(r.Context(), &s3.GetObjectInput{
			Bucket: aws.String(os.Getenv("BUCKET_NAME")),
			Key:    aws.String(storedFiles[index].Key),
		})
		if err == nil {
			previewFiles[index].SignedURL = request.URL
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"intakeId":      intakeID,
		"files":         previewFiles,
		"analysis":      analysis,
		"patientMatch":  patientMatch,
		"changePreview": changePreview,
		"message":       "Documentos e análise armazenados. Confira a extração antes de confirmar.",
	})
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
		finalAnalysis = mergePatientCase(existingAnalysis, draft.Analysis)
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
	referenced := referencedSourcePaths(finalAnalysis)
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

func validCaseID(id string) bool {
	return strings.HasPrefix(id, "case-") && validIntakeID("intake-"+strings.TrimPrefix(id, "case-"))
}

type intakeError struct {
	status  int
	message string
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

func cleanupObjects(ctx context.Context, client *s3.Client, keys []string) {
	for _, key := range keys {
		_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(os.Getenv("BUCKET_NAME")), Key: aws.String(key)})
	}
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

func storageClient(ctx context.Context) (*s3.Client, error) {
	return newStorageClient(ctx, os.Getenv("AWS_ENDPOINT_URL_S3"))
}

func storagePresigner(ctx context.Context) (*s3.PresignClient, error) {
	endpoint := os.Getenv("AWS_PUBLIC_ENDPOINT_URL_S3")
	if endpoint == "" {
		endpoint = os.Getenv("AWS_ENDPOINT_URL_S3")
	}
	client, err := newStorageClient(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	return s3.NewPresignClient(client), nil
}

func newStorageClient(ctx context.Context, endpoint string) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	}), nil
}

func safeName(name string) string {
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, " ", "-")
	return randomToken() + "-" + name
}

func randomToken() string {
	var token [4]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "file"
	}
	return hex.EncodeToString(token[:])
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := os.Getenv("CORS_ORIGINS")
		if allowed == "" {
			allowed = "http://localhost:5173,https://mateusb12.github.io"
		}
		for _, item := range strings.Split(allowed, ",") {
			if strings.TrimSpace(item) == origin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
