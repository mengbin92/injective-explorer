package explorer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/InjectiveLabs/sdk-go/client"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	"github.com/InjectiveLabs/sdk-go/client/common"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
)

func createChainClient() (chainclient.ChainClientV2, error) {
	network := common.LoadNetwork("mainnet", "lb")

	tmClient, err := rpchttp.New(network.TmEndpoint)
	if err != nil {
		return nil, err
	}

	clientCtx, err := chainclient.NewClientContext(
		network.ChainId,
		"",  // Sender address can be empty (query only)
		nil, // Keyring can be nil (query only)
	)
	if err != nil {
		return nil, err
	}

	// Configure node URI and RPC client
	clientCtx = clientCtx.WithNodeURI(network.TmEndpoint).WithClient(tmClient)

	chainClient, err := chainclient.NewChainClientV2(
		clientCtx,
		network,
		common.OptionGasPrices(client.DefaultGasPriceWithDenom),
	)
	if err != nil {
		return nil, err
	}

	return chainClient, nil
}

func TestTmWebsocket(t *testing.T) {
	network := common.LoadNetwork("mainnet", "lb")
	tmClient, err := rpchttp.New(network.TmEndpoint)
	if err != nil {
		t.Fatalf("Failed to create TM client: %v", err)
	}

	err = tmClient.Start()
	if err != nil {
		t.Fatalf("Failed to start TM client: %v", err)
	}

	defer tmClient.Stop()

	ctx := context.Background()

	sub, err := tmClient.Subscribe(ctx, "injective-block-listener", "tm.event='NewBlockHeader'")
	if err != nil {
		t.Fatalf("Failed to subscribe to TM client: %v", err)
	}

	for {
		select {
		case msg := <-sub:
			ev, ok := msg.Data.(tmtypes.EventDataNewBlockHeader)
			if !ok {
				continue
			}

			h := ev.Header

			fmt.Printf(
				"height=%d hash=%X time=%s proposer=%s\n",
				h.Height,
				h.Hash(),
				h.Time.Format(time.RFC3339),
				h.ProposerAddress.String(), // injvalcons
			)

		case <-ctx.Done():
			return
		}
	}
}

func TestGetValidators(t *testing.T) {
	chainClient, err := createChainClient()
	if err != nil {
		t.Fatalf("Failed to create chain client: %v", err)
	}
	stakingClient := stakingtypes.NewQueryClient(chainClient.QueryClient())
	validatorsResp, err := stakingClient.Validators(context.Background(), &stakingtypes.QueryValidatorsRequest{
		Status: stakingtypes.BondStatusBonded, // Query bonded validators
		Pagination: &query.PageRequest{
			Limit: 1000, // Query up to 1000 validators
		},
	})
	if err != nil {
		t.Fatalf("Failed to query validators: %v", err)
	}
	for _, validator := range validatorsResp.Validators {
		consensusPubKey, err := parseConsensusPubKey(validator.ConsensusPubkey)
		if err != nil {
			t.Fatalf("Failed to parse consensus pubkey: %v", err)
		}
		fmt.Println(consensusPubKey.Address().String())
	}

}

func parseConsensusPubKey(any *codectypes.Any) (cryptotypes.PubKey, error) {
	if any == nil {
		return nil, fmt.Errorf("consensus pubkey is nil")
	}

	// Create ProtoCodec with the InterfaceRegistry
	cdc := codec.NewProtoCodec(makeInterfaceRegistry())

	// Unpack the Any type to PubKey interface
	var pk cryptotypes.PubKey
	err := cdc.UnpackAny(any, &pk)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack consensus pubkey: %w", err)
	}

	return pk, nil
}

func makeInterfaceRegistry() codectypes.InterfaceRegistry {
	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	return registry
}

func TestGetTx(t *testing.T) {
	chainClient, err := createChainClient()
	if err != nil {
		t.Fatalf("Failed to create chain client: %v", err)
	}
	txResp, err := chainClient.GetTx(context.Background(), "3dd98009338cefac0d13daeb41ce3e9d537054f2b1b3beefd1633ace9d23c83b")
	if err != nil {
		t.Fatalf("Failed to get tx: %v", err)
	}
	fmt.Println(txResp)
}