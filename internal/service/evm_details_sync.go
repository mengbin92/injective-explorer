// Package service provides EVM transaction details synchronization service.
package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/chain"
	"injective-explorer/provider/db"
	"injective-explorer/provider/storage"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// EVMDetailsSyncService handles EVM transaction details synchronization.
type EVMDetailsSyncService struct {
	ctx         context.Context
	cancel      context.CancelFunc
	config      *EVMDetailsSyncConfig
	evmClient   *chain.EVMClient
	redisClient *redis.Client
	wg          sync.WaitGroup
}

// EVMDetailsSyncConfig holds the EVM details sync service configuration.
type EVMDetailsSyncConfig struct {
	QueueKey         string
	WorkerCount      int
	BatchSize        int
	RetryDelay       time.Duration
	MaxRetryAttempts int
	ProcessInterval  time.Duration
	Timeout          time.Duration
}

// NewEVMDetailsSyncService creates a new instance of EVMDetailsSyncService.
//
// Parameters:
//   - ctx: Context for cancellation
//   - config: EVM details sync configuration
//
// Returns:
//   - *EVMDetailsSyncService: A new EVM details sync service instance
//   - error: Error if the service cannot be created
func NewEVMDetailsSyncService(ctx context.Context, config *EVMDetailsSyncConfig) (*EVMDetailsSyncService, error) {
	serviceCtx, cancel := context.WithCancel(ctx)

	// Create EVM client
	networkName := global.NetworkName
	if networkName == "" {
		networkName = "mainnet" // Default to mainnet
	}
	evmClient, err := chain.NewEVMClient(networkName)
	if err != nil {
		cancel()
		return nil, errors.Wrap(err, "failed to create EVM client")
	}

	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		cancel()
		return nil, errors.New("redis client is not available")
	}

	return &EVMDetailsSyncService{
		ctx:         serviceCtx,
		cancel:      cancel,
		config:      config,
		evmClient:   evmClient,
		redisClient: redisClient,
	}, nil
}

// Start starts the EVM details sync service.
// It starts worker goroutines to process EVM transaction details tasks.
func (s *EVMDetailsSyncService) Start() {
	for i := 0; i < s.config.WorkerCount; i++ {
		s.wg.Add(1)
		go s.worker(i)
	}
	global.Logger.Infof("EVM details sync service started with %d workers", s.config.WorkerCount)
}

// Stop stops the EVM details sync service.
// It cancels the context and waits for all goroutines to finish.
func (s *EVMDetailsSyncService) Stop() {
	s.cancel()
	if s.evmClient != nil {
		s.evmClient.Close()
	}
	s.wg.Wait()
	global.Logger.Info("EVM details sync service stopped")
}

// worker is a worker goroutine that processes EVM transaction details tasks.
//
// Parameters:
//   - id: Worker ID for logging
func (s *EVMDetailsSyncService) worker(id int) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.ProcessInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.processBatch(id)
		}
	}
}

// processBatch processes a batch of EVM transaction details tasks.
//
// Parameters:
//   - workerID: Worker ID for logging
func (s *EVMDetailsSyncService) processBatch(workerID int) {
	ctx, cancel := context.WithTimeout(s.ctx, s.config.Timeout)
	defer cancel()

	// Batch pop tasks from queue
	tasks := make([]string, 0, s.config.BatchSize)
	for i := 0; i < s.config.BatchSize; i++ {
		result, err := s.redisClient.BRPop(ctx, time.Second, s.config.QueueKey).Result()
		if err != nil {
			if err != redis.Nil {
				global.Logger.Warnf("worker %d: failed to pop from queue: %v", workerID, err)
			}
			break
		}
		tasks = append(tasks, result[1])
	}

	if len(tasks) == 0 {
		return
	}

	global.Logger.Debugf("worker %d: processing %d EVM details tasks", workerID, len(tasks))

	// Process tasks concurrently
	var wg sync.WaitGroup
	for _, task := range tasks {
		wg.Add(1)
		go func(taskStr string) {
			defer wg.Done()
			s.processTask(ctx, taskStr, workerID)
		}(task)
	}
	wg.Wait()
}

// processTask processes a single EVM transaction details task.
//
// Parameters:
//   - ctx: Context for cancellation
//   - taskStr: Task string in format "cosmosHash:evmHash"
//   - workerID: Worker ID for logging
func (s *EVMDetailsSyncService) processTask(ctx context.Context, taskStr string, workerID int) {
	// Parse task: format is "cosmosHash:evmHash"
	parts := strings.Split(taskStr, ":")
	if len(parts) != 2 {
		global.Logger.Warnf("worker %d: invalid task format: %s", workerID, taskStr)
		return
	}

	cosmosHash := parts[0]
	evmHash := parts[1]

	// Check if transaction exists
	var tx models.Transaction
	err := db.Get().WithContext(ctx).
		Where("hash = ?", cosmosHash).
		First(&tx).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			global.Logger.Warnf("worker %d: transaction %s not found, skipping", workerID, cosmosHash)
			return
		}
		global.Logger.Errorf("worker %d: failed to query transaction %s: %v", workerID, cosmosHash, err)
		// Retry task
		s.retryTask(ctx, taskStr)
		return
	}

	// Skip if already synced
	if tx.EVMDetailsSynced {
		return
	}

	// Fetch EVM transaction details
	details, err := s.fetchEVMDetails(ctx, evmHash)
	if err != nil {
		global.Logger.Warnf("worker %d: failed to fetch EVM details for %s (evm: %s): %v",
			workerID, cosmosHash, evmHash, err)

		// Update error information
		s.updateSyncError(ctx, cosmosHash, err.Error())

		// Retry if should retry
		if s.shouldRetry(&tx) {
			s.retryTask(ctx, taskStr)
		}
		return
	}

	// Save transaction details
	if err := s.saveEVMDetails(ctx, &tx, details); err != nil {
		global.Logger.Errorf("worker %d: failed to save EVM details for %s: %v", workerID, cosmosHash, err)
		s.retryTask(ctx, taskStr)
		return
	}

	global.Logger.Debugf("worker %d: successfully synced EVM details for %s", workerID, cosmosHash)
}

// fetchEVMDetails fetches EVM transaction details from EVM RPC.
//
// Parameters:
//   - ctx: Context for cancellation
//   - evmHash: EVM transaction hash
//
// Returns:
//   - *chain.TransactionDetails: Transaction details
//   - error: Error if the operation fails
func (s *EVMDetailsSyncService) fetchEVMDetails(ctx context.Context, evmHash string) (*chain.TransactionDetails, error) {
	hash := ethcommon.HexToHash(evmHash)
	details, err := s.evmClient.GetTransactionDetails(ctx, hash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get transaction details for %s", evmHash)
	}
	return details, nil
}

// saveEVMDetails saves EVM transaction details to database or object storage.
// It also updates the EVM from/to addresses for optimized account queries.
//
// Parameters:
//   - ctx: Context for cancellation
//   - tx: Transaction model
//   - details: EVM transaction details
//
// Returns:
//   - error: Error if the operation fails
func (s *EVMDetailsSyncService) saveEVMDetails(ctx context.Context, tx *models.Transaction, details *chain.TransactionDetails) error {
	// Marshal to JSON
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return errors.Wrap(err, "failed to marshal transaction details")
	}

	// Check if should store to object storage (if data is large)
	storageKey := ""

	if storage.IsEnabled() && len(detailsJSON) > 3*1024 { // 3KB threshold
		storageClient := storage.Get()
		storageKey = storage.BuildStorageKey("evm_details", tx.Hash)

		if err := storageClient.PutObject(ctx, storageKey, detailsJSON); err != nil {
			global.Logger.Warnf("failed to upload EVM details to storage for %s: %v", tx.Hash, err)
			// Continue with database storage
			storageKey = ""
		}
	}

	// Update database with EVM details and addresses for optimized account queries
	now := time.Now()
	updates := map[string]interface{}{
		"evm_details_synced":     true,
		"evm_details_sync_at":     &now,
		"evm_details_sync_error":  "",
		"evm_from_address":        details.From,
		"evm_to_address":          details.To,
	}

	if storageKey != "" {
		updates["evm_details_storage_key"] = storageKey
	}

	err = db.Get().WithContext(ctx).
		Model(tx).
		Updates(updates).Error

	if err != nil {
		return errors.Wrapf(err, "failed to update transaction %s", tx.Hash)
	}

	return nil
}

// updateSyncError updates the sync error information for a transaction.
//
// Parameters:
//   - ctx: Context for cancellation
//   - cosmosHash: Cosmos transaction hash
//   - errorMsg: Error message
func (s *EVMDetailsSyncService) updateSyncError(ctx context.Context, cosmosHash string, errorMsg string) {
	db.Get().WithContext(ctx).
		Model(&models.Transaction{}).
		Where("hash = ?", cosmosHash).
		Updates(map[string]interface{}{
			"evm_details_sync_error": errorMsg,
		})
}

// shouldRetry checks if a task should be retried.
//
// Parameters:
//   - tx: Transaction model
//
// Returns:
//   - bool: true if should retry, false otherwise
func (s *EVMDetailsSyncService) shouldRetry(tx *models.Transaction) bool {
	// Simple retry logic: always retry if not synced
	// In production, you might want to check retry count or error type
	return !tx.EVMDetailsSynced
}

// retryTask retries a task by pushing it back to the queue.
//
// Parameters:
//   - ctx: Context for cancellation
//   - taskStr: Task string
func (s *EVMDetailsSyncService) retryTask(ctx context.Context, taskStr string) {
	// Retry after delay
	go func() {
		time.Sleep(s.config.RetryDelay)
		if err := s.redisClient.LPush(ctx, s.config.QueueKey, taskStr).Err(); err != nil {
			global.Logger.Warnf("failed to retry task %s: %v", taskStr, err)
		}
	}()
}

