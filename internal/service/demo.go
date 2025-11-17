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

	// First, try to query from database with pagination
	var dbBlocks []models.Block
	var total int64
	var isExactCount bool

	// Get approximate or exact count (optimized for performance)
	total, isExactCount = getApproximateCount(db.Get(), &models.Block{}, ctx)

	// Create independent context for database query to avoid HTTP timeout issues
	dbCtx, dbCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer dbCancel()

	// Build query based on pagination type
	query := db.Get().WithContext(dbCtx).Model(&models.Block{})

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

	// Create independent context for database query to avoid HTTP timeout issues
	dbCtx, dbCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer dbCancel()

	// First, try to query from database
	var dbBlock models.Block
	err := db.Get().WithContext(dbCtx).Where("height = ?", req.Height).First(&dbBlock).Error
	if err == nil {
		// Found in database, convert to DTO and return
		global.Logger.Infof("block found in database: height=%d", req.Height)

		blockDTO := models.ToBlockDTO(&dbBlock, models.DTOModeWithDetail)
		return s.marshalAndReply(blockDTO, "marshal block error")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: height=%d, error=%v", req.Height, err)
	}

	// Not found in database or conversion failed, query from chain
	global.Logger.Infof("block not found in database or conversion failed, querying from chain: height=%d", req.Height)
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

	// First, try to query from database with pagination
	var dbTxs []models.Transaction
	var total int64
	var isExactCount bool

	// Get approximate or exact count (optimized for performance)
	total, isExactCount = getApproximateCount(db.Get(), &models.Transaction{}, ctx)

	// Create independent context for database query to avoid HTTP timeout issues
	dbCtx, dbCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer dbCancel()

	// Build query based on pagination type
	query := db.Get().WithContext(dbCtx).Model(&models.Transaction{})

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
		dbTx := models.TransactionFromProto(transaction.Data)
		saveToDB(dbTx, "create transaction error")
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

	// Create independent context for database query to avoid HTTP timeout issues
	dbCtx, dbCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer dbCancel()

	// First, try to query from database
	var dbTx models.Transaction
	err := db.Get().WithContext(dbCtx).Where("hash = ?", "0x"+hash).First(&dbTx).Error
	if err == nil {
		// Found in database, convert to DTO and return
		global.Logger.Infof("transaction found in database: hash=0x%s", hash)
		txDTO := models.ToTransactionDTO(&dbTx, models.DTOModeWithDetail)
		return s.marshalAndReply(txDTO, "marshal transaction error")

	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: hash=%s, error=%v", hash, err)
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
	transactionModel := models.TransactionFromProto(transaction.Data)
	saveToDB(transactionModel, "create transaction error")

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

	// First, try to query from database with pagination
	// Note: For better performance, we use LIKE query on messages/events fields
	// In production, consider creating a separate account_transactions table for better indexing
	var dbTxs []models.Transaction
	var total int64
	var isExactCount bool

	// Create independent context for database query to avoid HTTP timeout issues
	dbCtx, dbCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer dbCancel()

	// Build base query with address filter
	baseQuery := db.Get().WithContext(dbCtx).Model(&models.Transaction{}).
		Where("payer = ?", address)

	// Get approximate count (optimized for performance)
	// For address-specific queries, exact count can be slow
	total, isExactCount = getApproximateCountForAddress(baseQuery, address)

	// Build query based on pagination type
	query := baseQuery

	if useCursor {
		// Use cursor-based pagination (more efficient for large datasets)
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
		global.Logger.Infof("account transactions found in database: count=%d, query_time=%v", len(dbTxs), queryDuration)

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
		for _, transaction := range transactions.Data {
			dbTx := models.TransactionFromProto(transaction)
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

// GetValidators retrieves validators from the database with pagination.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Pagination request containing page and page_size
//
// Returns:
//   - *pb.Reply: Response containing the validators data with pagination info
//   - error: Error if the operation fails
func (s *DemoService) GetValidators(ctx context.Context, req *pb.PaginationRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return s.errorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Normalize pagination parameters
	page, pageSize, offset, useCursor, cursorValue := normalizePagination(req.Page, req.PageSize, req.Cursor, MaxPageSize)

	global.Logger.Infof("get validators: page=%d, page_size=%d, use_cursor=%v, cursor=%v", page, pageSize, useCursor, cursorValue)

	// Query validators from database with pagination
	var dbValidators []models.Validator
	var total int64
	var isExactCount bool

	// Get approximate or exact count (optimized for performance)
	total, isExactCount = getApproximateCount(db.Get(), &models.Validator{}, ctx)

	// Create independent context for database query to avoid HTTP timeout issues
	dbCtx, dbCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer dbCancel()

	// Build query based on pagination type
	query := db.Get().WithContext(dbCtx).Model(&models.Validator{})

	if useCursor {
		// Use cursor-based pagination (more efficient for large datasets)
		// For validators, we can use ID as cursor (assuming ID is numeric or can be used for ordering)
		// Since validators don't have a natural ordering like height, we'll use created_at or ID
		query = query.Where("id < ?", cursorValue).
			Order("id DESC").
			Limit(int(pageSize))
	} else {
		// Use page-based pagination
		// Order by operator_address for consistent ordering
		query = query.Order("operator_address ASC").
			Offset(offset).
			Limit(int(pageSize))
	}

	startTime := time.Now()
	err := query.Find(&dbValidators).Error
	queryDuration := time.Since(startTime)

	if err == nil && len(dbValidators) > 0 {
		// Found in database, convert to DTO and return
		global.Logger.Infof("validators found in database: count=%d, query_time=%v", len(dbValidators), queryDuration)

		if len(dbValidators) > 0 {
			nextCursor := calculateNextCursor(useCursor, cursorValue, dbValidators, func(v models.Validator) uint64 { return uint64(v.ID) })
			validatorDTOs := models.ToValidatorDTOs(dbValidators)
			validatorsResp := buildPaginatedResponse(validatorDTOs, page, pageSize, total, isExactCount, useCursor, cursorValue, nextCursor, len(dbValidators))
			return s.marshalAndReply(validatorsResp, "marshal validators error")
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), return error
		global.Logger.Errorf("database query error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "database query error", err), err
	}

	// Not found in database, query from chain
	global.Logger.Infof("no validators found in database, querying from chain")
	client, err := GetSharedExplorerClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
	}

	// Use independent context for RPC calls to avoid HTTP timeout issues
	callCtx, cancel := context.WithTimeout(context.Background(), GRPCTimeout)
	defer cancel()

	validators, err := client.FetchValidators(callCtx)
	if err != nil {
		global.Logger.Errorf("get validators error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get validators error", err), err
	}

	// Save to database and build response with pagination
	dbValidators = dbValidators[:0] // Reset slice
	for _, validator := range validators.Data {
		dbValidator := models.ValidatorFromProto(validator)
		saveToDB(dbValidator, "create validator error")
		dbValidators = append(dbValidators, *dbValidator)
	}

	// Apply pagination to results
	if len(dbValidators) > int(pageSize) {
		dbValidators = dbValidators[:pageSize]
	}

	nextCursor := calculateNextCursor(useCursor, cursorValue, dbValidators, func(v models.Validator) uint64 { return uint64(v.ID) })
	validatorDTOs := models.ToValidatorDTOs(dbValidators)
	validatorsResp := buildPaginatedResponse(validatorDTOs, page, pageSize, int64(len(dbValidators)), true, useCursor, cursorValue, nextCursor, len(dbValidators))
	return s.marshalAndReply(validatorsResp, "marshal validators error")
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
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
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
		redisCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
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
func getApproximateCount(db *gorm.DB, model interface{}, ctx context.Context) (int64, bool) {
	var count int64

	// Try to get exact count first (with timeout)
	ctxWithTimeout, cancel := context.WithTimeout(ctx, CountQueryTimeout)
	defer cancel()

	err := db.WithContext(ctxWithTimeout).Model(model).Count(&count).Error
	if err == nil {
		return count, true
	}

	// If exact count times out or fails, try to get approximate count
	// For PostgreSQL, we can use pg_class.reltuples
	// For MySQL, we can use information_schema
	// For now, we'll return a large estimate and mark as approximate
	global.Logger.Warnf("exact count failed, using approximate: %v", err)
	return ApproximateCountFallback, false
}

// getApproximateCountForAddress gets an approximate count for address-specific queries.
// This is optimized for queries that filter by address in messages/events fields.
//
// Parameters:
//   - baseQuery: The base query with address filter
//   - address: The address to count
//
// Returns:
//   - approximate count and whether it's an exact count
func getApproximateCountForAddress(baseQuery *gorm.DB, address string) (int64, bool) {
	var count int64

	// Try to get exact count first (with timeout)
	ctx, cancel := context.WithTimeout(context.Background(), CountQueryTimeout)
	defer cancel()

	err := baseQuery.WithContext(ctx).Count(&count).Error
	if err == nil && count < ApproximateCountThreshold {
		// If count is reasonable, return exact count
		return count, true
	}

	// For larger datasets or timeouts, return approximate
	if err != nil {
		global.Logger.Warnf("exact count for address failed, using approximate: %v", err)
	}
	return ApproximateCountForAddress, false
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
	global.Logger.Infof("stream latest blocks: from_height=%d", req.GetFromHeight())

	// Subscribe to block events
	eventBus := GetEventBus()
	blockChan := eventBus.SubscribeBlocks()
	defer eventBus.UnsubscribeBlocks(blockChan)

	// Send initial/historical blocks
	// If from_height is specified, send blocks from that height
	// Otherwise, send the latest blocks (e.g., last 10 blocks)
	var dbBlocks []models.Block
	var err error

	if req.GetFromHeight() > 0 {
		// Query blocks from database starting from from_height
		global.Logger.Debugf("querying blocks from height %d", req.GetFromHeight())
		err = db.Get().WithContext(stream.Context()).
			Where("height >= ?", req.GetFromHeight()).
			Order("height ASC").
			Limit(100). // Limit to avoid sending too much data
			Find(&dbBlocks).Error
	} else {
		// If no from_height specified, send the latest blocks to show something immediately
		global.Logger.Debugf("querying latest blocks from database")
		err = db.Get().WithContext(stream.Context()).
			Order("height DESC").
			Limit(10). // Send last 10 blocks as initial data
			Find(&dbBlocks).Error
		// Reverse the order to send from oldest to newest
		for i, j := 0, len(dbBlocks)-1; i < j; i, j = i+1, j-1 {
			dbBlocks[i], dbBlocks[j] = dbBlocks[j], dbBlocks[i]
		}
	}

	// Log query result for debugging
	if err != nil {
		global.Logger.Warnf("database query error in StreamLatestBlocks: %v", err)
	} else {
		global.Logger.Infof("database query successful: found %d blocks", len(dbBlocks))
	}

	// Check if database has any blocks at all (for debugging)
	if len(dbBlocks) == 0 {
		var totalCount int64
		countErr := db.Get().WithContext(stream.Context()).Model(&models.Block{}).Count(&totalCount).Error
		if countErr != nil {
			global.Logger.Warnf("failed to count total blocks: %v", countErr)
		} else {
			global.Logger.Infof("no blocks found in query, total blocks in database: %d", totalCount)
		}
	}

	if err == nil && len(dbBlocks) > 0 {
		sentCount := 0
		for _, block := range dbBlocks {
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
		global.Logger.Infof("sent %d/%d initial blocks to stream", sentCount, len(dbBlocks))
	} else if len(dbBlocks) == 0 {
		global.Logger.Warnf("no blocks to send: query returned %d blocks", len(dbBlocks))
	}

	// Stream new blocks from event bus
	// Add a ticker to log that we're waiting for new blocks (helps with debugging)
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	global.Logger.Infof("StreamLatestBlocks: waiting for new block events...")

	for {
		select {
		case <-stream.Context().Done():
			global.Logger.Infof("stream context cancelled for StreamLatestBlocks")
			return stream.Context().Err()
		case <-heartbeatTicker.C:
			// Log that we're still waiting (helps debug if connection is alive but no blocks)
			global.Logger.Debugf("StreamLatestBlocks: still waiting for new blocks...")
		case blockEvent := <-blockChan:
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
	global.Logger.Infof("stream latest transactions: types=%v", req.GetTypes())

	// Subscribe to transaction events
	eventBus := GetEventBus()
	txChan := eventBus.SubscribeTransactions()
	defer eventBus.UnsubscribeTransactions(txChan)

	// Send initial/historical transactions
	// Always send the latest transactions to show something immediately
	var dbTxs []models.Transaction
	query := db.Get().WithContext(stream.Context()).
		Order("created_at DESC").
		Limit(20) // Send last 20 transactions as initial data

	// Apply type filter if specified
	if len(req.GetTypes()) > 0 {
		// Build OR condition for transaction types
		conditions := make([]string, 0, len(req.GetTypes()))
		args := make([]interface{}, 0, len(req.GetTypes()))
		for _, txType := range req.GetTypes() {
			conditions = append(conditions, "tx_type LIKE ?")
			args = append(args, "%"+strings.ToLower(txType)+"%")
		}
		if len(conditions) > 0 {
			query = query.Where("("+strings.Join(conditions, " OR ")+")", args...)
		}
	}

	err := query.Find(&dbTxs).Error
	if err == nil && len(dbTxs) > 0 {
		// Reverse the order to send from oldest to newest
		for i, j := 0, len(dbTxs)-1; i < j; i, j = i+1, j-1 {
			dbTxs[i], dbTxs[j] = dbTxs[j], dbTxs[i]
		}

		for _, tx := range dbTxs {
			txDTO := models.ToTransactionDTO(&tx, models.DTOModeWithDetail)

			update := &pb.TransactionUpdate{
				Transaction: txDTO,
				UpdateType:  "new",
				Timestamp:   tx.CreatedAt.Unix(),
			}

			if err := stream.Send(update); err != nil {
				return status.Errorf(codes.Internal, "failed to send transaction update: %v", err)
			}
		}
		global.Logger.Infof("sent %d initial transactions to stream", len(dbTxs))
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
		Where("payer = ?", address).
		Order("block_unix_timestamp DESC, tx_number DESC")

	// Apply timestamp filter if specified
	if req.GetFromTimestamp() > 0 {
		query = query.Where("block_unix_timestamp >= ?", req.GetFromTimestamp())
	}

	// Limit to avoid sending too much data
	query = query.Limit(50)

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
