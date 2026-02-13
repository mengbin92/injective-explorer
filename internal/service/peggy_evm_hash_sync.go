package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"injective-explorer/internal/converter"
	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// PeggyEvmHashSyncService asynchronously syncs EVM transaction hashes
// for Peggy cross-chain transactions.
type PeggyEvmHashSyncService struct {
	config  *Config
	client  PeggyEvmHashClient
	redis   *redis.Client
	db      *gorm.DB
	metrics *PeggyEvmSyncMetrics

	// Control channels
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// PeggyEvmHashClient defines the interface for querying EVM hashes.
type PeggyEvmHashClient interface {
	GetDepositTxHash(
		ctx context.Context,
		eventNonce uint64,
		eventHeight uint64,
		sender string,
		denom string,
		amount string,
	) (string, error)

	GetWithdrawalBatchTxHash(
		ctx context.Context,
		batchNonce uint64,
		denom string,
	) (string, error)
}

// Config defines the configuration for PeggyEvmHashSyncService.
type Config struct {
	QueueKey           string        // Redis queue key
	WorkerCount        int           // Number of concurrent workers
	BatchUpdateSize    int           // Batch size for DB updates
	MinConfirmBlocks   int           // Minimum confirmation blocks before querying
	RetryMaxAttempts   int           // Maximum retry attempts
	RetryInitialDelay  time.Duration // Initial retry delay
	TaskPollInterval   time.Duration // Interval for polling tasks when queue is empty
}

// DefaultPeggyEvmHashSyncConfig returns default configuration.
func DefaultPeggyEvmHashSyncConfig() *Config {
	return &Config{
		QueueKey:           "peggy:evm_hash:queue",
		WorkerCount:        2,
		BatchUpdateSize:    100,
		MinConfirmBlocks:   12,
		RetryMaxAttempts:   3,
		RetryInitialDelay:  time.Second,
		TaskPollInterval:   time.Second,
	}
}

// LoadPeggyEvmHashSyncConfig loads configuration from bootstrap config.
func LoadPeggyEvmHashSyncConfig(bc *conf.Bootstrap) *Config {
	config := DefaultPeggyEvmHashSyncConfig()

	if bc == nil || bc.Chain == nil {
		return config
	}

	// Load from chain config if available
	// Note: Most config values use defaults for now
	// Future: can add per-chain config if needed

	return config
}

// SyncTask represents a sync task in the queue.
type SyncTask struct {
	Type         string `json:"type"`         // "deposit" or "withdrawal"
	BlockHeight  uint64 `json:"block_height"`  // Ethereum block height
	EventNonce   uint64 `json:"event_nonce"`   // For deposit
	BatchNonce   uint64 `json:"batch_nonce"`   // For withdrawal
	TxHash       string `json:"tx_hash"`       // Cosmos tx hash
	Denom        string `json:"denom"`         // Token denom
	Amount       string `json:"amount"`        // For deposit
	Sender       string `json:"sender"`       // For deposit
}

// NewPeggyEvmHashSyncService creates a new sync service.
func NewPeggyEvmHashSyncService(
	cfg *Config,
	client PeggyEvmHashClient,
	redis *redis.Client,
	db *gorm.DB,
) *PeggyEvmHashSyncService {
	ctx, cancel := context.WithCancel(context.Background())

	// Set defaults
	if cfg.TaskPollInterval == 0 {
		cfg.TaskPollInterval = time.Second
	}
	if cfg.BatchUpdateSize == 0 {
		cfg.BatchUpdateSize = 100
	}

	return &PeggyEvmHashSyncService{
		config:  cfg,
		client:  client,
		redis:   redis,
		db:      db,
		metrics: NewPeggyEvmSyncMetrics(nil),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Run starts the sync service with multiple workers.
func (s *PeggyEvmHashSyncService) Run() error {
	global.Logger.Infof("Starting Peggy EVM Hash Sync Service with %d workers", s.config.WorkerCount)

	// Start metrics reporter
	go s.reportQueueLength()

	// Start workers
	for i := 0; i < s.config.WorkerCount; i++ {
		s.wg.Add(1)
		go s.worker(i)
	}

	global.Logger.Info("Peggy EVM Hash Sync Service started")
	return nil
}

// Stop gracefully stops the sync service.
func (s *PeggyEvmHashSyncService) Stop() error {
	global.Logger.Info("Stopping Peggy EVM Hash Sync Service...")

	// Cancel context to signal all workers to stop
	s.cancel()

	// Wait for all workers to finish (with timeout)
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		global.Logger.Info("Peggy EVM Hash Sync Service stopped gracefully")
		return nil
	case <-time.After(30 * time.Second):
		global.Logger.Warn("Peggy EVM Hash Sync Service stop timeout")
		return fmt.Errorf("stop timeout")
	}
}

// worker processes tasks from the queue.
func (s *PeggyEvmHashSyncService) worker(workerID int) {
	defer s.wg.Done()

	global.Logger.Debugf("Worker %d started", workerID)

	for {
		select {
		case <-s.ctx.Done():
			global.Logger.Debugf("Worker %d stopping", workerID)
			return
		default:
			task := s.dequeueTask()
			if task == nil {
				// Queue is empty, wait before polling again
				time.Sleep(s.config.TaskPollInterval)
				continue
			}

			s.processTask(task, workerID)
		}
	}
}

// dequeueTask pulls a task from the Redis queue.
func (s *PeggyEvmHashSyncService) dequeueTask() *SyncTask {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	// RPOP from the queue
	result, err := s.redis.RPop(ctx, s.config.QueueKey).Result()
	if err != nil || result == "" {
		return nil
	}

	var task SyncTask
	if err := json.Unmarshal([]byte(result), &task); err != nil {
		global.Logger.Errorf("Failed to unmarshal task: %v, data: %s", err, result)
		return nil
	}

	return &task
}

// processTask processes a single sync task.
func (s *PeggyEvmHashSyncService) processTask(task *SyncTask, workerID int) {
	start := time.Now()

	var txHash string
	var err error

	switch task.Type {
	case "deposit":
		txHash, err = s.processDeposit(task)
	case "withdrawal":
		txHash, err = s.processWithdrawal(task)
	default:
		global.Logger.Warnf("Unknown task type: %s", task.Type)
		s.metrics.RecordTask(task.Type, "invalid_type", time.Since(start))
		return
	}

	if err != nil {
		s.handleTaskError(task, err, workerID)
		s.metrics.RecordTask(task.Type, "error", time.Since(start))
		return
	}

	// Update database
	if err := s.updateDatabase(task, txHash); err != nil {
		global.Logger.Errorf("Failed to update database: %v", err)
		s.metrics.RecordTask(task.Type, "db_error", time.Since(start))
		return
	}

	s.metrics.RecordTask(task.Type, "success", time.Since(start))
	global.Logger.Infof("Worker %d: Successfully synced %s tx_hash=%s evm_tx_hash=%s duration=%s",
		workerID, task.Type, task.TxHash, txHash, time.Since(start))
}

// processDeposit processes a deposit sync task.
func (s *PeggyEvmHashSyncService) processDeposit(task *SyncTask) (string, error) {
	// Parse token contract from denom
	tokenContract, err := parseTokenContractFromDenom(task.Denom)
	if err != nil {
		return "", fmt.Errorf("invalid denom: %w", err)
	}

	// Query EVM chain
	txHash, err := s.client.GetDepositTxHash(
		s.ctx,
		task.EventNonce,
		task.BlockHeight,
		task.Sender,
		tokenContract,
		task.Amount,
	)

	if err != nil {
		return "", fmt.Errorf("failed to get deposit tx hash: %w", err)
	}

	return txHash, nil
}

// processWithdrawal processes a withdrawal sync task.
func (s *PeggyEvmHashSyncService) processWithdrawal(task *SyncTask) (string, error) {
	// Parse token contract from denom
	tokenContract, err := parseTokenContractFromDenom(task.Denom)
	if err != nil {
		return "", fmt.Errorf("invalid denom: %w", err)
	}

	// Query EVM chain
	txHash, err := s.client.GetWithdrawalBatchTxHash(
		s.ctx,
		task.BatchNonce,
		tokenContract,
	)

	if err != nil {
		return "", fmt.Errorf("failed to get withdrawal batch tx hash: %w", err)
	}

	return txHash, nil
}

// updateDatabase updates the database with the EVM tx hash.
func (s *PeggyEvmHashSyncService) updateDatabase(task *SyncTask, evmTxHash string) error {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	switch task.Type {
	case "deposit":
		return s.updateDeposit(ctx, task, evmTxHash)
	case "withdrawal":
		return s.updateWithdrawal(ctx, task, evmTxHash)
	default:
		return fmt.Errorf("unknown task type: %s", task.Type)
	}
}

// updateDeposit updates a deposit record.
func (s *PeggyEvmHashSyncService) updateDeposit(
	ctx context.Context,
	task *SyncTask,
	evmTxHash string,
) error {
	// Normalize the EVM tx hash
	evmTxHash = converter.NormalizeHash(evmTxHash)

	result := s.db.WithContext(ctx).
		Model(&models.PeggyDepositTx{}).
		Where("event_nonce = ? AND event_height = ?", task.EventNonce, task.BlockHeight).
		Where("eth_tx_hash = '' OR eth_tx_hash IS NULL").
		Update("eth_tx_hash", evmTxHash)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		global.Logger.Warnf("No deposit record updated: event_nonce=%d, block_height=%d",
			task.EventNonce, task.BlockHeight)
	}

	return nil
}

// updateWithdrawal updates withdrawal records (both MsgSendToEth and MsgWithdrawClaim).
func (s *PeggyEvmHashSyncService) updateWithdrawal(
	ctx context.Context,
	task *SyncTask,
	evmTxHash string,
) error {
	// Normalize the EVM tx hash
	evmTxHash = converter.NormalizeHash(evmTxHash)

	// Parse token contract from denom
	tokenContract, err := parseTokenContractFromDenom(task.Denom)
	if err != nil {
		return fmt.Errorf("invalid denom: %w", err)
	}

	// 1. Query batch mappings to get outgoing_tx_ids
	var outgoingTxIDs []uint64
	if err := s.db.WithContext(ctx).
		Model(&models.PeggyBatchTxMapping{}).
		Where("batch_nonce = ?", task.BatchNonce).
		Pluck("outgoing_tx_id", &outgoingTxIDs).Error; err != nil {
		global.Logger.Errorf("Failed to query batch mappings: %v", err)
		// Continue to update MsgWithdrawClaim even if mappings are missing
	}

	// 2. Update MsgSendToEth records (by outgoing_tx_id)
	if len(outgoingTxIDs) > 0 {
		if err := s.updateWithdrawalByOutgoingTxIDs(ctx, outgoingTxIDs, evmTxHash); err != nil {
			return err
		}
		global.Logger.Infof("Updated %d MsgSendToEth records for batch_nonce=%d",
			len(outgoingTxIDs), task.BatchNonce)
	}

	// 3. Update MsgWithdrawClaim records (by batch_nonce + denom)
	denomPattern := "peggy" + tokenContract + "%"
	result := s.db.WithContext(ctx).
		Model(&models.PeggyWithdrawalTx{}).
		Where("batch_nonce = ?", task.BatchNonce).
		Where("denom LIKE ?", denomPattern).
		Where("eth_tx_hash = '' OR eth_tx_hash IS NULL").
		Update("eth_tx_hash", evmTxHash)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected > 0 {
		global.Logger.Infof("Updated %d MsgWithdrawClaim records for batch_nonce=%d",
			result.RowsAffected, task.BatchNonce)
	}

	return nil
}

// updateWithdrawalByOutgoingTxIDs updates withdrawal records by outgoing_tx_ids in batches.
func (s *PeggyEvmHashSyncService) updateWithdrawalByOutgoingTxIDs(
	ctx context.Context,
	outgoingTxIDs []uint64,
	evmTxHash string,
) error {
	for i := 0; i < len(outgoingTxIDs); i += s.config.BatchUpdateSize {
		end := i + s.config.BatchUpdateSize
		if end > len(outgoingTxIDs) {
			end = len(outgoingTxIDs)
		}

		batchIDs := outgoingTxIDs[i:end]
		result := s.db.WithContext(ctx).
			Model(&models.PeggyWithdrawalTx{}).
			Where("outgoing_tx_id IN ?", batchIDs).
			Where("eth_tx_hash = '' OR eth_tx_hash IS NULL").
			Update("eth_tx_hash", evmTxHash)

		if result.Error != nil {
			return result.Error
		}
	}

	return nil
}

// handleTaskError handles task processing errors with retry logic.
func (s *PeggyEvmHashSyncService) handleTaskError(task *SyncTask, err error, workerID int) {
	global.Logger.Errorf("Worker %d: Failed to process task type=%s tx_hash=%s error=%v",
		workerID, task.Type, task.TxHash, err)

	// Check if error is retryable
	if isRetryableError(err) {
		s.retryTask(task)
		s.metrics.RecordTask(task.Type, "retry", 0)
	} else {
		s.metrics.RecordTask(task.Type, "failed", 0)
	}
}

// retryTask re-queues a task for retry.
func (s *PeggyEvmHashSyncService) retryTask(task *SyncTask) {
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Second)
	defer cancel()

	// Serialize task
	data, err := json.Marshal(task)
	if err != nil {
		global.Logger.Errorf("Failed to marshal task for retry: %v", err)
		return
	}

	// Push back to queue
	if err := s.redis.LPush(ctx, s.config.QueueKey, data).Err(); err != nil {
		global.Logger.Errorf("Failed to re-queue task: %v", err)
	}
}

// isRetryableError determines if an error is retryable.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	// Network timeouts, rate limits, and temporary errors are retryable
	if containsString(errMsg, "timeout") || containsString(errMsg, "rate limit") ||
		containsString(errMsg, "connection refused") || containsString(errMsg, "temporary") {
		return true
	}

	// "Not found" errors are not retryable
	if containsString(errMsg, "not found") {
		return false
	}

	return false
}

// contains checks if a string contains a substring (case-insensitive).
func containsString(s, substr string) bool {
	lower := toLower(s)
	subLower := toLower(substr)
	return indexOf(lower, subLower) >= 0
}

// indexOf finds the index of a substring in a string.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// parseTokenContractFromDenom extracts the token contract address from a Peggy denom.
func parseTokenContractFromDenom(denom string) (string, error) {
	const prefix = "peggy"
	if !hasPrefix(denom, prefix) {
		return "", fmt.Errorf("not a peggy denom: %s", denom)
	}

	addr := denom[len(prefix):]

	// Normalize address to lowercase with 0x prefix
	if len(addr) >= 2 && addr[0:2] == "0x" {
		addr = addr[2:]
	}

	return "0x" + toLower(addr), nil
}

// hasPrefix checks if a string has a prefix.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// toLower converts a string to lowercase.
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

// reportQueueLength periodically reports the queue length to metrics.
func (s *PeggyEvmHashSyncService) reportQueueLength() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
			length, err := s.redis.LLen(ctx, s.config.QueueKey).Result()
			cancel()

			if err == nil {
				s.metrics.UpdateQueueLength(int(length))
			}
		}
	}
}
