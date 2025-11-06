// Package service provides blockchain synchronization services.
// It continuously monitors new blocks, stores block data, processes transactions,
// and updates account information.
package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"

	"github.com/InjectiveLabs/sdk-go/chain/types"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	explorerclient "github.com/InjectiveLabs/sdk-go/client/explorer"
	tmclient "github.com/InjectiveLabs/sdk-go/client/tm"
	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// SyncService handles blockchain synchronization tasks.
// It monitors new blocks, stores block data, processes transactions,
// and updates account information.
type SyncService struct {
	// Configuration
	config *SyncConfig

	// Clients for blockchain interactions
	explorerClient explorerclient.ExplorerClient
	chainClient    chainclient.ChainClientV2
	tmClient       tmclient.TendermintClient

	// Current processed block height
	currentHeight uint64
	heightMutex   sync.RWMutex

	// Historical block sync state
	historicalSyncHeight uint64 // Current height being synced backwards
	historicalSyncMutex  sync.RWMutex

	// Control channels
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSyncService creates a new instance of SyncService.
//
// Parameters:
//   - config: Sync service configuration (can be nil to use defaults)
//
// Returns:
//   - *SyncService: A new sync service instance
func NewSyncService(config *SyncConfig) *SyncService {
	if config == nil {
		config = LoadSyncConfig(nil)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &SyncService{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start starts the blockchain synchronization service.
// It starts goroutines for block monitoring and transaction processing.
func (s *SyncService) Start() error {
	global.Logger.Infof("starting blockchain sync service")

	// Initialize clients
	if err := s.initClients(); err != nil {
		return errors.Wrap(err, "failed to initialize clients")
	}

	// Initialize current height from database and verify chain continuity
	// Then sync backwards from latest block to ensure we have the most recent blocks
	if err := s.initCurrentHeightWithVerification(); err != nil {
		global.Logger.Warnf("failed to initialize current height with verification: %v, starting from latest", err)
		// If failed, start from latest block
		latestHeight, err := s.getLatestBlockHeight()
		if err != nil {
			return errors.Wrap(err, "failed to get latest block height")
		}
		s.setCurrentHeight(latestHeight)
	} else {
		// Sync backwards from latest block to current height
		if err := s.syncBackwardsFromLatest(); err != nil {
			global.Logger.Warnf("failed to sync backwards from latest: %v", err)
			// Continue anyway, will sync forwards in the normal flow
		}
	}

	// Start block monitoring goroutine
	s.wg.Add(1)
	go s.blockMonitor()

	// Start multiple transaction processing goroutines for better performance
	for i := 0; i < s.config.ConcurrentTxProcessors; i++ {
		s.wg.Add(1)
		go s.transactionProcessor(i)
	}

	// Start historical block sync goroutine (low priority, runs in background)
	s.wg.Add(1)
	go s.historicalBlockSync()

	// Recover transactions from processing queue (in case of crash)
	if err := s.recoverProcessingQueue(); err != nil {
		global.Logger.Warnf("failed to recover processing queue: %v", err)
		// Continue anyway, will process them eventually
	}

	global.Logger.Infof("blockchain sync service started, current height: %d, %d transaction processors", s.getCurrentHeight(), s.config.ConcurrentTxProcessors)
	return nil
}

// Stop stops the blockchain synchronization service.
// It cancels the context and waits for all goroutines to finish with a timeout.
func (s *SyncService) Stop() error {
	global.Logger.Infof("stopping blockchain sync service")
	s.cancel()

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		global.Logger.Infof("blockchain sync service stopped gracefully")
		return nil
	case <-time.After(GracefulShutdownTimeout):
		global.Logger.Warnf("blockchain sync service stop timeout after %v, some goroutines may not have finished", GracefulShutdownTimeout)
		return errors.New("timeout waiting for goroutines to finish")
	}
}

// initClients initializes all blockchain clients.
// Uses shared client instances for better resource utilization.
func (s *SyncService) initClients() error {
	// Initialize explorer client (shared)
	client, err := GetSharedExplorerClient()
	if err != nil {
		return err
	}
	s.explorerClient = client

	// Initialize chain client (shared)
	chainClient, err := GetSharedChainClient()
	if err != nil {
		return err
	}
	s.chainClient = chainClient

	// Initialize Tendermint client (shared)
	s.tmClient = GetSharedTmClient()

	return nil
}

// initCurrentHeightWithVerification initializes the current height from the database
// and verifies that the stored blocks match the current chain.
func (s *SyncService) initCurrentHeightWithVerification() error {
	var maxHeight uint64
	var maxBlock models.Block

	// Get the maximum height from database
	err := db.Get().Model(&models.Block{}).
		Select("COALESCE(MAX(height), 0)").
		Scan(&maxHeight).Error

	if err != nil {
		return errors.Wrap(err, "failed to query max height from database")
	}

	// If no blocks in database, start from latest
	if maxHeight == 0 {
		latestHeight, err := s.getLatestBlockHeight()
		if err != nil {
			return errors.Wrap(err, "failed to get latest block height")
		}
		s.setCurrentHeight(latestHeight)
		global.Logger.Infof("no blocks in database, starting from latest height: %d", latestHeight)
		return nil
	}

	// Get the block at max height from database
	err = db.Get().Where("height = ?", maxHeight).First(&maxBlock).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Block not found, start from latest
			latestHeight, err := s.getLatestBlockHeight()
			if err != nil {
				return errors.Wrap(err, "failed to get latest block height")
			}
			s.setCurrentHeight(latestHeight)
			global.Logger.Warnf("block at height %d not found in database, starting from latest height: %d", maxHeight, latestHeight)
			return nil
		}
		return errors.Wrap(err, "failed to query max block from database")
	}

	// Verify the block hash matches the current chain
	// Get the same block from the chain
	ctx, cancel := context.WithTimeout(s.ctx, s.config.BlockQueryTimeout)
	defer cancel()

	blockResp, err := s.explorerClient.GetBlock(ctx, fmt.Sprintf("%d", maxHeight))
	if err != nil {
		// If we can't get the block, it might be from a different chain
		// Log warning and start from latest
		global.Logger.Warnf("failed to verify block at height %d: %v, starting from latest", maxHeight, err)
		latestHeight, err := s.getLatestBlockHeight()
		if err != nil {
			return errors.Wrap(err, "failed to get latest block height")
		}
		s.setCurrentHeight(latestHeight)
		return nil
	}

	if blockResp == nil || blockResp.Data == nil {
		global.Logger.Warnf("block response is nil at height %d, starting from latest", maxHeight)
		latestHeight, err := s.getLatestBlockHeight()
		if err != nil {
			return errors.Wrap(err, "failed to get latest block height")
		}
		s.setCurrentHeight(latestHeight)
		return nil
	}

	chainBlock := blockResp.Data

	// Compare block hashes
	if maxBlock.BlockHash != chainBlock.BlockHash {
		// Block hash doesn't match, this might be a different chain
		global.Logger.Warnf("block hash mismatch at height %d: database=%s, chain=%s, starting from latest",
			maxHeight, maxBlock.BlockHash, chainBlock.BlockHash)

		// Check if we can find a matching block at a lower height
		// Try to find the last common block by checking parent hash
		commonHeight, err := s.findCommonBlockHeight(maxBlock.BlockHash)
		if err != nil {
			global.Logger.Warnf("failed to find common block: %v, starting from latest", err)
			latestHeight, err := s.getLatestBlockHeight()
			if err != nil {
				return errors.Wrap(err, "failed to get latest block height")
			}
			s.setCurrentHeight(latestHeight)
			return nil
		}

		global.Logger.Infof("found common block at height %d, continuing from height %d", commonHeight, commonHeight)
		s.setCurrentHeight(commonHeight)
		return nil
	}

	// Block hash matches, continue from maxHeight
	s.setCurrentHeight(maxHeight)
	global.Logger.Infof("verified chain continuity at height %d, continuing from height %d", maxHeight, maxHeight)
	return nil
}

// findCommonBlockHeight tries to find the last common block by checking parent hashes.
func (s *SyncService) findCommonBlockHeight(knownHash string) (uint64, error) {
	// Start from a reasonable height (e.g., maxHeight - 1000) and check backwards
	// This is a simplified approach - in production, you might want a more sophisticated binary search
	var maxHeight uint64
	err := db.Get().Model(&models.Block{}).
		Select("COALESCE(MAX(height), 0)").
		Scan(&maxHeight).Error

	if err != nil {
		return 0, errors.Wrap(err, "failed to query max height")
	}

	// Check blocks backwards from maxHeight
	startHeight := maxHeight
	if startHeight > 1000 {
		startHeight = maxHeight - 1000
	}

	ctx, cancel := context.WithTimeout(s.ctx, s.config.BlockQueryTimeout)
	defer cancel()

	for height := startHeight; height <= maxHeight; height++ {
		// Check if this block exists in database
		var dbBlock models.Block
		err := db.Get().Where("height = ?", height).First(&dbBlock).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			continue
		}

		// Get the same block from chain
		blockResp, err := s.explorerClient.GetBlock(ctx, fmt.Sprintf("%d", height))
		if err != nil {
			continue
		}

		if blockResp == nil || blockResp.Data == nil {
			continue
		}

		chainBlock := blockResp.Data

		// If hash matches, this is a common block
		if dbBlock.BlockHash == chainBlock.BlockHash {
			return height, nil
		}
	}

	// No common block found, return 0 to start from beginning
	return 0, errors.New("no common block found")
}

// syncBackwardsFromLatest syncs backwards from the latest block to the current height.
// This ensures we have all the most recent blocks before continuing with forward sync.
func (s *SyncService) syncBackwardsFromLatest() error {
	currentHeight := s.getCurrentHeight()
	latestHeight, err := s.getLatestBlockHeight()
	if err != nil {
		return errors.Wrap(err, "failed to get latest block height")
	}

	// If latest height is less than or equal to current height, nothing to sync
	if latestHeight <= currentHeight {
		global.Logger.Debugf("latest height %d is not greater than current height %d, no backward sync needed", latestHeight, currentHeight)
		return nil
	}

	global.Logger.Infof("syncing backwards from latest height %d to current height %d", latestHeight, currentHeight)

	totalBlocks := latestHeight - currentHeight
	processedCount := uint64(0)

	// Use worker pool pattern for concurrent processing
	type blockJob struct {
		height uint64
	}

	jobs := make(chan blockJob, s.config.BackwardSyncBatchSize*2)
	results := make(chan error, s.config.BackwardSyncBatchSize*2)

	// Start worker goroutines
	var wg sync.WaitGroup
	for i := 0; i < s.config.BackwardSyncWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobs {
				// Check if context is cancelled
				select {
				case <-s.ctx.Done():
					results <- s.ctx.Err()
					return
				default:
				}

				// Process the block
				if err := s.processBlockOptimized(job.height); err != nil {
					// Check if error is due to context cancellation
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
						strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "context deadline exceeded") {
						results <- err
						return
					}
					global.Logger.Errorf("error processing block %d during backward sync (worker %d): %v", job.height, workerID, err)
					results <- nil // Continue processing other blocks
				} else {
					results <- nil
				}
			}
		}(i)
	}

	// Send jobs in batches
	go func() {
		defer close(jobs)
		for height := latestHeight; height > currentHeight; height-- {
			select {
			case <-s.ctx.Done():
				return
			case jobs <- blockJob{height: height}:
			}
		}
	}()

	// Collect results and update progress
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results
	lastLogTime := time.Now()
	for err := range results {
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				global.Logger.Infof("context cancelled during backward sync")
				return err
			}
		}
		processedCount++

		// Update current height periodically (use the highest processed height)
		if processedCount%uint64(s.config.BackwardSyncBatchSize) == 0 {
			// Update to the latest processed height
			latestProcessed := latestHeight - (processedCount - uint64(s.config.BackwardSyncBatchSize))
			s.setCurrentHeight(latestProcessed)
		}

		// Log progress every N blocks or every configured interval
		now := time.Now()
		if processedCount%uint64(s.config.BackwardSyncLogBatchSize) == 0 || now.Sub(lastLogTime) >= s.config.BackwardSyncLogInterval {
			global.Logger.Infof("backward sync progress: %d/%d blocks processed (%.2f%%)",
				processedCount, totalBlocks, float64(processedCount)/float64(totalBlocks)*100)
			lastLogTime = now
		}
	}

	// Update to final height
	s.setCurrentHeight(latestHeight)

	global.Logger.Infof("backward sync completed from height %d to %d", latestHeight, currentHeight)
	return nil
}

// getCurrentHeight returns the current processed block height.
func (s *SyncService) getCurrentHeight() uint64 {
	s.heightMutex.RLock()
	defer s.heightMutex.RUnlock()
	return s.currentHeight
}

// setCurrentHeight sets the current processed block height.
func (s *SyncService) setCurrentHeight(height uint64) {
	s.heightMutex.Lock()
	defer s.heightMutex.Unlock()
	s.currentHeight = height
}

// getLatestBlockHeight gets the latest block height from the blockchain.
func (s *SyncService) getLatestBlockHeight() (uint64, error) {
	height, err := s.tmClient.GetLatestBlockHeight(s.ctx)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get latest block height")
	}
	// Convert int64 to uint64
	if height < 0 {
		return 0, errors.New("block height cannot be negative")
	}
	return uint64(height), nil
}

// blockMonitor continuously monitors new blocks and processes them.
func (s *SyncService) blockMonitor() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.BlockPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := s.processNewBlocks(); err != nil {
				global.Logger.Errorf("error processing new blocks: %v", err)
			}
		}
	}
}

// processNewBlocks processes all new blocks since the last processed height.
func (s *SyncService) processNewBlocks() error {
	currentHeight := s.getCurrentHeight()
	latestHeight, err := s.getLatestBlockHeight()
	if err != nil {
		return errors.Wrap(err, "failed to get latest block height")
	}

	if latestHeight <= currentHeight {
		// No new blocks
		return nil
	}

	global.Logger.Infof("processing blocks from %d to %d", currentHeight+1, latestHeight)

	// Process blocks one by one
	for height := currentHeight + 1; height <= latestHeight; height++ {
		// Check if context is cancelled before processing each block
		select {
		case <-s.ctx.Done():
			global.Logger.Infof("context cancelled, stopping block processing at height %d", height-1)
			return s.ctx.Err()
		default:
		}

		if err := s.processBlock(height); err != nil {
			// Check if error is due to context cancellation
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				global.Logger.Infof("context cancelled or timeout while processing block %d, stopping", height)
				return err
			}
			global.Logger.Errorf("error processing block %d: %v", height, err)
			// Continue processing next block even if one fails
			continue
		}
		s.setCurrentHeight(height)
	}

	return nil
}

// processBlock processes a single block at the given height.
func (s *SyncService) processBlock(height uint64) error {
	ctx, cancel := context.WithTimeout(s.ctx, s.config.BlockQueryTimeout)
	defer cancel()

	// Get block information with retry logic
	var blockResp *injective_explorer_rpcpb.GetBlockResponse

	delay := s.config.RetryDelay
	var err error
	for attempt := 0; attempt < s.config.MaxRetryAttempts; attempt++ {
		blockResp, err = s.explorerClient.GetBlock(ctx, fmt.Sprintf("%d", height))
		if err == nil {
			break
		}

		// Check if error is due to context cancellation or timeout
		errStr := err.Error()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
			strings.Contains(errStr, "context canceled") || strings.Contains(errStr, "context deadline exceeded") {
			return err
		}

		// Check if error is retryable (5xx errors, Unavailable, etc.)
		if isRetryableError(err) {
			if attempt < s.config.MaxRetryAttempts-1 {
				global.Logger.Warnf("retryable error getting block at height %d (attempt %d/%d): %v, retrying in %v...",
					height, attempt+1, s.config.MaxRetryAttempts, err, delay)
				time.Sleep(delay)
				delay *= 2 // Exponential backoff
				continue
			}
		}

		// Non-retryable error or max retries reached
		return errors.Wrapf(err, "failed to get block at height %d", height)
	}

	if err != nil {
		return errors.Wrapf(err, "failed to get block at height %d after %d attempts", height, s.config.MaxRetryAttempts)
	}

	if blockResp == nil || blockResp.Data == nil {
		return errors.Errorf("block response is nil for height %d", height)
	}

	block := blockResp.Data

	// Save block to database using transaction for atomicity
	blockModel := models.BlockFromProto(block)
	if blockModel == nil {
		return errors.Errorf("failed to convert block at height %d", height)
	}

	// Use transaction to ensure atomicity
	err = db.Get().Transaction(func(tx *gorm.DB) error {
		// Check if block already exists
		var existingBlock models.Block
		err := tx.Where("height = ?", height).First(&existingBlock).Error
		if err == nil {
			// Block exists, update it
			err = tx.Model(&existingBlock).Updates(blockModel).Error
			if err != nil {
				return errors.Wrapf(err, "failed to update block at height %d", height)
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// Block doesn't exist, create it
			err = tx.Create(blockModel).Error
			if err != nil {
				return errors.Wrapf(err, "failed to create block at height %d", height)
			}
		} else {
			return errors.Wrapf(err, "failed to check block existence at height %d", height)
		}

		// Extract transaction hashes and push to Redis queue
		// Note: Redis operations are outside the database transaction
		// If Redis push fails, the block is still saved (eventual consistency)
		if len(block.Txs) > 0 {
			redisClient := cache.GetRedisClient()
			if redisClient != nil {
				for _, txHash := range block.Txs {
					// Push to Redis queue
					pushErr := redisClient.LPush(ctx, s.config.TxHashQueueKey, txHash.Hash).Err()
					if pushErr != nil {
						global.Logger.Errorf("failed to push tx hash %s to queue: %v", txHash.Hash, pushErr)
						// Continue processing other transactions
						// Don't fail the transaction if Redis push fails
						continue
					}
				}
			} else {
				global.Logger.Warnf("redis client is nil, skipping tx hash queue")
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// processBlockOptimized is an optimized version of processBlock for backward sync.
// It skips unnecessary checks and uses batch operations where possible.
func (s *SyncService) processBlockOptimized(height uint64) error {
	ctx, cancel := context.WithTimeout(s.ctx, s.config.BlockQueryTimeout)
	defer cancel()

	// Quick check if block already exists (skip hash verification for performance)
	var existingBlockCheck models.Block
	err := db.Get().Where("height = ?", height).First(&existingBlockCheck).Error
	if err == nil {
		// Block exists, skip processing (assume it's correct for backward sync)
		// We can skip hash verification to speed up sync
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error, log and continue
		global.Logger.Warnf("failed to check block existence at height %d: %v", height, err)
	}

	// Get block information with retry logic
	var blockResp *injective_explorer_rpcpb.GetBlockResponse

	delay := s.config.RetryDelay
	for attempt := 0; attempt < s.config.MaxRetryAttempts; attempt++ {
		blockResp, err = s.explorerClient.GetBlock(ctx, fmt.Sprintf("%d", height))
		if err == nil {
			break
		}

		// Check if error is due to context cancellation or timeout
		errStr := err.Error()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
			strings.Contains(errStr, "context canceled") || strings.Contains(errStr, "context deadline exceeded") {
			return err
		}

		// Check if error is retryable
		if isRetryableError(err) {
			if attempt < s.config.MaxRetryAttempts-1 {
				time.Sleep(delay)
				delay *= 2 // Exponential backoff
				continue
			}
		}

		// Non-retryable error or max retries reached
		return errors.Wrapf(err, "failed to get block at height %d", height)
	}

	if err != nil {
		return errors.Wrapf(err, "failed to get block at height %d after %d attempts", height, s.config.MaxRetryAttempts)
	}

	if blockResp == nil || blockResp.Data == nil {
		return errors.Errorf("block response is nil for height %d", height)
	}

	block := blockResp.Data

	// Save block to database using transaction for atomicity
	blockModel := models.BlockFromProto(block)
	if blockModel == nil {
		return errors.Errorf("failed to convert block at height %d", height)
	}

	// Use transaction to ensure atomicity
	err = db.Get().Transaction(func(tx *gorm.DB) error {
		// Check if block already exists
		var existingBlock models.Block
		err := tx.Where("height = ?", height).First(&existingBlock).Error
		if err == nil {
			// Block exists, update it
			err = tx.Model(&existingBlock).Updates(blockModel).Error
			if err != nil {
				return errors.Wrapf(err, "failed to update block at height %d", height)
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// Block doesn't exist, create it
			err = tx.Create(blockModel).Error
			if err != nil {
				return errors.Wrapf(err, "failed to create block at height %d", height)
			}
		} else {
			return errors.Wrapf(err, "failed to check block existence at height %d", height)
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Extract transaction hashes and push to Redis queue (batch push for better performance)
	if len(block.Txs) > 0 {
		redisClient := cache.GetRedisClient()
		if redisClient != nil {
			// Batch push to Redis for better performance
			txHashes := make([]interface{}, 0, len(block.Txs))
			for _, txHash := range block.Txs {
				txHashes = append(txHashes, txHash.Hash)
			}
			// Use pipeline for batch operations
			pipe := redisClient.Pipeline()
			for _, hash := range txHashes {
				pipe.LPush(ctx, s.config.TxHashQueueKey, hash)
			}
			_, err = pipe.Exec(ctx)
			if err != nil {
				global.Logger.Warnf("failed to batch push tx hashes to queue: %v", err)
				// Continue even if Redis push fails
			}
		}
	}

	return nil
}

// transactionProcessor continuously processes transactions from the Redis queue.
func (s *SyncService) transactionProcessor(id int) {
	defer func() {
		global.Logger.Infof("transaction processor %d exiting", id)
		s.wg.Done()
	}()

	global.Logger.Infof("transaction processor %d started", id)

	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		global.Logger.Errorf("redis client is nil, transaction processor %d cannot start", id)
		return
	}

	global.Logger.Infof("transaction processor %d ready, waiting for transactions from queue", id)

	processedCount := 0
	emptyCount := 0
	errorCount := 0

	for {
		select {
		case <-s.ctx.Done():
			global.Logger.Infof("transaction processor %d stopped by context, processed: %d, empty: %d, errors: %d",
				id, processedCount, emptyCount, errorCount)
			return
		default:
			// Process transaction immediately (no ticker, continuous processing)
			err := s.processTransaction(redisClient)
			if err != nil {
				if err == errQueueEmpty {
					// Queue is empty, this is normal
					emptyCount++
					// If queue is empty, wait a bit before checking again
					time.Sleep(s.config.TxProcessInterval)
				} else {
					// Real error occurred
					errorCount++
					global.Logger.Errorf("transaction processor %d: error processing transaction (error count: %d): %v", id, errorCount, err)
					// Wait a bit before retrying to avoid hammering on errors
					time.Sleep(s.config.TxProcessInterval)
				}
			} else {
				// Successfully processed
				processedCount++
			}
		}
	}
}

var errQueueEmpty = errors.New("queue is empty")

// processTransaction processes a single transaction from the Redis queue.
// It uses BRPOPLPUSH to move the transaction hash to a processing queue,
// ensuring that if the process crashes, the transaction can be recovered.
func (s *SyncService) processTransaction(redisClient *redis.Client) error {
	ctx, cancel := context.WithTimeout(s.ctx, s.config.BlockQueryTimeout)
	defer cancel()

	// Use BRPOPLPUSH to atomically move transaction hash from queue to processing queue
	// This ensures that if the process crashes, the transaction can be recovered
	// BRPOPLPUSH blocks until an element is available or timeout
	// Note: Redis requires timeout to be at least 1 second
	blockTimeout := s.config.TxProcessInterval
	if blockTimeout < time.Second {
		blockTimeout = time.Second
	}
	txHash, err := redisClient.BRPopLPush(ctx, s.config.TxHashQueueKey, s.config.TxHashProcessingQueueKey, blockTimeout).Result()
	if err != nil {
		// Check for empty queue error (this is expected when queue is empty)
		// redis.Nil is returned when the queue is empty or timeout
		if err == redis.Nil || err.Error() == "redis: nil" || strings.Contains(err.Error(), "redis: nil") {
			// Queue is empty or timeout, this is normal - don't log as error
			return errQueueEmpty
		}
		global.Logger.Errorf("failed to pop from tx hash queue: %v", err)
		return errors.Wrap(err, "failed to pop from tx hash queue")
	}

	// Get transaction information with retry logic
	var txResp *injective_explorer_rpcpb.GetTxByTxHashResponse

	delay := s.config.RetryDelay
	for attempt := 0; attempt < s.config.MaxRetryAttempts; attempt++ {
		txResp, err = s.explorerClient.GetTxByTxHash(ctx, txHash)
		if err == nil {
			break
		}

		// Check if error is due to context cancellation or timeout
		errStr := err.Error()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
			strings.Contains(errStr, "context canceled") || strings.Contains(errStr, "context deadline exceeded") {
			return err
		}

		// Check if error is retryable (5xx errors, Unavailable, etc.)
		if isRetryableError(err) {
			if attempt < s.config.MaxRetryAttempts-1 {
				global.Logger.Warnf("retryable error getting transaction %s (attempt %d/%d): %v, retrying in %v...",
					txHash, attempt+1, s.config.MaxRetryAttempts, err, delay)
				time.Sleep(delay)
				delay *= 2 // Exponential backoff
				continue
			}
		}

		// Non-retryable error or max retries reached
		// If transaction not found, log and continue
		global.Logger.Warnf("failed to get transaction %s: %v", txHash, err)
		return nil
	}

	if err != nil {
		global.Logger.Warnf("failed to get transaction %s after %d attempts: %v", txHash, s.config.MaxRetryAttempts, err)
		return nil
	}

	if txResp == nil || txResp.Data == nil {
		global.Logger.Warnf("transaction response is nil for hash %s", txHash)
		return nil
	}

	txData := txResp.Data

	// Save transaction to database using transaction for atomicity
	txModel := models.TransactionFromProto(txData)
	if txModel == nil {
		// If conversion fails, put the transaction hash back to the queue
		redisClient.LPush(ctx, s.config.TxHashQueueKey, txHash)
		redisClient.LRem(ctx, s.config.TxHashProcessingQueueKey, 0, txHash)
		global.Logger.Errorf("failed to convert transaction %s to model", txHash)
		return errors.Errorf("failed to convert transaction %s", txHash)
	}

	// Use database transaction to ensure atomicity
	err = db.Get().Transaction(func(tx *gorm.DB) error {
		// Check if transaction already exists
		var existingTx models.Transaction
		err := tx.Where("hash = ?", txHash).First(&existingTx).Error
		if err == nil {
			// Transaction exists, update it
			err = tx.Model(&existingTx).Updates(txModel).Error
			if err != nil {
				return errors.Wrapf(err, "failed to update transaction %s", txHash)
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// Transaction doesn't exist, create it
			err = tx.Create(txModel).Error
			if err != nil {
				return errors.Wrapf(err, "failed to create transaction %s", txHash)
			}
		} else {
			return errors.Wrapf(err, "failed to check transaction existence %s", txHash)
		}

		// Extract and update account information from transaction
		// Note: Account updates are done within the same transaction
		if err := s.updateAccountsFromTransactionWithTx(ctx, tx, txData); err != nil {
			global.Logger.Warnf("failed to update accounts from transaction %s: %v", txHash, err)
			// Don't fail the transaction processing if account update fails
			// But log it for debugging
		}

		return nil
	})

	if err != nil {
		// If database transaction fails, put the transaction hash back to the queue
		redisClient.LPush(ctx, s.config.TxHashQueueKey, txHash)
		redisClient.LRem(ctx, s.config.TxHashProcessingQueueKey, 0, txHash)
		global.Logger.Errorf("failed to save transaction %s: %v, put back to queue", txHash, err)
		return errors.Wrapf(err, "failed to save transaction %s", txHash)
	}

	// Transaction saved successfully, remove from processing queue
	redisClient.LRem(ctx, s.config.TxHashProcessingQueueKey, 0, txHash)

	return nil
}

// updateAccountsFromTransactionWithTx extracts account addresses from transaction and updates database using the provided transaction.
func (s *SyncService) updateAccountsFromTransactionWithTx(ctx context.Context, tx *gorm.DB, txData *injective_explorer_rpcpb.TxDetailData) error {
	// Extract addresses from transaction events
	addresses := make(map[string]bool)

	// Extract addresses from events
	// Events typically contain transfer events with sender/receiver addresses
	for _, event := range txData.Events {
		if event == nil {
			continue
		}
		eventStr := event.String()

		// Try to extract addresses from event string
		// Common patterns: "sender", "recipient", "from", "to", etc.
		addressesFromEvent := extractAddressesFromString(eventStr)
		for _, addr := range addressesFromEvent {
			if isValidAddress(addr) {
				addresses[addr] = true
			}
		}
	}

	// Also try to extract from messages field if it contains address information
	if len(txData.Messages) > 0 {
		messagesStr := string(txData.Messages)
		addressesFromMessages := extractAddressesFromString(messagesStr)
		for _, addr := range addressesFromMessages {
			if isValidAddress(addr) {
				addresses[addr] = true
			}
		}
	}

	for _, signer := range txData.Signatures {
		if signer == nil {
			continue
		}
		if isValidAddress(signer.Address) {
			addresses[signer.Address] = true
		}
	}

	// Update accounts in database using the provided transaction
	for address := range addresses {
		if address == "" {
			continue
		}
		// Get account information from chain
		account, err := s.chainClient.GetAccount(ctx, address)
		if err != nil {
			// If account doesn't exist or error, skip it
			continue
		}

		ethAccount := &types.EthAccount{}
		err = s.chainClient.ClientContext().Codec.Unmarshal(account.Account.Value, ethAccount)
		if err != nil {
			global.Logger.Errorf("unmarshal eth account error: %v", err)
			continue
		}

		// Save or update account in database using the provided transaction
		accountModel := models.AccountFromProto(ethAccount, address, s.chainClient.ClientContext().Codec)
		if accountModel == nil {
			global.Logger.Warnf("failed to convert account %s to model", address)
			continue
		}

		var existingAccount models.Account
		err = tx.Where("address = ?", address).First(&existingAccount).Error
		if err == nil {
			// Account exists, check if update is needed
			needsUpdate := existingAccount.BaseAccount != accountModel.BaseAccount ||
				existingAccount.CodeHash != accountModel.CodeHash

			if needsUpdate {
				// Only update if fields have changed
				err = tx.Model(&existingAccount).Updates(accountModel).Error
				if err != nil {
					global.Logger.Warnf("failed to update account %s: %v", address, err)
				}
			}
			// If no update needed, skip database operation
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// Account doesn't exist, create it
			err = tx.Create(accountModel).Error
			if err != nil {
				global.Logger.Warnf("failed to create account %s: %v", address, err)
			}
		} else {
			global.Logger.Warnf("failed to check account %s existence: %v", address, err)
			continue
		}

		// Get and update account balances (outside transaction for now)
		// Note: Balance updates are done separately as they may take longer
		if err := s.updateAccountBalances(ctx, address); err != nil {
			global.Logger.Warnf("failed to update balances for account %s: %v", address, err)
			// Continue processing even if balance update fails
		}
	}

	return nil
}

// updateAccountsFromTransaction extracts account addresses from transaction and updates database.
func (s *SyncService) updateAccountsFromTransaction(ctx context.Context, txData *injective_explorer_rpcpb.TxDetailData) error {
	// Extract addresses from transaction events
	addresses := make(map[string]bool)

	// Extract addresses from events
	// Events typically contain transfer events with sender/receiver addresses
	for _, event := range txData.Events {
		if event == nil {
			continue
		}
		eventStr := event.String()

		// Try to extract addresses from event string
		// Common patterns: "sender", "recipient", "from", "to", etc.
		addressesFromEvent := extractAddressesFromString(eventStr)
		for _, addr := range addressesFromEvent {
			if isValidAddress(addr) {
				addresses[addr] = true
			}
		}
	}

	// Also try to extract from messages field if it contains address information
	if len(txData.Messages) > 0 {
		messagesStr := string(txData.Messages)
		addressesFromMessages := extractAddressesFromString(messagesStr)
		for _, addr := range addressesFromMessages {
			if isValidAddress(addr) {
				addresses[addr] = true
			}
		}
	}

	for _, signer := range txData.Signatures {
		if signer == nil {
			continue
		}
		if isValidAddress(signer.Address) {
			addresses[signer.Address] = true
		}
	}

	// Update accounts in database
	for address := range addresses {
		if address == "" {
			continue
		}
		global.Logger.Infof("create or update address: %s", address)
		// Get account information from chain
		account, err := s.chainClient.GetAccount(ctx, address)
		if err != nil {
			// If account doesn't exist or error, skip it
			continue
		}

		ethAccount := &types.EthAccount{}
		err = s.chainClient.ClientContext().Codec.Unmarshal(account.Account.Value, ethAccount)
		if err != nil {
			global.Logger.Errorf("unmarshal eth account error: %v", err)
			continue
		}

		// Save or update account in database
		accountModel := models.AccountFromProto(ethAccount, address, s.chainClient.ClientContext().Codec)
		if accountModel == nil {
			global.Logger.Warnf("failed to convert account %s to model", address)
			continue
		}

		var existingAccount models.Account
		err = db.Get().Where("address = ?", address).First(&existingAccount).Error
		if err == nil {
			// Account exists, check if update is needed
			needsUpdate := existingAccount.BaseAccount != accountModel.BaseAccount ||
				existingAccount.CodeHash != accountModel.CodeHash

			if needsUpdate {
				// Only update if fields have changed
				err = db.Get().Model(&existingAccount).Updates(accountModel).Error
				if err != nil {
					global.Logger.Warnf("failed to update account %s: %v", address, err)
				}
			}
			// If no update needed, skip database operation
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// Account doesn't exist, create it
			err = db.Get().Create(accountModel).Error
			if err != nil {
				global.Logger.Warnf("failed to create account %s: %v", address, err)
			} else {
				global.Logger.Infof("created new account %s", address)
			}
		} else {
			global.Logger.Warnf("failed to check account %s existence: %v", address, err)
			continue
		}

		// Get and update account balances
		if err := s.updateAccountBalances(ctx, address); err != nil {
			global.Logger.Warnf("failed to update balances for account %s: %v", address, err)
			// Continue processing even if balance update fails
		}
	}

	return nil
}

// updateAccountBalances updates account balances for a given address.
func (s *SyncService) updateAccountBalances(ctx context.Context, address string) error {
	// Get account balances from chain
	balancesResp, err := s.chainClient.GetBankBalances(ctx, address)
	if err != nil {
		// If account doesn't exist or error, skip it
		return err
	}

	if balancesResp == nil || balancesResp.Balances == nil {
		return nil
	}

	// Update or create balance records for each denom
	for _, balance := range balancesResp.Balances {
		balanceModel := models.AccountBalanceFromProto(balance, address)
		if balanceModel == nil {
			continue
		}

		// Check if balance already exists
		var existingBalance models.AccountBalance
		err = db.Get().Where("address = ? AND denom = ?", address, balance.Denom).First(&existingBalance).Error
		if err == nil {
			// Balance exists, update it if amount changed
			if existingBalance.Amount != balanceModel.Amount {
				err = db.Get().Model(&existingBalance).Update("amount", balanceModel.Amount).Error
				if err != nil {
					global.Logger.Warnf("failed to update balance for account %s, denom %s: %v", address, balance.Denom, err)
				}
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// Balance doesn't exist, create it
			err = db.Get().Create(balanceModel).Error
			if err != nil {
				global.Logger.Warnf("failed to create balance for account %s, denom %s: %v", address, balance.Denom, err)
			}
		} else {
			global.Logger.Warnf("failed to check balance existence for account %s, denom %s: %v", address, balance.Denom, err)
		}
	}

	return nil
}

// extractAddressesFromString extracts Injective addresses from a string.
// Injective addresses typically start with "inj1" and are 42 characters long.
func extractAddressesFromString(s string) []string {
	// Pattern for Injective addresses: inj1 followed by 38 characters (base58 encoded)
	// Total length: 42 characters
	re := regexp.MustCompile(`inj1[0-9a-z]{38}`)
	matches := re.FindAllString(s, -1)

	// Remove duplicates
	seen := make(map[string]bool)
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		if !seen[match] {
			seen[match] = true
			result = append(result, match)
		}
	}

	return result
}

// isValidAddress checks if a string is a valid Injective address.
func isValidAddress(addr string) bool {
	// Injective addresses start with "inj1" and are 42 characters long
	return strings.HasPrefix(addr, "inj1") && len(addr) == 42
}

// isRetryableError checks if an error is retryable.
// Retryable errors include:
// - 5xx HTTP status codes (500, 502, 503, 504)
// - gRPC Unavailable errors
// - Network timeout errors (but not context cancellation)
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Check for gRPC status codes
	if strings.Contains(errStr, "code = Unavailable") {
		return true
	}

	// Check for HTTP 5xx status codes
	if strings.Contains(errStr, "503") || strings.Contains(errStr, "Service Unavailable") {
		return true
	}
	if strings.Contains(errStr, "500") || strings.Contains(errStr, "Internal Server Error") {
		return true
	}
	if strings.Contains(errStr, "502") || strings.Contains(errStr, "Bad Gateway") {
		return true
	}
	if strings.Contains(errStr, "504") || strings.Contains(errStr, "Gateway Timeout") {
		return true
	}

	// Check for network-related errors (but not context cancellation)
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "no such host") {
		return true
	}

	return false
}

// historicalBlockSync continuously syncs historical blocks in the background.
// It starts from the earliest block height supported by the RPC node (EarliestBlockHeight)
// and gradually syncs forward to the minimum height in the database.
// This runs with low priority to avoid affecting new block synchronization.
func (s *SyncService) historicalBlockSync() {
	defer func() {
		global.Logger.Infof("historical block sync exiting")
		s.wg.Done()
	}()

	global.Logger.Infof("historical block sync started")

	// Get RPC node status to determine earliest block height
	status, err := s.tmClient.GetStatus(s.ctx)
	if err != nil {
		global.Logger.Warnf("failed to get RPC node status: %v, will try again later", err)
		// Wait a bit and retry
		time.Sleep(s.config.RpcStatusRetryDelay)
		return
	}

	// Get earliest block height supported by the RPC node
	earliestBlockHeight := s.config.EarliestBlockHeight
	if status != nil && status.SyncInfo.EarliestBlockHeight > 0 {
		earliestBlockHeight = uint64(status.SyncInfo.EarliestBlockHeight)
		global.Logger.Infof("RPC node supports blocks from height %d", earliestBlockHeight)
	} else {
		global.Logger.Infof("RPC node earliest block height not available, defaulting to block 1")
	}

	// Initialize historical sync height from database
	// Start from earliest block height, or the minimum height in database if we have blocks
	var minHeight uint64 = earliestBlockHeight
	var maxHeight uint64
	err = db.Get().Model(&models.Block{}).
		Select("COALESCE(MIN(height), ?)", earliestBlockHeight).
		Scan(&minHeight).Error
	if err != nil {
		global.Logger.Warnf("failed to query min height from database: %v, starting from earliest block height %d", err, earliestBlockHeight)
		minHeight = earliestBlockHeight
	}

	// Get max height to determine sync range
	err = db.Get().Model(&models.Block{}).
		Select("COALESCE(MAX(height), 0)").
		Scan(&maxHeight).Error
	if err != nil {
		global.Logger.Warnf("failed to query max height from database: %v", err)
	}

	// If we already have blocks from earliest height, check if we need to sync historical blocks
	if minHeight > earliestBlockHeight {
		// We have blocks, but not from the beginning
		// Start syncing from earliest block height to minHeight - 1
		global.Logger.Infof("historical block sync: found blocks from height %d, will sync from height %d to %d", minHeight, earliestBlockHeight, minHeight-1)
		s.setHistoricalSyncHeight(earliestBlockHeight)
	} else {
		// No blocks or starting from earliest block height, start from earliest block height
		global.Logger.Infof("historical block sync: starting from earliest block height %d", earliestBlockHeight)
		s.setHistoricalSyncHeight(earliestBlockHeight)
	}

	ticker := time.NewTicker(s.config.HistoricalSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			global.Logger.Infof("historical block sync stopped by context")
			return
		case <-ticker.C:
			// Check RPC node status before syncing
			status, err := s.tmClient.GetStatus(s.ctx)
			if err != nil {
				global.Logger.Warnf("failed to get RPC node status: %v, skipping historical sync", err)
				continue
			}

			// Check if RPC node is syncing (not caught up)
			if status != nil && status.SyncInfo.CatchingUp {
				global.Logger.Debugf("RPC node is catching up, skipping historical sync to avoid pressure")
				continue
			}

			// Get current height to avoid syncing blocks that are being synced by forward sync
			currentHeight := s.getCurrentHeight()
			historicalHeight := s.getHistoricalSyncHeight()

			// Get earliest block height from RPC node status
			earliestBlockHeight := s.config.EarliestBlockHeight
			if status != nil && status.SyncInfo.EarliestBlockHeight > 0 {
				earliestBlockHeight = uint64(status.SyncInfo.EarliestBlockHeight)
			}

			// Get minimum height in database to determine sync target
			var dbMinHeight uint64 = earliestBlockHeight
			err = db.Get().Model(&models.Block{}).
				Select("COALESCE(MIN(height), ?)", earliestBlockHeight).
				Scan(&dbMinHeight).Error
			if err != nil {
				global.Logger.Warnf("failed to query min height from database: %v", err)
				continue
			}

			// Determine target height (sync up to min height in database, but not beyond current height)
			// Don't sync below earliest block height
			targetHeight := dbMinHeight - 1
			if targetHeight < earliestBlockHeight {
				targetHeight = earliestBlockHeight
			}

			// If we've reached the target, check if we need to continue
			if historicalHeight >= targetHeight {
				// Check if there are gaps in the database
				// Find the first missing block starting from historicalHeight
				nextMissingHeight := s.findNextMissingBlock(historicalHeight, targetHeight)
				if nextMissingHeight == 0 {
					// No missing blocks found, we're done
					global.Logger.Debugf("historical block sync: no missing blocks found, waiting...")
					time.Sleep(s.config.HistoricalSyncInterval)
					continue
				}
				targetHeight = nextMissingHeight
			}

			// Don't sync beyond current height to avoid conflicts
			if historicalHeight >= currentHeight {
				global.Logger.Debugf("historical block sync: reached current height %d, waiting...", currentHeight)
				time.Sleep(s.config.HistoricalSyncInterval)
				continue
			}

			// Sync a batch of historical blocks
			if err := s.syncHistoricalBlocksBatch(historicalHeight, targetHeight, currentHeight); err != nil {
				global.Logger.Errorf("error syncing historical blocks batch: %v", err)
				// Continue even if batch fails
			}
		}
	}
}

// syncHistoricalBlocksBatch syncs a batch of historical blocks.
// It processes blocks from startHeight to targetHeight, but not beyond currentHeight.
func (s *SyncService) syncHistoricalBlocksBatch(startHeight, targetHeight, currentHeight uint64) error {
	// Calculate batch size (don't exceed target or current height)
	batchEnd := startHeight + uint64(s.config.HistoricalSyncBatchSize)
	if batchEnd > targetHeight {
		batchEnd = targetHeight
	}
	if batchEnd > currentHeight {
		batchEnd = currentHeight
	}
	if batchEnd <= startHeight {
		return nil // No blocks to sync
	}

	global.Logger.Infof("historical block sync: syncing blocks from %d to %d", startHeight, batchEnd-1)

	// Use worker pool pattern for concurrent processing
	type blockJob struct {
		height uint64
	}

	jobs := make(chan blockJob, s.config.HistoricalSyncBatchSize)
	results := make(chan error, s.config.HistoricalSyncBatchSize)

	// Start worker goroutines
	var wg sync.WaitGroup
	for i := 0; i < s.config.HistoricalSyncWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobs {
				// Check if context is cancelled
				select {
				case <-s.ctx.Done():
					results <- s.ctx.Err()
					return
				default:
				}

				// Process the block
				if err := s.processBlockOptimized(job.height); err != nil {
					// Check if error is due to context cancellation
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
						strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "context deadline exceeded") {
						results <- err
						return
					}
					global.Logger.Warnf("error processing historical block %d (worker %d): %v", job.height, workerID, err)
					results <- nil // Continue processing other blocks
				} else {
					results <- nil
				}
			}
		}(i)
	}

	// Send jobs
	go func() {
		defer close(jobs)
		for height := startHeight; height < batchEnd; height++ {
			select {
			case <-s.ctx.Done():
				return
			case jobs <- blockJob{height: height}:
			}
		}
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results
	processedCount := uint64(0)
	lastHeight := startHeight
	for err := range results {
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
		}
		processedCount++
		lastHeight = startHeight + processedCount - 1
	}

	// Update historical sync height
	if processedCount > 0 {
		s.setHistoricalSyncHeight(lastHeight + 1)
		global.Logger.Infof("historical block sync: synced %d blocks, current height: %d", processedCount, lastHeight+1)
	}

	// Add delay between batches to avoid RPC pressure
	time.Sleep(s.config.HistoricalBatchDelay)

	return nil
}

// findNextMissingBlock finds the next missing block starting from startHeight.
// It returns 0 if no missing blocks are found.
func (s *SyncService) findNextMissingBlock(startHeight, maxHeight uint64) uint64 {
	// Check blocks in batches to find gaps
	batchSize := uint64(s.config.MissingBlockCheckBatchSize)
	for height := startHeight; height <= maxHeight; height += batchSize {
		endHeight := height + batchSize
		if endHeight > maxHeight {
			endHeight = maxHeight + 1
		}

		// Query database for blocks in this range
		var existingBlocks []uint64
		err := db.Get().Model(&models.Block{}).
			Where("height >= ? AND height < ?", height, endHeight).
			Pluck("height", &existingBlocks).Error
		if err != nil {
			global.Logger.Warnf("failed to query blocks in range [%d, %d): %v", height, endHeight, err)
			continue
		}

		// Create a map of existing blocks
		existingMap := make(map[uint64]bool)
		for _, h := range existingBlocks {
			existingMap[h] = true
		}

		// Find first missing block
		for h := height; h < endHeight; h++ {
			if !existingMap[h] {
				return h
			}
		}
	}

	return 0 // No missing blocks found
}

// getHistoricalSyncHeight returns the current historical sync height.
func (s *SyncService) getHistoricalSyncHeight() uint64 {
	s.historicalSyncMutex.RLock()
	defer s.historicalSyncMutex.RUnlock()
	return s.historicalSyncHeight
}

// setHistoricalSyncHeight sets the current historical sync height.
func (s *SyncService) setHistoricalSyncHeight(height uint64) {
	s.historicalSyncMutex.Lock()
	defer s.historicalSyncMutex.Unlock()
	s.historicalSyncHeight = height
}

// recoverProcessingQueue recovers transaction hashes from the processing queue
// and puts them back to the main queue. This is called on startup to handle
// transactions that were being processed when the service crashed.
func (s *SyncService) recoverProcessingQueue() error {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return errors.New("redis client is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get all transaction hashes from processing queue
	txHashes, err := redisClient.LRange(ctx, s.config.TxHashProcessingQueueKey, 0, -1).Result()
	if err != nil {
		return errors.Wrap(err, "failed to get processing queue")
	}

	if len(txHashes) == 0 {
		return nil
	}

	global.Logger.Infof("recovering %d transaction hashes from processing queue", len(txHashes))

	// Put them back to the main queue
	for _, txHash := range txHashes {
		err = redisClient.LPush(ctx, s.config.TxHashQueueKey, txHash).Err()
		if err != nil {
			global.Logger.Warnf("failed to recover transaction hash %s: %v", txHash, err)
			continue
		}
		// Remove from processing queue
		redisClient.LRem(ctx, s.config.TxHashProcessingQueueKey, 0, txHash)
	}

	global.Logger.Infof("recovered %d transaction hashes from processing queue", len(txHashes))
	return nil
}
