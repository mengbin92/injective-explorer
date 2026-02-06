package service

import (
	"testing"

	peggytypes "github.com/InjectiveLabs/sdk-go/chain/peggy/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"cosmossdk.io/math"
)

func TestParseMsgDepositClaim(t *testing.T) {
	msg := &peggytypes.MsgDepositClaim{
		EventNonce:     42,
		BlockHeight:    100000,
		TokenContract:  "0xabc123",
		Amount:         math.NewInt(1000000),
		EthereumSender: "0xdef456",
		CosmosReceiver: "inj1receiver",
		Orchestrator:   "inj1validator",
	}

	result, err := parseMsgDepositClaim(msg, nil, "txhash123", 50000, 1234567890)

	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, uint64(42), result.EventNonce)
	assert.Equal(t, uint64(100000), result.EventHeight)
	assert.Equal(t, "0xdef456", result.Sender)
	assert.Equal(t, "inj1receiver", result.Receiver)
	assert.Equal(t, "1000000", result.Amount)
	assert.Equal(t, "peggy0xabc123", result.Denom)
	assert.Equal(t, "inj1validator", result.OrchestratorAddress)
	assert.Equal(t, "completed", result.State)
	assert.Equal(t, int32(1), result.ClaimType)
	assert.Equal(t, "txhash123", result.TxHash)
	assert.Equal(t, uint64(50000), result.BlockHeight)
	assert.Equal(t, uint64(1234567890), result.BlockTimestamp)
}

func TestParseMsgDepositClaim_Nil(t *testing.T) {
	result, err := parseMsgDepositClaim(nil, nil, "txhash", 100, 200)

	assert.Error(t, err)
	assert.Nil(t, result)
}
