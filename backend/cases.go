package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

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

	deletedReceipts, receiptsErr := deleteCaseConfirmationReceipts(r.Context(), client, bucket, caseID)
	if receiptsErr != nil {
		writeError(w, http.StatusBadGateway, "não foi possível apagar todos os rastros de confirmação do caso")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"caseId":          caseID,
		"deletedObjects":  deletedCount + deletedReceipts,
		"deletedReceipts": deletedReceipts,
	})
}

func deleteCaseConfirmationReceipts(ctx context.Context, client *s3.Client, bucket, caseID string) (int, error) {
	deleted := 0
	var continuation *string

	for {
		listed, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String("intake-confirmations/"),
			ContinuationToken: continuation,
		})
		if err != nil {
			return deleted, err
		}

		for _, object := range listed.Contents {
			key := aws.ToString(object.Key)
			if key == "" {
				continue
			}

			receiptObject, getErr := client.GetObject(ctx, &s3.GetObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(key),
			})
			if getErr != nil {
				return deleted, getErr
			}

			var receipt confirmationReceipt
			decodeErr := json.NewDecoder(io.LimitReader(receiptObject.Body, 64<<10)).Decode(&receipt)
			receiptObject.Body.Close()
			if decodeErr != nil || receipt.CaseID != caseID {
				continue
			}

			if _, deleteErr := client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(key),
			}); deleteErr != nil {
				return deleted, deleteErr
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

func validCaseID(id string) bool {
	return strings.HasPrefix(id, "case-") && validIntakeID("intake-"+strings.TrimPrefix(id, "case-"))
}
