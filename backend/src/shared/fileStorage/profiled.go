package fileStorage

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"refratia/backend/src/shared/profiling"
)

// profiledRepository times every Garage call as a garage.<op> span. Only
// installed when profiling is on. GetObject is timed to the response headers;
// reading the body happens in the caller.
type profiledRepository struct{ Repository }

func (repository profiledRepository) PutObject(ctx context.Context, input *s3.PutObjectInput, options ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	span := profiling.Start(ctx, "garage.put")
	if input.ContentLength != nil {
		span.Set("bytes", *input.ContentLength)
	}
	output, err := repository.Repository.PutObject(ctx, input, options...)
	span.End(err)
	return output, err
}

func (repository profiledRepository) GetObject(ctx context.Context, input *s3.GetObjectInput, options ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	span := profiling.Start(ctx, "garage.get")
	output, err := repository.Repository.GetObject(ctx, input, options...)
	if err == nil && output.ContentLength != nil {
		span.Set("bytes", *output.ContentLength)
	}
	span.Set("not_found", err != nil && IsNotFound(err)).End(err)
	return output, err
}

func (repository profiledRepository) DeleteObject(ctx context.Context, input *s3.DeleteObjectInput, options ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	span := profiling.Start(ctx, "garage.delete")
	output, err := repository.Repository.DeleteObject(ctx, input, options...)
	span.End(err)
	return output, err
}

func (repository profiledRepository) CopyObject(ctx context.Context, input *s3.CopyObjectInput, options ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	span := profiling.Start(ctx, "garage.copy")
	output, err := repository.Repository.CopyObject(ctx, input, options...)
	span.End(err)
	return output, err
}

func (repository profiledRepository) HeadObject(ctx context.Context, input *s3.HeadObjectInput, options ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	span := profiling.Start(ctx, "garage.head")
	output, err := repository.Repository.HeadObject(ctx, input, options...)
	span.Set("not_found", err != nil && IsNotFound(err)).End(err)
	return output, err
}

func (repository profiledRepository) ListObjectsV2(ctx context.Context, input *s3.ListObjectsV2Input, options ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	span := profiling.Start(ctx, "garage.list")
	output, err := repository.Repository.ListObjectsV2(ctx, input, options...)
	if err == nil {
		span.Set("keys", len(output.Contents)+len(output.CommonPrefixes))
	}
	span.End(err)
	return output, err
}
