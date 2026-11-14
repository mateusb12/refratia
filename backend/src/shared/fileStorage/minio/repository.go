package minio

import (
	"context"

	"refratia/backend/src/shared/fileStorage/s3repository"
)

func New(ctx context.Context, endpoint string) (*s3repository.Repository, error) {
	return s3repository.New(ctx, endpoint)
}
