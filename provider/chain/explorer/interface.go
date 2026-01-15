package explorer

import (
	"context"

	"github.com/cosmos/gogoproto/grpc"

	explorerpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
)

type ExplorerClient interface {
	QueryClient() *grpc.ClientConn
	GetAccountTxs(ctx context.Context, req *explorerpb.GetAccountTxsRequest) (*explorerpb.GetAccountTxsResponse, error)
	FetchContractTxs(ctx context.Context, req *explorerpb.GetContractTxsRequest) (*explorerpb.GetContractTxsResponse, error)
	FetchContractTxsV2(ctx context.Context, req *explorerpb.GetContractTxsV2Request) (*explorerpb.GetContractTxsV2Response, error)
	GetBlocks(ctx context.Context) (*explorerpb.GetBlocksResponse, error)
	GetBlock(ctx context.Context, blockHeight string) (*explorerpb.GetBlockResponse, error)
	FetchValidators(ctx context.Context) (*explorerpb.GetValidatorsResponse, error)
	FetchValidator(ctx context.Context, address string) (*explorerpb.GetValidatorResponse, error)
	FetchValidatorUptime(ctx context.Context, address string) (*explorerpb.GetValidatorUptimeResponse, error)
	GetTxs(ctx context.Context, req *explorerpb.GetTxsRequest) (*explorerpb.GetTxsResponse, error)
	GetTxByTxHash(ctx context.Context, hash string) (*explorerpb.GetTxByTxHashResponse, error)
	GetPeggyDeposits(ctx context.Context, req *explorerpb.GetPeggyDepositTxsRequest) (*explorerpb.GetPeggyDepositTxsResponse, error)
	GetPeggyWithdrawals(
		ctx context.Context,
		req *explorerpb.GetPeggyWithdrawalTxsRequest,
	) (*explorerpb.GetPeggyWithdrawalTxsResponse, error)
	GetIBCTransfers(ctx context.Context, req *explorerpb.GetIBCTransferTxsRequest) (*explorerpb.GetIBCTransferTxsResponse, error)
	GetWasmCodes(ctx context.Context, req *explorerpb.GetWasmCodesRequest) (*explorerpb.GetWasmCodesResponse, error)
	GetWasmCodeByID(ctx context.Context, req *explorerpb.GetWasmCodeByIDRequest) (*explorerpb.GetWasmCodeByIDResponse, error)
	GetWasmContracts(ctx context.Context, req *explorerpb.GetWasmContractsRequest) (*explorerpb.GetWasmContractsResponse, error)
	GetWasmContractByAddress(
		ctx context.Context,
		req *explorerpb.GetWasmContractByAddressRequest,
	) (*explorerpb.GetWasmContractByAddressResponse, error)
	GetCW20Balance(ctx context.Context, req *explorerpb.GetCw20BalanceRequest) (*explorerpb.GetCw20BalanceResponse, error)
	FetchRelayers(ctx context.Context, marketIDs []string) (*explorerpb.RelayersResponse, error)
	FetchBankTransfers(ctx context.Context, req *explorerpb.GetBankTransfersRequest) (*explorerpb.GetBankTransfersResponse, error)
	StreamTxs(ctx context.Context) (explorerpb.InjectiveExplorerRPC_StreamTxsClient, error)
	StreamBlocks(ctx context.Context) (explorerpb.InjectiveExplorerRPC_StreamBlocksClient, error)
	Close()
}
