package chain

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
)

// CircuitBreaker implements a circuit breaker pattern for RPC calls
type CircuitBreaker struct {
	failures        int
	lastFailureTime time.Time
	state           string // "closed", "open", "half-open"
	mu              sync.Mutex
	failureThreshold int
	recoveryTimeout  time.Duration
}

// EVMConfig holds EVM client configuration
type EVMConfig struct {
	RPCURL    string
	ChainID   *big.Int
	Network   string
	Timeout   time.Duration
}

// EVMClient wraps go-ethereum ethclient for Injective EVM queries
type EVMClient struct {
	client          *ethclient.Client
	rpcURL         string
	chainID        *big.Int
	network        string
	circuitBreaker *CircuitBreaker
}

// NewEVMClientWithConfig creates a new EVM client with custom configuration
func NewEVMClientWithConfig(config *EVMConfig) (*EVMClient, error) {
	if config == nil {
		return nil, errors.New("EVMConfig cannot be nil")
	}

	if config.RPCURL == "" {
		return nil, errors.New("RPCURL cannot be empty")
	}

	if config.ChainID == nil {
		return nil, errors.New("ChainID cannot be nil")
	}

	client, err := ethclient.Dial(config.RPCURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to Injective EVM RPC: %s", config.RPCURL)
	}

	return &EVMClient{
		client:          client,
		rpcURL:          config.RPCURL,
		chainID:         config.ChainID,
		network:         config.Network,
		circuitBreaker:  newCircuitBreaker(5, 30*time.Second),
	}, nil
}

// NewEVMClient creates a new EVM client for Injective network with default settings
// This is a convenience function for backward compatibility
// network: "mainnet" or "testnet"
func NewEVMClient(network string) (*EVMClient, error) {
	var rpcURL string
	var chainID *big.Int

	switch network {
	case "mainnet":
		rpcURL = "https://sentry.evm-rpc.injective.network/"
		chainID = big.NewInt(1776)
	case "testnet":
		rpcURL = "https://k8s.testnet.json-rpc.injective.network/"
		chainID = big.NewInt(1439)
	default:
		return nil, fmt.Errorf("unsupported network: %s (must be 'mainnet' or 'testnet')", network)
	}

	return NewEVMClientWithConfig(&EVMConfig{
		RPCURL:  rpcURL,
		ChainID: chainID,
		Network: network,
		Timeout: 30 * time.Second,
	})
}

// Close closes the underlying client connection
func (c *EVMClient) Close() {
	if c.client != nil {
		c.client.Close()
	}
}

// GetChainID returns the chain ID
func (c *EVMClient) GetChainID() *big.Int {
	return c.chainID
}

// GetNetwork returns the network name
func (c *EVMClient) GetNetwork() string {
	return c.network
}

// GetBlockNumber gets the latest block number
func (c *EVMClient) GetBlockNumber(ctx context.Context) (*big.Int, error) {
	blockNumber, err := c.client.BlockNumber(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get block number")
	}
	return big.NewInt(int64(blockNumber)), nil
}

// GetBlockByNumber gets a block by block number
func (c *EVMClient) GetBlockByNumber(ctx context.Context, blockNumber *big.Int) (*types.Block, error) {
	block, err := c.client.BlockByNumber(ctx, blockNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get block by number: %s", blockNumber.String())
	}
	return block, nil
}

// GetBlockByHash gets a block by block hash
func (c *EVMClient) GetBlockByHash(ctx context.Context, blockHash common.Hash) (*types.Block, error) {
	block, err := c.client.BlockByHash(ctx, blockHash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get block by hash: %s", blockHash.Hex())
	}
	return block, nil
}

// GetTransactionByHash gets a transaction by hash
func (c *EVMClient) GetTransactionByHash(ctx context.Context, txHash common.Hash) (*types.Transaction, bool, error) {
	tx, isPending, err := c.client.TransactionByHash(ctx, txHash)
	if err != nil {
		return nil, false, errors.Wrapf(err, "failed to get transaction by hash: %s", txHash.Hex())
	}
	return tx, isPending, nil
}

// GetTransactionReceiptByHash gets a transaction receipt by hash
func (c *EVMClient) GetTransactionReceiptByHash(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	receipt, err := c.client.TransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get transaction receipt by hash: %s", txHash.Hex())
	}
	return receipt, nil
}

// GetTransactionSender gets the sender address of a transaction
func (c *EVMClient) GetTransactionSender(ctx context.Context, tx *types.Transaction, blockNumber *big.Int) (common.Address, error) {
	signer := types.LatestSignerForChainID(tx.ChainId())
	sender, err := types.Sender(signer, tx)
	if err != nil {
		return common.Address{}, errors.Wrap(err, "failed to get transaction sender")
	}
	return sender, nil
}

// GetTransactionReceipt gets the transaction receipt
func (c *EVMClient) GetTransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	receipt, err := c.client.TransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get transaction receipt: %s", txHash.Hex())
	}
	return receipt, nil
}

// GetBalance gets the balance of an address
func (c *EVMClient) GetBalance(ctx context.Context, address common.Address, blockNumber *big.Int) (*big.Int, error) {
	balance, err := c.client.BalanceAt(ctx, address, blockNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get balance for address: %s", address.Hex())
	}
	return balance, nil
}

// GetCode gets the contract code at an address
func (c *EVMClient) GetCode(ctx context.Context, address common.Address, blockNumber *big.Int) ([]byte, error) {
	code, err := c.client.CodeAt(ctx, address, blockNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get code for address: %s", address.Hex())
	}
	return code, nil
}

// IsContract checks if an address is a contract
func (c *EVMClient) IsContract(ctx context.Context, address common.Address) (bool, error) {
	code, err := c.GetCode(ctx, address, nil) // nil means latest block
	if err != nil {
		return false, err
	}
	return len(code) > 0, nil
}

// GetTransactionCount gets the nonce (transaction count) for an address
func (c *EVMClient) GetTransactionCount(ctx context.Context, address common.Address, blockNumber *big.Int) (uint64, error) {
	nonce, err := c.client.NonceAt(ctx, address, blockNumber)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get transaction count for address: %s", address.Hex())
	}
	return nonce, nil
}

// GetStorageAt gets the storage value at a specific position
func (c *EVMClient) GetStorageAt(ctx context.Context, address common.Address, position common.Hash, blockNumber *big.Int) ([]byte, error) {
	storage, err := c.client.StorageAt(ctx, address, position, blockNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get storage at address: %s, position: %s", address.Hex(), position.Hex())
	}
	return storage, nil
}

// GetLogs gets logs matching the filter criteria
func (c *EVMClient) GetLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	logs, err := c.client.FilterLogs(ctx, query)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get logs")
	}
	return logs, nil
}

// GetGasPrice gets the current gas price
func (c *EVMClient) GetGasPrice(ctx context.Context) (*big.Int, error) {
	gasPrice, err := c.client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get gas price")
	}
	return gasPrice, nil
}

// GetHeaderByNumber gets a block header by number
func (c *EVMClient) GetHeaderByNumber(ctx context.Context, blockNumber *big.Int) (*types.Header, error) {
	header, err := c.client.HeaderByNumber(ctx, blockNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get header by number: %s", blockNumber.String())
	}
	return header, nil
}

// GetHeaderByHash gets a block header by hash
func (c *EVMClient) GetHeaderByHash(ctx context.Context, blockHash common.Hash) (*types.Header, error) {
	header, err := c.client.HeaderByHash(ctx, blockHash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get header by hash: %s", blockHash.Hex())
	}
	return header, nil
}

// GetPendingNonceAt gets the pending nonce for an address
func (c *EVMClient) GetPendingNonceAt(ctx context.Context, address common.Address) (uint64, error) {
	nonce, err := c.client.PendingNonceAt(ctx, address)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get pending nonce for address: %s", address.Hex())
	}
	return nonce, nil
}

// GetPendingBalanceAt gets the pending balance for an address
func (c *EVMClient) GetPendingBalanceAt(ctx context.Context, address common.Address) (*big.Int, error) {
	balance, err := c.client.PendingBalanceAt(ctx, address)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get pending balance for address: %s", address.Hex())
	}
	return balance, nil
}

// CallContract calls a contract method (read-only)
func (c *EVMClient) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	result, err := c.client.CallContract(ctx, call, blockNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to call contract at address: %s", call.To.Hex())
	}
	return result, nil
}

// EstimateGas estimates the gas needed for a transaction
func (c *EVMClient) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	gas, err := c.client.EstimateGas(ctx, call)
	if err != nil {
		return 0, errors.Wrap(err, "failed to estimate gas")
	}
	return gas, nil
}

// SubscribeNewHead subscribes to new block headers
func (c *EVMClient) SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
	sub, err := c.client.SubscribeNewHead(ctx, ch)
	if err != nil {
		return nil, errors.Wrap(err, "failed to subscribe to new headers")
	}
	return sub, nil
}

// SubscribeFilterLogs subscribes to logs matching the filter
func (c *EVMClient) SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	sub, err := c.client.SubscribeFilterLogs(ctx, query, ch)
	if err != nil {
		return nil, errors.Wrap(err, "failed to subscribe to filter logs")
	}
	return sub, nil
}

// GetEVMHashFromCosmosHash converts a Cosmos transaction hash to EVM transaction hash.
// In Injective, a Cosmos transaction may contain an EVM transaction, but they have different hashes.
// This function queries the block to find the corresponding EVM transaction hash.
//
// Parameters:
//   - ctx: Context for the request
//   - cosmosHash: Cosmos transaction hash (with or without 0x prefix)
//   - blockNumber: Block number containing the transaction (required, must be > 0)
//
// Returns:
//   - string: EVM transaction hash (with 0x prefix), or empty string if not found
//   - error: Error if the operation fails
func (c *EVMClient) GetEVMHashFromCosmosHash(ctx context.Context, cosmosHash string, blockNumber uint64) (string, error) {
	// Block number is required to locate the EVM transaction.
	// Database lookup for block number is not yet implemented.
	if blockNumber == 0 {
		return "", fmt.Errorf("block number is required to convert Cosmos hash to EVM hash")
	}

	// Get EVM block
	block, err := c.GetBlockByNumber(ctx, big.NewInt(int64(blockNumber)))
	if err != nil {
		return "", errors.Wrapf(err, "failed to get block %d", blockNumber)
	}

	// In Injective, typically one Cosmos transaction contains one EVM transaction
	// If there's only one EVM transaction in the block, return it
	// Otherwise, we need more sophisticated matching logic
	if len(block.Transactions()) == 1 {
		return block.Transactions()[0].Hash().Hex(), nil
	}

	// If multiple transactions, we need to match by some criteria.
	// Currently returns the first EVM transaction as a fallback.
	// Better matching logic (e.g., by comparing transaction data) is not yet implemented.
	if len(block.Transactions()) > 0 {
		// Return the first EVM transaction as a fallback.
		// In practice, matching by other criteria may be needed.
		return block.Transactions()[0].Hash().Hex(), nil
	}

	return "", fmt.Errorf("no EVM transactions found in block %d", blockNumber)
}

// GetKnownContracts returns known contract addresses on Injective
func (c *EVMClient) GetKnownContracts() map[string]common.Address {
	contracts := make(map[string]common.Address)

	if c.network == "mainnet" {
		// Mainnet contracts from Injective docs
		contracts["USDT"] = common.HexToAddress("0x88f7F2b685F9692caf8c478f5BADF09eE9B1Cc13")
		contracts["wETH"] = common.HexToAddress("0x83A15000b753AC0EeE06D2Cb41a69e76D0D5c7F7")
		contracts["wINJ"] = common.HexToAddress("0x0000000088827d2d103ee2d9A6b781773AE03FfB")
		contracts["USDC"] = common.HexToAddress("0x2a25fbD67b3aE485e461fe55d9DbeF302B7D3989")
		contracts["MultiCall"] = common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11")
	} else if c.network == "testnet" {
		// Testnet contracts from Injective docs
		contracts["wINJ"] = common.HexToAddress("0x0000000088827d2d103ee2d9A6b781773AE03FfB")
		contracts["USDT"] = common.HexToAddress("0xaDC7bcB5d8fe053Ef19b4E0C861c262Af6e0db60")
	}

	return contracts
}

// GetERC20Decimals queries the decimals() function of an ERC20 token contract.
// Returns the number of decimal places for the token (e.g., 18 for most tokens, 6 for USDC/USDT).
// If the query fails or the contract doesn't implement decimals(), returns 18 as default.
//
// Parameters:
//   - ctx: Context for the request
//   - contractAddress: The ERC20 token contract address
//
// Returns:
//   - int: The number of decimal places
//   - error: Error if the call fails (returns 18 as default on error)
func (c *EVMClient) GetERC20Decimals(ctx context.Context, contractAddress string) (int, error) {
	// ERC20 decimals() function signature: 0x313ce567
	data := []byte{0x31, 0x3c, 0xe5, 0x67}
	addr := common.HexToAddress(contractAddress)

	result, err := c.client.CallContract(ctx, ethereum.CallMsg{
		To:   &addr,
		Data: data,
	}, nil)
	if err != nil {
		return 18, errors.Wrapf(err, "failed to query decimals for contract %s", contractAddress)
	}

	// Decode result (uint256)
	if len(result) == 0 {
		return 18, nil // Contract doesn't implement decimals(), return default
	}

	decimals := new(big.Int).SetBytes(result).Int64()
	if decimals < 0 || decimals > 255 {
		return 18, nil // Invalid value, return default
	}

	return int(decimals), nil
}

// newCircuitBreaker creates a new circuit breaker with default settings
func newCircuitBreaker(failureThreshold int, recoveryTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		failures:         0,
		state:            "closed",
		failureThreshold: failureThreshold,
		recoveryTimeout:  recoveryTimeout,
	}
}

// Execute executes a function through the circuit breaker
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Check if circuit breaker is in open state
	if cb.state == "open" {
		// Check if we should transition to half-open
		if time.Since(cb.lastFailureTime) > cb.recoveryTimeout {
			cb.state = "half-open"
		} else {
			return errors.New("circuit breaker is open")
		}
	}

	// Execute the function
	err := fn()

	if err != nil {
		// Record failure
		cb.failures++
		cb.lastFailureTime = time.Now()

		// Check if we should open the circuit
		if cb.failures >= cb.failureThreshold {
			cb.state = "open"
		}
		return err
	}

	// Success - reset failure count and close circuit if in half-open
	cb.failures = 0
	if cb.state == "half-open" {
		cb.state = "closed"
	}

	return nil
}

// ChainID returns the chain ID (convenience method for accessing chainID field)
func (c *EVMClient) ChainID() *big.Int {
	return c.chainID
}

// RecoverSender recovers the sender address from transaction signature components (v, r, s).
// This is used when we have raw transaction data but the from address was not recovered.
//
// Parameters:
//   - ctx: Context for the request
//   - evmHash: EVM transaction hash
//   - v, r, s: Signature components as hex strings
//
// Returns:
//   - string: Recovered sender address
//   - error: Error if recovery fails
func (c *EVMClient) RecoverSender(ctx context.Context, evmHash string, v, r, s string) (string, error) {
	// Parse signature components
	vInt := new(big.Int)
	if _, ok := vInt.SetString(v, 0); !ok {
		return "", errors.New("invalid v value")
	}

	rBytes := common.FromHex(r)
	if rBytes == nil {
		return "", errors.New("invalid r value")
	}

	sBytes := common.FromHex(s)
	if sBytes == nil {
		return "", errors.New("invalid s value")
	}

	// Fetch the transaction again to get the full transaction object
	txHash := common.HexToHash(evmHash)
	tx, isPending, err := c.client.TransactionByHash(ctx, txHash)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get transaction %s", evmHash)
	}
	if isPending {
		return "", errors.New("cannot recover sender for pending transaction")
	}

	// Recover sender from transaction
	signer := types.LatestSignerForChainID(tx.ChainId())
	sender, err := types.Sender(signer, tx)
	if err != nil {
		return "", errors.Wrapf(err, "failed to recover sender from transaction %s", evmHash)
	}

	return sender.Hex(), nil
}

// GetTransactionDetailsBatch gets transaction details for multiple transactions in batch.
// This significantly reduces RPC calls by batching requests for transactions, receipts, and blocks.
// Returns a slice of TransactionDetails and a slice of errors (one per transaction).
// If batch call fails, automatically falls back to individual GetTransactionDetails calls.
func (c *EVMClient) GetTransactionDetailsBatch(ctx context.Context, txHashes []common.Hash) ([]*TransactionDetails, []error) {
	if len(txHashes) == 0 {
		return nil, nil
	}

	results := make([]*TransactionDetails, len(txHashes))
	errs := make([]error, len(txHashes))

	// Try batch RPC call first
	batchResults, batchErrs, batchCallErr := c.batchGetTransactionDetailsInternal(ctx, txHashes)
	if batchCallErr == nil {
		// Batch call succeeded
		copy(results, batchResults)
		copy(errs, batchErrs)
		return results, errs
	}

	// Batch call failed, fall back to individual calls
	return c.fallbackToIndividualCalls(ctx, txHashes)
}

// batchGetTransactionDetailsInternal performs batch RPC calls to fetch transaction details.
// Returns results, errors per transaction, and an error if the batch call itself fails.
func (c *EVMClient) batchGetTransactionDetailsInternal(ctx context.Context, txHashes []common.Hash) ([]*TransactionDetails, []error, error) {
	results := make([]*TransactionDetails, len(txHashes))
	errs := make([]error, len(txHashes))

	// Create RPC client for batch calls
	rpcClient, err := rpc.DialContext(ctx, c.rpcURL)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to dial RPC for batch call")
	}
	defer rpcClient.Close()

	// Phase 1: Batch get transactions and receipts
	txs, receipts, err := c.batchGetTransactionsAndReceipts(ctx, rpcClient, txHashes)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to batch get transactions and receipts")
	}

	// Phase 2: Collect unique block numbers needed
	blockNumbers := make(map[uint64]bool)
	for i, receipt := range receipts {
		if receipt != nil && errs[i] == nil {
			blockNumbers[receipt.BlockNumber.Uint64()] = true
		}
	}

	// Phase 3: Batch get blocks
	blocks, err := c.batchGetBlocks(ctx, rpcClient, blockNumbers)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to batch get blocks")
	}

	// Phase 4: Get current block number (only once)
	currentBlockNumber, err := c.client.BlockNumber(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get current block number")
	}

	// Phase 5: Build TransactionDetails for each transaction
	for i := range txHashes {
		if txs[i] == nil || receipts[i] == nil {
			errs[i] = fmt.Errorf("transaction or receipt not found")
			continue
		}

		block := blocks[receipts[i].BlockNumber.Uint64()]
		if block == nil {
			errs[i] = fmt.Errorf("block not found")
			continue
		}

		// Build transaction details
		results[i], errs[i] = c.buildTransactionDetailsFromComponents(
			txHashes[i], txs[i], receipts[i], block, currentBlockNumber,
		)
	}

	return results, errs, nil
}

// batchGetTransactionsAndReceipts uses BatchCallContext to fetch transactions and receipts.
func (c *EVMClient) batchGetTransactionsAndReceipts(ctx context.Context, rpcClient *rpc.Client, txHashes []common.Hash) ([]*types.Transaction, []*types.Receipt, error) {
	// Build batch requests
	batchElems := make([]rpc.BatchElem, len(txHashes)*2)
	txResults := make([]*types.Transaction, len(txHashes))
	receiptResults := make([]*types.Receipt, len(txHashes))

	for i, hash := range txHashes {
		batchElems[i*2] = rpc.BatchElem{
			Method: "eth_getTransactionByHash",
			Args:   []interface{}{hash},
			Result: &txResults[i],
		}
		batchElems[i*2+1] = rpc.BatchElem{
			Method: "eth_getTransactionReceipt",
			Args:   []interface{}{hash},
			Result: &receiptResults[i],
		}
	}

	// Execute batch call
	err := rpcClient.BatchCallContext(ctx, batchElems)
	if err != nil {
		return nil, nil, err
	}

	// Check individual errors
	for i := range batchElems {
		if batchElems[i].Error != nil {
			// Log but don't fail the entire batch
			if i%2 == 0 {
				txResults[i/2] = nil
			} else {
				receiptResults[i/2] = nil
			}
		}
	}

	return txResults, receiptResults, nil
}

// batchGetBlocks fetches multiple blocks in batch.
func (c *EVMClient) batchGetBlocks(ctx context.Context, rpcClient *rpc.Client, blockNumbers map[uint64]bool) (map[uint64]*types.Block, error) {
	if len(blockNumbers) == 0 {
		return make(map[uint64]*types.Block), nil
	}

	// Convert map to slice
	numbers := make([]uint64, 0, len(blockNumbers))
	for num := range blockNumbers {
		numbers = append(numbers, num)
	}

	// Build batch requests for blocks
	batchElems := make([]rpc.BatchElem, len(numbers))
	blockResults := make([]*types.Block, len(numbers))

	for i, num := range numbers {
		batchElems[i] = rpc.BatchElem{
			Method: "eth_getBlockByNumber",
			Args:   []interface{}{fmt.Sprintf("0x%x", num), true}, // true = include transactions
			Result: &blockResults[i],
		}
	}

	// Execute batch call
	err := rpcClient.BatchCallContext(ctx, batchElems)
	if err != nil {
		return nil, err
	}

	// Build result map
	blocks := make(map[uint64]*types.Block)
	for i, num := range numbers {
		if batchElems[i].Error == nil && blockResults[i] != nil {
			blocks[num] = blockResults[i]
		}
	}

	return blocks, nil
}

// buildTransactionDetailsFromComponents builds TransactionDetails from individual components.
// This is extracted from getTransactionDetailsInternal to enable reuse in batch operations.
// It uses pre-fetched components (tx, receipt, block) instead of making RPC calls.
func (c *EVMClient) buildTransactionDetailsFromComponents(
	txHash common.Hash,
	tx *types.Transaction,
	receipt *types.Receipt,
	block *types.Block,
	currentBlockNumber uint64,
) (*TransactionDetails, error) {
	// Check for pending transaction
	if receipt == nil || block == nil {
		return nil, fmt.Errorf("transaction is still pending or not found")
	}

	// Recover From address
	signer := types.LatestSignerForChainID(tx.ChainId())
	from, err := types.Sender(signer, tx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to recover sender")
	}

	// Extract method signature
	method := ""
	if len(tx.Data()) >= 4 {
		method = "0x" + hex.EncodeToString(tx.Data()[:4])
	}

	// Calculate gas price and fee
	gasPrice := tx.GasPrice()
	var baseFee, maxFeePerGas, maxPriorityFee *big.Int

	// Check if this is an EIP-1559 transaction
	if tx.Type() == types.DynamicFeeTxType {
		maxFeePerGas = tx.GasFeeCap()
		maxPriorityFee = tx.GasTipCap()
		// Base fee must be obtained from block header
		// Use recovery to handle corrupted block objects with nil headers
		SafeExecuteWithTxHash(
			txHash,
			"accessing block.BaseFee()",
			"using tx.GasPrice()",
			func() {
				if block != nil && block.BaseFee() != nil {
					baseFee = block.BaseFee()
					// Actual gas price paid = baseFee + priorityFee
					gasPrice = new(big.Int).Add(baseFee, maxPriorityFee)
				}
			},
		)
	}

	// Calculate transaction fee
	transactionFee := new(big.Int).Mul(
		big.NewInt(int64(receipt.GasUsed)),
		gasPrice,
	)

	// Calculate gas usage percentage
	var gasUsedPercent float64
	if tx.Gas() > 0 {
		gasUsedPercent = float64(receipt.GasUsed) / float64(tx.Gas()) * 100
	}

	// Build log information
	logs := make([]LogInfo, len(receipt.Logs))
	for i, log := range receipt.Logs {
		topics := make([]string, len(log.Topics))
		for j, topic := range log.Topics {
			topics[j] = topic.Hex()
		}
		logs[i] = LogInfo{
			Address: log.Address.Hex(),
			Topics:  topics,
			Data:    hex.EncodeToString(log.Data),
			Index:   log.Index,
			Removed: log.Removed,
		}
	}

	// Determine To address
	to := ""
	if tx.To() != nil {
		to = tx.To().Hex()
	}

	// For contract creation, use contract address from receipt
	contractAddress := ""
	if receipt.ContractAddress != (common.Address{}) {
		contractAddress = receipt.ContractAddress.Hex()
		// If To is empty and we have a contract address, use it
		if to == "" {
			to = contractAddress
		}
	}

	// Determine status
	status := "Failed"
	if receipt.Status == 1 {
		status = "Success"
	}

	// Build result
	// Use recovery to handle corrupted block objects with nil headers
	var timestamp time.Time
	SafeExecuteWithTxHash(
		txHash,
		"accessing block.Time()",
		"using zero time value",
		func() {
			if block != nil {
				timestamp = time.Unix(int64(block.Time()), 0)
			}
		},
	)

	details := &TransactionDetails{
		Hash:             txHash.Hex(),
		Status:           status,
		Method:           method,
		BlockNumber:      receipt.BlockNumber.Uint64(),
		BlockHash:        receipt.BlockHash.Hex(),
		Confirmations:    currentBlockNumber - receipt.BlockNumber.Uint64(),
		Timestamp:        timestamp,
		From:             from.Hex(),
		To:               to,
		ContractAddress:  contractAddress,
		Value:            tx.Value().String(),
		TransactionFee:   transactionFee.String(),
		GasPrice:         gasPrice.String(),
		GasLimit:         tx.Gas(),
		GasUsed:          receipt.GasUsed,
		GasUsedPercent:   gasUsedPercent,
		Nonce:            tx.Nonce(),
		Input:            hex.EncodeToString(tx.Data()),
		TransactionIndex: receipt.TransactionIndex,
		Logs:             logs,
		ParseStatus:      "success",
	}

	// Add EIP-1559 related fields
	if baseFee != nil {
		details.BaseFee = baseFee.String()
	}
	if maxFeePerGas != nil {
		details.MaxFeePerGas = maxFeePerGas.String()
	}
	if maxPriorityFee != nil {
		details.MaxPriorityFee = maxPriorityFee.String()
	}

	return details, nil
}

// fallbackToIndividualCalls falls back to individual GetTransactionDetails calls when batch fails.
func (c *EVMClient) fallbackToIndividualCalls(ctx context.Context, txHashes []common.Hash) ([]*TransactionDetails, []error) {
	results := make([]*TransactionDetails, len(txHashes))
	errs := make([]error, len(txHashes))

	// Use a semaphore to limit concurrency
	sem := make(chan struct{}, 5) // Max 5 concurrent requests
	var wg sync.WaitGroup

	for i, hash := range txHashes {
		wg.Add(1)
		go func(idx int, txHash common.Hash) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release

			results[idx], errs[idx] = c.GetTransactionDetails(ctx, txHash)
		}(i, hash)
	}

	wg.Wait()
	return results, errs
}
