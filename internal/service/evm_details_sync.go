// Package service provides EVM transaction details synchronization service.
package service

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/chain"
	"injective-explorer/provider/db"
	"injective-explorer/provider/storage"

	"github.com/bytedance/sonic"
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
//   - bc: Bootstrap configuration (optional, can be nil)
//
// Returns:
//   - *EVMDetailsSyncService: A new EVM details sync service instance
//   - error: Error if the service cannot be created
func NewEVMDetailsSyncService(ctx context.Context, config *EVMDetailsSyncConfig, bc *conf.Bootstrap) (*EVMDetailsSyncService, error) {
	serviceCtx, cancel := context.WithCancel(ctx)

	// Create EVM client
	var evmClient *chain.EVMClient
	var err error

	if bc != nil && bc.Chain != nil && bc.Chain.Evm != nil && bc.Chain.Evm.Enabled {
		// Use custom EVM configuration from config file
		evmClient, err = createEVMClientFromConfig(bc.Chain.Evm)
		if err != nil {
			cancel()
			return nil, errors.Wrap(err, "failed to create EVM client from config")
		}
	} else {
		// Use default EVM client with network name from global config
		networkName := global.NetworkName
		if networkName == "" {
			networkName = "mainnet" // Default to mainnet
		}
		evmClient, err = chain.NewEVMClient(networkName)
		if err != nil {
			cancel()
			return nil, errors.Wrap(err, "failed to create EVM client")
		}
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

	// Check if EVM transaction exists in evm_transactions table
	var evmTx models.EVMTransaction
	err := db.Get().WithContext(ctx).
		Where("transaction_hash = ?", cosmosHash).
		First(&evmTx).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			global.Logger.Warnf("worker %d: EVM transaction %s not found in evm_transactions table, skipping", workerID, cosmosHash)
			return
		}
		global.Logger.Errorf("worker %d: failed to query EVM transaction %s: %v", workerID, cosmosHash, err)
		// Retry task
		s.retryTask(ctx, taskStr)
		return
	}

	// Skip if already synced
	if evmTx.EVMDetailsSynced {
		return
	}

	// Fetch EVM transaction details
	details, err := s.fetchEVMDetails(ctx, evmHash)
	if err != nil {
		// Categorize and enhance error message
		errorType := categorizeError(err)
		enhancedErrMsg := fmt.Sprintf("[%s] %v", errorType, err)

		global.Logger.Warnf("worker %d: failed to fetch EVM details for %s (evm: %s): %s",
			workerID, cosmosHash, evmHash, enhancedErrMsg)

		// Update error information
		s.updateSyncError(ctx, cosmosHash, enhancedErrMsg)

		// Retry if should retry
		if s.shouldRetryEVM(&evmTx, err) {
			s.retryTask(ctx, taskStr)
		}
		return
	}

	// Save transaction details to evm_transactions table
	if err := s.saveEVMDetails(ctx, &evmTx, details); err != nil {
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

// saveEVMDetails saves EVM transaction details to evm_transactions table or object storage.
// It updates EVM-related fields (evm_from_address, evm_to_address) in the evm_transactions table.
// This avoids affecting the transactions table and prevents clearing signatures/signer fields.
//
// Parameters:
//   - ctx: Context for cancellation
//   - evmTx: EVMTransaction model from evm_transactions table
//   - details: EVM transaction details
//
// Returns:
//   - error: Error if the operation fails
func (s *EVMDetailsSyncService) saveEVMDetails(ctx context.Context, evmTx *models.EVMTransaction, details *chain.TransactionDetails) error {
	// Marshal to JSON
	detailsJSON, err := sonic.Marshal(details)
	if err != nil {
		return errors.Wrap(err, "failed to marshal transaction details")
	}

	// Check if should store to object storage (if data is large)
	storageKey := ""

	if storage.IsEnabled() && len(detailsJSON) > 3*1024 { // 3KB threshold
		storageClient := storage.Get()
		storageKey = storage.BuildStorageKey("evm_details", evmTx.TransactionHash)

		if err := storageClient.PutObject(ctx, storageKey, detailsJSON); err != nil {
			global.Logger.Warnf("failed to upload EVM details to storage for %s: %v", evmTx.TransactionHash, err)
			// Continue with database storage
			storageKey = ""
		}
	}

	// Update evm_transactions table with EVM details and addresses
	// This table is separate from transactions table, so it won't affect signatures/signer fields
	now := time.Now()
	updates := map[string]interface{}{
		"evm_details_synced":     true,
		"evm_details_sync_at":    &now,
		"evm_details_sync_error": "",
		"evm_from_address":       details.From,
		"evm_to_address":         details.To,
	}

	if storageKey != "" {
		updates["evm_details_storage_key"] = storageKey
	}

	// Update evm_transactions table (separate from transactions table)
	err = db.Get().WithContext(ctx).
		Model(evmTx).
		Where("transaction_hash = ?", evmTx.TransactionHash).
		Updates(updates).Error

	if err != nil {
		return errors.Wrapf(err, "failed to update EVM transaction %s", evmTx.TransactionHash)
	}

	return nil
}

// updateSyncError updates the sync error information for an EVM transaction.
//
// Parameters:
//   - ctx: Context for cancellation
//   - cosmosHash: Cosmos transaction hash
//   - errorMsg: Error message
func (s *EVMDetailsSyncService) updateSyncError(ctx context.Context, cosmosHash string, errorMsg string) {
	db.Get().WithContext(ctx).
		Model(&models.EVMTransaction{}).
		Where("transaction_hash = ?", cosmosHash).
		Updates(map[string]interface{}{
			"evm_details_sync_error": errorMsg,
		})
}

// shouldRetryEVM checks if a task should be retried based on the error type.
//
// Parameters:
//   - evmTx: EVMTransaction model
//   - err: Error that occurred (for error type checking)
//
// Returns:
//   - bool: true if should retry, false otherwise
func (s *EVMDetailsSyncService) shouldRetryEVM(evmTx *models.EVMTransaction, err error) bool {
	if evmTx.EVMDetailsSynced {
		return false
	}

	// Don't retry for these error types
	errorType := categorizeError(err)
	nonRetryableErrors := []string{
		"NOT_FOUND",
		"PENDING",
		"CIRCUIT_BREAKER_OPEN",
	}

	for _, nonRetryable := range nonRetryableErrors {
		if errorType == nonRetryable {
			return false
		}
	}

	// Retry for network errors, context canceled, JSON parse errors, etc.
	return true
}

// retryTask retries a task by pushing it back to the queue with exponential backoff.
//
// Parameters:
//   - ctx: Context for cancellation
//   - taskStr: Task string
func (s *EVMDetailsSyncService) retryTask(ctx context.Context, taskStr string) {
	// Get current retry count from database if available
	// For now, we'll use a simple exponential backoff strategy
	maxRetries := s.config.MaxRetryAttempts
	baseDelay := s.config.RetryDelay

	go func() {
		// Simple exponential backoff: 5s, 10s, 20s, 40s
		for attempt := 0; attempt < maxRetries; attempt++ {
			delay := time.Duration(1<<uint(attempt)) * baseDelay
			time.Sleep(delay)

			// Try to push the task back to the queue
			err := s.redisClient.LPush(ctx, s.config.QueueKey, taskStr).Err()
			if err == nil {
				global.Logger.Infof("Task %s successfully queued for retry (attempt %d)", taskStr, attempt+1)
				return
			}

			global.Logger.Warnf("Failed to retry task %s (attempt %d/%d): %v", taskStr, attempt+1, maxRetries, err)
		}

		// All retries exhausted
		global.Logger.Errorf("Failed to retry task %s after %d attempts, giving up", taskStr, maxRetries)
	}()
}

// categorizeError categorizes errors into types for better debugging
func categorizeError(err error) string {
	if err == nil {
		return "UNKNOWN"
	}

	errStr := strings.ToLower(err.Error())

	// JSON parse errors
	if strings.Contains(errStr, "parse error") || strings.Contains(errStr, "invalid json") {
		return "JSON_PARSE_ERROR"
	}

	// Context canceled errors
	if strings.Contains(errStr, "context canceled") || strings.Contains(errStr, "context deadline exceeded") {
		return "CONTEXT_CANCELED"
	}

	// Network/RPC errors
	if strings.Contains(errStr, "connection refused") ||
	   strings.Contains(errStr, "network error") ||
	   strings.Contains(errStr, "temporary failure") ||
	   strings.Contains(errStr, "timeout") {
		return "NETWORK_ERROR"
	}

	// Circuit breaker errors
	if strings.Contains(errStr, "circuit breaker is open") {
		return "CIRCUIT_BREAKER_OPEN"
	}

	// Transaction not found
	if strings.Contains(errStr, "not found") {
		return "NOT_FOUND"
	}

	// Pending transaction
	if strings.Contains(errStr, "pending") {
		return "PENDING"
	}

	// Unknown error
	return "UNKNOWN_ERROR"
}

// createEVMClientFromConfig creates an EVM client from configuration
func createEVMClientFromConfig(evmConfig *conf.Chain_EVM) (*chain.EVMClient, error) {
	if evmConfig == nil {
		return nil, errors.New("EVM config cannot be nil")
	}

	if evmConfig.RpcEndpoint == "" {
		return nil, errors.New("EVM RPC endpoint cannot be empty")
	}

	chainID := big.NewInt(evmConfig.ChainId)

	return chain.NewEVMClientWithConfig(&chain.EVMConfig{
		RPCURL:  evmConfig.RpcEndpoint,
		ChainID: chainID,
		Network: global.NetworkName,
		Timeout: evmConfig.Timeout.AsDuration(),
	})
}
