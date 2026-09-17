package main

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func storageClient(operationContext context.Context) (*s3.Client, error) {
	return newStorageClient(operationContext, os.Getenv("AWS_ENDPOINT_URL_S3"))
}

func storagePresigner(operationContext context.Context) (*s3.PresignClient, error) {
	endpoint := os.Getenv("AWS_PUBLIC_ENDPOINT_URL_S3")
	if endpoint == "" {
		endpoint = os.Getenv("AWS_ENDPOINT_URL_S3")
	}
	client, storageClientCreationError := newStorageClient(operationContext, endpoint)
	if storageClientCreationError != nil {
		return nil, storageClientCreationError
	}
	return s3.NewPresignClient(client), nil
}

func newStorageClient(operationContext context.Context, endpoint string) (*s3.Client, error) {
	storageConfig, configurationLoadError := config.LoadDefaultConfig(operationContext)
	if configurationLoadError != nil {
		return nil, configurationLoadError
	}
	return s3.NewFromConfig(storageConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	}), nil
}

func cleanupObjects(operationContext context.Context, client *s3.Client, objectKeys []string) {
	for _, key := range objectKeys {
		_, _ = client.DeleteObject(operationContext, &s3.DeleteObjectInput{Bucket: aws.String(os.Getenv("BUCKET_NAME")), Key: aws.String(key)})
	}
}
