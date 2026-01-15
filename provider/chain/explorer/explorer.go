package explorer

import (
	"context"
	"io"
	"sync"

	"cosmossdk.io/errors"
	"github.com/InjectiveLabs/sdk-go/client/common"
	explorerpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	log "github.com/InjectiveLabs/suplog"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	tmtypes "github.com/cometbft/cometbft/types"
	cosmogogrpc "github.com/cosmos/gogoproto/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func NewExplorerClient(network common.Network, options ...common.ClientOption) (ExplorerClient, error) {
	// process options
	opts := common.DefaultClientOptions()
	if network.ChainTLSCert != nil {
		options = append(options, common.OptionTLSCert(network.ExchangeTLSCert))
	}
	for _, opt := range options {
		if err := opt(opts); err != nil {
			err = errors.Wrap(err, "error in client option")
			return nil, err
		}
	}

	// create grpc client
	var conn *grpc.ClientConn
	var err error
	if opts.TLSCert != nil {
		conn, err = grpc.NewClient(
			network.ExplorerGrpcEndpoint,
			grpc.WithTransportCredentials(opts.TLSCert),
			grpc.WithContextDialer(common.DialerFunc),
		)
	} else {
		conn, err = grpc.NewClient(
			network.ExplorerGrpcEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithContextDialer(common.DialerFunc),
		)
	}
	if err != nil {
		err := errors.Wrapf(err, "failed to connect to the gRPC: %s", network.ExplorerGrpcEndpoint)
		return nil, err
	}

	// build client
	cc := &explorerClient{
		opts:    opts,
		network: network,
		conn:    conn,

		explorerClient: explorerpb.NewInjectiveExplorerRPCClient(conn),
		logger: log.WithFields(log.Fields{
			"module": "sdk-go",
			"svc":    "exchangeClient",
		}),
	}

	return cc, nil
}

type explorerClient struct {
	opts    *common.ClientOptions
	network common.Network
	conn    *grpc.ClientConn
	logger  log.Logger

	explorerClient explorerpb.InjectiveExplorerRPCClient
}

func (c *explorerClient) Close() {
	// TODO: implement cleanup logic (close connection, etc.)
}

func (c *explorerClient) QueryClient() *cosmogogrpc.ClientConn {
	// TODO: implement
	return nil
}

func (c *explorerClient) GetAccountTxs(ctx context.Context, req *explorerpb.GetAccountTxsRequest) (*explorerpb.GetAccountTxsResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) FetchContractTxs(ctx context.Context, req *explorerpb.GetContractTxsRequest) (*explorerpb.GetContractTxsResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) FetchContractTxsV2(ctx context.Context, req *explorerpb.GetContractTxsV2Request) (*explorerpb.GetContractTxsV2Response, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) GetBlocks(ctx context.Context) (*explorerpb.GetBlocksResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) GetBlock(ctx context.Context, blockHeight string) (*explorerpb.GetBlockResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) FetchValidators(ctx context.Context) (*explorerpb.GetValidatorsResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) FetchValidator(ctx context.Context, address string) (*explorerpb.GetValidatorResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) FetchValidatorUptime(ctx context.Context, address string) (*explorerpb.GetValidatorUptimeResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) GetTxs(ctx context.Context, req *explorerpb.GetTxsRequest) (*explorerpb.GetTxsResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) GetTxByTxHash(ctx context.Context, hash string) (*explorerpb.GetTxByTxHashResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) GetPeggyDeposits(ctx context.Context, req *explorerpb.GetPeggyDepositTxsRequest) (*explorerpb.GetPeggyDepositTxsResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) GetPeggyWithdrawals(ctx context.Context, req *explorerpb.GetPeggyWithdrawalTxsRequest) (*explorerpb.GetPeggyWithdrawalTxsResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) GetIBCTransfers(ctx context.Context, req *explorerpb.GetIBCTransferTxsRequest) (*explorerpb.GetIBCTransferTxsResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) GetWasmCodes(ctx context.Context, req *explorerpb.GetWasmCodesRequest) (*explorerpb.GetWasmCodesResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) GetWasmCodeByID(ctx context.Context, req *explorerpb.GetWasmCodeByIDRequest) (*explorerpb.GetWasmCodeByIDResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) GetWasmContracts(ctx context.Context, req *explorerpb.GetWasmContractsRequest) (*explorerpb.GetWasmContractsResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) GetWasmContractByAddress(ctx context.Context, req *explorerpb.GetWasmContractByAddressRequest) (*explorerpb.GetWasmContractByAddressResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) GetCW20Balance(ctx context.Context, req *explorerpb.GetCw20BalanceRequest) (*explorerpb.GetCw20BalanceResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) FetchRelayers(ctx context.Context, marketIDs []string) (*explorerpb.RelayersResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) FetchBankTransfers(ctx context.Context, req *explorerpb.GetBankTransfersRequest) (*explorerpb.GetBankTransfersResponse, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) StreamTxs(ctx context.Context) (explorerpb.InjectiveExplorerRPC_StreamTxsClient, error) {
	// TODO: implement
	return nil, nil
}

func (c *explorerClient) StreamBlocks(ctx context.Context) (explorerpb.InjectiveExplorerRPC_StreamBlocksClient, error) {
	// Create Tendermint RPC HTTP client with extended timeout for slow endpoints
	tmClient, err := rpchttp.NewWithTimeout(c.network.TmEndpoint, 30)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create Tendermint client: %s", c.network.TmEndpoint)
	}

	// Start the client
	if err := tmClient.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start Tendermint client")
	}

	// Subscribe to NewBlockHeader events
	sub, err := tmClient.Subscribe(ctx, "injective-block-listener", "tm.event='NewBlockHeader'")
	if err != nil {
		tmClient.Stop()
		return nil, errors.Wrap(err, "failed to subscribe to NewBlockHeader events")
	}

	// Create and return the adapter
	adapter := &tmStreamBlocksAdapter{
		ctx:      ctx,
		tmClient: tmClient,
		sub:      sub,
		logger:   c.logger,
	}

	return adapter, nil
}

// tmStreamBlocksAdapter adapts Tendermint WebSocket subscription to gRPC StreamBlocksClient interface
type tmStreamBlocksAdapter struct {
	ctx      context.Context
	tmClient *rpchttp.HTTP
	sub      <-chan coretypes.ResultEvent
	logger   log.Logger
	mu       sync.Mutex
	closed   bool
}

// Recv receives the next block from the stream
func (a *tmStreamBlocksAdapter) Recv() (*explorerpb.StreamBlocksResponse, error) {
	for {
		a.mu.Lock()
		if a.closed {
			a.mu.Unlock()
			return nil, io.EOF
		}
		a.mu.Unlock()

		select {
		case <-a.ctx.Done():
			a.mu.Lock()
			if !a.closed {
				a.closed = true
				a.tmClient.Stop()
			}
			a.mu.Unlock()
			return nil, a.ctx.Err()
		case msg, ok := <-a.sub:
			if !ok {
				a.mu.Lock()
				if !a.closed {
					a.closed = true
					a.tmClient.Stop()
				}
				a.mu.Unlock()
				return nil, io.EOF
			}

			// Type assert to EventDataNewBlockHeader
			ev, ok := msg.Data.(tmtypes.EventDataNewBlockHeader)
			if !ok {
				// Skip non-block events and continue to next message
				continue
			}

			// Convert EventDataNewBlockHeader to StreamBlocksResponse
			h := ev.Header
			return &explorerpb.StreamBlocksResponse{
				Height: uint64(h.Height),
			}, nil
		}
	}
}

// Header returns the header metadata
func (a *tmStreamBlocksAdapter) Header() (metadata.MD, error) {
	return nil, nil
}

// Trailer returns the trailer metadata
func (a *tmStreamBlocksAdapter) Trailer() metadata.MD {
	return nil
}

// CloseSend closes the send direction of the stream
func (a *tmStreamBlocksAdapter) CloseSend() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.closed {
		a.closed = true
		a.tmClient.Stop()
	}
	return nil
}

// Context returns the context for this stream
func (a *tmStreamBlocksAdapter) Context() context.Context {
	return a.ctx
}

// SendMsg sends a message
func (a *tmStreamBlocksAdapter) SendMsg(m interface{}) error {
	return nil
}

// RecvMsg receives a message
func (a *tmStreamBlocksAdapter) RecvMsg(m interface{}) error {
	return nil
}
