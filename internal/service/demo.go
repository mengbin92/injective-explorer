// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"

	"github.com/InjectiveLabs/sdk-go/chain/types"
	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/gorm"
)

// DemoService implements the explorer API service.
// It provides methods to query blockchain data including blocks, transactions, and account information.
//
// The service maintains client connections for better performance,
// creating them lazily when needed and reusing them across requests.
type DemoService struct {
	pb.UnimplementedDemoServer
}

// NewDemoService creates a new instance of DemoService.
//
// Returns:
//   - *DemoService: A new service instance
func NewDemoService() *DemoService {
	return &DemoService{}
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
func (s *DemoService) GetLatestBlocks(ctx context.Context, req *pb.PaginationRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return s.errorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Normalize pagination parameters
	page, pageSize, offset, useCursor, cursorValue := normalizePagination(req.Page, req.PageSize, req.Cursor, MaxPageSize)

	global.Logger.Infof("get latest blocks: page=%d, page_size=%d, use_cursor=%v, cursor=%v", page, pageSize, useCursor, cursorValue)

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// First, try to query from Redis cache (only for first page without cursor)
	if !useCursor && page == MinPage {
		cachedBlocks, err := getBlocksFromCache(queryCtx, int(pageSize))
		if err != nil {
			// Log cache error but continue to database query (graceful degradation)
			global.Logger.Warnf("failed to get blocks from cache, fallback to DB: %v", err)
		} else if len(cachedBlocks) > 0 {
			global.Logger.Infof("blocks found in cache: count=%d", len(cachedBlocks))
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
					cacheTotal(queryCtx, CacheKeyTotalBlocks, total)
				}
			}
			nextCursor := calculateNextCursor(useCursor, cursorValue, cachedBlocks, func(b models.Block) uint64 { return b.Height })
			blockDTOs := models.ToBlockDTOs(cachedBlocks)
			blocksResp := buildPaginatedResponse(blockDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(cachedBlocks))
			return s.marshalAndReply(blocksResp, "marshal blocks error")
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
		cacheTotal(queryCtx, CacheKeyTotalBlocks, total)
	}

	// Build query based on pagination type
	query := db.Get().WithContext(queryCtx).Model(&models.Block{})

	if useCursor {
		// Use cursor-based pagination (more efficient for large datasets)
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
		global.Logger.Infof("blocks found in database: count=%d, query_time=%v", len(dbBlocks), queryDuration)

		if len(dbBlocks) > 0 {
			nextCursor := calculateNextCursor(useCursor, cursorValue, dbBlocks, func(b models.Block) uint64 { return b.Height })
			blockDTOs := models.ToBlockDTOs(dbBlocks)
			blocksResp := buildPaginatedResponse(blockDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(dbBlocks))
			return s.marshalAndReply(blocksResp, "marshal blocks error")
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: error=%v", err)
	}

	// Not found in database or conversion failed, query from chain
	global.Logger.Infof("blocks not found in database or conversion failed, querying from chain")
	client, err := GetSharedExplorerClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
	}

	// Use independent context for RPC calls to avoid HTTP timeout issues
	callCtx, cancel := context.WithTimeout(context.Background(), GRPCTimeout)
	defer cancel()

	blocks, err := client.GetBlocks(callCtx)
	if err != nil {
		global.Logger.Errorf("get blocks error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get blocks error", err), err
	}

	// Save to database and build response with pagination
	dbBlocks = dbBlocks[:0] // Reset slice
	for _, block := range blocks.Data {
		blockDetail, err := client.GetBlock(callCtx, fmt.Sprintf("%d", block.Height))
		if err != nil {
			global.Logger.Errorf("get block error: %v", err)
			continue
		}

		dbBlock := models.BlockFromProto(blockDetail.Data)
		saveToDB(dbBlock, "create block error")
		// Cache the block
		cacheBlock(callCtx, dbBlock)
		dbBlocks = append(dbBlocks, *dbBlock)
	}

	// Apply pagination to results
	if len(dbBlocks) > int(pageSize) {
		dbBlocks = dbBlocks[:pageSize]
	}

	nextCursor := calculateNextCursor(useCursor, cursorValue, dbBlocks, func(b models.Block) uint64 { return b.Height })
	blockDTOs := models.ToBlockDTOs(dbBlocks)
	blocksResp := buildPaginatedResponse(blockDTOs, page, pageSize, int64(len(dbBlocks)), true, useCursor, cursorValue, nextCursor, len(dbBlocks))
	return s.marshalAndReply(blocksResp, "marshal blocks error")
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
func (s *DemoService) GetBlockByHeight(ctx context.Context, req *pb.GetBlockByHeightRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return s.errorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Validate block height
	if err := ValidateBlockHeight(req.Height); err != nil {
		return s.errorReply(http.StatusBadRequest, "invalid block height", err), err
	}

	global.Logger.Infof("get block by height: height=%d", req.Height)

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// First, try to query from Redis cache
	cachedBlock, err := getBlockFromCache(queryCtx, uint64(req.Height))
	if err == nil && cachedBlock != nil {
		global.Logger.Infof("block found in cache: height=%d", req.Height)
		blockDTO := models.ToBlockDTO(cachedBlock, models.DTOModeWithDetail)
		return s.marshalAndReply(blockDTO, "marshal block error")
	}

	// Second, try to query from database
	var dbBlock models.Block
	startTime := time.Now()
	err = db.Get().WithContext(queryCtx).Where("height = ?", req.Height).First(&dbBlock).Error
	queryDuration := time.Since(startTime)

	if err == nil {
		// Found in database, convert to DTO and return
		global.Logger.Infof("block found in database: height=%d, query_time=%v", req.Height, queryDuration)

		// Cache the block for future queries (async to avoid blocking)
		go cacheBlock(context.Background(), &dbBlock)

		blockDTO := models.ToBlockDTO(&dbBlock, models.DTOModeWithDetail)
		return s.marshalAndReply(blockDTO, "marshal block error")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: height=%d, error=%v, query_time=%v", req.Height, err, queryDuration)
	} else {
		global.Logger.Infof("block not found in database: height=%d, query_time=%v", req.Height, queryDuration)
	}

	// Not found in database or conversion failed, query from chain
	// Only query from chain if block is reasonably recent (within last N blocks)
	// For very old blocks, return error instead of waiting for slow chain query
	global.Logger.Infof("block not found in database, querying from chain: height=%d", req.Height)
	client, err := GetSharedExplorerClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
	}

	// Use independent context for RPC calls to avoid HTTP timeout issues
	callCtx, cancel := context.WithTimeout(context.Background(), GRPCTimeout)
	defer cancel()

	block, err := client.GetBlock(callCtx, fmt.Sprintf("%d", req.Height))
	if err != nil {
		global.Logger.Errorf("get block error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get block error", err), err
	}

	// Save to database for future queries
	blockModel := models.BlockFromProto(block.Data)
	saveToDB(blockModel, "create block error")

	// Cache the block
	cacheBlock(callCtx, blockModel)

	blockDTO := models.ToBlockDTO(blockModel, models.DTOModeWithDetail)
	return s.marshalAndReply(blockDTO, "marshal block error")
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
func (s *DemoService) GetLatestBlockHeight(ctx context.Context, req *emptypb.Empty) (*pb.Reply, error) {
	global.Logger.Infof("get latest block height")

	client := GetSharedTmClient()
	height, err := client.GetLatestBlockHeight(ctx)
	if err != nil {
		global.Logger.Errorf("get latest block height error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get latest block height error", err), err
	}

	return s.marshalAndReply(&pb.LatestBlockHeightResponse{Height: height}, "marshal height error")
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
func (s *DemoService) GetLatestTransactions(ctx context.Context, req *pb.PaginationRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return s.errorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Normalize pagination parameters
	page, pageSize, offset, useCursor, cursorValue := normalizePagination(req.Page, req.PageSize, req.Cursor, MaxPageSize)

	global.Logger.Infof("get latest transactions: page=%d, page_size=%d, use_cursor=%v, cursor=%v", page, pageSize, useCursor, cursorValue)

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// First, try to query from Redis cache (only for first page without cursor)
	if !useCursor && page == MinPage {
		cachedTxs, err := getTransactionsFromCache(queryCtx, int(pageSize))
		if err != nil {
			// Log cache error but continue to database query (graceful degradation)
			global.Logger.Warnf("failed to get transactions from cache, fallback to DB: %v", err)
		} else if len(cachedTxs) > 0 {
			global.Logger.Infof("transactions found in cache: count=%d", len(cachedTxs))
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
					cacheTotal(queryCtx, CacheKeyTotalTransactions, total)
				}
			}
			nextCursor := calculateNextCursor(useCursor, cursorValue, cachedTxs, func(tx models.Transaction) uint64 { return tx.TxNumber })
			txDTOs := models.ToTransactionDTOs(cachedTxs)
			txsResp := buildPaginatedResponse(txDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(cachedTxs))
			return s.marshalAndReply(txsResp, "marshal transactions error")
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
		cacheTotal(queryCtx, CacheKeyTotalTransactions, total)
	}

	// Build query based on pagination type
	query := db.Get().WithContext(queryCtx).Model(&models.Transaction{})

	if useCursor {
		// Use cursor-based pagination (more efficient for large datasets)
		// Cursor is tx_number, find transactions with tx_number < cursor
		query = query.Where("tx_number < ?", cursorValue).
			Order("block_unix_timestamp DESC, tx_number DESC").
			Limit(int(pageSize))
	} else {
		// Use page-based pagination
		query = query.Order("block_unix_timestamp DESC, tx_number DESC").
			Offset(offset).
			Limit(int(pageSize))
	}

	startTime := time.Now()
	err := query.Find(&dbTxs).Error
	queryDuration := time.Since(startTime)

	if err == nil && len(dbTxs) > 0 {
		// Found in database, convert to DTO and return
		global.Logger.Infof("transactions found in database: count=%d, query_time=%v", len(dbTxs), queryDuration)

		if len(dbTxs) > 0 {
			nextCursor := calculateNextCursor(useCursor, cursorValue, dbTxs, func(tx models.Transaction) uint64 { return tx.TxNumber })
			txDTOs := models.ToTransactionDTOs(dbTxs)
			txsResp := buildPaginatedResponse(txDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(dbTxs))
			return s.marshalAndReply(txsResp, "marshal transactions error")
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: error=%v", err)
	}

	// Not found in database or conversion failed, query from chain
	global.Logger.Infof("transactions not found in database or conversion failed, querying from chain")
	client, err := GetSharedExplorerClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
	}

	// Use independent context for RPC calls to avoid HTTP timeout issues
	callCtx, cancel := context.WithTimeout(context.Background(), GRPCTimeout)
	defer cancel()

	transactions, err := client.GetTxs(callCtx, &injective_explorer_rpcpb.GetTxsRequest{Limit: pageSize})
	if err != nil {
		global.Logger.Errorf("get transactions error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get transactions error", err), err
	}

	// Save to database and build response with pagination
	dbTxs = dbTxs[:0] // Reset slice
	for _, tx := range transactions.Data {
		transaction, err := client.GetTxByTxHash(callCtx, tx.Hash)
		if err != nil {
			global.Logger.Errorf("get transaction error: %v", err)
			continue
		}
		// Get storage threshold from config
		storageThreshold := getStorageThresholdForSync()
		dbTx := models.TransactionFromProto(transaction.Data, storageThreshold)
		saveToDB(dbTx, "create transaction error")
		// Cache the transaction
		cacheTransaction(callCtx, dbTx)
		dbTxs = append(dbTxs, *dbTx)
	}

	nextCursor := calculateNextCursor(useCursor, cursorValue, dbTxs, func(tx models.Transaction) uint64 { return tx.TxNumber })
	txDTOs := models.ToTransactionDTOs(dbTxs)
	txsResp := buildPaginatedResponse(txDTOs, page, pageSize, int64(len(dbTxs)), true, useCursor, cursorValue, nextCursor, len(dbTxs))
	return s.marshalAndReply(txsResp, "marshal transactions error")
}

// GetTransactionByHash retrieves a specific transaction by its hash.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing the transaction hash
//
// Returns:
//   - *pb.Reply: Response containing the transaction data
//   - error: Error if the operation fails or validation fails
func (s *DemoService) GetTransactionByHash(ctx context.Context, req *pb.GetTransactionByHashRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return s.errorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Sanitize and validate transaction hash
	hash := SanitizeHash(req.Hash)
	if err := ValidateTransactionHash(hash); err != nil {
		return s.errorReply(http.StatusBadRequest, "invalid transaction hash", err), err
	}

	global.Logger.Infof("get transaction by hash: hash=%s", hash)

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// First, try to query from Redis cache
	cachedTx, err := getTransactionFromCache(queryCtx, "0x"+hash)
	if err == nil && cachedTx != nil {
		global.Logger.Infof("transaction found in cache: hash=0x%s", hash)
		txDTO := models.ToTransactionDTO(cachedTx, models.DTOModeWithDetail)
		return s.marshalAndReply(txDTO, "marshal transaction error")
	}

	// Second, try to query from database
	var dbTx models.Transaction
	err = db.Get().WithContext(queryCtx).Where("hash = ?", "0x"+hash).First(&dbTx).Error
	if err == nil {
		// Found in database
		global.Logger.Infof("transaction found in database: hash=0x%s, archived=%v", hash, dbTx.Archived)

		// If transaction is archived, load full data from archive table
		if dbTx.Archived {
			archiveService := NewArchiveService()
			archivedTx, archiveErr := archiveService.GetArchivedTransaction(queryCtx, "0x"+hash)
			if archiveErr == nil && archivedTx != nil {
				global.Logger.Infof("transaction found in archive: hash=0x%s", hash)
				// Cache the transaction for future queries
				cacheTransaction(queryCtx, archivedTx)
				txDTO := models.ToTransactionDTO(archivedTx, models.DTOModeWithDetail)
				return s.marshalAndReply(txDTO, "marshal transaction error")
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
		return s.marshalAndReply(txDTO, "marshal transaction error")

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
			return s.marshalAndReply(txDTO, "marshal transaction error")
		}
	}

	// Not found in database or conversion failed, query from chain
	global.Logger.Infof("transaction not found in database or conversion failed, querying from chain: hash=%s", hash)
	client, err := GetSharedExplorerClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
	}

	// Use independent context for RPC calls to avoid HTTP timeout issues
	callCtx, cancel := context.WithTimeout(context.Background(), GRPCTimeout)
	defer cancel()

	transaction, err := client.GetTxByTxHash(callCtx, hash)
	if err != nil {
		global.Logger.Errorf("get transaction error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get transaction error", err), err
	}

	// Save to database for future queries
	// Get storage threshold from config
	storageThreshold := getStorageThresholdForSync()
	transactionModel := models.TransactionFromProto(transaction.Data, storageThreshold)
	saveToDB(transactionModel, "create transaction error")

	// Cache the transaction
	cacheTransaction(callCtx, transactionModel)

	txDTO := models.ToTransactionDTO(transactionModel, models.DTOModeWithDetail)
	return s.marshalAndReply(txDTO, "marshal transaction error")
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
func (s *DemoService) GetAccountInfo(ctx context.Context, req *pb.GetAccountInfoRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return s.errorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Sanitize and validate address
	address := SanitizeAddress(req.Address)
	if err := ValidateAddress(address); err != nil {
		return s.errorReply(http.StatusBadRequest, "invalid address", err), err
	}

	global.Logger.Infof("get account info: address=%s", address)

	// Create independent context for database query to avoid HTTP timeout issues
	dbCtx, dbCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer dbCancel()

	// First, try to query from database using the original address
	var dbAccount models.Account
	err := db.Get().WithContext(dbCtx).Where("address = ?", address).First(&dbAccount).Error
	if err == nil {
		// Found in database, convert to DTO and return
		global.Logger.Infof("account found in database: address=%s", address)
		accountDTO := models.ToAccountDTO(&dbAccount)
		return s.marshalAndReply(accountDTO, "marshal account error")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: address=%s, error=%v", address, err)
	}

	// Not found in database or conversion failed, query from chain
	global.Logger.Infof("account not found in database or conversion failed, querying from chain: address=%s", address)
	client, err := GetSharedChainClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "build chain client error", err), err
	}

	// Determine address type and convert if needed
	queryAddress := address
	if strings.HasPrefix(queryAddress, "0x") {
		// Ethereum address, need to convert to Injective address
		evmAccountResp, err := client.FetchEVMCosmosAccount(ctx, queryAddress)
		if err != nil {
			global.Logger.Errorf("failed to fetch EVM cosmos account for %s: %v", queryAddress, err)
			return s.errorReply(http.StatusInternalServerError, "failed to convert Ethereum address to Injective address", err), err
		}
		if evmAccountResp == nil || evmAccountResp.CosmosAddress == "" {
			global.Logger.Errorf("empty cosmos address response for Ethereum address: %s", queryAddress)
			return s.errorReply(http.StatusNotFound, "Injective address not found for Ethereum address", errors.New("cosmos address not found")), errors.New("cosmos address not found")
		}
		queryAddress = evmAccountResp.CosmosAddress
		global.Logger.Infof("converted Ethereum address %s to Injective address %s", address, queryAddress)

		// Try to query database again with converted address
		err = db.Get().WithContext(dbCtx).Where("address = ?", address).First(&dbAccount).Error
		if err == nil {
			// Found in database with converted address
			global.Logger.Infof("account found in database with converted address: address=%s", address)
			accountDTO := models.ToAccountDTO(&dbAccount)
			return s.marshalAndReply(accountDTO, "marshal account error")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			// Database error (not just record not found), log and continue
			global.Logger.Warnf("database query error (not record not found), querying from chain: address=%s, error=%v", address, err)
		}
	} else if !strings.HasPrefix(queryAddress, "inj1") {
		// Unknown address format, try to use as-is
		global.Logger.Warnf("unknown address format, attempting to use as-is: %s", queryAddress)
	}

	account, err := client.GetAccount(ctx, queryAddress)
	if err != nil {
		global.Logger.Errorf("get account error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get account error", err), err
	}

	// Check if account or account.Account is nil
	if account == nil || account.Account == nil {
		global.Logger.Errorf("account or account.Account is nil for address: %s", queryAddress)
		return s.errorReply(http.StatusNotFound, fmt.Sprintf("account %s not found", address), errors.New("account is nil")), errors.New("account is nil")
	}

	// Check if account value is empty
	if len(account.Account.Value) == 0 {
		global.Logger.Errorf("account value is empty for address: %s", queryAddress)
		return s.errorReply(http.StatusNotFound, fmt.Sprintf("account %s not found", address), errors.New("account value is empty")), errors.New("account value is empty")
	}

	ethAccount := &types.EthAccount{}
	err = client.ClientContext().Codec.Unmarshal(account.Account.Value, ethAccount)
	if err != nil {
		global.Logger.Errorf("unmarshal eth account error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "unmarshal eth account error", err), err
	}

	// Save to database for future queries (use original address for storage)
	accountModel := models.AccountFromProto(ethAccount, address, client.ClientContext().Codec)
	saveToDB(accountModel, "create account error")

	accountDTO := models.ToAccountDTO(accountModel)
	return s.marshalAndReply(accountDTO, "marshal account error")
}

// GetAccountTransactions retrieves transactions for a given account address with pagination.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing the account address and pagination info
//
// Returns:
//   - *pb.Reply: Response containing the account transactions with pagination info
//   - error: Error if the operation fails or validation fails
func (s *DemoService) GetAccountTransactions(ctx context.Context, req *pb.GetAccountTransactionsRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return s.errorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Sanitize and validate address
	address := SanitizeAddress(req.Address)
	if err := ValidateAddress(address); err != nil {
		return s.errorReply(http.StatusBadRequest, "invalid address", err), err
	}

	// Normalize pagination parameters
	var page, pageSize int32 = MinPage, DefaultPageSize
	var offset int
	var useCursor bool
	var cursorValue uint64
	if req.Pagination != nil {
		page, pageSize, offset, useCursor, cursorValue = normalizePagination(req.Pagination.Page, req.Pagination.PageSize, req.Pagination.Cursor, MaxPageSize)
	}

	global.Logger.Infof("get account transactions: address=%s, page=%d, page_size=%d, use_cursor=%v", address, page, pageSize, useCursor)

	// Create independent context for database query to avoid HTTP timeout issues
	dbCtx, dbCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer dbCancel()

	// First, try to query from Redis cache (only for first page without cursor)
	if !useCursor && page == MinPage {
		cacheKey := fmt.Sprintf("cache:account:txs:%s", address)
		cachedTxs, err := getAccountTransactionsFromCache(dbCtx, cacheKey, int(pageSize))
		if err == nil && len(cachedTxs) > 0 {
			global.Logger.Infof("account transactions found in cache: address=%s, count=%d", address, len(cachedTxs))
			nextCursor := calculateNextCursor(useCursor, cursorValue, cachedTxs, func(tx models.Transaction) uint64 { return tx.TxNumber })
			txDTOs := models.ToTransactionDTOs(cachedTxs)
			// Use approximate count for cached data
			txsResp := buildPaginatedResponse(txDTOs, page, pageSize, ApproximateCountForAddress, false, useCursor, cursorValue, nextCursor, len(cachedTxs))
			return s.marshalAndReply(txsResp, "marshal transactions error")
		}
	}

	// Second, try to query from database with pagination
	// Note: For better performance, we use LIKE query on messages/events fields
	// In production, consider creating a separate account_transactions table for better indexing
	var dbTxs []models.Transaction

	// Build base query with address filter
	// Use composite index idx_signer_timestamp for optimal performance
	baseQuery := db.Get().WithContext(dbCtx).Model(&models.Transaction{}).
		Where("signer = ?", address)

	// Optimize COUNT query: use shorter timeout and skip if it takes too long
	// For better performance, we'll get count in parallel with data query or skip it
	var total int64
	var isExactCount bool
	var countErr error

	// Only get count if not using cursor (cursor-based pagination doesn't need total)
	// For first page, we can skip count to improve response time
	if !useCursor && page > MinPage {
		// Only get count for non-first pages to determine pagination
		total, isExactCount, countErr = getApproximateCountForAddressWithError(baseQuery, address)
		if countErr != nil {
			global.Logger.Warnf("failed to get approximate count for address %s: %v, using fallback value", address, countErr)
			total = ApproximateCountForAddress
			isExactCount = false
		}
	} else {
		// For first page or cursor-based pagination, skip count query for faster response
		total = ApproximateCountForAddress
		isExactCount = false
	}

	// Build query based on pagination type
	// Use composite index idx_signer_timestamp for optimal performance
	query := baseQuery

	if useCursor {
		// Use cursor-based pagination (more efficient for large datasets)
		// Simple tx_number comparison is sufficient with proper indexing
		query = query.Where("tx_number < ?", cursorValue).
			Order("block_unix_timestamp DESC, tx_number DESC").
			Limit(int(pageSize))
	} else {
		// Use page-based pagination
		// Use composite index: signer + block_unix_timestamp
		query = query.Order("block_unix_timestamp DESC, tx_number DESC").
			Offset(offset).
			Limit(int(pageSize))
	}

	startTime := time.Now()
	err := query.Find(&dbTxs).Error
	queryDuration := time.Since(startTime)

	if err == nil && len(dbTxs) > 0 {
		global.Logger.Infof("account transactions found in database: count=%d, query_time=%v", len(dbTxs), queryDuration)

		// Cache the results for first page to improve subsequent queries
		if !useCursor && page == MinPage {
			cacheKey := fmt.Sprintf("cache:account:txs:%s", address)
			cacheAccountTransactions(dbCtx, cacheKey, dbTxs)
		}

		if len(dbTxs) > 0 {
			nextCursor := calculateNextCursor(useCursor, cursorValue, dbTxs, func(tx models.Transaction) uint64 { return tx.TxNumber })
			txDTOs := models.ToTransactionDTOs(dbTxs)
			txsResp := buildPaginatedResponse(txDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(dbTxs))
			return s.marshalAndReply(txsResp, "marshal transactions error")
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: address=%s, error=%v", address, err)
	}

	// Not found in database or conversion failed, query from chain
	global.Logger.Infof("account transactions not found in database or conversion failed, querying from chain: address=%s", address)
	client, err := GetSharedExplorerClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
	}

	// Use independent context for RPC calls to avoid HTTP timeout issues
	callCtx, cancel := context.WithTimeout(context.Background(), GRPCTimeout)
	defer cancel()

	transactions, err := client.GetAccountTxs(callCtx, &injective_explorer_rpcpb.GetAccountTxsRequest{
		Address: address,
		Limit:   pageSize,
	})
	if err != nil {
		global.Logger.Errorf("get account transactions error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get account transactions error", err), err
	}

	// Save to database for future queries
	if transactions != nil && transactions.Data != nil {
		// Get storage threshold from config
		storageThreshold := getStorageThresholdForSync()
		for _, transaction := range transactions.Data {
			dbTx := models.TransactionFromProto(transaction, storageThreshold)
			saveToDB(dbTx, "create transaction error")
			dbTxs = append(dbTxs, *dbTx)
		}
		nextCursor := calculateNextCursor(useCursor, cursorValue, dbTxs, func(tx models.Transaction) uint64 { return tx.TxNumber })
		txDTOs := models.ToTransactionDTOs(dbTxs)
		transactionsResp := buildPaginatedResponse(txDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(dbTxs))
		return s.marshalAndReply(transactionsResp, "marshal transactions error")
	}
	return s.errorReply(http.StatusNotFound, "no transactions found", errors.New("no transactions found")), errors.New("no transactions found")
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
func (s *DemoService) GetAccountBalances(ctx context.Context, req *pb.GetAccountBalancesRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return s.errorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Sanitize and validate address
	address := SanitizeAddress(req.Address)
	if err := ValidateAddress(address); err != nil {
		return s.errorReply(http.StatusBadRequest, "invalid address", err), err
	}

	global.Logger.Infof("get account balances: address=%s", address)

	// Create independent context for database query to avoid HTTP timeout issues
	dbCtx, dbCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer dbCancel()

	// First, try to query from database
	var dbBalances []models.AccountBalance
	err := db.Get().WithContext(dbCtx).
		Where("address = ?", address).
		Find(&dbBalances).Error
	if err == nil && len(dbBalances) > 0 {
		// Found in database, convert to protobuf format and return
		global.Logger.Infof("account balances found in database: address=%s, count=%d", address, len(dbBalances))
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
			return s.marshalAndReply(balancesResp, "marshal balances error")
		}
		global.Logger.Warnf("no valid balances found in database, querying from chain: address=%s", address)
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: address=%s, error=%v", address, err)
	}

	// Not found in database or conversion failed, query from chain
	global.Logger.Infof("account balances not found in database or conversion failed, querying from chain: address=%s", address)
	client, err := GetSharedChainClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "build chain client error", err), err
	}

	balances, err := client.GetBankBalances(ctx, address)
	if err != nil {
		global.Logger.Errorf("get account balances error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get account balances error", err), err
	}

	// Save to database for future queries
	balancesList := make([]map[string]interface{}, 0, len(dbBalances))
	if balances != nil && balances.Balances != nil {
		for _, balance := range balances.Balances {
			balanceModel := models.AccountBalanceFromProto(balance, address)
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
	return s.marshalAndReply(balancesResp, "marshal balances error")
}

// errorReply creates a standardized error reply with sanitized error messages.
// This function ensures that sensitive information is not exposed to clients.
//
// Parameters:
//   - code: HTTP status code
//   - message: Error message prefix (user-friendly message)
//   - err: The underlying error (may contain sensitive information)
//
// Returns:
//   - *pb.Reply: A reply with sanitized error information
func (s *DemoService) errorReply(code int, message string, err error) *pb.Reply {
	// Sanitize error message to prevent sensitive information leakage
	sanitizedErr := s.sanitizeError(err)
	return &pb.Reply{
		Code:    int32(code),
		Message: fmt.Sprintf("%s: %s", message, sanitizedErr),
	}
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
func (s *DemoService) CheckHealthy(ctx context.Context, req *pb.CheckHealthyRequest) (*pb.CheckHealthyResponse, error) {
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
// Returns:
//   - *structpb.Struct: The converted struct
//   - error: Error if conversion fails
func (s *DemoService) toStruct(v interface{}) (*structpb.Struct, error) {
	// Marshal to JSON first
	jsonBytes, err := sonic.Marshal(v)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal to JSON")
	}

	// Unmarshal into structpb.Struct
	var structData structpb.Struct
	if err := protojson.Unmarshal(jsonBytes, &structData); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal JSON to struct")
	}

	return &structData, nil
}

// protoMessageToStructWithOptions converts a protobuf message to google.protobuf.Struct
// with customizable marshaling options.
//
// Parameters:
//   - msg: The protobuf message to convert
//   - options: Marshaling options (nil to use defaults)
//
// Returns:
//   - *structpb.Struct: The converted struct
//   - error: Error if conversion fails
func (s *DemoService) protoMessageToStructWithOptions(msg proto.Message, options *protojson.MarshalOptions) (*structpb.Struct, error) {
	// Use default options or custom options
	if options == nil {
		options = &protojson.MarshalOptions{
			UseProtoNames:   true,  // Use proto field names instead of JSON names
			EmitUnpopulated: false, // Don't output unset fields
		}
	}

	jsonBytes, err := options.Marshal(msg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal proto to JSON")
	}

	var structData structpb.Struct
	if err := protojson.Unmarshal(jsonBytes, &structData); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal JSON to struct")
	}

	return &structData, nil
}

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
// This function uses database statistics (information_schema/pg_class) for fast approximate counts
// instead of scanning the entire table, which is much more efficient for large tables.
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

// getApproximateCountForAddressWithError gets an approximate count for address-specific queries and returns errors.
// This function properly handles and returns errors instead of ignoring them.
//
// Parameters:
//   - baseQuery: The base query with address filter
//   - address: The address to count
//
// Returns:
//   - approximate count
//   - whether it's an exact count
//   - error if the count query failed (caller should handle this error)
func getApproximateCountForAddressWithError(baseQuery *gorm.DB, address string) (int64, bool, error) {
	var count int64

	// Try to get exact count first (with timeout)
	ctx, cancel := context.WithTimeout(context.Background(), CountQueryTimeout)
	defer cancel()

	err := baseQuery.WithContext(ctx).Count(&count).Error
	if err == nil && count < ApproximateCountThreshold {
		// If count is reasonable, return exact count
		return count, true, nil
	}

	// For larger datasets or timeouts, return approximate
	if err != nil {
		global.Logger.Warnf("exact count for address failed, using approximate: %v", err)
	}
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
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages == 0 {
		totalPages = 1
	}

	return map[string]interface{}{
		"page":        page,
		"page_size":   pageSize,
		"total":       total,
		"total_pages": totalPages,
		"has_prev":    page > 1,
		"has_next":    page < totalPages,
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
//
// Parameters:
//   - model: The model to save
//   - errorMsg: Error message prefix for logging
func saveToDB(model interface{}, errorMsg string) {
	if err := db.Get().Create(model).Error; err != nil {
		global.Logger.Errorf("%s: %v", errorMsg, err)
	}
}

// marshalAndReply serializes the value and returns a success reply.
// The value is converted to google.protobuf.Struct, allowing direct JSON object response.
//
// Parameters:
//   - v: The value to marshal
//   - errorMsg: Error message prefix if marshaling fails
//
// Returns:
//   - *pb.Reply: Success reply with data as Struct, or error reply if marshaling fails
//   - error: Error if marshaling fails
func (s *DemoService) marshalAndReply(v interface{}, errorMsg string) (*pb.Reply, error) {
	var structData *structpb.Struct
	var err error

	// Check if v is a proto.Message, use protoMessageToStructWithOptions if so
	if msg, ok := v.(proto.Message); ok {
		structData, err = s.protoMessageToStructWithOptions(msg, nil)
	} else {
		// For non-proto types (maps, etc.), use toStruct
		structData, err = s.toStruct(v)
	}

	if err != nil {
		global.Logger.Errorf("%s: %v", errorMsg, err)
		return s.errorReply(http.StatusInternalServerError, errorMsg, err), err
	}
	return &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "success",
		Data:    structData,
	}, nil
}

// sanitizeError removes sensitive information from error messages.
// This prevents leaking internal details like file paths, connection strings, etc.
//
// Parameters:
//   - err: The original error that may contain sensitive information
//
// Returns:
//   - string: A sanitized error message safe to return to clients
func (s *DemoService) sanitizeError(err error) string {
	if err == nil {
		return "unknown error"
	}

	errMsg := err.Error()

	// Remove common sensitive patterns
	sensitivePatterns := []string{
		// File paths
		"/Users/",
		"/home/",
		"C:\\",
		// Connection strings
		"@tcp(",
		"postgres://",
		"redis://",
		// API keys and tokens
		"Bearer ",
		"Authorization:",
		// Stack traces (keep only first line)
	}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(errMsg, pattern) {
			// Return a generic error message if sensitive patterns are detected
			return "internal server error"
		}
	}

	// If error contains stack trace, only return the first line
	if idx := strings.Index(errMsg, "\n"); idx > 0 {
		errMsg = errMsg[:idx]
	}

	// Limit error message length to prevent information leakage
	if len(errMsg) > MaxErrorLength {
		errMsg = errMsg[:MaxErrorLength] + "..."
	}

	return errMsg
}

// StreamLatestBlocks streams the latest blocks as they are added to the chain.
// This method subscribes to block events from the event bus and streams them to the client.
func (s *DemoService) StreamLatestBlocks(req *pb.StreamLatestBlocksRequest, stream pb.Demo_StreamLatestBlocksServer) error {
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
func (s *DemoService) StreamLatestTransactions(req *pb.StreamLatestTransactionsRequest, stream pb.Demo_StreamLatestTransactionsServer) error {
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
func (s *DemoService) StreamAccountUpdates(req *pb.StreamAccountUpdatesRequest, stream pb.Demo_StreamAccountUpdatesServer) error {
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

	// Use negative indices to get the latest blocks from the tail of the list
	// LRANGE with negative indices: -limit to -1 gets the last 'limit' elements
	// This ensures we get the latest (highest blockNumber) blocks first
	data, err := redisClient.LRange(ctx, CacheKeyBlockList, int64(-limit), -1).Result()
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

	return blocks, nil
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
	// Use negative indices to get the latest transactions from the tail of the list
	// LRANGE with negative indices: -limit to -1 gets the last 'limit' elements
	// This ensures we get the latest (highest blockNumber/txNumber) transactions first
	data, err := redisClient.LRange(ctx, CacheKeyTransactionList, int64(-limit), -1).Result()
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

	return txs, nil
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

	// Use negative indices to get the latest transactions from the tail of the list
	data, err := redisClient.LRange(ctx, cacheKey, int64(-limit), -1).Result()
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
func cacheTotal(ctx context.Context, key string, total int64) {
	redisClient := cache.GetRedisClient()
	if redisClient == nil {
		return
	}

	// Store total as string with TTL
	err := redisClient.Set(ctx, key, fmt.Sprintf("%d", total), CacheTotalTTL).Err()
	if err != nil {
		global.Logger.Warnf("failed to cache total (key=%s): %v", key, err)
	}
}

// transactionStatsData holds intermediate statistics data during parallel queries
type transactionStatsData struct {
	totalCount         int64
	count30d           int64
	count24h           int64
	activeAddresses24h int64
	failedTxCount24h   int64
	tps                float64
	avgGas        float64
	currentGas    float64
	maxGas24h     float64
	minGas24h     float64
	avgBlockTime       float64
	gasUtilization     float64
}

// getTotalTransactionCount retrieves the total transaction count from chain using GetTxs.
func getTotalTransactionCount(ctx context.Context) (int64, error) {
	client, err := GetSharedExplorerClient()
	if err != nil {
		global.Logger.Warnf("failed to get explorer client: %v, fallback to DB", err)
		return getTotalTransactionCountFromDB(ctx)
	}

	// Use GetTxs with limit=1 to get total count from paging info
	transactions, err := client.GetTxs(ctx, &injective_explorer_rpcpb.GetTxsRequest{
		Limit: 1,
	})
	if err != nil {
		global.Logger.Warnf("failed to get total transaction count from chain: %v, fallback to DB", err)
		return getTotalTransactionCountFromDB(ctx)
	}

	// Get total from paging info if available
	if transactions != nil && transactions.Paging != nil {
		return int64(transactions.Paging.Total), nil
	}

	global.Logger.Warnf("paging info not available in GetTxs response, fallback to DB")
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
func getActiveAddresses24h(ctx context.Context, twentyFourHoursAgoUnix uint64) (int64, error) {
	var activeAddresses24h int64
	err := db.Get().WithContext(ctx).Model(&models.Transaction{}).
		Where("block_unix_timestamp >= ?", twentyFourHoursAgoUnix).
		Distinct("signer").
		Count(&activeAddresses24h).Error
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
	if maxTimestamp > minTimestamp {
		timeSpanMs := float64(maxTimestamp - minTimestamp)
		timeSpanSeconds := timeSpanMs / MillisecondsPerSecond
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

	// Get latest blocks for average gas price calculation
	var latestBlocksForGas []models.Block
	err = db.Get().WithContext(ctx).Model(&models.Block{}).
		Order("height DESC").
		Limit(GasPriceBlockCount).
		Find(&latestBlocksForGas).Error
	if err != nil {
		global.Logger.Warnf("failed to get latest %d blocks for gas price: %v", GasPriceBlockCount, err)
		return 0.0, 0.0, 0.0, 0.0, nil // Return zeros instead of error
	}

	// Calculate average gas price from latest blocks
	if len(latestBlocksForGas) > 0 {
		var blockHeights []uint64
		for _, block := range latestBlocksForGas {
			blockHeights = append(blockHeights, block.Height)
		}

		var gasTxs []models.Transaction
		err = db.Get().WithContext(ctx).Model(&models.Transaction{}).
			Where("block_number IN ? AND gas_used > 0 AND gas_fee != ''", blockHeights).
			Find(&gasTxs).Error
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
	// Note: Since gas_fee is stored as JSON, we need to fetch transactions and calculate in Go
	// This is still more efficient than loading all transactions if we use LIMIT with proper indexing
	var gasTxs24h []models.Transaction
	err = db.Get().WithContext(ctx).Model(&models.Transaction{}).
		Where("block_unix_timestamp >= ? AND gas_used > 0 AND gas_fee != ''", twentyFourHoursAgoUnix).
		Order("block_unix_timestamp DESC").
		Limit(10000). // Limit to prevent memory issues, should cover most cases
		Find(&gasTxs24h).Error
	if err != nil {
		global.Logger.Warnf("failed to get 24h gas transactions: %v", err)
		return avgGasPrice, currentGasPrice, 0.0, 0.0, nil
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
func calculateAverageBlockTimeFromDB(ctx context.Context) (float64, error) {
	var blocksForBlockTime []models.Block
	err := db.Get().WithContext(ctx).Model(&models.Block{}).
		Order("height DESC").
		Limit(BlockTimeBlockCount).
		Find(&blocksForBlockTime).Error
	if err != nil {
		global.Logger.Warnf("failed to get latest %d blocks for block time: %v", BlockTimeBlockCount, err)
		return 0.0, nil // Return 0 instead of error for non-critical metric
	}

	return calculateAverageBlockTime(blocksForBlockTime), nil
}

// calculateGasUtilizationFromDB retrieves transactions and calculates gas utilization.
func calculateGasUtilizationFromDB(ctx context.Context) (float64, error) {
	var blocksForGasUtil []models.Block
	err := db.Get().WithContext(ctx).Model(&models.Block{}).
		Order("height DESC").
		Limit(GasUtilizationBlockCount).
		Find(&blocksForGasUtil).Error
	if err != nil {
		global.Logger.Warnf("failed to get latest %d blocks for gas utilization: %v", GasUtilizationBlockCount, err)
		return 0.0, nil // Return 0 instead of error for non-critical metric
	}

	if len(blocksForGasUtil) == 0 {
		return 0.0, nil
	}

	var blockHeights []uint64
	for _, block := range blocksForGasUtil {
		blockHeights = append(blockHeights, block.Height)
	}

	var utilTxs []models.Transaction
	err = db.Get().WithContext(ctx).Model(&models.Transaction{}).
		Where("block_number IN ?", blockHeights).
		Find(&utilTxs).Error
	if err != nil {
		global.Logger.Warnf("failed to get transactions for gas utilization: %v", err)
		return 0.0, nil
	}

	return calculateGasUtilization(utilTxs), nil
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
func (s *DemoService) GetTransactionStats(ctx context.Context, req *emptypb.Empty) (*pb.Reply, error) {
	global.Logger.Info("getting transaction statistics")

	// Create independent context for queries
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	stats := &pb.TransactionStatsResponse{}

	// Try to get statistics from cache first
	cachedStats := getTransactionStatsFromCache(queryCtx)
	if cachedStats != nil {
		global.Logger.Infof("transaction statistics found in cache")
		return s.marshalAndReply(cachedStats, "marshal transaction stats error")
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
	stats.TotalCount = data.totalCount
	stats.Count_30D = data.count30d
	stats.Count_24H = data.count24h
	stats.Tps = data.tps
	stats.ActiveAddresses_24H = data.activeAddresses24h
	stats.AvgGas = data.avgGas
	stats.CurrentGas = data.currentGas
	stats.MaxGas_24H = data.maxGas24h
	stats.MinGas_24H = data.minGas24h
	stats.AvgBlockTime = data.avgBlockTime
	stats.FailedTxCount_24H = data.failedTxCount24h
	stats.GasUtilization = data.gasUtilization

	global.Logger.Infof("transaction statistics: total=%d, 30d=%d, 24h=%d, tps=%.2f, active_addr_24h=%d, avg_gas=%.6f, avg_block_time=%.2f, gas_utilization=%.2f%%",
		stats.TotalCount, stats.Count_30D, stats.Count_24H, stats.Tps, stats.ActiveAddresses_24H, stats.AvgGas, stats.AvgBlockTime, stats.GasUtilization)

	// Cache the statistics for future queries (async to avoid blocking)
	go cacheTransactionStats(context.Background(), stats)

	return s.marshalAndReply(stats, "marshal transaction stats error")
}

// queryFailedTransactions24h queries failed transactions in the last 24 hours with pagination.
func queryFailedTransactions24h(ctx context.Context, twentyFourHoursAgoUnix uint64, pageSize int, offset int, useCursor bool, cursorValue uint64) ([]models.Transaction, error) {
	baseQuery := db.Get().WithContext(ctx).Model(&models.Transaction{}).
		Where("block_unix_timestamp >= ? AND code != 0", twentyFourHoursAgoUnix).
		Order("block_unix_timestamp DESC, tx_number DESC")

	// Apply cursor-based pagination if cursor is provided
	if useCursor && cursorValue > 0 {
		// For cursor pagination, use tx_number as cursor
		// Since we order by block_unix_timestamp DESC, tx_number DESC, we use tx_number < cursorValue
		baseQuery = baseQuery.Where("tx_number < ?", cursorValue)
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

// getFailedTransactions24hCount gets the total count of failed transactions in the last 24 hours.
// Uses timeout to prevent long-running queries.
func getFailedTransactions24hCount(ctx context.Context, twentyFourHoursAgoUnix uint64) (int64, bool, error) {
	// Use timeout for count query to prevent blocking
	ctxWithTimeout, cancel := context.WithTimeout(ctx, CountQueryTimeout)
	defer cancel()

	var totalCount int64
	err := db.Get().WithContext(ctxWithTimeout).Model(&models.Transaction{}).
		Where("block_unix_timestamp >= ? AND code != 0", twentyFourHoursAgoUnix).
		Count(&totalCount).Error

	if err != nil {
		global.Logger.Warnf("failed to get failed transaction count: %v, using fallback", err)
		return ApproximateCountFallback, false, nil // Return fallback instead of error
	}

	return totalCount, true, nil
}

// sanitizeErrorLog sanitizes and truncates error log messages for security and performance.
// Removes sensitive information and limits length to prevent performance issues.
func sanitizeErrorLog(errorLog string) string {
	if errorLog == "" {
		return ""
	}

	// Truncate to maximum length
	if len(errorLog) > MaxErrorLogLength {
		errorLog = errorLog[:MaxErrorLogLength] + "..."
	}

	// Remove common sensitive patterns (similar to sanitizeError)
	sensitivePatterns := []string{
		"/Users/",
		"/home/",
		"/root/",
		"password",
		"secret",
		"private_key",
		"api_key",
	}

	sanitized := errorLog
	for _, pattern := range sensitivePatterns {
		// Case-insensitive replacement
		if strings.Contains(strings.ToLower(sanitized), strings.ToLower(pattern)) {
			sanitized = strings.ReplaceAll(sanitized, pattern, "[REDACTED]")
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
// Performance:
//
//	This function uses parallel database queries via errgroup to minimize response time.
//	The transaction list query and count query are executed concurrently.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing pagination information
//
// Returns:
//   - *pb.Reply: Response containing failed transactions with pagination info
//   - error: Error if the operation fails
func (s *DemoService) GetFailedTransactions24H(ctx context.Context, req *pb.GetFailedTransactions24HRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return s.errorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
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

	// Parallel query 1: Get failed transactions list
	g.Go(func() error {
		var err error
		failedTxs, err = queryFailedTransactions24h(gCtx, twentyFourHoursAgoUnix, int(pageSize), offset, useCursor, cursorValue)
		return err
	})

	// Parallel query 2: Get total count (only if not using cursor, as cursor pagination doesn't need exact count)
	if !useCursor {
		g.Go(func() error {
			var err error
			totalCount, isExactCount, err = getFailedTransactions24hCount(gCtx, twentyFourHoursAgoUnix)
			return err
		})
	} else {
		// For cursor pagination, we don't need exact count, set a placeholder
		totalCount = 0
		isExactCount = false
	}

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

	// Build response
	response := &pb.FailedTransactions24HResponse{
		Transactions: failedTxInfos,
		Pagination:   paginationInfo,
	}

	global.Logger.Infof("retrieved %d failed transactions (24h), total=%d, exact=%v", len(failedTxs), totalCount, isExactCount)

	return s.marshalAndReply(response, "marshal failed transactions error")
}

// getTransactionStatsFromCache retrieves transaction statistics from Redis cache.
// Returns nil if not found in cache (cache miss).
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

// cacheTransactionStats stores transaction statistics in Redis cache with TTL.
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

	statsJSON, err := sonic.Marshal(stats)
	if err != nil {
		global.Logger.Warnf("failed to marshal transaction stats for cache: %v", err)
		return
	}

	// Cache for configured TTL (statistics don't need to be real-time)
	key := CacheKeyTransactionStats
	err = redisClient.Set(ctx, key, statsJSON, CacheTransactionStatsTTL).Err()
	if err != nil {
		global.Logger.Warnf("failed to cache transaction stats: %v", err)
	}
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
			// Convert from milliseconds to seconds
			timeDiff := float64(prevTimestamp-currTimestamp) / MillisecondsPerSecond
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
// Gas utilization = (Σgas_used / Σgas_wanted) × 100%
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
