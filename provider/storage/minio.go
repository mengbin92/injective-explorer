// Package storage provides MinIO implementation for object storage.
package storage

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
)

// MinIOStorage implements Storage interface using MinIO client.
type MinIOStorage struct {
	client     *minio.Client
	bucketName string
	pathPrefix string
}

// NewMinIOStorage creates a new MinIO storage instance.
//
// Parameters:
//   - endpoint: MinIO server endpoint (e.g., "localhost:9000")
//   - accessKeyID: Access key ID for authentication
//   - secretAccessKey: Secret access key for authentication
//   - bucketName: Name of the bucket to use
//   - region: Region for the bucket (required for some providers)
//   - useSSL: Whether to use SSL/TLS for connections
//   - pathPrefix: Path prefix for all object keys
//
// Returns:
//   - *MinIOStorage: A new MinIO storage instance
//   - error: Error if initialization fails
func NewMinIOStorage(endpoint, accessKeyID, secretAccessKey, bucketName, region string, useSSL bool, pathPrefix string) (*MinIOStorage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
		Region: region,
	})
	if err != nil {
		return nil, errors.Wrap(err, "create minio client")
	}

	storage := &MinIOStorage{
		client:     client,
		bucketName: bucketName,
		pathPrefix: pathPrefix,
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucketName)
	if err != nil {
		return nil, errors.Wrapf(err, "check bucket existence: %s", bucketName)
	}

	if !exists {
		err = client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{Region: region})
		if err != nil {
			return nil, errors.Wrapf(err, "create bucket: %s", bucketName)
		}
	}

	return storage, nil
}

// buildKey builds the full storage key by prepending the path prefix.
func (m *MinIOStorage) buildKey(key string) string {
	if m.pathPrefix == "" {
		return key
	}
	if m.pathPrefix[len(m.pathPrefix)-1] == '/' {
		return m.pathPrefix + key
	}
	return m.pathPrefix + "/" + key
}

// PutObject uploads an object to MinIO storage.
func (m *MinIOStorage) PutObject(ctx context.Context, key string, data []byte) error {
	key = m.buildKey(key)
	_, err := m.client.PutObject(ctx, m.bucketName, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{})
	return errors.Wrapf(err, "put object: %s", key)
}

// PutObjectFromReader uploads an object from a reader to MinIO storage.
func (m *MinIOStorage) PutObjectFromReader(ctx context.Context, key string, reader io.Reader, size int64) error {
	key = m.buildKey(key)
	_, err := m.client.PutObject(ctx, m.bucketName, key, reader, size, minio.PutObjectOptions{})
	return errors.Wrapf(err, "put object from reader: %s", key)
}

// GetObject retrieves an object from MinIO storage.
func (m *MinIOStorage) GetObject(ctx context.Context, key string) ([]byte, error) {
	key = m.buildKey(key)
	obj, err := m.client.GetObject(ctx, m.bucketName, key, minio.GetObjectOptions{})
	if err != nil {
		// Check if object doesn't exist
		if minioErr, ok := err.(minio.ErrorResponse); ok && minioErr.Code == "NoSuchKey" {
			return nil, ErrObjectNotFound
		}
		return nil, errors.Wrapf(err, "get object: %s", key)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, errors.Wrapf(err, "read object: %s", key)
	}
	return data, nil
}

// GetObjectReader returns a reader for an object in MinIO storage.
func (m *MinIOStorage) GetObjectReader(ctx context.Context, key string) (io.ReadCloser, error) {
	key = m.buildKey(key)
	obj, err := m.client.GetObject(ctx, m.bucketName, key, minio.GetObjectOptions{})
	if err != nil {
		// Check if object doesn't exist
		if minioErr, ok := err.(minio.ErrorResponse); ok && minioErr.Code == "NoSuchKey" {
			return nil, ErrObjectNotFound
		}
		return nil, errors.Wrapf(err, "get object reader: %s", key)
	}
	return obj, nil
}

// DeleteObject deletes an object from MinIO storage.
func (m *MinIOStorage) DeleteObject(ctx context.Context, key string) error {
	key = m.buildKey(key)
	err := m.client.RemoveObject(ctx, m.bucketName, key, minio.RemoveObjectOptions{})
	return errors.Wrapf(err, "delete object: %s", key)
}

// Exists checks if an object exists in MinIO storage.
func (m *MinIOStorage) Exists(ctx context.Context, key string) (bool, error) {
	key = m.buildKey(key)
	_, err := m.client.StatObject(ctx, m.bucketName, key, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, errors.Wrapf(err, "stat object: %s", key)
	}
	return true, nil
}

// GetObjectURL returns a presigned URL for accessing an object in MinIO storage.
// If expiresIn is 0, defaults to 7 days expiration.
func (m *MinIOStorage) GetObjectURL(ctx context.Context, key string, expiresIn int64) (string, error) {
	key = m.buildKey(key)
	expires := time.Duration(expiresIn) * time.Second
	if expiresIn == 0 {
		expires = 7 * 24 * time.Hour // Default 7 days
	}
	url, err := m.client.PresignedGetObject(ctx, m.bucketName, key, expires, nil)
	if err != nil {
		return "", errors.Wrapf(err, "get presigned url: %s", key)
	}
	return url.String(), nil
}
