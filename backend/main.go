package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

type uploadResponse struct {
	Key         string `json:"key"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/api/cases/gerinaldo-alfregildo/files", uploadHandler)

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

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxFileSize+(1<<20))
	if err := r.ParseMultipartForm(maxFileSize); err != nil {
		writeError(w, http.StatusBadRequest, "arquivo excede o limite de 20 MB ou é inválido")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "envie um campo file")
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if !allowedTypes[contentType] {
		writeError(w, http.StatusBadRequest, "tipo de arquivo não permitido")
		return
	}
	if header.Size > maxFileSize {
		writeError(w, http.StatusRequestEntityTooLarge, "arquivo excede o limite de 20 MB")
		return
	}

	key := fmt.Sprintf("cases/gerinaldo-alfregildo/%s-%s", time.Now().UTC().Format("20060102T150405Z"), safeName(header.Filename))
	client, err := storageClient(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage indisponível")
		return
	}
	_, err = client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(os.Getenv("BUCKET_NAME")),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "não foi possível armazenar o arquivo")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(uploadResponse{Key: key, Filename: header.Filename, ContentType: contentType, Size: header.Size})
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
	var token [4]byte
	if _, err := rand.Read(token[:]); err != nil {
		return name
	}
	return hex.EncodeToString(token[:]) + "-" + name
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
