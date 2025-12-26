package chain

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEVMClient(t *testing.T) {
	tests := []struct {
		name      string
		network   string
		wantErr   bool
		checkFunc func(t *testing.T, client *EVMClient)
	}{
		{
			name:    "mainnet client",
			network: "mainnet",
			wantErr: false,
			checkFunc: func(t *testing.T, client *EVMClient) {
				assert.NotNil(t, client)
				assert.Equal(t, "mainnet", client.GetNetwork())
				assert.Equal(t, big.NewInt(1776), client.GetChainID())
				assert.NotNil(t, client.client)
			},
		},
		{
			name:    "testnet client",
			network: "testnet",
			wantErr: false,
			checkFunc: func(t *testing.T, client *EVMClient) {
				assert.NotNil(t, client)
				assert.Equal(t, "testnet", client.GetNetwork())
				assert.Equal(t, big.NewInt(1439), client.GetChainID())
				assert.NotNil(t, client.client)
			},
		},
		{
			name:    "invalid network",
			network: "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewEVMClient(tt.network)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)
				defer client.Close()

				if tt.checkFunc != nil {
					tt.checkFunc(t, client)
				}
			}
		})
	}
}

func TestEVMClient_GetBlockNumber(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := NewEVMClient("mainnet")
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	blockNumber, err := client.GetBlockNumber(ctx)
	require.NoError(t, err)
	assert.NotNil(t, blockNumber)
	assert.Greater(t, blockNumber.Int64(), int64(0))

	fmt.Printf("Current block number: %s\n", blockNumber.String())
}

func TestEVMClient_GetBlockByNumber(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := NewEVMClient("mainnet")
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get latest block number first
	latestBlock, err := client.GetBlockNumber(ctx)
	require.NoError(t, err)

	// Get block by number
	block, err := client.GetBlockByNumber(ctx, latestBlock)
	require.NoError(t, err)
	assert.NotNil(t, block)
	assert.Equal(t, latestBlock.Uint64(), block.Number().Uint64())

	fmt.Printf("Block %d: %d transactions, hash: %s\n",
		block.Number().Uint64(),
		len(block.Transactions()),
		block.Hash().Hex())
}

func TestEVMClient_GetBalance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := NewEVMClient("mainnet")
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test with wINJ contract address (known contract on Injective)
	contracts := client.GetKnownContracts()
	wINJAddr := contracts["wINJ"]
	require.NotEqual(t, common.Address{}, wINJAddr)

	balance, err := client.GetBalance(ctx, wINJAddr, nil) // nil means latest block
	require.NoError(t, err)
	assert.NotNil(t, balance)

	fmt.Printf("wINJ contract balance: %s wei\n", balance.String())
}

func TestEVMClient_IsContract(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := NewEVMClient("mainnet")
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test with known contract
	contracts := client.GetKnownContracts()
	wINJAddr := contracts["wINJ"]
	require.NotEqual(t, common.Address{}, wINJAddr)

	isContract, err := client.IsContract(ctx, wINJAddr)
	require.NoError(t, err)
	assert.True(t, isContract, "wINJ should be a contract")

	fmt.Printf("wINJ is contract: %v\n", isContract)
}

func TestEVMClient_GetTransactionByHash(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := NewEVMClient("mainnet")
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get latest block to find a transaction
	// latestBlock, err := client.GetBlockNumber(ctx)
	// require.NoError(t, err)
	latestBlock := big.NewInt(147251781)

	block, err := client.GetBlockByNumber(ctx, latestBlock)
	require.NoError(t, err)

	if len(block.Transactions()) > 0 {
		txHash := block.Transactions()[0].Hash()

		tx, isPending, err := client.GetTransactionByHash(ctx, txHash)
		require.NoError(t, err)
		assert.NotNil(t, tx)
		assert.False(t, isPending, "transaction should not be pending")

		// Get transaction sender
		sender, err := client.GetTransactionSender(ctx, tx, block.Number())
		require.NoError(t, err)

		toAddr := "contract creation"
		if tx.To() != nil {
			toAddr = tx.To().Hex()
		}

		fmt.Printf("Transaction hash: %s, from: %s, to: %s\n",
			txHash.Hex(),
			sender.Hex(),
			toAddr)
	} else {
		t.Skip("No transactions in latest block")
	}
}

func TestEVMClient_GetEVMHashFromCosmosHash(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := NewEVMClient("mainnet")
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get latest block to find a transaction
	latestBlock, err := client.GetBlockNumber(ctx)
	require.NoError(t, err)

	block, err := client.GetBlockByNumber(ctx, latestBlock)
	require.NoError(t, err)

	if len(block.Transactions()) > 0 {
		// Get the EVM hash from the block
		evmHash := block.Transactions()[0].Hash().Hex()

		// Simulate a Cosmos hash (in practice, this would come from Blockscout or database)
		// For testing, we'll use a dummy Cosmos hash format
		// In real scenario, Cosmos hash would be like: "0x8b2543514428f2b9d0b176a1bdafe54b8a5ab0d58f22bcc233f9be695836e424"
		// or Injscan format: "E2B06516699DBCB0C2E1F3BA3E4D2484FAAEC884133FD042FA4D6194D3F2FD06"

		// Test with a Cosmos hash format (with 0x prefix, like Blockscout)
		cosmosHashWithPrefix := "0x8b2543514428f2b9d0b176a1bdafe54b8a5ab0d58f22bcc233f9be695836e424"
		evmHashFromCosmos, err := client.GetEVMHashFromCosmosHash(ctx, cosmosHashWithPrefix, latestBlock.Uint64())

		// Note: This test will only work if the cosmosHash actually corresponds to a transaction in the block
		// For now, we're just testing that the function doesn't crash
		// In a real scenario, you would need the actual Cosmos hash that corresponds to an EVM transaction in the block
		if err != nil {
			// If block doesn't have matching transaction, that's expected
			t.Logf("Expected error when Cosmos hash doesn't match: %v", err)
		} else {
			assert.NotEmpty(t, evmHashFromCosmos)
			assert.True(t, strings.HasPrefix(evmHashFromCosmos, "0x"))
			fmt.Printf("Converted Cosmos hash %s to EVM hash: %s\n", cosmosHashWithPrefix, evmHashFromCosmos)
		}

		// Test with Injscan format (uppercase, no 0x prefix)
		cosmosHashInjscan := "E2B06516699DBCB0C2E1F3BA3E4D2484FAAEC884133FD042FA4D6194D3F2FD06"
		evmHashFromInjscan, err := client.GetEVMHashFromCosmosHash(ctx, cosmosHashInjscan, latestBlock.Uint64())
		if err != nil {
			t.Logf("Expected error when Cosmos hash doesn't match: %v", err)
		} else {
			assert.NotEmpty(t, evmHashFromInjscan)
			assert.True(t, strings.HasPrefix(evmHashFromInjscan, "0x"))
			fmt.Printf("Converted Injscan hash %s to EVM hash: %s\n", cosmosHashInjscan, evmHashFromInjscan)
		}

		// Test that we can use the EVM hash to get the transaction
		evmHashCommon := common.HexToHash(evmHash)
		tx, isPending, err := client.GetTransactionByHash(ctx, evmHashCommon)
		require.NoError(t, err)
		assert.NotNil(t, tx)
		assert.False(t, isPending, "transaction should not be pending")
		fmt.Printf("Successfully retrieved transaction using EVM hash: %s\n", evmHash)
	} else {
		t.Skip("No transactions in latest block")
	}
}

func TestEVMClient_GetKnownContracts(t *testing.T) {
	client, err := NewEVMClient("mainnet")
	require.NoError(t, err)
	defer client.Close()

	contracts := client.GetKnownContracts()
	assert.NotEmpty(t, contracts)

	// Check mainnet contracts
	assert.Contains(t, contracts, "wINJ")
	assert.Contains(t, contracts, "USDT")
	assert.Contains(t, contracts, "USDC")
	assert.Contains(t, contracts, "wETH")
	assert.Contains(t, contracts, "MultiCall")

	fmt.Println("Mainnet contracts:")
	for name, addr := range contracts {
		fmt.Printf("  %s: %s\n", name, addr.Hex())
	}
}

func TestEVMClient_GetGasPrice(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := NewEVMClient("mainnet")
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	gasPrice, err := client.GetGasPrice(ctx)
	require.NoError(t, err)
	assert.NotNil(t, gasPrice)
	assert.Greater(t, gasPrice.Int64(), int64(0))

	fmt.Printf("Current gas price: %s wei\n", gasPrice.String())
}

func TestGetTransactionDetails(t *testing.T) {
	client, err := NewEVMClient("mainnet")
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	txHash := common.HexToHash("0xf0fae8bb6af7071875bc827d4998ff1fa190b20a986619fc38ce0612f57aceac")
	details, err := client.GetTransactionDetails(ctx, txHash)
	require.NoError(t, err)
	assert.NotNil(t, details)

	detailsBytes, err := sonic.Marshal(details)
	require.NoError(t, err)
	fmt.Println(string(detailsBytes))
}
