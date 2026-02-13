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

	// Build filter query
	// For indexed parameters, we need to use the hash of the value
	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(eventHeight)),
		ToBlock:   big.NewInt(int64(eventHeight + 10)), // Search window
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

	return "", fmt.Errorf("SendToCosmosEvent not found for event_nonce=%d", eventNonce)
}

// GetWithdrawalBatchTxHash queries the EVM chain for the TransactionBatchExecutedEvent
// corresponding to a withdrawal batch.
//
// Parameters:
//   - ctx: context
//   - batchNonce: the batch nonce from the withdrawal claim
//   - denom: the denom in format "peggy0x..."
//
// Returns the EVM transaction hash, or an error if not found.
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

	// Build filter query
	// For indexed parameters, we need to use the hash of the value
	// Note: We don't set FromBlock/ToBlock to search all blocks
	// In production, you might want to limit the search range
	query := ethereum.FilterQuery{
		Topics: [][]common.Hash{
			{TransactionBatchExecutedEventSig},
			{common.BigToHash(big.NewInt(int64(batchNonce)))},
			{common.HexToHash(tokenContract)},
		},
	}

	// Query logs
	logs, err := c.client.FilterLogs(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to query logs: %w", err)
	}

	// Return the first match (should be unique)
	if len(logs) > 0 {
		return logs[0].TxHash.Hex(), nil
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
// Format: "peggy0x..." -> "0x..."
func ParseTokenContractFromDenom(denom string) (string, error) {
	if !strings.HasPrefix(denom, "peggy") {
		return "", fmt.Errorf("not a peggy denom: %s", denom)
	}

	addr := strings.TrimPrefix(denom, "peggy")

	// Validate address format
	if !common.IsHexAddress(addr) {
		return "", fmt.Errorf("invalid address format: %s", addr)
	}

	// Normalize to lowercase with 0x prefix
	return normalizeHash(addr), nil
}

// normalizeHash normalizes an Ethereum address to lowercase with 0x prefix.
func normalizeHash(addr string) string {
	if len(addr) >= 2 && addr[0:2] == "0x" {
		addr = addr[2:]
	}
	return "0x" + strings.ToLower(addr)
}
