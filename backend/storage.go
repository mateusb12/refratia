package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"refratia/backend/fileStorage"
)

type fileRepository = fileStorage.Repository

const (
	refratiaCasesPrefix         = "refratia/cases/"
	refratiaDraftsPrefix        = "refratia/drafts/"
	refratiaCheckpointsPrefix   = "refratia/file-checkpoints/"
	refratiaConfirmationsPrefix = "refratia/intake-confirmations/"
)

func storageClient(operationContext context.Context) (fileRepository, error) {
	return fileStorage.New(operationContext)
}

func storagePresigner(operationContext context.Context) (fileStorage.PresignRepository, error) {
	client, err := fileStorage.NewPresigner(operationContext)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func cleanupObjects(operationContext context.Context, client fileRepository, objectKeys []string) {
	var firstErr error
	failed := 0
	for _, key := range objectKeys {
		if _, err := client.DeleteObject(operationContext, &s3.DeleteObjectInput{Bucket: aws.String(os.Getenv("BUCKET_NAME")), Key: aws.String(key)}); err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if failed > 0 {
		// Keys carry patient names: log counts only. Leftovers are garbage for cmd/reconcile.
		log.Printf("cleanup: %d/%d objects not deleted: %v", failed, len(objectKeys), firstErr)
	}
}
