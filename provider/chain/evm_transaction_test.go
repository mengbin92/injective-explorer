package chain

import (
	"context"
	"fmt"
	"testing"
	"time"

	evmtypes "github.com/InjectiveLabs/sdk-go/chain/evm/types"
	"github.com/InjectiveLabs/sdk-go/client"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	"github.com/InjectiveLabs/sdk-go/client/common"
	"github.com/bytedance/sonic"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
)

func TestGetEVMTransactionByCosmosHash(t *testing.T) {

	hash := "00F39E48ACBF393D70CC0C867BF0C68249C893877AC318A99D53A5B91C8FDD16"

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

	resp, err := chainClient.GetTx(context.Background(), hash)
	assert.NoError(t, err, "failed to get tx")

	var evmHash string
	for i, event := range resp.TxResponse.Events {
		fmt.Printf("Event %d: %s\n", i, event.Type)
		for _, attribute := range event.Attributes {
			fmt.Printf("\t%s: %s\n", attribute.Key, attribute.Value)
			if attribute.Key == evmtypes.AttributeKeyEthereumTxHash {
				evmHash = attribute.Value
			}
		}
		fmt.Println()
	}

	// Check if EVM hash was found
	assert.NotEmpty(t, evmHash, "EVM transaction hash not found in events")

	client, err := NewEVMClient("mainnet")
	assert.NoError(t, err)
	defer client.Close()

	// Create context with timeout to avoid context canceled errors
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	details, err := client.GetTransactionDetails(ctx, ethcommon.HexToHash(evmHash))
	assert.NoError(t, err, "failed to get transaction details by hash: %s", evmHash)
	assert.NotNil(t, details, "transaction details should not be nil")

	detailsBytes, err := sonic.Marshal(details)
	assert.NoError(t, err)
	fmt.Println(string(detailsBytes))
}
