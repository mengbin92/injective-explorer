package service

import (
	"testing"

	peggytypes "github.com/InjectiveLabs/sdk-go/chain/peggy/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
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

func TestParseMsgSendToEth(t *testing.T) {
	msg := &peggytypes.MsgSendToEth{
		Sender:  "inj1sender",
		EthDest: "0xethaddress",
		Amount: sdk.Coin{
			Denom:  "peggy0xtoken",
			Amount: math.NewInt(5000000),
		},
		BridgeFee: sdk.Coin{
			Denom:  "peggy0xtoken",
			Amount: math.NewInt(1000),
		},
	}

	result, err := parseMsgSendToEth(msg, nil, "txhash456", 60000, 1234567900)

	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "inj1sender", result.Sender)
	assert.Equal(t, "0xethaddress", result.Receiver)
	assert.Equal(t, "5000000", result.Amount)
	assert.Equal(t, "peggy0xtoken", result.Denom)
	assert.Equal(t, "1000", result.BridgeFee)
	assert.Equal(t, "pending", result.State) // MsgSendToEth starts as pending
	assert.Equal(t, int32(2), result.ClaimType)
	assert.Equal(t, "txhash456", result.TxHash)
	assert.Equal(t, uint64(60000), result.BlockHeight)
	assert.Equal(t, uint64(1234567900), result.BlockTimestamp)
	assert.Equal(t, uint64(0), result.OutgoingTxId) // Should be enriched from events
}

func TestParseMsgWithdrawClaim(t *testing.T) {
	msg := &peggytypes.MsgWithdrawClaim{
		EventNonce:   100,
		BatchNonce:   50,
		BlockHeight:  200000,
		Orchestrator: "inj1orchestrator",
	}

	result, err := parseMsgWithdrawClaim(msg, nil, "txhash789", 70000, 1234567910)

	require.NoError(t, err)
	require.NotNil(t, result)

	// MsgWithdrawClaim should have batch_nonce but no outgoing_tx_id
	assert.Equal(t, uint64(50), result.BatchNonce)
	assert.Equal(t, uint64(100), result.EventNonce)
	assert.Equal(t, uint64(200000), result.EventHeight)
	assert.Equal(t, "inj1orchestrator", result.OrchestratorAddress)
	assert.Equal(t, "completed", result.State) // MsgWithdrawClaim marks as completed
	assert.Equal(t, int32(2), result.ClaimType)
	assert.Equal(t, "txhash789", result.TxHash)
	assert.Equal(t, uint64(70000), result.BlockHeight)
	assert.Equal(t, uint64(1234567910), result.BlockTimestamp)

	// This is the key assertion: OutgoingTxId should be 0 for MsgWithdrawClaim
	assert.Equal(t, uint64(0), result.OutgoingTxId)
}

func TestParseMsgWithdrawClaim_Nil(t *testing.T) {
	result, err := parseMsgWithdrawClaim(nil, nil, "txhash", 100, 200)

	assert.Error(t, err)
	assert.Nil(t, result)
}
