package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
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
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	caseID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/cases/"), "/")
	if caseID == "" || strings.Contains(caseID, "/") {
		writeError(w, http.StatusBadRequest, "caso inválido")
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"caseId": caseID, "analysis": analysis})
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

	// No Tigris write here. The browser keeps the original Files until confirmation.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"files":    intakeMetadata(files),
		"analysis": analysis,
		"message":  "Documentos processados. Confira o JSON extraído antes de criar o caso.",
	})
}

func confirmIntakeHandler(w http.ResponseWriter, r *http.Request) {
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
	analysis, analysisErr := confirmedAnalysis(r.FormValue("analysis"), files)
	if analysisErr != nil {
		writeError(w, http.StatusBadRequest, analysisErr.Error())
		return
	}
	caseID := fmt.Sprintf("case-%s-%s", time.Now().UTC().Format("20060102T150405Z"), randomToken())
	client, storageErr := storageClient(r.Context())
	if storageErr != nil {
		writeError(w, http.StatusInternalServerError, "storage indisponível")
		return
	}
	result := make([]intakeFile, 0, len(files))
	keys := make([]string, 0, len(files)+1)
	for _, uploaded := range files {
		key := fmt.Sprintf("cases/%s/%s", caseID, safeName(uploaded.Metadata.Filename))
		_, putErr := client.PutObject(r.Context(), &s3.PutObjectInput{
			Bucket: aws.String(os.Getenv("BUCKET_NAME")), Key: aws.String(key), Body: bytes.NewReader(uploaded.Data), ContentType: aws.String(uploaded.Metadata.ContentType),
		})
		if putErr != nil {
			cleanupObjects(r.Context(), client, keys)
			writeError(w, http.StatusBadGateway, "não foi possível armazenar os arquivos")
			return
		}
		keys = append(keys, key)
		metadata := uploaded.Metadata
		metadata.Key = key
		result = append(result, metadata)
	}
	setStoredPaths(analysis, result)
	compiled, marshalErr := json.MarshalIndent(analysis, "", "  ")
	if marshalErr != nil {
		cleanupObjects(r.Context(), client, keys)
		writeError(w, http.StatusInternalServerError, "não foi possível montar o JSON compilado")
		return
	}
	analysisKey := fmt.Sprintf("cases/%s/paciente_compilado.json", caseID)
	_, putErr := client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket: aws.String(os.Getenv("BUCKET_NAME")), Key: aws.String(analysisKey), Body: bytes.NewReader(compiled), ContentType: aws.String("application/json"),
	})
	if putErr != nil {
		cleanupObjects(r.Context(), client, keys)
		writeError(w, http.StatusBadGateway, "não foi possível armazenar o JSON compilado")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"caseId": caseID, "files": result, "analysisKey": analysisKey})
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

func storageClient(ctx context.Context) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(os.Getenv("AWS_ENDPOINT_URL_S3"))
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
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
