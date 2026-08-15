package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const maxFileSize = 20 << 20

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
	Key         string `json:"key,omitempty"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	http.HandleFunc("/health", healthHandler)
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

func analyzeIntakeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	files, err := intakeFiles(w, r)
	if err != nil {
		writeError(w, err.status, err.message)
		return
	}

	// No Tigris write here. The browser keeps the original Files until confirmation.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"files":   files,
		"patient": nil,
		"message": "Arquivos recebidos. A identificação do paciente e a extração clínica ainda precisam de OCR/processamento.",
	})
}

func confirmIntakeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	files, err := intakeHeaders(w, r)
	if err != nil {
		writeError(w, err.status, err.message)
		return
	}
	caseID := fmt.Sprintf("case-%s-%s", time.Now().UTC().Format("20060102T150405Z"), randomToken())
	client, storageErr := storageClient(r.Context())
	if storageErr != nil {
		writeError(w, http.StatusInternalServerError, "storage indisponível")
		return
	}
	result := make([]intakeFile, 0, len(files))
	keys := make([]string, 0, len(files))
	for _, header := range files {
		file, openErr := header.Open()
		if openErr != nil {
			cleanupObjects(r.Context(), client, keys)
			writeError(w, http.StatusBadRequest, "não foi possível ler um dos arquivos")
			return
		}
		contentType := header.Header.Get("Content-Type")
		key := fmt.Sprintf("cases/%s/%s", caseID, safeName(header.Filename))
		_, putErr := client.PutObject(r.Context(), &s3.PutObjectInput{
			Bucket: aws.String(os.Getenv("BUCKET_NAME")), Key: aws.String(key), Body: file, ContentType: aws.String(contentType),
		})
		file.Close()
		if putErr != nil {
			cleanupObjects(r.Context(), client, keys)
			writeError(w, http.StatusBadGateway, "não foi possível armazenar os arquivos")
			return
		}
		keys = append(keys, key)
		result = append(result, intakeFile{Filename: header.Filename, ContentType: contentType, Size: header.Size, Key: key})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"caseId": caseID, "files": result})
}

type intakeError struct {
	status  int
	message string
}

func intakeFiles(w http.ResponseWriter, r *http.Request) ([]intakeFile, *intakeError) {
	headers, err := intakeHeaders(w, r)
	if err != nil {
		return nil, err
	}
	files := make([]intakeFile, 0, len(headers))
	for _, header := range headers {
		files = append(files, intakeFile{Filename: header.Filename, ContentType: header.Header.Get("Content-Type"), Size: header.Size})
	}
	return files, nil
}

func intakeHeaders(w http.ResponseWriter, r *http.Request) ([]*multipart.FileHeader, *intakeError) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFileSize*10+(1<<20))
	if err := r.ParseMultipartForm(maxFileSize * 10); err != nil {
		return nil, &intakeError{http.StatusBadRequest, "arquivos excedem o limite ou são inválidos"}
	}
	headers := r.MultipartForm.File["files"]
	if len(headers) == 0 {
		return nil, &intakeError{http.StatusBadRequest, "envie ao menos um arquivo no campo files"}
	}
	for _, header := range headers {
		contentType := header.Header.Get("Content-Type")
		if !allowedTypes[contentType] {
			return nil, &intakeError{http.StatusBadRequest, "tipo de arquivo não permitido: " + header.Filename}
		}
		if header.Size > maxFileSize {
			return nil, &intakeError{http.StatusRequestEntityTooLarge, "arquivo excede o limite de 20 MB: " + header.Filename}
		}
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
