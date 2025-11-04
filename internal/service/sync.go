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

const (
	// Redis queue key for transaction hashes
	txHashQueueKey = "tx:hash:queue"
	// Block polling interval
	blockPollInterval = 5 * time.Second
	// Transaction processing interval
	txProcessInterval = 500 * time.Millisecond
	// Number of concurrent transaction processors
	concurrentTxProcessors = 5
	// Maximum retry attempts for RPC calls
	maxRetryAttempts = 3
	// Initial retry delay
	retryDelay = 2 * time.Second
)

// SyncService handles blockchain synchronization tasks.
// It monitors new blocks, stores block data, processes transactions,
// and updates account information.
type SyncService struct {
	// Clients for blockchain interactions
	explorerClient explorerclient.ExplorerClient
	chainClient    chainclient.ChainClientV2
	tmClient       tmclient.TendermintClient

	// Current processed block height
	currentHeight uint64
	heightMutex   sync.RWMutex

	// Control channels
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Client initialization
	explorerClientOnce sync.Once
	chainClientOnce    sync.Once
	tmClientOnce       sync.Once

	// Errors from client initialization
	explorerClientErr error
	chainClientErr    error
}

// NewSyncService creates a new instance of SyncService.
//
// Returns:
//   - *SyncService: A new sync service instance
func NewSyncService() *SyncService {
	ctx, cancel := context.WithCancel(context.Background())
	return &SyncService{
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
	for i := 0; i < concurrentTxProcessors; i++ {
		s.wg.Add(1)
		go s.transactionProcessor(i)
	}

	global.Logger.Infof("blockchain sync service started, current height: %d, %d transaction processors", s.getCurrentHeight(), concurrentTxProcessors)
	return nil
}

// Stop stops the blockchain synchronization service.
func (s *SyncService) Stop() error {
	global.Logger.Infof("stopping blockchain sync service")
	s.cancel()
	s.wg.Wait()
	global.Logger.Infof("blockchain sync service stopped")
	return nil
}

// initClients initializes all blockchain clients.
func (s *SyncService) initClients() error {
	// Initialize explorer client
	s.explorerClientOnce.Do(func() {
		client, err := explorerclient.NewExplorerClient(global.ChainConfig)
		if err != nil {
			s.explorerClientErr = errors.Wrap(err, "create explorer client error")
			return
		}
		s.explorerClient = client
	})

	if s.explorerClientErr != nil {
		return s.explorerClientErr
	}

	// Initialize chain client
	s.chainClientOnce.Do(func() {
		client, err := buildChainClientV2Query()
		if err != nil {
			s.chainClientErr = errors.Wrap(err, "build chain client error")
			return
		}
		s.chainClient = client
	})

	if s.chainClientErr != nil {
		return s.chainClientErr
	}

	// Initialize Tendermint client
	s.tmClientOnce.Do(func() {
		s.tmClient = tmclient.NewRPCClient(global.ChainConfig.TmEndpoint)
	})

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
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
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

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
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

	// Sync backwards from latest to current height
	// We'll process blocks in reverse order to ensure we have the most recent ones first
	for height := latestHeight; height > currentHeight; height-- {
		// Check if context is cancelled
		select {
		case <-s.ctx.Done():
			global.Logger.Infof("context cancelled during backward sync at height %d", height)
			return s.ctx.Err()
		default:
		}

		// Check if block already exists in database
		var existingBlock models.Block
		err := db.Get().Where("height = ?", height).First(&existingBlock).Error
		if err == nil {
			// Block exists, verify it matches
			if existingBlock.BlockHash != "" {
				// Verify the hash matches
				ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
				blockResp, err := s.explorerClient.GetBlock(ctx, fmt.Sprintf("%d", height))
				cancel()

				if err == nil && blockResp != nil && blockResp.Data != nil {
					chainBlock := blockResp.Data
					if existingBlock.BlockHash == chainBlock.BlockHash {
						// Hash matches, block is already synced correctly
						global.Logger.Debugf("block at height %d already exists and matches, skipping", height)
						continue
					}
					// Hash doesn't match, need to update
					global.Logger.Debugf("block at height %d exists but hash doesn't match, updating", height)
				}
			}
		}

		// Process the block
		if err := s.processBlock(height); err != nil {
			// Check if error is due to context cancellation
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
				strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "context deadline exceeded") {
				return err
			}
			global.Logger.Errorf("error processing block %d during backward sync: %v", height, err)
			// Continue processing other blocks even if one fails
			continue
		}

		// Update current height as we process
		s.setCurrentHeight(height)

		// Log progress every 100 blocks
		if (latestHeight-height)%100 == 0 {
			global.Logger.Infof("backward sync progress: %d/%d blocks processed", latestHeight-height+1, latestHeight-currentHeight)
		}
	}

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

	ticker := time.NewTicker(blockPollInterval)
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
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// Get block information with retry logic
	var blockResp *injective_explorer_rpcpb.GetBlockResponse

	delay := retryDelay
	var err error
	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
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
			if attempt < maxRetryAttempts-1 {
				global.Logger.Warnf("retryable error getting block at height %d (attempt %d/%d): %v, retrying in %v...",
					height, attempt+1, maxRetryAttempts, err, delay)
				time.Sleep(delay)
				delay *= 2 // Exponential backoff
				continue
			}
		}

		// Non-retryable error or max retries reached
		return errors.Wrapf(err, "failed to get block at height %d", height)
	}

	if err != nil {
		return errors.Wrapf(err, "failed to get block at height %d after %d attempts", height, maxRetryAttempts)
	}

	if blockResp == nil || blockResp.Data == nil {
		return errors.Errorf("block response is nil for height %d", height)
	}

	block := blockResp.Data

	// Save block to database
	blockModel := models.BlockFromProto(block)
	if blockModel == nil {
		return errors.Errorf("failed to convert block at height %d", height)
	}

	// Check if block already exists
	var existingBlock models.Block
	err = db.Get().Where("height = ?", height).First(&existingBlock).Error
	if err == nil {
		// Block exists, update it
		err = db.Get().Model(&existingBlock).Updates(blockModel).Error
		if err != nil {
			return errors.Wrapf(err, "failed to update block at height %d", height)
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		// Block doesn't exist, create it
		err = db.Get().Create(blockModel).Error
		if err != nil {
			return errors.Wrapf(err, "failed to create block at height %d", height)
		}
	} else {
		return errors.Wrapf(err, "failed to check block existence at height %d", height)
	}

	// Extract transaction hashes and push to Redis queue
	if len(block.Txs) > 0 {
		redisClient := cache.GetRedisClient()
		if redisClient != nil {
			for _, txHash := range block.Txs {
				// Push to Redis queue
				err = redisClient.LPush(ctx, txHashQueueKey, txHash.Hash).Err()
				if err != nil {
					global.Logger.Errorf("failed to push tx hash %s to queue: %v", txHash.Hash, err)
					// Continue processing other transactions
					continue
				}
			}
		} else {
			global.Logger.Warnf("redis client is nil, skipping tx hash queue")
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
					time.Sleep(txProcessInterval)
				} else {
					// Real error occurred
					errorCount++
					global.Logger.Errorf("transaction processor %d: error processing transaction (error count: %d): %v", id, errorCount, err)
					// Wait a bit before retrying to avoid hammering on errors
					time.Sleep(txProcessInterval)
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
func (s *SyncService) processTransaction(redisClient *redis.Client) error {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// Pop transaction hash from queue (blocking pop with timeout)
	txHash, err := redisClient.RPop(ctx, txHashQueueKey).Result()
	if err != nil {
		// Check for empty queue error (this is expected when queue is empty)
		// redis.Nil is returned when the queue is empty
		if err == redis.Nil || err.Error() == "redis: nil" || strings.Contains(err.Error(), "redis: nil") {
			// Queue is empty, this is normal - don't log as error
			return errQueueEmpty
		}
		global.Logger.Errorf("failed to pop from tx hash queue: %v", err)
		return errors.Wrap(err, "failed to pop from tx hash queue")
	}

	// Get transaction information with retry logic
	var txResp *injective_explorer_rpcpb.GetTxByTxHashResponse

	delay := retryDelay
	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
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
			if attempt < maxRetryAttempts-1 {
				global.Logger.Warnf("retryable error getting transaction %s (attempt %d/%d): %v, retrying in %v...",
					txHash, attempt+1, maxRetryAttempts, err, delay)
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
		global.Logger.Warnf("failed to get transaction %s after %d attempts: %v", txHash, maxRetryAttempts, err)
		return nil
	}

	if txResp == nil || txResp.Data == nil {
		global.Logger.Warnf("transaction response is nil for hash %s", txHash)
		return nil
	}

	txData := txResp.Data

	// Save transaction to database
	txModel := models.TransactionFromProto(txData)
	if txModel == nil {
		global.Logger.Errorf("failed to convert transaction %s to model", txHash)
		return errors.Errorf("failed to convert transaction %s", txHash)
	}

	// Check if transaction already exists
	var existingTx models.Transaction
	err = db.Get().Where("hash = ?", txHash).First(&existingTx).Error
	if err == nil {
		// Transaction exists, update it
		err = db.Get().Model(&existingTx).Updates(txModel).Error
		if err != nil {
			global.Logger.Errorf("failed to update transaction %s in database: %v", txHash, err)
			return errors.Wrapf(err, "failed to update transaction %s", txHash)
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		// Transaction doesn't exist, create it
		err = db.Get().Create(txModel).Error
		if err != nil {
			global.Logger.Errorf("failed to create transaction %s in database: %v", txHash, err)
			return errors.Wrapf(err, "failed to create transaction %s", txHash)
		}
	} else {
		global.Logger.Errorf("failed to check transaction %s existence: %v", txHash, err)
		return errors.Wrapf(err, "failed to check transaction existence %s", txHash)
	}

	// Extract and update account information from transaction
	if err := s.updateAccountsFromTransaction(ctx, txData); err != nil {
		global.Logger.Warnf("failed to update accounts from transaction %s: %v", txHash, err)
		// Don't fail the transaction processing if account update fails
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

	// Update accounts in database
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


		// Save or update account in database
		accountModel := models.AccountFromProto(ethAccount, address)
		if accountModel == nil {
			global.Logger.Warnf("failed to convert account %s to model", address)
			continue
		}

		var existingAccount models.Account
		err = db.Get().Where("address = ?", address).First(&existingAccount).Error
		if err == nil {
			// Account exists, update it
			err = db.Get().Model(&existingAccount).Updates(accountModel).Error
			if err != nil {
				global.Logger.Warnf("failed to update account %s: %v", address, err)
			}
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
