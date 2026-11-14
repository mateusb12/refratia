package fileStorage

import (
	"context"
	"errors"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

const (
	CasesPrefix         = "refratia/cases/"
	DraftsPrefix        = "refratia/drafts/"
	CheckpointsPrefix   = "refratia/file-checkpoints/"
	ConfirmationsPrefix = "refratia/intake-confirmations/"
)

// MaxAnalysisSize caps how many bytes of a stored analysis JSON are read.
const MaxAnalysisSize = 5 << 20

func CleanupObjects(operationContext context.Context, client Repository, objectKeys []string) {
	var firstError error
	failed := 0
	for _, objectKey := range objectKeys {
		if _, deleteError := client.DeleteObject(operationContext, &s3.DeleteObjectInput{Bucket: aws.String(os.Getenv("BUCKET_NAME")), Key: aws.String(objectKey)}); deleteError != nil {
			failed++
			if firstError == nil {
				firstError = deleteError
			}
		}
	}
	if failed > 0 {
		// Keys carry patient names: log counts only. Leftovers are garbage for cmd/reconcile.
		log.Printf("cleanup: %d/%d objects not deleted: %v", failed, len(objectKeys), firstError)
	}
}

func IsNotFound(errorValue error) bool {
	var apiError smithy.APIError
	return errors.As(errorValue, &apiError) && (apiError.ErrorCode() == "NoSuchKey" || apiError.ErrorCode() == "NotFound")
}
