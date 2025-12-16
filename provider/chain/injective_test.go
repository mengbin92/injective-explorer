package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	pb "injective-explorer/api/explorer/v1"

	"cosmossdk.io/errors"
	"github.com/InjectiveLabs/sdk-go/client"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	"github.com/InjectiveLabs/sdk-go/client/common"
	explorerclient "github.com/InjectiveLabs/sdk-go/client/explorer"
	tmclient "github.com/InjectiveLabs/sdk-go/client/tm"
	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/bytedance/sonic"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
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

func TestGetAccountBalances(t *testing.T) {
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
	blocks, err := explorerClient.GetBlock(ctx, "98688735")
	if err != nil {
		panic(err)
	}
	fmt.Println(blocks.String())
}

func TestGetLatestTransactions(t *testing.T) {
	network := common.LoadNetwork("mainnet", "lb")
	explorerClient, err := explorerclient.NewExplorerClient(network)
	if err != nil {
		panic(err)
	}

	req := &injective_explorer_rpcpb.GetTxsRequest{
		Limit: 1,
	}

	ctx := context.Background()
	transactions, err := explorerClient.GetTxs(ctx, req)
	if err != nil {
		panic(err)
	}
	structData, err := toStruct(transactions)
	if err != nil {
		panic(err)
	}
	reply := &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "success",
		Data:    structData,
	}

	str, _ := json.Marshal(reply)
	fmt.Println(string(str))
}

func TestGetAccountInfo(t *testing.T) {
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

	account, err := chainClient.GetAccount(context.Background(), "inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z")
	if err != nil {
		panic(err)
	}
	structData, err := toStruct(account)
	if err != nil {
		panic(err)
	}
	reply := &pb.Reply{
		Code:    int32(http.StatusOK),
		Message: "success",
		Data:    structData,
	}

	str, _ := json.Marshal(reply)
	fmt.Println(string(str))
}

func TestGetValidators(t *testing.T) {
	network := common.LoadNetwork("mainnet", "lb")
	explorerClient, err := explorerclient.NewExplorerClient(network)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	validators, err := explorerClient.FetchValidators(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println(validators.String())
	fmt.Println(len(validators.Data))
}

func TestGetValidatorByValidatorAddress(t *testing.T) {
	network := common.LoadNetwork("mainnet", "lb")
	explorerClient, err := explorerclient.NewExplorerClient(network)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	validator, err := explorerClient.FetchValidator(ctx, "injvaloper13v0sulppc8pgtk4907p3vaqgq54vyl9yqmf7av")
	if err != nil {
		panic(err)
	}
	fmt.Println(validator.String())
}

func TestChainClientFetchValidator(t *testing.T) {
	network := common.LoadNetwork("mainnet", "lb")
	tmClient, _ := rpchttp.New(network.TmEndpoint)

	senderAddress, cosmosKeyring, err := chainclient.InitCosmosKeyring(
		"",
		"injectived",
		"file",
		"inj-user",
		"12345678",
		"5d386fbdbf11f1141010f81a46b40f94887367562bd33b452bbaa6ce1cd1381e",
		false,
	)

	if err != nil {
		panic(err)
	}

	clientCtx, err := chainclient.NewClientContext(
		network.ChainId,
		senderAddress.String(),
		cosmosKeyring,
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

	validatorAddr := "injvaloper13v0sulppc8pgtk4907p3vaqgq54vyl9yqmf7av"
	validator, err := chainClient.FetchValidatorDistributionInfo(context.Background(), validatorAddr)
	if err != nil {
		panic(err)
	}
	fmt.Println(validator.String())
}

func TestGetTransactionsByHash(t *testing.T) {
	network := common.LoadNetwork("mainnet", "lb")
	explorerClient, err := explorerclient.NewExplorerClient(network)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	transaction, err := explorerClient.GetTxByTxHash(ctx, "0xD129861AD54EEDB17E16C456524791258E098F75719C9CCA75C9936A65B692C3")
	if err != nil {
		panic(err)
	}

	fmt.Println(transaction.Data.GetClaimIds())
}

func toStruct(v interface{}) (*structpb.Struct, error) {
	// Marshal to JSON first
	jsonBytes, err := sonic.Marshal(v)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal to JSON")
	}

	// Unmarshal into structpb.Struct
	var structData structpb.Struct
	if err := protojson.Unmarshal(jsonBytes, &structData); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal JSON to struct")
	}

	return &structData, nil
}

func TestGetGasPrice(t *testing.T) {
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

	gasPrice := chainClient.CurrentChainGasPrice(context.Background())
	fmt.Println(gasPrice)
}

func TestGetTxsStats(t *testing.T) {
	// Prepare time boundaries for queries
	now := time.Now()
	thirtyDaysAgo := now.AddDate(0, 0, -30)
	twentyFourHoursAgo := now.Add(-24 * time.Hour)

	network := common.LoadNetwork("mainnet", "lb")
	explorerClient, err := explorerclient.NewExplorerClient(network)
	if err != nil {
		panic(err)
	}

	// total
	transactions, err := explorerClient.GetTxs(context.Background(), &injective_explorer_rpcpb.GetTxsRequest{
		Limit: 1,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("total: ", transactions.Paging.Total)

	// 30d
	transactions, err = explorerClient.GetTxs(context.Background(), &injective_explorer_rpcpb.GetTxsRequest{
		Limit:     1,
		StartTime: thirtyDaysAgo.UnixMilli(),
		EndTime:   now.UnixMilli(),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("30d: ", transactions.Paging.Total)

	// 24h
	transactions, err = explorerClient.GetTxs(context.Background(), &injective_explorer_rpcpb.GetTxsRequest{
		Limit:     1,
		StartTime: twentyFourHoursAgo.UnixMilli(),
		EndTime:   now.UnixMilli(),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("24h: ", transactions.Paging.Total)
}
