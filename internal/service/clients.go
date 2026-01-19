// Package service provides shared client initialization utilities.
package service

import (
	"sync"

	"injective-explorer/internal/global"

	"github.com/InjectiveLabs/sdk-go/client"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	"github.com/InjectiveLabs/sdk-go/client/common"
	tmclient "github.com/InjectiveLabs/sdk-go/client/tm"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/pkg/errors"
)

var (
	// Shared client instances (initialized once, reused across services)
	sharedChainClient chainclient.ChainClientV2
	sharedTmClient    tmclient.TendermintClient

	// Mutexes for thread-safe initialization
	sharedExplorerClientOnce sync.Once
	sharedChainClientOnce    sync.Once
	sharedTmClientOnce       sync.Once

	// Errors from client initialization
	sharedExplorerClientErr error
	sharedChainClientErr    error
)

// GetSharedChainClient returns a shared chain client instance.
// This client is initialized once and reused across all services.
func GetSharedChainClient() (chainclient.ChainClientV2, error) {
	sharedChainClientOnce.Do(func() {
		client, err := buildChainClientV2Query()
		if err != nil {
			sharedChainClientErr = errors.Wrap(err, "build chain client error")
			return
		}
		sharedChainClient = client
	})
	if sharedChainClientErr != nil {
		return nil, sharedChainClientErr
	}
	return sharedChainClient, nil
}

// GetSharedTmClient returns a shared Tendermint client instance.
// This client is initialized once and reused across all services.
func GetSharedTmClient() tmclient.TendermintClient {
	sharedTmClientOnce.Do(func() {
		sharedTmClient = tmclient.NewRPCClient(global.ChainConfig.TmEndpoint)
	})
	return sharedTmClient
}

// buildChainClientV2Query creates a new ChainClientV2 for query operations.
// This client does not require a keyring as it is only used for queries, not transactions.
//
// Returns:
//   - chainclient.ChainClientV2: A configured chain client
//   - error: Error if client creation fails
func buildChainClientV2Query() (chainclient.ChainClientV2, error) {
	// Create Tendermint RPC client
	tmClient, err := rpchttp.New(global.ChainConfig.TmEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "create tm client error")
	}

	// Create client context (no keyring needed for queries)
	// Note: This client cannot sign transactions, only perform queries
	clientCtx, err := chainclient.NewClientContext(
		global.ChainConfig.ChainId,
		"",  // Sender address can be empty (query only)
		nil, // Keyring can be nil (query only)
	)
	if err != nil {
		return nil, errors.Wrap(err, "create client context error")
	}

	// Configure node URI and RPC client
	clientCtx = clientCtx.WithNodeURI(global.ChainConfig.TmEndpoint).WithClient(tmClient)

	// Create ChainClientV2
	chainClient, err := chainclient.NewChainClientV2(
		clientCtx,
		global.ChainConfig,
		common.OptionGasPrices(client.DefaultGasPriceWithDenom),
	)
	if err != nil {
		return nil, errors.Wrap(err, "create chain client error")
	}

	return chainClient, nil
}
