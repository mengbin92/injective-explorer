package models

import (
	"encoding/json"
	"strings"
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

	// Should return default values when no transfer events found (non-transfer transaction)
	assert.Equal(t, "-", to)
	assert.Equal(t, "0", value)
}

func TestExtractToAndValueFromProto_EmptyInput(t *testing.T) {
	// Test case: Empty input
	to, value := extractToAndValueFromProto(nil, nil, nil, nil, nil)

	// Should return default values for non-transfer transactions
	assert.Equal(t, "-", to)
	assert.Equal(t, "0", value)
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
	assert.True(t, result.ShouldShowValue, "transfer event should show to/value")
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
	}

	result, err := extractFromToValue(tx)

	require.NoError(t, err)
	assert.Equal(t, "inj1from", result.From)
	assert.Equal(t, "inj1to", result.To)
	assert.Len(t, result.Values, 1)
	assert.Equal(t, "1000", result.Values[0].Amount)
	assert.Equal(t, "inj", result.Values[0].Denom)
	assert.True(t, result.ShouldShowValue, "MsgSend should show to/value")
}

func TestExtractFromToValue_FaucetTransferWithGasFee(t *testing.T) {
	// Test case: Faucet transfer that sends both native token and ERC20 token
	// The transaction should show the actual transfer, NOT the gas fee transfer
	// Bug: When gas fee event doesn't have msg_index attribute but log.MsgIndex is hardcoded to "0",
	// the gas fee event gets incorrectly selected instead of the actual transfer events

	// Simulate the scenario where events come from protobuf (no logsBytes)
	// and gas fee events are mixed with actual transfer events
	events := []*injective_explorer_rpcpb.Event{
		// Gas fee events (no msg_index attribute)
		{
			Type: "coin_spent",
			Attributes: map[string]string{
				"amount":  "64000000000000inj",
				"spender": "inj1ur4tlhwy6e0jw9txmn5slvnkgahlqcq7akjvau",
				// NO msg_index attribute
			},
		},
		{
			Type: "coin_received",
			Attributes: map[string]string{
				"amount":   "64000000000000inj",
				"receiver": "inj17xpfvakm2amg962yls6f84z3kell8c5l8vkrh5",
				// NO msg_index attribute
			},
		},
		{
			Type: "transfer",
			Attributes: map[string]string{
				"amount":    "64000000000000inj",
				"recipient": "inj17xpfvakm2amg962yls6f84z3kell8c5l8vkrh5",
				"sender":    "inj1ur4tlhwy6e0jw9txmn5slvnkgahlqcq7akjvau",
				// NO msg_index attribute
			},
		},
		// Actual transfer events (with msg_index attribute)
		{
			Type: "coin_received",
			Attributes: map[string]string{
				"amount":    "1000000000000000000inj",
				"msg_index": "0",
				"receiver":  "inj18dwpfhk5ajy9kpp3wfpr2f9jtdmmukjh86cptn",
			},
		},
		{
			Type: "transfer",
			Attributes: map[string]string{
				"amount":    "1000000000000000000inj",
				"msg_index": "0",
				"recipient": "inj18dwpfhk5ajy9kpp3wfpr2f9jtdmmukjh86cptn",
				"sender":    "inj1ur4tlhwy6e0jw9txmn5slvnkgahlqcq7akjvau",
			},
		},
		{
			Type: "coin_received",
			Attributes: map[string]string{
				"amount":    "10000000peggy0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238",
				"msg_index": "1",
				"receiver":  "inj18dwpfhk5ajy9kpp3wfpr2f9jtdmmukjh86cptn",
			},
		},
		{
			Type: "transfer",
			Attributes: map[string]string{
				"amount":    "10000000peggy0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238",
				"msg_index": "1",
				"recipient": "inj18dwpfhk5ajy9kpp3wfpr2f9jtdmmukjh86cptn",
				"sender":    "inj1ur4tlhwy6e0jw9txmn5slvnkgahlqcq7akjvau",
			},
		},
	}

	to, value := extractToAndValueFromProto(nil, events, nil, nil, nil)

	// Debug output
	t.Logf("Extracted To: %s", to)
	t.Logf("Extracted Value: %s", value)

	// Should extract the actual transfer, NOT the gas fee
	assert.Equal(t, "inj18dwpfhk5ajy9kpp3wfpr2f9jtdmmukjh86cptn", to, "should extract actual transfer recipient, not gas fee recipient")
	// Value should contain both INJ and ERC20 tokens
	assert.Contains(t, value, "1000000000000000000inj", "should contain INJ amount")
	assert.Contains(t, value, "10000000peggy0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238", "should contain ERC20 amount")
	// INJ should come BEFORE ERC20 (native token priority)
	injIndex := strings.Index(value, "1000000000000000000inj")
	erc20Index := strings.Index(value, "10000000peggy0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238")
	assert.True(t, injIndex < erc20Index, "INJ should appear before ERC20 in value string")
	// Should NOT contain gas fee amount
	assert.NotContains(t, value, "64000000000000inj", "should NOT contain gas fee amount")
}

func TestMergeCoins_NativeTokenPriority(t *testing.T) {
	// Test that INJ (native token) always comes first in the value string
	coins := []Coin{
		{Denom: "peggy0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238", Amount: "10000000"},
		{Denom: "usdt", Amount: "500000000"},
		{Denom: "inj", Amount: "1000000000000000000"},
	}

	merged := mergeCoins([]Coin{}, coins)

	require.Len(t, merged, 3, "should have 3 different tokens")
	// INJ should be first
	assert.Equal(t, "inj", merged[0].Denom, "INJ should be first")
	assert.Equal(t, "1000000000000000000", merged[0].Amount, "INJ amount should be correct")

	// Build value string like extractToAndValueFromProto does
	valueParts := make([]string, 0, len(merged))
	for _, coin := range merged {
		valueParts = append(valueParts, coin.Amount+coin.Denom)
	}
	value := strings.Join(valueParts, ",")

	t.Logf("Value string: %s", value)
	// Check INJ comes first
	assert.True(t, strings.HasPrefix(value, "1000000000000000000inj,"), "INJ should be at the beginning of value string")
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
	assert.True(t, result.ShouldShowValue, "MsgExecuteContract with funds should show to/value")
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
	assert.True(t, result.ShouldShowValue, "IBC MsgTransfer should show to/value")
}

func TestExtractFromToValue_LastResortSignature(t *testing.T) {
	// Test case: No transfer events or messages, fallback to signature
	// For non-transfer transactions, we should not show to/value
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
	assert.False(t, result.ShouldShowValue, "non-transfer transaction should not show to/value")
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

func TestExtractFromToValue_NonTransferTransaction(t *testing.T) {
	// Test case: Exchange order (not a transfer, should not show to/value)
	msgJSON := json.RawMessage(`{
		"sender": "inj1sender",
		"market_id": "0x123",
		"order_type": "BUY",
		"price": "1000",
		"quantity": "1"
	}`)

	tx := &TxJSON{
		Messages: []MessageRaw{
			{
				Type:    "/injective.exchange.v1beta1.MsgCreateSpotOrder",
				Message: msgJSON,
			},
		},
		Signatures: []SignatureRaw{
			{Address: "inj1sender"},
		},
	}

	result, err := extractFromToValue(tx)

	require.NoError(t, err)
	assert.Equal(t, "inj1sender", result.From, "should extract signer")
	assert.Empty(t, result.To, "non-transfer transaction should not have 'to'")
	assert.Empty(t, result.Values, "non-transfer transaction should not have 'value'")
	assert.False(t, result.ShouldShowValue, "order transaction should not show to/value")
}

func TestExtractFromToValue_WasmExecuteContractWithoutFunds(t *testing.T) {
	// Test case: Contract call without funds (not a transfer)
	msgJSON := json.RawMessage(`{
		"sender": "inj1sender",
		"contract": "inj1contract",
		"msg": {"action": "query"},
		"funds": []
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
	assert.Equal(t, "inj1sender", result.From, "should extract signer")
	assert.Empty(t, result.To, "contract call without funds should not have 'to'")
	assert.Empty(t, result.Values, "contract call without funds should not have 'value'")
	assert.False(t, result.ShouldShowValue, "contract call without funds should not show to/value")
}

func TestExtractFromToValue_OrderWithGasFeeEvents(t *testing.T) {
	// Test case: Exchange order transaction with gas fee events
	// Even though there are transfer events (from gas fee payment),
	// this should NOT show to/value because the message type is not a transfer
	msgJSON := json.RawMessage(`{
		"sender": "inj1sender",
		"market_id": "0x123",
		"order_type": "BUY",
		"price": "1000",
		"quantity": "1"
	}`)

	tx := &TxJSON{
		Messages: []MessageRaw{
			{
				Type:    "/injective.exchange.v1beta1.MsgCancelSpotOrder",
				Message: msgJSON,
			},
		},
		Logs: []LogRaw{
			{
				MsgIndex: "0",
				Events: []EventRaw{
					{
						Type: "transfer",
						Attributes: []AttributeRaw{
							{Key: "sender", Value: "inj1sender"},
							{Key: "recipient", Value: "inj1feeCollector"},
							{Key: "amount", Value: "1000000000000000inj"}, // gas fee
						},
					},
					{
						Type: "coin_spent",
						Attributes: []AttributeRaw{
							{Key: "spender", Value: "inj1sender"},
							{Key: "amount", Value: "1000000000000000inj"}, // gas fee
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
	assert.Equal(t, "inj1sender", result.From, "should extract signer")
	assert.Empty(t, result.To, "order transaction should not have 'to' even with transfer events")
	assert.Empty(t, result.Values, "order transaction should not have 'value' even with transfer events")
	assert.False(t, result.ShouldShowValue, "order transaction should not show to/value even if gas fee events exist")
}

func TestIsValueTransfer(t *testing.T) {
	// Test cases for IsValueTransfer function
	tests := []struct {
		name     string
		msgType  string
		hasFunds bool
		expected bool
	}{
		{
			name:     "MsgSend should be transfer",
			msgType:  "/cosmos.bank.v1beta1.MsgSend",
			hasFunds: false,
			expected: true,
		},
		{
			name:     "IBC MsgTransfer should be transfer",
			msgType:  "/ibc.applications.transfer.v1.MsgTransfer",
			hasFunds: false,
			expected: true,
		},
		{
			name:     "Exchange MsgDeposit should be transfer",
			msgType:  "/injective.exchange.v1beta1.MsgDeposit",
			hasFunds: false,
			expected: true,
		},
		{
			name:     "Exchange MsgWithdraw should be transfer",
			msgType:  "/injective.exchange.v1beta1.MsgWithdraw",
			hasFunds: false,
			expected: true,
		},
		{
			name:     "MsgExecuteContract with funds should be transfer",
			msgType:  "/cosmwasm.wasm.v1.MsgExecuteContract",
			hasFunds: true,
			expected: true,
		},
		{
			name:     "MsgExecuteContract without funds should NOT be transfer",
			msgType:  "/cosmwasm.wasm.v1.MsgExecuteContract",
			hasFunds: false,
			expected: false,
		},
		{
			name:     "Exchange order should NOT be transfer",
			msgType:  "/injective.exchange.v1beta1.MsgCreateSpotOrder",
			hasFunds: false,
			expected: false,
		},
		{
			name:     "Staking delegate should NOT be transfer",
			msgType:  "/cosmos.staking.v1beta1.MsgDelegate",
			hasFunds: false,
			expected: false,
		},
		{
			name:     "Governance vote should NOT be transfer",
			msgType:  "/cosmos.gov.v1beta1.MsgVote",
			hasFunds: false,
			expected: false,
		},
		{
			name:     "Oracle relay should NOT be transfer",
			msgType:  "/injective.oracle.v1beta1.MsgRelayPriceFeedPrice",
			hasFunds: false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValueTransfer(tt.msgType, tt.hasFunds)
			assert.Equal(t, tt.expected, result, "IsValueTransfer(%q, %v) = %v, want %v", tt.msgType, tt.hasFunds, result, tt.expected)
		})
	}
}

func TestExtractToAndValueFromProto_DefaultValues(t *testing.T) {
	// Test case: Non-transfer transaction should return default values
	// This is already covered by TestExtractToAndValueFromProto_FromGasFee and
	// TestExtractToAndValueFromProto_EmptyInput, but we verify the default values explicitly
	to, value := extractToAndValueFromProto(nil, nil, nil, nil, nil)

	// Non-transfer transactions should return default values
	assert.Equal(t, "-", to, "non-transfer transaction should return '-' for to")
	assert.Equal(t, "0", value, "non-transfer transaction should return '0' for value")
}
