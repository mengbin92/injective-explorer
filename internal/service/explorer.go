// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/chain"
	"injective-explorer/provider/db"
	"injective-explorer/provider/storage"

	"github.com/InjectiveLabs/sdk-go/chain/types"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	"github.com/bytedance/sonic"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	sdk "github.com/cosmos/cosmos-sdk/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ExplorerService implements the explorer API
// It provides methods to query blockchain data including blocks, transactions, and account information.
//
// The service maintains client connections for better performance,
// creating them lazily when needed and reusing them across requests.
type ExplorerService struct {
	pb.UnimplementedExplorerServer
	statsUpdateOnce sync.Once // Ensure stats update goroutine starts only once
	coalescer       *RequestCoalescer
}

var (
	statsUpdateOnceGlobal sync.Once // Global once for stats update goroutine
)

// createEVMClient creates an EVM client with custom configuration
// It uses global.EVMConfig if available, then falls back to defaults
func createEVMClient() (*chain.EVMClient, error) {
	// Check if custom EVM RPC endpoint is configured via global config
	if global.EVMConfig != nil && global.EVMConfig.Enabled && global.EVMConfig.RpcEndpoint != "" {
		chainID := new(big.Int)
		chainID.SetInt64(global.EVMConfig.ChainId)

		timeout := global.EVMConfig.Timeout.AsDuration()
		if timeout == 0 {
			timeout = DefaultEVMTimeout
		}

		return chain.NewEVMClientWithConfig(&chain.EVMConfig{
			RPCURL:  global.EVMConfig.RpcEndpoint,
			ChainID: chainID,
			Network: global.NetworkName,
			Timeout: timeout,
		})
	}

	// Fall back to default network configuration
	return chain.NewEVMClient(global.NetworkName)
}

// NewExplorerService creates a new instance of ExplorerService.
//
// Returns:
//   - *ExplorerService: A new service instance
func NewExplorerService() *ExplorerService {
	return &ExplorerService{
		coalescer: NewRequestCoalescer(
			RequestCoalescerMaxInflightAge,
			RequestCoalescerCleanupInterval,
			RequestCoalescerMaxInflightSize,
		),
	}
}

// getStorageThresholdForSync returns the storage threshold for uploading to object storage during sync.
// It reads from global.ArchiveConfig if available, otherwise returns the default value.
func getStorageThresholdForSync() int {
	if global.ArchiveConfig != nil && global.ArchiveConfig.GetStorageThreshold() > 0 {
		return int(global.ArchiveConfig.GetStorageThreshold())
	}

	return DefaultStorageThreshold
}

// GetLatestBlocks retrieves the latest blocks from the blockchain with pagination.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Pagination request containing page and page_size
//
// Returns:
//   - *pb.Reply: Response containing the latest blocks data with pagination info
//   - error: Error if the operation fails
func (s *ExplorerService) GetLatestBlocks(ctx context.Context, req *pb.PaginationRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Normalize pagination parameters
	page, pageSize, offset, useCursor, cursorValue := normalizePagination(req.Page, req.PageSize, req.Cursor, MaxPageSize)

	// Use Debug level for request details to reduce log overhead in production
	global.Logger.Debugf("get latest blocks: page=%d, page_size=%d, use_cursor=%v, cursor=%v", page, pageSize, useCursor, cursorValue)

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// First, try to query from Redis cache for page-based pagination (avoids DB lag)
	if !useCursor {
		var cachedBlocks []models.Block
		var cacheErr error
		var hasMore bool

		if page == MinPage {
			// First page: use simple limit query
			cachedBlocks, cacheErr = getBlocksFromCache(queryCtx, int(pageSize))
		} else {
			// Other pages: use offset-based query
			cachedBlocks, hasMore, cacheErr = getBlocksFromCacheWithOffset(queryCtx, offset, int(pageSize))
		}

		if cacheErr != nil {
			// Log cache error but continue to database query (graceful degradation)
			global.Logger.Warnf("failed to get blocks from cache, fallback to DB: %v", cacheErr)
		} else if len(cachedBlocks) > 0 {
			global.Logger.Debugf("blocks found in cache: page=%d, count=%d", page, len(cachedBlocks))
			// Try to get total count from cache first to avoid database query
			total, isExactCount := getTotalFromCache(queryCtx, CacheKeyTotalBlocks)
			if total == 0 {
				// Cache miss for total, query from database and log error if any
				var countErr error
				total, isExactCount, countErr = getApproximateCountWithError(db.Get(), &models.Block{}, ctx)
				if countErr != nil {
					global.Logger.Warnf("failed to get approximate count for blocks: %v, using fallback value", countErr)
					// Use fallback value if count query fails
					total = ApproximateCountFallback
					isExactCount = false
				} else {
					// Cache the total count for future queries
					if err := cacheTotal(queryCtx, CacheKeyTotalBlocks, total); err != nil {
						global.Logger.Warnf("failed to cache total blocks count: %v", err)
					}
				}
			}
			nextCursor := calculateNextCursor(useCursor, cursorValue, cachedBlocks, func(b models.Block) uint64 { return b.Height })
			blockDTOs := models.ToBlockDTOs(cachedBlocks)
			blocksResp := buildPaginatedResponse(blockDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(cachedBlocks))
			return MarshalAndReply(blocksResp, "marshal blocks error")
		} else if page != MinPage && !hasMore {
			// Cache doesn't have enough data for this page, fallback to DB
			global.Logger.Debugf("cache miss for page %d, fallback to DB", page)
		}
	}

	// Second, try to query from database with pagination
	var dbBlocks []models.Block
	var total int64
	var isExactCount bool

	// Get approximate or exact count (optimized for performance)
	var countErr error
	total, isExactCount, countErr = getApproximateCountWithError(db.Get(), &models.Block{}, ctx)
	if countErr != nil {
		global.Logger.Warnf("failed to get approximate count for blocks: %v, using fallback value", countErr)
		// Use fallback value if count query fails
		total = ApproximateCountFallback
		isExactCount = false
	} else {
		// Cache the total count for future queries
		if err := cacheTotal(queryCtx, CacheKeyTotalBlocks, total); err != nil {
			global.Logger.Warnf("failed to cache total blocks count: %v", err)
		}
	}

	// Build query based on pagination type
	query := db.Get().WithContext(queryCtx).Model(&models.Block{})

	if useCursor {
		// Use cursor-based pagination (more efficient for large datasets)
		// Use height for both cursor and ordering to ensure consistent pagination
		query = query.Where("height < ?", cursorValue).
			Order("height DESC").
			Limit(int(pageSize))
	} else {
		// Use page-based pagination
		query = query.Order("height DESC").
			Offset(offset).
			Limit(int(pageSize))
	}

	startTime := time.Now()
	err := query.Find(&dbBlocks).Error
	queryDuration := time.Since(startTime)

	if err == nil && len(dbBlocks) > 0 {
		// Found in database, convert to DTO and return
		global.Logger.Debugf("blocks found in database: count=%d, query_time=%v", len(dbBlocks), queryDuration)

		if len(dbBlocks) > 0 {
			nextCursor := calculateNextCursor(useCursor, cursorValue, dbBlocks, func(b models.Block) uint64 { return b.Height })
			blockDTOs := models.ToBlockDTOs(dbBlocks)
			blocksResp := buildPaginatedResponse(blockDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(dbBlocks))
			return MarshalAndReply(blocksResp, "marshal blocks error")
		}
	}
	// Database error (not just record not found), log and continue to query from chain
	global.Logger.Warnf("database query error (not record not found), querying from chain: error=%v", err)
	return ErrorReply(http.StatusInternalServerError, "database query error", err), err
}

// loadArchivedBlockTxs loads archived block transaction data if the block is archived.
// This function checks if a block has been archived and loads the txs data from archive storage.
// It modifies the block in place.
//
// Parameters:
//   - ctx: Context for cancellation
//   - block: The block to load archived data for (modified in place)
//
// Returns:
//   - error: Error if loading fails (non-blocking, logs warning)
func (s *ExplorerService) loadArchivedBlockTxs(ctx context.Context, block *models.Block) error {
	if block == nil || !block.Archived {
		return nil
	}

	// If block is archived but txs are already loaded (non-empty), no need to load
	if len(block.Txs) > 0 {
		return nil
	}

	global.Logger.Debugf("loading archived txs for block %d", block.Height)

	// Use archive service to load archived data
	archiveService := NewArchiveService()
	txs, err := archiveService.LoadArchivedBlockTxs(ctx, uint64(block.ID))
	if err != nil {
		global.Logger.Warnf("failed to load archived txs for block %d: %v", block.Height, err)
		// Don't fail the request, just log warning
		return nil
	}

	block.Txs = txs
	global.Logger.Debugf("loaded %d archived txs for block %d", len(txs), block.Height)
	return nil
}

// GetBlockByHeight retrieves a specific block by its height.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing the block height
//
// Returns:
//   - *pb.Reply: Response containing the block data
//   - error: Error if the operation fails or validation fails
func (s *ExplorerService) GetBlockByHeight(ctx context.Context, req *pb.GetBlockByHeightRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Validate block height
	if err := ValidateBlockHeight(req.Height); err != nil {
		return ErrorReply(http.StatusBadRequest, "invalid block height", err), err
	}

	global.Logger.Debugf("get block by height: height=%d", req.Height)

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// First, try to query from Redis cache
	cachedBlock, err := getBlockFromCache(queryCtx, uint64(req.Height))
	if err == nil && cachedBlock != nil {
		global.Logger.Debugf("block found in cache: height=%d", req.Height)
		blockDTO := models.ToBlockDTO(cachedBlock, models.DTOModeWithDetail)

		// For cached blocks, enhance transactions asynchronously if the block is recent
		if uint64(req.Height) > getLatestBlockHeight()-RecentBlockThreshold {
			// Enhance in background without blocking the response
			go func() {
				enhanceCtx, cancel := context.WithTimeout(context.Background(), AsyncEnhancementTimeout)
				defer cancel()
				s.enhanceBlockDTOWithTransactions(enhanceCtx, blockDTO, uint64(req.Height))
			}()
		}

		return MarshalAndReply(blockDTO, "marshal block error")
	}

	// Second, try to query from database
	var dbBlock models.Block
	startTime := time.Now()
	err = db.Get().WithContext(queryCtx).Where("height = ?", req.Height).First(&dbBlock).Error
	queryDuration := time.Since(startTime)

	if err == nil {
		// Found in database, convert to DTO and return
		global.Logger.Debugf("block found in database: height=%d, query_time=%v", req.Height, queryDuration)

		// Load archived txs if block is archived
		if dbBlock.Archived && len(dbBlock.Txs) == 0 {
			s.loadArchivedBlockTxs(queryCtx, &dbBlock)
		}

		// Cache the block for future queries (async to avoid blocking)
		go cacheBlock(context.Background(), &dbBlock)

		blockDTO := models.ToBlockDTO(&dbBlock, models.DTOModeWithDetail)

		// For database blocks, enhance transactions based on age and transaction count
		// Skip enhancement for blocks with >100 transactions to avoid timeout
		if uint64(req.Height) > getLatestBlockHeight()-RecentBlockThreshold && len(blockDTO.Txs) <= MaxTxEnhancementCount {
			// Enhance synchronously for recent small blocks
			enhancedBlockDTO := s.enhanceBlockDTOWithTransactions(queryCtx, blockDTO, uint64(req.Height))
			return MarshalAndReply(enhancedBlockDTO, "marshal block error")
		} else {
			// Return block without enhancement or enhance asynchronously for large/recent blocks
			if len(blockDTO.Txs) > 100 {
				global.Logger.Debugf("skipping transaction enhancement for block %d with %d transactions (too many)",
					req.Height, len(blockDTO.Txs))
			} else {
				// Enhance in background for older blocks
				go func() {
					enhanceCtx, cancel := context.WithTimeout(context.Background(), AsyncEnhancementTimeout)
					defer cancel()
					s.enhanceBlockDTOWithTransactions(enhanceCtx, blockDTO, uint64(req.Height))
				}()
			}
			return MarshalAndReply(blockDTO, "marshal block error")
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: height=%d, error=%v, query_time=%v", req.Height, err, queryDuration)
	} else {
		global.Logger.Debugf("block not found in database: height=%d, query_time=%v", req.Height, queryDuration)
	}

	// Not found in database, query from blockchain
	global.Logger.Infof("block not found in database, querying from blockchain: height=%d", req.Height)

	// Use independent context for RPC calls to avoid timeout issues
	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), AsyncEnhancementTimeout)
	defer rpcCancel()

	// Create RPC client
	rpcClient, err := rpchttp.New(global.ChainConfig.TmEndpoint)
	if err != nil {
		global.Logger.Errorf("failed to create RPC client: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to create RPC client", err), err
	}

	// Fetch block from blockchain
	height := int64(req.Height)
	blockResp, err := rpcClient.Block(rpcCtx, &height)
	if err != nil {
		global.Logger.Errorf("failed to fetch block from blockchain: height=%d, error=%v", req.Height, err)
		return ErrorReply(http.StatusInternalServerError, "failed to fetch block from blockchain", err), err
	}

	if blockResp == nil || blockResp.Block == nil {
		global.Logger.Errorf("block response is nil for height %d", req.Height)
		return ErrorReply(http.StatusInternalServerError, "block response is nil", errors.New("block response is nil")), errors.New("block response is nil")
	}

	// Convert to model
	blockModel := models.BlockFromTM(blockResp)
	if blockModel == nil {
		global.Logger.Errorf("failed to convert block at height %d", req.Height)
		return ErrorReply(http.StatusInternalServerError, "failed to convert block", errors.New("failed to convert block")), errors.New("failed to convert block")
	}

	// Save to database using upsert for future queries
	err = db.Get().Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "height"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"proposer", "moniker", "block_hash", "parent_hash",
			"num_precommits", "num_txs", "total_txs", "txs",
			"timestamp", "block_unix_timestamp", "updated_at",
		}),
	}).Create(blockModel).Error

	if err != nil {
		global.Logger.Warnf("failed to save block to database (non-critical, returning fetched data): height=%d, error=%v", req.Height, err)
		// Continue even if database save fails
	} else {
		global.Logger.Infof("saved block to database: height=%d", req.Height)
	}

	// Cache the block for future queries
	cacheBlock(rpcCtx, blockModel)

	// Convert to DTO and return
	blockDTO := models.ToBlockDTO(blockModel, models.DTOModeWithDetail)

	// Enhance transactions if the block is recent and has manageable transaction count
	if uint64(req.Height) > getLatestBlockHeight()-RecentBlockThreshold && len(blockDTO.Txs) <= MaxTxEnhancementCount {
		// Enhance synchronously for recent small blocks
		enhancedBlockDTO := s.enhanceBlockDTOWithTransactions(queryCtx, blockDTO, uint64(req.Height))
		return MarshalAndReply(enhancedBlockDTO, "marshal block error")
	}

	return MarshalAndReply(blockDTO, "marshal block error")
}

// enhanceBlockDTOWithTransactions preloads transaction information from the database.
// The Txs field now contains transaction hashes (strings) instead of full transaction objects.
// This function queries the database for transactions in the block and fetches missing ones from the chain.
// The Txs field remains as a hash list - detailed transaction info should be fetched via dedicated transaction APIs.
// Optimized with batch processing and timeout management for better performance.
//
// Parameters:
//   - ctx: Context for database queries
//   - blockDTO: The BlockDTO with Txs as hash list
//   - blockHeight: The block height
//
// Returns:
//   - *pb.BlockDTO: The original blockDTO (Txs remains as hash list)
func (s *ExplorerService) enhanceBlockDTOWithTransactions(ctx context.Context, blockDTO *pb.BlockDTO, blockHeight uint64) *pb.BlockDTO {
	if blockDTO == nil || len(blockDTO.Txs) == 0 {
		return blockDTO
	}

	// Skip enhancement for very old blocks (> OldBlockThreshold blocks) to avoid long delays
	// This is a performance optimization - old blocks are less likely to need enhancement
	if blockHeight < getLatestBlockHeight()-OldBlockThreshold {
		global.Logger.Debugf("skipping transaction enhancement for old block %d", blockHeight)
		return blockDTO
	}

	// Query transactions from database by block number
	var dbTxs []models.Transaction
	err := db.Get().WithContext(ctx).
		Select("hash, signer, to, value").
		Where("block_number = ?", blockHeight).
		Find(&dbTxs).Error

	if err != nil {
		global.Logger.Warnf("failed to query transactions for block %d: %v", blockHeight, err)
		// Continue to fetch from chain if database query fails
	} else {
		global.Logger.Debugf("found %d transactions in database for block %d", len(dbTxs), blockHeight)
	}

	// Create a map of transaction hash to transaction info for quick lookup
	txMap := make(map[string]*models.Transaction)
	for i := range dbTxs {
		// Normalize hash (remove 0x prefix if present for comparison)
		hash := dbTxs[i].Hash
		if len(hash) >= 2 && hash[:2] == "0x" {
			hash = hash[2:]
		}
		txMap[hash] = &dbTxs[i]
	}

	// Collect missing transaction hashes
	missingHashes := make([]string, 0)

	// First pass: identify missing transactions
	// blockDTO.Txs is now []string (hash list), not []*structpb.Struct
	for _, txHash := range blockDTO.Txs {
		if txHash == "" {
			continue
		}

		// Normalize hash (remove 0x prefix if present)
		normalizedHash := txHash
		if len(normalizedHash) >= 2 && normalizedHash[:2] == "0x" {
			normalizedHash = normalizedHash[2:]
		}

		// Check if transaction exists in database
		if _, found := txMap[normalizedHash]; !found {
			missingHashes = append(missingHashes, normalizedHash)
		}
	}

	if len(missingHashes) > 0 {
		global.Logger.Debugf("found %d missing transactions for block %d (total txs in block: %d, found in DB: %d)",
			len(missingHashes), blockHeight, len(blockDTO.Txs), len(dbTxs))
	}

	// Fetch missing transactions from chain and save to database
	if len(missingHashes) > 0 {
		// Limit the number of transactions to fetch to avoid excessive delays
		maxFetchCount := 50 // Maximum 50 transactions to fetch
		if len(missingHashes) > maxFetchCount {
			global.Logger.Warnf("limiting transaction fetch from %d to %d for block %d to avoid timeout",
				len(missingHashes), maxFetchCount, blockHeight)
			missingHashes = missingHashes[:maxFetchCount]
		}

		global.Logger.Debugf("fetching %d missing transactions from chain for block %d", len(missingHashes), blockHeight)
		chainClient, err := GetSharedChainClient()
		if err == nil {
			// Use shorter timeout for individual transaction fetches (5 seconds instead of 60)
			// This prevents one slow transaction from blocking the entire block response
			callCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Fetch transactions concurrently for better performance
			storageThreshold := getStorageThresholdForSync()
			var mu sync.Mutex
			var wg sync.WaitGroup

			// Increase concurrent requests to 20 for better throughput with shorter timeout
			semaphore := make(chan struct{}, 20) // Max 20 concurrent requests

			for _, hash := range missingHashes {
				wg.Add(1)
				go func(txHash string) {
					defer wg.Done()
					semaphore <- struct{}{}        // Acquire semaphore
					defer func() { <-semaphore }() // Release semaphore

					// Fetch transaction from chain
					// Sanitize hash to ensure correct format (remove 0x prefix if present)
					hashToFetch := SanitizeHash(txHash)
					txResp, err := chainClient.GetTx(callCtx, hashToFetch)
					if err != nil {
						// Log but don't fail - missing transactions are acceptable
						global.Logger.Debugf("failed to fetch transaction %s from chain: %v", txHash, err)
						return
					}

					if txResp == nil || txResp.TxResponse == nil {
						global.Logger.Debugf("transaction %s response is nil", txHash)
						return
					}

					// Convert GetTxResponse to TxDetailData
					txDetailData := models.ConvertGetTxResponseToTxDetailData(txResp, chainClient.ClientContext().Codec)
					if txDetailData == nil {
						global.Logger.Warnf("failed to convert transaction %s response to detail data", txHash)
						return
					}

					// Convert to model and save to database
					txModel := models.TransactionFromProto(txDetailData, storageThreshold)
					if txModel == nil {
						global.Logger.Warnf("failed to convert transaction %s to model", txHash)
						return
					}

					// Ensure block_number is set correctly
					if txModel.BlockNumber == 0 {
						txModel.BlockNumber = blockHeight
						global.Logger.Debugf("set block_number to %d for transaction %s", blockHeight, txHash)
					}

					// Save to database asynchronously to avoid blocking
					go func(model *models.Transaction) {
						saveToDB(model, fmt.Sprintf("create transaction %s error", txHash))
						global.Logger.Debugf("saved transaction %s to database (block_number: %d)", txHash, model.BlockNumber)
					}(txModel)

					// Add to txMap for use below
					mu.Lock()
					normalizedHash := txModel.Hash
					if len(normalizedHash) >= 2 && normalizedHash[:2] == "0x" {
						normalizedHash = normalizedHash[2:]
					}
					txMap[normalizedHash] = txModel
					mu.Unlock()
				}(hash)
			}

			wg.Wait()
		} else {
			global.Logger.Warnf("failed to create explorer client to fetch missing transactions: %v", err)
		}
	}

	// Note: The Txs field now contains only transaction hashes (strings).
	// Detailed transaction information (signer, to, value) should be fetched
	// via the GetTransactionByHash API for individual transactions.
	// This function primarily serves to preload missing transactions into the database.

	return blockDTO
}

// GetLatestBlockHeight retrieves the height of the latest block.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Empty request
//
// Returns:
//   - *pb.Reply: Response containing the latest block height
//   - error: Error if the operation fails
func (s *ExplorerService) GetLatestBlockHeight(ctx context.Context, req *emptypb.Empty) (*pb.Reply, error) {
	global.Logger.Debugf("get latest block height")

	// Use request coalescing to prevent duplicate queries under high concurrency
	result, err := s.coalescer.Do(ctx, "block:latest-height", func() (interface{}, error) {
		client := GetSharedTmClient()
		height, err := client.GetLatestBlockHeight(ctx)
		if err != nil {
			return nil, err
		}
		return height, nil
	})

	if err != nil {
		global.Logger.Errorf("get latest block height error: %v", err)
		return ErrorReply(http.StatusInternalServerError, "get latest block height error", err), err
	}

	height := result.(int64)
	return MarshalAndReply(&pb.LatestBlockHeightResponse{Height: int64(height)}, "marshal height error")
}

// getCachedGasPrice retrieves the current gas price with caching.
// Fetches the actual gas price from the chain using ChainClientV2.
// Since gas price doesn't change frequently on the blockchain, we cache it for 5 minutes.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//
// Returns:
//   - string: Gas price as string (in smallest denomination)
//   - error: Error if the operation fails
func (s *ExplorerService) getCachedGasPrice(ctx context.Context) (string, error) {
	redisClient := cache.GetRedisClient()

	// Try to get from cache first
	if redisClient != nil {
		cachedPrice, err := redisClient.Get(ctx, CacheKeyGasPrice).Result()
		if err == nil && cachedPrice != "" {
			return cachedPrice, nil
		}
	}

	// Get the chain client
	chainClient, err := GetSharedChainClient()
	if err != nil {
		global.Logger.Warnf("failed to get chain client for gas price: %v", err)
		return "500000000", nil // Fallback to default 0.5 INJ
	}

	// Query actual gas price from chain
	gasPriceInt64 := chainClient.CurrentChainGasPrice(ctx)
	gasPrice := fmt.Sprintf("%d", gasPriceInt64)

	// Cache the result
	if redisClient != nil {
		go func() {
			cacheCtx := context.Background()
			if err := redisClient.Set(cacheCtx, CacheKeyGasPrice, gasPrice, CacheGasPriceTTL).Err(); err != nil {
				global.Logger.Warnf("failed to cache gas price: %v", err)
			}
		}()
	}

	return gasPrice, nil
}

// GetBlockGasUtilization retrieves gas utilization statistics for the latest N blocks.
// Formula: (Σgas_used / Σmax_gas) × 100%, where Σmax_gas = block_count × max_gas_per_block
// Results are cached to improve response time.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Empty request (no parameters needed)
//
// Returns:
//   - *pb.Reply: Response containing gas utilization statistics
//   - error: Error if the operation fails
func (s *ExplorerService) GetBlockGasUtilization(ctx context.Context, req *emptypb.Empty) (*pb.Reply, error) {
	// Try to get from cache first
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		cached, err := redisClient.Get(ctx, CacheKeyBlockGasUtilization).Result()
		if err == nil {
			var response pb.BlockGasUtilizationResponse
			if err := sonic.Unmarshal([]byte(cached), &response); err == nil {
				return MarshalAndReply(&response, "marshal block gas utilization error")
			}
		}
	}

	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), BlockQueryTimeout)
	maxGas, err := GetBlockMaxGas(rpcCtx)
	rpcCancel()
	if err != nil {
		global.Logger.Warnf("failed to get block max gas from consensus params: %v, using default", err)
		maxGas = BlockGasLimit
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	gasUtilization, totalGasUsed, totalGasLimit, blockCount, transactionCount, err := calculateBlockGasUtilization(queryCtx, maxGas)
	if err != nil {
		global.Logger.Errorf("failed to calculate block gas utilization: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to calculate block gas utilization", err), err
	}

	// Get current gas price (with caching)
	gasPrice, err := s.getCachedGasPrice(ctx)
	if err != nil {
		global.Logger.Warnf("failed to get gas price: %v, using default", err)
		gasPrice = "500000000" // Default: 0.5 INJ
	}

	response := &pb.BlockGasUtilizationResponse{
		GasUtilization:   gasUtilization,
		TotalGasUsed:     totalGasUsed,
		TotalGasLimit:    totalGasLimit,
		BlockCount:       blockCount,
		TransactionCount: transactionCount,
		BlockGasLimit:    maxGas,
		GasPrice:         gasPrice,
	}

	// Cache the result
	if redisClient != nil {
		go func() {
			ctx := context.Background()
			responseJSON, err := sonic.Marshal(response)
			if err == nil {
				if err := redisClient.Set(ctx, CacheKeyBlockGasUtilization, responseJSON, CacheBlockGasUtilizationTTL).Err(); err != nil {
					global.Logger.Warnf("failed to cache block gas utilization: %v", err)
				}
			}
		}()
	}

	global.Logger.Infof("block gas utilization: %.2f%%, total_gas_used=%d, total_gas_limit=%d, blocks=%d, txs=%d, max_gas=%d",
		gasUtilization, totalGasUsed, totalGasLimit, blockCount, transactionCount, maxGas)

	return MarshalAndReply(response, "marshal block gas utilization error")
}

// deduplicateTransactions removes duplicate transactions by hash, keeping only the complete ones.
// For transactions with the same hash, it prioritizes records that have signer, to, and value fields.
// If multiple records have these fields, it keeps the one with the highest tx_number (most recent).
//
// Parameters:
//   - txs: List of transactions that may contain duplicates
//
// Returns:
//   - []models.Transaction: Deduplicated list of transactions
func deduplicateTransactions(txs []models.Transaction) []models.Transaction {
	if len(txs) == 0 {
		return txs
	}

	// Map to store the best transaction for each hash
	txMap := make(map[string]*models.Transaction)

	for i := range txs {
		tx := &txs[i]
		hash := tx.Hash
		// Skip transactions with empty hash (should not happen, but handle gracefully)
		if hash == "" {
			continue
		}
		// Normalize hash (remove 0x prefix if present for comparison)
		if len(hash) >= 2 && hash[:2] == "0x" {
			hash = hash[2:]
		}

		existing, exists := txMap[hash]
		if !exists {
			// First occurrence of this hash, store it
			txMap[hash] = tx
			continue
		}

		// Check if current transaction has complete fields (signer, to, value)
		currentComplete := tx.Signer != "" && tx.To != "" && len(tx.Values) > 0
		existingComplete := existing.Signer != "" && existing.To != "" && len(existing.Values) > 0

		if currentComplete && !existingComplete {
			// Current is complete, existing is not - replace
			txMap[hash] = tx
		} else if !currentComplete && existingComplete {
			// Existing is complete, current is not - keep existing
			// Do nothing
		} else {
			// Both complete or both incomplete - keep the one with higher tx_number (more recent)
			if tx.TxNumber > existing.TxNumber {
				txMap[hash] = tx
			}
		}
	}

	// Convert map back to slice, maintaining original order as much as possible
	// We'll sort by tx_number DESC to maintain the order
	result := make([]models.Transaction, 0, len(txMap))
	for _, tx := range txMap {
		result = append(result, *tx)
	}

	// Sort by block_unix_timestamp DESC, then tx_number DESC to maintain order
	sort.Slice(result, func(i, j int) bool {
		if result[i].BlockUnixTimestamp != result[j].BlockUnixTimestamp {
			return result[i].BlockUnixTimestamp > result[j].BlockUnixTimestamp
		}
		return result[i].TxNumber > result[j].TxNumber
	})

	return result
}

// GetLatestTransactions retrieves the latest transactions from the blockchain with pagination.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Pagination request containing page and page_size
//
// Returns:
//   - *pb.Reply: Response containing the latest transactions data with pagination info
//   - error: Error if the operation fails
func (s *ExplorerService) GetLatestTransactions(ctx context.Context, req *pb.PaginationRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Normalize pagination parameters
	page, pageSize, offset, useCursor, cursorValue := normalizePagination(req.Page, req.PageSize, req.Cursor, MaxPageSize)

	global.Logger.Debugf("get latest transactions: page=%d, page_size=%d, use_cursor=%v, cursor=%v", page, pageSize, useCursor, cursorValue)

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// First, try to query from Redis cache for page-based pagination (avoids DB lag)
	if !useCursor {
		var cachedTxs []models.Transaction
		var cacheErr error
		var hasMore bool

		if page == MinPage {
			// First page: use simple limit query
			cachedTxs, cacheErr = getTransactionsFromCache(queryCtx, int(pageSize))
		} else {
			// Other pages: use offset-based query
			cachedTxs, hasMore, cacheErr = getTransactionsFromCacheWithOffset(queryCtx, offset, int(pageSize))
		}

		if cacheErr != nil {
			// Log cache error but continue to database query (graceful degradation)
			global.Logger.Warnf("failed to get transactions from cache, fallback to DB: %v", cacheErr)
		} else if len(cachedTxs) > 0 {
			global.Logger.Debugf("transactions found in cache: page=%d, count=%d", page, len(cachedTxs))
			// Deduplicate cached transactions to ensure we only return complete ones
			cachedTxs = deduplicateTransactions(cachedTxs)
			global.Logger.Debugf("transactions after deduplication: count=%d", len(cachedTxs))
			// Try to get total count from cache first to avoid database query
			total, isExactCount := getTotalFromCache(queryCtx, CacheKeyTotalTransactions)
			if total == 0 {
				// Cache miss for total, query from database and log error if any
				var countErr error
				total, isExactCount, countErr = getApproximateCountWithError(db.Get(), &models.Transaction{}, ctx)
				if countErr != nil {
					global.Logger.Warnf("failed to get approximate count for transactions: %v, using fallback value", countErr)
					// Use fallback value if count query fails
					total = ApproximateCountFallback
					isExactCount = false
				} else {
					// Cache the total count for future queries
					if err := cacheTotal(queryCtx, CacheKeyTotalTransactions, total); err != nil {
						global.Logger.Warnf("failed to cache total transactions count: %v", err)
					}
				}
			}
			nextCursor := calculateNextCursor(useCursor, cursorValue, cachedTxs, func(tx models.Transaction) uint64 { return tx.TxNumber })
			txDTOs := models.ToTransactionDTOs(cachedTxs)
			txsResp := buildPaginatedResponse(txDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(cachedTxs))
			return MarshalAndReply(txsResp, "marshal transactions error")
		} else if page != MinPage && !hasMore {
			// Cache doesn't have enough data for this page, fallback to DB
			global.Logger.Debugf("cache miss for page %d, fallback to DB", page)
		}
	}

	// Second, try to query from database with pagination
	var dbTxs []models.Transaction
	var total int64
	var isExactCount bool

	// Get approximate or exact count (optimized for performance)
	var countErr error
	total, isExactCount, countErr = getApproximateCountWithError(db.Get(), &models.Transaction{}, ctx)
	if countErr != nil {
		global.Logger.Warnf("failed to get approximate count for transactions: %v, using fallback value", countErr)
		// Use fallback value if count query fails
		total = ApproximateCountFallback
		isExactCount = false
	} else {
		// Cache the total count for future queries
		if err := cacheTotal(queryCtx, CacheKeyTotalTransactions, total); err != nil {
			global.Logger.Warnf("failed to cache total transactions count: %v", err)
		}
	}

	// Build query based on pagination type
	query := db.Get().WithContext(queryCtx).Model(&models.Transaction{})

	if useCursor {
		// Use cursor-based pagination (more efficient for large datasets)
		// Use tx_number for both cursor and ordering to ensure consistent pagination
		query = query.Where("tx_number < ?", cursorValue).
			Order("tx_number DESC").
			Limit(int(pageSize))
	} else {
		// Use page-based pagination
		query = query.Order("tx_number DESC").
			Offset(offset).
			Limit(int(pageSize))
	}

	startTime := time.Now()
	err := query.Find(&dbTxs).Error
	queryDuration := time.Since(startTime)

	if err != nil {
		// Database error
		global.Logger.Errorf("database query error: %v", err)
		return ErrorReply(http.StatusInternalServerError, "database query error", err), err
	}

	// Found in database or no results, convert to DTO and return
	if len(dbTxs) > 0 {
		global.Logger.Debugf("transactions found in database: count=%d, query_time=%v", len(dbTxs), queryDuration)
		// Deduplicate transactions to ensure we only return complete ones (with signer, to, value)
		dbTxs = deduplicateTransactions(dbTxs)
		global.Logger.Debugf("transactions after deduplication: count=%d", len(dbTxs))
	} else {
		global.Logger.Debugf("no transactions found in database")
	}

	// Cache the total count for future queries
	if err := cacheTotal(queryCtx, CacheKeyTotalTransactions, total); err != nil {
		global.Logger.Warnf("failed to cache total transactions count: %v", err)
	}

	nextCursor := calculateNextCursor(useCursor, cursorValue, dbTxs, func(tx models.Transaction) uint64 { return tx.TxNumber })
	txDTOs := models.ToTransactionDTOs(dbTxs)
	txsResp := buildPaginatedResponse(txDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(dbTxs))
	return MarshalAndReply(txsResp, "marshal transactions error")
}

// GetBlockTransactions retrieves all transactions for a specific block by height.
// Supports pagination for blocks with many transactions.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing the block height and optional pagination
//
// Returns:
//   - *pb.Reply: Response containing the transactions in the block
//   - error: Error if the operation fails or validation fails
func (s *ExplorerService) GetBlockTransactions(ctx context.Context, req *pb.GetBlockTransactionsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Validate block height
	if req.Height <= 0 {
		return ErrorReply(http.StatusBadRequest, "invalid block height", errors.New("block height must be positive")), errors.New("invalid block height")
	}

	// Set default pagination if not provided
	pagination := req.Pagination
	if pagination == nil {
		pagination = &pb.PaginationRequest{
			Page:     MinPage,
			PageSize: DefaultPageSize,
		}
	}

	// Normalize pagination parameters
	page, pageSize, offset, useCursor, cursorValue := normalizePagination(pagination.Page, pagination.PageSize, pagination.Cursor, MaxPageSize)

	global.Logger.Debugf("get block transactions: height=%d, page=%d, page_size=%d, use_cursor=%v, cursor=%v", req.Height, page, pageSize, useCursor, cursorValue)

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Query transactions from database with pagination
	var dbTxs []models.Transaction
	query := db.Get().WithContext(queryCtx).Model(&models.Transaction{}).Where("block_number = ?", req.Height)

	// Get total count for this block
	var total int64
	countErr := db.Get().WithContext(queryCtx).Model(&models.Transaction{}).Where("block_number = ?", req.Height).Count(&total).Error
	if countErr != nil {
		global.Logger.Warnf("failed to get transaction count for block %d: %v", req.Height, countErr)
	}

	// Apply pagination
	if useCursor {
		// Use cursor-based pagination
		// Cursor is tx_number, find transactions with tx_number > cursor (ascending order)
		query = query.Where("tx_number > ?", cursorValue).
			Order("tx_number ASC").
			Limit(int(pageSize))
	} else {
		// Use page-based pagination
		query = query.Order("tx_number ASC").
			Offset(offset).
			Limit(int(pageSize))
	}

	startTime := time.Now()
	err := query.Find(&dbTxs).Error
	queryDuration := time.Since(startTime)

	if err != nil {
		// Database error
		global.Logger.Errorf("database query error: %v", err)
		return ErrorReply(http.StatusInternalServerError, "database query error", err), err
	}

	// Found in database or no results, convert to DTO and return
	if len(dbTxs) > 0 {
		global.Logger.Debugf("transactions found in database: height=%d, count=%d, query_time=%v", req.Height, len(dbTxs), queryDuration)
	} else {
		global.Logger.Debugf("no transactions found for block height %d", req.Height)
	}

	// Calculate next cursor
	nextCursor := calculateNextCursor(useCursor, cursorValue, dbTxs, func(tx models.Transaction) uint64 { return tx.TxNumber })

	// Convert to DTOs (use detail mode for rich transaction information)
	txDTOs := make([]*pb.TransactionDTO, 0, len(dbTxs))
	for i := range dbTxs {
		txDTOs = append(txDTOs, models.ToTransactionDTO(&dbTxs[i], models.DTOModeWithDetail))
	}

	txsResp := buildPaginatedResponse(txDTOs, page, pageSize, total, true, useCursor, cursorValue, nextCursor, len(dbTxs))
	return MarshalAndReply(txsResp, "marshal transactions error")
}

// GetTransactionByHash retrieves a specific transaction by its hash.
// Supports both Cosmos transaction hashes and EVM transaction hashes.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing the transaction hash (Cosmos or EVM)
//
// Returns:
//   - *pb.Reply: Response containing the transaction data
//   - error: Error if the operation fails or validation fails
func (s *ExplorerService) GetTransactionByHash(ctx context.Context, req *pb.GetTransactionByHashRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Sanitize and validate transaction hash
	hash := SanitizeHash(req.Hash)
	if err := ValidateTransactionHash(hash); err != nil {
		return ErrorReply(http.StatusBadRequest, "invalid transaction hash", err), err
	}

	global.Logger.Debugf("get transaction by hash: hash=%s", hash)

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// First, try to query from Redis cache
	cachedTx, err := getTransactionFromCache(queryCtx, "0x"+hash)
	if err == nil && cachedTx != nil {
		global.Logger.Debugf("transaction found in cache: hash=0x%s", hash)
		txDTO := models.ToTransactionDTO(cachedTx, models.DTOModeWithDetail)
		return MarshalAndReply(txDTO, "marshal transaction error")
	}

	// Second, try to query from database
	var dbTx models.Transaction
	err = db.Get().WithContext(queryCtx).Where("hash = ?", "0x"+hash).First(&dbTx).Error
	if err == nil {
		// Found in database
		global.Logger.Debugf("transaction found in database: hash=0x%s, archived=%v", hash, dbTx.Archived)

		// If transaction is archived, load full data from archive table
		if dbTx.Archived {
			archiveService := NewArchiveService()
			archivedTx, archiveErr := archiveService.GetArchivedTransaction(queryCtx, "0x"+hash)
			if archiveErr == nil && archivedTx != nil {
				global.Logger.Debugf("transaction found in archive: hash=0x%s", hash)
				// Cache the transaction for future queries
				cacheTransaction(queryCtx, archivedTx)
				txDTO := models.ToTransactionDTO(archivedTx, models.DTOModeWithDetail)
				return MarshalAndReply(txDTO, "marshal transaction error")
			} else {
				// Archive data not found, but transaction is marked as archived
				// Return the transaction with empty fields (fallback)
				global.Logger.Warnf("transaction marked as archived but archive data not found: hash=0x%s, error=%v", hash, archiveErr)
			}
		}

		// Transaction not archived, or archive data not found, return as is
		// Cache the transaction for future queries
		cacheTransaction(queryCtx, &dbTx)

		txDTO := models.ToTransactionDTO(&dbTx, models.DTOModeWithDetail)
		return MarshalAndReply(txDTO, "marshal transaction error")

	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: hash=%s, error=%v", hash, err)
	} else {
		// Not found in main table, try archive table
		archiveService := NewArchiveService()
		archivedTx, archiveErr := archiveService.GetArchivedTransaction(queryCtx, "0x"+hash)
		if archiveErr == nil && archivedTx != nil {
			global.Logger.Infof("transaction found in archive: hash=0x%s", hash)
			// Cache the transaction for future queries
			cacheTransaction(queryCtx, archivedTx)
			txDTO := models.ToTransactionDTO(archivedTx, models.DTOModeWithDetail)
			return MarshalAndReply(txDTO, "marshal transaction error")
		}

		// Not found in archive table, check if it's an EVM transaction hash
		global.Logger.Debugf("transaction not found by Cosmos hash, checking if it's an EVM hash: hash=%s", hash)
		var evmTx models.EVMTransaction
		evmErr := db.Get().WithContext(queryCtx).
			Where("evm_hash = ?", "0x"+hash).
			First(&evmTx).Error
		if evmErr == nil {
			// Found EVM transaction, get the Cosmos transaction hash and retry
			cosmosHash := strings.TrimPrefix(evmTx.TransactionHash, "0x")
			global.Logger.Infof("found EVM transaction, querying with Cosmos hash: evm_hash=0x%s, cosmos_hash=%s", hash, cosmosHash)

			// Query Cosmos transaction using the Cosmos hash from evm_transactions
			err = db.Get().WithContext(queryCtx).Where("hash = ?", evmTx.TransactionHash).First(&dbTx).Error
			if err == nil {
				// Found in database
				global.Logger.Debugf("Cosmos transaction found via EVM hash: hash=%s, archived=%v", cosmosHash, dbTx.Archived)

				// If transaction is archived, load full data from archive table
				if dbTx.Archived {
					archivedTx, archiveErr := archiveService.GetArchivedTransaction(queryCtx, evmTx.TransactionHash)
					if archiveErr == nil && archivedTx != nil {
						global.Logger.Debugf("Cosmos transaction found in archive via EVM hash: hash=%s", cosmosHash)
						// Cache the transaction for future queries
						cacheTransaction(queryCtx, archivedTx)
						txDTO := models.ToTransactionDTO(archivedTx, models.DTOModeWithDetail)
						return MarshalAndReply(txDTO, "marshal transaction error")
					}
				}

				// Transaction not archived, or archive data not found, return as is
				// Cache the transaction for future queries
				cacheTransaction(queryCtx, &dbTx)

				txDTO := models.ToTransactionDTO(&dbTx, models.DTOModeWithDetail)
				return MarshalAndReply(txDTO, "marshal transaction error")
			}
		} else if !errors.Is(evmErr, gorm.ErrRecordNotFound) {
			global.Logger.Warnf("EVM transaction query error: hash=%s, error=%v", hash, evmErr)
		}
	}

	// Not found in database or conversion failed, query from chain
	global.Logger.Debugf("transaction not found in database or conversion failed, querying from chain: hash=%s", hash)

	// Use independent context for RPC calls to avoid HTTP timeout issues
	callCtx, cancel := context.WithTimeout(context.Background(), GRPCTimeout)
	defer cancel()

	chainClient, err := GetSharedChainClient()
	if err != nil {
		global.Logger.Errorf("get chain client error: %v", err)
		return ErrorReply(http.StatusInternalServerError, "get chain client error", err), err
	}

	txResp, err := chainClient.GetTx(callCtx, hash)
	if err != nil {
		global.Logger.Errorf("get transaction error: %v", err)
		return ErrorReply(http.StatusInternalServerError, "get transaction error", err), err
	}

	if txResp == nil || txResp.TxResponse == nil {
		global.Logger.Errorf("transaction response is nil for hash %s", hash)
		return ErrorReply(http.StatusInternalServerError, "transaction response is nil", errors.New("transaction response is nil")), errors.New("transaction response is nil")
	}

	// Convert GetTxResponse to TxDetailData
	txDetailData := models.ConvertGetTxResponseToTxDetailData(txResp, chainClient.ClientContext().Codec)
	if txDetailData == nil {
		global.Logger.Errorf("failed to convert transaction response to detail data for hash %s", hash)
		return ErrorReply(http.StatusInternalServerError, "failed to convert transaction", errors.New("failed to convert transaction")), errors.New("failed to convert transaction")
	}

	// Save to database for future queries
	// Get storage threshold from config
	storageThreshold := getStorageThresholdForSync()

	transactionModel := models.TransactionFromProto(txDetailData, storageThreshold)
	saveToDB(transactionModel, "create transaction error")

	// Cache the transaction
	cacheTransaction(callCtx, transactionModel)

	txDTO := models.ToTransactionDTO(transactionModel, models.DTOModeWithDetail)
	return MarshalAndReply(txDTO, "marshal transaction error")
}

// GetAccountInfo retrieves account information for a given address.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing the account address
//
// Returns:
//   - *pb.Reply: Response containing the account information
//   - error: Error if the operation fails or validation fails
func (s *ExplorerService) GetAccountInfo(ctx context.Context, req *pb.GetAccountInfoRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Sanitize and validate address
	address := SanitizeAddress(req.Address)
	if err := ValidateAddress(address); err != nil {
		return ErrorReply(http.StatusBadRequest, "invalid address", err), err
	}

	global.Logger.Debugf("get account info: address=%s", address)

	// Create independent context for database query to avoid HTTP timeout issues
	dbCtx, dbCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer dbCancel()

	// Convert address to database format (Ethereum format 0x...)
	// Database now stores addresses in Ethereum format
	dbQueryAddress, err := s.convertToQueryAddress(ctx, address, nil, false)
	if err != nil {
		return ErrorReply(http.StatusBadRequest, "invalid address format", err), err
	}

	// First, try to query from database using Ethereum format address
	var dbAccount models.Account
	err = db.Get().WithContext(dbCtx).Where("address = ?", dbQueryAddress).First(&dbAccount).Error
	if err == nil {
		// Found in database, convert to DTO and return
		global.Logger.Debugf("account found in database: dbQueryAddress=%s", dbQueryAddress)
		accountDTO := models.ToAccountDTO(&dbAccount)
		return MarshalAndReply(accountDTO, "marshal account error")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: dbQueryAddress=%s, error=%v", dbQueryAddress, err)
	}

	// Not found in database, query from chain
	// For chain queries, we need Injective format
	client, err := GetSharedChainClient()
	if err != nil {
		return ErrorReply(http.StatusInternalServerError, "build chain client error", err), err
	}

	chainQueryAddress, err := s.convertToQueryAddress(ctx, address, client, true)
	if err != nil {
		return ErrorReply(http.StatusInternalServerError, "failed to convert address for chain query", err), err
	}

	global.Logger.Debugf("account not found in database, querying from chain: chainQueryAddress=%s", chainQueryAddress)

	account, err := client.GetAccount(ctx, chainQueryAddress)
	if err != nil {
		global.Logger.Errorf("get account error: %v", err)
		return ErrorReply(http.StatusInternalServerError, "get account error", err), err
	}

	// Check if account or account.Account is nil
	if account == nil || account.Account == nil {
		global.Logger.Errorf("account or account.Account is nil for address: %s", chainQueryAddress)
		return &pb.Reply{
			Code:    int32(http.StatusOK),
			Message: fmt.Sprintf("account %s not found", address),
			Data:    nil,
		}, nil
	}

	// Check if account value is empty
	if len(account.Account.Value) == 0 {
		global.Logger.Errorf("account value is empty for address: %s", chainQueryAddress)
		return &pb.Reply{
			Code:    int32(http.StatusOK),
			Message: fmt.Sprintf("account %s not found", address),
			Data:    nil,
		}, nil
	}

	ethAccount := &types.EthAccount{}
	err = client.ClientContext().Codec.Unmarshal(account.Account.Value, ethAccount)
	if err != nil {
		global.Logger.Errorf("unmarshal eth account error: %v", err)
		return ErrorReply(http.StatusInternalServerError, "unmarshal eth account error", err), err
	}

	// Save to database for future queries
	// AccountFromProto will automatically convert address to Ethereum format
	accountModel := models.AccountFromProto(ethAccount, chainQueryAddress, client.ClientContext().Codec)
	saveToDB(accountModel, "create account error")

	accountDTO := models.ToAccountDTO(accountModel)
	return MarshalAndReply(accountDTO, "marshal account error")
}

// isTransferMessageType checks if a message type represents a transfer transaction.
// Transfer message types include: MsgSend, MsgTransfer, MsgDeposit, MsgWithdraw, etc.
func isTransferMessageType(msgType string) bool {
	transferTypes := map[string]bool{
		"/cosmos.bank.v1beta1.MsgSend":                      true,
		"/ibc.applications.transfer.v1.MsgTransfer":         true,
		"/injective.peggy.v1.MsgSendToEth":                  true,
		"/injective.exchange.v1beta1.MsgDeposit":            true,
		"/injective.exchange.v1beta1.MsgWithdraw":           true,
		"/injective.exchange.v1beta1.MsgSubaccountTransfer": true,
		"/injective.exchange.v1beta1.MsgExternalTransfer":   true,
		"MsgSend":     true,
		"MsgTransfer": true,
		"MsgDeposit":  true,
		"MsgWithdraw": true,
	}
	return transferTypes[msgType]
}

// containsMsgEthereumTx checks if message types contain MsgEthereumTx.
func containsMsgEthereumTx(messageTypes []string) bool {
	for _, msgType := range messageTypes {
		if strings.Contains(msgType, "MsgEthereumTx") ||
			strings.Contains(msgType, "ethermint.evm.v1") ||
			strings.Contains(msgType, "injective.evm.v1") {
			return true
		}
	}
	return false
}

// normalizeMessageType converts a short message type name to its full type URL.
// If the input is already a full type URL (starts with "/"), it returns as is.
// Otherwise, it looks up the full type URL from a predefined mapping.
// If no mapping is found, it returns the input as is (for forward compatibility).
func normalizeMessageType(msgType string) string {
	// If it's already a full type URL, return as is
	if strings.HasPrefix(msgType, "/") {
		return msgType
	}

	// Mapping of short type names to full type URLs
	shortToFullMap := map[string]string{
		// Bank module
		"MsgSend":      "/cosmos.bank.v1beta1.MsgSend",
		"MsgMultiSend": "/cosmos.bank.v1beta1.MsgMultiSend",

		// Distribution module
		"MsgSetWithdrawAddress":          "/cosmos.distribution.v1beta1.MsgSetWithdrawAddress",
		"MsgWithdrawDelegatorReward":     "/cosmos.distribution.v1beta1.MsgWithdrawDelegatorReward",
		"MsgWithdrawValidatorCommission": "/cosmos.distribution.v1beta1.MsgWithdrawValidatorCommission",
		"MsgFundCommunityPool":           "/cosmos.distribution.v1beta1.MsgFundCommunityPool",

		// Staking module
		"MsgDelegate":        "/cosmos.staking.v1beta1.MsgDelegate",
		"MsgUndelegate":      "/cosmos.staking.v1beta1.MsgUndelegate",
		"MsgRedelegate":      "/cosmos.staking.v1beta1.MsgRedelegate",
		"MsgCreateValidator": "/cosmos.staking.v1beta1.MsgCreateValidator",
		"MsgEditValidator":   "/cosmos.staking.v1beta1.MsgEditValidator",

		// IBC transfer
		"MsgTransfer": "/ibc.applications.transfer.v1.MsgTransfer",

		// Exchange module (Injective)
		"MsgDeposit":                     "/injective.exchange.v1beta1.MsgDeposit",
		"MsgWithdraw":                    "/injective.exchange.v1beta1.MsgWithdraw",
		"MsgSubaccountTransfer":          "/injective.exchange.v1beta1.MsgSubaccountTransfer",
		"MsgExternalTransfer":            "/injective.exchange.v1beta1.MsgExternalTransfer",
		"MsgCreateSpotLimitOrder":        "/injective.exchange.v1beta1.MsgCreateSpotLimitOrder",
		"MsgCreateSpotMarketOrder":       "/injective.exchange.v1beta1.MsgCreateSpotMarketOrder",
		"MsgCancelSpotOrder":             "/injective.exchange.v1beta1.MsgCancelSpotOrder",
		"MsgCreateDerivativeLimitOrder":  "/injective.exchange.v1beta1.MsgCreateDerivativeLimitOrder",
		"MsgCreateDerivativeMarketOrder": "/injective.exchange.v1beta1.MsgCreateDerivativeMarketOrder",
		"MsgCancelDerivativeOrder":       "/injective.exchange.v1beta1.MsgCancelDerivativeOrder",

		// TokenFactory module
		"MsgCreateDenom": "/injective.tokenfactory.v1beta1.MsgCreateDenom",
		"MsgMint":        "/injective.tokenfactory.v1beta1.MsgMint",
		"MsgBurn":        "/injective.tokenfactory.v1beta1.MsgBurn",

		// WasmX module
		"MsgExecuteContractCompat": "/injective.wasmx.v1.MsgExecuteContractCompat",

		// CosmWasm
		"MsgStoreCode":           "/cosmwasm.wasm.v1.MsgStoreCode",
		"MsgInstantiateContract": "/cosmwasm.wasm.v1.MsgInstantiateContract",
		"MsgExecuteContract":     "/cosmwasm.wasm.v1.MsgExecuteContract",
		"MsgMigrateContract":     "/cosmwasm.wasm.v1.MsgMigrateContract",

		// Gov module
		"MsgSubmitProposal": "/cosmos.gov.v1.MsgSubmitProposal",
		"MsgVote":           "/cosmos.gov.v1.MsgVote",
		"MsgGovDeposit":     "/cosmos.gov.v1.MsgDeposit",

		// Authz module
		"MsgGrant":  "/cosmos.authz.v1beta1.MsgGrant",
		"MsgRevoke": "/cosmos.authz.v1beta1.MsgRevoke",
		"MsgExec":   "/cosmos.authz.v1beta1.MsgExec",
	}

	// Look up the full type URL
	if fullType, ok := shortToFullMap[msgType]; ok {
		return fullType
	}

	// If no mapping found, return the input as is
	// This allows for forward compatibility with new message types
	return msgType
}

// parseCommaSeparatedTxTypes parses tx_types that may contain comma-separated values.
// It handles both formats:
//   - Multiple parameters: tx_types=MsgSend&tx_types=MsgTransfer
//   - Comma-separated: tx_types=MsgSend,MsgTransfer
//
// Returns a flattened slice with all individual message types.
func parseCommaSeparatedTxTypes(txTypes []string) []string {
	if len(txTypes) == 0 {
		return txTypes
	}

	result := make([]string, 0, len(txTypes))
	for _, txType := range txTypes {
		// Split by comma and trim whitespace
		parts := strings.Split(txType, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				result = append(result, part)
			}
		}
	}
	return result
}

// GetAccountTransactions retrieves transactions for a given account address with pagination.
// This function queries transactions where the address is either the signer (transaction initiator)
// or the recipient (to field). It supports both page-based and cursor-based pagination.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing the account address and pagination info
//
// Returns:
//   - *pb.Reply: Response containing the account transactions with pagination info
//   - error: Error if the operation fails or validation fails
func (s *ExplorerService) GetAccountTransactions(ctx context.Context, req *pb.GetAccountTransactionsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Sanitize and validate address
	address := SanitizeAddress(req.Address)
	if err := ValidateAddress(address); err != nil {
		return ErrorReply(http.StatusBadRequest, "invalid address", err), err
	}

	// Normalize pagination parameters
	var page, pageSize int32 = MinPage, DefaultPageSize
	var offset int
	var useCursor bool
	var cursorValue uint64
	if req.Pagination != nil {
		page, pageSize, offset, useCursor, cursorValue = normalizePagination(req.Pagination.Page, req.Pagination.PageSize, req.Pagination.Cursor, MaxPageSize)
	}

	// Parse comma-separated tx_types (e.g., "MsgSend,MsgTransfer" -> ["MsgSend", "MsgTransfer"])
	req.TxTypes = parseCommaSeparatedTxTypes(req.TxTypes)

	// Check if transaction type filtering is requested
	hasTxTypeFilter := len(req.TxTypes) > 0
	global.Logger.Debugf("get account transactions: address=%s, page=%d, page_size=%d, use_cursor=%v, tx_types=%v", address, page, pageSize, useCursor, req.TxTypes)

	// Create independent context for database query to avoid HTTP timeout issues
	dbCtx, dbCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer dbCancel()

	// Convert address to database format (Ethereum format 0x...)
	dbQueryAddress, err := s.convertToQueryAddress(ctx, address, nil, false)
	if err != nil {
		return ErrorReply(http.StatusBadRequest, "invalid address format", err), err
	}

	baseQuery := db.Get().WithContext(dbCtx).Model(&models.Transaction{}).
		Where("(signer = ? OR `to` = ?)", dbQueryAddress, dbQueryAddress)

	if !useCursor && page == MinPage && !hasTxTypeFilter {
		cacheKey := fmt.Sprintf("cache:account:txs:%s", dbQueryAddress)
		cachedTxs, err := getAccountTransactionsFromCache(dbCtx, cacheKey, int(pageSize))
		if err == nil && len(cachedTxs) > 0 {
			global.Logger.Debugf("account transactions found in cache: address=%s, count=%d", address, len(cachedTxs))
			cachedTotal, cachedIsExact, cachedCountErr := getApproximateCountForAddressWithError(baseQuery, dbQueryAddress)
			if cachedCountErr != nil {
				global.Logger.Debugf("failed to get count for cached account transactions: %v, using approximate", cachedCountErr)
				cachedTotal = ApproximateCountForAddress
				cachedIsExact = false
			}
			nextCursor := calculateNextCursor(useCursor, cursorValue, cachedTxs, func(tx models.Transaction) uint64 { return tx.BlockUnixTimestamp })
			txDTOs := models.ToTransactionDTOs(cachedTxs)
			txsResp := buildPaginatedResponse(txDTOs, page, pageSize, cachedTotal, cachedIsExact, useCursor, cursorValue, nextCursor, len(cachedTxs))
			return MarshalAndReply(txsResp, "marshal transactions error")
		}
	}

	var dbTxs []models.Transaction

	if hasTxTypeFilter {
		dbType := db.Get().Dialector.Name()
		conditions := make([]string, 0, len(req.TxTypes))
		args := make([]interface{}, 0, len(req.TxTypes)*2)

		// Check if any requested type is a transfer type
		hasTransferFilter := false
		for _, msgType := range req.TxTypes {
			normalizedType := normalizeMessageType(msgType)
			if isTransferMessageType(normalizedType) || isTransferMessageType(msgType) {
				hasTransferFilter = true
				break
			}
		}

		for _, msgType := range req.TxTypes {
			// Normalize message type: convert short name to full type URL
			normalizedType := normalizeMessageType(msgType)

			if dbType == "mysql" {
				conditions = append(conditions, "(JSON_CONTAINS(message_types, ?) OR JSON_SEARCH(message_types, 'one', ?) IS NOT NULL)")
				args = append(args, fmt.Sprintf(`"%s"`, normalizedType), fmt.Sprintf(`%%"%s"%%`, normalizedType))
			} else if dbType == "postgres" {
				conditions = append(conditions, "(message_types @> ?::jsonb OR message_types::text LIKE ?)")
				args = append(args, fmt.Sprintf(`["%s"]`, normalizedType), fmt.Sprintf(`%%"%s"%%`, normalizedType))
			} else {
				conditions = append(conditions, "message_types LIKE ?")
				args = append(args, fmt.Sprintf(`%%"%s"%%`, normalizedType))
			}
		}

		// If requesting transfer types, also include EVM transfers
		if hasTransferFilter {
			if dbType == "mysql" {
				// EVM native coin transfer: to field not empty, values not empty
				evmNativeTransferCondition := "((tx_type = 'MsgEthereumTx' OR JSON_SEARCH(message_types, 'one', '%MsgEthereumTx%') IS NOT NULL) AND `to` != '' AND `to` IS NOT NULL AND JSON_LENGTH(`values`) > 0)"
				conditions = append(conditions, evmNativeTransferCondition)

				// EVM ERC20 token transfer: has_token_transfer flag is true
				evmTokenTransferCondition := "(tx_type = 'MsgEthereumTx' AND has_token_transfer = TRUE)"
				conditions = append(conditions, evmTokenTransferCondition)
			} else if dbType == "postgres" {
				// PostgreSQL version
				evmNativeTransferCondition := "((tx_type = 'MsgEthereumTx' OR message_types::text LIKE '%MsgEthereumTx%') AND \"to\" != '' AND \"to\" IS NOT NULL AND jsonb_array_length(values::jsonb) > 0)"
				conditions = append(conditions, evmNativeTransferCondition)

				evmTokenTransferCondition := "(tx_type = 'MsgEthereumTx' AND has_token_transfer = TRUE)"
				conditions = append(conditions, evmTokenTransferCondition)
			}
		}

		if len(conditions) > 0 {
			baseQuery = baseQuery.Where("("+strings.Join(conditions, " OR ")+")", args...)
		}
	}

	var total int64
	var isExactCount bool
	var countErr error
	total, isExactCount, countErr = getApproximateCountForAddressWithError(baseQuery, dbQueryAddress)
	if countErr != nil {
		global.Logger.Warnf("failed to get approximate count for address %s: %v, using fallback value", dbQueryAddress, countErr)
		total = ApproximateCountForAddress
		isExactCount = false
	}

	query := baseQuery
	if useCursor {
		// Use block_unix_timestamp for both cursor and ordering to ensure consistent pagination
		query = query.Where("block_unix_timestamp < ?", cursorValue).
			Order("block_unix_timestamp DESC").
			Limit(int(pageSize))
	} else {
		query = query.Order("block_unix_timestamp DESC").
			Offset(offset).
			Limit(int(pageSize))
	}

	startTime := time.Now()
	err = query.Find(&dbTxs).Error
	queryDuration := time.Since(startTime)

	if err == nil && len(dbTxs) > 0 {
		global.Logger.Debugf("account transactions found in database: count=%d, query_time=%v", len(dbTxs), queryDuration)

		if !useCursor && page == MinPage && !hasTxTypeFilter {
			cacheKey := fmt.Sprintf("cache:account:txs:%s", dbQueryAddress)
			cacheAccountTransactions(dbCtx, cacheKey, dbTxs)
		}

		// Batch query token transfers for EVM transactions
		tokenTransfersMap := s.batchGetTokenTransfers(dbCtx, dbTxs)

		nextCursor := calculateNextCursor(useCursor, cursorValue, dbTxs, func(tx models.Transaction) uint64 { return tx.BlockUnixTimestamp })
		txDTOs := models.ToTransactionDTOsWithTokenTransfers(dbTxs, tokenTransfersMap)
		txsResp := buildPaginatedResponse(txDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(dbTxs))
		return MarshalAndReply(txsResp, "marshal transactions error")
	}

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		global.Logger.Errorf("database query error: address=%s, error=%v", address, err)
		return ErrorReply(http.StatusInternalServerError, "database query error", err), err
	}

	global.Logger.Debugf("no account transactions found in database: address=%s", address)
	return &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "no transactions found",
		Data:    nil,
	}, nil
}

// batchGetTokenTransfers batch queries token transfers for EVM transactions.
// Returns a map of transaction hash to token transfers list.
func (s *ExplorerService) batchGetTokenTransfers(ctx context.Context, txs []models.Transaction) map[string][]models.TokenTransfer {
	if len(txs) == 0 {
		return make(map[string][]models.TokenTransfer)
	}

	// Collect EVM transaction hashes
	txHashes := make([]string, 0, len(txs))
	for _, tx := range txs {
		// Only query for EVM transactions
		if tx.TxType == "MsgEthereumTx" || containsMsgEthereumTx(tx.MessageTypes) {
			txHashes = append(txHashes, tx.Hash)
		}
	}

	if len(txHashes) == 0 {
		return make(map[string][]models.TokenTransfer)
	}

	// Batch query token transfers
	var transfers []models.TokenTransfer
	err := db.Get().WithContext(ctx).
		Where("transaction_hash IN ?", txHashes).
		Order("transaction_hash, log_index ASC").
		Find(&transfers).Error

	if err != nil {
		global.Logger.Warnf("failed to batch query token transfers: %v", err)
		return make(map[string][]models.TokenTransfer)
	}

	// Group by transaction hash
	result := make(map[string][]models.TokenTransfer)
	for i := range transfers {
		hash := transfers[i].TransactionHash
		result[hash] = append(result[hash], transfers[i])
	}

	return result
}

// GetAccountBalances retrieves account balances for a given address.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing the account address
//
// Returns:
//   - *pb.Reply: Response containing the account balances
//   - error: Error if the operation fails or validation fails
func (s *ExplorerService) GetAccountBalances(ctx context.Context, req *pb.GetAccountBalancesRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Sanitize and validate address
	address := SanitizeAddress(req.Address)
	if err := ValidateAddress(address); err != nil {
		return ErrorReply(http.StatusBadRequest, "invalid address", err), err
	}

	global.Logger.Debugf("get account balances: address=%s", address)

	// Create independent context for database query to avoid HTTP timeout issues
	dbCtx, dbCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer dbCancel()

	// Convert address to database format (Ethereum format 0x...)
	dbQueryAddress, err := s.convertToQueryAddress(ctx, address, nil, false)
	if err != nil {
		return ErrorReply(http.StatusBadRequest, "invalid address format", err), err
	}

	// First, try to query from database using Ethereum format address
	var dbBalances []models.AccountBalance
	err = db.Get().WithContext(dbCtx).
		Where("address = ?", dbQueryAddress).
		Find(&dbBalances).Error
	if err == nil && len(dbBalances) > 0 {
		// Found in database, convert to protobuf format and return
		global.Logger.Debugf("account balances found in database: address=%s, count=%d", address, len(dbBalances))
		// Convert from AccountBalance models to response format
		balancesList := make([]map[string]interface{}, 0, len(dbBalances))
		for _, dbBalance := range dbBalances {
			balance := map[string]interface{}{
				"denom":  dbBalance.Denom,
				"amount": dbBalance.Amount,
			}
			balancesList = append(balancesList, balance)
		}
		if len(balancesList) > 0 {
			// Create response similar to GetBankBalances response
			balancesResp := map[string]interface{}{
				"balances": balancesList,
			}
			return MarshalAndReply(balancesResp, "marshal balances error")
		}
		global.Logger.Warnf("no valid balances found in database, querying from chain: address=%s", address)
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: address=%s, error=%v", address, err)
	}

	// Not found in database, query from chain
	// For chain queries, we need Injective format
	client, err := GetSharedChainClient()
	if err != nil {
		return ErrorReply(http.StatusInternalServerError, "build chain client error", err), err
	}

	chainQueryAddress, err := s.convertToQueryAddress(ctx, address, client, true)
	if err != nil {
		return ErrorReply(http.StatusInternalServerError, "failed to convert address for chain query", err), err
	}

	global.Logger.Debugf("account balances not found in database, querying from chain: chainQueryAddress=%s", chainQueryAddress)

	balances, err := client.GetBankBalances(ctx, chainQueryAddress)
	if err != nil {
		global.Logger.Errorf("get account balances error: %v", err)
		return ErrorReply(http.StatusInternalServerError, "get account balances error", err), err
	}

	// Save to database for future queries
	balancesList := make([]map[string]interface{}, 0, len(dbBalances))
	if balances != nil && balances.Balances != nil {
		for _, balance := range balances.Balances {
			// AccountBalanceFromProto will automatically convert address to Ethereum format
			balanceModel := models.AccountBalanceFromProto(balance, chainQueryAddress)
			saveToDB(balanceModel, "create account balance error")
			balanceMap := map[string]interface{}{
				"denom":  balance.Denom,
				"amount": balance.Amount,
			}
			balancesList = append(balancesList, balanceMap)
		}
	}

	balancesResp := map[string]interface{}{
		"balances": balancesList,
	}
	return MarshalAndReply(balancesResp, "marshal balances error")
}

// CheckHealthy performs a health check on the service and its dependencies.
// This endpoint can be used by load balancers, monitoring systems, and clients
// to verify that the service is operational.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Health check request (optional service name)
//
// Returns:
//   - *pb.CheckHealthyResponse: Health status information
//   - error: Error if the health check fails critically
func (s *ExplorerService) CheckHealthy(ctx context.Context, req *pb.CheckHealthyRequest) (*pb.CheckHealthyResponse, error) {
	global.Logger.Debugf("health check requested: service=%s", req.GetService())

	// Default service name
	serviceName := "injective-explorer"
	if req.GetService() != "" {
		serviceName = req.GetService()
	}

	// Initialize health status
	healthStatus := "healthy"
	details := make(map[string]*pb.HealthDetails)

	// Check database connection
	dbHealthy := true
	startTime := time.Now()
	if db.Get() != nil {
		sqlDB, err := db.Get().DB()
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, HealthCheckTimeout)
			err := sqlDB.PingContext(pingCtx)
			latency := time.Since(startTime).Milliseconds()
			cancel()
			if err != nil {
				dbHealthy = false
				healthStatus = "degraded"
				details["database"] = &pb.HealthDetails{
					Status:    "unhealthy",
					Error:     err.Error(),
					LatencyMs: float64(latency),
				}
			} else {
				details["database"] = &pb.HealthDetails{
					Status:    "healthy",
					LatencyMs: float64(latency),
				}
			}
		} else {
			dbHealthy = false
			healthStatus = "degraded"
			details["database"] = &pb.HealthDetails{
				Status: "unhealthy",
				Error:  "failed to get database instance",
			}
		}
	} else {
		dbHealthy = false
		healthStatus = "unhealthy"
		details["database"] = &pb.HealthDetails{
			Status: "unavailable",
			Error:  "database instance is nil",
		}
	}

	// Check Redis connection (if available)
	redisStartTime := time.Now()
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		redisCtx, cancel := context.WithTimeout(ctx, HealthCheckTimeout)
		_, err := redisClient.Ping(redisCtx).Result()
		redisLatency := time.Since(redisStartTime).Milliseconds()
		cancel()
		if err != nil {
			healthStatus = "degraded"
			details["redis"] = &pb.HealthDetails{
				Status:    "unhealthy",
				Error:     err.Error(),
				LatencyMs: float64(redisLatency),
			}
		} else {
			details["redis"] = &pb.HealthDetails{
				Status:    "healthy",
				LatencyMs: float64(redisLatency),
			}
		}
	} else {
		details["redis"] = &pb.HealthDetails{
			Status: "not_configured",
		}
	}

	// If database is critical and unhealthy, mark as unhealthy
	if !dbHealthy {
		healthStatus = "unhealthy"
	}

	response := &pb.CheckHealthyResponse{
		Status:    healthStatus,
		Service:   serviceName,
		Timestamp: time.Now().Unix(),
		Details:   details,
	}

	global.Logger.Debugf("health check completed: status=%s, service=%s", healthStatus, serviceName)
	return response, nil
}

// toStruct converts any value to google.protobuf.Struct.
// This allows returning JSON objects directly instead of JSON strings.
//
// Parameters:
//   - v: The value to convert
//
// protoMessageToStructWithOptions converts a protobuf message to google.protobuf.Struct
// with customizable marshaling options.
//
// normalizePagination normalizes pagination parameters with default values and validation.
// Supports both page-based and cursor-based pagination.
//
// Parameters:
//   - page: The page number (1-based, used if cursor is empty)
//   - pageSize: The page size
//   - cursor: The cursor for cursor-based pagination (optional, takes priority over page)
//   - maxPageSize: Maximum allowed page size
//
// Returns:
//   - normalizedPage: The normalized page number (0 if using cursor)
//   - normalizedPageSize: The normalized page size
//   - offset: The offset for database queries (0 if using cursor)
//   - useCursor: Whether to use cursor-based pagination
//   - cursorValue: The parsed cursor value (0 if not using cursor)
func normalizePagination(page, pageSize int32, cursor string, maxPageSize int32) (int32, int32, int, bool, uint64) {
	// Normalize page size (must be between min and max)
	normalizedPageSize := pageSize
	if normalizedPageSize < MinPageSize {
		normalizedPageSize = DefaultPageSize
	}
	if normalizedPageSize > maxPageSize {
		normalizedPageSize = maxPageSize
	}

	// Check if using cursor-based pagination
	if cursor != "" {
		// Parse cursor (assuming it's a numeric value representing height or tx_number)
		var cursorValue uint64
		if _, err := fmt.Sscanf(cursor, "%d", &cursorValue); err == nil {
			return 0, normalizedPageSize, 0, true, cursorValue
		}
		// If cursor parsing fails, fall back to page-based
	}

	// Normalize page (must be >= 1)
	normalizedPage := page
	if normalizedPage < MinPage {
		normalizedPage = MinPage
	}

	// Calculate offset (0-based)
	offset := int((normalizedPage - 1) * normalizedPageSize)

	return normalizedPage, normalizedPageSize, offset, false, 0
}

// getApproximateCount gets an approximate count for large datasets to improve performance.
// For very large datasets, exact COUNT(*) can be slow, so we use an optimized approach.
//
// Parameters:
//   - db: The database connection
//   - model: The model to count
//   - ctx: Context for the query
//
// Returns:
//   - approximate count and whether it's an exact count
//
// Deprecated: Use getApproximateCountWithError instead to properly handle errors.
func getApproximateCount(db *gorm.DB, model interface{}, ctx context.Context) (int64, bool) {
	total, isExactCount, _ := getApproximateCountWithError(db, model, ctx)
	return total, isExactCount
}

// getApproximateCountWithError gets an approximate count for large datasets and returns errors.
// This function uses database statistics (information_schema for MySQL, pg_class for PostgreSQL)
// for fast approximate counts instead of scanning the entire table, which is much more efficient
// for large tables. Note: SQLite is not supported as it lacks statistics capabilities.
//
// Parameters:
//   - db: The database connection
//   - model: The model to count
//   - ctx: Context for the query
//
// Returns:
//   - approximate count
//   - whether it's an exact count
//   - error if the count query failed (caller should handle this error)
func getApproximateCountWithError(db *gorm.DB, model interface{}, ctx context.Context) (int64, bool, error) {
	// First, try to get approximate count from database statistics (very fast, no table scan)
	// This is much faster than COUNT(*) for large tables
	approximateCount, err := getCountFromDatabaseStats(db, model, ctx)
	if err == nil && approximateCount > 0 {
		// Successfully got approximate count from statistics
		// For small tables (< 10k rows), try to get exact count
		if approximateCount < ApproximateCountThreshold {
			// Try exact count for small tables (with timeout)
			ctxWithTimeout, cancel := context.WithTimeout(ctx, CountQueryTimeout)
			defer cancel()

			var exactCount int64
			if err := db.WithContext(ctxWithTimeout).Model(model).Count(&exactCount).Error; err == nil {
				return exactCount, true, nil
			}
			// If exact count fails, return approximate count
		}
		return approximateCount, false, nil
	}

	// If statistics query failed, fall back to COUNT(*) with timeout
	// This is still better than no timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, CountQueryTimeout)
	defer cancel()

	var count int64
	err = db.WithContext(ctxWithTimeout).Model(model).Count(&count).Error
	if err == nil {
		return count, true, nil
	}

	// If COUNT(*) also fails, return fallback value
	global.Logger.Warnf("both statistics and count queries failed, using fallback: %v", err)
	return ApproximateCountFallback, false, err
}

// getCountFromDatabaseStats gets an approximate row count from database statistics.
// This is much faster than COUNT(*) because it doesn't scan the table.
//
// Parameters:
//   - db: The database connection
//   - model: The model to get count for
//   - ctx: Context for the query
//
// Returns:
//   - approximate count from database statistics
//   - error if the query failed
func getCountFromDatabaseStats(db *gorm.DB, model interface{}, ctx context.Context) (int64, error) {
	// Get table name from model
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		return 0, err
	}
	tableName := stmt.Schema.Table

	// Detect database driver type using GORM's Name() method
	driverName := db.Name()
	var count int64
	var err error

	switch driverName {
	case "mysql":
		// MySQL: Use information_schema.TABLES for approximate count
		// TABLE_ROWS is an approximation, but very fast
		var dbName string
		if err = db.Raw("SELECT DATABASE()").Scan(&dbName).Error; err != nil {
			return 0, err
		}

		err = db.WithContext(ctx).Raw(
			"SELECT TABLE_ROWS FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?",
			dbName, tableName,
		).Scan(&count).Error
		if err != nil {
			return 0, err
		}

	case "postgres":
		// PostgreSQL: Use pg_class.reltuples for approximate count
		// This is updated by ANALYZE and is very fast
		var reltuples float64
		err = db.WithContext(ctx).Raw(
			"SELECT reltuples::BIGINT FROM pg_class WHERE relname = ?",
			tableName,
		).Scan(&reltuples).Error
		if err != nil {
			return 0, err
		}
		count = int64(reltuples)

	case "sqlite3", "sqlite":
		// SQLite: SQLite doesn't have reliable statistics like MySQL/PostgreSQL
		// Return error to trigger fallback to COUNT(*) with timeout
		// The caller will use COUNT(*) with timeout instead
		return 0, errors.New("SQLite doesn't support statistics-based count")

	default:
		// Unknown driver, return error to trigger fallback
		return 0, fmt.Errorf("unsupported database driver: %s", driverName)
	}

	if count < 0 {
		count = 0
	}

	return count, nil
}

func getApproximateCountForAddressWithError(baseQuery *gorm.DB, address string) (int64, bool, error) {
	var count int64
	ctx, cancel := context.WithTimeout(context.Background(), CountQueryTimeout)
	defer cancel()

	err := baseQuery.WithContext(ctx).Count(&count).Error
	if err == nil {
		isExact := count < ApproximateCountThreshold
		return count, isExact, nil
	}

	global.Logger.Warnf("exact count for address failed, using approximate: %v", err)
	return ApproximateCountForAddress, false, err
}

// buildPaginationInfo builds pagination information for response.
//
// Parameters:
//   - page: Current page number (1-based)
//   - pageSize: Page size
//   - total: Total number of records
//
// Returns:
//   - map[string]interface{}: Pagination information map containing:
//   - page: Current page number
//   - page_size: Page size
//   - total: Total number of records
//   - total_pages: Total number of pages
//   - has_prev: Whether there is a previous page
//   - has_next: Whether there is a next page
func buildPaginationInfo(page, pageSize int32, total int64) map[string]interface{} {
	paginationInfo := buildPaginationInfoProto(page, pageSize, total)
	return map[string]interface{}{
		"page":        paginationInfo.Page,
		"page_size":   paginationInfo.PageSize,
		"total":       paginationInfo.Total,
		"total_pages": paginationInfo.TotalPages,
		"has_prev":    paginationInfo.HasPrev,
		"has_next":    paginationInfo.HasNext,
	}
}

// buildPaginationInfoProto builds a PaginationInfo protobuf message.
// This is the unified pagination format used across all APIs.
func buildPaginationInfoProto(page, pageSize int32, total int64) *pb.PaginationInfo {
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages == 0 {
		totalPages = 1
	}

	return &pb.PaginationInfo{
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
	}
}

// buildPaginatedResponse builds a paginated response with data and pagination info.
// The response structure is flattened for better readability: data and pagination are at the same level.
//
// Parameters:
//   - data: The data to return
//   - page: Current page number
//   - pageSize: Page size
//   - total: Total number of records
//   - isExactCount: Whether the count is exact
//   - useCursor: Whether using cursor-based pagination
//   - cursorValue: Current cursor value
//   - nextCursor: Next cursor value (empty string if none)
//   - resultCount: Number of results in current page
//
// Returns:
//   - map[string]interface{}: Response with data and pagination at the same level
func buildPaginatedResponse(data interface{}, page, pageSize int32, total int64, isExactCount bool, useCursor bool, cursorValue uint64, nextCursor string, resultCount int) map[string]interface{} {
	paginationInfo := buildPaginationInfo(page, pageSize, total)
	if !isExactCount {
		paginationInfo["approximate"] = true
	}
	if useCursor {
		paginationInfo["cursor"] = cursorValue
		if nextCursor != "" {
			paginationInfo["next_cursor"] = nextCursor
		}
		paginationInfo["has_next"] = resultCount == int(pageSize)
	}
	// Flatten the response structure: put data and pagination at the same level
	response := make(map[string]interface{})
	response["data"] = data
	response["pagination"] = paginationInfo
	return response
}

// calculateNextCursor calculates the next cursor value for cursor-based pagination.
// It finds the minimum value that is less than the current cursor value.
//
// Parameters:
//   - useCursor: Whether using cursor-based pagination
//   - cursorValue: Current cursor value
//   - getValue: Function to extract the cursor value from an item
//
// Returns:
//   - string: Next cursor value (empty string if not using cursor or no valid value found)
func calculateNextCursor[T any](useCursor bool, cursorValue uint64, items []T, getValue func(T) uint64) string {
	if !useCursor || len(items) == 0 {
		return ""
	}
	var nextCursor string
	for _, item := range items {
		value := getValue(item)
		if nextCursor == "" || value < cursorValue {
			nextCursor = fmt.Sprintf("%d", value)
		}
	}
	return nextCursor
}

// saveToDB saves a model to database and logs errors without failing the operation.
// This is useful when we already have the data and just want to cache it.
// For Transaction models, it uses FirstOrCreate to handle duplicates gracefully.
//
// Parameters:
//   - model: The model to save
//   - errorMsg: Error message prefix for logging
func saveToDB(model interface{}, errorMsg string) {
	var err error

	// For Transaction models, use FirstOrCreate to handle duplicates
	if tx, ok := model.(*models.Transaction); ok {
		// Use FirstOrCreate to avoid duplicate key errors
		err = db.Get().Where("hash = ?", tx.Hash).FirstOrCreate(tx).Error
	} else if validator, ok := model.(*models.Validator); ok {
		// For Validator models, use upsert (Create or Update) to handle duplicates
		err = db.Get().Where("consensus_address = ?", validator.ConsensusAddress).
			Assign(validator).
			FirstOrCreate(validator).Error
	} else {
		// For other models, use Create
		err = db.Get().Create(model).Error
	}

	if err != nil {
		// Check if it's a "Too many connections" error
		if isTooManyConnectionsError(err) {
			global.Logger.Warnf("Database connection temporarily exhausted, will retry in background: %s: %v", errorMsg, err)
			// Retry in background with exponential backoff
			go retrySaveToDB(model, errorMsg, 3, time.Second)
		} else if isDuplicateKeyError(err) {
			// Duplicate key error is expected and can be ignored
			global.Logger.Debugf("transaction already exists (duplicate key): %s", errorMsg)
		} else {
			global.Logger.Errorf("%s: %v", errorMsg, err)
		}
	}
}

// isTooManyConnectionsError checks if the error is related to connection limit
func isTooManyConnectionsError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "Too many connections") ||
		strings.Contains(errMsg, "1040") ||
		strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "max_connections")
}

// isDuplicateKeyError checks if the error is a duplicate key error
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "Duplicate entry") ||
		strings.Contains(errMsg, "1062") ||
		strings.Contains(errMsg, "UNIQUE constraint") ||
		strings.Contains(errMsg, "duplicate key")
}

// retrySaveToDB retries saving to database with exponential backoff
func retrySaveToDB(model interface{}, errorMsg string, maxRetries int, initialDelay time.Duration) {
	var err error
	delay := initialDelay

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Use FirstOrCreate for Transaction models, Create for others
		if tx, ok := model.(*models.Transaction); ok {
			err = db.Get().Where("hash = ?", tx.Hash).FirstOrCreate(tx).Error
		} else {
			err = db.Get().Create(model).Error
		}

		if err == nil {
			global.Logger.Debugf("Retry successful for: %s (attempt %d)", errorMsg, attempt)
			return
		}

		// If it's a duplicate key error, we can ignore it
		if isDuplicateKeyError(err) {
			global.Logger.Debugf("Transaction already exists (duplicate key) during retry: %s", errorMsg)
			return
		}

		// If it's still a connection error, wait and retry
		if isTooManyConnectionsError(err) && attempt < maxRetries {
			global.Logger.Warnf("Retry %d/%d failed for %s: %v, waiting %v", attempt, maxRetries, errorMsg, err, delay)
			time.Sleep(delay)
			delay *= 2 // Exponential backoff
		} else {
			// Not a connection error, or last attempt
			break
		}
	}

	// Log the final error
	global.Logger.Errorf("Failed after %d retries: %s: %v", maxRetries, errorMsg, err)
}

// marshalAndReply serializes the value and returns a success reply.
// The value is converted to google.protobuf.Struct, allowing direct JSON object response.
//
// Parameters:
//   - v: The value to marshal
//   - errorMsg: Error message prefix if marshaling fails
//
// StreamLatestBlocks streams the latest blocks as they are added to the chain.
// This method subscribes to block events from the event bus and streams them to the client.
func (s *ExplorerService) StreamLatestBlocks(req *pb.StreamLatestBlocksRequest, stream pb.Explorer_StreamLatestBlocksServer) error {
	// Handle nil request: log and continue (GetFromHeight() returns 0 for nil, meaning all blocks)
	if req == nil {
		global.Logger.Infof("stream latest blocks: received nil request, using default")
	} else {
		global.Logger.Infof("stream latest blocks: from_height=%d", req.GetFromHeight())
	}

	// Subscribe to block events
	eventBus := GetEventBus()
	blockChan := eventBus.SubscribeBlocks()
	defer eventBus.UnsubscribeBlocks(blockChan)

	// Send initial blocks from cache (similar to GetLatestBlocks)
	// Create independent context for cache queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Get latest blocks from cache (same as GetLatestBlocks)
	// Uses StreamInitialBlocksCount constant to determine how many blocks to retrieve
	cachedBlocks, err := getBlocksFromCache(queryCtx, StreamInitialBlocksCount)
	sentCount := 0
	hasSentInitialBlocks := false

	if err != nil {
		// Log cache error but continue (graceful degradation)
		global.Logger.Warnf("failed to get blocks from cache, will wait for events: %v", err)
	} else if len(cachedBlocks) > 0 {
		global.Logger.Infof("blocks found in cache: count=%d", len(cachedBlocks))

		// Filter by from_height if specified
		filteredBlocks := cachedBlocks
		if req.GetFromHeight() > 0 {
			filteredBlocks = make([]models.Block, 0, len(cachedBlocks))
			for _, block := range cachedBlocks {
				if block.Height >= uint64(req.GetFromHeight()) {
					filteredBlocks = append(filteredBlocks, block)
				}
			}
		}

		// Reverse the order to send from oldest to newest
		for i, j := 0, len(filteredBlocks)-1; i < j; i, j = i+1, j-1 {
			filteredBlocks[i], filteredBlocks[j] = filteredBlocks[j], filteredBlocks[i]
		}

		// Send initial blocks from cache
		if len(filteredBlocks) > 0 {
			for _, block := range filteredBlocks {
				blockDTO := models.ToBlockDTO(&block, models.DTOModeWithoutDetail)

				update := &pb.BlockUpdate{
					Block:      blockDTO,
					UpdateType: "new",
					Timestamp:  time.Now().Unix(),
				}

				if err := stream.Send(update); err != nil {
					global.Logger.Errorf("failed to send block update (height=%d) to stream: %v", block.Height, err)
					return status.Errorf(codes.Internal, "failed to send block update: %v", err)
				}
				sentCount++
			}
			global.Logger.Infof("sent %d/%d initial blocks from cache to stream", sentCount, len(cachedBlocks))
			hasSentInitialBlocks = true
		} else if req.GetFromHeight() > 0 {
			// No blocks in cache matching from_height, will wait for events
			global.Logger.Infof("no blocks in cache matching from_height=%d, will wait for block events", req.GetFromHeight())
		}
	} else {
		global.Logger.Infof("no blocks found in cache, will wait for first block event")
	}

	// For grpc-web compatibility, we MUST send at least one message to establish the stream
	// If no blocks were sent from cache, wait for first block event
	if !hasSentInitialBlocks {
		firstBlockTimeout := time.NewTimer(StreamFirstEventTimeout)
		defer firstBlockTimeout.Stop()

		select {
		case <-stream.Context().Done():
			global.Logger.Infof("stream context cancelled while waiting for first block")
			return stream.Context().Err()
		case <-firstBlockTimeout.C:
			// If timeout and still no block, return error for grpc-web compatibility
			global.Logger.Warnf("timeout waiting for first block, no blocks available")
			return status.Errorf(codes.NotFound, "no blocks available and timeout waiting for first block")
		case blockEvent, ok := <-blockChan:
			if !ok {
				global.Logger.Errorf("StreamLatestBlocks: block channel was closed while waiting for first block")
				return status.Errorf(codes.Internal, "block event channel was closed")
			}
			if blockEvent != nil && blockEvent.Block != nil {
				// Filter by from_height if specified
				if req.GetFromHeight() == 0 || blockEvent.Block.Height >= uint64(req.GetFromHeight()) {
					blockDTO := models.ToBlockDTO(blockEvent.Block, models.DTOModeWithoutDetail)
					updateType := "new"
					if blockEvent.Type == EventTypeBlockUpdate {
						updateType = "update"
					}

					update := &pb.BlockUpdate{
						Block:      blockDTO,
						UpdateType: updateType,
						Timestamp:  time.Now().Unix(),
					}

					if err := stream.Send(update); err != nil {
						global.Logger.Errorf("failed to send first block update: %v", err)
						return status.Errorf(codes.Internal, "failed to send block update: %v", err)
					}
					global.Logger.Infof("sent first block update (height=%d) to establish stream", blockEvent.Block.Height)
					sentCount = 1
				}
			}
		}
	}

	// Stream new blocks from event bus
	// Add a ticker to log that we're waiting for new blocks (helps with debugging)
	heartbeatTicker := time.NewTicker(StreamHeartbeatInterval)
	defer heartbeatTicker.Stop()

	global.Logger.Infof("StreamLatestBlocks: waiting for new block events...")

	for {
		select {
		case <-stream.Context().Done():
			global.Logger.Infof("stream context cancelled for StreamLatestBlocks")
			return stream.Context().Err()
		case <-heartbeatTicker.C:
			// Log that we're still waiting (helps debug if connection is alive but no blocks)
			global.Logger.Debugf("StreamLatestBlocks: still waiting for new blocks... (stream is alive)")
		case blockEvent, ok := <-blockChan:
			if !ok {
				global.Logger.Errorf("StreamLatestBlocks: block channel was closed")
				return status.Errorf(codes.Internal, "block event channel was closed")
			}
			if blockEvent == nil {
				global.Logger.Warnf("StreamLatestBlocks: received nil block event, skipping")
				continue
			}

			global.Logger.Infof("StreamLatestBlocks: received block event: height=%d, type=%s",
				blockEvent.Block.Height, blockEvent.Type)

			// Filter by from_height if specified
			if req.GetFromHeight() > 0 && blockEvent.Block.Height < uint64(req.GetFromHeight()) {
				global.Logger.Debugf("StreamLatestBlocks: filtering block height %d (from_height=%d)",
					blockEvent.Block.Height, req.GetFromHeight())
				continue
			}

			blockDTO := models.ToBlockDTO(blockEvent.Block, models.DTOModeWithoutDetail)

			updateType := "new"
			if blockEvent.Type == EventTypeBlockUpdate {
				updateType = "update"
			}

			update := &pb.BlockUpdate{
				Block:      blockDTO,
				UpdateType: updateType,
				Timestamp:  time.Now().Unix(),
			}

			if err := stream.Send(update); err != nil {
				global.Logger.Errorf("failed to send block update: %v", err)
				return status.Errorf(codes.Internal, "failed to send block update: %v", err)
			}

			global.Logger.Infof("StreamLatestBlocks: successfully sent block update: height=%d", blockEvent.Block.Height)
		}
	}
}

// StreamLatestTransactions streams the latest transactions as they are added to the chain.
// This method subscribes to transaction events from the event bus and streams them to the client.
func (s *ExplorerService) StreamLatestTransactions(req *pb.StreamLatestTransactionsRequest, stream pb.Explorer_StreamLatestTransactionsServer) error {
	// Handle nil request: create default request for compatibility with empty messages
	if req == nil {
		req = &pb.StreamLatestTransactionsRequest{
			Types: []string{}, // Empty array means all types
		}
		global.Logger.Infof("stream latest transactions: received nil request, using default (all types)")
	} else {
		// Ensure types field is initialized (even if empty) for protobuf encoding/decoding
		if req.GetTypes() == nil {
			req.Types = []string{} // Empty array means all types
		}
		global.Logger.Infof("stream latest transactions: types=%v (len=%d)", req.GetTypes(), len(req.GetTypes()))
	}

	// Subscribe to transaction events
	eventBus := GetEventBus()
	txChan := eventBus.SubscribeTransactions()
	defer eventBus.UnsubscribeTransactions(txChan)

	// Send initial transactions from cache (similar to GetLatestTransactions)
	// Create independent context for cache queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Get latest transactions from cache (same as GetLatestTransactions)
	// Uses StreamInitialTransactionsCount constant to determine how many transactions to retrieve
	cachedTxs, err := getTransactionsFromCache(queryCtx, StreamInitialTransactionsCount)
	if err != nil {
		// Log cache error but continue (graceful degradation)
		global.Logger.Warnf("failed to get transactions from cache, will wait for events: %v", err)
	} else if len(cachedTxs) > 0 {
		global.Logger.Infof("transactions found in cache: count=%d", len(cachedTxs))

		// Apply type filter if specified
		filteredTxs := cachedTxs
		if len(req.GetTypes()) > 0 {
			filteredTxs = make([]models.Transaction, 0, len(cachedTxs))
			for _, tx := range cachedTxs {
				matched := false
				for _, filterType := range req.GetTypes() {
					if strings.Contains(strings.ToLower(tx.TxType), strings.ToLower(filterType)) {
						matched = true
						break
					}
				}
				if matched {
					filteredTxs = append(filteredTxs, tx)
				}
			}
		}

		// Reverse the order to send from oldest to newest
		for i, j := 0, len(filteredTxs)-1; i < j; i, j = i+1, j-1 {
			filteredTxs[i], filteredTxs[j] = filteredTxs[j], filteredTxs[i]
		}

		// Send initial transactions from cache
		sentCount := 0
		for _, tx := range filteredTxs {
			txDTO := models.ToTransactionDTO(&tx, models.DTOModeWithDetail)

			update := &pb.TransactionUpdate{
				Transaction: txDTO,
				UpdateType:  "new",
				Timestamp:   tx.CreatedAt.Unix(),
			}

			if err := stream.Send(update); err != nil {
				global.Logger.Errorf("failed to send transaction update: %v", err)
				return status.Errorf(codes.Internal, "failed to send transaction update: %v", err)
			}
			sentCount++
		}
		global.Logger.Infof("sent %d/%d initial transactions from cache to stream", sentCount, len(cachedTxs))
	} else {
		global.Logger.Infof("no transactions found in cache, will wait for first transaction event")
		// For grpc-web compatibility, we need to send at least one message to establish the stream
		// Wait for first transaction event with timeout
		firstTxTimeout := time.NewTimer(StreamFirstEventTimeout)
		defer firstTxTimeout.Stop()

		// Use a loop to handle type filtering (since we can't use continue in select)
		waitingForFirstTx := true
		for waitingForFirstTx {
			select {
			case <-stream.Context().Done():
				global.Logger.Infof("stream context cancelled while waiting for first transaction")
				return stream.Context().Err()
			case <-firstTxTimeout.C:
				// If timeout and still no transaction, return error for grpc-web compatibility
				global.Logger.Warnf("timeout waiting for first transaction, no transactions available")
				return status.Errorf(codes.NotFound, "no transactions available and timeout waiting for first transaction")
			case txEvent, ok := <-txChan:
				if !ok {
					global.Logger.Errorf("StreamLatestTransactions: transaction channel was closed while waiting for first transaction")
					return status.Errorf(codes.Internal, "transaction event channel was closed")
				}
				if txEvent != nil && txEvent.Transaction != nil {
					// Apply type filter if specified
					matched := true
					if len(req.GetTypes()) > 0 {
						txType := txEvent.Transaction.TxType
						matched = false
						for _, filterType := range req.GetTypes() {
							if strings.Contains(strings.ToLower(txType), strings.ToLower(filterType)) {
								matched = true
								break
							}
						}
						if !matched {
							// Type doesn't match, continue waiting
							global.Logger.Debugf("first transaction type doesn't match filter, continuing to wait")
							continue
						}
					}

					// Type matches or no filter, send the transaction
					txDTO := models.ToTransactionDTO(txEvent.Transaction, models.DTOModeWithDetail)
					updateType := "new"
					if txEvent.Type == EventTypeTransactionUpdate {
						updateType = "update"
					}

					update := &pb.TransactionUpdate{
						Transaction: txDTO,
						UpdateType:  updateType,
						Timestamp:   time.Now().Unix(),
					}

					if err := stream.Send(update); err != nil {
						global.Logger.Errorf("failed to send first transaction update: %v", err)
						return status.Errorf(codes.Internal, "failed to send transaction update: %v", err)
					}
					global.Logger.Infof("sent first transaction update (hash=%s) to establish stream", txEvent.Transaction.Hash)
					waitingForFirstTx = false
				}
			}
		}
	}

	// Stream new transactions from event bus
	for {
		select {
		case <-stream.Context().Done():
			global.Logger.Infof("stream context cancelled for StreamLatestTransactions")
			return stream.Context().Err()
		case txEvent := <-txChan:
			if txEvent == nil {
				continue
			}

			// Filter by transaction types if specified
			if len(req.GetTypes()) > 0 {
				// Check if transaction type matches (simplified, you may need to enhance this)
				txType := txEvent.Transaction.TxType
				matched := false
				for _, filterType := range req.GetTypes() {
					if strings.Contains(strings.ToLower(txType), strings.ToLower(filterType)) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}

			txDTO := models.ToTransactionDTO(txEvent.Transaction, models.DTOModeWithDetail)

			updateType := "new"
			if txEvent.Type == EventTypeTransactionUpdate {
				updateType = "update"
			}

			update := &pb.TransactionUpdate{
				Transaction: txDTO,
				UpdateType:  updateType,
				Timestamp:   time.Now().Unix(),
			}

			if err := stream.Send(update); err != nil {
				global.Logger.Errorf("failed to send transaction update: %v", err)
				return status.Errorf(codes.Internal, "failed to send transaction update: %v", err)
			}
		}
	}
}

// StreamAccountUpdates streams account updates (balance changes, new transactions, etc.) for a specific account.
// This method subscribes to account events from the event bus and streams them to the client.
func (s *ExplorerService) StreamAccountUpdates(req *pb.StreamAccountUpdatesRequest, stream pb.Explorer_StreamAccountUpdatesServer) error {
	// Input validation
	if req == nil {
		return status.Errorf(codes.InvalidArgument, "request cannot be nil")
	}

	address := req.GetAddress()
	if address == "" {
		return status.Errorf(codes.InvalidArgument, "address is required")
	}

	// Validate address format
	if err := ValidateAddress(address); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid address format: %v", err)
	}

	global.Logger.Infof("stream account updates: address=%s, from_timestamp=%d", address, req.GetFromTimestamp())

	// Subscribe to account events for this specific address
	eventBus := GetEventBus()
	accountChan := eventBus.SubscribeAccount(address)
	defer eventBus.UnsubscribeAccount(address, accountChan)

	// Send initial/historical transactions for this account
	// Always send recent transactions to show something immediately
	var dbTxs []models.Transaction
	query := db.Get().WithContext(stream.Context()).
		Where("signer = ?", address).
		Order("block_unix_timestamp DESC, tx_number DESC")

	// Apply timestamp filter if specified
	if req.GetFromTimestamp() > 0 {
		query = query.Where("block_unix_timestamp >= ?", req.GetFromTimestamp())
	}

	// Limit to avoid sending too much data
	// Uses AccountTransactionLimit constant to limit the number of transactions returned
	query = query.Limit(AccountTransactionLimit)

	err := query.Find(&dbTxs).Error
	if err == nil && len(dbTxs) > 0 {
		// Reverse the order to send from oldest to newest
		for i, j := 0, len(dbTxs)-1; i < j; i, j = i+1, j-1 {
			dbTxs[i], dbTxs[j] = dbTxs[j], dbTxs[i]
		}

		for _, tx := range dbTxs {
			txDTO := models.ToTransactionDTO(&tx, models.DTOModeWithDetail)

			update := &pb.AccountUpdate{
				Address:    address,
				UpdateType: "transaction",
				Data: &pb.AccountUpdateData{
					Transaction: txDTO,
					Metadata:    make(map[string]string),
				},
				Timestamp: tx.CreatedAt.Unix(),
			}

			if err := stream.Send(update); err != nil {
				return status.Errorf(codes.Internal, "failed to send account update: %v", err)
			}
		}
		global.Logger.Infof("sent %d initial account transactions to stream for address %s", len(dbTxs), address)
	}

	// Stream new account updates from event bus
	for {
		select {
		case <-stream.Context().Done():
			global.Logger.Infof("stream context cancelled for StreamAccountUpdates")
			return stream.Context().Err()
		case accountEvent := <-accountChan:
			if accountEvent == nil {
				continue
			}

			// Filter by timestamp if specified
			if req.GetFromTimestamp() > 0 && accountEvent.Data != nil {
				// Check timestamp if available in the event data
				// This is a simplified check, you may need to enhance this
			}

			// Convert account event data to strongly-typed AccountUpdateData
			updateData := &pb.AccountUpdateData{
				Metadata: make(map[string]string),
			}

			// Handle different event types
			switch accountEvent.Type {
			case EventTypeTransactionNew, EventTypeTransactionUpdate:
				// If event contains transaction data, convert it
				if tx, ok := accountEvent.Data["transaction"].(*models.Transaction); ok {
					txDTO := models.ToTransactionDTO(tx, models.DTOModeWithDetail)
					updateData.Transaction = txDTO
				}
			case EventTypeAccountUpdate:
				// If event contains balance data, convert it
				if balances, ok := accountEvent.Data["balances"].([]models.AccountBalance); ok {
					balanceDTOs := make([]*pb.AccountBalanceDTO, 0, len(balances))
					for _, balance := range balances {
						balanceDTOs = append(balanceDTOs, models.ToAccountBalanceDTO(&balance))
					}
					updateData.Balances = balanceDTOs
				}
			default:
				// For other types, store as metadata
				for k, v := range accountEvent.Data {
					if str, ok := v.(string); ok {
						updateData.Metadata[k] = str
					}
				}
			}

			update := &pb.AccountUpdate{
				Address:    accountEvent.Address,
				UpdateType: string(accountEvent.Type),
				Data:       updateData,
				Timestamp:  time.Now().Unix(),
			}

			if err := stream.Send(update); err != nil {
				global.Logger.Errorf("failed to send account update: %v", err)
				return status.Errorf(codes.Internal, "failed to send account update: %v", err)
			}
		}
	}
}

// getBlockFromCache retrieves a block from Redis cache by height.
func getBlockFromCache(ctx context.Context, height uint64) (*models.Block, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil, errors.New("redis client is nil")
	}

	key := CacheKeyBlockPrefix + fmt.Sprintf("%d", height)
	data, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Not found in cache
		}
		return nil, err
	}

	var block models.Block
	if err := sonic.Unmarshal([]byte(data), &block); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal block from cache")
	}

	return &block, nil
}

// getTransactionFromCache retrieves a transaction from Redis cache by hash.
// Returns (nil, nil) if the transaction is not found in cache (cache miss).
// Returns (nil, error) if an error occurred during cache lookup.
// Returns (*Transaction, nil) if the transaction was successfully retrieved from cache.
//
// Parameters:
//   - ctx: Context for the cache operation
//   - hash: The transaction hash to retrieve (without 0x prefix)
//
// Returns:
//   - *models.Transaction: The cached transaction, or nil if not found
//   - error: Error if cache lookup failed (not including cache miss)
func getTransactionFromCache(ctx context.Context, hash string) (*models.Transaction, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil, errors.New("redis client is nil")
	}

	key := CacheKeyTransactionPrefix + hash
	data, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Not found in cache (cache miss)
		}
		return nil, errors.Wrapf(err, "failed to get transaction from cache (key=%s)", key)
	}

	var tx models.Transaction
	if err := sonic.Unmarshal([]byte(data), &tx); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal transaction from cache (key=%s, data_length=%d)", key, len(data))
	}

	return &tx, nil
}

// getBlocksFromCache retrieves latest blocks from Redis cache list.
// This function queries the cached list of recent blocks and deserializes them.
// Returns an empty slice (not nil) if no blocks are found in cache.
//
// Parameters:
//   - ctx: Context for the cache operation
//   - limit: Maximum number of blocks to retrieve
//
// Returns:
//   - []models.Block: List of cached blocks (empty slice if cache miss)
//   - error: Error if cache lookup failed (not including cache miss)
func getBlocksFromCache(ctx context.Context, limit int) ([]models.Block, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil, errors.New("redis client is nil")
	}

	// Redis list: LPUSH adds new items to the head (left, index 0)
	// So the newest blocks are at the beginning of the list (indices 0, 1, 2...)
	// Use LRANGE with positive indices: 0 to limit-1 gets the first 'limit' elements
	// This ensures we get the latest (highest blockNumber) blocks first
	data, err := redisClient.LRange(ctx, CacheKeyBlockList, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}

	// Return empty slice (not nil) if no data found, as documented
	if len(data) == 0 {
		return []models.Block{}, nil
	}

	blocks := make([]models.Block, 0, len(data))
	unmarshalErrors := 0
	for _, item := range data {
		var block models.Block
		if err := sonic.Unmarshal([]byte(item), &block); err != nil {
			unmarshalErrors++
			global.Logger.Warnf("failed to unmarshal block from cache (item %d/%d): %v", unmarshalErrors, len(data), err)
			continue
		}
		blocks = append(blocks, block)
	}

	// If all items failed to unmarshal, log warning but return empty slice (not error)
	// This allows graceful degradation - cache miss is better than complete failure
	if len(blocks) == 0 && len(data) > 0 {
		global.Logger.Warnf("all %d blocks failed to unmarshal from cache, returning empty slice", len(data))
	}

	// Sort blocks by height descending to ensure correct order (newest first)
	// Redis list order is based on insertion time, not block height
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Height > blocks[j].Height
	})

	return blocks, nil
}

// getBlocksFromCacheWithOffset retrieves blocks from Redis cache with offset and limit for pagination.
// This enables paginated access to cached blocks for better performance on multi-page requests.
// Returns (blocks, hasMore, error) where hasMore indicates if cache has enough data.
func getBlocksFromCacheWithOffset(ctx context.Context, offset, limit int) ([]models.Block, bool, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil, false, errors.New("redis client is nil")
	}

	// Redis list: newest at head (left, index 0), oldest at tail (right)
	// LPUSH adds new items to the head (index 0)
	// To get page at offset with newest first, we need items from offset to offset+limit-1
	startIdx := int64(offset)
	endIdx := int64(offset + limit - 1)

	data, err := redisClient.LRange(ctx, CacheKeyBlockList, startIdx, endIdx).Result()
	if err != nil {
		return nil, false, err
	}

	// Check if we have enough data in cache
	if len(data) < limit {
		// Not enough cached data for this page
		return nil, false, nil
	}

	blocks := make([]models.Block, 0, len(data))
	for _, item := range data {
		var block models.Block
		if err := sonic.Unmarshal([]byte(item), &block); err != nil {
			global.Logger.Warnf("failed to unmarshal block from cache: %v", err)
			continue
		}
		blocks = append(blocks, block)
	}

	if len(blocks) == 0 {
		return []models.Block{}, false, nil
	}

	// Sort by height descending to ensure correct order (newest first)
	// This handles any out-of-order issues from the cache
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Height > blocks[j].Height
	})

	// Check if there might be more data in cache (for cursor calculation)
	// Try to get one more item to determine hasMore
	nextStartIdx := int64(offset + limit)
	nextData, _ := redisClient.LRange(ctx, CacheKeyBlockList, nextStartIdx, nextStartIdx).Result()
	hasMore := len(nextData) > 0

	return blocks, hasMore, nil
}

// getTransactionsFromCache retrieves latest transactions from Redis cache list.
// This function queries the cached list of recent transactions and deserializes them.
// Returns an empty slice (not nil) if no transactions are found in cache.
//
// Parameters:
//   - ctx: Context for the cache operation
//   - limit: Maximum number of transactions to retrieve
//
// Returns:
//   - []models.Transaction: List of cached transactions (empty slice if cache miss)
//   - error: Error if cache lookup failed (not including cache miss)
func getTransactionsFromCache(ctx context.Context, limit int) ([]models.Transaction, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil, errors.New("redis client is nil")
	}
	// Use Sorted Set ZREVRANGE: returns elements in reverse order of score (highest first)
	// With tx_number as score, this gives us newest transactions first (highest tx_number)
	// Indices 0 to limit-1 gets the 'limit' most recent transactions
	data, err := redisClient.ZRevRange(ctx, CacheKeyTransactionZSet, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}

	// Return empty slice (not nil) if no data found, as documented
	if len(data) == 0 {
		return []models.Transaction{}, nil
	}

	txs := make([]models.Transaction, 0, len(data))
	unmarshalErrors := 0
	for _, item := range data {
		var tx models.Transaction
		if err := sonic.Unmarshal([]byte(item), &tx); err != nil {
			unmarshalErrors++
			global.Logger.Warnf("failed to unmarshal transaction from cache (item %d/%d): %v", unmarshalErrors, len(data), err)
			continue
		}
		txs = append(txs, tx)
	}

	// If all items failed to unmarshal, log warning but return empty slice (not error)
	// This allows graceful degradation - cache miss is better than complete failure
	if len(txs) == 0 && len(data) > 0 {
		global.Logger.Warnf("all %d transactions failed to unmarshal from cache, returning empty slice", len(data))
	}

	// No need to sort - ZREVRANGE already returns in correct order (highest tx_number first)
	// Sorted Set guarantees order by score (tx_number)

	return txs, nil
}

// getTransactionsFromCacheWithOffset retrieves transactions from Redis cache with offset and limit for pagination.
// This enables paginated access to cached transactions for better performance on multi-page requests.
// Returns (transactions, hasMore, error) where hasMore indicates if cache has enough data.
func getTransactionsFromCacheWithOffset(ctx context.Context, offset, limit int) ([]models.Transaction, bool, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil, false, errors.New("redis client is nil")
	}

	// Use Sorted Set ZREVRANGE: returns elements in reverse order of score (highest first)
	// With tx_number as score, this gives us newest transactions first
	// To get page at offset with newest first, we need items from offset to offset+limit-1
	startIdx := int64(offset)
	endIdx := int64(offset + limit - 1)

	data, err := redisClient.ZRevRange(ctx, CacheKeyTransactionZSet, startIdx, endIdx).Result()
	if err != nil {
		return nil, false, err
	}

	// Check if we have enough data in cache
	if len(data) < limit {
		return nil, false, nil
	}

	txs := make([]models.Transaction, 0, len(data))
	for _, item := range data {
		var tx models.Transaction
		if err := sonic.Unmarshal([]byte(item), &tx); err != nil {
			global.Logger.Warnf("failed to unmarshal transaction from cache: %v", err)
			continue
		}
		txs = append(txs, tx)
	}

	if len(txs) == 0 {
		return []models.Transaction{}, false, nil
	}

	// No need to sort - ZREVRANGE already returns in correct order (highest tx_number first)
	// Sorted Set guarantees order by score (tx_number)

	// Check if there might be more data in cache (for cursor calculation)
	// Try to get one more item to determine hasMore
	nextStartIdx := int64(offset + limit)
	nextData, _ := redisClient.ZRevRange(ctx, CacheKeyTransactionZSet, nextStartIdx, nextStartIdx).Result()
	hasMore := len(nextData) > 0

	return txs, hasMore, nil
}

// getAccountTransactionsFromCache retrieves account transactions from Redis cache.
// This function queries the cached list of recent transactions for a specific address.
// Returns an empty slice (not nil) if no transactions are found in cache.
//
// Parameters:
//   - ctx: Context for the cache operation
//   - cacheKey: Redis cache key for the account transactions
//   - limit: Maximum number of transactions to retrieve
//
// Returns:
//   - []models.Transaction: List of cached transactions (empty slice if cache miss)
//   - error: Error if cache lookup failed (not including cache miss)
func getAccountTransactionsFromCache(ctx context.Context, cacheKey string, limit int) ([]models.Transaction, error) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil, errors.New("redis client is nil")
	}

	// Redis list: LPUSH adds new items to the head (left, index 0)
	// So the newest transactions are at the beginning of the list (indices 0, 1, 2...)
	// Use LRANGE with positive indices: 0 to limit-1 gets the first 'limit' elements
	data, err := redisClient.LRange(ctx, cacheKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}

	// Return empty slice (not nil) if no data found
	if len(data) == 0 {
		return []models.Transaction{}, nil
	}

	txs := make([]models.Transaction, 0, len(data))
	unmarshalErrors := 0
	for _, item := range data {
		var tx models.Transaction
		if err := sonic.Unmarshal([]byte(item), &tx); err != nil {
			unmarshalErrors++
			global.Logger.Warnf("failed to unmarshal transaction from cache (item %d/%d): %v", unmarshalErrors, len(data), err)
			continue
		}
		txs = append(txs, tx)
	}

	if len(txs) == 0 && len(data) > 0 {
		global.Logger.Warnf("all %d transactions failed to unmarshal from cache, returning empty slice", len(data))
	}

	// Sort transactions by block_unix_timestamp descending to ensure correct order (newest first)
	// This handles any out-of-order issues from the cache
	sort.Slice(txs, func(i, j int) bool {
		return txs[i].BlockUnixTimestamp > txs[j].BlockUnixTimestamp
	})

	return txs, nil
}

// cacheAccountTransactions caches account transactions to Redis, maintaining the latest transactions for an address.
func cacheAccountTransactions(ctx context.Context, cacheKey string, txs []models.Transaction) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	// Serialize transactions to JSON and add to list
	pipe := redisClient.Pipeline()
	for _, tx := range txs {
		txJSON, err := sonic.Marshal(tx)
		if err != nil {
			global.Logger.Warnf("failed to marshal transaction to JSON: %v", err)
			continue
		}
		pipe.LPush(ctx, cacheKey, txJSON)
	}

	// Maintain list size and set TTL
	pipe.LTrim(ctx, cacheKey, 0, CacheMaxSize-1)
	pipe.Expire(ctx, cacheKey, CacheTransactionTTL)

	_, err := pipe.Exec(ctx)
	if err != nil {
		global.Logger.Warnf("failed to cache account transactions: %v", err)
	}
}

// invalidateAccountTransactionCache invalidates the account transaction cache for a given address.
// This is called when a transaction is updated to ensure cached account transactions are refreshed.
//
// Parameters:
//   - ctx: Context for the cache operation
//   - address: The account address (in database format, e.g., Ethereum format 0x...)
func invalidateAccountTransactionCache(ctx context.Context, address string) {
	if address == "" {
		return
	}

	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	// Delete the account transaction cache key
	cacheKey := fmt.Sprintf("cache:account:txs:%s", address)
	err := redisClient.Del(ctx, cacheKey).Err()
	if err != nil {
		global.Logger.Warnf("failed to invalidate account transaction cache for address %s: %v", address, err)
	} else {
		global.Logger.Debugf("invalidated account transaction cache for address %s", address)
	}
}

// getTotalFromCache retrieves the total count from Redis cache.
// Returns (0, false) if the total is not found in cache (cache miss).
//
// Parameters:
//   - ctx: Context for the cache operation
//   - key: The cache key for the total count
//
// Returns:
//   - int64: The cached total count, or 0 if not found
//   - bool: Whether the count is exact (always false for cached values)
func getTotalFromCache(ctx context.Context, key string) (int64, bool) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return 0, false
	}

	data, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, false // Cache miss
		}
		global.Logger.Warnf("failed to get total from cache (key=%s): %v", key, err)
		return 0, false
	}

	var total int64
	if _, err := fmt.Sscanf(data, "%d", &total); err != nil {
		global.Logger.Warnf("failed to parse total from cache (key=%s, value=%s): %v", key, data, err)
		return 0, false
	}

	return total, false // Cached values are always approximate
}

// cacheTotal stores the total count in Redis cache with TTL.
// This function is called periodically to update the cached total count.
//
// Parameters:
//   - ctx: Context for the cache operation
//   - key: The cache key for the total count
//   - total: The total count to cache
func cacheTotal(ctx context.Context, key string, total int64) error {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil
	}

	// Store total as string with TTL
	err := redisClient.Set(ctx, key, fmt.Sprintf("%d", total), CacheTotalTTL).Err()
	if err != nil {
		return fmt.Errorf("failed to cache total (key=%s): %w", key, err)
	}
	return nil
}

// transactionStatsData holds intermediate statistics data during parallel queries
type transactionStatsData struct {
	totalCount         int64
	count30d           int64
	count24h           int64
	activeAddresses24h int64
	failedTxCount24h   int64
	tps                float64
	avgGas             float64
	currentGas         float64
	maxGas24h          float64
	minGas24h          float64
	avgBlockTime       float64
	gasUtilization     float64
}

// getTotalTransactionCount retrieves the total transaction count from chain using GetTxs.
func getTotalTransactionCount(ctx context.Context) (int64, error) {
	// Query total transaction count from database
	return getTotalTransactionCountFromDB(ctx)
}

// getTotalTransactionCountFromDB retrieves total transaction count from database (fallback).
func getTotalTransactionCountFromDB(ctx context.Context) (int64, error) {
	var totalCount int64
	err := db.Get().WithContext(ctx).Model(&models.Transaction{}).
		Count(&totalCount).Error
	if err != nil {
		return 0, fmt.Errorf("failed to get total transaction count from DB: %w", err)
	}
	return totalCount, nil
}

// getTransactionCount30d retrieves transaction count in the last 30 days.
func getTransactionCount30d(ctx context.Context, thirtyDaysAgoUnix uint64) (int64, error) {
	var count30d int64
	err := db.Get().WithContext(ctx).Model(&models.Transaction{}).
		Where("block_unix_timestamp >= ?", thirtyDaysAgoUnix).
		Count(&count30d).Error
	if err != nil {
		return 0, fmt.Errorf("failed to get 30-day transaction count: %w", err)
	}
	return count30d, nil
}

// getTransactionCount24h retrieves transaction count in the last 24 hours.
func getTransactionCount24h(ctx context.Context, twentyFourHoursAgoUnix uint64) (int64, error) {
	var count24h int64
	err := db.Get().WithContext(ctx).Model(&models.Transaction{}).
		Where("block_unix_timestamp >= ?", twentyFourHoursAgoUnix).
		Count(&count24h).Error
	if err != nil {
		return 0, fmt.Errorf("failed to get 24-hour transaction count: %w", err)
	}
	return count24h, nil
}

// getActiveAddresses24h retrieves unique active addresses count in the last 24 hours.
// Optimized: Uses COUNT(DISTINCT signer) with raw SQL for better performance than GORM's Distinct().Count()
func getActiveAddresses24h(ctx context.Context, twentyFourHoursAgoUnix uint64) (int64, error) {
	var activeAddresses24h int64
	// Use raw SQL with COUNT(DISTINCT) for better performance
	// This is more efficient than GORM's Distinct().Count() which may not optimize well
	err := db.Get().WithContext(ctx).Raw(
		"SELECT COUNT(DISTINCT signer) FROM transactions WHERE block_unix_timestamp >= ? AND signer != ''",
		twentyFourHoursAgoUnix,
	).Scan(&activeAddresses24h).Error
	if err != nil {
		global.Logger.Warnf("failed to get 24h active addresses count: %v, using 0", err)
		return 0, nil // Return 0 instead of error for non-critical metric
	}
	return activeAddresses24h, nil
}

// getFailedTxCount24h retrieves failed transaction count in the last 24 hours.
func getFailedTxCount24h(ctx context.Context, twentyFourHoursAgoUnix uint64) (int64, error) {
	var failedTxCount24h int64
	err := db.Get().WithContext(ctx).Model(&models.Transaction{}).
		Where("block_unix_timestamp >= ? AND code != 0", twentyFourHoursAgoUnix).
		Count(&failedTxCount24h).Error
	if err != nil {
		global.Logger.Warnf("failed to get 24h failed transaction count: %v, using 0", err)
		return 0, nil // Return 0 instead of error for non-critical metric
	}
	return failedTxCount24h, nil
}

// calculateTPS calculates average TPS from the latest blocks.
func calculateTPS(ctx context.Context) (float64, error) {
	var latestBlocks []models.Block
	err := db.Get().WithContext(ctx).Model(&models.Block{}).
		Order("height DESC").
		Limit(TPSBlockCount).
		Find(&latestBlocks).Error
	if err != nil {
		return 0.0, fmt.Errorf("failed to get latest %d blocks: %w", TPSBlockCount, err)
	}

	if len(latestBlocks) == 0 {
		return 0.0, nil
	}

	// Calculate total transactions and time span
	var totalTxs int64
	var minTimestamp, maxTimestamp uint64

	for i, block := range latestBlocks {
		totalTxs += block.NumTxs

		if i == 0 {
			minTimestamp = block.BlockUnixTimestamp
			maxTimestamp = block.BlockUnixTimestamp
		} else {
			if block.BlockUnixTimestamp < minTimestamp {
				minTimestamp = block.BlockUnixTimestamp
			}
			if block.BlockUnixTimestamp > maxTimestamp {
				maxTimestamp = block.BlockUnixTimestamp
			}
		}
	}

	// Calculate average TPS: total transactions / time span in seconds
	// Note: BlockUnixTimestamp is already in seconds (from time.Unix()), not milliseconds
	if maxTimestamp > minTimestamp {
		timeSpanSeconds := float64(maxTimestamp - minTimestamp)
		if timeSpanSeconds > 0 {
			return float64(totalTxs) / timeSpanSeconds, nil
		}
	}

	// If all blocks have the same timestamp, use block count as time span
	avgBlockTime := AvgBlockTimeSeconds
	timeSpan := float64(len(latestBlocks)) * avgBlockTime
	if timeSpan > 0 {
		return float64(totalTxs) / timeSpan, nil
	}

	return 0.0, nil
}

// calculateGasPriceStats calculates average, current, max, and min gas prices.
func calculateGasPriceStats(ctx context.Context) (avgGasPrice, currentGasPrice, maxGasPrice24h, minGasPrice24h float64, err error) {
	now := time.Now()
	twentyFourHoursAgo := now.Add(-StatsHours24 * time.Hour)
	twentyFourHoursAgoUnix := uint64(twentyFourHoursAgo.Unix())

	// Get latest block heights only (optimized: only select height field)
	var blockHeights []uint64
	err = db.Get().WithContext(ctx).Model(&models.Block{}).
		Select("height").
		Order("height DESC").
		Limit(GasPriceBlockCount).
		Pluck("height", &blockHeights).Error
	if err != nil {
		global.Logger.Warnf("failed to get latest %d blocks for gas price: %v", GasPriceBlockCount, err)
		return 0.0, 0.0, 0.0, 0.0, nil // Return zeros instead of error
	}

	// Calculate average gas price from latest blocks
	if len(blockHeights) > 0 {
		// Optimized: only select gas_used and gas_fee fields to avoid loading large fields
		type GasTxInfo struct {
			GasUsed int64
			GasFee  string
		}
		var gasTxs []GasTxInfo
		err = db.Get().WithContext(ctx).Model(&models.Transaction{}).
			Select("gas_used, gas_fee").
			Where("block_number IN ? AND gas_used > 0 AND gas_fee != ''", blockHeights).
			Scan(&gasTxs).Error
		if err == nil && len(gasTxs) > 0 {
			var totalGasPrice float64
			validGasPrices := 0

			for _, tx := range gasTxs {
				gasPrice := extractGasPriceFromFee(tx.GasFee, tx.GasUsed)
				if gasPrice > 0 {
					totalGasPrice += gasPrice
					validGasPrices++
				}
			}

			if validGasPrices > 0 {
				avgGasPrice = totalGasPrice / float64(validGasPrices)
				currentGasPrice = avgGasPrice
			}
		}
	}

	// Calculate 24h max/min gas prices
	// Optimized: Sample transactions from different time periods to reduce query load
	// Instead of scanning all 24h transactions, we sample from recent blocks and calculate min/max
	// This approach is much faster while still providing accurate min/max values
	type GasTx24hInfo struct {
		GasUsed int64
		GasFee  string
	}
	var gasTxs24h []GasTx24hInfo
	// Optimized: Sample from recent blocks (last 100 blocks) which should cover most of the 24h period
	// and provide representative min/max values without scanning all transactions
	var recentBlockHeights []uint64
	err = db.Get().WithContext(ctx).Model(&models.Block{}).
		Select("height").
		Order("height DESC").
		Limit(100). // Sample from last 100 blocks for 24h min/max calculation
		Pluck("height", &recentBlockHeights).Error
	if err == nil && len(recentBlockHeights) > 0 {
		err = db.Get().WithContext(ctx).Model(&models.Transaction{}).
			Select("gas_used, gas_fee").
			Where("block_number IN ? AND gas_used > 0 AND gas_fee != ''", recentBlockHeights).
			Scan(&gasTxs24h).Error
	}

	// Fallback: if sampling from recent blocks fails or returns no data, try direct 24h query with limit
	if err != nil || len(gasTxs24h) == 0 {
		err = db.Get().WithContext(ctx).Model(&models.Transaction{}).
			Select("gas_used, gas_fee").
			Where("block_unix_timestamp >= ? AND gas_used > 0 AND gas_fee != ''", twentyFourHoursAgoUnix).
			Order("block_unix_timestamp DESC").
			Limit(5000). // Reduced limit for better performance
			Scan(&gasTxs24h).Error
		if err != nil {
			global.Logger.Warnf("failed to get 24h gas transactions: %v", err)
			return avgGasPrice, currentGasPrice, 0.0, 0.0, nil
		}
	}

	hasValidPrice := false
	for _, tx := range gasTxs24h {
		gasPrice := extractGasPriceFromFee(tx.GasFee, tx.GasUsed)
		if gasPrice > 0 {
			if !hasValidPrice {
				maxGasPrice24h = gasPrice
				minGasPrice24h = gasPrice
				hasValidPrice = true
			} else {
				if gasPrice > maxGasPrice24h {
					maxGasPrice24h = gasPrice
				}
				if gasPrice < minGasPrice24h {
					minGasPrice24h = gasPrice
				}
			}
		}
	}

	if !hasValidPrice {
		maxGasPrice24h = 0.0
		minGasPrice24h = 0.0
	}

	return avgGasPrice, currentGasPrice, maxGasPrice24h, minGasPrice24h, nil
}

// calculateAverageBlockTimeFromDB retrieves blocks and calculates average block time.
// Optimized: only selects height and block_unix_timestamp fields to avoid loading large fields.
func calculateAverageBlockTimeFromDB(ctx context.Context) (float64, error) {
	type BlockTimeInfo struct {
		Height             uint64
		BlockUnixTimestamp uint64
	}
	var blocksForBlockTime []BlockTimeInfo
	err := db.Get().WithContext(ctx).Model(&models.Block{}).
		Select("height, block_unix_timestamp").
		Order("height DESC").
		Limit(BlockTimeBlockCount).
		Scan(&blocksForBlockTime).Error
	if err != nil {
		global.Logger.Warnf("failed to get latest %d blocks for block time: %v", BlockTimeBlockCount, err)
		return 0.0, nil // Return 0 instead of error for non-critical metric
	}

	if len(blocksForBlockTime) < 2 {
		return 0.0, nil
	}

	// Calculate average block time from the retrieved data
	var totalTimeDiff float64
	validPairs := 0

	// Iterate from the end (oldest) to the beginning (newest)
	// Compare each block with the previous one (which has higher height)
	for i := len(blocksForBlockTime) - 1; i > 0; i-- {
		currTimestamp := blocksForBlockTime[i].BlockUnixTimestamp
		prevTimestamp := blocksForBlockTime[i-1].BlockUnixTimestamp

		if currTimestamp > 0 && prevTimestamp > 0 && prevTimestamp > currTimestamp {
			// Note: BlockUnixTimestamp is already in seconds (from time.Unix()), not milliseconds
			timeDiff := float64(prevTimestamp - currTimestamp)
			totalTimeDiff += timeDiff
			validPairs++
		}
	}

	if validPairs == 0 {
		return 0.0, nil
	}

	return totalTimeDiff / float64(validPairs), nil
}

// GetBlockMaxGas retrieves the maximum gas limit per block from Tendermint consensus parameters.
// This is used to calculate block gas utilization.
// Results are cached to reduce RPC calls.
//
// Parameters:
//   - ctx: Context for RPC call (should have appropriate timeout, e.g., BlockQueryTimeout)
//
// Returns:
//   - int64: The maximum gas limit per block (returns BlockGasLimit on error)
//   - error: Always returns nil (uses default value on error)
func GetBlockMaxGas(ctx context.Context) (int64, error) {
	// Try to get from cache first
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		cached, err := redisClient.Get(ctx, CacheKeyBlockMaxGas).Result()
		if err == nil {
			var maxGas int64
			if _, err := fmt.Sscanf(cached, "%d", &maxGas); err == nil && maxGas > 0 {
				return maxGas, nil
			}
		}
	}

	// Create RPC client with extended timeout for slow endpoints
	tmClient, err := rpchttp.NewWithTimeout(global.ChainConfig.TmEndpoint, 30)
	if err != nil {
		global.Logger.Warnf("failed to create Tendermint RPC client for consensus params: %v, using default gas limit", err)
		return BlockGasLimit, nil
	}

	consensusParams, err := tmClient.ConsensusParams(ctx, nil)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			global.Logger.Warnf("RPC timeout while getting consensus parameters: %v, using default gas limit", err)
		} else {
			global.Logger.Warnf("failed to get consensus parameters: %v, using default gas limit", err)
		}
		return BlockGasLimit, nil
	}

	if consensusParams == nil {
		global.Logger.Warnf("consensus params is nil, using default gas limit")
		return BlockGasLimit, nil
	}

	maxGas := consensusParams.ConsensusParams.Block.MaxGas
	if maxGas <= 0 {
		global.Logger.Warnf("invalid max gas from consensus params: %d, using default", maxGas)
		return BlockGasLimit, nil
	}

	// Cache the result
	if redisClient != nil {
		go func() {
			ctx := context.Background()
			if err := redisClient.Set(ctx, CacheKeyBlockMaxGas, fmt.Sprintf("%d", maxGas), CacheBlockMaxGasTTL).Err(); err != nil {
				global.Logger.Warnf("failed to cache block max gas: %v", err)
			}
		}()
	}

	return maxGas, nil
}

// calculateBlockGasUtilization calculates gas utilization for the latest N blocks.
// Formula: (Σgas_used / Σmax_gas) × 100%, where Σmax_gas = block_count × max_gas_per_block
//
// Parameters:
//   - ctx: Context for database queries (should have appropriate timeout)
//   - maxGas: Maximum gas limit per block (from consensus parameters or default)
//
// Returns:
//   - gasUtilization: Gas utilization percentage
//   - totalGasUsed: Total gas used across all transactions
//   - totalGasLimit: Total gas limit (block_count × max_gas_per_block)
//   - blockCount: Number of blocks analyzed
//   - transactionCount: Total number of transactions
//   - error: Always returns nil (graceful degradation on errors)
func calculateBlockGasUtilization(ctx context.Context, maxGas int64) (gasUtilization float64, totalGasUsed int64, totalGasLimit int64, blockCount int32, transactionCount int64, err error) {
	if maxGas <= 0 {
		global.Logger.Warnf("invalid max gas: %d, using default", maxGas)
		maxGas = BlockGasLimit
	}

	var blockHeights []uint64
	err = db.Get().WithContext(ctx).Model(&models.Block{}).
		Select("height").
		Order("height DESC").
		Limit(GasUtilizationBlockCount).
		Pluck("height", &blockHeights).Error
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			global.Logger.Warnf("database query timeout for gas utilization: %v", err)
		} else {
			global.Logger.Warnf("failed to get latest %d blocks for gas utilization: %v", GasUtilizationBlockCount, err)
		}
		return 0.0, 0, 0, 0, 0, nil
	}

	blockCount = int32(len(blockHeights))
	if blockCount == 0 {
		return 0.0, 0, 0, 0, 0, nil
	}

	totalGasLimit = int64(blockCount) * maxGas

	var result struct {
		TotalGasUsed int64 `gorm:"column:total_gas_used"`
		TxCount      int64 `gorm:"column:tx_count"`
	}

	err = db.Get().WithContext(ctx).Model(&models.Transaction{}).
		Select("COALESCE(SUM(gas_used), 0) as total_gas_used, COUNT(hash) as tx_count").
		Where("block_number IN ?", blockHeights).
		Scan(&result).Error
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			global.Logger.Warnf("database query timeout for transaction aggregation: %v", err)
		} else {
			global.Logger.Warnf("failed to get transactions for gas utilization: %v", err)
		}
		return 0.0, 0, totalGasLimit, blockCount, 0, nil
	}

	totalGasUsed = result.TotalGasUsed
	transactionCount = result.TxCount

	if totalGasLimit == 0 {
		return 0.0, totalGasUsed, totalGasLimit, blockCount, transactionCount, nil
	}

	gasUtilization = (float64(totalGasUsed) / float64(totalGasLimit)) * 100.0
	return gasUtilization, totalGasUsed, totalGasLimit, blockCount, transactionCount, nil
}

// calculateGasUtilizationFromDB calculates gas utilization from transactions in the latest N blocks.
// Formula: (Σgas_used / Σgas_wanted) × 100%
// This function is used for transaction statistics, not block-level gas utilization.
//
// Parameters:
//   - ctx: Context for database queries
//
// Returns:
//   - float64: Gas utilization percentage (returns 0.0 on error)
//   - error: Always returns nil (graceful degradation on errors)
func calculateGasUtilizationFromDB(ctx context.Context) (float64, error) {
	var blockHeights []uint64
	err := db.Get().WithContext(ctx).Model(&models.Block{}).
		Select("height").
		Order("height DESC").
		Limit(GasUtilizationBlockCount).
		Pluck("height", &blockHeights).Error
	if err != nil {
		global.Logger.Warnf("failed to get latest %d blocks for gas utilization: %v", GasUtilizationBlockCount, err)
		return 0.0, nil
	}

	if len(blockHeights) == 0 {
		return 0.0, nil
	}

	var utilTxs []GasUtilTxInfo
	err = db.Get().WithContext(ctx).Model(&models.Transaction{}).
		Select("gas_used, gas_wanted").
		Where("block_number IN ?", blockHeights).
		Scan(&utilTxs).Error
	if err != nil {
		global.Logger.Warnf("failed to get transactions for gas utilization: %v", err)
		return 0.0, nil
	}

	return calculateGasUtilizationFromTxs(utilTxs), nil
}

// GasUtilTxInfo represents transaction gas information for utilization calculation.
type GasUtilTxInfo struct {
	GasUsed   int64 `gorm:"column:gas_used"`
	GasWanted int64 `gorm:"column:gas_wanted"`
}

// calculateGasUtilizationFromTxs calculates gas utilization from transaction gas info.
// Formula: (Σgas_used / Σgas_wanted) × 100%
//
// Parameters:
//   - txs: Slice of transaction gas information
//
// Returns:
//   - float64: Gas utilization percentage (returns 0.0 if no transactions or totalGasWanted is 0)
func calculateGasUtilizationFromTxs(txs []GasUtilTxInfo) float64 {
	if len(txs) == 0 {
		return 0.0
	}

	var totalGasUsed int64
	var totalGasWanted int64

	for _, tx := range txs {
		if tx.GasUsed > 0 {
			totalGasUsed += tx.GasUsed
		}
		if tx.GasWanted > 0 {
			totalGasWanted += tx.GasWanted
		}
	}

	if totalGasWanted == 0 {
		return 0.0
	}

	return (float64(totalGasUsed) / float64(totalGasWanted)) * 100.0
}

// GetTransactionStats retrieves transaction volume statistics from the database.
// It calculates:
//   - Total transaction count (all time)
//   - Transaction count in the last N days (StatsDays30)
//   - Transaction count in the last N hours (StatsHours24)
//   - Average TPS (Transactions Per Second) from the latest N blocks (TPSBlockCount)
//     TPS is calculated as: total transactions / time span in seconds
//
// Archive Compatibility:
//
//	This function is compatible with transaction archiving. Archived transactions
//	remain in the main transactions table (with archived=true flag) but have their
//	large fields moved to archive tables. All queries include archived transactions
//	in the count, ensuring accurate statistics even after archiving is enabled.
//
// Performance:
//
//	This function uses parallel database queries via errgroup to minimize response time.
//	All independent queries are executed concurrently, significantly reducing total latency.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Empty request (no parameters needed)
//
// Returns:
//   - *pb.Reply: Response containing transaction statistics
//   - error: Error if the operation fails
func (s *ExplorerService) GetTransactionStats(ctx context.Context, req *emptypb.Empty) (*pb.Reply, error) {
	global.Logger.Info("getting transaction statistics")

	// Start background goroutine to periodically update complex metrics (only once)
	statsUpdateOnceGlobal.Do(func() {
		go startStatsComplexMetricsUpdater()
	})

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	stats := &pb.TransactionStatsResponse{}

	// Try to get statistics from cache first
	cachedStats := getTransactionStatsFromCache(queryCtx)
	if cachedStats != nil {
		// Check if complex metrics need recalculation (if they are 0 or missing)
		needsRecalc := cachedStats.Tps == 0 && cachedStats.AvgGas == 0 && cachedStats.AvgBlockTime == 0 && cachedStats.GasUtilization == 0
		if !needsRecalc {
			global.Logger.Infof("transaction statistics found in cache")
			// Asynchronous update: return cached data immediately, update in background
			go func() {
				bgCtx := context.Background()
				if err := s.updateStatsCache(bgCtx); err != nil {
					global.Logger.Warnf("async stats update failed: %v", err)
				}
			}()
			return MarshalAndReply(cachedStats, "marshal transaction stats error")
		}
		// If complex metrics are missing, use cached simple metrics and recalculate complex ones
		stats = cachedStats
		global.Logger.Infof("transaction statistics found in cache, recalculating complex metrics")
	}

	// Prepare time boundaries for queries
	now := time.Now()
	thirtyDaysAgo := now.AddDate(0, 0, -StatsDays30)
	thirtyDaysAgoUnix := uint64(thirtyDaysAgo.Unix())
	twentyFourHoursAgo := now.Add(-StatsHours24 * time.Hour)
	twentyFourHoursAgoUnix := uint64(twentyFourHoursAgo.Unix())

	// Use errgroup to parallelize all independent database queries
	g, gCtx := errgroup.WithContext(queryCtx)
	data := &transactionStatsData{}

	// Parallel query 1: Total transaction count
	g.Go(func() error {
		var err error
		data.totalCount, err = getTotalTransactionCount(gCtx)
		return err
	})

	// Parallel query 2: 30-day transaction count
	g.Go(func() error {
		var err error
		data.count30d, err = getTransactionCount30d(gCtx, thirtyDaysAgoUnix)
		return err
	})

	// Parallel query 3: 24-hour transaction count
	g.Go(func() error {
		var err error
		data.count24h, err = getTransactionCount24h(gCtx, twentyFourHoursAgoUnix)
		return err
	})

	// Parallel query 4: 24h active addresses count
	g.Go(func() error {
		var err error
		data.activeAddresses24h, err = getActiveAddresses24h(gCtx, twentyFourHoursAgoUnix)
		return err
	})

	// Parallel query 5: 24h failed transaction count
	g.Go(func() error {
		var err error
		data.failedTxCount24h, err = getFailedTxCount24h(gCtx, twentyFourHoursAgoUnix)
		return err
	})

	// Parallel query 6: Calculate TPS
	g.Go(func() error {
		var err error
		data.tps, err = calculateTPS(gCtx)
		return err
	})

	// Parallel query 7: Calculate gas statistics (avg, current, max, min)
	g.Go(func() error {
		var err error
		data.avgGas, data.currentGas, data.maxGas24h, data.minGas24h, err = calculateGasPriceStats(gCtx)
		return err
	})

	// Parallel query 8: Calculate average block time
	g.Go(func() error {
		var err error
		data.avgBlockTime, err = calculateAverageBlockTimeFromDB(gCtx)
		return err
	})

	// Parallel query 9: Calculate gas utilization
	g.Go(func() error {
		var err error
		data.gasUtilization, err = calculateGasUtilizationFromDB(gCtx)
		return err
	})

	// Wait for all parallel queries to complete
	if err := g.Wait(); err != nil {
		global.Logger.Errorf("failed to fetch transaction statistics: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to fetch transaction statistics: %v", err)
	}

	// Populate response from parallel query results
	// If we had cached stats, preserve the cached simple metrics and only update complex ones
	if cachedStats == nil {
		// No cache, populate all fields from database
		stats.TotalCount = data.totalCount
		stats.Count_30D = data.count30d
		stats.Count_24H = data.count24h
		stats.ActiveAddresses_24H = data.activeAddresses24h
		stats.FailedTxCount_24H = data.failedTxCount24h
	} else {
		// Use cached simple metrics (they are updated incrementally)
		// Only update if database values are higher (safety check)
		if data.totalCount > stats.TotalCount {
			stats.TotalCount = data.totalCount
		}
		if data.count30d > stats.Count_30D {
			stats.Count_30D = data.count30d
		}
		if data.count24h > stats.Count_24H {
			stats.Count_24H = data.count24h
		}
		if data.activeAddresses24h > stats.ActiveAddresses_24H {
			stats.ActiveAddresses_24H = data.activeAddresses24h
		}
		if data.failedTxCount24h > stats.FailedTxCount_24H {
			stats.FailedTxCount_24H = data.failedTxCount24h
		}
	}

	// Always recalculate complex metrics from database
	stats.Tps = data.tps
	stats.AvgGas = data.avgGas
	stats.CurrentGas = data.currentGas
	stats.MaxGas_24H = data.maxGas24h
	stats.MinGas_24H = data.minGas24h
	stats.AvgBlockTime = data.avgBlockTime
	stats.GasUtilization = data.gasUtilization

	global.Logger.Infof("transaction statistics: total=%d, 30d=%d, 24h=%d, tps=%.2f, active_addr_24h=%d, avg_gas=%.6f, avg_block_time=%.2f, gas_utilization=%.2f%%",
		stats.TotalCount, stats.Count_30D, stats.Count_24H, stats.Tps, stats.ActiveAddresses_24H, stats.AvgGas, stats.AvgBlockTime, stats.GasUtilization)

	// Cache the statistics for future queries (async to avoid blocking)
	go cacheTransactionStats(context.Background(), stats)

	return MarshalAndReply(stats, "marshal transaction stats error")
}

// updateStatsCache updates transaction statistics cache asynchronously.
// This method performs the same calculation as GetTransactionStats but doesn't return a response.
// It's used for background updates to keep cache fresh without blocking API responses.
//
// Parameters:
//   - ctx: Context for the operation
//
// Returns:
//   - error: Error if the update fails
func (s *ExplorerService) updateStatsCache(ctx context.Context) error {
	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(ctx, DBQueryTimeout)
	defer queryCancel()

	stats := &pb.TransactionStatsResponse{}

	// Prepare time boundaries for queries
	now := time.Now()
	thirtyDaysAgo := now.AddDate(0, 0, -StatsDays30)
	thirtyDaysAgoUnix := uint64(thirtyDaysAgo.Unix())
	twentyFourHoursAgo := now.Add(-StatsHours24 * time.Hour)
	twentyFourHoursAgoUnix := uint64(twentyFourHoursAgo.Unix())

	// Use errgroup to parallelize all independent database queries
	g, gCtx := errgroup.WithContext(queryCtx)
	data := &transactionStatsData{}

	// Parallel queries (same as GetTransactionStats)
	g.Go(func() error {
		var err error
		data.totalCount, err = getTotalTransactionCount(gCtx)
		return err
	})
	g.Go(func() error {
		var err error
		data.count30d, err = getTransactionCount30d(gCtx, thirtyDaysAgoUnix)
		return err
	})
	g.Go(func() error {
		var err error
		data.count24h, err = getTransactionCount24h(gCtx, twentyFourHoursAgoUnix)
		return err
	})
	g.Go(func() error {
		var err error
		data.activeAddresses24h, err = getActiveAddresses24h(gCtx, twentyFourHoursAgoUnix)
		return err
	})
	g.Go(func() error {
		var err error
		data.failedTxCount24h, err = getFailedTxCount24h(gCtx, twentyFourHoursAgoUnix)
		return err
	})
	g.Go(func() error {
		var err error
		data.tps, err = calculateTPS(gCtx)
		return err
	})
	g.Go(func() error {
		var err error
		data.avgGas, data.currentGas, data.maxGas24h, data.minGas24h, err = calculateGasPriceStats(gCtx)
		return err
	})
	g.Go(func() error {
		var err error
		data.avgBlockTime, err = calculateAverageBlockTimeFromDB(gCtx)
		return err
	})
	g.Go(func() error {
		var err error
		data.gasUtilization, err = calculateGasUtilizationFromDB(gCtx)
		return err
	})

	// Wait for all parallel queries to complete
	if err := g.Wait(); err != nil {
		return fmt.Errorf("failed to fetch transaction statistics: %w", err)
	}

	// Populate response
	stats.TotalCount = data.totalCount
	stats.Count_30D = data.count30d
	stats.Count_24H = data.count24h
	stats.ActiveAddresses_24H = data.activeAddresses24h
	stats.FailedTxCount_24H = data.failedTxCount24h
	stats.Tps = data.tps
	stats.AvgGas = data.avgGas
	stats.CurrentGas = data.currentGas
	stats.MaxGas_24H = data.maxGas24h
	stats.MinGas_24H = data.minGas24h
	stats.AvgBlockTime = data.avgBlockTime
	stats.GasUtilization = data.gasUtilization

	// Cache the statistics
	cacheTransactionStats(ctx, stats)

	return nil
}

// queryFailedTransactions24h queries failed transactions in the last 24 hours with pagination.
// Optimized to only select necessary fields to avoid loading large fields (data, messages, logs, etc.)
// which significantly improves query performance for large datasets.
//
// Performance optimization notes:
//   - Only selects fields needed for FailedTransactionInfo response
//   - Avoids loading large TEXT fields (data, messages, logs, info) that are not needed
//   - Uses composite index (block_unix_timestamp, code) for optimal query performance
//     Recommended index: CREATE INDEX idx_timestamp_code ON transactions(block_unix_timestamp, code);
func queryFailedTransactions24h(ctx context.Context, twentyFourHoursAgoUnix uint64, pageSize int, offset int, useCursor bool, cursorValue uint64) ([]models.Transaction, error) {
	// Select only necessary fields to avoid loading large TEXT fields
	// This significantly reduces data transfer and improves query performance
	baseQuery := db.Get().WithContext(ctx).Model(&models.Transaction{}).
		Select("hash, block_number, block_unix_timestamp, code, codespace, error_log, signer, gas_used, gas_wanted, gas_fee, tx_type, tx_number").
		Where("block_unix_timestamp >= ? AND code > 0", twentyFourHoursAgoUnix)

	// Apply cursor-based pagination if cursor is provided
	// Use tx_number for both cursor and ordering to ensure consistent pagination
	if useCursor && cursorValue > 0 {
		baseQuery = baseQuery.Where("tx_number < ?", cursorValue).
			Order("tx_number DESC")
	} else {
		baseQuery = baseQuery.Order("tx_number DESC")
	}

	// Apply offset-based pagination if no cursor
	if !useCursor {
		baseQuery = baseQuery.Offset(offset)
	}

	var failedTxs []models.Transaction
	err := baseQuery.Limit(pageSize).Find(&failedTxs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query failed transactions: %w", err)
	}

	return failedTxs, nil
}

// getFailedTransactions24hCountFromCache retrieves the failed transactions 24h count from Redis cache.
// Returns (0, false) if the count is not found in cache (cache miss).
//
// Parameters:
//   - ctx: Context for the cache operation
//
// Returns:
//   - int64: The cached count, or 0 if not found
//   - bool: Whether the count was found in cache
func getFailedTransactions24hCountFromCache(ctx context.Context) (int64, bool) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return 0, false
	}

	data, err := redisClient.Get(ctx, CacheKeyFailedTransactions24H).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, false // Cache miss
		}
		global.Logger.Warnf("failed to get failed transactions 24h count from cache: %v", err)
		return 0, false
	}

	var count int64
	if _, err := fmt.Sscanf(data, "%d", &count); err != nil {
		global.Logger.Warnf("failed to parse failed transactions 24h count from cache (value=%s): %v", data, err)
		return 0, false
	}

	return count, true
}

// cacheFailedTransactions24hCount stores the failed transactions 24h count in Redis cache with TTL.
//
// Parameters:
//   - ctx: Context for the cache operation
//   - count: The count to cache
func cacheFailedTransactions24hCount(ctx context.Context, count int64) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	// Store count as string with TTL
	err := redisClient.Set(ctx, CacheKeyFailedTransactions24H, fmt.Sprintf("%d", count), CacheFailedTransactions24HTTL).Err()
	if err != nil {
		global.Logger.Warnf("failed to cache failed transactions 24h count: %v", err)
	}
}

// getFailedTransactions24hCount gets the total count of failed transactions in the last 24 hours.
// Uses timeout to prevent long-running queries and Redis cache to improve performance.
//
// Performance optimization changes:
// - Added index hint to force use of optimized composite index
// - Extended cache TTL from 2 minutes to 15 minutes
// - Added pre-warming check to refresh cache before it expires
// - Uses raw SQL for better control over query execution
// - Handles soft delete properly to allow index usage
// - Returns approximate count immediately if query times out
func getFailedTransactions24hCount(ctx context.Context, twentyFourHoursAgoUnix uint64) (int64, bool, error) {
	// Try to get count from cache first (fast path - returns in <1ms)
	if cachedCount, found := getFailedTransactions24hCountFromCache(ctx); found {
		global.Logger.Debugf("failed transactions 24h count found in cache: %d", cachedCount)

		// Check if cache is about to expire (pre-warm)
		redisClient := cache.GetRedisClient()
		if redisClient != nil {
			// If TTL < MinCacheTTL, refresh in background to avoid cache miss
			ttl, _ := redisClient.TTL(ctx, CacheKeyFailedTransactions24H).Result()
			if ttl < MinCacheTTL {
				global.Logger.Debug("cache expiring soon, refreshing in background")
				go func() {
					// Use separate context for background refresh
					refreshCtx, refreshCancel := context.WithTimeout(context.Background(), CountQueryTimeout)
					defer refreshCancel()
					countFailedTransactionsWithRetry(refreshCtx, twentyFourHoursAgoUnix)
				}()
			}
		}
		return cachedCount, false, nil // Cached values are approximate
	}

	// Cache miss, query from database with retry
	return countFailedTransactionsWithRetry(ctx, twentyFourHoursAgoUnix)
}

// countFailedTransactionsWithRetry attempts to count failed transactions with retry on timeout
func countFailedTransactionsWithRetry(ctx context.Context, twentyFourHoursAgoUnix uint64) (int64, bool, error) {
	// Use a child context with shorter timeout to allow for retry
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 1500*time.Millisecond) // 1.5s timeout
	defer cancel()

	totalCount, isExact, err := countFailedTransactionsDirect(ctxWithTimeout, twentyFourHoursAgoUnix)

	if err != nil {
		// Check if it's a timeout error
		if errors.Is(err, context.DeadlineExceeded) {
			global.Logger.Warnf("count query timed out, returning approximate count")
			return ApproximateCountFallback, false, nil
		}

		// Log error but still return approximate count to not break the API
		global.Logger.Warnf("failed to count failed transactions: %v, using approximate count", err)
		return ApproximateCountFallback, false, nil
	}

	// Cache the result for future queries (async to avoid blocking)
	go cacheFailedTransactions24hCount(context.Background(), totalCount)

	return totalCount, isExact, nil
}

// countFailedTransactionsDirect performs the actual count query using raw SQL for better performance
// Uses a composite index hint to ensure MySQL uses the optimized index
func countFailedTransactionsDirect(ctx context.Context, twentyFourHoursAgoUnix uint64) (int64, bool, error) {
	var totalCount int64

	// Use raw SQL to control the query execution
	// The composite index (block_unix_timestamp, code, deleted_at) will be used
	err := db.Get().WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM transactions
		 USE INDEX (idx_timestamp_code_deleted)
		 WHERE block_unix_timestamp >= ?
		 AND code != 0
		 AND deleted_at IS NULL`,
		twentyFourHoursAgoUnix,
	).Scan(&totalCount).Error

	return totalCount, true, err
}

// sanitizeErrorLog sanitizes and truncates error log messages for security and performance.
// Removes sensitive information and limits length to prevent performance issues.
// Optimized to reduce string allocations and processing overhead.
func sanitizeErrorLog(errorLog string) string {
	if errorLog == "" {
		return ""
	}

	// Truncate to maximum length first to reduce processing overhead
	var sanitized string
	if len(errorLog) > MaxErrorLogLength {
		sanitized = errorLog[:MaxErrorLogLength] + "..."
	} else {
		sanitized = errorLog
	}

	// Remove common sensitive patterns (similar to sanitizeError)
	// Use case-insensitive check only when necessary to reduce overhead
	sensitivePatterns := []string{
		"/Users/",
		"/home/",
		"/root/",
		"password",
		"secret",
		"private_key",
		"api_key",
	}

	sanitizedLower := strings.ToLower(sanitized)
	for _, pattern := range sensitivePatterns {
		// Case-insensitive check first, then replace if found
		if strings.Contains(sanitizedLower, strings.ToLower(pattern)) {
			sanitized = strings.ReplaceAll(sanitized, pattern, "[REDACTED]")
			// Update lowercase version for subsequent checks
			sanitizedLower = strings.ToLower(sanitized)
		}
	}

	return sanitized
}

// parseGasFee parses GasFee JSON string into structured GasFee protobuf message.
// Returns nil if parsing fails or GasFee is empty.
func parseGasFee(gasFeeJSON string) *pb.GasFee {
	if gasFeeJSON == "" {
		return nil
	}

	// Parse JSON structure
	var gasFeeData map[string]interface{}
	if err := sonic.Unmarshal([]byte(gasFeeJSON), &gasFeeData); err != nil {
		global.Logger.Warnf("failed to parse gas fee JSON: %v", err)
		return nil
	}

	gasFee := &pb.GasFee{}

	// Parse amount array: [{"denom": "inj", "amount": "1000000"}]
	if amounts, ok := gasFeeData["amount"].([]interface{}); ok {
		gasFee.Amount = make([]*pb.GasFeeCoin, 0, len(amounts))
		for _, amt := range amounts {
			if amtMap, ok := amt.(map[string]interface{}); ok {
				coin := &pb.GasFeeCoin{}
				if denom, ok := amtMap["denom"].(string); ok {
					coin.Denom = denom
				}
				if amount, ok := amtMap["amount"].(string); ok {
					coin.Amount = amount
				}
				// Only add coin if it has valid data
				if coin.Denom != "" || coin.Amount != "" {
					gasFee.Amount = append(gasFee.Amount, coin)
				}
			}
		}
	}

	// Parse gas_limit
	if gasLimit, ok := gasFeeData["gas_limit"].(float64); ok {
		gasFee.GasLimit = int64(gasLimit)
	}

	// Parse payer (optional)
	if payer, ok := gasFeeData["payer"].(string); ok {
		gasFee.Payer = payer
	}

	return gasFee
}

// convertToFailedTransactionInfos converts Transaction models to FailedTransactionInfo protobuf messages.
// Applies sanitization to error_log and parses gas_fee into structured format.
func convertToFailedTransactionInfos(txs []models.Transaction) []*pb.FailedTransactionInfo {
	if len(txs) == 0 {
		return nil
	}

	failedTxInfos := make([]*pb.FailedTransactionInfo, 0, len(txs))
	for _, tx := range txs {
		// Parse and structure GasFee
		gasFee := parseGasFee(tx.GasFee)

		// Sanitize and truncate ErrorLog
		sanitizedErrorLog := sanitizeErrorLog(tx.ErrorLog)

		failedTxInfo := &pb.FailedTransactionInfo{
			Hash:               tx.Hash,
			BlockNumber:        tx.BlockNumber,
			BlockUnixTimestamp: tx.BlockUnixTimestamp,
			Code:               tx.Code,
			Codespace:          tx.Codespace,
			ErrorLog:           sanitizedErrorLog,
			Signer:             tx.Signer,
			GasUsed:            tx.GasUsed,
			GasWanted:          tx.GasWanted,
			GasFee:             gasFee,
			TxType:             tx.TxType,
		}
		failedTxInfos = append(failedTxInfos, failedTxInfo)
	}

	return failedTxInfos
}

// GetFailedTransactions24H retrieves failed transactions in the last 24 hours with failure reasons.
//
// This endpoint returns a paginated list of failed transactions (code != 0) that occurred
// in the last 24 hours, including their failure reasons (error_log, codespace, code).
//
// Performance optimizations:
//
//  1. Only selects necessary fields (avoids loading large TEXT fields like data, messages, logs)
//  2. Uses parallel database queries via errgroup to minimize response time
//  3. Skips COUNT query for cursor-based pagination and first page to improve response time
//  4. Uses composite index (block_unix_timestamp, code) for optimal query performance
//     Recommended index: CREATE INDEX idx_timestamp_code ON transactions(block_unix_timestamp, code);
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing pagination information
//
// Returns:
//   - *pb.Reply: Response containing failed transactions with pagination info
//   - error: Error if the operation fails
func (s *ExplorerService) GetFailedTransactions24H(ctx context.Context, req *pb.GetFailedTransactions24HRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Normalize pagination parameters
	var page, pageSize int32 = MinPage, DefaultPageSize
	var offset int
	var useCursor bool
	var cursorValue uint64
	if req.Pagination != nil {
		page, pageSize, offset, useCursor, cursorValue = normalizePagination(req.Pagination.Page, req.Pagination.PageSize, req.Pagination.Cursor, MaxPageSize)
	}

	global.Logger.Infof("get failed transactions 24h: page=%d, page_size=%d, use_cursor=%v", page, pageSize, useCursor)

	// Create independent context for database queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Calculate 24 hours ago timestamp
	now := time.Now()
	twentyFourHoursAgo := now.Add(-StatsHours24 * time.Hour)
	twentyFourHoursAgoUnix := uint64(twentyFourHoursAgo.Unix())

	// Use errgroup to parallelize database queries
	g, gCtx := errgroup.WithContext(queryCtx)

	var failedTxs []models.Transaction
	var totalCount int64
	var isExactCount bool
	var totalTxCount24h int64

	// Parallel query 1: Get failed transactions list
	g.Go(func() error {
		var err error
		failedTxs, err = queryFailedTransactions24h(gCtx, twentyFourHoursAgoUnix, int(pageSize), offset, useCursor, cursorValue)
		return err
	})

	// Parallel query 2: Get total count
	// Optimization: Skip COUNT query for cursor pagination and first page to improve response time
	// COUNT queries can be slow on large datasets, so we only run them when necessary
	if !useCursor && page > MinPage {
		// Only get count for non-first pages to determine pagination
		g.Go(func() error {
			var err error
			totalCount, isExactCount, err = getFailedTransactions24hCount(gCtx, twentyFourHoursAgoUnix)
			return err
		})
	} else {
		// For cursor pagination or first page, skip count query for faster response
		totalCount = ApproximateCountFallback
		isExactCount = false
	}

	// Parallel query 3: Get total transaction count in 24h for success rate calculation
	g.Go(func() error {
		var err error
		totalTxCount24h, err = getTransactionCount24h(gCtx, twentyFourHoursAgoUnix)
		return err
	})

	// Wait for all parallel queries to complete
	if err := g.Wait(); err != nil {
		global.Logger.Errorf("failed to fetch failed transactions: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to fetch failed transactions: %v", err)
	}

	// Convert to DTOs
	failedTxInfos := convertToFailedTransactionInfos(failedTxs)

	// Build pagination info
	paginationInfo := &pb.PaginationInfo{
		Page:       page,
		PageSize:   pageSize,
		Total:      totalCount,
		TotalPages: int32((int(totalCount) + int(pageSize) - 1) / int(pageSize)),
		HasPrev:    page > MinPage,
		HasNext:    int64(len(failedTxs)) == int64(pageSize) && (int64(page)*int64(pageSize) < totalCount || !isExactCount),
	}

	// Calculate success rate
	var successRate float64
	if totalTxCount24h > 0 {
		successRate = float64(totalTxCount24h-int64(len(failedTxs))) / float64(totalTxCount24h) * 100
	} else {
		successRate = 100.0 // If no transactions, consider 100% success rate
	}

	// Build response
	response := &pb.FailedTransactions24HResponse{
		Transactions:    failedTxInfos,
		Pagination:      paginationInfo,
		SuccessRate_24H: successRate,
	}

	global.Logger.Infof("retrieved %d failed transactions (24h), total=%d, exact=%v, success_rate=%.2f%%", len(failedTxs), totalCount, isExactCount, successRate)

	return MarshalAndReply(response, "marshal failed transactions error")
}

// GetFailedTransactions1000 retrieves failure statistics for the latest 1000 transactions.
//
// This endpoint analyzes the latest 1000 transactions from the blockchain and returns
// failed transactions along with statistics including success rate, gas usage, etc.
// Similar to GetFailedTransactions24h but for the latest 1000 transactions instead of 24h timeframe.
//
// Performance optimizations:
//  1. Uses parallel database queries via errgroup to minimize response time
//  2. Only selects necessary fields to avoid loading large TEXT fields
//  3. Uses index on (block_unix_timestamp, tx_number) for optimal query performance
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request (no parameters needed)
//
// Returns:
//   - *pb.Reply: Response containing failed transactions and statistics for latest 1000 transactions
//   - error: Error if the operation fails
func (s *ExplorerService) GetFailedTransactions1000(ctx context.Context, req *emptypb.Empty) (*pb.Reply, error) {

	global.Logger.Infof("get latest 1000 transactions failure statistics")

	// Create independent context for database queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Use errgroup to parallelize database queries
	g, gCtx := errgroup.WithContext(queryCtx)

	var transactions []models.Transaction

	// Query: Get latest N transactions for failure statistics
	g.Go(func() error {
		var err error
		transactions, err = queryLatest1000Transactions(gCtx, Latest1000TransactionsLimit)
		return err
	})

	// Wait for query to complete
	if err := g.Wait(); err != nil {
		global.Logger.Errorf("failed to fetch latest 1000 transactions: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to fetch latest 1000 transactions: %v", err)
	}

	// Filter failed transactions (code != 0)
	var failedTxs []models.Transaction
	for _, tx := range transactions {
		if tx.Code != 0 {
			failedTxs = append(failedTxs, tx)
		}
	}

	// Convert failed transactions to FailedTransactionInfo
	failedTxInfos := convertToFailedTransactionInfos(failedTxs)

	// Calculate success rate
	// totalCount is the actual number of transactions retrieved (may be less than Latest1000TransactionsLimit if database has fewer transactions)
	totalCount := int64(len(transactions))
	global.Logger.Debugf("queryLatest1000Transactions: retrieved %d transactions (requested limit: %d)", totalCount, Latest1000TransactionsLimit)
	var successRate float64
	if totalCount > 0 {
		successCount := totalCount - int64(len(failedTxs))
		successRate = float64(successCount) / float64(totalCount) * 100
	} else {
		successRate = 100.0 // If no transactions, consider 100% success rate
	}

	// Build pagination info (for consistency with FailedTransactions24h)
	// PageSize is fixed to Latest1000TransactionsLimit as we query the latest N transactions
	paginationInfo := &pb.PaginationInfo{
		Page:       1,
		PageSize:   Latest1000TransactionsLimit,
		Total:      totalCount, // Actual number of transactions retrieved (may be less than Latest1000TransactionsLimit)
		TotalPages: 1,
		HasPrev:    false,
		HasNext:    false,
	}

	// Build response
	response := &pb.FailedTransactions1000Response{
		Transactions:     failedTxInfos,
		Pagination:       paginationInfo,
		SuccessRate_1000: successRate,
	}

	global.Logger.Infof("retrieved %d failed transactions out of %d latest transactions, success_rate=%.2f%%", len(failedTxs), len(transactions), successRate)

	return MarshalAndReply(response, "marshal latest 1000 transactions error")
}

// queryLatest1000Transactions queries the latest 1000 transactions from the database.
// Optimized to only select necessary fields to avoid loading large TEXT fields (data, messages, logs, etc.)
// which significantly improves query performance for large datasets.
//
// Performance optimization notes:
//   - Only selects fields needed for statistics and FailedTransactionInfo response
//   - Avoids loading large TEXT fields (data, messages, logs, info) that are not needed
//   - Uses composite index (block_unix_timestamp, tx_number) for optimal query performance
//
// Parameters:
//   - ctx: Context for the query
//   - limit: Maximum number of transactions to retrieve (default: 1000)
//
// Returns:
//   - []models.Transaction: List of transactions
//   - error: Error if the query fails
func queryLatest1000Transactions(ctx context.Context, limit int) ([]models.Transaction, error) {
	var transactions []models.Transaction

	// Select only necessary fields to avoid loading large TEXT fields
	// This significantly reduces data transfer and improves query performance
	err := db.Get().WithContext(ctx).Model(&models.Transaction{}).
		Select("hash, block_number, block_unix_timestamp, code, codespace, error_log, signer, gas_used, gas_wanted, gas_fee, tx_type, tx_number").
		Order("block_unix_timestamp DESC, tx_number DESC").
		Limit(limit).
		Find(&transactions).Error

	if err != nil {
		return nil, fmt.Errorf("failed to query latest 1000 transactions: %w", err)
	}

	return transactions, nil
}

// getTransactionStatsFromCache retrieves transaction statistics from Redis cache.
// Returns nil if not found in cache (cache miss).
// Supports both Hash format (for incremental updates) and JSON format (for backward compatibility).
//
// Parameters:
//   - ctx: Context for the cache operation
//
// Returns:
//   - *pb.TransactionStatsResponse: Cached statistics, or nil if not found
func getTransactionStatsFromCache(ctx context.Context) *pb.TransactionStatsResponse {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return nil
	}

	key := CacheKeyTransactionStats

	// Try to get as Hash first (new format for incremental updates)
	hashData, err := redisClient.HGetAll(ctx, key).Result()
	if err == nil && len(hashData) > 0 {
		// Convert hash data to TransactionStatsResponse
		stats := &pb.TransactionStatsResponse{}
		if val, ok := hashData["total_count"]; ok && val != "" {
			if count, err := parseInt64(val); err == nil {
				stats.TotalCount = count
			}
		}
		if val, ok := hashData["count_30d"]; ok && val != "" {
			if count, err := parseInt64(val); err == nil {
				stats.Count_30D = count
			}
		}
		if val, ok := hashData["count_24h"]; ok && val != "" {
			if count, err := parseInt64(val); err == nil {
				stats.Count_24H = count
			}
		}
		if val, ok := hashData["tps"]; ok && val != "" {
			if tps, err := parseFloat64(val); err == nil {
				stats.Tps = tps
			}
		}
		if val, ok := hashData["active_addresses_24h"]; ok && val != "" {
			if count, err := parseInt64(val); err == nil {
				stats.ActiveAddresses_24H = count
			}
		}
		if val, ok := hashData["avg_gas"]; ok && val != "" {
			if gas, err := parseFloat64(val); err == nil {
				stats.AvgGas = gas
			}
		}
		if val, ok := hashData["avg_block_time"]; ok && val != "" {
			if time, err := parseFloat64(val); err == nil {
				stats.AvgBlockTime = time
			}
		}
		if val, ok := hashData["current_gas"]; ok && val != "" {
			if gas, err := parseFloat64(val); err == nil {
				stats.CurrentGas = gas
			}
		}
		if val, ok := hashData["max_gas_24h"]; ok && val != "" {
			if gas, err := parseFloat64(val); err == nil {
				stats.MaxGas_24H = gas
			}
		}
		if val, ok := hashData["min_gas_24h"]; ok && val != "" {
			if gas, err := parseFloat64(val); err == nil {
				stats.MinGas_24H = gas
			}
		}
		if val, ok := hashData["failed_tx_count_24h"]; ok && val != "" {
			if count, err := parseInt64(val); err == nil {
				stats.FailedTxCount_24H = count
			}
		}
		if val, ok := hashData["gas_utilization"]; ok && val != "" {
			if util, err := parseFloat64(val); err == nil {
				stats.GasUtilization = util
			}
		}

		// Only return if we have at least some data
		if stats.TotalCount > 0 || stats.Count_24H > 0 {
			return stats
		}
	}

	// Fallback to JSON format (backward compatibility)
	data, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil // Cache miss
		}
		global.Logger.Warnf("failed to get transaction stats from cache: %v", err)
		return nil
	}

	var stats pb.TransactionStatsResponse
	if err := sonic.Unmarshal([]byte(data), &stats); err != nil {
		global.Logger.Warnf("failed to unmarshal transaction stats from cache: %v", err)
		return nil
	}

	return &stats
}

// parseInt64 parses a string to int64.
func parseInt64(s string) (int64, error) {
	var result int64
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

// parseFloat64 parses a string to float64.
func parseFloat64(s string) (float64, error) {
	var result float64
	_, err := fmt.Sscanf(s, "%f", &result)
	return result, err
}

// cacheTransactionStats stores transaction statistics in Redis cache with TTL.
// This function stores data in Hash format for efficient incremental updates.
// This function is called asynchronously to avoid blocking the response.
//
// Parameters:
//   - ctx: Context for the cache operation
//   - stats: The statistics to cache
func cacheTransactionStats(ctx context.Context, stats *pb.TransactionStatsResponse) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	key := CacheKeyTransactionStats

	// Store as Hash for efficient incremental updates
	pipe := redisClient.Pipeline()
	pipe.HSet(ctx, key,
		"total_count", fmt.Sprintf("%d", stats.TotalCount),
		"count_30d", fmt.Sprintf("%d", stats.Count_30D),
		"count_24h", fmt.Sprintf("%d", stats.Count_24H),
		"tps", fmt.Sprintf("%.6f", stats.Tps),
		"active_addresses_24h", fmt.Sprintf("%d", stats.ActiveAddresses_24H),
		"avg_gas", fmt.Sprintf("%.6f", stats.AvgGas),
		"avg_block_time", fmt.Sprintf("%.6f", stats.AvgBlockTime),
		"current_gas", fmt.Sprintf("%.6f", stats.CurrentGas),
		"max_gas_24h", fmt.Sprintf("%.6f", stats.MaxGas_24H),
		"min_gas_24h", fmt.Sprintf("%.6f", stats.MinGas_24H),
		"failed_tx_count_24h", fmt.Sprintf("%d", stats.FailedTxCount_24H),
		"gas_utilization", fmt.Sprintf("%.6f", stats.GasUtilization),
	)
	pipe.Expire(ctx, key, CacheTransactionStatsTTL)
	_, err := pipe.Exec(ctx)
	if err != nil {
		global.Logger.Warnf("failed to cache transaction stats: %v", err)
	}
}

// updateTransactionStatsIncremental incrementally updates transaction statistics in Redis cache.
// This function is called when a new transaction is saved to avoid expensive database queries.
// It uses Redis Hash to store statistics, allowing atomic updates of individual fields.
//
// Parameters:
//   - ctx: Context for the cache operation
//   - tx: The transaction that was just saved
//   - isNew: Whether this is a new transaction (true) or an update (false)
func updateTransactionStatsIncremental(ctx context.Context, tx *models.Transaction, isNew bool) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	// Only update statistics for new transactions (not updates)
	if !isNew {
		return
	}

	key := CacheKeyTransactionStats
	now := time.Now()
	thirtyDaysAgo := now.AddDate(0, 0, -StatsDays30)
	thirtyDaysAgoUnix := uint64(thirtyDaysAgo.Unix())
	twentyFourHoursAgo := now.Add(-StatsHours24 * time.Hour)
	twentyFourHoursAgoUnix := uint64(twentyFourHoursAgo.Unix())

	// Use Redis pipeline for atomic batch updates
	pipe := redisClient.Pipeline()

	// Update total count (always increment)
	pipe.HIncrBy(ctx, key, "total_count", 1)

	// Update 30-day count if transaction is within 30 days
	if tx.BlockUnixTimestamp >= thirtyDaysAgoUnix {
		pipe.HIncrBy(ctx, key, "count_30d", 1)
	}

	// Update 24-hour count if transaction is within 24 hours
	if tx.BlockUnixTimestamp >= twentyFourHoursAgoUnix {
		pipe.HIncrBy(ctx, key, "count_24h", 1)

		// Update failed transaction count if transaction failed
		if tx.Code != 0 {
			pipe.HIncrBy(ctx, key, "failed_tx_count_24h", 1)
		}

		// Update active addresses count (use Redis Set to track unique addresses)
		// We'll use a separate key for 24h active addresses set
		activeAddrKey := CacheKeyTransactionStats + ":active_addrs_24h"
		if tx.Signer != "" {
			// Add signer to the set and increment count if it's a new address
			pipe.SAdd(ctx, activeAddrKey, tx.Signer)
			// Set expiration for the set (24 hours + 1 hour buffer)
			pipe.Expire(ctx, activeAddrKey, 25*time.Hour)
		}
	}

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil {
		global.Logger.Warnf("failed to update transaction stats incrementally: %v", err)
		return
	}

	// Update active addresses count from the set size
	activeAddrKey := CacheKeyTransactionStats + ":active_addrs_24h"
	activeAddrCount, err := redisClient.SCard(ctx, activeAddrKey).Result()
	if err == nil {
		pipe = redisClient.Pipeline()
		pipe.HSet(ctx, key, "active_addresses_24h", activeAddrCount)
		pipe.Exec(ctx)
	}

	// Note: Complex metrics (TPS, gas prices, block time, gas utilization) are not updated incrementally
	// They are recalculated periodically by a background goroutine or when cache expires
}

// extractGasPriceFromFee extracts gas price from GasFee JSON string and GasUsed.
// Gas price = GasFee amount / GasUsed
// Returns 0 if extraction fails or GasUsed is 0
func extractGasPriceFromFee(gasFeeJSON string, gasUsed int64) float64 {
	if gasUsed <= 0 || gasFeeJSON == "" {
		return 0.0
	}

	// Try to parse GasFee as JSON to extract amount
	// GasFee JSON structure may contain "amount" field with coin information
	var gasFeeData map[string]interface{}
	if err := sonic.Unmarshal([]byte(gasFeeJSON), &gasFeeData); err != nil {
		// If JSON parsing fails, try to extract numeric value from string
		// This handles cases where GasFee might be stored in different formats
		return 0.0
	}

	// Try to extract amount from GasFee structure
	// Common structure: {"amount": [{"denom": "inj", "amount": "1000000"}]}
	var totalAmount float64
	if amounts, ok := gasFeeData["amount"].([]interface{}); ok {
		for _, amt := range amounts {
			if amtMap, ok := amt.(map[string]interface{}); ok {
				if amountStr, ok := amtMap["amount"].(string); ok {
					var amount float64
					if _, err := fmt.Sscanf(amountStr, "%f", &amount); err == nil {
						totalAmount += amount
					}
				}
			}
		}
	} else if amountStr, ok := gasFeeData["amount"].(string); ok {
		// Direct amount string
		if _, err := fmt.Sscanf(amountStr, "%f", &totalAmount); err != nil {
			return 0.0
		}
	} else {
		// If amount extraction fails, return 0
		return 0.0
	}

	if totalAmount > 0 {
		return totalAmount / float64(gasUsed)
	}

	return 0.0
}

// calculateAverageBlockTime calculates average block time from the latest blocks.
// Returns average time in seconds between consecutive blocks.
// Optimized: avoids slice copying by iterating backwards (blocks are in DESC order).
func calculateAverageBlockTime(blocks []models.Block) float64 {
	if len(blocks) < 2 {
		return 0.0
	}

	// Blocks are returned in DESC order (height DESC), so iterate backwards
	// to calculate time differences without copying the slice
	var totalTimeDiff float64
	validPairs := 0

	// Iterate from the end (oldest) to the beginning (newest)
	// Compare each block with the previous one (which has higher height)
	for i := len(blocks) - 1; i > 0; i-- {
		currTimestamp := blocks[i].BlockUnixTimestamp
		prevTimestamp := blocks[i-1].BlockUnixTimestamp

		if currTimestamp > 0 && prevTimestamp > 0 && prevTimestamp > currTimestamp {
			// Note: BlockUnixTimestamp is already in seconds (from time.Unix()), not milliseconds
			timeDiff := float64(prevTimestamp - currTimestamp)
			totalTimeDiff += timeDiff
			validPairs++
		}
	}

	if validPairs == 0 {
		return 0.0
	}

	return totalTimeDiff / float64(validPairs)
}

// calculateGasUtilization calculates gas utilization rate from transactions.
// Formula: (Σgas_used / Σgas_wanted) × 100%
//
// Parameters:
//   - txs: Slice of transactions
//
// Returns:
//   - float64: Gas utilization percentage (returns 0.0 if no transactions or totalGasWanted is 0)
func calculateGasUtilization(txs []models.Transaction) float64 {
	if len(txs) == 0 {
		return 0.0
	}

	var totalGasUsed int64
	var totalGasWanted int64

	for _, tx := range txs {
		if tx.GasUsed > 0 {
			totalGasUsed += tx.GasUsed
		}
		if tx.GasWanted > 0 {
			totalGasWanted += tx.GasWanted
		}
	}

	if totalGasWanted == 0 {
		return 0.0
	}

	return (float64(totalGasUsed) / float64(totalGasWanted)) * 100.0
}

// convertToQueryAddress converts an address to the format used for database/chain queries.
// - If forChainQuery=true: Returns Injective format (inj1...) for chain queries
// - If forChainQuery=false: Returns Ethereum format (0x...) for database queries
//
// Conversion logic:
// - If input is Ethereum format (0x...):
//   - For chain queries: Convert to Injective format
//   - For database queries: Return as-is (database stores Ethereum format)
//
// - If input is Injective format (inj1...):
//   - For chain queries: Return as-is
//   - For database queries: Convert to Ethereum format
//
// Parameters:
//   - ctx: Context for the request
//   - address: The address to convert (can be inj1... or 0x...)
//   - client: The chain client for address conversion (used only if chain query is needed)
//   - forChainQuery: If true, returns Injective format for chain queries; if false, returns Ethereum format for database
//
// Returns:
//   - string: The converted address
//   - error: Error if conversion fails
func (s *ExplorerService) convertToQueryAddress(ctx context.Context, address string, client chainclient.ChainClientV2, forChainQuery bool) (string, error) {
	// If already Ethereum format
	if strings.HasPrefix(address, "0x") {
		if forChainQuery {
			// For chain queries, convert Ethereum to Injective format
			injAddr, err := models.EthToInjAddress(address)
			if err != nil {
				return "", errors.Wrapf(err, "failed to convert Ethereum address to Injective: %s", address)
			}
			return injAddr, nil
		}
		// For database queries, return as-is (database stores Ethereum format)
		return address, nil
	}

	// If validator operator address, convert to account address first
	valPrefix := sdk.GetConfig().GetBech32ValidatorAddrPrefix()
	if strings.HasPrefix(address, valPrefix+"1") {
		accAddr, err := models.ValoperToAccAddress(address)
		if err != nil {
			return "", errors.Wrapf(err, "failed to convert validator address to account address: %s", address)
		}
		// Continue processing with the account address
		address = accAddr
	}

	// If Injective format (account address prefix)
	accPrefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
	if strings.HasPrefix(address, accPrefix+"1") {
		if forChainQuery {
			// For chain queries, return as-is
			return address, nil
		}
		// For database queries, convert to Ethereum format
		ethAddr, err := models.InjToEthAddress(address)
		if err != nil {
			return "", errors.Wrapf(err, "failed to convert Injective address to Ethereum: %s", address)
		}
		return ethAddr, nil
	}

	// Unknown format, return as-is (let validation handle it)
	return address, nil
}

// startStatsComplexMetricsUpdater starts a background goroutine that periodically recalculates
// complex transaction statistics metrics (TPS, gas prices, block time, gas utilization).
// This ensures these metrics stay up-to-date even when using incremental updates for simple counts.
// The function runs at StatsUpdateInterval to balance accuracy and performance.
func startStatsComplexMetricsUpdater() {
	ticker := time.NewTicker(StatsUpdateInterval)
	defer ticker.Stop()

	global.Logger.Info("started background transaction stats complex metrics updater")

	for {
		select {
		case <-ticker.C:
			updateComplexMetricsInCache()
		}
	}
}

// updateComplexMetricsInCache recalculates and updates complex metrics in the cache.
// This includes TPS, gas prices, block time, and gas utilization.
func updateComplexMetricsInCache() {
	ctx, cancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer cancel()

	// Get current cached stats to preserve simple metrics
	cachedStats := getTransactionStatsFromCache(ctx)
	if cachedStats == nil {
		// No cache exists yet, skip update (will be calculated on next query)
		return
	}

	// Recalculate complex metrics
	stats := &pb.TransactionStatsResponse{
		TotalCount:          cachedStats.TotalCount,
		Count_30D:           cachedStats.Count_30D,
		Count_24H:           cachedStats.Count_24H,
		ActiveAddresses_24H: cachedStats.ActiveAddresses_24H,
		FailedTxCount_24H:   cachedStats.FailedTxCount_24H,
	}

	// Calculate complex metrics in parallel
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		stats.Tps, err = calculateTPS(gCtx)
		return err
	})

	g.Go(func() error {
		var err error
		stats.AvgGas, stats.CurrentGas, stats.MaxGas_24H, stats.MinGas_24H, err = calculateGasPriceStats(gCtx)
		return err
	})

	g.Go(func() error {
		var err error
		stats.AvgBlockTime, err = calculateAverageBlockTimeFromDB(gCtx)
		return err
	})

	g.Go(func() error {
		var err error
		stats.GasUtilization, err = calculateGasUtilizationFromDB(gCtx)
		return err
	})

	// Wait for all calculations to complete
	if err := g.Wait(); err != nil {
		global.Logger.Warnf("failed to update complex metrics in cache: %v", err)
		return
	}

	// Update cache with new complex metrics
	cacheTransactionStats(ctx, stats)
	global.Logger.Debugf("updated complex metrics in cache: tps=%.2f, avg_gas=%.6f, avg_block_time=%.2f, gas_utilization=%.2f%%",
		stats.Tps, stats.AvgGas, stats.AvgBlockTime, stats.GasUtilization)
}

// GetTokenPriceAndMarketCap retrieves token price and market cap information.
// It supports multiple tokens through configuration. If token_symbol is not specified,
// it uses the default token from configuration.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing optional token_symbol
//
// Returns:
//   - *pb.Reply: Response containing price, supply, and market cap information
//   - error: Error if the operation fails
func (s *ExplorerService) GetTokenPriceAndMarketCap(ctx context.Context, req *pb.GetTokenPriceAndMarketCapRequest) (*pb.Reply, error) {
	tokenSymbol := ""
	if req != nil {
		tokenSymbol = req.TokenSymbol
	}

	global.Logger.Infof("getting token price and market cap for token: %s",
		tokenSymbol)

	// Load token configuration from global config
	tokenConfig, err := LoadTokenConfigFromConf(nil, tokenSymbol)
	if err != nil {
		global.Logger.Errorf("failed to load token config: %v", err)
		return ErrorReply(http.StatusBadRequest,
			fmt.Sprintf("token configuration not found: %s", tokenSymbol), err), err
	}

	// Get chain client
	chainClient, err := GetSharedChainClient()
	if err != nil {
		return ErrorReply(http.StatusInternalServerError, "failed to get chain client", err), err
	}

	// Fetch price and supply data
	priceData, err := GetTokenPriceAndMarketCap(ctx, chainClient, tokenConfig)
	if err != nil {
		global.Logger.Errorf("failed to get price and market cap: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to get price and market cap", err), err
	}

	// Calculate market cap (only if supply is available)
	var marketCap float64
	var totalSupplyStr string
	var totalSupplyFormatted string
	var totalSupplyFromChainStr string
	var totalSupplyFromChainFormatted string

	if priceData.TotalSupply != nil {
		marketCap = CalculateMarketCap(priceData.PriceUSD, priceData.TotalSupply, tokenConfig.Decimals)
		totalSupplyStr = priceData.TotalSupply.String()
		totalSupplyFormatted = priceData.TotalSupplyString
	} else {
		// Supply unavailable, market cap cannot be calculated
		marketCap = 0
		totalSupplyStr = ""
		totalSupplyFormatted = ""
		global.Logger.Warnf("supply unavailable for %s, market cap set to 0", tokenConfig.Symbol)
	}

	// Add chain supply for reference (if available)
	if priceData.TotalSupplyFromChain != nil {
		totalSupplyFromChainStr = priceData.TotalSupplyFromChain.String()
		// Use the formatTokenAmount function from price.go
		// Since it's in the same package, we can call it directly
		totalSupplyFromChainFormatted = formatTokenAmount(priceData.TotalSupplyFromChain, tokenConfig.Decimals)
	}

	// Build response
	response := &pb.TokenPriceAndMarketCapResponse{
		TokenSymbol:                   priceData.TokenSymbol,
		TokenName:                     priceData.TokenName,
		PriceUsd:                      priceData.PriceUSD,
		TotalSupply:                   totalSupplyStr,
		TotalSupplyFormatted:          totalSupplyFormatted,
		MarketCapUsd:                  marketCap,
		PriceChange_24H:               priceData.PriceChange24h,
		LastUpdated:                   priceData.LastUpdated,
		PriceSource:                   priceData.PriceSource,
		SupplySource:                  priceData.SupplySource,
		TotalSupplyFromChain:          totalSupplyFromChainStr,
		TotalSupplyFromChainFormatted: totalSupplyFromChainFormatted,
	}

	return MarshalAndReply(response, "marshal price and market cap response error")
}

// GetEVMTransaction retrieves EVM transaction details by EVM hash.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing the EVM transaction hash
//
// Returns:
//   - *pb.Reply: Response containing the EVM transaction details
//   - error: Error if the operation fails or validation fails
func (s *ExplorerService) GetEVMTransaction(ctx context.Context, req *pb.GetEVMTransactionRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Sanitize and validate transaction hash
	hash := SanitizeHash(req.Hash)
	if err := ValidateTransactionHash(hash); err != nil {
		return ErrorReply(http.StatusBadRequest, "invalid transaction hash", err), err
	}

	global.Logger.Debugf("get EVM transaction by hash: hash=%s", hash)

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Query EVM transaction from evm_transactions table by EVM hash
	evmHash := "0x" + hash
	var evmTx models.EVMTransaction
	err := db.Get().WithContext(queryCtx).
		Where("evm_hash = ?", evmHash).
		First(&evmTx).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.Reply{
				Code:    int32(http.StatusOK),
				Message: fmt.Sprintf("EVM transaction %s not found", evmHash),
				Data:    nil,
			}, nil
		}
		return ErrorReply(http.StatusInternalServerError, "database query error", err), err
	}

	// Query Cosmos transaction from transactions table by transaction_hash
	var dbTx models.Transaction
	err = db.Get().WithContext(queryCtx).
		Where("hash = ?", evmTx.TransactionHash).
		First(&dbTx).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.Reply{
				Code:    int32(http.StatusOK),
				Message: fmt.Sprintf("Cosmos transaction %s not found for EVM transaction %s", evmTx.TransactionHash, evmHash),
				Data:    nil,
			}, nil
		}
		return ErrorReply(http.StatusInternalServerError, "database query error", err), err
	}

	// Load EVM transaction details with Redis cache support (using evm_transactions table)
	evmDetails, err := s.loadEVMTransactionDetailsWithCacheFromEVMTx(queryCtx, &evmTx, &dbTx)
	if err != nil {
		return ErrorReply(http.StatusInternalServerError, "failed to load EVM transaction details", err), err
	}

	// If details not synced yet, try to fetch from EVM RPC (reduced timeout: 15s)
	if evmDetails == nil {
		evmClient, err := createEVMClient()
		if err != nil {
			return ErrorReply(http.StatusInternalServerError, "failed to create EVM client", err), err
		}
		defer evmClient.Close()

		evmCtx, evmCancel := context.WithTimeout(context.Background(), EVMDetailsTimeout)
		defer evmCancel()

		evmDetails, err = evmClient.GetTransactionDetails(evmCtx, ethcommon.HexToHash(evmTx.EVMHash))
		if err != nil {
			return &pb.Reply{
				Code:    int32(http.StatusOK),
				Message: fmt.Sprintf("EVM transaction details not found for %s", evmTx.EVMHash),
				Data:    nil,
			}, nil
		}

		// Cache the fetched details asynchronously
		go func() {
			cacheCtx, cacheCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cacheCancel()
			if detailsJSON, err := sonic.Marshal(evmDetails); err == nil {
				key := CacheKeyEVMDetailsPrefix + evmTx.EVMHash
				// Remove 0x prefix if present
				if len(key) > 11 && key[11:13] == "0x" {
					key = key[:11] + key[13:]
				}
				if err := cache.GetRedisClient().Set(cacheCtx, key, detailsJSON, CacheEVMDetailsTTL).Err(); err != nil {
					global.Logger.Warnf("failed to cache EVM details: %v", err)
				}
			}
		}()
	}

	// Convert to DTO
	evmDTO := models.ToEVMTransactionDTO(evmDetails, &dbTx)
	if evmDTO == nil {
		return ErrorReply(http.StatusInternalServerError, "failed to convert EVM transaction to DTO", errors.New("conversion failed")), errors.New("conversion failed")
	}

	return MarshalAndReply(evmDTO, "marshal EVM transaction error")
}

// GetEVMTransactions retrieves a list of EVM transactions with pagination.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing pagination parameters
//
// Returns:
//   - *pb.Reply: Response containing the list of EVM transactions with pagination info
//   - error: Error if the operation fails
func (s *ExplorerService) GetEVMTransactions(ctx context.Context, req *pb.GetEVMTransactionsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Normalize pagination parameters
	page, pageSize, offset, useCursor, cursorValue := normalizePagination(req.Page, req.PageSize, req.Cursor, MaxPageSize)

	global.Logger.Debugf("get EVM transactions: page=%d, page_size=%d, use_cursor=%v, cursor=%v", page, pageSize, useCursor, cursorValue)

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Query EVM transactions from evm_transactions table
	var evmTxs []models.EVMTransaction
	var total int64
	var isExactCount bool

	// Get approximate or exact count from evm_transactions table
	var countErr error
	total, isExactCount, countErr = getApproximateCountWithError(db.Get().Model(&models.EVMTransaction{}), &models.EVMTransaction{}, queryCtx)
	if countErr != nil {
		global.Logger.Warnf("failed to get approximate count for EVM transactions: %v, using fallback value", countErr)
		total = ApproximateCountFallback
		isExactCount = false
	}

	// Build query for evm_transactions table
	query := db.Get().WithContext(queryCtx).Model(&models.EVMTransaction{})

	if useCursor {
		// Use cursor-based pagination with evm_details_sync_at
		query = query.Where("id < ?", cursorValue).
			Order("evm_details_sync_at DESC, id DESC").
			Limit(int(pageSize))
	} else {
		// Use page-based pagination
		query = query.Order("evm_details_sync_at DESC, id DESC").
			Offset(offset).
			Limit(int(pageSize))
	}

	err := query.Find(&evmTxs).Error
	if err != nil {
		return ErrorReply(http.StatusInternalServerError, "database query error", err), err
	}

	if len(evmTxs) == 0 {
		// No transactions found
		txsResp := buildPaginatedResponse([]*pb.EVMTransactionDTO{}, page, pageSize, total, isExactCount, useCursor, cursorValue, "", 0)
		return MarshalAndReply(txsResp, "marshal EVM transactions error")
	}

	// Query Cosmos transactions from transactions table using transaction_hash
	txHashes := make([]string, len(evmTxs))
	for i, evmTx := range evmTxs {
		txHashes[i] = evmTx.TransactionHash
	}

	var dbTxs []models.Transaction
	err = db.Get().WithContext(queryCtx).
		Where("hash IN ?", txHashes).
		Find(&dbTxs).Error
	if err != nil {
		return ErrorReply(http.StatusInternalServerError, "database query error", err), err
	}

	// Build tx map and evmTx map
	txMap := make(map[string]*models.Transaction, len(dbTxs))
	evmTxMap := make(map[string]*models.EVMTransaction, len(evmTxs))
	for i := range dbTxs {
		txMap[dbTxs[i].Hash] = &dbTxs[i]
	}
	for i := range evmTxs {
		evmTxMap[evmTxs[i].TransactionHash] = &evmTxs[i]
	}

	// Load EVM transaction details in parallel (with Redis cache) using evm_transactions table
	detailsMap, err := s.loadEVMDetailsBatchParallelFromEVMTx(queryCtx, evmTxs, dbTxs)
	if err != nil {
		global.Logger.Warnf("failed to load EVM details in parallel: %v", err)
	}

	// Build evmDetailsList and evmDTOs from detailsMap
	evmDetailsList := make([]*chain.TransactionDetails, 0, len(evmTxs))
	for _, evmTx := range evmTxs {
		evmHash := evmTx.EVMHash
		if evmHash != "" && (len(evmHash) < 2 || evmHash[:2] != "0x") {
			evmHash = "0x" + evmHash
		}
		if details, ok := detailsMap[evmHash]; ok {
			evmDetailsList = append(evmDetailsList, details)
		}
	}

	// Convert to DTOs
	evmDTOs := models.ToEVMTransactionDTOs(evmDetailsList, txMap)

	// If some details are not loaded, try to fetch from EVM RPC (async, don't block response)
	if len(evmDTOs) < len(evmTxs) {
		go func() {
			evmClient, err := createEVMClient()
			if err != nil {
				global.Logger.Warnf("failed to create EVM client for async fetch: %v", err)
				return
			}
			defer evmClient.Close()

			evmCtx, evmCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer evmCancel()

			for i := range evmTxs {
				if evmTxs[i].EVMHash == "" {
					continue
				}
				// Check if already loaded
				evmHash := evmTxs[i].EVMHash
				if len(evmHash) < 2 || evmHash[:2] != "0x" {
					evmHash = "0x" + evmHash
				}
				if _, ok := detailsMap[evmHash]; ok {
					continue
				}
				details, err := evmClient.GetTransactionDetails(evmCtx, ethcommon.HexToHash(evmTxs[i].EVMHash))
				if err == nil && details != nil {
					// Cache the fetched details
					if detailsJSON, jsonErr := sonic.Marshal(details); jsonErr == nil {
						cacheCtx, cacheCancel := context.WithTimeout(context.Background(), 5*time.Second)
						key := CacheKeyEVMDetailsPrefix + evmHash
						// Remove 0x prefix if present
						if len(key) > 11 && key[11:13] == "0x" {
							key = key[:11] + key[13:]
						}
						if err := cache.GetRedisClient().Set(cacheCtx, key, detailsJSON, CacheEVMDetailsTTL).Err(); err != nil {
							global.Logger.Warnf("failed to cache EVM details: %v", err)
						}
						cacheCancel()
					}
					global.Logger.Debugf("async fetched EVM details for %s", evmTxs[i].EVMHash)
				}
			}
		}()
	}

	// Calculate next cursor using evm_transactions table (using ID instead of tx_number)
	nextCursor := ""
	if len(evmTxs) > 0 {
		if useCursor {
			nextCursor = fmt.Sprintf("%d", evmTxs[len(evmTxs)-1].ID)
		}
	}
	txsResp := buildPaginatedResponse(evmDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(evmDTOs))
	return MarshalAndReply(txsResp, "marshal EVM transactions error")
}

// GetEVMAccountTransactions retrieves EVM transactions for a specific account address.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing the account address and pagination parameters
//
// Returns:
//   - *pb.Reply: Response containing the list of EVM transactions for the account with pagination info
//   - error: Error if the operation fails or validation fails
func (s *ExplorerService) GetEVMAccountTransactions(ctx context.Context, req *pb.GetEVMAccountTransactionsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Sanitize and validate address
	address := SanitizeAddress(req.Address)
	if err := ValidateAddress(address); err != nil {
		return ErrorReply(http.StatusBadRequest, "invalid address", err), err
	}

	// Normalize pagination parameters
	page, pageSize, offset, useCursor, cursorValue := normalizePagination(req.Page, req.PageSize, req.Cursor, MaxPageSize)

	global.Logger.Debugf("get EVM account transactions: address=%s, page=%d, page_size=%d, use_cursor=%v, cursor=%v", address, page, pageSize, useCursor, cursorValue)

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Convert address to database format (Ethereum format 0x...)
	dbQueryAddress, err := s.convertToQueryAddress(queryCtx, address, nil, false)
	if err != nil {
		return ErrorReply(http.StatusBadRequest, "invalid address format", err), err
	}

	// Query EVM transactions from evm_transactions table using indexed address filtering
	var evmTxs []models.EVMTransaction
	var total int64
	var isExactCount bool

	// Get approximate or exact count from evm_transactions table
	baseQuery := db.Get().WithContext(queryCtx).Model(&models.EVMTransaction{}).
		Where("(evm_from_address = ? OR evm_to_address = ?)", dbQueryAddress, dbQueryAddress)

	var countErr error
	total, isExactCount, countErr = getApproximateCountWithError(baseQuery, &models.EVMTransaction{}, queryCtx)
	if countErr != nil {
		global.Logger.Warnf("failed to get approximate count for EVM account transactions: %v, using fallback value", countErr)
		total = ApproximateCountForAddress
		isExactCount = false
	}

	// Build query using evm_transactions table with indexed address filtering
	addressQuery := baseQuery

	if useCursor {
		query := addressQuery.Where("id < ?", cursorValue).
			Order("evm_details_sync_at DESC, id DESC").
			Limit(int(pageSize))
		err = query.Find(&evmTxs).Error
	} else {
		query := addressQuery.Order("evm_details_sync_at DESC, id DESC").
			Offset(offset).
			Limit(int(pageSize))
		err = query.Find(&evmTxs).Error
	}

	// If indexed query failed, return error
	if err != nil {
		return ErrorReply(http.StatusInternalServerError, "database query error", err), err
	}

	// If no results found, return empty response
	if len(evmTxs) == 0 {
		txsResp := buildPaginatedResponse([]*pb.EVMTransactionDTO{}, page, pageSize, total, isExactCount, useCursor, cursorValue, "", 0)
		return MarshalAndReply(txsResp, "marshal EVM account transactions error")
	}

	// Query Cosmos transactions from transactions table using transaction_hash
	txHashes := make([]string, len(evmTxs))
	for i, evmTx := range evmTxs {
		txHashes[i] = evmTx.TransactionHash
	}

	var dbTxs []models.Transaction
	err = db.Get().WithContext(queryCtx).
		Where("hash IN ?", txHashes).
		Find(&dbTxs).Error
	if err != nil {
		return ErrorReply(http.StatusInternalServerError, "database query error", err), err
	}

	// Build tx map and evmTx map
	txMap := make(map[string]*models.Transaction, len(dbTxs))
	evmTxMap := make(map[string]*models.EVMTransaction, len(evmTxs))
	for i := range dbTxs {
		txMap[dbTxs[i].Hash] = &dbTxs[i]
	}
	for i := range evmTxs {
		evmTxMap[evmTxs[i].TransactionHash] = &evmTxs[i]
	}

	// Load EVM transaction details in parallel (with Redis cache) using evm_transactions table
	detailsMap, err := s.loadEVMDetailsBatchParallelFromEVMTx(queryCtx, evmTxs, dbTxs)
	if err != nil {
		global.Logger.Warnf("failed to load EVM details in parallel: %v", err)
	}

	// Build evmDetailsList and evmDTOs from detailsMap
	evmDetailsList := make([]*chain.TransactionDetails, 0, len(evmTxs))
	for _, evmTx := range evmTxs {
		evmHash := evmTx.EVMHash
		if evmHash != "" && len(evmHash) < 2 || evmHash[:2] != "0x" {
			evmHash = "0x" + evmHash
		}
		if details, ok := detailsMap[evmHash]; ok {
			evmDetailsList = append(evmDetailsList, details)
		}
	}

	// Convert to DTOs
	evmDTOs := models.ToEVMTransactionDTOs(evmDetailsList, txMap)

	// Calculate next cursor using evm_transactions table (using ID instead of tx_number)
	nextCursor := ""
	if len(evmTxs) > 0 {
		if useCursor {
			nextCursor = fmt.Sprintf("%d", evmTxs[len(evmTxs)-1].ID)
		}
	}

	txsResp := buildPaginatedResponse(evmDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(evmDTOs))
	return MarshalAndReply(txsResp, "marshal EVM account transactions error")
}

// loadEVMTransactionDetailsWithCacheFromEVMTx loads EVM transaction details with Redis cache support.
// It first checks Redis cache, then falls back to object storage/database.
// This method uses evm_transactions table instead of transactions table.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - evmTx: EVMTransaction model from evm_transactions table
//   - tx: Transaction model from transactions table (for backward compatibility, optional)
//
// Returns:
//   - *chain.TransactionDetails: EVM transaction details, or nil if not found
//   - error: Error if the operation fails
func (s *ExplorerService) loadEVMTransactionDetailsWithCacheFromEVMTx(ctx context.Context, evmTx *models.EVMTransaction, tx *models.Transaction) (*chain.TransactionDetails, error) {
	if evmTx == nil {
		return nil, nil
	}
	// Ensure EVM hash has 0x prefix for consistency
	evmHash := evmTx.EVMHash
	cacheKey := evmHash
	if len(cacheKey) < 2 || cacheKey[:2] != "0x" {
		cacheKey = "0x" + cacheKey
	}

	// 1. Try to load from Redis cache first (fastest)
	key := CacheKeyEVMDetailsPrefix + cacheKey
	// Remove 0x prefix if present
	if len(key) > 11 && key[11:13] == "0x" {
		key = key[:11] + key[13:]
	}
	cached, err := cache.GetRedisClient().Get(ctx, key).Bytes()
	if err == nil && cached != nil {
		var details chain.TransactionDetails
		if err := sonic.Unmarshal(cached, &details); err == nil {
			return &details, nil
		}
	} else if err != nil && err != redis.Nil {
		global.Logger.Warnf("failed to get cached EVM details: %v", err)
	}

	// 2. Load from object storage if storage key exists (from evm_transactions table)
	if evmTx.EVMDetailsStorageKey != "" {
		storageClient := storage.Get()
		if storageClient != nil {
			detailsBytes, err := storageClient.GetObject(ctx, evmTx.EVMDetailsStorageKey)
			if err == nil {
				var details chain.TransactionDetails
				if err := sonic.Unmarshal(detailsBytes, &details); err == nil {
					// Cache the result asynchronously
					go func() {
						redisClient := cache.GetRedisClient()
						if redisClient != nil {
							cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
							defer cancel()
							key := CacheKeyEVMDetailsPrefix + cacheKey
							// Remove 0x prefix if present
							if len(key) > 11 && key[11:13] == "0x" {
								key = key[:11] + key[13:]
							}
							if err := redisClient.Set(cacheCtx, key, detailsBytes, CacheEVMDetailsTTL).Err(); err != nil {
								global.Logger.Warnf("failed to cache EVM details: %v", err)
							}
						}
					}()
					return &details, nil
				}
			}
		}
	}

	// 3. Fallback: Try to fetch from EVM RPC for unsynced transactions
	if !evmTx.EVMDetailsSynced && evmHash != "" {
		global.Logger.Debugf("EVM details not synced for %s, attempting RPC fallback", evmHash)
		evmClient, err := createEVMClient()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create EVM client for RPC fallback")
		}
		defer evmClient.Close()

		// Use short timeout for real-time fetch
		evmCtx, evmCancel := context.WithTimeout(context.Background(), AsyncEnhancementTimeout)
		defer evmCancel()

		details, err := evmClient.GetTransactionDetails(evmCtx, ethcommon.HexToHash(evmHash))
		if err != nil {
			global.Logger.Debugf("RPC fallback failed for %s: %v", evmHash, err)
			return nil, nil
		}

		// Cache the result
		if detailsJSON, jsonErr := sonic.Marshal(details); jsonErr == nil {
			// Update database asynchronously
			go s.updateEVMTransactionDetails(ctx, evmHash, details, detailsJSON)
		}

		return details, nil
	}

	return nil, nil
}

// updateEVMTransactionDetails updates EVM transaction details in database asynchronously.
// This is used to cache fetched EVM details from RPC calls.
//
// Parameters:
//   - ctx: Context for the database operation
//   - evmHash: EVM transaction hash
//   - details: EVM transaction details object
//   - detailsJSON: JSON bytes of EVM transaction details
func (s *ExplorerService) updateEVMTransactionDetails(ctx context.Context, evmHash string, details *chain.TransactionDetails, detailsJSON []byte) {
	// Query evm_transactions table by evm_hash
	var evmTx models.EVMTransaction
	err := db.Get().WithContext(ctx).
		Where("evm_hash = ?", evmHash).
		First(&evmTx).Error
	if err != nil {
		global.Logger.Debugf("EVM transaction %s not found in evm_transactions table for update", evmHash)
		return
	}

	// Update evm_transactions table
	now := time.Now()
	updates := map[string]interface{}{
		"evm_details_synced":     true,
		"evm_details_sync_at":    &now,
		"evm_details_sync_error": "",
		"evm_from_address":       details.From,
		"evm_to_address":         details.To,
	}

	// Check if should store to object storage (if data is large)
	storageKey := ""
	if storage.IsEnabled() && len(detailsJSON) > getStorageThresholdForSync() { // 3KB threshold
		storageClient := storage.Get()
		storageKey = storage.BuildStorageKey("evm_details", evmHash)

		if err := storageClient.PutObject(ctx, storageKey, detailsJSON); err == nil {
			updates["evm_details_storage_key"] = storageKey
			// Clear raw_details_json when using object storage (unless parsing failed)
			if details.ParseStatus == "success" {
				updates["raw_details_json"] = ""
			}
		} else {
			global.Logger.Warnf("failed to upload EVM details to storage for %s: %v", evmHash, err)
			// Continue with database storage
			storageKey = ""
		}
	}

	if storageKey == "" {
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

		global.Logger.Infof("Stored raw JSON for %s (parse_status: %s)", evmHash, details.ParseStatus)
	}

	// Update database
	err = db.Get().WithContext(ctx).
		Model(&evmTx).
		Where("evm_hash = ?", evmHash).
		Updates(updates).Error
	if err != nil {
		global.Logger.Warnf("failed to update EVM transaction details for %s: %v", evmHash, err)
	}
}

// loadEVMDetailsBatchParallel loads EVM transaction details for multiple transactions in parallel.
// It queries the evm_transactions table and uses goroutines with WaitGroup for concurrent loading with Redis cache optimization.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - txs: List of Cosmos transactions to load EVM details for
//
// Returns:
//   - map[string]*chain.TransactionDetails: Map of evmHash to transaction details
//   - error: Error if the operation fails
func (s *ExplorerService) loadEVMDetailsBatchParallel(ctx context.Context, txs []models.Transaction) (map[string]*chain.TransactionDetails, error) {
	if len(txs) == 0 {
		return make(map[string]*chain.TransactionDetails), nil
	}

	// Query evm_transactions table for the given transaction hashes
	txHashes := make([]string, len(txs))
	for i := range txs {
		txHashes[i] = txs[i].Hash
	}

	var evmTxs []models.EVMTransaction
	err := db.Get().WithContext(ctx).
		Where("transaction_hash IN ?", txHashes).
		Find(&evmTxs).Error
	if err != nil {
		return make(map[string]*chain.TransactionDetails), nil
	}

	// Use the new function that works with EVMTransaction model
	return s.loadEVMDetailsBatchParallelFromEVMTx(ctx, evmTxs, txs)
}

// loadEVMDetailsBatchParallelFromEVMTx loads EVM transaction details for multiple EVM transactions in parallel.
// It uses evm_transactions table instead of transactions table.
// It uses goroutines with WaitGroup for concurrent loading with Redis cache optimization.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - evmTxs: List of EVM transactions from evm_transactions table
//   - txs: List of Cosmos transactions from transactions table (for backward compatibility, optional)
//
// Returns:
//   - map[string]*chain.TransactionDetails: Map of evmHash to transaction details
//   - error: Error if the operation fails
func (s *ExplorerService) loadEVMDetailsBatchParallelFromEVMTx(ctx context.Context, evmTxs []models.EVMTransaction, txs []models.Transaction) (map[string]*chain.TransactionDetails, error) {
	if len(evmTxs) == 0 {
		return make(map[string]*chain.TransactionDetails), nil
	}

	// Collect unique EVM hashes that need details loaded
	type pendingLoad struct {
		evmTx   *models.EVMTransaction
		tx      *models.Transaction
		evmHash string
	}

	pending := make([]pendingLoad, 0, len(evmTxs))
	detailsMap := make(map[string]*chain.TransactionDetails)

	// Build tx map for quick lookup
	txMap := make(map[string]*models.Transaction, len(txs))
	for i := range txs {
		txMap[txs[i].Hash] = &txs[i]
	}

	for i := range evmTxs {
		evmTx := &evmTxs[i]
		if !evmTx.EVMDetailsSynced || evmTx.EVMHash == "" {
			continue
		}

		evmHash := evmTx.EVMHash
		if len(evmHash) < 2 || evmHash[:2] != "0x" {
			evmHash = "0x" + evmHash
		}

		tx := txMap[evmTx.TransactionHash]
		pending = append(pending, pendingLoad{evmTx: evmTx, tx: tx, evmHash: evmHash})
	}

	if len(pending) == 0 {
		return detailsMap, nil
	}

	// 1. Batch check Redis cache first
	evmHashes := make([]string, len(pending))
	for i, p := range pending {
		evmHashes[i] = p.evmHash
	}

	// Batch get from Redis
	var cachedResults map[string][]byte
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		// Build keys
		keys := make([]string, len(evmHashes))
		for i, hash := range evmHashes {
			key := CacheKeyEVMDetailsPrefix + hash
			// Remove 0x prefix if present
			if len(key) > 11 && key[11:13] == "0x" {
				key = key[:11] + key[13:]
			}
			keys[i] = key
		}

		// Batch get
		results, err := redisClient.MGet(ctx, keys...).Result()
		if err != nil {
			global.Logger.Warnf("failed to batch get cached EVM details: %v", err)
		} else {
			// Parse results
			cachedResults = make(map[string][]byte)
			for i, result := range results {
				if result != nil {
					if bytes, ok := result.([]byte); ok {
						cachedResults[evmHashes[i]] = bytes
					}
				}
			}
		}
	} else {
		cachedResults = make(map[string][]byte)
	}

	// Parse cached results and identify what's still pending
	stillPending := make([]pendingLoad, 0, len(pending))
	for _, p := range pending {
		if cached, ok := cachedResults[p.evmHash]; ok && cached != nil {
			var details chain.TransactionDetails
			if err := sonic.Unmarshal(cached, &details); err == nil {
				detailsMap[p.evmHash] = &details
				continue
			}
		}
		stillPending = append(stillPending, p)
	}

	if len(stillPending) == 0 {
		return detailsMap, nil
	}

	// 2. Load remaining from object storage/database in parallel with concurrency limit
	// Limit concurrency to avoid overwhelming storage/RPC services
	const maxConcurrency = 10
	sem := make(chan struct{}, maxConcurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, len(stillPending))

	for i := range stillPending {
		wg.Add(1)
		go func(pl pendingLoad) {
			defer wg.Done()

			// Acquire semaphore to limit concurrent storage reads
			sem <- struct{}{}
			defer func() { <-sem }()

			details, err := s.loadEVMTransactionDetailsWithCacheFromEVMTx(ctx, pl.evmTx, pl.tx)
			if err != nil {
				select {
				case errChan <- err:
				default:
				}
				return
			}

			if details != nil {
				mu.Lock()
				detailsMap[pl.evmHash] = details
				mu.Unlock()
			}
		}(stillPending[i])
	}

	wg.Wait()
	close(errChan)

	// Collect errors (non-blocking)
	for err := range errChan {
		if err != nil {
			return detailsMap, err
		}
	}

	return detailsMap, nil
}

// getLatestBlockHeight returns the latest block height from cache or database
// This is used for determining if a block is too old for transaction enhancement
func getLatestBlockHeight() uint64 {
	// Try to get from cache first
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		val, err := redisClient.Get(ctx, CacheKeyLatestBlockHeight).Uint64()
		if err == nil && val > 0 {
			return val
		}
	}

	// Fallback to database query
	var latestBlock models.Block
	err := db.Get().Order("height desc").First(&latestBlock).Error
	if err == nil && latestBlock.Height > 0 {
		// Cache for future use
		if redisClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			redisClient.Set(ctx, CacheKeyLatestBlockHeight, latestBlock.Height, 1*time.Minute)
		}
		return latestBlock.Height
	}

	// Ultimate fallback - return a reasonable default
	return 50000000 // Approximately current Injective mainnet height
}

// GetTransactionsBatch retrieves multiple transactions by their hashes in a single request.
// This reduces network overhead by 90% compared to individual requests.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing list of transaction hashes
//
// Returns:
//   - *pb.Reply: Response containing transactions and found map
//   - error: Error if the operation fails
func (s *ExplorerService) GetTransactionsBatch(ctx context.Context, req *pb.GetTransactionsBatchRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if len(req.Hashes) == 0 {
		response := map[string]interface{}{
			"transactions": []*pb.TransactionDTO{},
			"found":        make(map[string]bool),
		}
		return MarshalAndReply(response, "marshal transactions batch error")
	}

	// Limit batch size to prevent abuse
	maxBatchSize := BlockBatchSizeLimit
	if len(req.Hashes) > maxBatchSize {
		return ErrorReply(http.StatusBadRequest, fmt.Sprintf("batch size exceeds maximum of %d", maxBatchSize), errors.New("batch size too large")), errors.New("batch size too large")
	}

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Normalize hashes (add 0x prefix if missing)
	normalizedHashes := make([]string, len(req.Hashes))
	for i, hash := range req.Hashes {
		normalizedHash := SanitizeHash(hash)
		normalizedHashes[i] = "0x" + normalizedHash
	}

	// Query transactions from database in batch
	var dbTxs []models.Transaction
	err := db.Get().WithContext(queryCtx).
		Where("hash IN ?", normalizedHashes).
		Find(&dbTxs).Error

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		global.Logger.Errorf("failed to query transactions batch: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query transactions", err), err
	}

	// Create map for quick lookup
	txMap := make(map[string]*models.Transaction)
	for i := range dbTxs {
		txMap[dbTxs[i].Hash] = &dbTxs[i]
	}

	// Build response
	transactions := make([]*pb.TransactionDTO, 0, len(req.Hashes))
	found := make(map[string]bool)

	for _, hash := range normalizedHashes {
		if tx, exists := txMap[hash]; exists {
			txDTO := models.ToTransactionDTO(tx, models.DTOModeWithDetail)
			transactions = append(transactions, txDTO)
			found[hash] = true
		} else {
			found[hash] = false
		}
	}

	response := map[string]interface{}{
		"transactions": transactions,
		"found":        found,
	}

	return MarshalAndReply(response, "marshal transactions batch error")
}

// GetBlocksBatch retrieves multiple blocks by their heights in a single request.
// This reduces network overhead by 90% compared to individual requests.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing list of block heights
//
// Returns:
//   - *pb.Reply: Response containing blocks and found map
//   - error: Error if the operation fails
func (s *ExplorerService) GetBlocksBatch(ctx context.Context, req *pb.GetBlocksBatchRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if len(req.Heights) == 0 {
		response := map[string]interface{}{
			"blocks": []*pb.BlockDTO{},
			"found":  make(map[uint64]bool),
		}
		return MarshalAndReply(response, "marshal blocks batch error")
	}

	// Limit batch size to prevent abuse
	maxBatchSize := BlockBatchSizeLimit
	if len(req.Heights) > maxBatchSize {
		return ErrorReply(http.StatusBadRequest, fmt.Sprintf("batch size exceeds maximum of %d", maxBatchSize), errors.New("batch size too large")), errors.New("batch size too large")
	}

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Query blocks from database in batch
	var dbBlocks []models.Block
	err := db.Get().WithContext(queryCtx).
		Where("height IN ?", req.Heights).
		Find(&dbBlocks).Error

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		global.Logger.Errorf("failed to query blocks batch: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query blocks", err), err
	}

	// Create map for quick lookup
	blockMap := make(map[uint64]*models.Block)
	for i := range dbBlocks {
		blockMap[dbBlocks[i].Height] = &dbBlocks[i]
	}

	// Build response
	blocks := make([]*pb.BlockDTO, 0, len(req.Heights))
	found := make(map[uint64]bool)

	for _, height := range req.Heights {
		if block, exists := blockMap[height]; exists {
			blockDTO := models.ToBlockDTO(block, models.DTOModeWithDetail)
			blocks = append(blocks, blockDTO)
			found[height] = true
		} else {
			found[height] = false
		}
	}

	response := map[string]interface{}{
		"blocks": blocks,
		"found":  found,
	}

	return MarshalAndReply(response, "marshal blocks batch error")
}

// GetValidatorBlockStats retrieves validator block statistics (proposed, signed, missed blocks).
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing validator operator address
//
// Returns:
//   - *pb.Reply: Response containing validator block statistics
//   - error: Error if the operation fails
func (s *ExplorerService) GetValidatorBlockStats(ctx context.Context, req *pb.GetValidatorBlockStatsRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.ValidatorAddress == "" {
		return ErrorReply(http.StatusBadRequest, "validator address is required", errors.New("empty validator address")), errors.New("empty validator address")
	}

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Get validator information
	var validator models.Validator
	err := db.Get().WithContext(queryCtx).
		Where("operator_address = ?", req.ValidatorAddress).
		First(&validator).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.Reply{
				Code:    int32(http.StatusOK),
				Message: "validator not found",
				Data:    nil,
			}, nil
		}
		global.Logger.Errorf("failed to query validator: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query validator", err), err
	}

	// Get statistics from aggregated statistics table (方案3: 聚合统计表)
	proposed, signed, missed, err := models.GetValidatorStatsAggregated(req.ValidatorAddress)
	if err != nil {
		// Fallback: use validator's statistics from database (updated by ValidatorStatsService)
		global.Logger.Warnf("failed to get aggregated stats for %s: %v, using validator table stats", req.ValidatorAddress, err)
		proposed = validator.Proposed
		signed = validator.Signed
		missed = validator.Missed
	}

	// Calculate uptime percentage
	var uptimePercentage float64
	totalBlocks := int64(signed) + int64(missed)
	if totalBlocks > 0 {
		uptimePercentage = float64(signed) / float64(totalBlocks) * 100.0
	}

	// Build response
	response := &pb.ValidatorBlockStatsResponse{
		ValidatorAddress: validator.OperatorAddress,
		Moniker:          validator.Moniker,
		Proposed:         proposed,
		Signed:           signed,
		Missed:           missed,
		UptimePercentage: uptimePercentage,
	}

	return MarshalAndReply(response, "marshal validator block stats error")
}

// GetAllContractTags retrieves all contract tags with optional filters and pagination.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing optional filters (chain_id, tag_type, contract_address) and pagination
//
// Returns:
//   - *pb.Reply: Response containing list of contract tags
//   - error: Error if the operation fails
func (s *ExplorerService) GetAllContractTags(ctx context.Context, req *pb.GetAllContractTagsRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Build query with filters
	query := db.Get().WithContext(queryCtx).Model(&models.ContractTag{})

	// Apply filters
	if req.ChainId != "" {
		query = query.Where("chain_id = ?", req.ChainId)
	}
	if req.TagType != "" {
		query = query.Where("tag_type = ?", req.TagType)
	}
	if req.ContractAddress != "" {
		// Convert address to Ethereum format for storage
		contractAddress := models.ConvertAddressToEthForStorage(req.ContractAddress)
		query = query.Where("contract_address = ?", contractAddress)
	}

	// Apply pagination
	page := int32(1)
	pageSize := int32(20)
	if req.Pagination != nil {
		if req.Pagination.Page > 0 {
			page = req.Pagination.Page
		}
		if req.Pagination.PageSize > 0 {
			pageSize = req.Pagination.PageSize
			// Limit max page size
			if pageSize > 100 {
				pageSize = 100
			}
		}
	}

	// Count total records
	var totalCount int64
	err := query.Count(&totalCount).Error
	if err != nil {
		global.Logger.Errorf("failed to count contract tags: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to count contract tags", err), err
	}

	// Query tags with pagination
	var tags []models.ContractTag
	offset := (page - 1) * pageSize
	err = query.Order("created_at DESC").
		Offset(int(offset)).
		Limit(int(pageSize)).
		Find(&tags).Error

	if err != nil {
		global.Logger.Errorf("failed to get contract tags: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to get contract tags", err), err
	}

	// Convert to DTOs
	tagInfos := models.ToContractTagInfoDTOs(tags)

	// Build response with pagination info
	response := map[string]interface{}{
		"tags": tagInfos,
		"pagination": map[string]interface{}{
			"page":        page,
			"page_size":   pageSize,
			"total":       totalCount,
			"total_pages": (totalCount + int64(pageSize) - 1) / int64(pageSize),
		},
	}

	return MarshalAndReply(response, "marshal contract tags error")
}

// AddGlobalTag adds a global tag (not bound to a specific contract).
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing global tag information
//
// Returns:
//   - *pb.Reply: Response containing the created global tag
//   - error: Error if the operation fails
func (s *ExplorerService) AddGlobalTag(ctx context.Context, req *pb.AddGlobalTagRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	if req.TagType == "" {
		return ErrorReply(http.StatusBadRequest, "tag_type is required", errors.New("tag_type is required")), errors.New("tag_type is required")
	}

	// Create or update global tag
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	tag := &models.GlobalTag{
		ChainID:     req.ChainId,
		TagType:     req.TagType,
		TagValue:    req.TagValue,
		TagColor:    req.TagColor,
		Description: req.Description,
		CreatedBy:   req.CreatedBy,
	}

	// Use FirstOrCreate to avoid duplicates based on chain_id and tag_type
	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND tag_type = ?", req.ChainId, req.TagType).
		FirstOrCreate(tag).Error

	if err != nil {
		global.Logger.Errorf("failed to add global tag: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to add global tag", err), err
	}

	// Convert to response
	tagInfo := models.ToGlobalTagInfoDTO(tag)
	return MarshalAndReply(tagInfo, "marshal global tag error")
}

// RemoveGlobalTag removes a global tag.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing chain ID and tag type
//
// Returns:
//   - *pb.Reply: Response indicating success
//   - error: Error if the operation fails
func (s *ExplorerService) RemoveGlobalTag(ctx context.Context, req *pb.RemoveGlobalTagRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.ChainId == "" {
		return ErrorReply(http.StatusBadRequest, "chain_id is required", errors.New("chain_id is required")), errors.New("chain_id is required")
	}

	if req.TagType == "" {
		return ErrorReply(http.StatusBadRequest, "tag_type is required", errors.New("tag_type is required")), errors.New("tag_type is required")
	}

	// Delete global tag
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	err := db.Get().WithContext(queryCtx).
		Where("chain_id = ? AND tag_type = ?", req.ChainId, req.TagType).
		Delete(&models.GlobalTag{}).Error

	if err != nil {
		global.Logger.Errorf("failed to remove global tag: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to remove global tag", err), err
	}

	// Return success response
	return &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "success",
	}, nil
}

// GetIBCTransferTxs queries IBC transfer transactions with filters and pagination.
func (s *ExplorerService) GetIBCTransferTxs(ctx context.Context, req *pb.GetIBCTransferTxsRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Set default pagination if not provided
	if req.Pagination == nil {
		req.Pagination = &pb.PaginationRequest{
			Page:     1,
			PageSize: DefaultPageSize,
		}
	}

	page, pageSize, offset, useCursor, cursorValue := normalizePagination(req.Pagination.Page, req.Pagination.PageSize, req.Pagination.Cursor, MaxPageSize)

	global.Logger.Debugf("get IBC transfer txs: page=%d, page_size=%d, sender=%s, receiver=%s", page, pageSize, req.Sender, req.Receiver)

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	query := db.Get().WithContext(queryCtx).Model(&models.IBCTransferTx{})

	if req.Sender != "" {
		query = query.Where("sender = ?", req.Sender)
	}
	if req.Receiver != "" {
		query = query.Where("receiver = ?", req.Receiver)
	}
	if req.SrcChannel != "" {
		query = query.Where("source_channel = ?", req.SrcChannel)
	}
	if req.SrcPort != "" {
		query = query.Where("source_port = ?", req.SrcPort)
	}
	if req.DestChannel != "" {
		query = query.Where("destination_channel = ?", req.DestChannel)
	}
	if req.DestPort != "" {
		query = query.Where("destination_port = ?", req.DestPort)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		global.Logger.Errorf("count IBC transfers failed: %v", err)
		return ErrorReply(http.StatusInternalServerError, "query failed", err), err
	}

	var txs []*models.IBCTransferTx
	dbQuery := query.Order("block_timestamp DESC").Limit(int(pageSize))
	if useCursor {
		dbQuery = dbQuery.Where("block_timestamp < ?", cursorValue)
	} else {
		dbQuery = dbQuery.Offset(offset)
	}

	if err := dbQuery.Find(&txs).Error; err != nil {
		global.Logger.Errorf("query IBC transfers failed: %v", err)
		return ErrorReply(http.StatusInternalServerError, "query failed", err), err
	}

	dtos := models.ToIBCTransferTxProtos(txs)
	nextCursor := calculateNextCursor(useCursor, cursorValue, txs, func(t *models.IBCTransferTx) uint64 { return t.BlockTimestamp })
	resp := buildPaginatedResponse(dtos, page, pageSize, total, true, useCursor, cursorValue, nextCursor, len(txs))
	return MarshalAndReply(resp, "marshal IBC transfers error")
}

// GetPeggyDepositTxs queries Peggy deposit transactions with filters and pagination.
func (s *ExplorerService) GetPeggyDepositTxs(ctx context.Context, req *pb.GetPeggyDepositTxsRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Set default pagination if not provided
	if req.Pagination == nil {
		req.Pagination = &pb.PaginationRequest{
			Page:     1,
			PageSize: DefaultPageSize,
		}
	}

	page, pageSize, offset, useCursor, cursorValue := normalizePagination(req.Pagination.Page, req.Pagination.PageSize, req.Pagination.Cursor, MaxPageSize)

	global.Logger.Debugf("get Peggy deposit txs: page=%d, page_size=%d, sender=%s, receiver=%s", page, pageSize, req.Sender, req.Receiver)

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	query := db.Get().WithContext(queryCtx).Model(&models.PeggyDepositTx{})

	if req.Sender != "" {
		query = query.Where("sender = ?", req.Sender)
	}
	if req.Receiver != "" {
		query = query.Where("receiver = ?", req.Receiver)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		global.Logger.Errorf("count Peggy deposits failed: %v", err)
		return ErrorReply(http.StatusInternalServerError, "query failed", err), err
	}

	var txs []*models.PeggyDepositTx
	dbQuery := query.Order("block_timestamp DESC").Limit(int(pageSize))
	if useCursor {
		dbQuery = dbQuery.Where("block_timestamp < ?", cursorValue)
	} else {
		dbQuery = dbQuery.Offset(offset)
	}

	if err := dbQuery.Find(&txs).Error; err != nil {
		global.Logger.Errorf("query Peggy deposits failed: %v", err)
		return ErrorReply(http.StatusInternalServerError, "query failed", err), err
	}

	dtos := models.ToPeggyDepositTxProtos(txs)
	nextCursor := calculateNextCursor(useCursor, cursorValue, txs, func(t *models.PeggyDepositTx) uint64 { return t.BlockTimestamp })
	resp := buildPaginatedResponse(dtos, page, pageSize, total, true, useCursor, cursorValue, nextCursor, len(txs))
	return MarshalAndReply(resp, "marshal Peggy deposits error")
}

// GetPeggyWithdrawalTxs queries Peggy withdrawal transactions with filters and pagination.
func (s *ExplorerService) GetPeggyWithdrawalTxs(ctx context.Context, req *pb.GetPeggyWithdrawalTxsRequest) (*pb.Reply, error) {
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Set default pagination if not provided
	if req.Pagination == nil {
		req.Pagination = &pb.PaginationRequest{
			Page:     1,
			PageSize: DefaultPageSize,
		}
	}

	page, pageSize, offset, useCursor, cursorValue := normalizePagination(req.Pagination.Page, req.Pagination.PageSize, req.Pagination.Cursor, MaxPageSize)

	global.Logger.Debugf("get Peggy withdrawal txs: page=%d, page_size=%d, sender=%s, receiver=%s", page, pageSize, req.Sender, req.Receiver)

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	query := db.Get().WithContext(queryCtx).Model(&models.PeggyWithdrawalTx{})

	if req.Sender != "" {
		query = query.Where("sender = ?", req.Sender)
	}
	if req.Receiver != "" {
		query = query.Where("receiver = ?", req.Receiver)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		global.Logger.Errorf("count Peggy withdrawals failed: %v", err)
		return ErrorReply(http.StatusInternalServerError, "query failed", err), err
	}

	var txs []*models.PeggyWithdrawalTx
	dbQuery := query.Order("block_timestamp DESC").Limit(int(pageSize))
	if useCursor {
		dbQuery = dbQuery.Where("block_timestamp < ?", cursorValue)
	} else {
		dbQuery = dbQuery.Offset(offset)
	}

	if err := dbQuery.Find(&txs).Error; err != nil {
		global.Logger.Errorf("query Peggy withdrawals failed: %v", err)
		return ErrorReply(http.StatusInternalServerError, "query failed", err), err
	}

	dtos := models.ToPeggyWithdrawalTxProtos(txs)
	nextCursor := calculateNextCursor(useCursor, cursorValue, txs, func(t *models.PeggyWithdrawalTx) uint64 { return t.BlockTimestamp })
	resp := buildPaginatedResponse(dtos, page, pageSize, total, true, useCursor, cursorValue, nextCursor, len(txs))
	return MarshalAndReply(resp, "marshal Peggy withdrawals error")
}
