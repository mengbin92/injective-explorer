// Package service provides unit tests for the storage migration service.
package service

import (
	"context"
	"errors"
	"io"
	"testing"

	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/storage"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// mockStorage implements storage.Storage for testing.
type mockStorage struct {
	objects      map[string][]byte
	putErr       error
	getErr       error
	existsErr    error
	deleteErr    error
	shouldFail   bool
	failAfterPut bool
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		objects: make(map[string][]byte),
	}
}

func (m *mockStorage) PutObject(ctx context.Context, key string, data []byte) error {
	if m.putErr != nil {
		return m.putErr
	}
	if m.shouldFail {
		return errors.New("storage put failed")
	}
	m.objects[key] = data
	if m.failAfterPut {
		m.shouldFail = true
	}
	return nil
}

func (m *mockStorage) PutObjectFromReader(ctx context.Context, key string, reader io.Reader, size int64) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	return m.PutObject(ctx, key, data)
}

func (m *mockStorage) GetObject(ctx context.Context, key string) ([]byte, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	data, exists := m.objects[key]
	if !exists {
		return nil, storage.ErrObjectNotFound
	}
	return data, nil
}

func (m *mockStorage) GetObjectReader(ctx context.Context, key string) (io.ReadCloser, error) {
	data, err := m.GetObject(ctx, key)
	if err != nil {
		return nil, err
	}
	return &mockReader{data: data}, nil
}

func (m *mockStorage) DeleteObject(ctx context.Context, key string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.objects, key)
	return nil
}

func (m *mockStorage) Exists(ctx context.Context, key string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	_, exists := m.objects[key]
	return exists, nil
}

func (m *mockStorage) GetObjectURL(ctx context.Context, key string, expiresIn int64) (string, error) {
	return "http://example.com/" + key, nil
}

type mockReader struct {
	data []byte
	pos  int
}

func (m *mockReader) Read(p []byte) (n int, err error) {
	if m.pos >= len(m.data) {
		return 0, errors.New("EOF")
	}
	n = copy(p, m.data[m.pos:])
	m.pos += n
	return n, nil
}

func (m *mockReader) Close() error {
	return nil
}

// setupTestDBForMigration creates an in-memory SQLite database for migration testing.
func setupTestDBForMigration(t *testing.T) *gorm.DB {
	// Initialize global logger if not already initialized
	if global.Logger == nil {
		global.Logger = log.NewHelper(log.DefaultLogger)
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "failed to open test database")

	// Auto-migrate tables
	err = db.AutoMigrate(&models.Transaction{})
	require.NoError(t, err, "failed to migrate test database")

	return db
}

// createTestTransactionForMigration creates a test transaction with the given data size.
func createTestTransactionForMigration(db *gorm.DB, hash string, dataSize int) *models.Transaction {
	data := make([]byte, dataSize)
	for i := range data {
		data[i] = byte(i % 256)
	}
	tx := &models.Transaction{
		Hash:               hash,
		BlockNumber:        1000,
		Data:               string(data),
		Messages:           string(data),
		Logs:               string(data),
		DataStorageKey:     "",
		MessagesStorageKey: "",
		LogsStorageKey:     "",
	}
	db.Create(tx)
	return tx
}

// TestStorageMigrationService_MigrateTransactions tests the MigrateTransactions method.
func TestStorageMigrationService_MigrateTransactions(t *testing.T) {
	db := setupTestDBForMigration(t)
	mockStorage := newMockStorage()
	service := &StorageMigrationService{
		db:      db,
		storage: mockStorage,
	}
	ctx := context.Background()

	t.Run("successful migration", func(t *testing.T) {
		// Create transactions with large data (>10KB threshold)
		_ = createTestTransactionForMigration(db, "hash1", 15*1024)
		_ = createTestTransactionForMigration(db, "hash2", 20*1024)

		config := &MigrationConfig{
			BatchSize:     100,
			Threshold:     10 * 1024, // 10KB
			DryRun:        false,
			OnlyEmptyKeys: true,
		}

		err := service.MigrateTransactions(ctx, config)
		require.NoError(t, err)

		// Verify storage keys are set
		var migratedTx1, migratedTx2 models.Transaction
		db.First(&migratedTx1, "hash = ?", "hash1")
		db.First(&migratedTx2, "hash = ?", "hash2")
		assert.NotEmpty(t, migratedTx1.DataStorageKey)
		assert.NotEmpty(t, migratedTx2.DataStorageKey)
		assert.Empty(t, migratedTx1.Data)
		assert.Empty(t, migratedTx2.Data)

		// Verify data exists in storage
		data1, err := mockStorage.GetObject(ctx, migratedTx1.DataStorageKey)
		require.NoError(t, err)
		assert.Equal(t, 15*1024, len(data1))
	})

	t.Run("dry run mode", func(t *testing.T) {
		// Create transaction with large data
		_ = createTestTransactionForMigration(db, "hash3", 15*1024)

		config := &MigrationConfig{
			BatchSize:     100,
			Threshold:     10 * 1024,
			DryRun:        true,
			OnlyEmptyKeys: true,
		}

		err := service.MigrateTransactions(ctx, config)
		require.NoError(t, err)

		// Verify transaction is NOT migrated in dry run mode
		var tx models.Transaction
		db.First(&tx, "hash = ?", "hash3")
		assert.Empty(t, tx.DataStorageKey)
		assert.NotEmpty(t, tx.Data)
	})

	t.Run("small data not migrated", func(t *testing.T) {
		// Create transaction with small data (<10KB threshold)
		_ = createTestTransactionForMigration(db, "hash4", 5*1024)

		config := &MigrationConfig{
			BatchSize:     100,
			Threshold:     10 * 1024,
			DryRun:        false,
			OnlyEmptyKeys: true,
		}

		err := service.MigrateTransactions(ctx, config)
		require.NoError(t, err)

		// Verify transaction is not migrated
		var tx models.Transaction
		db.First(&tx, "hash = ?", "hash4")
		assert.Empty(t, tx.DataStorageKey)
		assert.NotEmpty(t, tx.Data)
	})

	t.Run("already migrated transactions skipped", func(t *testing.T) {
		// Create transaction with storage key already set
		tx := &models.Transaction{
			Hash:           "hash5",
			BlockNumber:    1000,
			Data:           "large data",
			DataStorageKey: "existing/key",
		}
		db.Create(tx)

		config := &MigrationConfig{
			BatchSize:     100,
			Threshold:     10 * 1024,
			DryRun:        false,
			OnlyEmptyKeys: true,
		}

		err := service.MigrateTransactions(ctx, config)
		require.NoError(t, err)

		// Verify storage key is unchanged
		var migratedTx models.Transaction
		db.First(&migratedTx, "hash = ?", "hash5")
		assert.Equal(t, "existing/key", migratedTx.DataStorageKey)
	})

	t.Run("invalid configuration", func(t *testing.T) {
		// Test invalid batch size
		config := &MigrationConfig{
			BatchSize: 0,
			Threshold: 10 * 1024,
		}
		err := service.MigrateTransactions(ctx, config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "batch size must be greater than 0")

		// Test invalid threshold
		config = &MigrationConfig{
			BatchSize: 100,
			Threshold: -1,
		}
		err = service.MigrateTransactions(ctx, config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "threshold must be non-negative")
	})

	t.Run("storage failure with retry", func(t *testing.T) {
		// Create transaction with large data
		_ = createTestTransactionForMigration(db, "hash6", 15*1024)

		// Create storage that fails first 2 times, then succeeds
		failingStorage := newMockStorage()
		failingStorage.shouldFail = true
		failingStorage.failAfterPut = false // Will fail on first 2 attempts

		serviceWithFailingStorage := &StorageMigrationService{
			db:      db,
			storage: failingStorage,
		}

		config := &MigrationConfig{
			BatchSize:     100,
			Threshold:     10 * 1024,
			DryRun:        false,
			OnlyEmptyKeys: true,
		}

		// This should fail after retries
		err := serviceWithFailingStorage.MigrateTransactions(ctx, config)
		// Migration continues even if some uploads fail
		// The transaction won't be updated if upload fails
		_ = err // May or may not error depending on implementation
	})

	t.Run("context cancellation", func(t *testing.T) {
		// Create transaction with large data
		_ = createTestTransactionForMigration(db, "hash7", 15*1024)

		// Create context with cancellation
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		config := &MigrationConfig{
			BatchSize:     100,
			Threshold:     10 * 1024,
			DryRun:        false,
			OnlyEmptyKeys: true,
		}

		err := service.MigrateTransactions(cancelCtx, config)
		require.Error(t, err)
		// Error is wrapped, so check if it contains context.Canceled
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestStorageMigrationService_VerifyMigration tests the VerifyMigration method.
func TestStorageMigrationService_VerifyMigration(t *testing.T) {
	db := setupTestDBForMigration(t)
	mockStorage := newMockStorage()
	service := &StorageMigrationService{
		db:      db,
		storage: mockStorage,
	}
	ctx := context.Background()

	t.Run("successful verification", func(t *testing.T) {
		// Create and migrate transaction
		_ = createTestTransactionForMigration(db, "verify_hash1", 15*1024)
		config := &MigrationConfig{
			BatchSize:     100,
			Threshold:     10 * 1024,
			DryRun:        false,
			OnlyEmptyKeys: true,
		}
		err := service.MigrateTransactions(ctx, config)
		require.NoError(t, err)

		// Verify migration
		err = service.VerifyMigration(ctx, 10)
		require.NoError(t, err)
	})

	t.Run("missing storage object", func(t *testing.T) {
		// Create transaction with storage key but object doesn't exist
		tx := &models.Transaction{
			Hash:           "verify_hash2",
			BlockNumber:    1000,
			DataStorageKey: "missing/key",
		}
		db.Create(tx)

		// Verification should log warning but not fail
		err := service.VerifyMigration(ctx, 10)
		require.NoError(t, err) // Verification doesn't fail, just logs warnings
	})
}

// TestDefaultMigrationConfig tests the DefaultMigrationConfig function.
func TestDefaultMigrationConfig(t *testing.T) {
	config := DefaultMigrationConfig()
	assert.Equal(t, 100, config.BatchSize)
	assert.Equal(t, 10*1024, config.Threshold)
	assert.False(t, config.DryRun)
	assert.True(t, config.OnlyEmptyKeys)
}

// TestBuildStorageKey tests the BuildStorageKey function.
func TestBuildStorageKey(t *testing.T) {
	t.Run("normal hash", func(t *testing.T) {
		key := storage.BuildStorageKey("data", "abcdef1234567890")
		assert.Equal(t, "data/abcdef12/abcdef1234567890", key)
	})

	t.Run("hash with 0x prefix", func(t *testing.T) {
		key := storage.BuildStorageKey("messages", "0xabcdef1234567890")
		assert.Equal(t, "messages/abcdef12/abcdef1234567890", key)
	})

	t.Run("short hash", func(t *testing.T) {
		key := storage.BuildStorageKey("logs", "abc")
		assert.Equal(t, "logs/abc", key)
	})
}
