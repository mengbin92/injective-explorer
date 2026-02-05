// Package models provides test cases for message parser functionality
package models

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/bytedance/sonic"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cosmostxpb "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMessagesToFormattedJSON_EmptyInput(t *testing.T) {
	result := ParseMessagesToFormattedJSON(nil)
	assert.Equal(t, "", result)

	result = ParseMessagesToFormattedJSON([]byte{})
	assert.Equal(t, "", result)
}

func TestParseMessagesToFormattedJSON_TxBodyWithMockMessages(t *testing.T) {
	// Create a simple TxBody with mock messages
	txBody := &cosmostxpb.TxBody{
		Messages: []*codectypes.Any{
			{
				TypeUrl: "/cosmos.bank.v1beta1.MsgSend",
				Value:   []byte("mock_value"), // This will be base64 encoded if type not found
			},
		},
		Memo: "test memo",
	}

	// Marshal TxBody to bytes
	txBodyBytes, err := txBody.Marshal()
	require.NoError(t, err)

	// Parse the messages
	result := ParseMessagesToFormattedJSON(txBodyBytes)

	// Verify the result is valid JSON
	var resultMap map[string]interface{}
	err = sonic.Unmarshal([]byte(result), &resultMap)
	require.NoError(t, err)

	// Check that messages array exists
	messages, ok := resultMap["messages"].([]interface{})
	assert.True(t, ok)
	assert.Equal(t, 1, len(messages))

	// Check that memo is included
	memo, ok := resultMap["memo"].(string)
	assert.True(t, ok)
	assert.Equal(t, "test memo", memo)

	// Check message structure
	msg, ok := messages[0].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "/cosmos.bank.v1beta1.MsgSend", msg["@type"])
}

func TestParseMessagesToFormattedJSON_TxRaw(t *testing.T) {
	// Create a TxBody
	txBody := &cosmostxpb.TxBody{
		Messages: []*codectypes.Any{
			{
				TypeUrl: "/cosmos.bank.v1beta1.MsgSend",
				Value:   []byte("test"),
			},
		},
		Memo: "test memo",
	}

	// Marshal TxBody
	bodyBytes, err := txBody.Marshal()
	require.NoError(t, err)

	// Create TxRaw with the BodyBytes
	txRaw := &cosmostxpb.TxRaw{
		BodyBytes: bodyBytes,
	}

	// Marshal TxRaw
	txRawBytes, err := txRaw.Marshal()
	require.NoError(t, err)

	// Parse the messages
	result := ParseMessagesToFormattedJSON(txRawBytes)

	// Verify the result is valid JSON
	var resultMap map[string]interface{}
	err = sonic.Unmarshal([]byte(result), &resultMap)
	require.NoError(t, err)

	// Check that messages array exists
	messages, ok := resultMap["messages"].([]interface{})
	assert.True(t, ok)
	assert.Equal(t, 1, len(messages))

	// Check memo
	assert.Equal(t, "test memo", resultMap["memo"])
}

func TestParseMessagesToFormattedJSON_JSONInput(t *testing.T) {
	// Test with existing JSON format (should be enhanced if possible)
	inputJSON := `{
		"messages": [
			{
				"@type": "/cosmos.bank.v1beta1.MsgSend",
				"value": "dGVzdA=="
			}
		],
		"memo": "test"
	}`

	result := ParseMessagesToFormattedJSON([]byte(inputJSON))

	// Verify the result is valid JSON
	var resultMap map[string]interface{}
	err := sonic.Unmarshal([]byte(result), &resultMap)
	require.NoError(t, err)

	// Should maintain the structure
	messages, ok := resultMap["messages"].([]interface{})
	assert.True(t, ok)
	assert.Equal(t, 1, len(messages))
}

func TestParseMessagesToFormattedJSON_InvalidInput(t *testing.T) {
	// Test with completely invalid protobuf data
	invalidData := []byte{0xFF, 0xFF, 0xFF, 0xFF}

	result := ParseMessagesToFormattedJSON(invalidData)

	// Should return base64 encoded fallback
	assert.True(t, strings.Contains(result, base64.StdEncoding.EncodeToString(invalidData)) ||
		result == base64.StdEncoding.EncodeToString(invalidData))
}

func TestIsAddressFieldName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"from_address", "from_address", true},
		{"to_address", "to_address", true},
		{"sender", "sender", true},
		{"delegator", "delegator", true},
		{"validator", "validator", true},
		{"not_address", "amount", false},
		{"denom", "denom", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAddressFieldName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsAddressLikeValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid_cosmos", "cosmos1abc123def456ghi789", true},
		{"valid_inj", "inj1qwertyuiopasdfghjkl", true},
		{"valid_osmo", "osmo1zxcvbnmlkjhgfdsaqwert", true},
		{"too_short", "inj1abc", false},
		{"no_separator", "cosmosabc123", false},
		{"invalid_chars", "cosmos1ABC123", false},
		{"not_address", "1000000", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAddressLikeValue(tt.input)
			assert.Equal(t, tt.expected, result, "For input: %s", tt.input)
		})
	}
}

func TestConvertAddressesInValue(t *testing.T) {
	tests := []struct {
		name      string
		value     interface{}
		fieldName string
		expected  interface{}
	}{
		{
			name:      "string_address_field",
			value:     "cosmos1abc123def456ghi789jklmno",
			fieldName: "from_address",
			expected:  ConvertAddressToEthForStorage("cosmos1abc123def456ghi789jklmno"),
		},
		{
			name:      "string_non_address",
			value:     "1000000",
			fieldName: "amount",
			expected:  "1000000",
		},
		{
			name: "nested_map",
			value: map[string]interface{}{
				"from_address": "cosmos1test",
				"amount":       "1000",
			},
			fieldName: "data",
			expected: map[string]interface{}{
				"from_address": ConvertAddressToEthForStorage("cosmos1test"),
				"amount":       "1000",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertAddressesInValue(tt.value, tt.fieldName)
			assert.Equal(t, tt.expected, result)
		})
	}
}
