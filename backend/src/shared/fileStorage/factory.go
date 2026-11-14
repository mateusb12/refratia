package fileStorage

import (
	"context"
	"fmt"
	"os"

	"refratia/backend/src/shared/fileStorage/garage"
	"refratia/backend/src/shared/fileStorage/minio"
	"refratia/backend/src/shared/fileStorage/s3repository"
)

func New(ctx context.Context) (Repository, error) {
	return newRepository(ctx, endpoint(false))
}

func NewPresigner(ctx context.Context) (PresignRepository, error) {
	return newRepository(ctx, endpoint(true))
}

func newRepository(ctx context.Context, configuredEndpoint string) (Repository, error) {
	driver := os.Getenv("FILE_STORAGE_DRIVER")
	if driver == "" {
		driver = "minio"
	}

	var (
		repository *s3repository.Repository
		err        error
	)

	switch driver {
	case "minio":
		repository, err = minio.New(ctx, configuredEndpoint)
	case "garage":
		repository, err = garage.New(ctx, configuredEndpoint)
	default:
		return nil, fmt.Errorf("file storage driver inválido: %s", driver)
	}
	if err != nil {
		return nil, err
	}

	return repository, nil
}

func endpoint(public bool) string {
	if os.Getenv("FILE_STORAGE_DRIVER") == "garage" {
		if public {
			if value := os.Getenv("GARAGE_S3_PUBLIC_API_URL"); value != "" {
				return value
			}
		}
		if value := os.Getenv("GARAGE_S3_API_URL"); value != "" {
			return value
		}
	}

	if public {
		if value := os.Getenv("FILE_STORAGE_PUBLIC_ENDPOINT_URL"); value != "" {
			return value
		}
		if value := os.Getenv("AWS_PUBLIC_ENDPOINT_URL_S3"); value != "" {
			return value
		}
	}

	if value := os.Getenv("FILE_STORAGE_ENDPOINT_URL"); value != "" {
		return value
	}
	if value := os.Getenv("AWS_ENDPOINT_URL_S3"); value != "" {
		return value
	}
	return os.Getenv("GARAGE_S3_API_URL")
}
