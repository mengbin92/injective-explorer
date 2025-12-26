package chain

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
)

// EVMClient wraps go-ethereum ethclient for Injective EVM queries
type EVMClient struct {
	client  *ethclient.Client
	rpcURL  string
	chainID *big.Int
	network string
}

// NewEVMClient creates a new EVM client for Injective network
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

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to Injective EVM RPC: %s", rpcURL)
	}

	return &EVMClient{
		client:  client,
		rpcURL:  rpcURL,
		chainID: chainID,
		network: network,
	}, nil
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
	signer := types.NewEIP155Signer(c.chainID)
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
//   - blockNumber: Block number containing the transaction (optional, if 0, will try to find from database)
//
// Returns:
//   - string: EVM transaction hash (with 0x prefix), or empty string if not found
//   - error: Error if the operation fails
func (c *EVMClient) GetEVMHashFromCosmosHash(ctx context.Context, cosmosHash string, blockNumber uint64) (string, error) {
	// Normalize Cosmos hash (remove 0x prefix if present, as Cosmos hashes typically don't have it)
	normalizedCosmosHash := cosmosHash
	if strings.HasPrefix(normalizedCosmosHash, "0x") {
		normalizedCosmosHash = normalizedCosmosHash[2:]
	}
	normalizedCosmosHash = strings.ToUpper(normalizedCosmosHash)

	// If block number is not provided, we need to find it from the database
	// For now, we'll require block number to be provided
	// TODO: Add database lookup if block number is 0
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

	// If multiple transactions, we need to match by some criteria
	// For now, return the first one (this is a limitation)
	// TODO: Implement better matching logic (e.g., by comparing transaction data)
	if len(block.Transactions()) > 0 {
		// Return the first EVM transaction as a fallback
		// In practice, you might want to match by other criteria
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
