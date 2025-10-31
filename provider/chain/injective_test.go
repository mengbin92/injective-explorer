package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/InjectiveLabs/sdk-go/client"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	"github.com/InjectiveLabs/sdk-go/client/common"
	explorerclient "github.com/InjectiveLabs/sdk-go/client/explorer"
	tmclient "github.com/InjectiveLabs/sdk-go/client/tm"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
)

func TestInjectiveWithSDK(t *testing.T) {
	// 使用SDK内置的网络配置
	network := common.LoadNetwork("testnet", "lb")

	tmClient := tmclient.NewRPCClient(network.TmEndpoint)

	blockHeight, err := tmClient.GetLatestBlockHeight(context.Background())
	if err != nil {
		panic(err)
	}

	tmBlock, err := tmClient.GetBlock(context.Background(), blockHeight)
	if err != nil {
		panic(err)
	}
	str, _ := json.MarshalIndent(tmBlock, "", "\t")
	fmt.Print(string(str))

	explorerClient, err := explorerclient.NewExplorerClient(network)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	res, err := explorerClient.GetBlock(ctx, fmt.Sprintf("%d", blockHeight))
	if err != nil {
		fmt.Println(err)
	}

	explorerClient.GetTxByTxHash(ctx, "0x1234567890abcdef")

	str, _ = json.MarshalIndent(res, "", "\t")
	fmt.Print(string(str))
}

func TestGetAccout(t *testing.T) {
	network := common.LoadNetwork("mainnet", "lb")
	tmClient, _ := rpchttp.New(network.TmEndpoint)

	// 初始化客户端
	clientCtx, err := chainclient.NewClientContext(
		network.ChainId,
		"",
		nil, // 对于只读查询可以传 nil
	)
	if err != nil {
		panic(err)
	}

	clientCtx = clientCtx.WithNodeURI(network.TmEndpoint).WithClient(tmClient)

	chainClient, err := chainclient.NewChainClientV2(
		clientCtx,
		network,
		common.OptionGasPrices(client.DefaultGasPriceWithDenom),
	)
	if err != nil {
		panic(err)
	}

	balances, err := chainClient.GetBankBalances(context.Background(), "inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z")
	if err != nil {
		panic(err)
	}
	fmt.Println(balances)
	injBalance := balances.Balances.AmountOf("inj")
	fmt.Println(injBalance)

	block, err := chainClient.FetchBlockByHeight(context.Background(), 139454029)
	if err != nil {
		panic(err)
	}
	fmt.Println(block.String())

	chainClient.FetchValidatorSetByHeight(context.Background(), 139454029, nil)
}

func TestGetBlocks(t *testing.T) {
	network := common.LoadNetwork("testnet", "lb")

	explorerClient, err := explorerclient.NewExplorerClient(network)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	blocks, err := explorerClient.GetBlocks(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println(blocks.String())
}

func TestGetBlockByHeight(t *testing.T) {
	network := common.LoadNetwork("testnet", "lb")
	explorerClient, err := explorerclient.NewExplorerClient(network)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	blocks, err := explorerClient.GetBlock(ctx,"98688735")
	if err != nil {
		panic(err)
	}
	fmt.Println(blocks.String())
}
