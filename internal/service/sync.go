// Package service provides blockchain synchronization services.
// It uses Tendermint RPC client subscription to receive new blocks, HTTP polling for
// historical data, batch processing for efficiency, and updates account information.
package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/internal/utils"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"

	"github.com/InjectiveLabs/sdk-go/chain/types"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	tmclient "github.com/InjectiveLabs/sdk-go/client/tm"
	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/bytedance/sonic"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	github_com_cosmos_cosmos_sdk_types "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/cosmos-sdk/types/tx"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// processResult collects worker processing results.
type processResult struct {
	height   uint64
	err      error
	duration time.Duration
}

// validatorQueryClientImpl implements models.ValidatorQueryClient interface
type validatorQueryClientImpl struct {
	chainClient chainclient.ChainClientV2
}

// GetValidators queries all validators from chain
func (v *validatorQueryClientImpl) GetValidators(ctx interface{}) ([]stakingtypes.Validator, error) {
	stakingClient := stakingtypes.NewQueryClient(v.chainClient.QueryClient())
	validatorsResp, err := stakingClient.Validators(context.Background(), &stakingtypes.QueryValidatorsRequest{
		Pagination: &query.PageRequest{
			Limit: MaxValidatorQueryLimit,
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to query validators from chain")
	}
	return validatorsResp.Validators, nil
}

// GetValidatorByConsensusAddress queries a single validator by consensus address.
// This is an optimized method that attempts to find the validator without fetching all validators.
// Since Cosmos SDK staking module doesn't provide a direct query by consensus address,
// this method returns an error to indicate fallback to GetValidators is needed.
func (v *validatorQueryClientImpl) GetValidatorByConsensusAddress(ctx context.Context, consAddr string) (*stakingtypes.Validator, error) {
	// Cosmos SDK staking module doesn't provide a direct query by consensus address.
	// We return an error to indicate that the caller should fall back to GetValidators.
	// The BlockSignerService will handle this fallback automatically.
	return nil, fmt.Errorf("direct query by consensus address not supported, fallback to GetValidators")
}

// blockCacheEntry stores block data with expiration time for cache management
type blockCacheEntry struct {
	blockResp *coretypes.ResultBlock
	expiresAt time.Time
}

// SyncService handles blockchain synchronization tasks using WebSocket streaming.
// It receives new blocks in real-time via gRPC streaming, stores block data,
// processes transactions, and updates account information.
type SyncService struct {
	// Configuration
	config *SyncConfig

	// Bootstrap configuration (for EVM details sync)
	bc *conf.Bootstrap

	// Clients for blockchain interactions
	// explorerClient explorerclient.ExplorerClient
	chainClient chainclient.ChainClientV2
	tmClient    tmclient.TendermintClient

	rpcClient *rpchttp.HTTP

	// EVM details sync service
	evmDetailsSyncService *EVMDetailsSyncService

	// Block signer service for processing block signers
	blockSignerService *models.BlockSignerService

	// Block cache for tx_number calculation (tx_number = tx_index + 1, per-block)
	// Maps block height -> blockResp for authoritative tx_index calculation
	blockCache sync.Map // map[uint64]*blockCacheEntry

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

	// Distributed lock for sync service (ensures only one instance runs sync)
	syncLockKey     string
	syncLockValue   string
	syncLockRenewal *sync.WaitGroup

	// Monitoring metrics
	metrics struct {
		blockProcessingCount    atomic.Uint64
		blockProcessingErrors   atomic.Uint64
		totalProcessingDuration atomic.Int64 // nanoseconds
	}
}

// NewSyncService creates a new instance of SyncService.
//
// Parameters:
//   - config: Sync service configuration (can be nil to use defaults)
//   - bc: Bootstrap configuration (can be nil)
//
// Returns:
//   - *SyncService: A new sync service instance
func NewSyncService(config *SyncConfig, bc *conf.Bootstrap) *SyncService {
	if config == nil {
		config = LoadSyncConfig(nil)
	}
	ctx, cancel := context.WithCancel(context.Background())

	// Generate unique lock value for this instance
	hostname, _ := os.Hostname()
	lockValue := fmt.Sprintf("%s-%d-%d", hostname, os.Getpid(), time.Now().Unix())

	return &SyncService{
		config:          config,
		bc:              bc,
		ctx:             ctx,
		cancel:          cancel,
		syncLockKey:     "sync:lock:blockchain",
		syncLockValue:   lockValue,
		syncLockRenewal: &sync.WaitGroup{},
	}
}

// TryAcquireSyncLock attempts to acquire a distributed lock for sync service.
// This ensures only one instance runs sync service in a horizontal scaling setup.
//
// Parameters:
//   - ctx: Context for the operation
//
// Returns:
//   - bool: true if lock was acquired, false if already held by another instance
//   - error: Error if the lock check fails
func (s *SyncService) TryAcquireSyncLock(ctx context.Context) (bool, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		// If Redis is not available, allow sync to start (graceful degradation)
		// In production, you might want to return false to prevent sync from starting
		global.Logger.Warnf("Redis client not available, cannot acquire sync lock, allowing sync to start")
		return true, nil
	}

	lockTTL := SyncLockTTL

	// Try to acquire lock using SETNX (SET if Not eXists)
	acquired, err := redisClient.SetNX(ctx, s.syncLockKey, s.syncLockValue, lockTTL).Result()
	if err != nil {
		return false, errors.Wrap(err, "failed to acquire sync lock")
	}

	if acquired {
		global.Logger.Infof("Sync service lock acquired: %s", s.syncLockValue)
		// Start lock renewal goroutine
		s.syncLockRenewal.Add(1)
		go s.renewSyncLock(ctx, lockTTL)
		return true, nil
	}

	// Lock already held by another instance
	// Check if it's still valid (not expired)
	currentValue, err := redisClient.Get(ctx, s.syncLockKey).Result()
	if err == redis.Nil {
		// Lock expired, try to acquire again
		acquired, err := redisClient.SetNX(ctx, s.syncLockKey, s.syncLockValue, lockTTL).Result()
		if err != nil {
			return false, errors.Wrap(err, "failed to acquire sync lock after expiration check")
		}
		if acquired {
			global.Logger.Infof("Sync service lock acquired after expiration: %s", s.syncLockValue)
			s.syncLockRenewal.Add(1)
			go s.renewSyncLock(ctx, lockTTL)
			return true, nil
		}
	} else if err != nil {
		return false, errors.Wrap(err, "failed to check sync lock status")
	}

	global.Logger.Infof("Sync service lock already held by another instance: %s", currentValue)
	return false, nil
}

// renewSyncLock periodically renews the sync service lock to prevent expiration.
// This ensures the lock remains valid as long as this instance is running.
//
// Parameters:
//   - ctx: Context for the operation
//   - ttl: Time-to-live for the lock
func (s *SyncService) renewSyncLock(ctx context.Context, ttl time.Duration) {
	defer s.syncLockRenewal.Done()

	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	// Renew lock every TTL/2 to ensure it doesn't expire
	ticker := time.NewTicker(ttl / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Release lock on shutdown
			if err := redisClient.Del(ctx, s.syncLockKey).Err(); err != nil {
				global.Logger.Warnf("Failed to release sync lock on shutdown: %v", err)
			} else {
				global.Logger.Infof("Sync service lock released on shutdown")
			}
			return
		case <-ticker.C:
			// Check if lock still belongs to this instance
			currentValue, err := redisClient.Get(ctx, s.syncLockKey).Result()
			if err == redis.Nil {
				// Lock expired, try to reacquire
				acquired, err := redisClient.SetNX(ctx, s.syncLockKey, s.syncLockValue, ttl).Result()
				if err != nil {
					global.Logger.Warnf("Failed to reacquire sync lock: %v", err)
					return
				}
				if !acquired {
					// Lock acquired by another instance, stop renewal
					global.Logger.Warnf("Sync lock acquired by another instance, stopping renewal")
					return
				}
				global.Logger.Debugf("Sync lock reacquired after expiration")
			} else if err != nil {
				global.Logger.Warnf("Failed to check sync lock during renewal: %v", err)
				return
			} else if currentValue != s.syncLockValue {
				// Lock held by another instance, stop renewal
				global.Logger.Warnf("Sync lock held by another instance (%s), stopping renewal", currentValue)
				return
			}

			// Renew lock
			if err := redisClient.Expire(ctx, s.syncLockKey, ttl).Err(); err != nil {
				global.Logger.Warnf("Failed to renew sync lock: %v", err)
				return
			}
			global.Logger.Debugf("Sync lock renewed successfully")
		}
	}
}

// Start starts the blockchain synchronization service.
// It starts goroutines for WebSocket block streaming and transaction processing.
// The initialization operations are performed asynchronously
// to avoid blocking service startup.
func (s *SyncService) Start() error {
	global.Logger.Infof("starting blockchain sync service")

	// Initialize clients
	if err := s.initClients(); err != nil {
		return errors.Wrap(err, "failed to initialize clients")
	}

	// Set up block signer service with dependency injection
	validatorQueryClient := &validatorQueryClientImpl{chainClient: s.chainClient}
	s.blockSignerService = models.NewBlockSignerService(validatorQueryClient)
	// Set default service for backward compatibility (deprecated)
	models.SetDefaultBlockSignerService(s.blockSignerService)

	// Start initialization in background goroutine
	// This allows the service to start immediately without waiting for initialization to complete
	s.wg.Add(1)
	go s.initializeAndSync()

	// Start block streaming (real-time) via WebSocket
	// This is the only method for block synchronization
	s.wg.Add(1)
	go s.tmStreamBlocks(s.ctx)

	// Start validator sync (initial sync)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.getAllValidators(s.ctx); err != nil {
			global.Logger.Errorf("failed to sync validators: %v", err)
		}
	}()

	// Start periodic validator update (every 10 minutes)
	s.wg.Add(1)
	go s.periodicValidatorUpdate(s.ctx)

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

	// Start EVM details sync service (only if EVM is enabled in config)
	if s.bc != nil && s.bc.Chain != nil && s.bc.Chain.Evm != nil && s.bc.Chain.Evm.Enabled {
		evmConfig := LoadEVMDetailsSyncConfig(nil)
		evmConfig.QueueKey = s.config.EVMDetailsQueueKey
		evmDetailsSyncService, err := NewEVMDetailsSyncService(s.ctx, evmConfig, s.bc)
		if err != nil {
			global.Logger.Warnf("failed to create EVM details sync service: %v", err)
			// Continue without EVM details sync
		} else {
			s.evmDetailsSyncService = evmDetailsSyncService
			evmDetailsSyncService.Start()
			global.Logger.Infof("EVM details sync service started (enabled)")
		}
	} else {
		global.Logger.Infof("EVM details sync service is disabled (config: evm.enabled = false)")
	}

	global.Logger.Infof("blockchain sync service started, initialization in progress, mode=websocket streaming, %d transaction processors",
		s.config.ConcurrentTxProcessors)
	return nil
}

// initializeAndSync performs initialization operations asynchronously.
// This allows the service to start immediately while initialization happens in the background.
// Note: Backward sync has been removed - only WebSocket streaming is used for block synchronization.
func (s *SyncService) initializeAndSync() {
	defer s.wg.Done()

	global.Logger.Infof("starting async initialization")

	// Initialize current height from database
	// We only initialize the height, no backward sync - WebSocket streaming will handle new blocks
	if err := s.initCurrentHeightWithVerification(); err != nil {
		global.Logger.Warnf("failed to initialize current height with verification: %v, starting from latest", err)
		// If failed, start from latest block
		latestHeight, err := s.getLatestBlockHeight()
		if err != nil {
			global.Logger.Errorf("failed to get latest block height: %v", err)
			return
		}
		s.setCurrentHeight(latestHeight)
		global.Logger.Infof("initialized current height to latest: %d", latestHeight)
	}

	global.Logger.Infof("async initialization completed, current height: %d", s.getCurrentHeight())
}

// Stop stops the blockchain synchronization service.
// It cancels the context and waits for all goroutines to finish with a timeout.
func (s *SyncService) Stop() error {
	global.Logger.Infof("stopping blockchain sync service")
	s.cancel()

	// Stop EVM details sync service
	if s.evmDetailsSyncService != nil {
		s.evmDetailsSyncService.Stop()
	}

	// Wait for lock renewal goroutine to finish
	s.syncLockRenewal.Wait()

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	if s.rpcClient != nil {
		s.rpcClient.Stop()
	}

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
	// Initialize chain client (shared)
	chainClient, err := GetSharedChainClient()
	if err != nil {
		return err
	}
	s.chainClient = chainClient

	// Initialize Tendermint client (shared)
	s.tmClient = GetSharedTmClient()

	// Create RPC client with extended timeout for slow endpoints
	// Using 30 second timeout to handle high latency RPC nodes
	rpcClient, err := rpchttp.NewWithTimeout(global.ChainConfig.TmEndpoint, 30)
	if err != nil {
		return errors.Wrap(err, "failed to create RPC client")
	}
	s.rpcClient = rpcClient

	err = rpcClient.Start()
	if err != nil {
		return errors.Wrap(err, "failed to start RPC client")
	}

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

	blockResp, err := s.tmProcessBlock(ctx, int64(maxHeight))
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

	if blockResp == nil {
		global.Logger.Warnf("block response is nil at height %d, starting from latest", maxHeight)
		latestHeight, err := s.getLatestBlockHeight()
		if err != nil {
			return errors.Wrap(err, "failed to get latest block height")
		}
		s.setCurrentHeight(latestHeight)
		return nil
	}

	chainHash := fmt.Sprintf("0x%s", strings.ToLower(blockResp.BlockID.Hash.String()))
	// Compare block hashes
	if maxBlock.BlockHash != chainHash {
		// Block hash doesn't match, this might be a different chain
		global.Logger.Warnf("block hash mismatch at height %d: database=%s, chain=%s, starting from latest",
			maxHeight, maxBlock.BlockHash, chainHash)

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
	if startHeight > CommonBlockSearchBackwardLimit {
		startHeight = maxHeight - CommonBlockSearchBackwardLimit
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
		blockResp, err := s.tmProcessBlock(ctx, int64(height))
		if err != nil {
			continue
		}

		if blockResp == nil {
			continue
		}

		chainHash := fmt.Sprintf("0x%s", strings.ToLower(blockResp.BlockID.Hash.String()))

		// If hash matches, this is a common block
		if dbBlock.BlockHash == chainHash {
			return height, nil
		}
	}

	// No common block found, return 0 to start from beginning
	return 0, errors.New("no common block found")
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

// batchCheckBlockExistence checks if blocks exist in batch.
func (s *SyncService) batchCheckBlockExistence(heights []uint64) (map[uint64]bool, error) {
	result := make(map[uint64]bool)

	if len(heights) == 0 {
		return result, nil
	}

	start := time.Now()

	// Use IN query for batch check
	var existingBlocks []struct {
		Height uint64 `gorm:"column:height"`
	}

	// Batch query to avoid too many SQL parameters (1000 per batch)
	batchSize := HeightQueryBatchSize
	for i := 0; i < len(heights); i += batchSize {
		end := i + batchSize
		if end > len(heights) {
			end = len(heights)
		}

		batch := heights[i:end]
		err := db.Get().Model(&models.Block{}).
			Where("height IN ?", batch).
			Select("height").
			Find(&existingBlocks).Error

		if err != nil {
			return nil, errors.Wrap(err, "failed to batch check block existence")
		}
	}

	// Build result map
	for _, block := range existingBlocks {
		result[block.Height] = true
	}

	duration := time.Since(start)
	global.Logger.Debugf("batch checked %d blocks, found %d existing, took %v",
		len(heights), len(existingBlocks), duration)

	return result, nil
}

// blockWorker processes blocks in worker goroutines.
func (s *SyncService) blockWorker(id int, taskChan <-chan uint64, resultChan chan<- processResult, wg *sync.WaitGroup) {
	defer wg.Done()

	global.Logger.Debugf("block worker %d started", id)
	processedCount := 0

	for height := range taskChan {
		select {
		case <-s.ctx.Done():
			global.Logger.Debugf("block worker %d stopped by context, processed: %d", id, processedCount)
			return
		default:
			start := time.Now()
			err := s.processBlock(height)
			duration := time.Since(start)

			resultChan <- processResult{
				height:   height,
				err:      err,
				duration: duration,
			}

			processedCount++

			// Print progress every N blocks processed
			if processedCount%ProgressLogInterval == 0 {
				global.Logger.Debugf("block worker %d processed %d blocks", id, processedCount)
			}
		}
	}

	global.Logger.Debugf("block worker %d finished, total processed: %d", id, processedCount)
}

// processBlock processes a single block at the given height.
func (s *SyncService) processBlock(height uint64) error {
	ctx, cancel := context.WithTimeout(s.ctx, s.config.BlockQueryTimeout)
	defer cancel()

	// Get block information with retry logic
	var blockResp *coretypes.ResultBlock

	delay := s.config.RetryDelay
	var err error
	for attempt := 0; attempt < s.config.MaxRetryAttempts; attempt++ {
		blockResp, err = s.tmProcessBlock(ctx, int64(height))
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
		if utils.IsRetryableError(err) {
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

	if blockResp == nil {
		return errors.Errorf("block response is nil for height %d", height)
	}

	// Save block to database using transaction for atomicity
	blockModel := models.BlockFromTM(blockResp)
	if blockModel == nil {
		return errors.Errorf("failed to convert block at height %d", height)
	}

	// Use transaction to ensure atomicity with upsert for reduced database round trips
	var isUpdate bool
	err = db.Get().Transaction(func(tx *gorm.DB) error {
		// Check if block already exists (for event type determination)
		var existingBlock models.Block
		checkErr := tx.Where("height = ?", height).First(&existingBlock).Error
		isUpdate = (checkErr == nil)

		// Use upsert to reduce database round trips from 2 to 1
		result := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "height"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"proposer", "moniker", "block_hash", "parent_hash",
				"num_precommits", "num_txs", "total_txs", "txs",
				"timestamp", "block_unix_timestamp", "updated_at",
			}),
		}).Create(blockModel)

		if result.Error != nil {
			return errors.Wrapf(result.Error, "failed to upsert block at height %d", height)
		}

		// Extract transaction hashes and push to Redis queue
		// Note: Redis operations are outside the database transaction
		// If Redis push fails, the block is still saved (eventual consistency)
		if len(blockResp.Block.Data.Txs) > 0 {
			redisClient := cache.GetRedisClient()
			if redisClient != nil {
				// Use Pipeline for batch operations to reduce network round trips
				pipe := redisClient.Pipeline()
				for _, tx := range blockResp.Block.Data.Txs {
					pipe.LPush(ctx, s.config.TxHashQueueKey, hex.EncodeToString(tx.Hash()))
				}
				_, err := pipe.Exec(ctx)
				if err != nil {
					global.Logger.Warnf("failed to batch push tx hashes to queue: %v", err)
					// Continue even if Redis push fails (eventual consistency)
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

	// Publish block event for streaming
	eventBus := GetEventBus()
	eventType := EventTypeBlockNew
	if isUpdate {
		eventType = EventTypeBlockUpdate
	}
	eventBus.PublishBlock(&BlockEvent{
		Block: blockModel,
		Type:  eventType,
	})

	// Cache block to Redis
	cacheBlock(ctx, blockModel)

	// Cache blockResp for tx_number calculation (tx_number = tx_index + 1, per-block)
	// Cache expires after 1 hour to prevent memory bloat
	s.cacheBlockResp(height, blockResp)

	return nil
}

// cacheBlockResp caches blockResp for tx_number calculation (tx_number = tx_index + 1, per-block).
// The cache expires after 1 hour to prevent memory bloat.
func (s *SyncService) cacheBlockResp(height uint64, blockResp *coretypes.ResultBlock) {
	if blockResp == nil {
		return
	}
	entry := &blockCacheEntry{
		blockResp: blockResp,
		expiresAt: time.Now().Add(1 * time.Hour),
	}
	s.blockCache.Store(height, entry)
}

// getCachedBlockResp retrieves cached blockResp for tx_number calculation (tx_number = tx_index + 1, per-block).
// Returns nil if not found or expired.
func (s *SyncService) getCachedBlockResp(height uint64) *coretypes.ResultBlock {
	value, ok := s.blockCache.Load(height)
	if !ok {
		return nil
	}
	entry, ok := value.(*blockCacheEntry)
	if !ok {
		return nil
	}
	// Check if expired
	if time.Now().After(entry.expiresAt) {
		s.blockCache.Delete(height)
		return nil
	}
	return entry.blockResp
}

// cleanupExpiredBlockCache removes expired entries from block cache.
// Should be called periodically to prevent memory bloat.
func (s *SyncService) cleanupExpiredBlockCache() {
	now := time.Now()
	s.blockCache.Range(func(key, value interface{}) bool {
		entry, ok := value.(*blockCacheEntry)
		if !ok {
			s.blockCache.Delete(key)
			return true
		}
		if now.After(entry.expiresAt) {
			s.blockCache.Delete(key)
		}
		return true
	})
}

// processBlockOptimized is an optimized version of processBlock for backward sync.
// It skips unnecessary checks and uses batch operations where possible.
func (s *SyncService) processBlockOptimized(height uint64) error {
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

	return s.processBlockOptimizedSkipCheck(height)
}

// processBlocksBatch processes multiple blocks in a batch for better performance.
// This reduces database transaction overhead by batching inserts.
//
// Parameters:
//   - heights: List of block heights to process
//   - batchSize: Number of blocks to process in each database batch
//
// Returns:
//   - error: Error if batch processing fails
func (s *SyncService) processBlocksBatch(heights []uint64, batchSize int) error {
	if len(heights) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(s.ctx, s.config.BlockQueryTimeout*time.Duration(len(heights)))
	defer cancel()

	// Fetch all blocks concurrently
	type blockResult struct {
		height    uint64
		block     *models.Block
		blockResp *coretypes.ResultBlock // Store blockResp for saving signers
		err       error
	}

	results := make(chan blockResult, len(heights))
	var wg sync.WaitGroup

	// Fetch blocks concurrently (limit concurrency to avoid overwhelming the API)
	// Use configurable concurrency based on backward sync workers, but cap at reasonable limit
	maxConcurrency := s.config.BackwardSyncWorkers
	if maxConcurrency <= 0 {
		maxConcurrency = DefaultMaxConcurrency // Default
	}
	if maxConcurrency > 20 {
		maxConcurrency = 20 // Cap at 20 to avoid overwhelming the API
	}
	sem := make(chan struct{}, maxConcurrency)

	for _, height := range heights {
		wg.Add(1)
		go func(h uint64) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			blockResp, err := s.tmProcessBlock(ctx, int64(h))
			if err != nil {
				results <- blockResult{height: h, err: err}
				return
			}

			if blockResp == nil {
				results <- blockResult{height: h, err: errors.Errorf("block response is nil for height %d", h)}
				return
			}

			// Use object pool to reduce allocations
			poolBlock := GetBlockFromPool()
			blockModel := models.BlockFromTMWithPool(blockResp, poolBlock)
			if blockModel == nil {
				PutBlockToPool(poolBlock) // Return to pool if conversion failed
				results <- blockResult{height: h, err: errors.Errorf("failed to convert block at height %d", h)}
				return
			}
			// Note: blockModel will be returned to pool after use in the caller

			results <- blockResult{height: h, block: blockModel, blockResp: blockResp}
			// Note: blockModel came from pool, will be returned after use
		}(height)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results first
	type blockData struct {
		height    uint64
		block     *models.Block
		blockResp *coretypes.ResultBlock // Store blockResp for saving signers
		err       error
		fromPool  bool // Track if block came from pool
	}
	// Pre-allocate slice with known capacity to avoid reallocation
	collectedBlocks := make([]blockData, 0, len(heights))
	for result := range results {
		collectedBlocks = append(collectedBlocks, blockData{
			height:    result.height,
			block:     result.block,
			blockResp: result.blockResp,
			err:       result.err,
			fromPool:  true, // All blocks in processBlocksBatch come from pool
		})
	}

	// Batch check which blocks already exist in database
	// This reduces database queries from N queries to 1 query
	// Pre-allocate map with estimated size to avoid reallocation
	existingHeights := make(map[uint64]uint, len(collectedBlocks))
	if len(collectedBlocks) > 0 {
		heightsToCheck := make([]uint64, 0, len(collectedBlocks))
		for _, bd := range collectedBlocks {
			if bd.err == nil && bd.block != nil {
				heightsToCheck = append(heightsToCheck, bd.height)
			}
		}

		if len(heightsToCheck) > 0 {
			// Batch query: get all existing block heights in one query
			var existingBlocks []models.Block
			err := db.Get().Select("id, height").Where("height IN ?", heightsToCheck).Find(&existingBlocks).Error
			if err == nil {
				for _, eb := range existingBlocks {
					existingHeights[eb.Height] = uint(eb.ID)
				}
			} else {
				global.Logger.Warnf("failed to batch check block existence: %v", err)
				// Fallback to individual checks if batch query fails
			}
		}
	}

	// Separate blocks into insert and update lists based on batch check results
	blocksToInsert := make([]*models.Block, 0, len(collectedBlocks))
	blocksToUpdate := make([]*models.Block, 0, len(collectedBlocks))
	processingErrors := make([]error, 0)

	for _, bd := range collectedBlocks {
		if bd.err != nil {
			processingErrors = append(processingErrors, bd.err)
			continue
		}
		if bd.block == nil {
			continue
		}

		// Check if block exists using batch check result
		if blockID, exists := existingHeights[bd.height]; exists {
			// Block exists, prepare for update
			bd.block.ID = blockID
			blocksToUpdate = append(blocksToUpdate, bd.block)
		} else {
			// Block doesn't exist, prepare for insert
			blocksToInsert = append(blocksToInsert, bd.block)
		}
	}

	// Unified batch upsert - merge all blocks and process in a single operation
	allBlocks := append(blocksToInsert, blocksToUpdate...)
	if len(allBlocks) > 0 {
		err := db.Get().Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "height"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"proposer", "moniker", "block_hash", "parent_hash",
				"num_precommits", "num_txs", "total_txs", "txs",
				"timestamp", "block_unix_timestamp", "updated_at",
			}),
		}).CreateInBatches(allBlocks, batchSize).Error

		if err != nil {
			return errors.Wrapf(err, "failed to batch upsert %d blocks", len(allBlocks))
		}
		global.Logger.Debugf("batch upserted %d blocks", len(allBlocks))
	}

	// Publish events and cache blocks
	eventBus := GetEventBus()

	// Track blocks from pool for cleanup
	// Note: We need to track which blocks came from pool to return them later
	// For now, we'll return all blocks to pool after processing
	poolBlocksToReturn := make([]*models.Block, 0, len(blocksToInsert)+len(blocksToUpdate))

	for _, block := range blocksToInsert {
		eventBus.PublishBlock(&BlockEvent{
			Block: block,
			Type:  EventTypeBlockNew,
		})
		cacheBlock(ctx, block)
		poolBlocksToReturn = append(poolBlocksToReturn, block)
	}

	for _, block := range blocksToUpdate {
		eventBus.PublishBlock(&BlockEvent{
			Block: block,
			Type:  EventTypeBlockUpdate,
		})
		cacheBlock(ctx, block)
		poolBlocksToReturn = append(poolBlocksToReturn, block)
	}

	// Return blocks to pool after use (async to avoid blocking)
	// Note: This assumes all blocks came from pool, which is true in processBlocksBatch
	go func() {
		for _, block := range poolBlocksToReturn {
			PutBlockToPool(block)
		}
	}()

	// Save block signers information asynchronously (proposer and signers)
	// This is done in background to avoid blocking batch processing
	go func() {
		// Capture service reference for goroutine
		service := s.blockSignerService
		if service == nil {
			global.Logger.Warnf("block signer service not initialized, using default")
			service = models.NewBlockSignerService(&validatorQueryClientImpl{chainClient: s.chainClient})
		}
		for _, bd := range collectedBlocks {
			if bd.err == nil && bd.blockResp != nil {
				if err := service.SaveBlockSigners(bd.blockResp); err != nil {
					global.Logger.Warnf("failed to save block signers for height %d: %v", bd.height, err)
					// Don't fail batch processing if signer saving fails
				}
			}
		}
	}()

	// Note: Transaction hashes are stored in the block's Txs field, but we need to fetch
	// the actual transaction hashes from the proto data. For now, we'll handle this
	// in the individual block processing path. This can be optimized further by
	// storing transaction hashes during batch fetch.

	// Log errors but don't fail the entire batch
	if len(processingErrors) > 0 {
		global.Logger.Warnf("encountered %d errors during batch processing: %v", len(processingErrors), processingErrors)
	}

	return nil
}

// processBlockOptimizedSkipCheck processes a block without checking existence first.
// This is used when we've already batch-checked block existence.
func (s *SyncService) processBlockOptimizedSkipCheck(height uint64) error {
	ctx, cancel := context.WithTimeout(s.ctx, s.config.BlockQueryTimeout)
	defer cancel()

	// Get block information with retry logic
	var blockResp *coretypes.ResultBlock

	delay := s.config.RetryDelay
	var err error
	for attempt := 0; attempt < s.config.MaxRetryAttempts; attempt++ {
		blockResp, err = s.tmProcessBlock(ctx, int64(height))
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
		if utils.IsRetryableError(err) {
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

	if blockResp == nil {
		return errors.Errorf("block response is nil for height %d", height)
	}

	// Save block to database using transaction for atomicity
	blockModel := models.BlockFromTM(blockResp)
	if blockModel == nil {
		return errors.Errorf("failed to convert block at height %d", height)
	}

	// Use transaction to ensure atomicity with upsert for reduced database round trips
	var isUpdate bool
	err = db.Get().Transaction(func(tx *gorm.DB) error {
		// Check if block already exists (for event type determination)
		var existingBlock models.Block
		checkErr := tx.Where("height = ?", height).First(&existingBlock).Error
		isUpdate = (checkErr == nil)

		// Use upsert to reduce database round trips from 2 to 1
		result := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "height"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"proposer", "moniker", "block_hash", "parent_hash",
				"num_precommits", "num_txs", "total_txs", "txs",
				"timestamp", "block_unix_timestamp", "updated_at",
			}),
		}).Create(blockModel)

		if result.Error != nil {
			return errors.Wrapf(result.Error, "failed to upsert block at height %d", height)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Publish block event for streaming
	eventBus := GetEventBus()
	eventType := EventTypeBlockNew
	if isUpdate {
		eventType = EventTypeBlockUpdate
	}
	eventBus.PublishBlock(&BlockEvent{
		Block: blockModel,
		Type:  eventType,
	})

	// Extract transaction hashes and push to Redis queue (batch push for better performance)
	if len(blockResp.Block.Data.Txs) > 0 {
		redisClient := cache.GetRedisClient()
		if redisClient != nil {
			// Batch push to Redis for better performance
			txHashes := make([]string, 0, len(blockResp.Block.Data.Txs))
			for _, tx := range blockResp.Block.Data.Txs {
				txHashes = append(txHashes, hex.EncodeToString(tx.Hash()))
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

	// Cache block to Redis
	cacheBlock(ctx, blockModel)

	// Save block signers information (proposer and signers)
	// This is done asynchronously to avoid blocking block processing
	go func() {
		// Capture service reference for goroutine
		service := s.blockSignerService
		if service == nil {
			global.Logger.Warnf("block signer service not initialized, using default")
			service = models.NewBlockSignerService(&validatorQueryClientImpl{chainClient: s.chainClient})
		}
		if err := service.SaveBlockSigners(blockResp); err != nil {
			global.Logger.Warnf("failed to save block signers for height %d: %v", height, err)
			// Don't fail block processing if signer saving fails
		}
	}()

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
			// Process transactions in batch for better performance
			batchProcessed, err := s.processTransactionBatch(redisClient)
			if err != nil {
				if err == errQueueEmpty {
					// Queue is empty, this is normal
					emptyCount++
					// If queue is empty, wait a bit before checking again
					time.Sleep(s.config.TxProcessInterval)
				} else {
					// Real error occurred
					errorCount++
					global.Logger.Errorf("transaction processor %d: error processing transaction batch (error count: %d): %v", id, errorCount, err)
					// Wait a bit before retrying to avoid hammering on errors
					time.Sleep(s.config.TxProcessInterval)
				}
			} else {
				// Successfully processed batch
				processedCount += batchProcessed
			}
		}
	}
}

var errQueueEmpty = errors.New("queue is empty")

// processTransactionBatch processes a batch of transactions from the Redis queue for better performance.
// This matches the batch processing approach used for blocks to maintain consistent sync speed.
// Uses a detached context to allow batch operations to complete even during graceful shutdown.
func (s *SyncService) processTransactionBatch(redisClient *redis.Client) (int, error) {
	// Use context.Background() as parent to allow batch operations to complete
	// even if the service is shutting down (graceful shutdown with in-flight operations)
	ctx, cancel := context.WithTimeout(context.Background(), s.config.BlockQueryTimeout)
	defer cancel()

	// Collect a batch of transaction hashes from the queue
	batchSize := s.config.TxBatchSize
	if batchSize <= 0 {
		batchSize = 50 // Default batch size
	}

	txHashes := make([]string, 0, batchSize)
	blockTimeout := s.config.TxProcessInterval
	if blockTimeout < time.Second {
		blockTimeout = time.Second
	}

	// Collect transaction hashes from queue (non-blocking, collect what's available)
	// Note: Redis BRPopLPush requires minimum timeout of 1 second
	redisTimeout := blockTimeout
	if redisTimeout < time.Second {
		redisTimeout = time.Second
	}
	for i := 0; i < batchSize; i++ {
		txHash, err := redisClient.BRPopLPush(ctx, s.config.TxHashQueueKey, s.config.TxHashProcessingQueueKey, redisTimeout).Result()
		if err != nil {
			if err == redis.Nil || err.Error() == "redis: nil" || strings.Contains(err.Error(), "redis: nil") {
				// Queue is empty or timeout, break and process what we have
				break
			}
			// Real error, return it
			if len(txHashes) == 0 {
				return 0, errors.Wrap(err, "failed to pop from tx hash queue")
			}
			// If we have some hashes, continue processing them
			global.Logger.Warnf("error popping tx hash (collected %d): %v, continuing with batch", len(txHashes), err)
			break
		}
		txHashes = append(txHashes, txHash)
	}

	if len(txHashes) == 0 {
		return 0, errQueueEmpty
	}

	global.Logger.Debugf("processing transaction batch: size=%d", len(txHashes))

	// Process batch: get transaction details, convert to models, and batch insert/update
	successCount, err := s.processTransactionBatchInternal(ctx, redisClient, txHashes)
	if err != nil {
		// Put failed hashes back to queue
		pipe := redisClient.Pipeline()
		for _, hash := range txHashes {
			pipe.LPush(ctx, s.config.TxHashQueueKey, hash)
			pipe.LRem(ctx, s.config.TxHashProcessingQueueKey, 0, hash)
		}
		pipe.Exec(ctx) // Ignore errors for cleanup
		return successCount, err
	}

	return successCount, nil
}

// processTransactionBatchInternal processes a batch of transaction hashes.
// It fetches transaction details concurrently, converts them to models, and batch inserts/updates them.
func (s *SyncService) processTransactionBatchInternal(ctx context.Context, redisClient *redis.Client, txHashes []string) (int, error) {
	storageThreshold := getStorageThresholdForSync()

	// Step 1: Fetch transaction details concurrently
	type txResult struct {
		hash    string
		txResp  *tx.GetTxResponse
		err     error
		retries int
	}

	txResults := make([]txResult, len(txHashes))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, MaxConcurrentRPCCalls) // Limit concurrent RPC calls

	for i, txHash := range txHashes {
		wg.Add(1)
		go func(idx int, hash string) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			result := txResult{hash: hash}
			delay := s.config.RetryDelay

			for attempt := 0; attempt < s.config.MaxRetryAttempts; attempt++ {
				result.txResp, result.err = s.chainClient.GetTx(ctx, hash)
				if result.err == nil {
					break
				}

				// Check if error is due to context cancellation or timeout
				errStr := result.err.Error()
				if errors.Is(result.err, context.Canceled) || errors.Is(result.err, context.DeadlineExceeded) ||
					strings.Contains(errStr, "context canceled") || strings.Contains(errStr, "context deadline exceeded") {
					result.retries = attempt + 1
					return
				}

				// Check if error is retryable
				if utils.IsRetryableError(result.err) {
					if attempt < s.config.MaxRetryAttempts-1 {
						time.Sleep(delay)
						delay *= 2 // Exponential backoff
						result.retries = attempt + 1
						continue
					}
				}

				// Non-retryable error or max retries reached
				result.retries = attempt + 1
				return
			}

			txResults[idx] = result
		}(i, txHash)
	}

	wg.Wait()

	// Step 2: Convert successful responses to models
	type txModelData struct {
		hash    string
		txModel *models.Transaction
		txData  *tx.GetTxResponse
		err     error
	}

	txModels := make([]txModelData, 0, len(txHashes))

	// Track failure reasons for better diagnostics
	var fetchErrors, nilResponseErrors, convertErrors []string
	for _, result := range txResults {
		if result.err != nil {
			global.Logger.Warnf("failed to get transaction %s after %d attempts: %v", result.hash, result.retries, result.err)
			fetchErrors = append(fetchErrors, result.hash)
			continue
		}

		if result.txResp == nil {
			global.Logger.Warnf("transaction response is nil for hash %s", result.hash)
			nilResponseErrors = append(nilResponseErrors, result.hash)
			continue
		}

		poolTx := GetTransactionFromPool()
		
		// Try to get blockResp from cache for authoritative tx_number calculation (tx_number = tx_index + 1, per-block)
		var blockResp *coretypes.ResultBlock
		if result.txResp != nil && result.txResp.TxResponse != nil {
			blockHeight := uint64(result.txResp.TxResponse.Height)
			blockResp = s.getCachedBlockResp(blockHeight)
			if blockResp == nil {
				// Try to fetch from chain if not in cache
				blockCtx, blockCancel := context.WithTimeout(ctx, 10*time.Second)
				var err error
				blockResp, err = s.tmProcessBlock(blockCtx, int64(blockHeight))
				blockCancel()
				if err != nil {
					global.Logger.Warnf("block %d not found in cache and failed to fetch from chain for tx %s: %v, will use legacy tx_number calculation", blockHeight, result.hash, err)
				} else if blockResp != nil {
					global.Logger.Infof("✓ fetched block %d from chain for authoritative tx_number calculation for tx %s (block has %d txs)",
						blockHeight, result.hash, len(blockResp.Block.Data.Txs))
				}
			} else {
				global.Logger.Infof("✓ using cached block %d for authoritative tx_number calculation for tx %s (block has %d txs)",
					blockHeight, result.hash, len(blockResp.Block.Data.Txs))
			}
		}
		
		txModel := models.TransactionFromGetTxResponse(result.txResp, storageThreshold, s.chainClient.ClientContext().Codec, blockResp)
		if txModel == nil {
			PutTransactionToPool(poolTx)
			global.Logger.Errorf("failed to convert transaction %s to model", result.hash)
			convertErrors = append(convertErrors, result.hash)
			continue
		}

		// Check if this is an EVM transaction and extract EVM hash (only if EVM sync is enabled)
		// Convert GetTxResponse to TxDetailData for EVM check
		// Pass blockResp for authoritative tx_number calculation (tx_number = tx_index + 1, per-block)
		if s.bc != nil && s.bc.Chain != nil && s.bc.Chain.Evm != nil && s.bc.Chain.Evm.Enabled {
			txDetailData := models.ConvertGetTxResponseToTxDetailData(result.txResp, s.chainClient.ClientContext().Codec, blockResp)
			if txDetailData != nil {
				if IsEVMTransaction(txDetailData) {
					evmHash := ExtractEVMHash(txDetailData)

					// If hash not found in explorer data, try to get from chainClient (abci.Event)
					// This is synchronous to ensure data consistency in batch processing
					if evmHash == "" && s.chainClient != nil {
						evmCtx, evmCancel := context.WithTimeout(ctx, EVMContextTimeoutSeconds*time.Second)
						txResp, err := s.chainClient.GetTx(evmCtx, txModel.Hash)
						evmCancel()

						if err == nil && txResp != nil && txResp.TxResponse != nil {
							evmHash = ExtractEVMHashFromABCIEvents(txResp.TxResponse.Events)
							if evmHash != "" {
								global.Logger.Debugf("extracted EVM hash %s from abci events for transaction %s", evmHash, txModel.Hash)
							}
						} else if err != nil {
							global.Logger.Debugf("failed to get tx from chainClient for EVM hash extraction: %v", err)
						}
					}

					if evmHash != "" {
						// Store EVM information in separate table to avoid affecting transactions table
						// This prevents signatures and signer fields from being cleared
						go func(cosmosHash, evmHash string) {
							evmCtx, evmCancel := context.WithTimeout(context.Background(), StatsContextTimeoutSeconds*time.Second)
							defer evmCancel()

							// Upsert EVM transaction (create if not exists, update evm_hash if exists)
							evmTx := &models.EVMTransaction{}
							err := db.Get().WithContext(evmCtx).
								Where("transaction_hash = ?", cosmosHash).
								First(evmTx).Error

							if err != nil {
								if errors.Is(err, gorm.ErrRecordNotFound) {
									// Create new EVM transaction record
									evmTx = &models.EVMTransaction{
										TransactionHash:  cosmosHash,
										EVMHash:          evmHash,
										EVMDetailsSynced: false,
									}
									if err := db.Get().WithContext(evmCtx).Create(evmTx).Error; err != nil {
										global.Logger.Warnf("failed to create EVM transaction %s (evm: %s): %v", cosmosHash, evmHash, err)
									}
								} else {
									global.Logger.Warnf("failed to query EVM transaction %s: %v", cosmosHash, err)
								}
							} else {
								// Update evm_hash if record exists
								if err := db.Get().WithContext(evmCtx).
									Model(evmTx).
									Where("transaction_hash = ?", cosmosHash).
									Update("evm_hash", evmHash).Error; err != nil {
									global.Logger.Warnf("failed to update EVM transaction %s (evm: %s): %v", cosmosHash, evmHash, err)
								}
							}

							global.Logger.Debugf("extracted EVM hash %s for transaction %s", evmHash, cosmosHash)

							// Push to EVM details queue for async processing
							redisClient := cache.GetRedisClient()
							if redisClient != nil {
								taskStr := fmt.Sprintf("%s:%s", cosmosHash, evmHash)
								if err := redisClient.LPush(evmCtx, s.config.EVMDetailsQueueKey, taskStr).Err(); err != nil {
									global.Logger.Warnf("failed to push EVM details task to queue: %v", err)
								}
							}
						}(txModel.Hash, evmHash)
					} else {
						global.Logger.Debugf("EVM transaction detected but hash not found for transaction %s", txModel.Hash)
					}
				}
			}
		}

		txModels = append(txModels, txModelData{
			hash:    result.hash,
			txModel: txModel,
			txData:  result.txResp,
		})
		// Note: We keep the original hash for queue cleanup, but use txModel.Hash for database operations
	}

	if len(txModels) == 0 {
		// All transactions failed, clean up processing queue
		pipe := redisClient.Pipeline()
		for _, hash := range txHashes {
			pipe.LRem(ctx, s.config.TxHashProcessingQueueKey, 0, hash)
		}
		pipe.Exec(ctx)

		// Build detailed error message with failure breakdown
		errMsg := fmt.Sprintf("all transactions in batch failed to fetch or convert (total: %d): ", len(txHashes))
		var parts []string
		if len(fetchErrors) > 0 {
			sample := fetchErrors
			if len(sample) > ErrorSampleCount {
				sample = append([]string(nil), sample[:ErrorSampleCount]...)
				sample = append(sample, fmt.Sprintf("...and %d more", len(fetchErrors)-ErrorSampleCount))
			}
			parts = append(parts, fmt.Sprintf("fetch errors=%d (samples: %v)", len(fetchErrors), sample))
		}
		if len(nilResponseErrors) > 0 {
			sample := nilResponseErrors
			if len(sample) > ErrorSampleCount {
				sample = append([]string(nil), sample[:ErrorSampleCount]...)
				sample = append(sample, fmt.Sprintf("...and %d more", len(nilResponseErrors)-ErrorSampleCount))
			}
			parts = append(parts, fmt.Sprintf("nil responses=%d (samples: %v)", len(nilResponseErrors), sample))
		}
		if len(convertErrors) > 0 {
			sample := convertErrors
			if len(sample) > ErrorSampleCount {
				sample = append([]string(nil), sample[:ErrorSampleCount]...)
				sample = append(sample, fmt.Sprintf("...and %d more", len(convertErrors)-ErrorSampleCount))
			}
			parts = append(parts, fmt.Sprintf("conversion errors=%d (samples: %v)", len(convertErrors), sample))
		}
		errMsg += strings.Join(parts, "; ")

		return 0, errors.New(errMsg)
	}

	// Step 3: Batch check which transactions already exist
	txHashesToCheck := make([]string, len(txModels))
	for i, tm := range txModels {
		txHashesToCheck[i] = tm.txModel.Hash
	}

	var existingTxs []models.Transaction
	err := db.Get().WithContext(ctx).
		Select("id, hash, signatures, signer").
		Where("hash IN ?", txHashesToCheck).
		Find(&existingTxs).Error
	if err != nil {
		global.Logger.Errorf("failed to check existing transactions: %v", err)
		// Continue anyway, will check individually
	}

	existingTxMap := make(map[string]uint, len(existingTxs))
	// Also store the full transaction data for preserving signatures and signer
	existingTxFullMap := make(map[string]*models.Transaction, len(existingTxs))
	for i := range existingTxs {
		existingTxMap[existingTxs[i].Hash] = existingTxs[i].ID
		existingTxFullMap[existingTxs[i].Hash] = &existingTxs[i]
	}

	// Step 4: Separate into inserts and updates
	txsToInsert := make([]*models.Transaction, 0, len(txModels))
	txsToUpdate := make([]*models.Transaction, 0, len(txModels))
	txDataMap := make(map[string]*injective_explorer_rpcpb.TxDetailData, len(txModels))

	for _, tm := range txModels {
		// Try to get blockResp from cache for authoritative tx_number calculation (tx_number = tx_index + 1, per-block)
		var blockResp *coretypes.ResultBlock
		if tm.txData != nil && tm.txData.TxResponse != nil {
			blockHeight := uint64(tm.txData.TxResponse.Height)
			blockResp = s.getCachedBlockResp(blockHeight)
			if blockResp == nil {
				// Try to fetch from chain if not in cache
				blockCtx, blockCancel := context.WithTimeout(ctx, 10*time.Second)
				var err error
				blockResp, err = s.tmProcessBlock(blockCtx, int64(blockHeight))
				blockCancel()
				if err != nil {
					global.Logger.Debugf("block %d not found in cache and failed to fetch from chain for tx %s: %v, will use legacy tx_number calculation", blockHeight, tm.hash, err)
				}
			}
		}
		
		// Convert GetTxResponse to TxDetailData for txDataMap
		// Pass blockResp for authoritative tx_number calculation (tx_number = tx_index + 1, per-block)
		txDetailData := models.ConvertGetTxResponseToTxDetailData(tm.txData, s.chainClient.ClientContext().Codec, blockResp)
		if txDetailData != nil {
			txDataMap[tm.txModel.Hash] = txDetailData
		}
		// Use hash to check existence (hash is the unique identifier)
		if id, exists := existingTxMap[tm.txModel.Hash]; exists {
			tm.txModel.ID = id
			// Preserve existing signatures and signer from database
			// This prevents empty values from GetTxResponse from overwriting good data
			// Only restore if database has non-empty values (protect against overwriting good data with empty values)
			if existingTx, ok := existingTxFullMap[tm.txModel.Hash]; ok {
				// Only restore signatures if database has non-empty value
				// This protects against overwriting good data with empty values
				if len(existingTx.Signatures) > 0 {
					tm.txModel.Signatures = existingTx.Signatures
				}
				// Only restore signer if database has non-empty value
				if existingTx.Signer != "" {
					tm.txModel.Signer = existingTx.Signer
				}
			}
			txsToUpdate = append(txsToUpdate, tm.txModel)
		} else {
			txsToInsert = append(txsToInsert, tm.txModel)
		}
	}

	// Step 5: Batch insert new transactions
	successCount := 0
	batchSize := s.config.TxBatchSize
	if batchSize <= 0 {
		batchSize = 50 // Default batch size
	}
	if len(txsToInsert) > 0 {
		err = db.Get().WithContext(ctx).CreateInBatches(txsToInsert, batchSize).Error
		if err != nil {
			global.Logger.Errorf("failed to batch insert transactions: %v", err)
			return 0, errors.Wrap(err, "failed to batch insert transactions")
		}
		successCount += len(txsToInsert)
		global.Logger.Debugf("batch inserted %d transactions", len(txsToInsert))
	}

	// Step 6: Batch update existing transactions
	if len(txsToUpdate) > 0 {
		// Update in batches
		for i := 0; i < len(txsToUpdate); i += batchSize {
			end := i + batchSize
			if end > len(txsToUpdate) {
				end = len(txsToUpdate)
			}
			batch := txsToUpdate[i:end]

			// Pre-serialize JSON fields for the batch to avoid GORM serialization issues
			// When using map[string]interface{} with Updates, GORM doesn't apply serializer:json tags
			type serializedData struct {
				eventsJSON   string
				msgTypesJSON string
				sigsJSON     string
				claimIdsJSON string
				valuesJSON   string
			}
			serializedBatch := make([]serializedData, len(batch))

			for i, txModel := range batch {
				// Serialize events
				if txModel.Events != nil {
					eventsJSON, err := sonic.Marshal(txModel.Events)
					if err != nil {
						global.Logger.Warnf("Failed to serialize events for transaction %s: %v", txModel.Hash, err)
					} else {
						serializedBatch[i].eventsJSON = string(eventsJSON)
					}
				}
				// Serialize message_types
				if txModel.MessageTypes != nil {
					msgTypesJSON, err := sonic.Marshal(txModel.MessageTypes)
					if err != nil {
						global.Logger.Warnf("Failed to serialize message_types for transaction %s: %v", txModel.Hash, err)
					} else {
						serializedBatch[i].msgTypesJSON = string(msgTypesJSON)
					}
				}
				// Serialize signatures - only if non-nil and non-empty
				// This prevents empty signature data from overwriting existing database records
				if txModel.Signatures != nil {
					sigsJSON, err := sonic.Marshal(txModel.Signatures)
					if err != nil {
						global.Logger.Warnf("Failed to serialize signatures for transaction %s: %v", txModel.Hash, err)
					} else {
						serializedBatch[i].sigsJSON = string(sigsJSON)
					}
				}
				// Serialize claim_ids
				if txModel.ClaimIds != nil {
					claimIdsJSON, err := sonic.Marshal(txModel.ClaimIds)
					if err != nil {
						global.Logger.Warnf("Failed to serialize claim_ids for transaction %s: %v", txModel.Hash, err)
					} else {
						serializedBatch[i].claimIdsJSON = string(claimIdsJSON)
					}
				}
				// Serialize values - ensure it's always an array format
				if txModel.Values != nil {
					valuesJSON, err := sonic.Marshal(txModel.Values)
					if err != nil {
						global.Logger.Warnf("Failed to serialize values for transaction %s: %v", txModel.Hash, err)
					} else {
						serializedBatch[i].valuesJSON = string(valuesJSON)
					}
				}
			}

			err = db.Get().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				for i, txModel := range batch {
					// Explicitly select only the fields we want to update
					// This ensures signatures and signer are NEVER touched during updates
					// These fields should only be set when the transaction is first created
					updateFields := map[string]interface{}{
						"block_number":         txModel.BlockNumber,
						"block_timestamp":      txModel.BlockTimestamp,
						"code":                 txModel.Code,
						"data":                 txModel.Data,
						"info":                 txModel.Info,
						"gas_wanted":           txModel.GasWanted,
						"gas_used":             txModel.GasUsed,
						"gas_fee":              txModel.GasFee,
						"codespace":            txModel.Codespace,
						"events":               serializedBatch[i].eventsJSON,
						"tx_type":              txModel.TxType,
						"message_types":        serializedBatch[i].msgTypesJSON,
						"messages":             txModel.Messages,
						"memo":                 txModel.Memo,
						"tx_number":            txModel.TxNumber,
						"block_unix_timestamp": txModel.BlockUnixTimestamp,
						"error_log":            txModel.ErrorLog,
						"logs":                 txModel.Logs,
						"claim_ids":            serializedBatch[i].claimIdsJSON,
						"to":                   txModel.To,
						"values":               serializedBatch[i].valuesJSON,
						"archived":             txModel.Archived,
						"data_storage_key":     txModel.DataStorageKey,
						"messages_storage_key": txModel.MessagesStorageKey,
						"logs_storage_key":     txModel.LogsStorageKey,
					}
					// EVM-related fields are now stored in evm_transactions table
					// Do NOT update evm_hash, evm_from_address, evm_to_address, etc. in transactions table

					// Use Omit to explicitly exclude signatures and signer fields from update
					// This is more reliable than Select() because it explicitly excludes the fields we don't want to update
					// This ensures signatures and signer are NEVER touched during updates
					// These fields should only be set when the transaction is first created or when explicitly updating them
					if err := tx.Table("transactions").
						Omit("signatures", "signer").
						Where("hash = ?", txModel.Hash).
						Updates(updateFields).Error; err != nil {
						return errors.Wrapf(err, "failed to update transaction %s", txModel.Hash)
					}
				}
				return nil
			})
			if err != nil {
				global.Logger.Errorf("failed to batch update transactions: %v", err)
				return successCount, errors.Wrap(err, "failed to batch update transactions")
			}
		}
		successCount += len(txsToUpdate)
		global.Logger.Debugf("batch updated %d transactions", len(txsToUpdate))
	}

	// Step 7: Update accounts for all transactions (can be done in background)
	// Note: Account updates are done individually but can be optimized later
	go func() {
		accountCtx, accountCancel := context.WithTimeout(context.Background(), AccountQueryTimeout)
		defer accountCancel()
			for _, tm := range txModels {
				// Try to get blockResp from cache for authoritative tx_number calculation (tx_number = tx_index + 1, per-block)
				var blockResp *coretypes.ResultBlock
				if tm.txData != nil && tm.txData.TxResponse != nil {
					blockHeight := uint64(tm.txData.TxResponse.Height)
					blockResp = s.getCachedBlockResp(blockHeight)
					if blockResp == nil {
						// Try to fetch from chain if not in cache
						blockCtx, blockCancel := context.WithTimeout(accountCtx, 10*time.Second)
						var err error
						blockResp, err = s.tmProcessBlock(blockCtx, int64(blockHeight))
						blockCancel()
						if err != nil {
							global.Logger.Debugf("block %d not found in cache and failed to fetch from chain for tx %s: %v, will use legacy tx_number calculation", blockHeight, tm.hash, err)
						}
					}
				}
				
				// Convert GetTxResponse to TxDetailData for account extraction
				// Pass blockResp for authoritative tx_number calculation (tx_number = tx_index + 1, per-block)
				txDetailData := models.ConvertGetTxResponseToTxDetailData(tm.txData, s.chainClient.ClientContext().Codec, blockResp)
				if txDetailData != nil {
					if err := s.updateAccountsFromTransaction(accountCtx, txDetailData); err != nil {
					global.Logger.Warnf("failed to update accounts from transaction %s: %v", tm.hash, err)
				}
			}
		}
	}()

	// Step 8: Cache transactions and publish events
	eventBus := GetEventBus()
	for _, tm := range txModels {
		// Cache transaction to Redis
		cacheTransaction(ctx, tm.txModel)

		// Update transaction statistics incrementally (async to avoid blocking)
		isUpdate := existingTxMap[tm.txModel.Hash] != 0
		go func(tx *models.Transaction, isNew bool) {
			statsCtx, statsCancel := context.WithTimeout(context.Background(), StatsContextTimeoutSeconds*time.Second)
			defer statsCancel()
			updateTransactionStatsIncremental(statsCtx, tx, isNew)
		}(tm.txModel, !isUpdate)

		// Publish transaction event
		txEventType := EventTypeTransactionNew
		if isUpdate {
			txEventType = EventTypeTransactionUpdate
		}
		eventBus.PublishTransaction(&TransactionEvent{
			Transaction: tm.txModel,
			Type:        txEventType,
		})

		// Publish account events
		// Convert GetTxResponse to TxDetailData for address extraction
		// Try to get blockResp from cache for authoritative tx_number calculation (tx_number = tx_index + 1, per-block)
		var blockResp *coretypes.ResultBlock
		if tm.txData != nil && tm.txData.TxResponse != nil {
			blockHeight := uint64(tm.txData.TxResponse.Height)
			blockResp = s.getCachedBlockResp(blockHeight)
			if blockResp == nil {
				// Try to fetch from chain if not in cache (use background context since this is async)
				blockCtx, blockCancel := context.WithTimeout(context.Background(), 10*time.Second)
				var err error
				blockResp, err = s.tmProcessBlock(blockCtx, int64(blockHeight))
				blockCancel()
				if err != nil {
					global.Logger.Debugf("block %d not found in cache and failed to fetch from chain for tx %s: %v, will use legacy tx_number calculation", blockHeight, tm.hash, err)
				}
			}
		}
		
		// Pass blockResp for authoritative tx_number calculation (tx_number = tx_index + 1, per-block)
		txDetailData := models.ConvertGetTxResponseToTxDetailData(tm.txData, s.chainClient.ClientContext().Codec, blockResp)
		if txDetailData != nil {
			addresses := s.extractAddressesFromTransaction(txDetailData)
			for address := range addresses {
				if address != "" && utils.IsValidAddress(address) {
					eventBus.PublishAccount(address, &AccountEvent{
						Address: address,
						Type:    EventTypeAccountUpdate,
						Data:    map[string]interface{}{"transaction": tm.txModel, "type": "update"},
					})
				}
			}
		}

		// Return transaction to pool
		go func(tx *models.Transaction) {
			time.Sleep(ShortDelayMilliseconds * time.Millisecond)
			PutTransactionToPool(tx)
		}(tm.txModel)
	}

	// Step 9: Clean up processing queue for successful transactions
	// Use original hash from queue (tm.hash) for cleanup, as it matches what's in the queue
	pipe := redisClient.Pipeline()
	for _, tm := range txModels {
		pipe.LRem(ctx, s.config.TxHashProcessingQueueKey, 0, tm.hash)
	}
	pipe.Exec(ctx) // Ignore errors for cleanup

	global.Logger.Debugf("batch processed %d/%d transactions successfully", successCount, len(txHashes))
	return successCount, nil
}

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
	var txResp *tx.GetTxResponse

	delay := s.config.RetryDelay
	for attempt := 0; attempt < s.config.MaxRetryAttempts; attempt++ {
		txResp, err = s.chainClient.GetTx(ctx, txHash)
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
		if utils.IsRetryableError(err) {
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

	if txResp == nil || txResp.TxResponse == nil {
		global.Logger.Warnf("transaction response is nil for hash %s", txHash)
		return nil
	}

	// Fetch block data for authoritative tx_number calculation (tx_number = tx_index + 1, per-block)
	var blockResp *coretypes.ResultBlock
	if txResp.TxResponse.Height > 0 {
		blockResp = s.getCachedBlockResp(uint64(txResp.TxResponse.Height))
		if blockResp == nil {
			// Try to fetch from chain if not in cache
			blockCtx, blockCancel := context.WithTimeout(ctx, 10*time.Second)
			var err error
			blockResp, err = s.tmProcessBlock(blockCtx, int64(txResp.TxResponse.Height))
			blockCancel()
			if err != nil {
				global.Logger.Warnf("failed to fetch block %d for tx_number calculation: %v, will use legacy method", txResp.TxResponse.Height, err)
				// Continue without block data - will fall back to legacy calculation
			} else if blockResp != nil {
				global.Logger.Infof("✓ fetched block %d from chain for authoritative tx_number calculation (block has %d txs)",
					txResp.TxResponse.Height, len(blockResp.Block.Data.Txs))
			}
		}
	}

	// Save transaction to database using transaction for atomicity
	// Get storage threshold from config
	storageThreshold := getStorageThresholdForSync()

	// Use object pool to reduce allocations
	poolTx := GetTransactionFromPool()
	txModel := models.TransactionFromGetTxResponse(txResp, storageThreshold, s.chainClient.ClientContext().Codec, blockResp)
	if txModel == nil {
		// If conversion fails, return transaction to pool and put the transaction hash back to the queue
		PutTransactionToPool(poolTx)
		// Use Pipeline to batch Redis operations
		pipe := redisClient.Pipeline()
		pipe.LPush(ctx, s.config.TxHashQueueKey, txHash)
		pipe.LRem(ctx, s.config.TxHashProcessingQueueKey, 0, txHash)
		pipe.Exec(ctx) // Ignore errors for cleanup operations
		global.Logger.Errorf("failed to convert transaction %s to model", txHash)
		return errors.Errorf("failed to convert transaction %s", txHash)
	}

	// Use database transaction to ensure atomicity
	var isTxUpdate bool
	err = db.Get().Transaction(func(tx *gorm.DB) error {
		// Check if transaction already exists
		var existingTx models.Transaction
		err := tx.Where("hash = ?", txHash).First(&existingTx).Error
		if err == nil {
			// Transaction exists, update it
			isTxUpdate = true
			// Use raw SQL update to explicitly exclude signatures and signer fields
			// This ensures these fields are never overwritten during updates
			// Build the update map with all fields except signatures and signer
			updateFields := map[string]interface{}{
				"block_number":         txModel.BlockNumber,
				"block_timestamp":      txModel.BlockTimestamp,
				"code":                 txModel.Code,
				"data":                 txModel.Data,
				"info":                 txModel.Info,
				"gas_wanted":           txModel.GasWanted,
				"gas_used":             txModel.GasUsed,
				"gas_fee":              txModel.GasFee,
				"codespace":            txModel.Codespace,
				"tx_type":              txModel.TxType,
				"messages":             txModel.Messages,
				"memo":                 txModel.Memo,
				"tx_number":            txModel.TxNumber,
				"block_unix_timestamp": txModel.BlockUnixTimestamp,
				"error_log":            txModel.ErrorLog,
				"logs":                 txModel.Logs,
				"to":                   txModel.To,
				"values":               txModel.Values,
				"archived":             txModel.Archived,
				"data_storage_key":     txModel.DataStorageKey,
				"messages_storage_key": txModel.MessagesStorageKey,
				"logs_storage_key":     txModel.LogsStorageKey,
			}
			// EVM-related fields are now stored in evm_transactions table
			// Do NOT update evm_hash, evm_from_address, evm_to_address, etc. in transactions table
			// Serialize JSON fields manually since we're not using the model directly
			if txModel.Events != nil {
				if eventsJSON, err := sonic.Marshal(txModel.Events); err == nil {
					updateFields["events"] = string(eventsJSON)
				}
			}
			if txModel.MessageTypes != nil {
				if msgTypesJSON, err := sonic.Marshal(txModel.MessageTypes); err == nil {
					updateFields["message_types"] = string(msgTypesJSON)
				}
			}
			if txModel.ClaimIds != nil {
				if claimIdsJSON, err := sonic.Marshal(txModel.ClaimIds); err == nil {
					updateFields["claim_ids"] = string(claimIdsJSON)
				}
			}
			// Serialize values - ensure it's always an array format
			if txModel.Values != nil {
				if valuesJSON, err := sonic.Marshal(txModel.Values); err == nil {
					updateFields["values"] = string(valuesJSON)
				}
			}

			// Use Table() with Omit to explicitly exclude signatures and signer fields from update
			// This is more reliable than Select() because it explicitly excludes the fields we don't want to update
			// This ensures signatures and signer are NEVER touched during updates
			if err := tx.Table("transactions").
				Omit("signatures", "signer").
				Where("hash = ?", txModel.Hash).
				Updates(updateFields).Error; err != nil {
				return errors.Wrapf(err, "failed to update transaction %s", txHash)
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// Transaction doesn't exist, create it
			isTxUpdate = false
			err = tx.Create(txModel).Error
			if err != nil {
				return errors.Wrapf(err, "failed to create transaction %s", txHash)
			}
		} else {
			return errors.Wrapf(err, "failed to check transaction existence %s", txHash)
		}

		// Extract and update account information from transaction
		// Note: Account updates are done within the same transaction
		// blockResp is already fetched above, reuse it here
		
		// Convert GetTxResponse to TxDetailData for account extraction
		// Pass blockResp for authoritative tx_number calculation (tx_number = tx_index + 1, per-block)
		txDetailData := models.ConvertGetTxResponseToTxDetailData(txResp, s.chainClient.ClientContext().Codec, blockResp)
		if txDetailData != nil {
			if err := s.updateAccountsFromTransactionWithTx(ctx, tx, txDetailData); err != nil {
				global.Logger.Warnf("failed to update accounts from transaction %s: %v", txHash, err)
				// Don't fail the transaction processing if account update fails
				// But log it for debugging
			}
		}

		return nil
	})

	if err != nil {
		// If database transaction fails, return transaction to pool and put the transaction hash back to the queue
		PutTransactionToPool(poolTx)
		// Use Pipeline to batch Redis operations
		pipe := redisClient.Pipeline()
		pipe.LPush(ctx, s.config.TxHashQueueKey, txHash)
		pipe.LRem(ctx, s.config.TxHashProcessingQueueKey, 0, txHash)
		pipe.Exec(ctx) // Ignore errors for cleanup operations
		global.Logger.Errorf("failed to save transaction %s: %v, put back to queue", txHash, err)
		return errors.Wrapf(err, "failed to save transaction %s", txHash)
	}

	// Transaction saved successfully, remove from processing queue
	// Use async operation for non-critical cleanup to avoid blocking
	go func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), CleanupContextTimeoutSeconds*time.Second)
		defer cleanupCancel()
		redisClient.LRem(cleanupCtx, s.config.TxHashProcessingQueueKey, 0, txHash)
	}()

	// Extract and save cross-chain transaction details
	// This is done asynchronously to avoid blocking the main flow
	go func() {
		crossChainCtx, crossChainCancel := context.WithTimeout(context.Background(), CrossChainContextTimeoutSeconds*time.Second)
		defer crossChainCancel()

		if err := s.saveCrossChainTransaction(crossChainCtx, txResp, txModel); err != nil {
			global.Logger.Warnf("failed to save cross-chain transaction %s: %v", txHash, err)
			// Don't fail the main flow, just log the error
		}
	}()

	// Cache transaction to Redis
	cacheTransaction(ctx, txModel)

	// Update transaction statistics incrementally (async to avoid blocking)
	go func() {
		statsCtx, statsCancel := context.WithTimeout(context.Background(), StatsContextTimeoutSeconds*time.Second)
		defer statsCancel()
		updateTransactionStatsIncremental(statsCtx, txModel, !isTxUpdate)
	}()

	// Publish transaction event for streaming
	eventBus := GetEventBus()
	txEventType := EventTypeTransactionNew
	if isTxUpdate {
		txEventType = EventTypeTransactionUpdate
	}
	eventBus.PublishTransaction(&TransactionEvent{
		Transaction: txModel,
		Type:        txEventType,
	})

	// Publish account events for addresses involved in the transaction
	// blockResp is already fetched above, reuse it here
	
	// Convert GetTxResponse to TxDetailData for address extraction
	// Pass blockResp for authoritative tx_number calculation (per tx_number.md spec)
	txDetailData := models.ConvertGetTxResponseToTxDetailData(txResp, s.chainClient.ClientContext().Codec, blockResp)
	if txDetailData != nil {
		addresses := s.extractAddressesFromTransaction(txDetailData)
		for address := range addresses {
			if address != "" && utils.IsValidAddress(address) {
				eventBus.PublishAccount(address, &AccountEvent{
					Address: address,
					Type:    EventTypeAccountUpdate,
					Data:    map[string]interface{}{"transaction": txModel, "type": "update"},
				})
			}
		}
	}

	// Return transaction to pool after all operations complete
	// Use async operation to avoid blocking, but ensure it happens after all synchronous operations
	go func() {
		// Small delay to ensure all references are done
		time.Sleep(ShortDelayMilliseconds * time.Millisecond)
		PutTransactionToPool(poolTx)
	}()

	return nil
}

// updateAccountsFromTransactionWithTx extracts account addresses from transaction and updates database using the provided transaction.
// Optimized to use batch account queries to reduce database I/O.
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
		addressesFromEvent := utils.ExtractAddressesFromString(eventStr)
		for _, addr := range addressesFromEvent {
			if utils.IsValidAddress(addr) {
				addresses[addr] = true
			}
		}
	}

	// Also try to extract from messages field if it contains address information
	if len(txData.Messages) > 0 {
		messagesStr := string(txData.Messages)
		addressesFromMessages := utils.ExtractAddressesFromString(messagesStr)
		for _, addr := range addressesFromMessages {
			if utils.IsValidAddress(addr) {
				addresses[addr] = true
			}
		}
	}

	for _, signer := range txData.Signatures {
		if signer == nil {
			continue
		}
		if utils.IsValidAddress(signer.Address) {
			addresses[signer.Address] = true
		}
	}

	if len(addresses) == 0 {
		return nil
	}

	// Convert addresses map to slice for batch query
	addressList := make([]string, 0, len(addresses))
	for addr := range addresses {
		if addr != "" {
			addressList = append(addressList, addr)
		}
	}

	// Batch query existing accounts to reduce database queries
	var existingAccounts []models.Account
	err := tx.Where("address IN ?", addressList).Find(&existingAccounts).Error
	if err != nil {
		global.Logger.Warnf("failed to batch query accounts: %v", err)
		// Fallback to individual queries
		existingAccounts = nil
	}

	// Create a map of existing accounts for quick lookup
	existingAccountMap := make(map[string]*models.Account, len(existingAccounts))
	for i := range existingAccounts {
		existingAccountMap[existingAccounts[i].Address] = &existingAccounts[i]
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

		// Check if account exists using the batch query result
		existingAccount, exists := existingAccountMap[address]
		if exists {
			// Account exists, check if update is needed
			needsUpdate := existingAccount.BaseAccount != accountModel.BaseAccount ||
				existingAccount.CodeHash != accountModel.CodeHash

			if needsUpdate {
				// Only update if fields have changed
				err = tx.Model(existingAccount).Updates(accountModel).Error
				if err != nil {
					global.Logger.Warnf("failed to update account %s: %v", address, err)
				}
			}
			// If no update needed, skip database operation
		} else {
			// Account doesn't exist, create it
			err = tx.Create(accountModel).Error
			if err != nil {
				global.Logger.Warnf("failed to create account %s: %v", address, err)
			}
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
		addressesFromEvent := utils.ExtractAddressesFromString(eventStr)
		for _, addr := range addressesFromEvent {
			if utils.IsValidAddress(addr) {
				addresses[addr] = true
			}
		}
	}

	// Also try to extract from messages field if it contains address information
	if len(txData.Messages) > 0 {
		messagesStr := string(txData.Messages)
		addressesFromMessages := utils.ExtractAddressesFromString(messagesStr)
		for _, addr := range addressesFromMessages {
			if utils.IsValidAddress(addr) {
				addresses[addr] = true
			}
		}
	}

	for _, signer := range txData.Signatures {
		if signer == nil {
			continue
		}
		if utils.IsValidAddress(signer.Address) {
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
// Optimized to use batch upsert operations to reduce database I/O.
// Uses independent context with timeout to avoid deadline exceeded errors.
func (s *SyncService) updateAccountBalances(ctx context.Context, address string) error {
	// Create independent context with timeout for balance query
	// Use AccountQueryTimeout as balance queries may take time
	balanceCtx, cancel := context.WithTimeout(context.Background(), AccountQueryTimeout)
	defer cancel()

	// Get account balances from chain with retry logic
	var balancesResp interface{}
	var err error

	// Retry up to 2 times with exponential backoff
	maxRetries := BalanceQueryMaxRetries
	retryDelay := BalanceQueryRetryDelaySeconds * time.Second
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-balanceCtx.Done():
				return balanceCtx.Err()
			case <-time.After(retryDelay):
				retryDelay *= 2 // Exponential backoff
			}
			global.Logger.Debugf("retrying balance query for account %s (attempt %d/%d)", address, attempt+1, maxRetries+1)
		}

		// Use type inference - let compiler figure out the type
		balancesResp, err = s.chainClient.GetBankBalances(balanceCtx, address)
		if err == nil {
			break // Success
		}

		// Check if error is due to context cancellation or timeout
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// Context timeout, don't retry
			global.Logger.Warnf("balance query timeout for account %s (attempt %d/%d): %v", address, attempt+1, maxRetries+1, err)
			return err
		}

		// For other errors, log and retry
		if attempt < maxRetries {
			global.Logger.Debugf("balance query error for account %s (attempt %d/%d), will retry: %v", address, attempt+1, maxRetries+1, err)
		}
	}

	if err != nil {
		// All retries failed
		global.Logger.Warnf("failed to get balances for account %s after %d attempts: %v", address, maxRetries+1, err)
		return err
	}

	// Access Balances field - the SDK returns *types.QueryAllBalancesResponse
	// Use reflection to access the Balances field directly
	if balancesResp == nil {
		return nil
	}

	// Use reflection to access the Balances field
	balancesValue := reflect.ValueOf(balancesResp)
	if balancesValue.Kind() == reflect.Ptr {
		balancesValue = balancesValue.Elem()
	}

	// Get the Balances field
	balancesField := balancesValue.FieldByName("Balances")
	if !balancesField.IsValid() {
		global.Logger.Warnf("Balances field not found in response for account %s, type: %T", address, balancesResp)
		return nil
	}

	// Handle different field types (slice, pointer to slice, etc.)
	var balancesSlice reflect.Value
	if balancesField.Kind() == reflect.Slice {
		balancesSlice = balancesField
	} else if balancesField.Kind() == reflect.Ptr || balancesField.Kind() == reflect.Interface {
		if balancesField.IsNil() {
			return nil
		}
		balancesSlice = balancesField.Elem()
	} else {
		global.Logger.Warnf("Balances field has unexpected kind for account %s, kind: %v, type: %T", address, balancesField.Kind(), balancesField.Interface())
		return nil
	}

	// Verify it's a slice
	if balancesSlice.Kind() != reflect.Slice {
		global.Logger.Warnf("Balances is not a slice for account %s, kind: %v", address, balancesSlice.Kind())
		return nil
	}

	// Check if empty
	if balancesSlice.Len() == 0 {
		return nil
	}

	// Convert all balances to models
	balanceModels := make([]*models.AccountBalance, 0, balancesSlice.Len())
	for i := 0; i < balancesSlice.Len(); i++ {
		balance := balancesSlice.Index(i).Interface()

		// Try to type assert to cosmos-sdk types.Coin first (most common case)
		if coin, ok := balance.(github_com_cosmos_cosmos_sdk_types.Coin); ok {
			balanceModel := models.AccountBalanceFromProto(coin, address)
			if balanceModel != nil {
				balanceModels = append(balanceModels, balanceModel)
			}
			continue
		}

		// Fallback: use reflection to extract Denom and Amount
		balanceVal := reflect.ValueOf(balance)
		if balanceVal.Kind() == reflect.Ptr {
			balanceVal = balanceVal.Elem()
		}

		// Get Denom field
		denomField := balanceVal.FieldByName("Denom")
		if !denomField.IsValid() {
			// Try GetDenom method
			getDenomMethod := balanceVal.MethodByName("GetDenom")
			if getDenomMethod.IsValid() {
				results := getDenomMethod.Call(nil)
				if len(results) > 0 {
					denomField = results[0]
				}
			}
		}

		// Get Amount field
		amountField := balanceVal.FieldByName("Amount")
		if !amountField.IsValid() {
			// Try GetAmount method
			getAmountMethod := balanceVal.MethodByName("GetAmount")
			if getAmountMethod.IsValid() {
				results := getAmountMethod.Call(nil)
				if len(results) > 0 {
					amountField = results[0]
				}
			}
		}

		if !denomField.IsValid() || !amountField.IsValid() {
			global.Logger.Warnf("unable to extract Denom/Amount from balance for account %s, type: %T", address, balance)
			continue
		}

		denom := denomField.String()
		var amountStr string

		// Try to get amount as string
		if amountField.Kind() == reflect.String {
			amountStr = amountField.String()
		} else {
			// Try to call String() method
			stringMethod := amountField.MethodByName("String")
			if stringMethod.IsValid() {
				results := stringMethod.Call(nil)
				if len(results) > 0 {
					amountStr = results[0].String()
				}
			} else {
				// Fallback: convert to string
				amountStr = fmt.Sprintf("%v", amountField.Interface())
			}
		}

		if denom == "" || amountStr == "" {
			global.Logger.Warnf("empty denom or amount for balance for account %s", address)
			continue
		}

		// Address conversion is handled in AccountBalanceFromProto
		// Create AccountBalance directly with converted address
		// We need to convert the address to Ethereum format first
		ethAddress, err := models.InjToEthAddress(address)
		if err != nil {
			// If conversion fails, use original address (for backward compatibility)
			ethAddress = address
		}
		balanceModel := &models.AccountBalance{
			Address: ethAddress,
			Denom:   denom,
			Amount:  amountStr,
		}
		balanceModels = append(balanceModels, balanceModel)
	}

	if len(balanceModels) == 0 {
		return nil
	}

	// Use batch upsert in a single transaction to reduce I/O
	err = db.Get().Transaction(func(tx *gorm.DB) error {
		// For each balance, use upsert (insert or update)
		// This reduces the number of database queries from 2N (check + insert/update) to N (upsert)
		for _, balanceModel := range balanceModels {
			// Use Save which will insert if not exists or update if exists
			// Note: This requires a unique constraint on (address, denom) in the database
			err := tx.Where("address = ? AND denom = ?", balanceModel.Address, balanceModel.Denom).
				Assign("amount", balanceModel.Amount).
				FirstOrCreate(balanceModel).Error
			if err != nil {
				global.Logger.Warnf("failed to upsert balance for account %s, denom %s: %v", address, balanceModel.Denom, err)
				// Continue processing other balances even if one fails
				continue
			}
		}
		return nil
	})

	if err != nil {
		global.Logger.Warnf("failed to update balances for account %s: %v", address, err)
	}

	return nil
}

// extractAddressesFromTransaction extracts all addresses from a transaction.
// It extracts addresses from events, messages, and signatures.
func (s *SyncService) extractAddressesFromTransaction(txData *injective_explorer_rpcpb.TxDetailData) map[string]bool {
	addresses := make(map[string]bool)

	if txData == nil {
		return addresses
	}

	// Extract addresses from events
	for _, event := range txData.Events {
		if event == nil {
			continue
		}
		eventStr := event.String()
		addressesFromEvent := utils.ExtractAddressesFromString(eventStr)
		for _, addr := range addressesFromEvent {
			if utils.IsValidAddress(addr) {
				addresses[addr] = true
			}
		}
	}

	// Extract addresses from messages
	if len(txData.Messages) > 0 {
		messagesStr := string(txData.Messages)
		addressesFromMessages := utils.ExtractAddressesFromString(messagesStr)
		for _, addr := range addressesFromMessages {
			if utils.IsValidAddress(addr) {
				addresses[addr] = true
			}
		}
	}

	// Extract addresses from signatures
	for _, signer := range txData.Signatures {
		if signer == nil {
			continue
		}
		if utils.IsValidAddress(signer.Address) {
			addresses[signer.Address] = true
		}
	}

	return addresses
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

	// Get earliest block height: use config value as default, override with RPC node value if available
	earliestBlockHeight := s.config.EarliestBlockHeight
	if status != nil && status.SyncInfo.EarliestBlockHeight > 0 {
		rpcEarliestHeight := uint64(status.SyncInfo.EarliestBlockHeight)
		// Use the larger value to ensure we don't sync below what the RPC node supports
		if rpcEarliestHeight > earliestBlockHeight {
			earliestBlockHeight = rpcEarliestHeight
			global.Logger.Infof("RPC node earliest block height %d is higher than config %d, using RPC value", rpcEarliestHeight, s.config.EarliestBlockHeight)
		} else {
			global.Logger.Infof("using config earliest block height %d (RPC node supports from %d)", earliestBlockHeight, rpcEarliestHeight)
		}
	} else {
		global.Logger.Infof("RPC node earliest block height not available, using config value: %d", earliestBlockHeight)
	}

	// Initialize historical sync height from Redis persistence or database
	// Try to recover from Redis first (for crash recovery)
	var minHeight uint64 = earliestBlockHeight
	var maxHeight uint64

	// Try to load persisted sync height from Redis
	redisClient := cache.GetRedisClient()
	var persistedHeight uint64 = 0
	if redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), RedisContextTimeoutSeconds*time.Second)
		defer cancel()

		heightStr, err := redisClient.Get(ctx, CacheKeyHistoricalSyncHeight).Result()
		if err == nil && heightStr != "" {
			if _, err := fmt.Sscanf(heightStr, "%d", &persistedHeight); err == nil {
				global.Logger.Infof("historical block sync: recovered persisted sync height %d from Redis", persistedHeight)
			}
		}
	}

	// Get database min/max heights
	err = db.Get().Model(&models.Block{}).
		Select("COALESCE(MIN(height), ?)", earliestBlockHeight).
		Scan(&minHeight).Error
	if err != nil {
		global.Logger.Warnf("failed to query min height from database: %v, starting from earliest block height %d", err, earliestBlockHeight)
		minHeight = earliestBlockHeight
	}

	err = db.Get().Model(&models.Block{}).
		Select("COALESCE(MAX(height), 0)").
		Scan(&maxHeight).Error
	if err != nil {
		global.Logger.Warnf("failed to query max height from database: %v", err)
	}

	// Determine starting height for historical sync
	var startHeight uint64
	if persistedHeight > 0 {
		// Recover from persisted height, but ensure it's not below RPC's earliest height
		if persistedHeight < earliestBlockHeight {
			startHeight = earliestBlockHeight
			global.Logger.Infof("historical block sync: persisted height %d is below RPC earliest height %d, adjusting to %d", persistedHeight, earliestBlockHeight, startHeight)
		} else {
			startHeight = persistedHeight
			global.Logger.Infof("historical block sync: resuming from persisted height %d", startHeight)
		}
	} else if minHeight > earliestBlockHeight {
		// We have blocks, but not from the beginning
		// Start syncing from earliest block height to minHeight - 1
		startHeight = earliestBlockHeight
		global.Logger.Infof("historical block sync: found blocks from height %d, will sync from height %d to %d", minHeight, startHeight, minHeight-1)
	} else {
		// No blocks or starting from earliest block height
		startHeight = earliestBlockHeight
		global.Logger.Infof("historical block sync: starting from earliest block height %d", startHeight)
	}

	s.setHistoricalSyncHeight(startHeight)

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

			// Get earliest block height: use config value as default, override with RPC node value if it's higher
			earliestBlockHeight := s.config.EarliestBlockHeight
			if status != nil && status.SyncInfo.EarliestBlockHeight > 0 {
				rpcEarliestHeight := uint64(status.SyncInfo.EarliestBlockHeight)
				// Use the larger value to ensure we don't sync below what the RPC node supports
				if rpcEarliestHeight > earliestBlockHeight {
					earliestBlockHeight = rpcEarliestHeight
				}
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

			// Determine target height based on database state
			var targetHeight uint64
			if dbMinHeight == earliestBlockHeight {
				// Database is empty or only has blocks from earliest height
				// We need to sync from earliest height up to current height
				targetHeight = currentHeight
				global.Logger.Debugf("historical block sync: database is empty, will sync up to current height %d", targetHeight)
			} else {
				// Database has some blocks, sync gaps below dbMinHeight
				targetHeight = dbMinHeight - 1
				if targetHeight < earliestBlockHeight {
					targetHeight = earliestBlockHeight
				}
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

	// Batch check which blocks already exist to reduce database queries
	heightsToCheck := make([]uint64, 0, batchEnd-startHeight)
	for height := startHeight; height < batchEnd; height++ {
		heightsToCheck = append(heightsToCheck, height)
	}

	blocksExist, err := batchCheckBlocksExist(heightsToCheck)
	if err != nil {
		global.Logger.Warnf("failed to batch check blocks existence: %v, falling back to individual checks", err)
		blocksExist = make(map[uint64]bool) // Fallback: treat all as non-existent
	}

	// Filter out blocks that already exist
	heightsToProcess := make([]uint64, 0, len(heightsToCheck))
	for _, height := range heightsToCheck {
		if !blocksExist[height] {
			heightsToProcess = append(heightsToProcess, height)
		}
	}

	if len(heightsToProcess) == 0 {
		global.Logger.Debugf("all blocks from %d to %d already exist, skipping", startHeight, batchEnd-1)
		s.setHistoricalSyncHeight(batchEnd)
		return nil
	}

	global.Logger.Debugf("batch check: %d blocks exist, %d blocks need processing", len(heightsToCheck)-len(heightsToProcess), len(heightsToProcess))

	// Use worker pool pattern for concurrent processing
	type blockJob struct {
		height uint64
	}

	jobs := make(chan blockJob, len(heightsToProcess))
	results := make(chan error, len(heightsToProcess))

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

				// Process the block (skip existence check since we already checked in batch)
				if err := s.processBlockOptimizedSkipCheck(job.height); err != nil {
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
		for _, height := range heightsToProcess {
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

// batchCheckBlocksExist checks if multiple blocks exist in the database.
// Returns a map of height -> exists (true if exists, false if not).
// This reduces database queries from N to 1 for batch operations.
func batchCheckBlocksExist(heights []uint64) (map[uint64]bool, error) {
	if len(heights) == 0 {
		return make(map[uint64]bool), nil
	}

	var existingHeights []uint64
	err := db.Get().Model(&models.Block{}).
		Where("height IN ?", heights).
		Pluck("height", &existingHeights).Error

	if err != nil {
		return nil, errors.Wrap(err, "failed to batch check blocks existence")
	}

	// Create a map of existing blocks
	exists := make(map[uint64]bool, len(existingHeights))
	for _, h := range existingHeights {
		exists[h] = true
	}

	// Mark non-existing blocks as false
	for _, h := range heights {
		if !exists[h] {
			exists[h] = false
		}
	}

	return exists, nil
}

// batchCheckTransactionsExist checks if multiple transactions exist in the database.
// Returns a map of hash -> exists (true if exists, false if not).
func batchCheckTransactionsExist(hashes []string) (map[string]bool, error) {
	if len(hashes) == 0 {
		return make(map[string]bool), nil
	}

	var existingHashes []string
	err := db.Get().Model(&models.Transaction{}).
		Where("hash IN ?", hashes).
		Pluck("hash", &existingHashes).Error

	if err != nil {
		return nil, errors.Wrap(err, "failed to batch check transactions existence")
	}

	// Create a map of existing transactions
	exists := make(map[string]bool, len(existingHashes))
	for _, h := range existingHashes {
		exists[h] = true
	}

	// Mark non-existing transactions as false
	for _, h := range hashes {
		if !exists[h] {
			exists[h] = false
		}
	}

	return exists, nil
}

// batchGetAccounts retrieves multiple accounts from the database in a single query.
// Returns a map of address -> account model.
func batchGetAccounts(addresses []string) (map[string]*models.Account, error) {
	if len(addresses) == 0 {
		return make(map[string]*models.Account), nil
	}

	var accounts []models.Account
	err := db.Get().Where("address IN ?", addresses).Find(&accounts).Error
	if err != nil {
		return nil, errors.Wrap(err, "failed to batch get accounts")
	}

	// Create a map of address -> account
	accountMap := make(map[string]*models.Account, len(accounts))
	for i := range accounts {
		accountMap[accounts[i].Address] = &accounts[i]
	}

	return accountMap, nil
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

// getHistoricalSyncHeight returns current historical sync height.
// First tries to load from Redis persistence, falls back to in-memory value.
func (s *SyncService) getHistoricalSyncHeight() uint64 {
	s.historicalSyncMutex.RLock()
	defer s.historicalSyncMutex.RUnlock()

	// Try to load from Redis persistence
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), RedisContextTimeoutSeconds*time.Second)
		defer cancel()

		heightStr, err := redisClient.Get(ctx, CacheKeyHistoricalSyncHeight).Result()
		if err == nil && heightStr != "" {
			var height uint64
			if _, err := fmt.Sscanf(heightStr, "%d", &height); err == nil {
				// Update in-memory value if Redis has a more recent value
				if height > s.historicalSyncHeight {
					s.historicalSyncHeight = height
				}
				return s.historicalSyncHeight
			}
		}
	}

	return s.historicalSyncHeight
}

// setHistoricalSyncHeight sets current historical sync height and persists it to Redis.
func (s *SyncService) setHistoricalSyncHeight(height uint64) {
	s.historicalSyncMutex.Lock()
	defer s.historicalSyncMutex.Unlock()
	s.historicalSyncHeight = height

	// Persist to Redis for recovery after restart
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), RedisContextTimeoutSeconds*time.Second)
		defer cancel()

		// Set with no expiration (sync state should persist indefinitely)
		if err := redisClient.Set(ctx, CacheKeyHistoricalSyncHeight, height, RedisPersistNoExpiry).Err(); err != nil {
			global.Logger.Warnf("failed to persist historical sync height to Redis: %v", err)
		}
	}
}

// clearHistoricalSyncHeight clears the persisted historical sync height from Redis.
// This is useful when you want to restart historical sync from the beginning.
func (s *SyncService) clearHistoricalSyncHeight() {
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), RedisContextTimeoutSeconds*time.Second)
		defer cancel()

		if err := redisClient.Del(ctx, CacheKeyHistoricalSyncHeight).Err(); err != nil {
			global.Logger.Warnf("failed to clear historical sync height from Redis: %v", err)
		}
	}
}

// recoverProcessingQueue recovers transaction hashes from the processing queue
// and puts them back to the main queue. This is called on startup to handle
// transactions that were being processed when the service crashed.
func (s *SyncService) recoverProcessingQueue() error {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return errors.New("redis client is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), RedisLongContextTimeoutSeconds*time.Second)
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

// cacheBlock caches a block to Redis, maintaining the latest 1000 blocks.
func cacheBlock(ctx context.Context, block *models.Block) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	// Serialize block to JSON
	blockJSON, err := sonic.Marshal(block)
	if err != nil {
		global.Logger.Warnf("failed to marshal block to JSON: %v", err)
		return
	}

	// Add to list and maintain size
	pipe := redisClient.Pipeline()
	pipe.LPush(ctx, CacheKeyBlockList, blockJSON)
	pipe.LTrim(ctx, CacheKeyBlockList, 0, CacheMaxSize-1)
	// Also cache by height for direct lookup with TTL to prevent memory leak
	pipe.Set(ctx, CacheKeyBlockPrefix+fmt.Sprintf("%d", block.Height), blockJSON, CacheBlockTTL)
	_, err = pipe.Exec(ctx)
	if err != nil {
		global.Logger.Warnf("failed to cache block to Redis: %v", err)
	}
}

// cacheTransaction caches a transaction to Redis using Sorted Set for automatic deduplication.
// Uses tx_number as score to maintain chronological order. Duplicate transactions (same hash)
// are automatically overwritten with the latest data.
// Maintains the latest 1000 transactions by removing old entries.
func cacheTransaction(ctx context.Context, tx *models.Transaction) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	// Serialize transaction to JSON
	txJSON, err := sonic.Marshal(tx)
	if err != nil {
		global.Logger.Warnf("failed to marshal transaction to JSON: %v", err)
		return
	}

	// Use Sorted Set with composite score for automatic deduplication and ordering
	// Member is the JSON string itself (automatically deduplicated by content)
	// Score = block_number * 1000000 + tx_number for chronological ordering
	// This ensures:
	// - Different blocks have different score ranges
	// - Transactions within the same block are ordered by tx_number (tx_index + 1, per-block)
	// - Overall ordering: newest blocks first, then by tx_number within each block
	// Formula: block_number * 1000000 + tx_number (1000000 allows up to 1M txs per block)
	compositeScore := float64(tx.BlockNumber)*1000000 + float64(tx.TxNumber)
	pipe := redisClient.Pipeline()
	pipe.ZAdd(ctx, CacheKeyTransactionZSet, redis.Z{
		Score:  compositeScore,
		Member: string(txJSON),
	})
	// Maintain only the latest CacheMaxSize entries (remove old ones with lowest scores)
	// Use -int64(CacheMaxSize)-1 to keep elements from rank -CacheMaxSize to -1 (newest CacheMaxSize entries)
	pipe.ZRemRangeByRank(ctx, CacheKeyTransactionZSet, 0, -int64(CacheMaxSize)-1)
	// Set expiration for the sorted set
	pipe.Expire(ctx, CacheKeyTransactionZSet, CacheTransactionTTL)
	// Also cache by hash for direct lookup with TTL to prevent memory leak
	pipe.Set(ctx, CacheKeyTransactionPrefix+tx.Hash, txJSON, CacheTransactionTTL)
	_, err = pipe.Exec(ctx)
	if err != nil {
		global.Logger.Warnf("failed to cache transaction to Redis: %v", err)
	}
}

// saveCrossChainTransaction extracts and saves cross-chain transaction details.
// It parses IBC transfers, Peggy deposits, and Peggy withdrawals from the transaction response.
func (s *SyncService) saveCrossChainTransaction(ctx context.Context, txResp *tx.GetTxResponse, txModel *models.Transaction) error {
	if txResp == nil || txResp.TxResponse == nil {
		return nil
	}

	// Parse transaction messages to extract cross-chain information
	var txData models.TxJSON
	if err := sonic.UnmarshalString(txModel.Messages, &txData); err != nil {
		global.Logger.Debugf("failed to unmarshal messages for cross-chain extraction: %v", err)
		return nil // Don't fail, just skip
	}

	// Extract logs/events for additional information
	logsBytes := []byte(txModel.Logs)
	if len(logsBytes) > 0 {
		var logs []models.LogRaw
		if err := sonic.Unmarshal(logsBytes, &logs); err == nil {
			txData.Logs = logs
		}
	}

	// Process each message for cross-chain transactions
	for _, msg := range txData.Messages {
		switch msg.Type {
		case "/ibc.applications.transfer.v1.MsgTransfer":
			if err := s.saveIBCTransferFromMessage(ctx, msg.Message, &txData, txModel); err != nil {
				global.Logger.Warnf("failed to save IBC transfer: %v", err)
			}

		case "/injective.peggy.v1.MsgDepositClaim":
			if err := s.savePeggyDepositFromMessage(ctx, msg.Message, &txData, txModel); err != nil {
				global.Logger.Warnf("failed to save Peggy deposit: %v", err)
			}

		case "/injective.peggy.v1.MsgSendToEth":
			if err := s.savePeggyWithdrawalFromMessage(ctx, msg.Message, &txData, txModel); err != nil {
				global.Logger.Warnf("failed to save Peggy withdrawal: %v", err)
			}
		}
	}

	return nil
}

// saveIBCTransferFromMessage extracts and saves IBC transfer details.
func (s *SyncService) saveIBCTransferFromMessage(ctx context.Context, msg json.RawMessage, txData *models.TxJSON, txModel *models.Transaction) error {
	var ibcMsg struct {
		Sender   string `json:"sender"`
		Receiver string `json:"receiver"`
		Token    struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"token"`
		SourcePort       string          `json:"source_port"`
		SourceChannel    string          `json:"source_channel"`
		TimeoutHeight    json.RawMessage `json:"timeout_height"`
		TimeoutTimestamp string          `json:"timeout_timestamp"`
	}

	if err := sonic.Unmarshal(msg, &ibcMsg); err != nil {
		return errors.Wrap(err, "failed to unmarshal IBC MsgTransfer")
	}

	// Extract event information
	ibcDetail := &models.IBCTransferDetail{
		SourcePort:    ibcMsg.SourcePort,
		SourceChannel: ibcMsg.SourceChannel,
	}

	if len(ibcMsg.TimeoutHeight) > 0 {
		ibcDetail.TimeoutHeight = string(ibcMsg.TimeoutHeight)
	}

	if ibcMsg.TimeoutTimestamp != "" {
		if ts, err := parseUint64(ibcMsg.TimeoutTimestamp); err == nil {
			ibcDetail.TimeoutTimestamp = ts
		}
	}

	// Extract info from events
	for _, log := range txData.Logs {
		for _, event := range log.Events {
			switch event.Type {
			case "send_packet":
				for _, attr := range event.Attributes {
					key := attr.Key
					value := stringifyAttrValue(attr.Value)
					switch key {
					case "packet_sequence":
						if seq, err := parseUint64(value); err == nil {
							ibcDetail.PacketSequence = seq
						}
					case "packet_timeout_height":
						ibcDetail.TimeoutHeight = value
					case "packet_timeout_timestamp":
						if ts, err := parseUint64(value); err == nil {
							ibcDetail.TimeoutTimestamp = ts
						}
					case "packet_data":
						ibcDetail.DataHex = []byte(value)
					}
				}
			case "recv_packet":
				for _, attr := range event.Attributes {
					key := attr.Key
					value := stringifyAttrValue(attr.Value)
					if key == "packet_dst_port" {
						ibcDetail.DestinationPort = value
					} else if key == "packet_dst_channel" {
						ibcDetail.DestinationChannel = value
					}
				}
			}
		}
	}

	// Create and save IBC transfer record
	ibcTx := &models.IBCTransferTx{
		Sender:             ibcMsg.Sender,
		Receiver:           ibcMsg.Receiver,
		SourcePort:         ibcDetail.SourcePort,
		SourceChannel:      ibcDetail.SourceChannel,
		DestinationPort:    ibcDetail.DestinationPort,
		DestinationChannel: ibcDetail.DestinationChannel,
		Amount:             ibcMsg.Token.Amount,
		Denom:              ibcMsg.Token.Denom,
		TimeoutHeight:      ibcDetail.TimeoutHeight,
		TimeoutTimestamp:   ibcDetail.TimeoutTimestamp,
		PacketSequence:     ibcDetail.PacketSequence,
		DataHex:            ibcDetail.DataHex,
		State:              "completed",
		TxHash:             txModel.Hash,
		BlockHeight:        txModel.BlockNumber,
		BlockTimestamp:     txModel.BlockUnixTimestamp,
	}

	return db.Get().WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tx_hash"}, {Name: "packet_sequence"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"sender", "receiver", "source_port", "source_channel",
			"destination_port", "destination_channel", "amount", "denom",
			"timeout_height", "timeout_timestamp", "state", "updated_at",
		}),
	}).Create(ibcTx).Error
}

// savePeggyDepositFromMessage extracts and saves Peggy deposit details.
func (s *SyncService) savePeggyDepositFromMessage(ctx context.Context, msg json.RawMessage, txData *models.TxJSON, txModel *models.Transaction) error {
	var depositMsg struct {
		EventNonce        string `json:"event_nonce"`
		EthSender         string `json:"eth_sender"`
		InjectiveReceiver string `json:"injective_receiver"`
		Amount            struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"amount"`
	}

	if err := sonic.Unmarshal(msg, &depositMsg); err != nil {
		return errors.Wrap(err, "failed to unmarshal Peggy MsgDepositClaim")
	}

	// Extract event information
	depositDetail := &models.PeggyDepositDetail{
		ClaimType: 1, // DepositClaim
	}

	if depositMsg.EventNonce != "" {
		if nonce, err := parseUint64(depositMsg.EventNonce); err == nil {
			depositDetail.EventNonce = nonce
		}
	}

	// Extract info from events
	for _, log := range txData.Logs {
		for _, event := range log.Events {
			if event.Type == "deposit_claim" {
				for _, attr := range event.Attributes {
					key := attr.Key
					value := stringifyAttrValue(attr.Value)
					switch key {
					case "event_nonce":
						if nonce, err := parseUint64(value); err == nil {
							depositDetail.EventNonce = nonce
						}
					case "event_height":
						if height, err := parseUint64(value); err == nil {
							depositDetail.EventHeight = height
						}
					case "orchestrator_address":
						depositDetail.OrchestratorAddress = value
					}
				}
			}
		}
	}

	// Create and save Peggy deposit record
	deposit := &models.PeggyDepositTx{
		Sender:              depositMsg.EthSender,
		Receiver:            depositMsg.InjectiveReceiver,
		Amount:              depositMsg.Amount.Amount,
		Denom:               depositMsg.Amount.Denom,
		EventNonce:          depositDetail.EventNonce,
		EventHeight:         depositDetail.EventHeight,
		OrchestratorAddress: depositDetail.OrchestratorAddress,
		State:               "completed",
		ClaimType:           depositDetail.ClaimType,
		TxHash:              txModel.Hash,
		BlockHeight:         txModel.BlockNumber,
		BlockTimestamp:      txModel.BlockUnixTimestamp,
	}

	return db.Get().WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "event_nonce"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"sender", "receiver", "amount", "denom",
			"event_height", "orchestrator_address", "state", "updated_at",
		}),
	}).Create(deposit).Error
}

// savePeggyWithdrawalFromMessage extracts and saves Peggy withdrawal details.
func (s *SyncService) savePeggyWithdrawalFromMessage(ctx context.Context, msg json.RawMessage, txData *models.TxJSON, txModel *models.Transaction) error {
	var withdrawalMsg struct {
		Sender  string `json:"sender"`
		EthDest string `json:"eth_dest"`
		Amount  struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"amount"`
		BridgeFee struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"bridge_fee"`
	}

	if err := sonic.Unmarshal(msg, &withdrawalMsg); err != nil {
		return errors.Wrap(err, "failed to unmarshal Peggy MsgSendToEth")
	}

	// Extract event information
	withdrawalDetail := &models.PeggyWithdrawalDetail{
		BridgeFee: withdrawalMsg.BridgeFee.Amount,
		ClaimType: 2, // WithdrawalClaim
	}

	// Extract info from events
	for _, log := range txData.Logs {
		for _, event := range log.Events {
			switch event.Type {
			case "send_to_eth":
				for _, attr := range event.Attributes {
					key := attr.Key
					value := stringifyAttrValue(attr.Value)
					switch key {
					case "outgoing_tx_id":
						if id, err := parseUint64(value); err == nil {
							withdrawalDetail.OutgoingTxId = id
						}
					case "batch_timeout":
						if ts, err := parseUint64(value); err == nil {
							withdrawalDetail.BatchTimeout = ts
						}
					}
				}
			case "withdrawal_claim":
				for _, attr := range event.Attributes {
					key := attr.Key
					value := stringifyAttrValue(attr.Value)
					switch key {
					case "event_nonce":
						if nonce, err := parseUint64(value); err == nil {
							withdrawalDetail.EventNonce = nonce
						}
					case "event_height":
						if height, err := parseUint64(value); err == nil {
							withdrawalDetail.EventHeight = height
						}
					case "batch_nonce":
						if nonce, err := parseUint64(value); err == nil {
							withdrawalDetail.BatchNonce = nonce
						}
					case "orchestrator_address":
						withdrawalDetail.OrchestratorAddress = value
					}
				}
			}
		}
	}

	// Determine state based on available information
	state := "pending"
	if withdrawalDetail.BatchNonce > 0 {
		state = "completed"
	} else if withdrawalDetail.BatchTimeout > 0 {
		state = "batched"
	}

	// Create and save Peggy withdrawal record
	withdrawal := &models.PeggyWithdrawalTx{
		Sender:              withdrawalMsg.Sender,
		Receiver:            withdrawalMsg.EthDest,
		Amount:              withdrawalMsg.Amount.Amount,
		Denom:               withdrawalMsg.Amount.Denom,
		BridgeFee:           withdrawalDetail.BridgeFee,
		OutgoingTxId:        withdrawalDetail.OutgoingTxId,
		BatchTimeout:        withdrawalDetail.BatchTimeout,
		BatchNonce:          withdrawalDetail.BatchNonce,
		OrchestratorAddress: withdrawalDetail.OrchestratorAddress,
		EventNonce:          withdrawalDetail.EventNonce,
		EventHeight:         withdrawalDetail.EventHeight,
		State:               state,
		ClaimType:           withdrawalDetail.ClaimType,
		TxHash:              txModel.Hash,
		BlockHeight:         txModel.BlockNumber,
		BlockTimestamp:      txModel.BlockUnixTimestamp,
	}

	return db.Get().WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "outgoing_tx_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"sender", "receiver", "amount", "denom", "bridge_fee",
			"batch_timeout", "batch_nonce", "orchestrator_address",
			"event_nonce", "event_height", "state", "updated_at",
		}),
	}).Create(withdrawal).Error
}

// parseUint64 safely parses a uint64 from string.
func parseUint64(s string) (uint64, error) {
	if s == "" {
		return 0, nil
	}
	var result uint64
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

// stringifyAttrValue converts an attribute value to string.
// Handles both string and []interface{} types.
func stringifyAttrValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case []interface{}:
		// Join array elements
		var parts []string
		for _, item := range val {
			if s := stringifyAttrValue(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprintf("%v", val)
	}
}
