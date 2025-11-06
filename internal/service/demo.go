// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/InjectiveLabs/sdk-go/chain/types"
	"github.com/InjectiveLabs/sdk-go/client"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	"github.com/InjectiveLabs/sdk-go/client/common"
	explorerclient "github.com/InjectiveLabs/sdk-go/client/explorer"
	tmclient "github.com/InjectiveLabs/sdk-go/client/tm"
	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/bytedance/sonic"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
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

	// Cached clients to avoid recreating connections
	explorerClient explorerclient.ExplorerClient
	chainClient    chainclient.ChainClientV2
	tmClient       tmclient.TendermintClient

	// Mutexes for thread-safe client initialization
	explorerClientOnce sync.Once
	chainClientOnce    sync.Once
	tmClientOnce       sync.Once

	// Errors from client initialization
	explorerClientErr error
	chainClientErr    error
}

// NewDemoService creates a new instance of DemoService.
//
// Returns:
//   - *DemoService: A new service instance
func NewDemoService() *DemoService {
	return &DemoService{}
}

// GetLatestBlocks retrieves the latest blocks from the blockchain.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Empty request
//
// Returns:
//   - *pb.Reply: Response containing the latest blocks data
//   - error: Error if the operation fails
func (s *DemoService) GetLatestBlocks(ctx context.Context, req *emptypb.Empty) (*pb.Reply, error) {
	global.Logger.Infof("get latest blocks")

	client, err := s.getExplorerClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
	}

	// Use context with longer timeout for gRPC operations
	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	blocks, err := client.GetBlocks(callCtx)
	if err != nil {
		global.Logger.Errorf("get blocks error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get blocks error", err), err
	}

	data, err := s.marshalToJSON(blocks)
	if err != nil {
		global.Logger.Errorf("marshal blocks error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "marshal blocks error", err), err
	}

	return s.successReply(data), nil
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
		blockDetail := models.BlockToProto(&dbBlock)
		if blockDetail == nil {
			global.Logger.Warnf("failed to convert block to proto, querying from chain: height=%d", req.Height)
			// Continue to query from chain
		} else {
			blockResp := &injective_explorer_rpcpb.GetBlockResponse{
				Data: blockDetail,
			}
			data, err := s.marshalToJSON(blockResp)
			if err != nil {
				global.Logger.Errorf("marshal block error: %v", err)
				return s.errorReply(http.StatusInternalServerError, "marshal block error", err), err
			}
			return s.successReply(data), nil
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
	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	block, err := client.GetBlock(callCtx, fmt.Sprintf("%d", req.Height))
	if err != nil {
		global.Logger.Errorf("get block error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get block error", err), err
	}

	data, err := s.marshalToJSON(block)
	if err != nil {
		global.Logger.Errorf("marshal block error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "marshal block error", err), err
	}

	// Save to database for future queries
	err = db.Get().Create(models.BlockFromProto(block.Data)).Error
	if err != nil {
		global.Logger.Errorf("create block error: %v", err)
		// Don't return error, just log it since we already have the data
	}

	return s.successReply(data), nil
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

// GetLatestTransactions retrieves the latest transactions from the blockchain.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Empty request
//
// Returns:
//   - *pb.Reply: Response containing the latest transactions data
//   - error: Error if the operation fails
func (s *DemoService) GetLatestTransactions(ctx context.Context, req *pb.LimitRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return s.errorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	// Validate limit (maximum 1000 transactions per request)
	const maxLimit = 1000
	if err := ValidateLimit(req.Limit, maxLimit); err != nil {
		return s.errorReply(http.StatusBadRequest, "invalid limit", err), err
	}

	global.Logger.Infof("get latest transactions with limit=%d", req.Limit)

	client, err := s.getExplorerClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
	}

	// Use context with longer timeout for gRPC operations
	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	transactions, err := client.GetTxs(callCtx, &injective_explorer_rpcpb.GetTxsRequest{Limit: req.Limit})
	if err != nil {
		global.Logger.Errorf("get transactions error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get transactions error", err), err
	}

	data, err := s.marshalToJSON(transactions)
	if err != nil {
		global.Logger.Errorf("marshal transactions error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "marshal transactions error", err), err
	}

	return s.successReply(data), nil
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
			data, err := s.marshalToJSON(txResp)
			if err != nil {
				global.Logger.Errorf("marshal transaction error: %v", err)
				return s.errorReply(http.StatusInternalServerError, "marshal transaction error", err), err
			}
			return s.successReply(data), nil
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
	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	transaction, err := client.GetTxByTxHash(callCtx, hash)
	if err != nil {
		global.Logger.Errorf("get transaction error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get transaction error", err), err
	}

	data, err := s.marshalToJSON(transaction)
	if err != nil {
		global.Logger.Errorf("marshal transaction error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "marshal transaction error", err), err
	}

	// Save to database for future queries
	err = db.Get().Create(models.TransactionFromProto(transaction.Data)).Error
	if err != nil {
		global.Logger.Errorf("create transaction error: %v", err)
		// Don't return error, just log it since we already have the data
	}

	return s.successReply(data), nil
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

// GetAccountTransactions retrieves transactions for a given account address.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing the account address
//
// Returns:
//   - *pb.Reply: Response containing the account transactions
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

	// Validate limit if provided
	if req.Limit > 0 {
		const maxLimit = 1000
		if err := ValidateLimit(req.Limit, maxLimit); err != nil {
			return s.errorReply(http.StatusBadRequest, "invalid limit", err), err
		}
	}

	global.Logger.Infof("get account transactions: address=%s, limit=%d", address, req.Limit)

	client, err := s.getExplorerClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "create explorer client error", err), err
	}

	// Use context with longer timeout for gRPC operations
	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	transactions, err := client.GetAccountTxs(callCtx, &injective_explorer_rpcpb.GetAccountTxsRequest{
		Address: address,
		Limit:   req.Limit,
	})
	if err != nil {
		global.Logger.Errorf("get account transactions error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get account transactions error", err), err
	}

	data, err := s.marshalToJSON(transactions)
	if err != nil {
		global.Logger.Errorf("marshal transactions error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "marshal transactions error", err), err
	}

	for _, transaction := range transactions.Data {
		err = db.Get().Create(models.TransactionFromProto(transaction)).Error
		if err != nil {
			global.Logger.Errorf("create transaction error: %v", err)
			return s.errorReply(http.StatusInternalServerError, "create transaction error", err), err
		}
	}

	return s.successReply(data), nil
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

	client, err := s.getChainClient()
	if err != nil {
		return s.errorReply(http.StatusInternalServerError, "build chain client error", err), err
	}

	balances, err := client.GetBankBalances(ctx, address)
	if err != nil {
		global.Logger.Errorf("get account balances error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "get account balances error", err), err
	}

	data, err := s.marshalToJSON(balances)
	if err != nil {
		global.Logger.Errorf("marshal balances error: %v", err)
		return s.errorReply(http.StatusInternalServerError, "marshal balances error", err), err
	}

	for _, balance := range balances.Balances {
		err = db.Get().Create(models.AccountBalanceFromProto(balance, address)).Error
		if err != nil {
			global.Logger.Errorf("create account balance error: %v", err)
			return s.errorReply(http.StatusInternalServerError, "create account balance error", err), err
		}
	}

	return s.successReply(data), nil
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
	const maxErrorLength = 200
	if len(errMsg) > maxErrorLength {
		errMsg = errMsg[:maxErrorLength] + "..."
	}

	return errMsg
}

// getExplorerClient returns a cached or newly created explorer client.
// This method is thread-safe and uses lazy initialization for better performance.
// The client is created once and reused across requests to avoid connection overhead.
//
// Returns:
//   - ExplorerClient: The explorer client instance
//   - error: Error if client creation fails
func (s *DemoService) getExplorerClient() (explorerclient.ExplorerClient, error) {
	s.explorerClientOnce.Do(func() {
		global.Logger.Infof("initializing explorer client (one-time setup)")
		client, err := explorerclient.NewExplorerClient(global.ChainConfig)
		if err != nil {
			global.Logger.Errorf("create explorer client error: %v", err)
			s.explorerClientErr = errors.Wrap(err, "create explorer client error")
			return
		}
		s.explorerClient = client
	})

	if s.explorerClientErr != nil {
		return nil, s.explorerClientErr
	}
	return s.explorerClient, nil
}

// getChainClient returns a cached or newly created chain client.
// This method is thread-safe and uses lazy initialization for better performance.
// The client is created once and reused across requests to avoid connection overhead.
//
// Returns:
//   - ChainClient: The chain client instance
//   - error: Error if client creation fails
func (s *DemoService) getChainClient() (chainclient.ChainClientV2, error) {
	s.chainClientOnce.Do(func() {
		global.Logger.Infof("initializing chain client (one-time setup)")
		client, err := buildChainClientV2Query()
		if err != nil {
			global.Logger.Errorf("build chain client error: %v", err)
			s.chainClientErr = errors.Wrap(err, "build chain client error")
			return
		}
		s.chainClient = client
	})

	if s.chainClientErr != nil {
		return nil, s.chainClientErr
	}
	return s.chainClient, nil
}

// getTMClient returns a cached or newly created Tendermint client.
// This method is thread-safe and uses lazy initialization for better performance.
// The client is created once and reused across requests.
//
// Returns:
//   - TMClient: The Tendermint client instance
func (s *DemoService) getTMClient() tmclient.TendermintClient {
	s.tmClientOnce.Do(func() {
		s.tmClient = tmclient.NewRPCClient(global.ChainConfig.TmEndpoint)
	})
	return s.tmClient
}

// buildChainClientV2Query creates a new ChainClientV2 for query operations.
// This client does not require a keyring as it is only used for queries, not transactions.
//
// Returns:
//   - chainclient.ChainClientV2: A configured chain client
//   - error: Error if client creation fails
func buildChainClientV2Query() (chainclient.ChainClientV2, error) {
	// Create Tendermint RPC client
	tmClient, err := rpchttp.New(global.ChainConfig.TmEndpoint)
	if err != nil {
		global.Logger.Errorf("create tm client error: %v", err)
		return nil, errors.Wrap(err, "create tm client error")
	}

	// Create client context (no keyring needed for queries)
	// Note: This client cannot sign transactions, only perform queries
	clientCtx, err := chainclient.NewClientContext(
		global.ChainConfig.ChainId,
		"",  // Sender address can be empty (query only)
		nil, // Keyring can be nil (query only)
	)
	if err != nil {
		return nil, errors.Wrap(err, "create client context error")
	}

	// Configure node URI and RPC client
	clientCtx = clientCtx.WithNodeURI(global.ChainConfig.TmEndpoint).WithClient(tmClient)

	// Create ChainClientV2
	chainClient, err := chainclient.NewChainClientV2(
		clientCtx,
		global.ChainConfig,
		common.OptionGasPrices(client.DefaultGasPriceWithDenom),
	)
	if err != nil {
		return nil, errors.Wrap(err, "create chain client error")
	}

	return chainClient, nil
}
