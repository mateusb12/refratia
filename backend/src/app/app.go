// Package app composes the HTTP server: it builds each feature's handlers from
// explicit dependencies and wires them to the routes the frontend calls.
package app

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"gorm.io/gorm"

	"refratia/backend/src/features/intake"
	"refratia/backend/src/features/persistence/cases"
	"refratia/backend/src/features/persistence/exams"
)

// Routes returns the full API, wrapped in CORS.
func Routes(sqlDatabase *gorm.DB) http.Handler {
	caseHandler := cases.Handler{Database: sqlDatabase}
	examHandler := exams.Handler{Database: sqlDatabase}
	intakeHandler := intake.Handler{Database: sqlDatabase}

	router := http.NewServeMux()
	router.HandleFunc("/health", healthHandler)
	router.HandleFunc("/api/cases", caseHandler.ServeCases)
	router.HandleFunc("/api/cases/", caseHandler.ServeCase)
	router.HandleFunc("/api/exams/download", examHandler.ServeDownload)
	router.HandleFunc("/api/exams", examHandler.ServeExams)
	router.HandleFunc("/api/exams/upload-url", examHandler.ServeUploadURL)
	router.HandleFunc("/api/exams/confirm", examHandler.ServeConfirm)
	router.HandleFunc("/api/file-checkpoints/check", intake.CheckFileCheckpoints)
	router.HandleFunc("/api/intakes/analyze", intakeHandler.AnalyzeIntake)
	router.HandleFunc("/api/intakes/inspect", intake.InspectIntakeHandler)
	router.HandleFunc("/api/benchmark/extract-fields", intake.BenchmarkExtractFieldsHandler)
	router.HandleFunc("/api/intakes/confirm", intakeHandler.ConfirmIntake)
	router.HandleFunc("/api/intakes/", intake.DeleteIntakeHandler)
	return cors(router)
}

func healthHandler(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	responseWriter.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(responseWriter, "hello world")
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		allowed := os.Getenv("CORS_ORIGINS")
		if allowed == "" {
			allowed = "http://localhost:5173,https://mateusb12.github.io"
		}
		for _, item := range strings.Split(allowed, ",") {
			if strings.TrimSpace(item) == origin {
				responseWriter.Header().Set("Access-Control-Allow-Origin", origin)
				responseWriter.Header().Set("Vary", "Origin")
			}
		}
		responseWriter.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		responseWriter.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if request.Method == http.MethodOptions {
			responseWriter.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(responseWriter, request)
	})
}
