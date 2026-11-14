package cases

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gorm.io/gorm"

	"refratia/backend/src/shared/database"
	"refratia/backend/src/shared/fileStorage"
	"refratia/backend/src/shared/httpresponse"
)

// Handler serves /api/cases and /api/cases/{caseId}.
type Handler struct {
	Database *gorm.DB
}

func (handler Handler) ServeCases(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	client, storageError := fileStorage.New(request.Context())
	if storageError != nil {
		httpresponse.WriteError(responseWriter, http.StatusInternalServerError, "storage indisponível")
		return
	}
	listed, listError := client.ListObjectsV2(request.Context(), &s3.ListObjectsV2Input{
		Bucket:    aws.String(os.Getenv("BUCKET_NAME")),
		Prefix:    aws.String(fileStorage.CasesPrefix),
		Delimiter: aws.String("/"),
	})
	if listError != nil {
		httpresponse.WriteError(responseWriter, http.StatusBadGateway, "não foi possível listar os casos")
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
		caseID := strings.TrimSuffix(strings.TrimPrefix(casePrefix, fileStorage.CasesPrefix), "/")
		analysisKey := casePrefix + "paciente_compilado.json"
		object, getError := client.GetObject(request.Context(), &s3.GetObjectInput{
			Bucket: aws.String(os.Getenv("BUCKET_NAME")),
			Key:    aws.String(analysisKey),
		})
		if getError != nil {
			continue
		}
		var analysis struct {
			Patient struct {
				FullName string `json:"full_name"`
			} `json:"patient"`
		}
		decodeError := json.NewDecoder(io.LimitReader(object.Body, 1<<20)).Decode(&analysis)
		object.Body.Close()
		if decodeError != nil {
			continue
		}
		cases = append(cases, storedCase{CaseID: caseID, PatientName: analysis.Patient.FullName, AnalysisKey: analysisKey})
	}
	sort.Slice(cases, func(left, right int) bool { return cases[left].CaseID > cases[right].CaseID })
	responseWriter.Header().Set("Content-Type", "application/json")
	json.NewEncoder(responseWriter).Encode(map[string]any{"cases": cases})
}

func (handler Handler) ServeCase(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodDelete {
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	caseID := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/cases/"), "/")
	if !ValidID(caseID) {
		httpresponse.WriteError(responseWriter, http.StatusBadRequest, "caso inválido")
		return
	}
	if request.Method == http.MethodDelete {
		handler.deleteCase(responseWriter, request, caseID)
		return
	}
	client, storageError := fileStorage.New(request.Context())
	if storageError != nil {
		httpresponse.WriteError(responseWriter, http.StatusInternalServerError, "storage indisponível")
		return
	}
	analysisKey := fmt.Sprintf(fileStorage.CasesPrefix+"%s/paciente_compilado.json", caseID)
	object, getError := client.GetObject(request.Context(), &s3.GetObjectInput{
		Bucket: aws.String(os.Getenv("BUCKET_NAME")),
		Key:    aws.String(analysisKey),
	})
	if getError != nil {
		httpresponse.WriteError(responseWriter, http.StatusNotFound, "caso não encontrado")
		return
	}
	defer object.Body.Close()
	var analysis map[string]any
	if decodeError := json.NewDecoder(io.LimitReader(object.Body, fileStorage.MaxAnalysisSize)).Decode(&analysis); decodeError != nil {
		httpresponse.WriteError(responseWriter, http.StatusBadGateway, "não foi possível ler a análise do caso")
		return
	}
	presigner, presignerError := fileStorage.NewPresigner(request.Context())
	if presignerError != nil {
		httpresponse.WriteError(responseWriter, http.StatusInternalServerError, "storage indisponível")
		return
	}
	if files, ok := analysis["source_files"].([]any); ok {
		for _, rawFile := range files {
			file, _ := rawFile.(map[string]any)
			objectKey, ok := file["path"].(string)
			if !ok || !strings.HasPrefix(objectKey, fmt.Sprintf(fileStorage.CasesPrefix+"%s/", caseID)) {
				continue
			}
			signed, presignError := presigner.PresignGetObject(request.Context(), &s3.GetObjectInput{
				Bucket: aws.String(os.Getenv("BUCKET_NAME")),
				Key:    aws.String(objectKey),
			})
			if presignError == nil {
				file["signed_url"] = signed.URL
			}
		}
	}
	responseWriter.Header().Set("Content-Type", "application/json")
	json.NewEncoder(responseWriter).Encode(map[string]any{"caseId": caseID, "analysis": analysis})
}

func (handler Handler) deleteCase(responseWriter http.ResponseWriter, request *http.Request, caseID string) {
	token := os.Getenv("CASE_DELETE_TOKEN")
	if token == "" {
		httpresponse.WriteError(responseWriter, http.StatusNotFound, "exclusão de casos desabilitada")
		return
	}
	provided := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
		httpresponse.WriteError(responseWriter, http.StatusUnauthorized, "token de exclusão inválido")
		return
	}
	client, storageError := fileStorage.New(request.Context())
	if storageError != nil {
		httpresponse.WriteError(responseWriter, http.StatusInternalServerError, "storage indisponível")
		return
	}
	bucket := os.Getenv("BUCKET_NAME")
	prefix := fmt.Sprintf(fileStorage.CasesPrefix+"%s/", caseID)
	marker := prefix + "paciente_compilado.json"
	objectKeys := make([]string, 0)
	var continuation *string
	markerExists := false
	for {
		listed, listError := client.ListObjectsV2(request.Context(), &s3.ListObjectsV2Input{
			Bucket: aws.String(bucket), Prefix: aws.String(prefix), ContinuationToken: continuation,
		})
		if listError != nil {
			httpresponse.WriteError(responseWriter, http.StatusBadGateway, "não foi possível listar os arquivos do caso")
			return
		}
		for _, object := range listed.Contents {
			objectKey := aws.ToString(object.Key)
			if objectKey == marker {
				markerExists = true
			} else {
				objectKeys = append(objectKeys, objectKey)
			}
		}
		if !aws.ToBool(listed.IsTruncated) {
			break
		}
		continuation = listed.NextContinuationToken
	}
	if len(objectKeys) == 0 && !markerExists {
		httpresponse.WriteError(responseWriter, http.StatusNotFound, "caso não encontrado")
		return
	}
	// D9: SQL primeiro (idempotente): se o Garage falhar no meio, o retry
	// ainda encontra os objetos e repete a limpeza.
	if sqlError := handler.deleteCaseRows(caseID); sqlError != nil {
		httpresponse.WriteError(responseWriter, http.StatusInternalServerError, "não foi possível apagar os registros do caso")
		return
	}
	deletedCount := len(objectKeys)
	if markerExists {
		deletedCount++
	}
	for _, objectKey := range append(objectKeys, marker) {
		if objectKey == marker && !markerExists {
			continue
		}
		if _, deleteError := client.DeleteObject(request.Context(), &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(objectKey)}); deleteError != nil {
			httpresponse.WriteError(responseWriter, http.StatusBadGateway, "não foi possível apagar todos os arquivos do caso")
			return
		}
	}

	deletedReceipts, receiptsError := deleteCaseConfirmationReceipts(request.Context(), client, bucket, caseID)
	if receiptsError != nil {
		httpresponse.WriteError(responseWriter, http.StatusBadGateway, "não foi possível apagar todos os rastros de confirmação do caso")
		return
	}
	responseWriter.Header().Set("Content-Type", "application/json")
	json.NewEncoder(responseWriter).Encode(map[string]any{
		"caseId":          caseID,
		"deletedObjects":  deletedCount + deletedReceipts,
		"deletedReceipts": deletedReceipts,
	})
}

func deleteCaseConfirmationReceipts(operationContext context.Context, client fileStorage.Repository, bucket, caseID string) (int, error) {
	deleted := 0
	var continuation *string

	for {
		listed, listError := client.ListObjectsV2(operationContext, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String(fileStorage.ConfirmationsPrefix),
			ContinuationToken: continuation,
		})
		if listError != nil {
			return deleted, listError
		}

		for _, object := range listed.Contents {
			objectKey := aws.ToString(object.Key)
			if objectKey == "" {
				continue
			}

			receiptObject, getError := client.GetObject(operationContext, &s3.GetObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(objectKey),
			})
			if getError != nil {
				return deleted, getError
			}

			var receipt ConfirmationReceipt
			decodeError := json.NewDecoder(io.LimitReader(receiptObject.Body, 64<<10)).Decode(&receipt)
			receiptObject.Body.Close()
			if decodeError != nil || receipt.CaseID != caseID {
				continue
			}

			if _, deleteError := client.DeleteObject(operationContext, &s3.DeleteObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(objectKey),
			}); deleteError != nil {
				return deleted, deleteError
			}
			deleted++
		}

		if !aws.ToBool(listed.IsTruncated) {
			break
		}
		continuation = listed.NextContinuationToken
	}

	return deleted, nil
}

// ValidID accepts "case-<intake stamp>": the same shape as intake.ValidID with
// the "case-" prefix. ponytail: copied here because intake imports cases.
func ValidID(caseID string) bool {
	if !strings.HasPrefix(caseID, "case-") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(caseID, "case-"), "-")
	if len(parts) != 2 || len(parts[1]) != 8 {
		return false
	}
	_, timeError := time.Parse("20060102T150405Z", parts[0])
	_, tokenError := hex.DecodeString(parts[1])
	return timeError == nil && tokenError == nil
}

// deleteCaseRows removes the exams of the case and its patient row (one case
// per patient, so the patient has nothing left once the case is gone).
func (handler Handler) deleteCaseRows(caseID string) error {
	if handler.Database == nil {
		return errors.New("database is not initialized")
	}
	return handler.Database.Transaction(func(transaction *gorm.DB) error {
		if deleteError := transaction.Where("case_id = ?", caseID).Delete(&database.Exam{}).Error; deleteError != nil {
			return deleteError
		}
		return transaction.Where("case_id = ?", caseID).Delete(&database.Patient{}).Error
	})
}
