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

	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/bytedance/sonic"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EVMDetailsSyncService handles EVM transaction details synchronization.
type EVMDetailsSyncService struct {
	ctx              context.Context
	cancel           context.CancelFunc
	config           *EVMDetailsSyncConfig
	evmClient        *chain.EVMClient
	redisClient      *redis.Client
	wg               sync.WaitGroup
	taskDeduplicator *RequestCoalescer // Request coalescer for task deduplication
	// Batch update buffer
	updateBuffer      []*EVMTransactionUpdate
	updateBufferMutex sync.Mutex
	updateBufferSize  int
	flushTicker       *time.Ticker
}

// EVMTransactionUpdate represents an EVM transaction update for batch processing.
type EVMTransactionUpdate struct {
	ID                   uint
	TransactionHash      string
	EVMDetailsSynced     bool
	EVMDetailsSyncAt     *time.Time
	EVMDetailsSyncError  string
	EVMFromAddress       string
	EVMToAddress         string
	EVMDetailsStorageKey string
	RawDetailsJSON       string
	UpdatedAt            time.Time
}

// EVMDetailsSyncConfig holds the EVM details sync service configuration.
type EVMDetailsSyncConfig struct {
	QueueKey           string
	WorkerCount        int
	BatchSize          int
	RetryDelay         time.Duration
	MaxRetryAttempts   int
	ProcessInterval    time.Duration
	Timeout            time.Duration
	BatchUpdateSize    int           // Batch update buffer size
	BatchFlushInterval time.Duration // Batch update flush interval
}

// NewEVMDetailsSyncService creates a new instance of EVMDetailsSyncService.
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
			networkName = "mainnet"
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
		taskDeduplicator: NewRequestCoalescer(
			EVMDetailsTaskMaxAge,
			EVMDetailsTaskCleanupInterval,
			EVMDetailsTaskMaxSize,
		),
		updateBuffer:     make([]*EVMTransactionUpdate, 0, config.BatchUpdateSize),
		updateBufferSize: config.BatchUpdateSize,
		flushTicker:      time.NewTicker(config.BatchFlushInterval),
	}, nil
}

// Start starts the EVM details sync service.
// It starts worker goroutines to process EVM transaction details tasks.
func (s *EVMDetailsSyncService) Start() {
	for i := 0; i < s.config.WorkerCount; i++ {
		s.wg.Add(1)
		go s.worker(i)
	}
	// Start background worker to periodically check and update missing signatures/signer
	s.wg.Add(1)
	go s.periodicSignerUpdateWorker()
	// Start task deduplication metrics logger
	s.wg.Add(1)
	go s.logTaskDeduplicationMetrics()
	// Start batch update flusher
	s.wg.Add(1)
	go s.batchUpdateFlusher()
	global.Logger.Infof("EVM details sync service started with %d workers", s.config.WorkerCount)
}

// Stop stops the EVM details sync service.
// It cancels the context and waits for all goroutines to finish.
func (s *EVMDetailsSyncService) Stop() {
	s.cancel()

	// Flush any remaining updates in the buffer
	s.flushUpdateBuffer()

	// Stop flush ticker
	if s.flushTicker != nil {
		s.flushTicker.Stop()
	}

	// Stop RequestCoalescer cleanup goroutine
	if s.taskDeduplicator != nil {
		s.taskDeduplicator.Stop()
	}

	if s.evmClient != nil {
		s.evmClient.Close()
	}
	s.wg.Wait()
	global.Logger.Info("EVM details sync service stopped")
}

// worker is a worker goroutine that processes EVM transaction details tasks.
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
// Uses batch RPC calls to fetch multiple transaction details at once, significantly reducing RPC overhead.
func (s *EVMDetailsSyncService) processBatch(workerID int) {
	ctx, cancel := context.WithTimeout(s.ctx, s.config.Timeout)
	defer cancel()

	// Batch fetch tasks using LRANGE + LTRIM for efficient batch processing
	// This is much faster than multiple BRPop calls with 1s minimum timeout
	tasks, err := s.batchPopTasks(ctx, workerID)
	if err != nil {
		global.Logger.Warnf("worker %d: failed to batch pop tasks: %v", workerID, err)
		return
	}

	if len(tasks) == 0 {
		return
	}

	global.Logger.Debugf("worker %d: processing %d EVM details tasks", workerID, len(tasks))

	// Parse all tasks into structured data
	type taskData struct {
		cosmosHash string
		evmHash    string
		taskStr    string
	}
	taskList := make([]taskData, 0, len(tasks))
	evmHashes := make([]string, 0, len(tasks))

	for _, taskStr := range tasks {
		parts := strings.Split(taskStr, ":")
		if len(parts) != 2 {
			global.Logger.Warnf("worker %d: invalid task format: %s", workerID, taskStr)
			continue
		}
		taskList = append(taskList, taskData{
			cosmosHash: parts[0],
			evmHash:    parts[1],
			taskStr:    taskStr,
		})
		evmHashes = append(evmHashes, parts[1])
	}

	if len(taskList) == 0 {
		return
	}

	// Fetch all EVM details in batch (significantly reduces RPC calls)
	// Instead of N calls to GetTransactionDetails, we make 1 batch call
	detailsList, errorsList := s.fetchEVMDetailsBatch(ctx, evmHashes)

	// Process tasks concurrently with pre-fetched details
	var wg sync.WaitGroup
	for i, task := range taskList {
		wg.Add(1)
		go func(idx int, t taskData) {
			defer wg.Done()

			var details *chain.TransactionDetails
			var fetchErr error

			// Use pre-fetched details if available
			if idx < len(detailsList) {
				details = detailsList[idx]
			}
			if idx < len(errorsList) {
				fetchErr = errorsList[idx]
			}

			// Process with pre-fetched details
			s.processTaskWithDetails(ctx, t.cosmosHash, t.evmHash, details, fetchErr, workerID)
		}(i, task)
	}
	wg.Wait()
}

// batchPopTasks fetches and removes a batch of tasks from the Redis queue efficiently.
// Uses LRANGE + LTRIM in a pipeline for atomic batch fetching (much faster than multiple BRPop calls).
func (s *EVMDetailsSyncService) batchPopTasks(ctx context.Context, workerID int) ([]string, error) {
	// Use LRANGE to get up to BatchSize items, then LTRIM to remove them
	// This is done atomically using a pipeline
	pipe := s.redisClient.Pipeline()

	// LRANGE queue 0 -1 returns all items, we limit to BatchSize
	lrangeCmd := pipe.LRange(ctx, s.config.QueueKey, 0, int64(s.config.BatchSize-1))
	// LTRIM queue BatchSize -1 removes the fetched items
	ltrimCmd := pipe.LTrim(ctx, s.config.QueueKey, int64(s.config.BatchSize), -1)

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, errors.Wrap(err, "failed to execute batch pop pipeline")
	}

	// Get results
	tasks, err := lrangeCmd.Result()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get LRANGE result")
	}

	// LTRIM result is "OK" on success
	_, err = ltrimCmd.Result()
	if err != nil && err != redis.Nil {
		return nil, errors.Wrap(err, "failed to trim queue")
	}

	return tasks, nil
}

// processTask processes a single EVM transaction details task.
func (s *EVMDetailsSyncService) processTask(ctx context.Context, taskStr string, workerID int) {
	// Parse task: format is "cosmosHash:evmHash"
	parts := strings.Split(taskStr, ":")
	if len(parts) != 2 {
		global.Logger.Warnf("worker %d: invalid task format: %s", workerID, taskStr)
		return
	}

	cosmosHash := parts[0]
	evmHash := parts[1]

	// Use RequestCoalescer to deduplicate concurrent processing of the same transaction
	// This prevents concurrent processing which can cause deadlocks and wasted resources
	_, err := s.taskDeduplicator.Do(ctx, cosmosHash, func() (interface{}, error) {
		return nil, s.processTaskInternal(ctx, cosmosHash, evmHash, workerID)
	})

	if err != nil {
		// Context errors are expected during shutdown
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			global.Logger.Errorf("worker %d: unexpected error from task deduplicator for %s: %v", workerID, cosmosHash, err)
		}
	}
}

// processTaskInternal is the core logic for processing a single EVM transaction details task.
// This is called by processTask after deduplication check.
func (s *EVMDetailsSyncService) processTaskInternal(ctx context.Context, cosmosHash, evmHash string, workerID int) error {
	// Fetch EVM transaction details (for non-batch path)
	details, err := s.fetchEVMDetails(ctx, evmHash)
	if err != nil {
		return s.handleFetchError(ctx, cosmosHash, evmHash, err, workerID)
	}

	return s.saveAndProcessDetails(ctx, cosmosHash, evmHash, details, workerID)
}

// processTaskWithDetails processes a task using pre-fetched transaction details.
// This is used by batch RPC processing to avoid redundant fetches.
func (s *EVMDetailsSyncService) processTaskWithDetails(
	ctx context.Context,
	cosmosHash, evmHash string,
	details *chain.TransactionDetails,
	fetchErr error,
	workerID int,
) {
	// Use RequestCoalescer to deduplicate concurrent processing of the same transaction
	_, err := s.taskDeduplicator.Do(ctx, cosmosHash, func() (interface{}, error) {
		if fetchErr != nil {
			return nil, s.handleFetchError(ctx, cosmosHash, evmHash, fetchErr, workerID)
		}
		return nil, s.saveAndProcessDetails(ctx, cosmosHash, evmHash, details, workerID)
	})

	if err != nil {
		// Context errors are expected during shutdown
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			global.Logger.Errorf("worker %d: unexpected error from task deduplicator for %s: %v", workerID, cosmosHash, err)
		}
	}
}

// handleFetchError handles errors from fetching EVM details.
func (s *EVMDetailsSyncService) handleFetchError(ctx context.Context, cosmosHash, evmHash string, fetchErr error, workerID int) error {
	// Check if EVM transaction exists in evm_transactions table
	var evmTx models.EVMTransaction
	err := db.Get().WithContext(ctx).
		Where("transaction_hash = ?", cosmosHash).
		First(&evmTx).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			global.Logger.Warnf("worker %d: EVM transaction %s not found in evm_transactions table, skipping", workerID, cosmosHash)
			return nil
		}
		global.Logger.Errorf("worker %d: failed to query EVM transaction %s: %v", workerID, cosmosHash, err)
		s.retryTask(ctx, cosmosHash+":"+evmHash)
		return err
	}

	// Categorize and enhance error message
	errorType := categorizeError(fetchErr)
	enhancedErrMsg := fmt.Sprintf("[%s] %v", errorType, fetchErr)

	global.Logger.Warnf("worker %d: failed to fetch EVM details for %s (evm: %s): %s",
		workerID, cosmosHash, evmHash, enhancedErrMsg)

	// Update error information
	s.updateSyncError(ctx, cosmosHash, enhancedErrMsg)

	// IMPORTANT: Do NOT clear Redis cache on failure
	s.preserveTransactionCache(ctx, cosmosHash)

	// Retry if should retry
	if s.shouldRetryEVM(&evmTx, fetchErr) {
		s.retryTask(ctx, cosmosHash+":"+evmHash)
	}

	return fetchErr
}

// saveAndProcessDetails saves fetched transaction details and processes them.
func (s *EVMDetailsSyncService) saveAndProcessDetails(
	ctx context.Context,
	cosmosHash, evmHash string,
	details *chain.TransactionDetails,
	workerID int,
) error {
	// Check if EVM transaction exists in evm_transactions table
	var evmTx models.EVMTransaction
	err := db.Get().WithContext(ctx).
		Where("transaction_hash = ?", cosmosHash).
		First(&evmTx).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			global.Logger.Warnf("worker %d: EVM transaction %s not found in evm_transactions table, skipping", workerID, cosmosHash)
			return nil
		}
		global.Logger.Errorf("worker %d: failed to query EVM transaction %s: %v", workerID, cosmosHash, err)
		// Retry task
		s.retryTask(ctx, cosmosHash+":"+evmHash)
		return err
	}

	// Skip if already synced and data exists
	// Check if data actually exists: either in database (raw_details_json) or in object storage (evm_details_storage_key)
	if evmTx.EVMDetailsSynced {
		// Check if data exists in database or object storage
		hasDataInDB := evmTx.RawDetailsJSON != ""
		hasDataInStorage := evmTx.EVMDetailsStorageKey != ""

		// Check object storage or database based on configuration
		if storage.IsEnabled() {
			if hasDataInStorage {
				// Data exists in object storage, skip
				return nil
			}
			// Data missing in object storage, need to re-sync
			global.Logger.Infof("worker %d: EVM transaction %s marked as synced but data missing in object storage, re-syncing", workerID, cosmosHash)
		} else {
			if hasDataInDB {
				// Data exists in database, skip
				return nil
			}
			// Data missing in database, need to re-sync
			global.Logger.Infof("worker %d: EVM transaction %s marked as synced but raw_details_json is empty (object storage disabled), re-syncing", workerID, cosmosHash)
		}
		// Reset sync status to allow re-sync
		// Use optimistic locking to prevent concurrent update issues
		result := db.Get().WithContext(ctx).
			Model(&evmTx).
			Where("id = ? AND transaction_hash = ?", evmTx.ID, cosmosHash).
			Update("evm_details_synced", false)

		if result.Error != nil {
			global.Logger.Warnf("worker %d: failed to reset sync status for %s: %v", workerID, cosmosHash, result.Error)
			// Continue anyway, will retry on next attempt
		}

		if result.RowsAffected == 0 {
			// Record was not found or already updated by another worker
			global.Logger.Debugf("worker %d: sync status reset for %s affected 0 rows, skipping", workerID, cosmosHash)
			return nil
		}
	}

	// Save transaction details to evm_transactions table
	if err := s.saveEVMDetails(ctx, &evmTx, details); err != nil {
		global.Logger.Errorf("worker %d: failed to save EVM details for %s: %v", workerID, cosmosHash, err)

		// IMPORTANT: Do NOT clear Redis cache on save failure
		// Preserve the transaction cache so it can be retried later
		s.preserveTransactionCache(ctx, cosmosHash)

		s.retryTask(ctx, cosmosHash+":"+evmHash)
		return err
	}

	global.Logger.Debugf("worker %d: successfully synced EVM details for %s", workerID, cosmosHash)
	return nil
}

// fetchEVMDetails fetches EVM transaction details from EVM RPC.
func (s *EVMDetailsSyncService) fetchEVMDetails(ctx context.Context, evmHash string) (*chain.TransactionDetails, error) {
	hash := ethcommon.HexToHash(evmHash)
	details, err := s.evmClient.GetTransactionDetails(ctx, hash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get transaction details for %s", evmHash)
	}
	return details, nil
}

// fetchEVMDetailsBatch fetches EVM transaction details for multiple transactions in batch.
// This significantly reduces RPC calls by batching requests.
func (s *EVMDetailsSyncService) fetchEVMDetailsBatch(ctx context.Context, evmHashes []string) ([]*chain.TransactionDetails, []error) {
	hashes := make([]ethcommon.Hash, len(evmHashes))
	for i, h := range evmHashes {
		hashes[i] = ethcommon.HexToHash(h)
	}
	return s.evmClient.GetTransactionDetailsBatch(ctx, hashes)
}

// saveEVMDetails saves EVM transaction details to evm_transactions table or object storage.
// Updates EVM-related fields (evm_from_address, evm_to_address) in the evm_transactions table.
func (s *EVMDetailsSyncService) saveEVMDetails(ctx context.Context, evmTx *models.EVMTransaction, details *chain.TransactionDetails) error {
	// Marshal to JSON
	detailsJSON, err := sonic.Marshal(details)
	if err != nil {
		return errors.Wrap(err, "failed to marshal transaction details")
	}

	// Check if should store to object storage (if data is large)
	storageKey := ""
	storageThreshold := getStorageThresholdForSync()

	if storage.IsEnabled() && len(detailsJSON) > storageThreshold {
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
		// Store in object storage, only save storage key to database
		updates["evm_details_storage_key"] = storageKey
		// Clear raw_details_json when using object storage (unless parsing failed)
		if details.ParseStatus == "success" {
			updates["raw_details_json"] = ""
		}
	} else {
		// Object storage not enabled or upload failed, store JSON in database
		updates["raw_details_json"] = string(detailsJSON)
		// Clear storage key when using database storage
		updates["evm_details_storage_key"] = ""
	}

	// Store raw JSON if parsing failed (fallback mechanism)
	if details.ParseStatus != "success" {
		// Build raw details JSON structure
		rawDetailsMap := make(map[string]interface{})
		if len(details.RawTransactionJSON) > 0 {
			var txData interface{}
			sonic.Unmarshal(details.RawTransactionJSON, &txData)
			rawDetailsMap["transaction"] = txData
		}
		if len(details.RawReceiptJSON) > 0 {
			var receiptData interface{}
			sonic.Unmarshal(details.RawReceiptJSON, &receiptData)
			rawDetailsMap["receipt"] = receiptData
		}

		rawDetailsJSON, _ := sonic.Marshal(rawDetailsMap)
		updates["raw_details_json"] = string(rawDetailsJSON)

		global.Logger.Infof("Stored raw JSON for %s (parse_status: %s)", evmTx.TransactionHash, details.ParseStatus)
	}

	// Build EVMTransactionUpdate for batch processing
	var rawDetailsJSON string
	var storageKeyStr string

	if val, ok := updates["raw_details_json"].(string); ok {
		rawDetailsJSON = val
	}
	if val, ok := updates["evm_details_storage_key"].(string); ok {
		storageKeyStr = val
	}

	update := &EVMTransactionUpdate{
		ID:                   evmTx.ID,
		TransactionHash:      evmTx.TransactionHash,
		EVMDetailsSynced:     true,
		EVMDetailsSyncAt:     &now,
		EVMDetailsSyncError:  "",
		EVMFromAddress:       details.From,
		EVMToAddress:         details.To,
		EVMDetailsStorageKey: storageKeyStr,
		RawDetailsJSON:       rawDetailsJSON,
		UpdatedAt:            now,
	}

	// Add to batch update buffer instead of immediate UPDATE
	s.addToUpdateBuffer(update)

	// Update transactions table with signer and signatures if they are empty
	// This is important for EVM transactions from go-ethereum/MetaMask where
	// the signer cannot be extracted from Cosmos SDK's AuthInfo.SignerInfos
	if details.From != "" && details.ParseStatus == "success" {
		// Use a separate context with timeout to avoid using expired context
		// This ensures the update can complete even if the original context timed out
		updateCtx, updateCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer updateCancel()

		// Check if signer is empty in transactions table
		var existingTx models.Transaction
		err := db.Get().WithContext(updateCtx).
			Select("hash, signer, signatures").
			Where("hash = ?", evmTx.TransactionHash).
			First(&existingTx).Error

		if err == nil {
			// Transaction exists, check if signer is empty or to/value need updating
			if existingTx.Signer == "" || existingTx.To == "" || len(existingTx.Values) == 0 {
				// Convert EVM address to Ethereum format for storage
				ethSigner := models.ConvertAddressToEthForStorage(details.From)
				ethTo := models.ConvertAddressToEthForStorage(details.To)
				ethValue := details.Value

				// Try to get original Cosmos transaction to extract full signature information
				var signatures []string
				chainClient, err := GetSharedChainClient()
				if err == nil {
					// Create a context with timeout for the GetTx call
					txCtx, txCancel := context.WithTimeout(updateCtx, EVMContextTimeoutSeconds*time.Second)
					defer txCancel()

					txResp, err := chainClient.GetTx(txCtx, evmTx.TransactionHash)
					if err == nil && txResp != nil {
						// Convert GetTxResponse to TxDetailData to extract signatures
						txDetailData := models.ConvertGetTxResponseToTxDetailData(txResp, chainClient.ClientContext().Codec)
						if txDetailData != nil && len(txDetailData.Signatures) > 0 {
							// Use the signatures from the original Cosmos transaction
							sigMessages := make([]proto.Message, 0, len(txDetailData.Signatures))
							for _, sig := range txDetailData.Signatures {
								if sig != nil {
									// Ensure address is set (use EVM From address if not set)
									if sig.Address == "" {
										sig.Address = ethSigner
									}
									sigMessages = append(sigMessages, sig)
								}
							}
							signatures, err = models.MarshalProtoSliceToJSON(sigMessages)
							if err != nil {
								global.Logger.Warnf("failed to marshal signatures from Cosmos transaction %s: %v", evmTx.TransactionHash, err)
								signatures = nil
							}
						}
					} else if err != nil {
						global.Logger.Debugf("failed to get Cosmos transaction %s for signature extraction: %v", evmTx.TransactionHash, err)
					}
				} else {
					global.Logger.Debugf("failed to get shared chain client for signature extraction: %v", err)
				}

				// Fallback: create minimal signature data if we couldn't get it from Cosmos transaction
				if signatures == nil {
					// Create signature data with at least the address
					sigData := &injective_explorer_rpcpb.Signature{
						Address:   ethSigner,
						Pubkey:    "", // Not available from EVM RPC
						Sequence:  0,  // Default value (uint64)
						Signature: "", // Not available from EVM RPC
					}

					// Marshal signature to JSON string
					sigMessages := []proto.Message{sigData}
					signatures, err = models.MarshalProtoSliceToJSON(sigMessages)
					if err != nil {
						global.Logger.Warnf("failed to marshal signature for transaction %s: %v", evmTx.TransactionHash, err)
						signatures = nil
					}
				}

				// Update transactions table with signer, signatures, to, and values
				// Note: We need to manually serialize signatures and values to JSON array string
				// because GORM's Updates(map[string]interface{}) doesn't apply the serializer:json tag
				txUpdates := map[string]interface{}{
					"signer": ethSigner,
					"to":     ethTo,
				}

				// Manually serialize values to JSON array string to ensure proper format
				// Values should be an array of strings, e.g., ["100000000"] not just "100000000"
				valuesArray := []string{ethValue}
				valuesJSON, err := sonic.Marshal(valuesArray)
				if err != nil {
					global.Logger.Warnf("failed to marshal values to JSON array for transaction %s: %v", evmTx.TransactionHash, err)
				} else {
					txUpdates["values"] = string(valuesJSON)
				}

				if signatures != nil {
					// Marshal []string to JSON array string (e.g., ["{...}", "{...}"])
					signaturesJSON, err := sonic.Marshal(signatures)
					if err != nil {
						global.Logger.Warnf("failed to marshal signatures to JSON array for transaction %s: %v", evmTx.TransactionHash, err)
					} else {
						txUpdates["signatures"] = string(signaturesJSON)
					}
				}

				err = db.Get().WithContext(updateCtx).
					Model(&models.Transaction{}).
					Where("hash = ?", evmTx.TransactionHash).
					Updates(txUpdates).Error

				if err != nil {
					global.Logger.Warnf("failed to update signer and signatures for transaction %s: %v", evmTx.TransactionHash, err)
					// Don't fail the whole operation if this update fails
					// But still preserve the cache so it can be retried later
					// Use a separate context to avoid using expired context
					s.preserveTransactionCache(ctx, evmTx.TransactionHash)
				} else {
					global.Logger.Debugf("updated signer and signatures for EVM transaction %s (signer: %s)", evmTx.TransactionHash, ethSigner)
					// Update Redis cache with the updated transaction data
					// This ensures the cache is updated with the latest signatures and signer
					// Use a separate context with timeout to avoid using expired context
					cacheCtx, cacheCancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cacheCancel()

					var updatedTx models.Transaction
					if err := db.Get().WithContext(cacheCtx).
						Where("hash = ?", evmTx.TransactionHash).
						First(&updatedTx).Error; err == nil {
						// Update single transaction cache
						cacheTransaction(cacheCtx, &updatedTx)

						// Invalidate account transaction caches for both signer and to addresses
						// This ensures cached account transactions are refreshed with updated data
						invalidateAccountTransactionCache(cacheCtx, updatedTx.Signer)
						if updatedTx.To != "" && updatedTx.To != updatedTx.Signer {
							invalidateAccountTransactionCache(cacheCtx, updatedTx.To)
						}

						global.Logger.Debugf("updated Redis cache for transaction %s with signatures and signer", evmTx.TransactionHash)
					} else {
						global.Logger.Debugf("failed to reload transaction %s for cache update: %v", evmTx.TransactionHash, err)
						// Even if reload fails, preserve the cache with current data
						s.preserveTransactionCache(ctx, evmTx.TransactionHash)
					}
				}
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			// Log error but don't fail the operation
			global.Logger.Warnf("failed to check transaction %s for signer update: %v", evmTx.TransactionHash, err)
		}
	}

	return nil
}

// updateSyncError updates the sync error information for an EVM transaction.
// Uses optimistic locking to prevent concurrent update issues.
func (s *EVMDetailsSyncService) updateSyncError(ctx context.Context, cosmosHash string, errorMsg string) {
	// Use a separate context with timeout to avoid using expired context
	// This ensures the error update can complete even if the original context timed out
	updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer updateCancel()

	result := db.Get().WithContext(updateCtx).
		Model(&models.EVMTransaction{}).
		Where("transaction_hash = ?", cosmosHash).
		Updates(map[string]interface{}{
			"evm_details_sync_error": errorMsg,
		})

	if result.Error != nil {
		// Check if error is due to context timeout
		if errors.Is(result.Error, context.DeadlineExceeded) || errors.Is(result.Error, context.Canceled) {
			global.Logger.Debugf("context expired while updating sync error for %s", cosmosHash)
		} else {
			global.Logger.Warnf("failed to update sync error for %s: %v", cosmosHash, result.Error)
		}
		return
	}

	if result.RowsAffected == 0 {
		global.Logger.Debugf("sync error update for %s affected 0 rows, possibly already updated", cosmosHash)
	}
}

// shouldRetryEVM checks if a task should be retried based on the error type.
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
func (s *EVMDetailsSyncService) retryTask(ctx context.Context, taskStr string) {
	// Use simple exponential backoff strategy
	maxRetries := s.config.MaxRetryAttempts
	baseDelay := s.config.RetryDelay

	go func() {
		// Exponential backoff: 5s, 10s, 20s, 40s
		for attempt := 0; attempt < maxRetries; attempt++ {
			delay := time.Duration(1<<uint(attempt)) * baseDelay
			time.Sleep(delay)

			// Use a separate context with timeout to avoid using expired context
			// This ensures the retry can complete even if the original context timed out
			retryCtx, retryCancel := context.WithTimeout(context.Background(), 5*time.Second)

			// Try to push the task back to the queue
			err := s.redisClient.LPush(retryCtx, s.config.QueueKey, taskStr).Err()
			retryCancel()

			if err == nil {
				global.Logger.Infof("Task %s successfully queued for retry (attempt %d)", taskStr, attempt+1)
				return
			}

			global.Logger.Warnf("Failed to retry task %s (attempt %d/%d): %v", taskStr, attempt+1, maxRetries, err)
		}

		// All retries exhausted
		// Don't log as error if it's just context timeout - this is expected in some cases
		global.Logger.Warnf("Failed to retry task %s after %d attempts, giving up. Task may be retried later by periodic check.", taskStr, maxRetries)
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

// preserveTransactionCache ensures that a transaction remains in Redis cache
// even when EVM details sync fails. This allows the transaction to be queried
// and updated when sync succeeds later.
func (s *EVMDetailsSyncService) preserveTransactionCache(ctx context.Context, cosmosHash string) {
	// Check if context is already expired to avoid unnecessary operations
	if ctx.Err() != nil {
		global.Logger.Debugf("context expired for transaction %s, skipping cache preservation", cosmosHash)
		return
	}

	// Use a separate context with timeout to avoid using expired context
	// This ensures cache preservation can complete even if the original context timed out
	preserveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if transaction exists in database
	var tx models.Transaction
	err := db.Get().WithContext(preserveCtx).
		Where("hash = ?", cosmosHash).
		First(&tx).Error

	if err != nil {
		// Transaction not found in database, nothing to preserve
		if errors.Is(err, gorm.ErrRecordNotFound) {
			global.Logger.Debugf("transaction %s not found in database, skipping cache preservation", cosmosHash)
		} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			global.Logger.Debugf("context expired while querying transaction %s for cache preservation", cosmosHash)
		} else {
			global.Logger.Warnf("failed to query transaction %s for cache preservation: %v", cosmosHash, err)
		}
		return
	}

	// Update Redis cache with current transaction data
	// This ensures the transaction remains available even if EVM details sync fails
	cacheTransaction(preserveCtx, &tx)
	global.Logger.Debugf("preserved transaction cache for %s after EVM details sync failure", cosmosHash)
}

// periodicSignerUpdateWorker periodically checks for EVM transactions with missing
// signatures or signer and attempts to update them from EVM details.
// This ensures that even if initial sync failed, the data can be updated later.
func (s *EVMDetailsSyncService) periodicSignerUpdateWorker() {
	defer s.wg.Done()

	// Check every 5 minutes for missing signatures/signer
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkAndUpdateMissingSigners()
		}
	}
}

// checkAndUpdateMissingSigners checks for EVM transactions with missing signatures or signer
// and attempts to update them from evm_transactions table.
func (s *EVMDetailsSyncService) checkAndUpdateMissingSigners() {
	ctx, cancel := context.WithTimeout(s.ctx, EVMDetailsTimeout)
	defer cancel()

	// Find EVM transactions that have been synced but transactions table has empty signer
	// Include transactions where evm_from_address is empty - we'll recover it from raw transaction data
	// Limit to 100 transactions per check to avoid overloading
	var evmTxs []models.EVMTransaction
	err := db.Get().WithContext(ctx).
		Joins("JOIN transactions ON evm_transactions.transaction_hash = transactions.hash").
		Where("evm_transactions.evm_details_synced = ? AND (transactions.signer = '' OR transactions.signatures IS NULL OR transactions.signatures = '[]' OR transactions.signatures = '')", true).
		Limit(100).
		Find(&evmTxs).Error

	if err != nil {
		global.Logger.Warnf("failed to query EVM transactions with missing signers: %v", err)
		return
	}

	if len(evmTxs) == 0 {
		return
	}

	global.Logger.Infof("found %d EVM transactions with missing signatures/signer, attempting to update", len(evmTxs))

	// Update each transaction
	successCount := 0
	for _, evmTx := range evmTxs {
		if err := s.updateSignerFromEVMDetails(ctx, &evmTx); err != nil {
			global.Logger.Warnf("failed to update signer for transaction %s: %v", evmTx.TransactionHash, err)
		} else {
			successCount++
		}
	}

	if successCount > 0 {
		global.Logger.Infof("successfully updated %d/%d EVM transactions with missing signatures/signer", successCount, len(evmTxs))
	}
}

// updateSignerFromEVMDetails updates the signer and signatures for a transaction
// from the EVM details stored in evm_transactions table.
func (s *EVMDetailsSyncService) updateSignerFromEVMDetails(ctx context.Context, evmTx *models.EVMTransaction) error {
	var ethSigner, ethTo, value string
	var err error

	// First try to use cached from address if available
	if evmTx.EVMFromAddress != "" {
		ethSigner = models.ConvertAddressToEthForStorage(evmTx.EVMFromAddress)
		ethTo = models.ConvertAddressToEthForStorage(evmTx.EVMToAddress)
	} else {
		// From address is empty - need to recover it from raw transaction data
		// This happens when the original sync failed to recover the sender
		ethSigner, ethTo, value, err = s.recoverFromRawTransaction(ctx, evmTx)
		if err != nil {
			return errors.Wrap(err, "failed to recover from address from raw transaction")
		}
	}

	// Try to get value from raw_details_json or object storage
	if evmTx.RawDetailsJSON != "" {
		// Parse JSON to extract value
		var details map[string]interface{}
		if err := sonic.Unmarshal([]byte(evmTx.RawDetailsJSON), &details); err == nil {
			if val, ok := details["value"].(string); ok {
				value = val
			}
		}
	} else if evmTx.EVMDetailsStorageKey != "" && storage.IsEnabled() {
		// Try to load from object storage
		storageClient := storage.Get()
		if data, err := storageClient.GetObject(ctx, evmTx.EVMDetailsStorageKey); err == nil {
			var details map[string]interface{}
			if err := sonic.Unmarshal(data, &details); err == nil {
				if val, ok := details["value"].(string); ok {
					value = val
				}
			}
		}
	}

	// Create minimal signature data
	sigData := &injective_explorer_rpcpb.Signature{
		Address:   ethSigner,
		Pubkey:    "", // Not available from EVM RPC
		Sequence:  0,  // Default value
		Signature: "", // Not available from EVM RPC
	}

	// Marshal signature to JSON string
	sigMessages := []proto.Message{sigData}
	signatures, err := models.MarshalProtoSliceToJSON(sigMessages)
	if err != nil {
		return errors.Wrap(err, "failed to marshal signature")
	}

	// Update transactions table
	txUpdates := map[string]interface{}{
		"signer": ethSigner,
		"to":     ethTo,
	}
	if value != "" {
		// Manually serialize values to JSON array string to ensure proper format
		// Values should be an array of strings, e.g., ["100000000"] not just "100000000"
		valuesArray := []string{value}
		valuesJSON, err := sonic.Marshal(valuesArray)
		if err != nil {
			global.Logger.Warnf("failed to marshal values to JSON array for transaction %s: %v", evmTx.TransactionHash, err)
		} else {
			txUpdates["values"] = string(valuesJSON)
		}
	}
	if signatures != nil {
		signaturesJSON, err := sonic.Marshal(signatures)
		if err != nil {
			global.Logger.Warnf("failed to marshal signatures to JSON array for transaction %s: %v", evmTx.TransactionHash, err)
		} else {
			txUpdates["signatures"] = string(signaturesJSON)
		}
	}

	err = db.Get().WithContext(ctx).
		Model(&models.Transaction{}).
		Where("hash = ?", evmTx.TransactionHash).
		Updates(txUpdates).Error

	if err != nil {
		return errors.Wrap(err, "failed to update transaction")
	}

	// Update Redis cache
	var updatedTx models.Transaction
	if err := db.Get().WithContext(ctx).
		Where("hash = ?", evmTx.TransactionHash).
		First(&updatedTx).Error; err == nil {
		// Update single transaction cache
		cacheTransaction(ctx, &updatedTx)

		// Invalidate account transaction caches for both signer and to addresses
		// This ensures cached account transactions are refreshed with updated data
		invalidateAccountTransactionCache(ctx, updatedTx.Signer)
		if updatedTx.To != "" && updatedTx.To != updatedTx.Signer {
			invalidateAccountTransactionCache(ctx, updatedTx.To)
		}
	}

	return nil
}

// recoverFromRawTransaction recovers the from address, to address, and value
// from the raw transaction data stored in evm_transactions table.
// This is used for historical data where evm_from_address was not populated during initial sync.
func (s *EVMDetailsSyncService) recoverFromRawTransaction(ctx context.Context, evmTx *models.EVMTransaction) (from, to, value string, err error) {
	// Parse raw_details_json to extract transaction data
	var rawDetails map[string]interface{}
	if evmTx.RawDetailsJSON != "" {
		if err := sonic.Unmarshal([]byte(evmTx.RawDetailsJSON), &rawDetails); err != nil {
			return "", "", "", errors.Wrap(err, "failed to parse raw_details_json")
		}
	} else if evmTx.EVMDetailsStorageKey != "" && storage.IsEnabled() {
		// Try to load from object storage
		storageClient := storage.Get()
		data, err := storageClient.GetObject(ctx, evmTx.EVMDetailsStorageKey)
		if err != nil {
			return "", "", "", errors.Wrap(err, "failed to load from object storage")
		}
		if err := sonic.Unmarshal(data, &rawDetails); err != nil {
			return "", "", "", errors.Wrap(err, "failed to parse storage data")
		}
	} else {
		return "", "", "", errors.New("no raw transaction data available")
	}

	// Extract transaction data
	txData, ok := rawDetails["transaction"].(map[string]interface{})
	if !ok {
		return "", "", "", errors.New("transaction data not found in raw_details_json")
	}

	// Extract to address
	if toAddr, ok := txData["to"].(string); ok && toAddr != "" {
		to = models.ConvertAddressToEthForStorage(toAddr)
	}

	// Extract value
	if val, ok := txData["value"].(string); ok {
		value = val
	}

	// Recover from address from transaction signature
	// Need to use go-ethereum's transaction types

	// Try to decode and recover from address
	// The transaction data should have v, r, s values
	v, hasV := txData["v"].(string)
	r, hasR := txData["r"].(string)
	sVal, hasS := txData["s"].(string)

	if !hasV || !hasR || !hasS {
		return "", "", "", errors.New("transaction signature (v,r,s) not found in raw data")
	}

	// Use EVM client to recover from address
	recoveredFrom, err := s.evmClient.RecoverSender(ctx, evmTx.EVMHash, v, r, sVal)
	if err != nil {
		return "", "", "", errors.Wrap(err, "failed to recover sender from signature")
	}

	from = models.ConvertAddressToEthForStorage(recoveredFrom)

	// Also update evm_transactions table with the recovered from address
	// This prevents needing to recover it again in the future
	updateErr := db.Get().WithContext(ctx).
		Model(&models.EVMTransaction{}).
		Where("transaction_hash = ?", evmTx.TransactionHash).
		Update("evm_from_address", from).Error
	if updateErr != nil {
		global.Logger.Warnf("failed to update evm_from_address for %s: %v", evmTx.TransactionHash, updateErr)
		// Don't fail - we have the from address, just continue
	}

	return from, to, value, nil
}

// logTaskDeduplicationMetrics periodically logs task deduplication metrics.
// This helps monitor the effectiveness of request coalescing and detect potential issues.
func (s *EVMDetailsSyncService) logTaskDeduplicationMetrics() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if s.taskDeduplicator != nil {
				metrics := s.taskDeduplicator.GetMetrics()
				global.Logger.Infof(
					"EVM task dedup metrics: inflight=%d/%d, total=%d, coalesced=%d (%.1f%%), direct=%d, cleanup=%d",
					metrics.InflightCount,
					metrics.MaxInflightSize,
					metrics.TotalRequests,
					metrics.CoalescedRequests,
					metrics.CoalescedRatio*100,
					metrics.DirectRequests,
					metrics.CleanupCount,
				)
			}
		}
	}
}

// batchUpdateFlusher periodically flushes the batch update buffer.
func (s *EVMDetailsSyncService) batchUpdateFlusher() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.flushTicker.C:
			s.flushUpdateBuffer()
		}
	}
}

// addToUpdateBuffer adds an EVM transaction update to the batch buffer.
// If buffer is full, it triggers an immediate flush.
func (s *EVMDetailsSyncService) addToUpdateBuffer(update *EVMTransactionUpdate) {
	s.updateBufferMutex.Lock()
	s.updateBuffer = append(s.updateBuffer, update)
	shouldFlush := len(s.updateBuffer) >= s.updateBufferSize
	s.updateBufferMutex.Unlock()

	// Flush immediately if buffer is full (async to avoid blocking)
	if shouldFlush {
		go s.flushUpdateBuffer()
	}
}

// flushUpdateBuffer flushes all pending updates in the buffer to the database.
func (s *EVMDetailsSyncService) flushUpdateBuffer() {
	s.updateBufferMutex.Lock()
	if len(s.updateBuffer) == 0 {
		s.updateBufferMutex.Unlock()
		return
	}

	// Take ownership of current buffer and create new one
	updates := s.updateBuffer
	s.updateBuffer = make([]*EVMTransactionUpdate, 0, s.updateBufferSize)
	s.updateBufferMutex.Unlock()

	// Execute batch update
	if err := s.executeBatchUpdate(updates); err != nil {
		global.Logger.Errorf("Batch update failed for %d txs: %v", len(updates), err)
		// Fallback to individual updates on error
		s.fallbackToIndividualUpdates(updates)
	} else {
		global.Logger.Debugf("Successfully batch updated %d EVM transactions", len(updates))
	}
}

// executeBatchUpdate executes a batch update using GORM's CreateInBatches with OnConflict.
func (s *EVMDetailsSyncService) executeBatchUpdate(updates []*EVMTransactionUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Convert EVMTransactionUpdate to EVMTransaction for GORM
	evmTxs := make([]*models.EVMTransaction, len(updates))
	for i, u := range updates {
		evmTxs[i] = &models.EVMTransaction{
			TransactionHash:      u.TransactionHash,
			EVMDetailsSynced:     u.EVMDetailsSynced,
			EVMDetailsSyncAt:     u.EVMDetailsSyncAt,
			EVMDetailsSyncError:  u.EVMDetailsSyncError,
			EVMFromAddress:       u.EVMFromAddress,
			EVMToAddress:         u.EVMToAddress,
			EVMDetailsStorageKey: u.EVMDetailsStorageKey,
			RawDetailsJSON:       u.RawDetailsJSON,
		}
		evmTxs[i].ID = u.ID
		evmTxs[i].UpdatedAt = u.UpdatedAt
	}

	// Use CreateInBatches with ON CONFLICT UPDATE (similar to sync.go:950-965)
	// This performs INSERT ... ON DUPLICATE KEY UPDATE which is more efficient than UPDATE
	return db.Get().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Use Clauses with OnConflict for batch upsert
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "transaction_hash"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"evm_details_synced",
				"evm_details_sync_at",
				"evm_details_sync_error",
				"evm_from_address",
				"evm_to_address",
				"evm_details_storage_key",
				"raw_details_json",
				"updated_at",
			}),
		}).CreateInBatches(evmTxs, 100).Error
	})
}

// fallbackToIndividualUpdates falls back to individual updates when batch update fails.
// This ensures data consistency even if batch operations fail.
func (s *EVMDetailsSyncService) fallbackToIndividualUpdates(updates []*EVMTransactionUpdate) {
	global.Logger.Warnf("Falling back to individual updates for %d transactions", len(updates))

	successCount := 0
	for _, u := range updates {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := db.Get().WithContext(ctx).
			Model(&models.EVMTransaction{}).
			Where("id = ? AND transaction_hash = ?", u.ID, u.TransactionHash).
			Updates(map[string]interface{}{
				"evm_details_synced":      u.EVMDetailsSynced,
				"evm_details_sync_at":     u.EVMDetailsSyncAt,
				"evm_details_sync_error":  u.EVMDetailsSyncError,
				"evm_from_address":        u.EVMFromAddress,
				"evm_to_address":          u.EVMToAddress,
				"evm_details_storage_key": u.EVMDetailsStorageKey,
				"raw_details_json":        u.RawDetailsJSON,
				"updated_at":              u.UpdatedAt,
			}).Error
		cancel()

		if err != nil {
			global.Logger.Errorf("Individual update failed for transaction %s: %v", u.TransactionHash, err)
		} else {
			successCount++
		}
	}

	global.Logger.Infof("Individual updates completed: %d/%d successful", successCount, len(updates))
}
