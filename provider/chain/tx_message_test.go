package chain

import (
	"context"
	"fmt"
	"testing"

	"github.com/InjectiveLabs/sdk-go/client/common"
	explorerclient "github.com/InjectiveLabs/sdk-go/client/explorer"
	cosmostxpb "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/stretchr/testify/assert"
)

func TestGetTxMessage(t *testing.T) {
	network := common.LoadNetwork("mainnet", "lb")
	explorerClient, err := explorerclient.NewExplorerClient(network)
	assert.NoError(t, err, "failed to create explorer client")

	tx, err := explorerClient.GetTxByTxHash(context.Background(), "0xb59239ef3c2c0468b95593bfca718a84f95e5d2d9becf73177b1010d315deefa")
	assert.NoError(t, err, "failed to get transaction by hash")
	// fmt.Println(tx.String())

	txMessage := tx.Data.GetMessages()

	fmt.Println(string(txMessage))

	fmt.Println("================================================")

	txBody := &cosmostxpb.TxBody{}

	err = txBody.Unmarshal(txMessage)
	assert.NoError(t, err, "failed to unmarshal tx body")
	fmt.Println(txBody.String())
}
