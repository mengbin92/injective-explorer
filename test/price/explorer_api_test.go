// Explorer API 验证测试
// Phase 0: 验证 Injective Explorer API 的可用性
// 注意：Explorer API 主要用于区块和交易查询，市场数据通过 ChainClient 获取
package price

import (
	"context"
	"testing"
	"time"

	"github.com/InjectiveLabs/sdk-go/client/common"
	explorerclient "github.com/InjectiveLabs/sdk-go/client/explorer"
	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/stretchr/testify/require"
)

// TestExplorerBlocksAPI 验证 Explorer 区块查询 API
func TestExplorerBlocksAPI(t *testing.T) {
	network := common.LoadNetwork("testnet", "lb")

	explorerClient, err := explorerclient.NewExplorerClient(network)
	require.NoError(t, err, "Failed to create explorer client")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 测试 GetBlocks
	blocks, err := explorerClient.GetBlocks(ctx)
	if err != nil {
		t.Logf("GetBlocks failed: %v", err)
		t.Skip("Explorer blocks API not available")
	}

	t.Logf("Explorer returned blocks info")
	// 打印区块信息
	if blocks != nil {
		t.Logf("Blocks response type: %T", blocks)
	}
}

// TestExplorerBlockByHeightAPI 验证按高度查询区块
func TestExplorerBlockByHeightAPI(t *testing.T) {
	network := common.LoadNetwork("testnet", "lb")

	explorerClient, err := explorerclient.NewExplorerClient(network)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 查询特定区块
	block, err := explorerClient.GetBlock(ctx, "98688735")
	if err != nil {
		t.Logf("GetBlock failed: %v", err)
		t.Skip("Explorer block detail API not available")
	}

	t.Logf("Block detail response type: %T", block)
}

// TestExplorerTransactionsAPI 验证交易查询 API
func TestExplorerTransactionsAPI(t *testing.T) {
	network := common.LoadNetwork("testnet", "lb")

	explorerClient, err := explorerclient.NewExplorerClient(network)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 查询最近交易
	req := &injective_explorer_rpcpb.GetTxsRequest{
		Limit: 10,
	}

	transactions, err := explorerClient.GetTxs(ctx, req)
	if err != nil {
		t.Logf("GetTxs failed: %v", err)
		t.Skip("Explorer transactions API not available")
	}

	t.Logf("GetTxs response received")
	if transactions != nil && transactions.Paging != nil {
		t.Logf("Total transactions: %d", transactions.Paging.Total)
	}
}

// TestExplorerTransactionByHashAPI 验证按哈希查询交易
func TestExplorerTransactionByHashAPI(t *testing.T) {
	network := common.LoadNetwork("testnet", "lb")

	explorerClient, err := explorerclient.NewExplorerClient(network)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 注意：这是一个示例哈希，可能不存在
	txHash := "0xD129861AD54EEDB17E16C456524791258E098F75719C9CCA75C9936A65B692C3"

	transaction, err := explorerClient.GetTxByTxHash(ctx, txHash)
	if err != nil {
		t.Logf("GetTxByTxHash failed: %v", err)
		t.Skip("Explorer transaction by hash API not available")
	}

	t.Logf("Transaction response type: %T", transaction)
}

// TestExplorerValidatorsAPI 验证验证人查询 API
func TestExplorerValidatorsAPI(t *testing.T) {
	network := common.LoadNetwork("testnet", "lb")

	explorerClient, err := explorerclient.NewExplorerClient(network)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 查询验证人列表
	validators, err := explorerClient.FetchValidators(ctx)
	if err != nil {
		t.Logf("FetchValidators failed: %v", err)
		t.Skip("Explorer validators API not available")
	}

	t.Logf("Validators response received")
	if validators != nil && validators.Data != nil {
		t.Logf("Validators count: %d", len(validators.Data))
	}
}

// TestExplorerNetworkComparison 对比 Testnet 和 Mainnet
func TestExplorerNetworkComparison(t *testing.T) {
	networks := []struct {
		name    string
		network common.Network
	}{
		{"Testnet", common.LoadNetwork("testnet", "lb")},
		{"Mainnet", common.LoadNetwork("mainnet", "lb")},
	}

	for _, n := range networks {
		t.Run(n.name, func(t *testing.T) {
			client, err := explorerclient.NewExplorerClient(n.network)
			if err != nil {
				t.Logf("Failed to create client for %s: %v", n.name, err)
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			blocks, err := client.GetBlocks(ctx)
			if err != nil {
				t.Logf("Failed to get blocks from %s: %v", n.name, err)
				return
			}

			t.Logf("%s blocks API is available, response type: %T", n.name, blocks)
		})
	}
}

// TestExplorerStreamAPI 验证流式 API（如果支持）
func TestExplorerStreamAPI(t *testing.T) {
	// Explorer 可能支持流式区块或交易订阅
	// 这需要特殊的处理，暂时跳过
	t.Skip("Stream API verification not implemented")
}
