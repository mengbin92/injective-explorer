// Package storage provides AWS S3 implementation for object storage.
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/pkg/errors"
)

// S3Storage implements Storage interface using AWS S3 client.
type S3Storage struct {
	client     *s3.Client
	bucketName string
	pathPrefix string
}

// NewS3Storage creates a new S3 storage instance.
//
// Parameters:
//   - endpoint: S3 endpoint (e.g., "s3.amazonaws.com" or custom endpoint)
//   - accessKeyID: AWS access key ID for authentication
//   - secretAccessKey: AWS secret access key for authentication
//   - bucketName: Name of the S3 bucket to use
//   - region: AWS region (e.g., "us-east-1")
//   - useSSL: Whether to use SSL/TLS for connections
//   - pathPrefix: Path prefix for all object keys
//
// Returns:
//   - *S3Storage: A new S3 storage instance
//   - error: Error if initialization fails
func NewS3Storage(endpoint, accessKeyID, secretAccessKey, bucketName, region string, useSSL bool, pathPrefix string) (*S3Storage, error) {
	// Build AWS config
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
	)
	if err != nil {
		return nil, errors.Wrap(err, "load aws config")
	}

	// Create S3 client with custom endpoint if provided
	clientOptions := []func(*s3.Options){
		func(o *s3.Options) {
			o.Region = region
		},
	}

	// If custom endpoint is provided, use it (for S3-compatible services)
	if endpoint != "" && endpoint != "s3.amazonaws.com" && endpoint != fmt.Sprintf("s3.%s.amazonaws.com", region) {
		clientOptions = append(clientOptions, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(func() string {
				if useSSL {
					return fmt.Sprintf("https://%s", endpoint)
				}
				return fmt.Sprintf("http://%s", endpoint)
			}())
			o.UsePathStyle = true // Use path-style addressing for custom endpoints
		})
	}

	client := s3.NewFromConfig(cfg, clientOptions...)

	storage := &S3Storage{
		client:     client,
		bucketName: bucketName,
		pathPrefix: pathPrefix,
	}

	// Check if bucket exists
	ctx := context.Background()
	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		var notFound *types.NotFound
		var noSuchBucket *types.NoSuchBucket
		if errors.As(err, &notFound) || errors.As(err, &noSuchBucket) {
			// Bucket doesn't exist, try to create it
			createInput := &s3.CreateBucketInput{
				Bucket: aws.String(bucketName),
			}

			// For us-east-1, don't set LocationConstraint (it's the default region)
			// For other regions, set LocationConstraint
			if region != "" && region != "us-east-1" {
				createInput.CreateBucketConfiguration = &types.CreateBucketConfiguration{
					LocationConstraint: types.BucketLocationConstraint(region),
				}
			}

			_, createErr := client.CreateBucket(ctx, createInput)
			if createErr != nil {
				return nil, errors.Wrapf(createErr, "create bucket: %s", bucketName)
			}
		} else {
			return nil, errors.Wrapf(err, "check bucket existence: %s", bucketName)
		}
	}

	return storage, nil
}

// buildKey builds the full storage key by prepending the path prefix.
func (s *S3Storage) buildKey(key string) string {
	if s.pathPrefix == "" {
		return key
	}
	if s.pathPrefix[len(s.pathPrefix)-1] == '/' {
		return s.pathPrefix + key
	}
	return s.pathPrefix + "/" + key
}

// PutObject uploads an object to S3 storage.
func (s *S3Storage) PutObject(ctx context.Context, key string, data []byte) error {
	key = s.buildKey(key)
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	return errors.Wrapf(err, "put object: %s", key)
}

// PutObjectFromReader uploads an object from a reader to S3 storage.
func (s *S3Storage) PutObjectFromReader(ctx context.Context, key string, reader io.Reader, size int64) error {
	key = s.buildKey(key)
	uploader := manager.NewUploader(s.client)
	_, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
		Body:   reader,
	})
	return errors.Wrapf(err, "put object from reader: %s", key)
}

// GetObject retrieves an object from S3 storage.
func (s *S3Storage) GetObject(ctx context.Context, key string) ([]byte, error) {
	key = s.buildKey(key)
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return nil, ErrObjectNotFound
		}
		return nil, errors.Wrapf(err, "get object: %s", key)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "read object: %s", key)
	}
	return data, nil
}

// GetObjectReader returns a reader for an object in S3 storage.
func (s *S3Storage) GetObjectReader(ctx context.Context, key string) (io.ReadCloser, error) {
	key = s.buildKey(key)
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return nil, ErrObjectNotFound
		}
		return nil, errors.Wrapf(err, "get object reader: %s", key)
	}
	return result.Body, nil
}

// DeleteObject deletes an object from S3 storage.
func (s *S3Storage) DeleteObject(ctx context.Context, key string) error {
	key = s.buildKey(key)
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	return errors.Wrapf(err, "delete object: %s", key)
}

// Exists checks if an object exists in S3 storage.
func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	key = s.buildKey(key)
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		return false, errors.Wrapf(err, "head object: %s", key)
	}
	return true, nil
}

// GetObjectURL returns a presigned URL for accessing an object in S3 storage.
// If expiresIn is 0, defaults to 7 days expiration.
func (s *S3Storage) GetObjectURL(ctx context.Context, key string, expiresIn int64) (string, error) {
	key = s.buildKey(key)
	expires := time.Duration(expiresIn) * time.Second
	if expiresIn == 0 {
		expires = 7 * 24 * time.Hour // Default 7 days
	}

	presignClient := s3.NewPresignClient(s.client)
	request, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return "", errors.Wrapf(err, "get presigned url: %s", key)
	}
	return request.URL, nil
}
