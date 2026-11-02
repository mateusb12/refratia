package garage

import (
	"context"

	"refratia/backend/fileStorage/s3repository"
)

func New(ctx context.Context, endpoint string) (*s3repository.Repository, error) {
	return s3repository.New(ctx, endpoint)
}
