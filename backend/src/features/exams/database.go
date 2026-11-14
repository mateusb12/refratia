package exams

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

	"refratia/backend/src/features/cases"
	patientfeature "refratia/backend/src/features/patient"
	"refratia/backend/src/shared/database"
	"refratia/backend/src/shared/fileStorage"
	"refratia/backend/src/shared/httpresponse"
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
	"ANTSEG":                "anterior_segment_photo",
	"TOPO":                  "corneal_topography_other",
}

// outOfProtocolExamKeys are persisted as-is (rows outside the protocol).
var outOfProtocolExamKeys = map[string]bool{"anterior_segment_photo": true, "corneal_topography_other": true}

func TypeKey(label string) string {
	if key, ok := examTypeKeys[label]; ok {
		return key
	}
	if patientfeature.IsOfficialExamKey(label) || outOfProtocolExamKeys[label] {
		return label
	}
	return "unclassified"
}

// Eyes expands a document eye into the eyes it holds: AO documents are
// binocular and produce one row per eye (contract C1).
func Eyes(eye string) []string {
	switch strings.ToUpper(strings.TrimSpace(eye)) {
	case "AO":
		return []string{"OD", "OS"}
	case "OE":
		return []string{"OS"}
	default:
		return []string{strings.ToUpper(strings.TrimSpace(eye))}
	}
}

// UpsertRows writes one row per (document, eye), idempotent on
// UNIQUE(object_key, eye).
func UpsertRows(transaction *gorm.DB, rows []database.Exam) error {
	if len(rows) == 0 {
		return nil
	}
	return transaction.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "object_key"}, {Name: "eye"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"patient_id", "case_id", "patient_name", "filename", "exam_type",
			"content_type", "bucket", "size", "referenced", "updated_at",
		}),
	}).Create(&rows).Error
}

func (handler Handler) ServeConfirm(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !authorizeExamMutation(responseWriter, request) {
		return
	}

	var payload struct {
		CaseID      string `json:"caseId"`
		Path        string `json:"path"`
		Filename    string `json:"filename"`
		ContentType string `json:"contentType"`
		Size        int64  `json:"size"`
		ExamType    string `json:"examType"`
		Eye         string `json:"eye"`
	}
	if decodeError := json.NewDecoder(io.LimitReader(request.Body, 64<<10)).Decode(&payload); decodeError != nil {
		httpresponse.WriteError(responseWriter, http.StatusBadRequest, "payload inválido")
		return
	}
	if !cases.ValidID(payload.CaseID) || !validExamObjectPath(payload.CaseID, payload.Path) {
		httpresponse.WriteError(responseWriter, http.StatusBadRequest, "arquivo inválido")
		return
	}

	client, storageError := fileStorage.New(request.Context())
	if storageError != nil {
		httpresponse.WriteError(responseWriter, http.StatusInternalServerError, "storage indisponível")
		return
	}
	bucket := os.Getenv("BUCKET_NAME")
	head, headError := client.HeadObject(request.Context(), &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(payload.Path),
	})
	if headError != nil {
		httpresponse.WriteError(responseWriter, http.StatusNotFound, "arquivo não encontrado no Garage")
		return
	}

	if handler.Database == nil {
		httpresponse.WriteError(responseWriter, http.StatusInternalServerError, "banco de dados indisponível")
		return
	}

	var existing database.Exam
	lookup := handler.Database.Where("object_key = ?", payload.Path).First(&existing)
	if lookup.Error == nil {
		writeExamJSON(responseWriter, existing)
		return
	}
	if !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
		httpresponse.WriteError(responseWriter, http.StatusInternalServerError, "não foi possível consultar o exame")
		return
	}

	// revert removes the just-uploaded object when it cannot be committed.
	revert := func(status int, message string) {
		fileStorage.CleanupObjects(request.Context(), client, []string{payload.Path})
		httpresponse.WriteError(responseWriter, status, message)
	}

	metadata, manifestErr := examMetadataFromManifest(request, client, bucket, payload.CaseID, payload.Path)
	if manifestErr != nil {
		if fileStorage.IsNotFound(manifestErr) {
			revert(http.StatusNotFound, "caso não encontrado; arquivo revertido")
		} else {
			revert(http.StatusBadGateway, "não foi possível ler o caso; arquivo revertido")
		}
		return
	}
	// Identidade validada antes de qualquer escrita em SQL.
	if _, _, _, identityErr := patientfeature.IdentityFromAnalysis(metadata.analysis); identityErr != nil {
		revert(http.StatusUnprocessableEntity, "paciente do caso sem nome ou data de nascimento confiáveis; arquivo revertido")
		return
	}

	examType, eye := metadata.examType, metadata.eye
	if !metadata.referenced {
		if patientfeature.IsOfficialExamKey(payload.ExamType) || outOfProtocolExamKeys[payload.ExamType] {
			examType = payload.ExamType
		}
		eye = strings.ToUpper(strings.TrimSpace(payload.Eye))
	}
	filename := filepath.Base(strings.TrimSpace(payload.Filename))
	if filename == "." || filename == "" {
		filename = filepath.Base(payload.Path)
	}
	size := payload.Size
	if head.ContentLength != nil {
		size = *head.ContentLength
	}
	contentType := payload.ContentType
	if contentType == "" {
		contentType = aws.ToString(head.ContentType)
	}

	var records []database.Exam
	transactionError := handler.Database.Transaction(func(transaction *gorm.DB) error {
		patient, upsertError := patientfeature.UpsertFromAnalysis(transaction, metadata.analysis, payload.CaseID)
		if upsertError != nil {
			return upsertError
		}
		for _, rowEye := range Eyes(eye) {
			records = append(records, database.Exam{
				CaseID:      payload.CaseID,
				PatientID:   &patient.ID,
				PatientName: metadata.patientName,
				Filename:    filename,
				ExamType:    TypeKey(examType),
				Eye:         rowEye,
				ContentType: contentType,
				Bucket:      bucket,
				ObjectKey:   payload.Path,
				Size:        size,
				Referenced:  metadata.referenced,
			})
		}
		return UpsertRows(transaction, records)
	})
	if transactionError != nil {
		if errors.Is(transactionError, patientfeature.ErrIdentity) {
			revert(http.StatusUnprocessableEntity, "paciente do caso sem nome ou data de nascimento confiáveis; arquivo revertido")
		} else {
			revert(http.StatusInternalServerError, "não foi possível salvar os metadados; arquivo revertido")
		}
		return
	}

	writeExamJSON(responseWriter, records[0])
}

func (handler Handler) deleteExamRecord(objectKey, bucket string) error {
	if handler.Database == nil {
		return fmt.Errorf("database is not initialized")
	}
	return handler.Database.Where("object_key = ? AND bucket = ?", objectKey, bucket).Delete(&database.Exam{}).Error
}

type manifestExamMetadata struct {
	patientName string
	examType    string
	eye         string
	referenced  bool
	analysis    map[string]any
}

func examMetadataFromManifest(
	request *http.Request,
	client fileStorage.Repository,
	bucket string,
	caseID string,
	path string,
) (manifestExamMetadata, error) {
	result := manifestExamMetadata{}
	analysis, _, manifestError := loadExamManifest(request.Context(), client, bucket, caseID)
	if manifestError != nil {
		return result, manifestError
	}
	result.analysis = analysis
	if patient, ok := analysis["patient"].(map[string]any); ok {
		result.patientName = readExamMetadata(patient, "full_name")
	}
	ClassifySourceFiles(analysis)
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
	prefix := fmt.Sprintf(fileStorage.CasesPrefix+"%s/", caseID)
	return strings.HasPrefix(path, prefix) && path != prefix && path != prefix+"paciente_compilado.json"
}

func writeExamJSON(responseWriter http.ResponseWriter, record database.Exam) {
	responseWriter.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(responseWriter).Encode(map[string]any{
		"id":        record.ID,
		"caseId":    record.CaseID,
		"objectKey": record.ObjectKey,
		"filename":  record.Filename,
		"exam":      record.ExamType,
		"eye":       record.Eye,
	})
}
