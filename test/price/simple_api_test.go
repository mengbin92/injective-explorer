// 简单 API 验证测试 - 用于确认 SDK 返回的结构
package price

import (
	"context"
	"fmt"
	"testing"
	"time"

	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	"github.com/InjectiveLabs/sdk-go/client/common"
	"github.com/bytedance/sonic"
	"github.com/stretchr/testify/require"
)

// TestSimpleSpotMarkets 简单的现货市场查询测试
func TestSimpleSpotMarkets(t *testing.T) {
	network := common.LoadNetwork("testnet", "lb")

	clientCtx, err := chainclient.NewClientContext(network.ChainId, "", nil)
	require.NoError(t, err)

	chainClient, err := chainclient.NewChainClientV2(clientCtx, network)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 获取现货市场列表
	resp, err := chainClient.FetchChainSpotMarkets(ctx, "", nil)
	if err != nil {
		t.Fatalf("FetchChainSpotMarkets failed: %v", err)
	}

	// 打印响应的 JSON 结构
	jsonBytes, err := sonic.MarshalIndent(resp, "", "  ")
	if err != nil {
		t.Logf("Failed to marshal response: %v", err)
	} else {
		// 只打印前 2000 个字符
		str := string(jsonBytes)
		if len(str) > 2000 {
			str = str[:2000] + "..."
		}
		t.Logf("Response structure:\n%s", str)
	}

	t.Logf("Total markets: %d", len(resp.Markets))

	// 打印第一个市场的类型信息
	if len(resp.Markets) > 0 {
		first := resp.Markets[0]
		t.Logf("First market type: %T", first)
		// 打印所有可访问的字段
		fmt.Printf("Market fields: %+v\n", first)
	}
}

// TestSimpleMidPrice 简单的中间价查询测试
func TestSimpleMidPrice(t *testing.T) {
	network := common.LoadNetwork("testnet", "lb")

	clientCtx, err := chainclient.NewClientContext(network.ChainId, "", nil)
	require.NoError(t, err)

	chainClient, err := chainclient.NewChainClientV2(clientCtx, network)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 先获取市场列表
	marketsResp, err := chainClient.FetchChainSpotMarkets(ctx, "", nil)
	if err != nil {
		t.Skip("Cannot get markets list")
	}

	if len(marketsResp.Markets) == 0 {
		t.Skip("No markets available")
	}

	marketID := marketsResp.Markets[0].MarketId
	t.Logf("Testing with market ID: %s", marketID)

	// 获取中间价
	midPriceResp, err := chainClient.FetchSpotMidPriceAndTOB(ctx, marketID)
	if err != nil {
		t.Logf("FetchSpotMidPriceAndTOB failed: %v", err)
		t.Skip("Mid price API not available")
	}

	// 打印响应
	jsonBytes, _ := sonic.MarshalIndent(midPriceResp, "", "  ")
	t.Logf("Mid price response:\n%s", string(jsonBytes))
}

// TestSimpleDerivativeMarkets 简单的永续合约市场测试
func TestSimpleDerivativeMarkets(t *testing.T) {
	network := common.LoadNetwork("testnet", "lb")

	clientCtx, err := chainclient.NewClientContext(network.ChainId, "", nil)
	require.NoError(t, err)

	chainClient, err := chainclient.NewChainClientV2(clientCtx, network)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 获取永续合约市场
	resp, err := chainClient.FetchChainDerivativeMarkets(ctx, "", nil, true)
	if err != nil {
		t.Logf("FetchChainDerivativeMarkets failed: %v", err)
		t.Skip("Derivative markets API not available")
	}

	t.Logf("Derivative markets count: %d", len(resp.Markets))

	// 打印第一个市场的结构
	if len(resp.Markets) > 0 {
		first := resp.Markets[0]
		t.Logf("First derivative market type: %T", first)
		jsonBytes, _ := sonic.MarshalIndent(first, "", "  ")
		str := string(jsonBytes)
		if len(str) > 2000 {
			str = str[:2000] + "..."
		}
		t.Logf("First market:\n%s", str)
	}
}
