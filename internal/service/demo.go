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
	"injective-explorer/provider/db"

	"github.com/InjectiveLabs/sdk-go/chain/types"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	explorerclient "github.com/InjectiveLabs/sdk-go/client/explorer"
	tmclient "github.com/InjectiveLabs/sdk-go/client/tm"
	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
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

	// Build query based on pagination type
	query := db.Get().WithContext(ctx).Model(&models.Block{})

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
		// Found in database, convert to protobuf format and return
		global.Logger.Infof("blocks found in database: count=%d, query_time=%v", len(dbBlocks), queryDuration)

		// Convert database blocks to response format
		blockList := make([]*injective_explorer_rpcpb.BlockDetailInfo, 0, len(dbBlocks))
		var nextCursor string

		for _, dbBlock := range dbBlocks {
			block := models.BlockToProto(&dbBlock)
			blockList = append(blockList, block)

			// Set next cursor to the smallest height in this page (for next page)
			if useCursor {
				if nextCursor == "" || dbBlock.Height < cursorValue {
					nextCursor = fmt.Sprintf("%d", dbBlock.Height)
				}
			}
		}

		if len(blockList) > 0 {
			// Build pagination info
			paginationInfo := buildPaginationInfo(page, pageSize, total)
			if !isExactCount {
				paginationInfo["approximate"] = true
			}

			// Add cursor info if using cursor-based pagination
			if useCursor {
				paginationInfo["cursor"] = cursorValue
				if nextCursor != "" {
					paginationInfo["next_cursor"] = nextCursor
				}
				paginationInfo["has_next"] = len(dbBlocks) == int(pageSize) // If we got full page, likely has more
			}

			blocksResp := map[string]interface{}{
				"data":       blockList,
				"pagination": paginationInfo,
			}
			return s.marshalAndReply(blocksResp, "marshal blocks error")
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: error=%v", err)
	}

	// Not found in database or conversion failed, query from chain
	global.Logger.Infof("blocks not found in database or conversion failed, querying from chain")
	client, err := s.getExplorerClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
	}

	// Use context with longer timeout for gRPC operations
	callCtx, cancel := withGRPCTimeout(ctx, 0)
	defer cancel()

	blocks, err := client.GetBlocks(callCtx)
	if err != nil {
		global.Logger.Errorf("get blocks error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get blocks error", err), err
	}

	// Save to database for future queries
	blockDetails := make([]*injective_explorer_rpcpb.BlockDetailInfo, 0, len(blocks.Data))
	for _, block := range blocks.Data {
		client, err := s.getExplorerClient()
		if err != nil {
			return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
		}

		// Use context with longer timeout for gRPC operations
		callCtx, cancel := withGRPCTimeout(ctx, 0)
		defer cancel()

		blockDetail, err := client.GetBlock(callCtx, fmt.Sprintf("%d", block.Height))
		if err != nil {
			global.Logger.Errorf("get block error: %v", err)
			return s.errorReply(http.StatusInternalServerError, "get block error", err), err
		}

		// Save to database for future queries
		err = db.Get().Create(models.BlockFromProto(blockDetail.Data)).Error
		if err != nil {
			global.Logger.Errorf("create block error: %v", err)
			// Don't return error, just log it since we already have the data
		}
		blockDetails = append(blockDetails, blockDetail.Data)
	}

	return s.marshalAndReply(blockDetails, "marshal blocks error")
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

	// First, try to query from database
	var dbBlock models.Block
	err := db.Get().WithContext(ctx).Where("height = ?", req.Height).First(&dbBlock).Error
	if err == nil {
		// Found in database, convert to protobuf format and return
		global.Logger.Infof("block found in database: height=%d", req.Height)
		block := models.BlockToProto(&dbBlock)
		if block == nil {
			global.Logger.Warnf("failed to convert block to proto, querying from chain: height=%d", req.Height)
			// Continue to query from chain
		} else {
			blockResp := &injective_explorer_rpcpb.GetBlockResponse{
				Data: block,
			}
			return s.marshalAndReply(blockResp, "marshal block error")
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: height=%d, error=%v", req.Height, err)
	}

	// Not found in database or conversion failed, query from chain
	global.Logger.Infof("block not found in database or conversion failed, querying from chain: height=%d", req.Height)
	client, err := s.getExplorerClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
	}

	// Use context with longer timeout for gRPC operations
	callCtx, cancel := withGRPCTimeout(ctx, 0)
	defer cancel()

	block, err := client.GetBlock(callCtx, fmt.Sprintf("%d", req.Height))
	if err != nil {
		global.Logger.Errorf("get block error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get block error", err), err
	}

	// Save to database for future queries
	err = db.Get().Create(models.BlockFromProto(block.Data)).Error
	if err != nil {
		global.Logger.Errorf("create block error: %v", err)
		// Don't return error, just log it since we already have the data
	}

	return s.marshalAndReply(block, "marshal block error")
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

	client := s.getTMClient()
	height, err := client.GetLatestBlockHeight(ctx)
	if err != nil {
		global.Logger.Errorf("get latest block height error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get latest block height error", err), err
	}

	return &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "success",
		Data:    fmt.Sprintf("%d", height),
	}, nil
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

	// Build query based on pagination type
	query := db.Get().WithContext(ctx).Model(&models.Transaction{})

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
		// Found in database, convert to protobuf format and return
		global.Logger.Infof("transactions found in database: count=%d, query_time=%v", len(dbTxs), queryDuration)

		// Convert database transactions to response format
		txList := make([]*injective_explorer_rpcpb.TxDetailData, 0, len(dbTxs))
		var nextCursor string

		for _, dbTx := range dbTxs {
			txMap := models.TransactionToProto(&dbTx)
			txList = append(txList, txMap)

			// Set next cursor to the smallest tx_number in this page
			if useCursor {
				if nextCursor == "" || dbTx.TxNumber < cursorValue {
					nextCursor = fmt.Sprintf("%d", dbTx.TxNumber)
				}
			}
		}

		if len(txList) > 0 {
			// Build pagination info
			paginationInfo := buildPaginationInfo(page, pageSize, total)
			if !isExactCount {
				paginationInfo["approximate"] = true
			}

			// Add cursor info if using cursor-based pagination
			if useCursor {
				paginationInfo["cursor"] = cursorValue
				if nextCursor != "" {
					paginationInfo["next_cursor"] = nextCursor
				}
				paginationInfo["has_next"] = len(dbTxs) == int(pageSize)
			}

			txsResp := map[string]interface{}{
				"data":       txList,
				"pagination": paginationInfo,
			}
			return s.marshalAndReply(txsResp, "marshal transactions error")
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: error=%v", err)
	}

	// Not found in database or conversion failed, query from chain
	// Note: Chain API doesn't support pagination, so we'll return first page with limited results
	global.Logger.Infof("transactions not found in database or conversion failed, querying from chain")
	client, err := s.getExplorerClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
	}

	// Use context with longer timeout for gRPC operations
	callCtx, cancel := withGRPCTimeout(ctx, 0)
	defer cancel()

	// Use page_size as limit for chain query
	transactions, err := client.GetTxs(callCtx, &injective_explorer_rpcpb.GetTxsRequest{Limit: pageSize})
	if err != nil {
		global.Logger.Errorf("get transactions error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get transactions error", err), err
	}

	// Save to database for future queries
	// Note: GetTxs returns TxData, but we need TxDetailData for TransactionFromProto
	// We'll skip saving individual transactions from GetTxs response since they don't have full detail
	// Individual transactions will be saved when queried by hash via GetTransactionByHash

	return s.marshalAndReply(transactions, "marshal transactions error")
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

	// First, try to query from database
	var dbTx models.Transaction
	err := db.Get().WithContext(ctx).Where("hash = ?", hash).First(&dbTx).Error
	if err == nil {
		// Found in database, convert to protobuf format and return
		global.Logger.Infof("transaction found in database: hash=%s", hash)
		txDetail := models.TransactionToProto(&dbTx)
		if txDetail == nil {
			global.Logger.Warnf("failed to convert transaction to proto, querying from chain: hash=%s", hash)
			// Continue to query from chain
		} else {
			txResp := &injective_explorer_rpcpb.GetTxByTxHashResponse{
				Data: txDetail,
			}
			return s.marshalAndReply(txResp, "marshal transaction error")
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: hash=%s, error=%v", hash, err)
	}

	// Not found in database or conversion failed, query from chain
	global.Logger.Infof("transaction not found in database or conversion failed, querying from chain: hash=%s", hash)
	client, err := s.getExplorerClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
	}

	// Use context with longer timeout for gRPC operations
	callCtx, cancel := withGRPCTimeout(ctx, 0)
	defer cancel()

	transaction, err := client.GetTxByTxHash(callCtx, hash)
	if err != nil {
		global.Logger.Errorf("get transaction error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get transaction error", err), err
	}

	// Save to database for future queries
	err = db.Get().Create(models.TransactionFromProto(transaction.Data)).Error
	if err != nil {
		global.Logger.Errorf("create transaction error: %v", err)
		// Don't return error, just log it since we already have the data
	}

	return s.marshalAndReply(transaction, "marshal transaction error")
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

	// First, try to query from database using the original address
	var dbAccount models.Account
	err := db.Get().WithContext(ctx).Where("address = ?", address).First(&dbAccount).Error
	if err == nil {
		// Found in database, convert to JSON and return
		global.Logger.Infof("account found in database: address=%s", address)
		data, err := models.AccountToEthAccountJSON(&dbAccount)
		if err != nil {
			global.Logger.Warnf("convert account to JSON error, querying from chain: address=%s, error=%v", address, err)
			// Continue to query from chain
		} else {
			return s.successReply(data), nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: address=%s, error=%v", address, err)
	}

	// Not found in database or conversion failed, query from chain
	global.Logger.Infof("account not found in database or conversion failed, querying from chain: address=%s", address)
	client, err := s.getChainClient()
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
		err = db.Get().WithContext(ctx).Where("address = ?", address).First(&dbAccount).Error
		if err == nil {
			// Found in database with converted address
			global.Logger.Infof("account found in database with converted address: address=%s", address)
			data, err := models.AccountToEthAccountJSON(&dbAccount)
			if err != nil {
				global.Logger.Warnf("convert account to JSON error, querying from chain: address=%s, error=%v", address, err)
				// Continue to query from chain
			} else {
				return s.successReply(data), nil
			}
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
	err = db.Get().Create(accountModel).Error
	if err != nil {
		global.Logger.Errorf("create account error: %v", err)
		// Don't return error, just log it since we already have the data
	}
	data, err := s.marshalToJSON(accountModel)
	if err != nil {
		global.Logger.Errorf("marshal account error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "marshal account error", err), err
	}

	return s.successReply(string(data)), nil
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

	// Build base query with address filter
	baseQuery := db.Get().WithContext(ctx).Model(&models.Transaction{}).
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

		// Convert to response format
		txList := make([]*injective_explorer_rpcpb.TxDetailData, 0, len(dbTxs))
		var nextCursor string

		for _, dbTx := range dbTxs {
			txMap := models.TransactionToProto(&dbTx)
			txList = append(txList, txMap)

			// Set next cursor to the smallest tx_number in this page
			if useCursor {
				if nextCursor == "" || dbTx.TxNumber < cursorValue {
					nextCursor = fmt.Sprintf("%d", dbTx.TxNumber)
				}
			}
		}

		if len(txList) > 0 {
			// Build pagination info
			paginationInfo := buildPaginationInfo(page, pageSize, total)
			if !isExactCount {
				paginationInfo["approximate"] = true
			}

			// Add cursor info if using cursor-based pagination
			if useCursor {
				paginationInfo["cursor"] = cursorValue
				if nextCursor != "" {
					paginationInfo["next_cursor"] = nextCursor
				}
				paginationInfo["has_next"] = len(dbTxs) == int(pageSize)
			}

			txsResp := map[string]interface{}{
				"data":       txList,
				"pagination": paginationInfo,
			}
			return s.marshalAndReply(txsResp, "marshal transactions error")
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), log and continue to query from chain
		global.Logger.Warnf("database query error (not record not found), querying from chain: address=%s, error=%v", address, err)
	}

	// Not found in database or conversion failed, query from chain
	global.Logger.Infof("account transactions not found in database or conversion failed, querying from chain: address=%s", address)
	client, err := s.getExplorerClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
	}

	// Use context with longer timeout for gRPC operations
	callCtx, cancel := withGRPCTimeout(ctx, 0)
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
			err = db.Get().Create(models.TransactionFromProto(transaction)).Error
			if err != nil {
				global.Logger.Errorf("create transaction error: %v", err)
				// Don't return error, just log it
			}
		}
	}

	return s.marshalAndReply(transactions, "marshal transactions error")
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

	// First, try to query from database
	var dbBalances []models.AccountBalance
	err := db.Get().WithContext(ctx).
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
	client, err := s.getChainClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "build chain client error", err), err
	}

	balances, err := client.GetBankBalances(ctx, address)
	if err != nil {
		global.Logger.Errorf("get account balances error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get account balances error", err), err
	}

	// Save to database for future queries
	if balances != nil && balances.Balances != nil {
		for _, balance := range balances.Balances {
			err = db.Get().Create(models.AccountBalanceFromProto(balance, address)).Error
			if err != nil {
				global.Logger.Errorf("create account balance error: %v", err)
				// Don't return error, just log it since we already have the data
			}
		}
	}

	return s.marshalAndReply(balances, "marshal balances error")
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

	// Build query based on pagination type
	query := db.Get().WithContext(ctx).Model(&models.Validator{})

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
		// Found in database, convert to protobuf format and return
		global.Logger.Infof("validators found in database: count=%d, query_time=%v", len(dbValidators), queryDuration)

		// Convert database validators to response format
		validatorList := make([]*injective_explorer_rpcpb.Validator, 0, len(dbValidators))
		var nextCursor string

		for _, dbValidator := range dbValidators {
			validator := models.ValidatorToProto(&dbValidator)
			if validator == nil {
				global.Logger.Warnf("failed to convert validator to proto, skipping: operator_address=%s", dbValidator.OperatorAddress)
				continue
			}
			validatorList = append(validatorList, validator)

			// Set next cursor to the smallest ID in this page (for next page)
			if useCursor {
				// For cursor-based pagination, we can use the validator's database ID
				// Since we're using id < cursorValue, we need to track the minimum ID
				if nextCursor == "" || dbValidator.ID < uint(cursorValue) {
					nextCursor = fmt.Sprintf("%d", dbValidator.ID)
				}
			}
		}

		if len(validatorList) > 0 {
			// Build pagination info
			paginationInfo := buildPaginationInfo(page, pageSize, total)
			if !isExactCount {
				paginationInfo["approximate"] = true
			}

			// Add cursor info if using cursor-based pagination
			if useCursor {
				paginationInfo["cursor"] = cursorValue
				if nextCursor != "" {
					paginationInfo["next_cursor"] = nextCursor
				}
				paginationInfo["has_next"] = len(dbValidators) == int(pageSize) // If we got full page, likely has more
			}

			validatorsResp := map[string]interface{}{
				"data":       validatorList,
				"pagination": paginationInfo,
			}
			return s.marshalAndReply(validatorsResp, "marshal validators error")
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// Database error (not just record not found), return error
		global.Logger.Errorf("database query error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "database query error", err), err
	}

	// Not found in database, return empty result
	global.Logger.Infof("no validators found in database")
	client, err := s.getExplorerClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
	}

	// Use context with longer timeout for gRPC operations
	callCtx, cancel := withGRPCTimeout(ctx, 0)
	defer cancel()

	validators, err := client.FetchValidators(callCtx)
	if err != nil {
		global.Logger.Errorf("get block error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get block error", err), err
	}

	// Save to database for future queries
	for _, validator := range validators.Data {
		err = db.Get().Create(models.ValidatorFromProto(validator)).Error
		if err != nil {
			global.Logger.Errorf("create validator error: %v", err)
			// Don't return error, just log it since we already have the data
		}
	}

	return s.marshalAndReply(validators, "marshal block error")
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

// withGRPCTimeout creates a context with timeout for gRPC operations.
// This is a helper function to reduce code duplication.
//
// Parameters:
//   - ctx: The parent context
//   - timeout: The timeout duration (defaults to 60 seconds if 0)
//
// Returns:
//   - context.Context: A context with timeout
//   - context.CancelFunc: A cancel function to cancel the context
func withGRPCTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout == 0 {
		timeout = GRPCTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

// successReply creates a standardized success reply.
//
// Parameters:
//   - data: The JSON string data to return
//
// Returns:
//   - *pb.Reply: A reply with success status and data
func (s *DemoService) successReply(data string) *pb.Reply {
	return &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "success",
		Data:    data,
	}
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
//   - map[string]interface{}: Pagination information map
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

// marshalToJSON serializes the given value to a JSON string.
// This function is optimized for performance and reduces code duplication.
//
// Parameters:
//   - v: The value to marshal (any type that can be serialized to JSON)
//
// Returns:
//   - string: The JSON string representation of the value
//   - error: Error if marshaling fails
func (s *DemoService) marshalToJSON(v interface{}) (string, error) {
	if msg, ok := v.(proto.Message); ok {
		// Use protojson for protobuf messages to properly handle Any types
		jsonBytes, err := protojson.Marshal(msg)
		if err != nil {
			return "", errors.Wrap(err, "failed to marshal protobuf message to JSON")
		}
		return string(jsonBytes), nil
	}
	jsonBytes, err := sonic.Marshal(v)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal to JSON")
	}
	return string(jsonBytes), nil
}

// marshalAndReply serializes the value to JSON and returns a success reply.
// This helper function reduces code duplication for JSON serialization error handling.
//
// Parameters:
//   - v: The value to marshal
//   - errorMsg: Error message prefix if marshaling fails
//
// Returns:
//   - *pb.Reply: Success reply with JSON data, or error reply if marshaling fails
//   - error: Error if marshaling fails
func (s *DemoService) marshalAndReply(v interface{}, errorMsg string) (*pb.Reply, error) {
	data, err := s.marshalToJSON(v)
	if err != nil {
		global.Logger.Errorf("%s: %v", errorMsg, err)
		return s.errorReply(http.StatusInternalServerError, errorMsg, err), err
	}
	return s.successReply(data), nil
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

// getExplorerClient returns a cached or newly created explorer client.
// This method uses shared client instances for better resource utilization.
//
// Returns:
//   - ExplorerClient: The explorer client instance
//   - error: Error if client creation fails
func (s *DemoService) getExplorerClient() (explorerclient.ExplorerClient, error) {
	return GetSharedExplorerClient()
}

// getChainClient returns a cached or newly created chain client.
// This method uses shared client instances for better resource utilization.
//
// Returns:
//   - ChainClient: The chain client instance
//   - error: Error if client creation fails
func (s *DemoService) getChainClient() (chainclient.ChainClientV2, error) {
	return GetSharedChainClient()
}

// getTMClient returns a cached or newly created Tendermint client.
// This method uses shared client instances for better resource utilization.
//
// Returns:
//   - TMClient: The Tendermint client instance
func (s *DemoService) getTMClient() tmclient.TendermintClient {
	return GetSharedTmClient()
}
