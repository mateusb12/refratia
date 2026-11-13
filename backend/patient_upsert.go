package main

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
	"github.com/aws/smithy-go"
	"gorm.io/gorm"

	patientfeature "refratia/backend/features/patient"
)

type confirmationReceipt struct {
	CaseID      string `json:"caseId"`
	AnalysisKey string `json:"analysisKey"`
	Action      string `json:"action"`
}

type patientCaseCandidate struct {
	CaseID   string
	Analysis map[string]any
}

func findExistingPatientCase(
	ctx context.Context,
	client fileRepository,
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
		listed, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String(refratiaCasesPrefix),
			Delimiter:         aws.String("/"),
			ContinuationToken: continuation,
		})
		if err != nil {
			return "", nil, false, err
		}

		for _, prefix := range listed.CommonPrefixes {
			casePrefix := aws.ToString(prefix.Prefix)
			caseID := strings.TrimSuffix(strings.TrimPrefix(casePrefix, refratiaCasesPrefix), "/")
			if caseID == "" {
				continue
			}

			analysis, err := loadStoredCaseAnalysis(ctx, client, bucket, caseID)
			if err != nil {
				var syntaxErr *json.SyntaxError
				if isNotFound(err) || errors.As(err, &syntaxErr) || errors.Is(err, io.ErrUnexpectedEOF) {
					// Um case sem marcador ou corrompido não impede os demais,
					// mas fica registrado (sem PII: só o id do case).
					log.Printf("identity scan: skipping unreadable case %s: %v", caseID, err)
					continue
				}
				// Falha de storage: não podemos afirmar que o paciente não existe.
				return "", nil, false, err
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
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CaseID < candidates[j].CaseID
	})

	selected := candidates[len(candidates)-1]
	return selected.CaseID, selected.Analysis, true, nil
}

func loadStoredCaseAnalysis(
	ctx context.Context,
	client fileRepository,
	bucket string,
	caseID string,
) (map[string]any, error) {
	key := refratiaCasesPrefix + caseID + "/paciente_compilado.json"

	object, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer object.Body.Close()

	var analysis map[string]any
	if err := json.NewDecoder(io.LimitReader(object.Body, maxAnalysisSize)).Decode(&analysis); err != nil {
		return nil, err
	}

	return analysis, nil
}

func confirmationKey(intakeID string) string {
	return refratiaConfirmationsPrefix + intakeID + ".json"
}

func loadConfirmationReceipt(
	ctx context.Context,
	client fileRepository,
	bucket string,
	intakeID string,
) (confirmationReceipt, bool) {
	object, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(confirmationKey(intakeID)),
	})
	if err != nil {
		return confirmationReceipt{}, false
	}
	defer object.Body.Close()

	var receipt confirmationReceipt
	if json.NewDecoder(io.LimitReader(object.Body, 64<<10)).Decode(&receipt) != nil {
		return confirmationReceipt{}, false
	}

	if receipt.CaseID == "" || receipt.AnalysisKey == "" {
		return confirmationReceipt{}, false
	}

	return receipt, true
}

func storeConfirmationReceipt(
	ctx context.Context,
	client fileRepository,
	bucket string,
	intakeID string,
	receipt confirmationReceipt,
) error {
	raw, err := json.Marshal(receipt)
	if err != nil {
		return err
	}

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(confirmationKey(intakeID)),
		Body:        bytes.NewReader(raw),
		ContentType: aws.String("application/json"),
	})

	return err
}

func isNotFound(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NoSuchKey" || apiErr.ErrorCode() == "NotFound")
}

// resolveExistingCase finds the case of an already-known patient: the SQL row
// (patients.case_id) is authoritative; the O(cases) Garage scan is only the
// fallback for legacy cases that have no row/case_id yet.
// ponytail: the scan also runs for brand-new patients; drop it once legacy
// cases are backfilled (cmd/reconcile) .
func resolveExistingCase(
	ctx context.Context,
	db *gorm.DB,
	client fileRepository,
	bucket string,
	analysis map[string]any,
) (string, map[string]any, bool, error) {
	identity, ok := patientfeature.Identity(analysis)
	if !ok {
		return "", nil, false, nil
	}
	caseID, err := patientCaseFromSQL(db, identity)
	if err != nil {
		return "", nil, false, err
	}
	if caseID != "" {
		stored, loadErr := loadStoredCaseAnalysis(ctx, client, bucket, caseID)
		if loadErr == nil {
			return caseID, stored, true, nil
		}
		if !isNotFound(loadErr) {
			return "", nil, false, loadErr
		}
	}
	return findExistingPatientCase(ctx, client, bucket, analysis)
}
