package chain

import (
	"context"
	"fmt"
	"testing"

	"github.com/InjectiveLabs/sdk-go/client/common"
	explorerclient "github.com/InjectiveLabs/sdk-go/client/explorer"
	cosmostxpb "github.com/cosmos/cosmos-sdk/types/tx"
)

func TestGetTxMessage(t *testing.T) {
	network := common.LoadNetwork("mainnet", "lb")
	explorerClient, err := explorerclient.NewExplorerClient(network)
	if err != nil {
		panic(err)
	}

	tx, err := explorerClient.GetTxByTxHash(context.Background(), "0xb59239ef3c2c0468b95593bfca718a84f95e5d2d9becf73177b1010d315deefa")
	if err != nil {
		panic(err)
	}
	// fmt.Println(tx.String())

	txMessage := tx.Data.GetMessages()

	fmt.Println(string(txMessage))

	fmt.Println("================================================")

	txBody := &cosmostxpb.TxBody{}
	
	err = txBody.Unmarshal(txMessage)
	if err != nil {
		panic(err)
	}
	fmt.Println(txBody.String())
}
