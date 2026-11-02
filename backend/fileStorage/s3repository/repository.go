package s3repository

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Repository struct {
	*s3.Client
	*s3.PresignClient
}

func New(ctx context.Context, endpoint string) (*Repository, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})

	return &Repository{
		Client:        client,
		PresignClient: s3.NewPresignClient(client),
	}, nil
}
