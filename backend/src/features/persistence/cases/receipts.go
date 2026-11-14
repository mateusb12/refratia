package cases

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gorm.io/gorm"

	patientfeature "refratia/backend/src/features/persistence/patient"
	"refratia/backend/src/shared/fileStorage"
)

type ConfirmationReceipt struct {
	CaseID      string `json:"caseId"`
	AnalysisKey string `json:"analysisKey"`
	Action      string `json:"action"`
}

type patientCaseCandidate struct {
	CaseID   string
	Analysis map[string]any
}

func FindExistingPatientCase(
	operationContext context.Context,
	client fileStorage.Repository,
	bucket string,
	incoming map[string]any,
) (string, map[string]any, bool, error) {
	identity, ok := patientfeature.Identity(incoming)
	if !ok {
		return "", nil, false, nil
	}

	candidates := make([]patientCaseCandidate, 0)
	var continuation *string

	for {
		listed, listError := client.ListObjectsV2(operationContext, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String(fileStorage.CasesPrefix),
			Delimiter:         aws.String("/"),
			ContinuationToken: continuation,
		})
		if listError != nil {
			return "", nil, false, listError
		}

		for _, prefix := range listed.CommonPrefixes {
			casePrefix := aws.ToString(prefix.Prefix)
			caseID := strings.TrimSuffix(strings.TrimPrefix(casePrefix, fileStorage.CasesPrefix), "/")
			if caseID == "" {
				continue
			}

			analysis, loadError := LoadStoredCaseAnalysis(operationContext, client, bucket, caseID)
			if loadError != nil {
				var syntaxError *json.SyntaxError
				if fileStorage.IsNotFound(loadError) || errors.As(loadError, &syntaxError) || errors.Is(loadError, io.ErrUnexpectedEOF) {
					// Um case sem marcador ou corrompido não impede os demais,
					// mas fica registrado (sem PII: só o id do case).
					log.Printf("identity scan: skipping unreadable case %s: %v", caseID, loadError)
					continue
				}
				// Falha de storage: não podemos afirmar que o paciente não existe.
				return "", nil, false, loadError
			}

			existingIdentity, identifiable := patientfeature.Identity(analysis)
			if identifiable && existingIdentity == identity {
				candidates = append(candidates, patientCaseCandidate{
					CaseID:   caseID,
					Analysis: analysis,
				})
			}
		}

		if !aws.ToBool(listed.IsTruncated) {
			break
		}

		continuation = listed.NextContinuationToken
	}

	if len(candidates) == 0 {
		return "", nil, false, nil
	}

	// Se já existirem duplicatas legadas do mesmo paciente, usamos o case
	// lexicograficamente mais recente. Não apagamos nada automaticamente.
	sort.Slice(candidates, func(left, right int) bool {
		return candidates[left].CaseID < candidates[right].CaseID
	})

	selected := candidates[len(candidates)-1]
	return selected.CaseID, selected.Analysis, true, nil
}

func LoadStoredCaseAnalysis(
	operationContext context.Context,
	client fileStorage.Repository,
	bucket string,
	caseID string,
) (map[string]any, error) {
	objectKey := fileStorage.CasesPrefix + caseID + "/paciente_compilado.json"

	object, getError := client.GetObject(operationContext, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	})
	if getError != nil {
		return nil, getError
	}
	defer object.Body.Close()

	var analysis map[string]any
	if decodeError := json.NewDecoder(io.LimitReader(object.Body, fileStorage.MaxAnalysisSize)).Decode(&analysis); decodeError != nil {
		return nil, decodeError
	}

	return analysis, nil
}

func ConfirmationKey(intakeID string) string {
	return fileStorage.ConfirmationsPrefix + intakeID + ".json"
}

func LoadConfirmationReceipt(
	operationContext context.Context,
	client fileStorage.Repository,
	bucket string,
	intakeID string,
) (ConfirmationReceipt, bool) {
	object, getError := client.GetObject(operationContext, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(ConfirmationKey(intakeID)),
	})
	if getError != nil {
		return ConfirmationReceipt{}, false
	}
	defer object.Body.Close()

	var receipt ConfirmationReceipt
	if json.NewDecoder(io.LimitReader(object.Body, 64<<10)).Decode(&receipt) != nil {
		return ConfirmationReceipt{}, false
	}

	if receipt.CaseID == "" || receipt.AnalysisKey == "" {
		return ConfirmationReceipt{}, false
	}

	return receipt, true
}

func StoreConfirmationReceipt(
	operationContext context.Context,
	client fileStorage.Repository,
	bucket string,
	intakeID string,
	receipt ConfirmationReceipt,
) error {
	raw, marshalError := json.Marshal(receipt)
	if marshalError != nil {
		return marshalError
	}

	_, putError := client.PutObject(operationContext, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(ConfirmationKey(intakeID)),
		Body:        bytes.NewReader(raw),
		ContentType: aws.String("application/json"),
	})

	return putError
}

// ResolveExistingCase finds the case of an already-known patient: the SQL row
// (patients.case_id) is authoritative; the O(cases) Garage scan is only the
// fallback for legacy cases that have no row/case_id yet.
// ponytail: the scan also runs for brand-new patients; drop it once legacy
// cases are backfilled (cmd/reconcile) .
func ResolveExistingCase(
	operationContext context.Context,
	sqlDatabase *gorm.DB,
	client fileStorage.Repository,
	bucket string,
	analysis map[string]any,
) (string, map[string]any, bool, error) {
	identity, ok := patientfeature.Identity(analysis)
	if !ok {
		return "", nil, false, nil
	}
	caseID, lookupError := patientfeature.CaseFromSQL(sqlDatabase, identity)
	if lookupError != nil {
		return "", nil, false, lookupError
	}
	if caseID != "" {
		stored, loadError := LoadStoredCaseAnalysis(operationContext, client, bucket, caseID)
		if loadError == nil {
			return caseID, stored, true, nil
		}
		if !fileStorage.IsNotFound(loadError) {
			return "", nil, false, loadError
		}
	}
	return FindExistingPatientCase(operationContext, client, bucket, analysis)
}
