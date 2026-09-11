package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

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
	client *s3.Client,
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
			Prefix:            aws.String("cases/"),
			Delimiter:         aws.String("/"),
			ContinuationToken: continuation,
		})
		if err != nil {
			return "", nil, false, err
		}

		for _, prefix := range listed.CommonPrefixes {
			casePrefix := aws.ToString(prefix.Prefix)
			caseID := strings.TrimSuffix(strings.TrimPrefix(casePrefix, "cases/"), "/")
			if caseID == "" {
				continue
			}

			analysis, err := loadStoredCaseAnalysis(ctx, client, bucket, caseID)
			if err != nil {
				// Um case quebrado não deve impedir a confirmação dos demais.
				continue
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
	client *s3.Client,
	bucket string,
	caseID string,
) (map[string]any, error) {
	key := "cases/" + caseID + "/paciente_compilado.json"

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
	return "intake-confirmations/" + intakeID + ".json"
}

func loadConfirmationReceipt(
	ctx context.Context,
	client *s3.Client,
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
	client *s3.Client,
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
