package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
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

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/api/cases", casesHandler)
	http.HandleFunc("/api/cases/", caseHandler)
	http.HandleFunc("/api/exams", routeSavedExams)
	http.HandleFunc("/api/exams/upload-url", prepareExamUpload)
	http.HandleFunc(
		"/api/file-checkpoints/check",
		checkFileCheckpoints,
	)
	http.HandleFunc("/api/intakes/analyze", analyzeIntakeHandler)
	http.HandleFunc("/api/benchmark/extract-fields", benchmarkExtractFieldsHandler)
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
