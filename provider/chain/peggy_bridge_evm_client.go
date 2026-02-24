package chain

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/time/rate"
)

// PeggyBridgeEVMClient connects to the Peggy-bridged EVM chain (Sepolia/Mainnet)
// to query cross-chain transaction EVM tx hashes.
type PeggyBridgeEVMClient struct {
	client    *ethclient.Client
	chainID   *big.Int
	timeout   time.Duration
	rateLimit *rate.Limiter
}

// Config defines the configuration for PeggyBridgeEVMClient.
type Config struct {
	RPCEndpoint string        // JSON-RPC endpoint (e.g., https://rpc.sepolia.org)
	ChainID     int64         // Chain ID (11155111=Sepolia, 1=Mainnet)
	BridgeContract string     // Optional: bridge contract address for filtering
	Timeout     time.Duration // Request timeout
	RateLimit   int           // Max requests per second
}

// NewPeggyBridgeEVMClient creates a new Peggy bridge EVM client.
func NewPeggyBridgeEVMClient(cfg Config) (*PeggyBridgeEVMClient, error) {
	client, err := ethclient.Dial(cfg.RPCEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum RPC: %w", err)
	}

	// Verify chain ID
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	chainID, err := client.ChainID(ctx)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to get chain ID: %w", err)
	}

	if chainID.Int64() != cfg.ChainID {
		client.Close()
		return nil, fmt.Errorf("chain ID mismatch: expected %d, got %d", cfg.ChainID, chainID.Int64())
	}

	// Set default rate limit if not specified
	if cfg.RateLimit <= 0 {
		cfg.RateLimit = 10 // 10 requests per second
	}

	return &PeggyBridgeEVMClient{
		client:    client,
		chainID:   chainID,
		timeout:   cfg.Timeout,
		rateLimit: rate.NewLimiter(rate.Limit(cfg.RateLimit), cfg.RateLimit),
	}, nil
}

// Close closes the underlying Ethereum client.
func (c *PeggyBridgeEVMClient) Close() {
	if c.client != nil {
		c.client.Close()
	}
}

// Event signatures (computed using keccak256)
var (
	// SendToCosmosEvent(address,address,string,uint256,uint256)
	SendToCosmosEventSig = crypto.Keccak256Hash([]byte("SendToCosmosEvent(address,address,string,uint256,uint256)"))

	// TransactionBatchExecutedEvent(uint256,address,uint256)
	TransactionBatchExecutedEventSig = crypto.Keccak256Hash([]byte("TransactionBatchExecutedEvent(uint256,address,uint256)"))
)

// GetDepositTxHash queries the EVM chain for the SendToCosmosEvent
// corresponding to a deposit transaction.
//
// Parameters:
//   - ctx: context
//   - eventNonce: the event nonce from the deposit claim
//   - eventHeight: the Ethereum block height where the event occurred
//   - sender: the Ethereum sender address (0x...)
//   - denom: the denom in format "peggy0x..."
//   - amount: the amount as a string
//
// Returns the EVM transaction hash, or an error if not found.
func (c *PeggyBridgeEVMClient) GetDepositTxHash(
	ctx context.Context,
	eventNonce uint64,
	eventHeight uint64,
	sender string,
	denom string,
	amount string,
) (string, error) {
	// Wait for rate limiter
	if err := c.rateLimit.Wait(ctx); err != nil {
		return "", fmt.Errorf("rate limit: %w", err)
	}

	// Parse token contract from denom
	tokenContract, err := ParseTokenContractFromDenom(denom)
	if err != nil {
		return "", fmt.Errorf("invalid denom %s: %w", denom, err)
	}

	// Normalize addresses
	tokenContractAddr := common.HexToAddress(tokenContract)

	// Build filter query with block range
	// Search window: eventHeight to eventHeight+searchWindow (max 9990 to stay under RPC limit)
	// Wider window helps when eventHeight is approximate or from different chain semantics
	const depositSearchWindow = 5000
	toBlock := eventHeight + depositSearchWindow
	if toBlock > eventHeight+maxLogQueryBlocks {
		toBlock = eventHeight + maxLogQueryBlocks
	}

	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(eventHeight)),
		ToBlock:   big.NewInt(int64(toBlock)),
		Addresses: []common.Address{tokenContractAddr},
		Topics: [][]common.Hash{
			{SendToCosmosEventSig},
			{common.HexToHash(tokenContract)},
			{common.HexToHash(sender)},
		},
	}

	// Query logs
	logs, err := c.client.FilterLogs(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to query logs: %w", err)
	}

	// Match eventNonce and amount
	for _, log := range logs {
		if txHash, match := c.matchDepositLog(log, eventNonce, amount); match {
			return txHash, nil
		}
	}

	// Fallback: search recent blocks when eventHeight may be wrong (e.g. Cosmos block height)
	latestBlock, err := c.client.BlockNumber(ctx)
	if err != nil {
		return "", fmt.Errorf("SendToCosmosEvent not found for event_nonce=%d", eventNonce)
	}
	fromFallback := uint64(0)
	if latestBlock > maxLogQueryBlocks {
		fromFallback = latestBlock - maxLogQueryBlocks
	}
	fallbackQuery := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(fromFallback)),
		ToBlock:   big.NewInt(int64(latestBlock)),
		Addresses: []common.Address{tokenContractAddr},
		Topics: [][]common.Hash{
			{SendToCosmosEventSig},
			{common.HexToHash(tokenContract)},
			{common.HexToHash(sender)},
		},
	}
	fallbackLogs, fallbackErr := c.client.FilterLogs(ctx, fallbackQuery)
	if fallbackErr == nil {
		for _, log := range fallbackLogs {
			if txHash, match := c.matchDepositLog(log, eventNonce, amount); match {
				return txHash, nil
			}
		}
	}

	return "", fmt.Errorf("SendToCosmosEvent not found for event_nonce=%d", eventNonce)
}

// maxLogQueryBlocks is the maximum block range for FilterLogs.
// Many free-tier RPC providers (Alchemy, Infura) limit to 10,000 blocks.
// We use 9990 (instead of 10000) to stay well under the common 10k limit and provide a safety buffer.
const maxLogQueryBlocks = 9990

// maxWithdrawalSearchChunks limits how far back we search for withdrawal batches.
// 5 chunks × 9990 blocks ≈ 50k blocks (~7 days on Ethereum, ~3 days on Sepolia).
const maxWithdrawalSearchChunks = 5

// GetWithdrawalBatchTxHash queries the EVM chain for the TransactionBatchExecutedEvent
// corresponding to a withdrawal batch.
//
// Parameters:
//   - ctx: context
//   - batchNonce: the batch nonce from the withdrawal claim
//   - denom: the denom in format "peggy0x..."
//
// Returns the EVM transaction hash, or an error if not found.
// Uses chunked search (max 10k blocks per query) to comply with free-tier RPC limits.
func (c *PeggyBridgeEVMClient) GetWithdrawalBatchTxHash(
	ctx context.Context,
	batchNonce uint64,
	denom string,
) (string, error) {
	// Wait for rate limiter
	if err := c.rateLimit.Wait(ctx); err != nil {
		return "", fmt.Errorf("rate limit: %w", err)
	}

	// Parse token contract from denom
	tokenContract, err := ParseTokenContractFromDenom(denom)
	if err != nil {
		return "", fmt.Errorf("invalid denom %s: %w", denom, err)
	}

	// Chunked search: query in segments of maxLogQueryBlocks going backwards from latest
	toBlock, err := c.client.BlockNumber(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get block number: %w", err)
	}

	for chunk := 0; chunk < maxWithdrawalSearchChunks; chunk++ {
		var fromBlock uint64
		if toBlock > maxLogQueryBlocks {
			fromBlock = toBlock - maxLogQueryBlocks
		} else {
			fromBlock = 0
		}

		query := ethereum.FilterQuery{
			FromBlock: big.NewInt(int64(fromBlock)),
			ToBlock:   big.NewInt(int64(toBlock)),
			Topics: [][]common.Hash{
				{TransactionBatchExecutedEventSig},
				{common.BigToHash(big.NewInt(int64(batchNonce)))},
				{common.HexToHash(tokenContract)},
			},
		}

		logs, err := c.client.FilterLogs(ctx, query)
		if err != nil {
			return "", fmt.Errorf("failed to query logs: %w", err)
		}

		if len(logs) > 0 {
			return logs[0].TxHash.Hex(), nil
		}

		if fromBlock == 0 {
			break
		}
		toBlock = fromBlock - 1
	}

	return "", fmt.Errorf("TransactionBatchExecutedEvent not found for batch_nonce=%d", batchNonce)
}

// matchDepositLog checks if a log matches the expected eventNonce and amount.
func (c *PeggyBridgeEVMClient) matchDepositLog(
	log types.Log,
	expectedEventNonce uint64,
	expectedAmount string,
) (string, bool) {
	if len(log.Data) < 64 {
		return "", false
	}

	// Parse the log data
	// SendToCosmosEvent data layout: _destination (string) + _amount (uint256) + _eventNonce (uint256)
	// For simplicity, we skip the string parsing and just check the last 64 bytes
	// which should contain _amount (32 bytes) + _eventNonce (32 bytes)

	// Extract _eventNonce (last 32 bytes)
	eventNonceBytes := log.Data[len(log.Data)-32:]
	eventNonce := new(big.Int).SetBytes(eventNonceBytes).Uint64()

	// Extract _amount (second to last 32 bytes)
	amountBytes := log.Data[len(log.Data)-64 : len(log.Data)-32]
	amount := new(big.Int).SetBytes(amountBytes).String()

	// Match both eventNonce and amount
	if eventNonce == expectedEventNonce && amount == expectedAmount {
		return log.TxHash.Hex(), true
	}

	return "", false
}

// ParseTokenContractFromDenom extracts the token contract address from a Peggy denom.
// Accepts two formats:
// 1. "peggy0x..." - standard Peggy denom (peggy prefix + contract address)
// 2. "0x..." - raw EVM contract address (caller may pass already-parsed address)
func ParseTokenContractFromDenom(denom string) (string, error) {
	denom = strings.TrimSpace(strings.Trim(denom, "\"'"))
	if denom == "" {
		return "", fmt.Errorf("not a peggy denom: empty string")
	}

	if strings.HasPrefix(denom, "peggy") {
		addr := strings.TrimPrefix(denom, "peggy")
		if !common.IsHexAddress(addr) {
			return "", fmt.Errorf("invalid address format: %s", addr)
		}
		return normalizeHash(addr), nil
	}

	// Accept raw 0x-prefixed EVM address (40 hex chars) - caller may pass parsed tokenContract
	if len(denom) >= 42 && (denom[0:2] == "0x" || denom[0:2] == "0X") && common.IsHexAddress(denom) {
		return normalizeHash(denom), nil
	}

	return "", fmt.Errorf("not a peggy denom: %s", denom)
}

// normalizeHash normalizes an Ethereum address to lowercase with 0x prefix.
func normalizeHash(addr string) string {
	if len(addr) >= 2 && addr[0:2] == "0x" {
		addr = addr[2:]
	}
	return "0x" + strings.ToLower(addr)
}
