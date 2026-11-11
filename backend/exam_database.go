package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gorm.io/gorm"

	"refratia/backend/database"
)

func confirmSavedExam(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !authorizeExamMutation(w, r) {
		return
	}

	var request struct {
		CaseID      string `json:"caseId"`
		Path        string `json:"path"`
		Filename    string `json:"filename"`
		ContentType string `json:"contentType"`
		Size        int64  `json:"size"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "payload inválido")
		return
	}
	if !validCaseID(request.CaseID) || !validExamObjectPath(request.CaseID, request.Path) {
		writeError(w, http.StatusBadRequest, "arquivo inválido")
		return
	}

	client, err := storageClient(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage indisponível")
		return
	}
	bucket := os.Getenv("BUCKET_NAME")
	head, err := client.HeadObject(r.Context(), &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(request.Path),
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "arquivo não encontrado no Garage")
		return
	}

	if appDatabase == nil {
		writeError(w, http.StatusInternalServerError, "banco de dados indisponível")
		return
	}

	var existing database.Exam
	lookup := appDatabase.Gorm.Where("object_key = ?", request.Path).First(&existing)
	if lookup.Error == nil {
		writeExamJSON(w, existing)
		return
	}
	if !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
		writeError(w, http.StatusInternalServerError, "não foi possível consultar o exame")
		return
	}

	metadata := examMetadataFromManifest(r, client, bucket, request.CaseID, request.Path)
	filename := filepath.Base(strings.TrimSpace(request.Filename))
	if filename == "." || filename == "" {
		filename = filepath.Base(request.Path)
	}
	size := request.Size
	if head.ContentLength != nil {
		size = *head.ContentLength
	}
	contentType := request.ContentType
	if contentType == "" {
		contentType = aws.ToString(head.ContentType)
	}

	record := database.Exam{
		CaseID:      request.CaseID,
		PatientName: metadata.patientName,
		Filename:    filename,
		ExamType:    metadata.examType,
		Eye:         metadata.eye,
		ContentType: contentType,
		Bucket:      bucket,
		ObjectKey:   request.Path,
		Size:        size,
		Referenced:  metadata.referenced,
	}
	if err := appDatabase.Gorm.Create(&record).Error; err != nil {
		_, _ = client.DeleteObject(r.Context(), &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(request.Path),
		})
		writeError(w, http.StatusInternalServerError, "não foi possível salvar os metadados; arquivo revertido")
		return
	}

	writeExamJSON(w, record)
}

func deleteExamRecord(objectKey, bucket string) error {
	if appDatabase == nil {
		return fmt.Errorf("database is not initialized")
	}
	return appDatabase.Gorm.Where("object_key = ? AND bucket = ?", objectKey, bucket).Delete(&database.Exam{}).Error
}

type manifestExamMetadata struct {
	patientName string
	examType    string
	eye         string
	referenced  bool
}

func examMetadataFromManifest(
	r *http.Request,
	client fileRepository,
	bucket string,
	caseID string,
	path string,
) manifestExamMetadata {
	result := manifestExamMetadata{}
	analysis, _, err := loadExamManifest(r.Context(), client, bucket, caseID)
	if err != nil {
		return result
	}
	if patient, ok := analysis["patient"].(map[string]any); ok {
		result.patientName = readExamMetadata(patient, "full_name")
	}
	classifySourceFilesFromExams(analysis)
	if sourceFiles, ok := analysis["source_files"].([]any); ok {
		for _, raw := range sourceFiles {
			file, ok := raw.(map[string]any)
			if !ok || readExamMetadata(file, "path") != path {
				continue
			}
			result.examType = readExamMetadata(file, "exam")
			result.eye = readExamMetadata(file, "eye")
			result.referenced = true
			return result
		}
	}
	return result
}

func validExamObjectPath(caseID, path string) bool {
	prefix := fmt.Sprintf("cases/%s/", caseID)
	return strings.HasPrefix(path, prefix) && path != prefix && path != prefix+"paciente_compilado.json"
}

func writeExamJSON(w http.ResponseWriter, record database.Exam) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":        record.ID,
		"caseId":    record.CaseID,
		"objectKey": record.ObjectKey,
		"filename":  record.Filename,
		"exam":      record.ExamType,
		"eye":       record.Eye,
	})
}
