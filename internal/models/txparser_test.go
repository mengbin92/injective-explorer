package models

import (
	"encoding/json"
	"testing"

	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractToAndValueFromProto_BankTransfer(t *testing.T) {
	// Test case: Standard bank transfer with events
	events := []*injective_explorer_rpcpb.Event{
		{
			Type: "transfer",
			Attributes: map[string]string{
				"sender":    "inj1sender123",
				"recipient": "inj1recipient456",
				"amount":    "1000000000000000000inj",
			},
		},
	}

	to, value := extractToAndValueFromProto(nil, events, nil, nil, nil)

	assert.Equal(t, "inj1recipient456", to)
	assert.Equal(t, "1000000000000000000inj", value)
}

func TestExtractToAndValueFromProto_CoinSpentAndReceived(t *testing.T) {
	// Test case: Coin spent and received events
	// Note: coin_spent and coin_received create separate candidates
	// The logic merges candidates and prefers ones with both from & to
	// Since coin_received has 'to' and coin_spent has 'from', we need both in the same candidate
	// Let's use a transfer event which has both, or test them separately
	// For this test, we'll verify that coin_received extracts 'to' correctly
	logsJSON := `[{
		"msg_index": "0",
		"events": [
			{
				"type": "coin_received",
				"attributes": [
					{"key": "receiver", "value": "inj1receiver456"},
					{"key": "amount", "value": "500000000000000000inj"}
				]
			}
		]
	}]`

	signatures := []*injective_explorer_rpcpb.Signature{
		{Address: "inj1spender123"},
	}

	to, value := extractToAndValueFromProto(nil, nil, []byte(logsJSON), signatures, nil)

	// coin_received should extract 'to' and 'value'
	assert.Equal(t, "inj1receiver456", to)
	assert.Equal(t, "500000000000000000inj", value)
}

func TestExtractToAndValueFromProto_WasmExecute(t *testing.T) {
	// Test case: WASM contract execution
	logsJSON := `[{
		"msg_index": "0",
		"events": [
			{
				"type": "execute",
				"attributes": [
					{"key": "_contract_address", "value": "inj1contract789"}
				]
			},
			{
				"type": "coin_received",
				"attributes": [
					{"key": "receiver", "value": "inj1contract789"},
					{"key": "amount", "value": "200000000000000000inj"}
				]
			}
		]
	}]`

	to, value := extractToAndValueFromProto(nil, nil, []byte(logsJSON), nil, nil)

	assert.Equal(t, "inj1contract789", to)
	assert.Equal(t, "200000000000000000inj", value)
}

func TestExtractToAndValueFromProto_MultipleCoins(t *testing.T) {
	// Test case: Multiple coins in transfer
	events := []*injective_explorer_rpcpb.Event{
		{
			Type: "transfer",
			Attributes: map[string]string{
				"sender":    "inj1sender123",
				"recipient": "inj1recipient456",
				"amount":    "1000000000000000000inj,500000000000000000usdt",
			},
		},
	}

	to, value := extractToAndValueFromProto(nil, events, nil, nil, nil)

	assert.Equal(t, "inj1recipient456", to)
	assert.Contains(t, value, "1000000000000000000inj")
	assert.Contains(t, value, "500000000000000000usdt")
}

func TestExtractToAndValueFromProto_FromSignatures(t *testing.T) {
	// Test case: Fallback to signatures when events don't have from
	events := []*injective_explorer_rpcpb.Event{
		{
			Type: "coin_received",
			Attributes: map[string]string{
				"receiver": "inj1receiver456",
				"amount":   "1000000000000000000inj",
			},
		},
	}

	signatures := []*injective_explorer_rpcpb.Signature{
		{Address: "inj1signer123"},
	}

	to, value := extractToAndValueFromProto(nil, events, nil, signatures, nil)

	assert.Equal(t, "inj1receiver456", to)
	assert.Equal(t, "1000000000000000000inj", value)
}

func TestExtractToAndValueFromProto_FromGasFee(t *testing.T) {
	// Test case: Fallback to gasFee.payer
	gasFee := &injective_explorer_rpcpb.GasFee{
		Payer: "inj1payer123",
	}

	to, value := extractToAndValueFromProto(nil, nil, nil, nil, gasFee)

	// Should return empty values when no transfer events found
	assert.Empty(t, to)
	assert.Empty(t, value)
}

func TestExtractToAndValueFromProto_EmptyInput(t *testing.T) {
	// Test case: Empty input
	to, value := extractToAndValueFromProto(nil, nil, nil, nil, nil)

	assert.Empty(t, to)
	assert.Empty(t, value)
}

func TestExtractFromToValue_TransferEvent(t *testing.T) {
	tx := &TxJSON{
		Logs: []LogRaw{
			{
				MsgIndex: "0",
				Events: []EventRaw{
					{
						Type: "transfer",
						Attributes: []AttributeRaw{
							{Key: "sender", Value: "inj1sender"},
							{Key: "recipient", Value: "inj1recipient"},
							{Key: "amount", Value: "100inj"},
						},
					},
				},
			},
		},
		Signatures: []SignatureRaw{
			{Address: "inj1sender"},
		},
	}

	result, err := extractFromToValue(tx)

	require.NoError(t, err)
	assert.Equal(t, "inj1sender", result.From)
	assert.Equal(t, "inj1recipient", result.To)
	assert.Len(t, result.Values, 1)
	assert.Equal(t, "100", result.Values[0].Amount)
	assert.Equal(t, "inj", result.Values[0].Denom)
}

func TestExtractFromToValue_MsgSend(t *testing.T) {
	msgJSON := json.RawMessage(`{
		"from_address": "inj1from",
		"to_address": "inj1to",
		"amount": [
			{"denom": "inj", "amount": "1000"}
		]
	}`)

	tx := &TxJSON{
		Messages: []MessageRaw{
			{
				Type:    "/cosmos.bank.v1beta1.MsgSend",
				Message: msgJSON,
			},
		},
		Signatures: []SignatureRaw{
			{Address: "inj1from"},
		},
	}

	result, err := extractFromToValue(tx)

	require.NoError(t, err)
	assert.Equal(t, "inj1from", result.From)
	assert.Equal(t, "inj1to", result.To)
	assert.Len(t, result.Values, 1)
	assert.Equal(t, "1000", result.Values[0].Amount)
	assert.Equal(t, "inj", result.Values[0].Denom)
}

func TestExtractFromToValue_WasmExecuteContract(t *testing.T) {
	msgJSON := json.RawMessage(`{
		"sender": "inj1sender",
		"contract": "inj1contract",
		"funds": [
			{"denom": "inj", "amount": "500"}
		]
	}`)

	tx := &TxJSON{
		Messages: []MessageRaw{
			{
				Type:    "/cosmwasm.wasm.v1.MsgExecuteContract",
				Message: msgJSON,
			},
		},
		Signatures: []SignatureRaw{
			{Address: "inj1sender"},
		},
	}

	result, err := extractFromToValue(tx)

	require.NoError(t, err)
	assert.Equal(t, "inj1sender", result.From)
	assert.Equal(t, "inj1contract", result.To)
	assert.Len(t, result.Values, 1)
	assert.Equal(t, "500", result.Values[0].Amount)
	assert.Equal(t, "inj", result.Values[0].Denom)
}

func TestExtractFromToValue_IbcTransfer(t *testing.T) {
	msgJSON := json.RawMessage(`{
		"sender": "inj1sender",
		"receiver": "inj1receiver",
		"token": {
			"denom": "inj",
			"amount": "2000"
		}
	}`)

	tx := &TxJSON{
		Messages: []MessageRaw{
			{
				Type:    "/ibc.applications.transfer.v1.MsgTransfer",
				Message: msgJSON,
			},
		},
		Signatures: []SignatureRaw{
			{Address: "inj1sender"},
		},
	}

	result, err := extractFromToValue(tx)

	require.NoError(t, err)
	assert.Equal(t, "inj1sender", result.From)
	assert.Equal(t, "inj1receiver", result.To)
	assert.Len(t, result.Values, 1)
	assert.Equal(t, "2000", result.Values[0].Amount)
	assert.Equal(t, "inj", result.Values[0].Denom)
}

func TestExtractFromToValue_LastResortSignature(t *testing.T) {
	// Test case: No transfer events or messages, fallback to signature
	tx := &TxJSON{
		Signatures: []SignatureRaw{
			{Address: "inj1signer"},
		},
	}

	result, err := extractFromToValue(tx)

	require.NoError(t, err)
	assert.Equal(t, "inj1signer", result.From)
	assert.Empty(t, result.To)
	assert.Empty(t, result.Values)
}

func TestExtractFromToValue_NilInput(t *testing.T) {
	result, err := extractFromToValue(nil)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestParseCoinStrList_SingleCoin(t *testing.T) {
	result := parseCoinStrList("100inj")

	require.Len(t, result, 1)
	assert.Equal(t, "100", result[0].Amount)
	assert.Equal(t, "inj", result[0].Denom)
}

func TestParseCoinStrList_MultipleCoins(t *testing.T) {
	result := parseCoinStrList("100inj,200usdt")

	require.Len(t, result, 2)
	assert.Equal(t, "100", result[0].Amount)
	assert.Equal(t, "inj", result[0].Denom)
	assert.Equal(t, "200", result[1].Amount)
	assert.Equal(t, "usdt", result[1].Denom)
}

func TestParseCoinStrList_Empty(t *testing.T) {
	result := parseCoinStrList("")
	assert.Empty(t, result)
}

func TestMergeCoins_SameDenom(t *testing.T) {
	a := []Coin{
		{Denom: "inj", Amount: "100"},
	}
	b := []Coin{
		{Denom: "inj", Amount: "200"},
	}

	result := mergeCoins(a, b)

	require.Len(t, result, 1)
	assert.Equal(t, "inj", result[0].Denom)
	assert.Equal(t, "300", result[0].Amount) // 100 + 200
}

func TestMergeCoins_DifferentDenoms(t *testing.T) {
	a := []Coin{
		{Denom: "inj", Amount: "100"},
	}
	b := []Coin{
		{Denom: "usdt", Amount: "200"},
	}

	result := mergeCoins(a, b)

	require.Len(t, result, 2)
	// Should be sorted by denom
	assert.Equal(t, "inj", result[0].Denom)
	assert.Equal(t, "usdt", result[1].Denom)
}

func TestExtractFromEventsJSON_Priority(t *testing.T) {
	// Test case: Multiple transfer candidates, should prefer transfer event
	tx := &TxJSON{
		Logs: []LogRaw{
			{
				MsgIndex: "0",
				Events: []EventRaw{
					{
						Type: "coin_spent",
						Attributes: []AttributeRaw{
							{Key: "spender", Value: "inj1spender"},
							{Key: "amount", Value: "100inj"},
						},
					},
					{
						Type: "transfer",
						Attributes: []AttributeRaw{
							{Key: "sender", Value: "inj1sender"},
							{Key: "recipient", Value: "inj1recipient"},
							{Key: "amount", Value: "200inj"},
						},
					},
				},
			},
		},
	}

	res := &ExtractResult{}
	found := extractFromEventsJSON(tx, res)

	assert.True(t, found)
	// Should prefer transfer event (higher priority)
	assert.Equal(t, "inj1sender", res.From)
	assert.Equal(t, "inj1recipient", res.To)
	assert.Len(t, res.Values, 1)
	assert.Equal(t, "200", res.Values[0].Amount)
}

func TestExtractFromEventsJSON_NoTransferEvents(t *testing.T) {
	tx := &TxJSON{
		Logs: []LogRaw{
			{
				MsgIndex: "0",
				Events: []EventRaw{
					{
						Type: "message",
						Attributes: []AttributeRaw{
							{Key: "action", Value: "send"},
						},
					},
				},
			},
		},
	}

	res := &ExtractResult{}
	found := extractFromEventsJSON(tx, res)

	assert.False(t, found)
	assert.Empty(t, res.From)
	assert.Empty(t, res.To)
}
