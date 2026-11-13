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
	"gorm.io/gorm/clause"

	"refratia/backend/database"
	patientfeature "refratia/backend/features/patient"
)

// examTypeKeys maps the intake label vocabulary to the exam_type stored in SQL
// (contract C1). Unknown or missing never becomes ”.
var examTypeKeys = map[string]string{
	"PENTACAM":              "pentacam_corneal_tomography",
	"EYESUITE":              "iol_calculation",
	"AUTO":                  "refractometry",
	"MICROSCOPIA_ESPECULAR": "specular_microscopy",
	"RETINA":                "fundus_retinography",
	"OCT":                   "oct_retina",
}

func examTypeKey(label string) string {
	if key, ok := examTypeKeys[label]; ok {
		return key
	}
	if patientfeature.IsOfficialExamKey(label) {
		return label
	}
	return "unclassified"
}

// examEyes expands a document eye into the eyes it holds: AO documents are
// binocular and produce one row per eye (contract C1).
func examEyes(eye string) []string {
	switch strings.ToUpper(strings.TrimSpace(eye)) {
	case "AO":
		return []string{"OD", "OS"}
	case "OE":
		return []string{"OS"}
	default:
		return []string{strings.ToUpper(strings.TrimSpace(eye))}
	}
}

// upsertExamRows writes one row per (document, eye), idempotent on
// UNIQUE(object_key, eye).
func upsertExamRows(tx *gorm.DB, rows []database.Exam) error {
	if len(rows) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "object_key"}, {Name: "eye"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"patient_id", "case_id", "patient_name", "filename", "exam_type",
			"content_type", "bucket", "size", "referenced", "updated_at",
		}),
	}).Create(&rows).Error
}

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
		ExamType    string `json:"examType"`
		Eye         string `json:"eye"`
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

	// revert removes the just-uploaded object when it cannot be committed.
	revert := func(status int, message string) {
		cleanupObjects(r.Context(), client, []string{request.Path})
		writeError(w, status, message)
	}

	metadata, manifestErr := examMetadataFromManifest(r, client, bucket, request.CaseID, request.Path)
	if manifestErr != nil {
		if isNotFound(manifestErr) {
			revert(http.StatusNotFound, "caso não encontrado; arquivo revertido")
		} else {
			revert(http.StatusBadGateway, "não foi possível ler o caso; arquivo revertido")
		}
		return
	}
	// Identidade validada antes de qualquer escrita em SQL.
	if _, _, _, identityErr := patientIdentity(metadata.analysis); identityErr != nil {
		revert(http.StatusUnprocessableEntity, "paciente do caso sem nome ou data de nascimento confiáveis; arquivo revertido")
		return
	}

	examType, eye := metadata.examType, metadata.eye
	if !metadata.referenced {
		if patientfeature.IsOfficialExamKey(request.ExamType) {
			examType = request.ExamType
		}
		eye = strings.ToUpper(strings.TrimSpace(request.Eye))
	}
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

	var records []database.Exam
	txErr := appDatabase.Gorm.Transaction(func(tx *gorm.DB) error {
		patient, err := upsertPatientFromAnalysis(tx, metadata.analysis, request.CaseID)
		if err != nil {
			return err
		}
		for _, rowEye := range examEyes(eye) {
			records = append(records, database.Exam{
				CaseID:      request.CaseID,
				PatientID:   &patient.ID,
				PatientName: metadata.patientName,
				Filename:    filename,
				ExamType:    examTypeKey(examType),
				Eye:         rowEye,
				ContentType: contentType,
				Bucket:      bucket,
				ObjectKey:   request.Path,
				Size:        size,
				Referenced:  metadata.referenced,
			})
		}
		return upsertExamRows(tx, records)
	})
	if txErr != nil {
		if errors.Is(txErr, errPatientIdentity) {
			revert(http.StatusUnprocessableEntity, "paciente do caso sem nome ou data de nascimento confiáveis; arquivo revertido")
		} else {
			revert(http.StatusInternalServerError, "não foi possível salvar os metadados; arquivo revertido")
		}
		return
	}

	writeExamJSON(w, records[0])
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
	analysis    map[string]any
}

func examMetadataFromManifest(
	r *http.Request,
	client fileRepository,
	bucket string,
	caseID string,
	path string,
) (manifestExamMetadata, error) {
	result := manifestExamMetadata{}
	analysis, _, err := loadExamManifest(r.Context(), client, bucket, caseID)
	if err != nil {
		return result, err
	}
	result.analysis = analysis
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
			return result, nil
		}
	}
	return result, nil
}

func validExamObjectPath(caseID, path string) bool {
	prefix := fmt.Sprintf(refratiaCasesPrefix+"%s/", caseID)
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
