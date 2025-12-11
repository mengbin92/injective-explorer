// Package storage provides storage initialization functions.
package storage

import (
	"context"

	"injective-explorer/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/pkg/errors"
)

var (
	// gStorage is the global storage instance
	gStorage Storage
)

// Init initializes the storage based on configuration.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - cfg: Object storage configuration
//   - logger: Logger instance for logging
//
// Returns:
//   - error: Error if initialization fails
//
// If cfg is nil or cfg.Enabled is false, a NoOpStorage will be used.
func Init(ctx context.Context, cfg *conf.Data_ObjectStorage, logger log.Logger) error {
	if cfg == nil || !cfg.Enabled {
		// Use no-op storage if disabled
		gStorage = &NoOpStorage{}
		log.NewHelper(logger).Infof("object storage disabled, using no-op storage")
		return nil
	}

	var err error
	switch cfg.Provider {
	case "minio":
		gStorage, err = NewMinIOStorage(
			cfg.Endpoint,
			cfg.AccessKeyId,
			cfg.SecretAccessKey,
			cfg.BucketName,
			cfg.Region,
			cfg.UseSsl,
			cfg.PathPrefix,
		)
	case "s3":
		gStorage, err = NewS3Storage(
			cfg.Endpoint,
			cfg.AccessKeyId,
			cfg.SecretAccessKey,
			cfg.BucketName,
			cfg.Region,
			cfg.UseSsl,
			cfg.PathPrefix,
		)
	case "oss":
		// TODO: Implement OSS storage
		return errors.New("OSS storage not implemented yet")
	case "cos":
		// TODO: Implement COS storage
		return errors.New("COS storage not implemented yet")
	default:
		return errors.Errorf("unsupported storage provider: %s", cfg.Provider)
	}

	if err != nil {
		return errors.Wrap(err, "initialize storage")
	}

	log.NewHelper(logger).Infof("object storage initialized: provider=%s, bucket=%s", cfg.Provider, cfg.BucketName)
	return nil
}

// Get returns the global storage instance.
// Returns NoOpStorage if storage has not been initialized.
func Get() Storage {
	if gStorage == nil {
		return &NoOpStorage{}
	}
	return gStorage
}

// IsEnabled checks if object storage is actually enabled (not NoOpStorage).
// This is useful to determine if storage operations will actually persist data.
func IsEnabled() bool {
	if gStorage == nil {
		return false
	}
	// Check the type: if it's NoOpStorage, storage is disabled
	_, isNoOp := gStorage.(*NoOpStorage)
	return !isNoOp
}
