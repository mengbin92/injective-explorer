package explorer

import (
	"context"

	explorerpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
)

// ExplorerClient provides streaming access to blockchain data.
// This is a simplified interface that only includes the actually implemented method.
type ExplorerClient interface {
	// StreamBlocks establishes a WebSocket connection to stream new block events.
	// Returns a StreamBlocksClient that receives block height updates.
	StreamBlocks(ctx context.Context) (explorerpb.InjectiveExplorerRPC_StreamBlocksClient, error)

	// Close closes the client connection and releases resources.
	Close()
}
