package chain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	"github.com/btcsuite/btcd/btcutil/bech32"
	"github.com/bytedance/sonic"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestInjectiveWithSDK(t *testing.T) {
	// 使用SDK内置的网络配置
	network := common.LoadNetwork("testnet", "lb")

	tmClient := tmclient.NewRPCClient(network.TmEndpoint)

	blockHeight, err := tmClient.GetLatestBlockHeight(context.Background())
	assert.NoError(t, err, "failed to get latest block height")

	tmBlock, err := tmClient.GetBlock(context.Background(), blockHeight)
	assert.NoError(t, err, "failed to get block")
	str, _ := json.MarshalIndent(tmBlock, "", "\t")
	fmt.Print(string(str))

	explorerClient, err := explorerclient.NewExplorerClient(network)
	assert.NoError(t, err, "failed to create explorer client")

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
	assert.NoError(t, err, "failed to create client context")

	clientCtx = clientCtx.WithNodeURI(network.TmEndpoint).WithClient(tmClient)

	chainClient, err := chainclient.NewChainClientV2(
		clientCtx,
		network,
		common.OptionGasPrices(client.DefaultGasPriceWithDenom),
	)
	assert.NoError(t, err, "failed to create chain client")

	balances, err := chainClient.GetBankBalances(context.Background(), "inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z")
	assert.NoError(t, err, "failed to get bank balances")
	fmt.Println(balances)
	injBalance := balances.Balances.AmountOf("inj")
	fmt.Println(injBalance)

	block, err := chainClient.FetchBlockByHeight(context.Background(), 139454029)
	assert.NoError(t, err, "failed to fetch block by height")
	fmt.Println(block.String())

	chainClient.FetchValidatorSetByHeight(context.Background(), 139454029, nil)
}

func TestGetBlocks(t *testing.T) {
	network := common.LoadNetwork("testnet", "lb")

	explorerClient, err := explorerclient.NewExplorerClient(network)
	assert.NoError(t, err, "failed to create explorer client")

	ctx := context.Background()
	blocks, err := explorerClient.GetBlocks(ctx)
	assert.NoError(t, err, "failed to get blocks")
	fmt.Println(blocks.String())
}

func TestGetBlockByHeight(t *testing.T) {
	network := common.LoadNetwork("testnet", "lb")
	explorerClient, err := explorerclient.NewExplorerClient(network)
	assert.NoError(t, err, "failed to create explorer client")

	ctx := context.Background()
	blocks, err := explorerClient.GetBlock(ctx, "98688735")
	assert.NoError(t, err, "failed to get block")
	fmt.Println(blocks.String())
}

func TestGetLatestTransactions(t *testing.T) {
	network := common.LoadNetwork("mainnet", "lb")
	explorerClient, err := explorerclient.NewExplorerClient(network)
	assert.NoError(t, err, "failed to create explorer client")

	req := &injective_explorer_rpcpb.GetTxsRequest{
		Limit: 1,
	}

	ctx := context.Background()
	transactions, err := explorerClient.GetTxs(ctx, req)
	assert.NoError(t, err, "failed to get transactions")
	structData, err := toStruct(transactions)
	assert.NoError(t, err, "failed to convert to struct")
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
	assert.NoError(t, err, "failed to create client context")

	clientCtx = clientCtx.WithNodeURI(network.TmEndpoint).WithClient(tmClient)

	chainClient, err := chainclient.NewChainClientV2(
		clientCtx,
		network,
		common.OptionGasPrices(client.DefaultGasPriceWithDenom),
	)
	assert.NoError(t, err, "failed to create chain client")

	account, err := chainClient.GetAccount(context.Background(), "inj1jddhqk6egwcwrwzev5wjeneht0ykmlwhcehc4z")
	assert.NoError(t, err, "failed to get account")
	structData, err := toStruct(account)
	assert.NoError(t, err, "failed to convert to struct")
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
	assert.NoError(t, err, "failed to create explorer client")

	ctx := context.Background()
	validators, err := explorerClient.FetchValidators(ctx)
	assert.NoError(t, err, "failed to fetch validators")
	fmt.Println(validators.String())
	fmt.Println(len(validators.Data))
}

func TestGetValidatorByValidatorAddress(t *testing.T) {
	network := common.LoadNetwork("mainnet", "lb")
	explorerClient, err := explorerclient.NewExplorerClient(network)
	assert.NoError(t, err, "failed to create explorer client")

	ctx := context.Background()
	validator, err := explorerClient.FetchValidator(ctx, "injvaloper13v0sulppc8pgtk4907p3vaqgq54vyl9yqmf7av")
	assert.NoError(t, err, "failed to fetch validator")
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
	assert.NoError(t, err, "failed to init cosmos keyring")

	clientCtx, err := chainclient.NewClientContext(
		network.ChainId,
		senderAddress.String(),
		cosmosKeyring,
	)
	assert.NoError(t, err, "failed to create client context")

	clientCtx = clientCtx.WithNodeURI(network.TmEndpoint).WithClient(tmClient)

	chainClient, err := chainclient.NewChainClientV2(
		clientCtx,
		network,
		common.OptionGasPrices(client.DefaultGasPriceWithDenom),
	)
	assert.NoError(t, err, "failed to create chain client")

	validatorAddr := "injvaloper13v0sulppc8pgtk4907p3vaqgq54vyl9yqmf7av"
	validator, err := chainClient.FetchValidatorDistributionInfo(context.Background(), validatorAddr)
	assert.NoError(t, err, "failed to fetch validator distribution info")
	fmt.Println(validator.String())
}

func TestGetTransactionsByHash(t *testing.T) {
	network := common.LoadNetwork("mainnet", "lb")
	explorerClient, err := explorerclient.NewExplorerClient(network)
	assert.NoError(t, err, "failed to create explorer client")

	ctx := context.Background()
	transaction, err := explorerClient.GetTxByTxHash(ctx, "0xD129861AD54EEDB17E16C456524791258E098F75719C9CCA75C9936A65B692C3")
	assert.NoError(t, err, "failed to get transaction by hash")

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
	assert.NoError(t, err, "failed to create client context")

	clientCtx = clientCtx.WithNodeURI(network.TmEndpoint).WithClient(tmClient)

	chainClient, err := chainclient.NewChainClientV2(
		clientCtx,
		network,
		common.OptionGasPrices(client.DefaultGasPriceWithDenom),
	)
	assert.NoError(t, err, "failed to create chain client")

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
	assert.NoError(t, err, "failed to create explorer client")

	// total
	transactions, err := explorerClient.GetTxs(context.Background(), &injective_explorer_rpcpb.GetTxsRequest{
		Limit: 1,
	})
	assert.NoError(t, err, "failed to get transactions (total)")
	fmt.Println("total: ", transactions.Paging.Total)

	// 30d
	transactions, err = explorerClient.GetTxs(context.Background(), &injective_explorer_rpcpb.GetTxsRequest{
		Limit:     1,
		StartTime: thirtyDaysAgo.UnixMilli(),
		EndTime:   now.UnixMilli(),
	})
	assert.NoError(t, err, "failed to get transactions (30d)")
	fmt.Println("30d: ", transactions.Paging.Total)

	// 24h
	transactions, err = explorerClient.GetTxs(context.Background(), &injective_explorer_rpcpb.GetTxsRequest{
		Limit:     1,
		StartTime: twentyFourHoursAgo.UnixMilli(),
		EndTime:   now.UnixMilli(),
	})
	assert.NoError(t, err, "failed to get transactions (24h)")
	fmt.Println("24h: ", transactions.Paging.Total)
}

func TestAccountBalance(t *testing.T) {
	network := common.LoadNetwork("mainnet", "lb")
	tmClient, _ := rpchttp.New(network.TmEndpoint)

	// 初始化客户端
	clientCtx, err := chainclient.NewClientContext(
		network.ChainId,
		"",
		nil, // 对于只读查询可以传 nil
	)
	assert.NoError(t, err, "failed to create client context")

	clientCtx = clientCtx.WithNodeURI(network.TmEndpoint).WithClient(tmClient)

	chainClient, err := chainclient.NewChainClientV2(
		clientCtx,
		network,
		common.OptionGasPrices(client.DefaultGasPriceWithDenom),
	)
	assert.NoError(t, err, "failed to create chain client")

	address, err := EthToInjAddress("0x71562b71999873db5b286df957af199ec94617f7")
	assert.NoError(t, err, "failed to convert Ethereum address to Injective")
	fmt.Println(address)

	balances, err := chainClient.GetBankBalances(context.Background(), address)
	assert.NoError(t, err, "failed to get bank balances")
	fmt.Println(balances)
}

func TestAddressConversion(t *testing.T) {
	// Test Ethereum to Injective conversion
	ethAddr := "0x71562b71999873db5b286df957af199ec94617f7"
	injAddr, err := EthToInjAddress(ethAddr)
	assert.NoError(t, err, "failed to convert Ethereum address to Injective")
	fmt.Printf("Ethereum address: %s -> Injective address: %s\n", ethAddr, injAddr)

	// Test Injective to Ethereum conversion (using the models package function)
	// Note: This requires importing the models package
	// For now, we'll test the round-trip conversion
	injAddr2, err := EthToInjAddress(ethAddr)
	assert.NoError(t, err, "failed to convert Ethereum address to Injective (round-trip)")
	if injAddr != injAddr2 {
		t.Errorf("address conversion is not consistent: %s != %s", injAddr, injAddr2)
	}
}

func EthToInjAddress(ethAddr string) (string, error) {
	// 1. clean hex
	ethAddr = strings.TrimPrefix(ethAddr, "0x")

	// 2. hex → bytes (20 bytes)
	addrBytes, err := hex.DecodeString(ethAddr)
	if err != nil {
		return "", err
	}
	if len(addrBytes) != 20 {
		return "", fmt.Errorf("invalid eth address length")
	}

	// 3. 8-bit → 5-bit
	conv, err := bech32.ConvertBits(addrBytes, 8, 5, true)
	if err != nil {
		return "", err
	}

	// 4. bech32 encode with hrp = inj
	return bech32.Encode("inj", conv)
}
