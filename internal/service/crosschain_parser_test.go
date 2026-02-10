package service

import (
	"context"
	"strings"
	"testing"

	"injective-explorer/internal/models"

	peggytypes "github.com/InjectiveLabs/sdk-go/chain/peggy/types"
	abci "github.com/cometbft/cometbft/abci/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	tx "github.com/cosmos/cosmos-sdk/types/tx"
	ibctypes "github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
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

	result, err := parseMsgDepositClaim(msg, nil, "txhash123", "", 50000, 1234567890)

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
	assert.Equal(t, "0xtxhash123", result.TxHash) // Should be normalized
	assert.Equal(t, uint64(50000), result.BlockHeight)
	assert.Equal(t, uint64(1234567890), result.BlockTimestamp)
}

func TestParseMsgDepositClaim_Nil(t *testing.T) {
	result, err := parseMsgDepositClaim(nil, nil, "txhash", "", 100, 200)

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

	result, err := parseMsgSendToEth(msg, nil, "txhash456", "", 60000, 1234567900)

	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "inj1sender", result.Sender)
	assert.Equal(t, "0xethaddress", result.Receiver)
	assert.Equal(t, "5000000", result.Amount)
	assert.Equal(t, "peggy0xtoken", result.Denom)
	assert.Equal(t, "1000", result.BridgeFee)
	assert.Equal(t, "pending", result.State) // MsgSendToEth starts as pending
	assert.Equal(t, int32(2), result.ClaimType)
	assert.Equal(t, "0xtxhash456", result.TxHash)
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

	result, err := parseMsgWithdrawClaim(msg, nil, "txhash789", "", 70000, 1234567910)

	require.NoError(t, err)
	require.NotNil(t, result)

	// MsgWithdrawClaim should have batch_nonce but no outgoing_tx_id
	assert.Equal(t, uint64(50), result.BatchNonce)
	assert.Equal(t, uint64(100), result.EventNonce)
	assert.Equal(t, uint64(200000), result.EventHeight)
	assert.Equal(t, "inj1orchestrator", result.OrchestratorAddress)
	assert.Equal(t, "completed", result.State) // MsgWithdrawClaim marks as completed
	assert.Equal(t, int32(2), result.ClaimType)
	assert.Equal(t, "0xtxhash789", result.TxHash)
	assert.Equal(t, uint64(70000), result.BlockHeight)
	assert.Equal(t, uint64(1234567910), result.BlockTimestamp)

	// This is the key assertion: OutgoingTxId should be 0 for MsgWithdrawClaim
	assert.Equal(t, uint64(0), result.OutgoingTxId)
}

func TestParseMsgWithdrawClaim_Nil(t *testing.T) {
	result, err := parseMsgWithdrawClaim(nil, nil, "txhash", "", 100, 200)

	assert.Error(t, err)
	assert.Nil(t, result)
}

// TestParseMsgTransfer tests IBC transfer message parsing
func TestParseMsgTransfer(t *testing.T) {
	msg := &ibctypes.MsgTransfer{
		SourcePort:    "transfer",
		SourceChannel: "channel-0",
		Token: sdk.Coin{
			Denom:  "uatom",
			Amount: math.NewInt(1000000),
		},
		Sender:   "cosmos1sender",
		Receiver: "inj1receiver",
		TimeoutHeight: clienttypes.Height{
			RevisionNumber: 1,
			RevisionHeight: 1000,
		},
		TimeoutTimestamp: 1234567890000000000,
	}

	result, err := parseMsgTransfer(msg, nil, "txhash123", "", 50000, 1234567890)

	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "cosmos1sender", result.Sender)
	assert.Equal(t, "inj1receiver", result.Receiver)
	assert.Equal(t, "transfer", result.SourcePort)
	assert.Equal(t, "channel-0", result.SourceChannel)
	assert.Equal(t, "1000000", result.Amount)
	assert.Equal(t, "uatom", result.Denom)
	assert.Equal(t, "1-1000", result.TimeoutHeight)
	assert.Equal(t, uint64(1234567890000000000), result.TimeoutTimestamp)
	assert.Equal(t, "completed", result.State)
}

// TestEnrichIBCTransferFromEvents tests IBC event enrichment
func TestEnrichIBCTransferFromEvents(t *testing.T) {
	transfer := &models.IBCTransferTx{}

	events := []*abci.Event{
		{
			Type: "send_packet",
			Attributes: []abci.EventAttribute{
				{Key: "packet_sequence", Value: "42"},
				{Key: "packet_dst_port", Value: "transfer"},
				{Key: "packet_dst_channel", Value: "channel-1"},
				{Key: "packet_data", Value: "test_data"},
			},
		},
	}

	enrichIBCTransferFromEvents(transfer, events)

	assert.Equal(t, uint64(42), transfer.PacketSequence)
	assert.Equal(t, "transfer", transfer.DestinationPort)
	assert.Equal(t, "channel-1", transfer.DestinationChannel)
	assert.Equal(t, []byte("test_data"), transfer.DataHex)
}

// TestEnrichPeggyWithdrawalFromEvents tests Peggy event enrichment
func TestEnrichPeggyWithdrawalFromEvents(t *testing.T) {
	withdrawal := &models.PeggyWithdrawalTx{}

	events := []*abci.Event{
		{
			Type: "send_to_eth",
			Attributes: []abci.EventAttribute{
				{Key: "outgoing_tx_id", Value: "123"},
				{Key: "batch_timeout", Value: "999999"},
			},
		},
	}

	enrichPeggyWithdrawalFromEvents(withdrawal, events)

	assert.Equal(t, uint64(123), withdrawal.OutgoingTxId)
	assert.Equal(t, uint64(999999), withdrawal.BatchTimeout)
}

// TestEnrichPeggyWithdrawalFromEvents_InvalidData tests error handling
func TestEnrichPeggyWithdrawalFromEvents_InvalidData(t *testing.T) {
	withdrawal := &models.PeggyWithdrawalTx{}

	events := []*abci.Event{
		{
			Type: "send_to_eth",
			Attributes: []abci.EventAttribute{
				{Key: "outgoing_tx_id", Value: "invalid"},
				{Key: "batch_timeout", Value: "also_invalid"},
			},
		},
	}

	enrichPeggyWithdrawalFromEvents(withdrawal, events)

	// Should remain 0 when parsing fails
	assert.Equal(t, uint64(0), withdrawal.OutgoingTxId)
	assert.Equal(t, uint64(0), withdrawal.BatchTimeout)
}

// TestParseUint64 tests the parseUint64 helper function
func TestParseUint64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint64
		wantErr bool
	}{
		{"valid number", "123", 123, false},
		{"zero", "0", 0, false},
		{"large number", "18446744073709551615", 18446744073709551615, false},
		{"empty string", "", 0, true},
		{"invalid string", "abc", 0, true},
		{"negative number", "-1", 0, true},
		{"float", "123.45", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUint64(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestParseCrossChainTransaction_Empty tests nil/empty inputs
func TestParseCrossChainTransaction_Empty(t *testing.T) {
	tests := []struct {
		name    string
		txResp  *tx.GetTxResponse
		wantNil bool
	}{
		{
			name:    "nil response",
			txResp:  nil,
			wantNil: true,
		},
		{
			name: "nil tx",
			txResp: &tx.GetTxResponse{
				Tx: nil,
			},
			wantNil: true,
		},
		{
			name: "nil body",
			txResp: &tx.GetTxResponse{
				Tx: &tx.Tx{Body: nil},
			},
			wantNil: true,
		},
		{
			name: "empty messages",
			txResp: &tx.GetTxResponse{
				Tx: &tx.Tx{
					Body: &tx.TxBody{Messages: []*codectypes.Any{}},
				},
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseCrossChainTransaction(context.Background(), tt.txResp, 100, 200)
			assert.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, result)
			}
		})
	}
}

// TestParseCrossChainTransaction_MsgDepositClaim tests full integration
func TestParseCrossChainTransaction_MsgDepositClaim(t *testing.T) {
	// Create a MsgDepositClaim
	msg := &peggytypes.MsgDepositClaim{
		EventNonce:     42,
		BlockHeight:    100000,
		TokenContract:  "0xabc123",
		Amount:         math.NewInt(1000000),
		EthereumSender: "0xdef456",
		CosmosReceiver: "inj1receiver",
		Orchestrator:   "inj1validator",
	}

	// Marshal the message
	cdc := getCodec()
	msgBytes, err := cdc.Marshal(msg)
	require.NoError(t, err)

	// Create Any wrapper
	anyMsg := &codectypes.Any{
		TypeUrl: "/injective.peggy.v1.MsgDepositClaim",
		Value:   msgBytes,
	}

	// Create transaction response with proper type
	txResp := &tx.GetTxResponse{
		Tx: &tx.Tx{
			Body: &tx.TxBody{
				Messages: []*codectypes.Any{anyMsg},
			},
		},
		TxResponse: &sdk.TxResponse{
			TxHash: "test_tx_hash",
			Events: []abci.Event{},
		},
	}

	// Parse
	result, err := ParseCrossChainTransaction(context.Background(), txResp, 50000, 1234567890)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsEmpty())
	assert.Len(t, result.PeggyDeposits, 1)

	deposit := result.PeggyDeposits[0]
	assert.Equal(t, uint64(42), deposit.EventNonce)
	assert.Equal(t, "0xdef456", deposit.Sender)
	assert.Equal(t, "inj1receiver", deposit.Receiver)
	assert.Equal(t, "completed", deposit.State)
}

// TestParseCrossChainTransaction_MultipleMsgs tests multiple messages in one tx
func TestParseCrossChainTransaction_MultipleMsgs(t *testing.T) {
	cdc := getCodec()

	// Create MsgDepositClaim
	depositMsg := &peggytypes.MsgDepositClaim{
		EventNonce:     1,
		BlockHeight:    100000,
		TokenContract:  "0xabc",
		Amount:         math.NewInt(1000),
		EthereumSender: "0xsender",
		CosmosReceiver: "inj1receiver",
	}
	depositBytes, _ := cdc.Marshal(depositMsg)

	// Create MsgSendToEth
	sendToEthMsg := &peggytypes.MsgSendToEth{
		Sender:  "inj1sender",
		EthDest: "0xdest",
		Amount: sdk.Coin{
			Denom:  "peggy0xtoken",
			Amount: math.NewInt(5000),
		},
		BridgeFee: sdk.Coin{
			Denom:  "peggy0xtoken",
			Amount: math.NewInt(100),
		},
	}
	sendToEthBytes, _ := cdc.Marshal(sendToEthMsg)

	txResp := &tx.GetTxResponse{
		Tx: &tx.Tx{
			Body: &tx.TxBody{
				Messages: []*codectypes.Any{
					{TypeUrl: "/injective.peggy.v1.MsgDepositClaim", Value: depositBytes},
					{TypeUrl: "/injective.peggy.v1.MsgSendToEth", Value: sendToEthBytes},
				},
			},
		},
		TxResponse: &sdk.TxResponse{
			TxHash: "multi_msg_tx",
			Events: []abci.Event{},
		},
	}

	result, err := ParseCrossChainTransaction(context.Background(), txResp, 50000, 1234567890)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.PeggyDeposits, 1)
	assert.Len(t, result.PeggyWithdrawals, 1)
	assert.Len(t, result.IBCTransfers, 0)
}

// TestParseCrossChainTransaction_WithEVMHash tests that EthTxHash is properly set when EVM hash is present
func TestParseCrossChainTransaction_WithEVMHash(t *testing.T) {
	cdc := getCodec()

	// Create a MsgDepositClaim
	msg := &peggytypes.MsgDepositClaim{
		EventNonce:     99,
		BlockHeight:    200000,
		TokenContract:  "0xTokenContract",
		Amount:         math.NewInt(5000000),
		EthereumSender: "0xEthSender",
		CosmosReceiver: "inj1receiver",
		Orchestrator:   "inj1orchestrator",
	}

	msgBytes, err := cdc.Marshal(msg)
	require.NoError(t, err)

	anyMsg := &codectypes.Any{
		TypeUrl: "/injective.peggy.v1.MsgDepositClaim",
		Value:   msgBytes,
	}

	// Create transaction response with EVM event
	evmHash := "0xABCDEF1234567890ABCDEF1234567890ABCDEF1234567890ABCDEF1234567890"
	txResp := &tx.GetTxResponse{
		Tx: &tx.Tx{
			Body: &tx.TxBody{
				Messages: []*codectypes.Any{anyMsg},
			},
		},
		TxResponse: &sdk.TxResponse{
			TxHash: "cosmos_tx_hash",
			Events: []abci.Event{
				{
					Type: "ethereum_tx",
					Attributes: []abci.EventAttribute{
						{
							Key:   "ethereumTxHash",
							Value: evmHash,
						},
					},
				},
			},
		},
	}

	// Parse
	result, err := ParseCrossChainTransaction(context.Background(), txResp, 60000, 9999999999)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.PeggyDeposits, 1)

	deposit := result.PeggyDeposits[0]

	// Verify that EthTxHash is set to the EVM hash (normalized)
	expectedEthTxHash := strings.ToLower(evmHash)
	assert.Equal(t, expectedEthTxHash, deposit.EthTxHash, "EthTxHash should be set to normalized EVM hash")

	// Verify that TxHash is also set to EVM hash (since evmHash is available)
	assert.Equal(t, expectedEthTxHash, deposit.TxHash, "TxHash should be set to normalized EVM hash when available")

	// Verify other fields
	assert.Equal(t, uint64(99), deposit.EventNonce)
	assert.Equal(t, "0xethsender", deposit.Sender) // Normalized to lowercase
}

// TestParseCrossChainTransaction_WithoutEVMHash tests that EthTxHash is empty when no EVM hash is present
func TestParseCrossChainTransaction_WithoutEVMHash(t *testing.T) {
	cdc := getCodec()

	msg := &peggytypes.MsgSendToEth{
		Sender:  "inj1sender",
		EthDest: "0xDestination",
		Amount: sdk.Coin{
			Denom:  "peggy0xToken",
			Amount: math.NewInt(3000),
		},
		BridgeFee: sdk.Coin{
			Denom:  "peggy0xToken",
			Amount: math.NewInt(100),
		},
	}

	msgBytes, err := cdc.Marshal(msg)
	require.NoError(t, err)

	anyMsg := &codectypes.Any{
		TypeUrl: "/injective.peggy.v1.MsgSendToEth",
		Value:   msgBytes,
	}

	// Create transaction response WITHOUT EVM event
	txResp := &tx.GetTxResponse{
		Tx: &tx.Tx{
			Body: &tx.TxBody{
				Messages: []*codectypes.Any{anyMsg},
			},
		},
		TxResponse: &sdk.TxResponse{
			TxHash: "COSMOS_TX_HASH_UPPER",
			Events: []abci.Event{
				{
					Type: "send_to_eth",
					Attributes: []abci.EventAttribute{
						{
							Key:   "outgoing_tx_id",
							Value: "123",
						},
					},
				},
			},
		},
	}

	result, err := ParseCrossChainTransaction(context.Background(), txResp, 70000, 8888888888)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.PeggyWithdrawals, 1)

	withdrawal := result.PeggyWithdrawals[0]

	// Verify that EthTxHash is empty (normalized empty string)
	assert.Equal(t, "", withdrawal.EthTxHash, "EthTxHash should be empty when no EVM hash is present")

	// Verify that TxHash is set to normalized Cosmos hash
	assert.Equal(t, "0xcosmos_tx_hash_upper", withdrawal.TxHash, "TxHash should be normalized Cosmos hash")

	// Verify that outgoing_tx_id was extracted from events
	assert.Equal(t, uint64(123), withdrawal.OutgoingTxId)
}

// TestCrossChainData_IsEmpty tests the IsEmpty method
func TestCrossChainData_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		data *CrossChainData
		want bool
	}{
		{
			name: "completely empty",
			data: &CrossChainData{},
			want: true,
		},
		{
			name: "has IBC transfer",
			data: &CrossChainData{
				IBCTransfers: []*models.IBCTransferTx{{}},
			},
			want: false,
		},
		{
			name: "has peggy deposit",
			data: &CrossChainData{
				PeggyDeposits: []*models.PeggyDepositTx{{}},
			},
			want: false,
		},
		{
			name: "has peggy withdrawal",
			data: &CrossChainData{
				PeggyWithdrawals: []*models.PeggyWithdrawalTx{{}},
			},
			want: false,
		},
		{
			name: "has all types",
			data: &CrossChainData{
				IBCTransfers:     []*models.IBCTransferTx{{}},
				PeggyDeposits:    []*models.PeggyDepositTx{{}},
				PeggyWithdrawals: []*models.PeggyWithdrawalTx{{}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.data.IsEmpty())
		})
	}
}

// TestGetCodec_Singleton tests that codec is reused
func TestGetCodec_Singleton(t *testing.T) {
	codec1 := getCodec()
	codec2 := getCodec()

	// Should return the same instance
	assert.Same(t, codec1, codec2, "getCodec should return singleton instance")
}

// TestExtractEvents tests event extraction
func TestExtractEvents(t *testing.T) {
	tests := []struct {
		name string
		resp *tx.GetTxResponse
		want int
	}{
		{
			name: "nil response",
			resp: &tx.GetTxResponse{},
			want: 0,
		},
		{
			name: "no events",
			resp: &tx.GetTxResponse{
				TxResponse: &sdk.TxResponse{Events: []abci.Event{}},
			},
			want: 0,
		},
		{
			name: "with events",
			resp: &tx.GetTxResponse{
				TxResponse: &sdk.TxResponse{
					Events: []abci.Event{
						{Type: "event1"},
						{Type: "event2"},
					},
				},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := extractEvents(tt.resp)
			assert.Len(t, events, tt.want)
		})
	}
}
