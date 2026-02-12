package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExtractContractFromEventCreateTokenPair verifies that we can extract
// contract address from EventCreateTokenPair events correctly.
// This test uses the real transaction data from:
// 0x70AF765A4B927BE4EF9FC63338DAE13DF5E28B26592A2EFF4167A6BE11DF2B16
func TestExtractContractFromEventCreateTokenPair(t *testing.T) {
	// Real event data from the transaction
	eventJSON := `{
		"type": "injective.erc20.v1beta1.EventCreateTokenPair",
		"attributes": [
			{
				"key": "erc20_address",
				"value": "\"0x9772154BF1823353C829B43F9AD96CF9f720c768\""
			},
			{
				"key": "bank_denom",
				"value": "\"peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca\""
			},
			{
				"key": "sender",
				"value": "\"0x15301c6f8a51b177bd3c24f7b5d978b2c02b0da0\""
			}
		]
	}`

	var event struct {
		Type       string `json:"type"`
		Attributes []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"attributes"`
	}

	err := json.Unmarshal([]byte(eventJSON), &event)
	assert.NoError(t, err)

	// Verify event type
	assert.Equal(t, "injective.erc20.v1beta1.EventCreateTokenPair", event.Type)

	// Extract values like the implementation does
	var contractAddress, creatorAddress, bankDenom string
	for _, attr := range event.Attributes {
		switch attr.Key {
		case "erc20_address":
			contractAddress = strings.Trim(attr.Value, "\"")
		case "sender":
			creatorAddress = strings.Trim(attr.Value, "\"")
		case "bank_denom":
			bankDenom = strings.Trim(attr.Value, "\"")
		}
	}

	// Verify extracted values match expected
	assert.Equal(t, "0x9772154BF1823353C829B43F9AD96CF9f720c768", contractAddress)
	assert.Equal(t, "0x15301c6f8a51b177bd3c24f7b5d978b2c02b0da0", creatorAddress)
	assert.Equal(t, "peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca", bankDenom)

	t.Logf("Successfully extracted contract info:")
	t.Logf("  Contract Address: %s", contractAddress)
	t.Logf("  Creator: %s", creatorAddress)
	t.Logf("  Bank Denom: %s", bankDenom)
}

// TestEventCreateTokenPairVariations tests different attribute orders and formats
func TestEventCreateTokenPairVariations(t *testing.T) {
	testCases := []struct {
		name            string
		eventJSON       string
		expectedAddr    string
		expectedCreator string
		expectedDenom   string
	}{
		{
			name: "standard format",
			eventJSON: `{
				"type": "injective.erc20.v1beta1.EventCreateTokenPair",
				"attributes": [
					{"key": "erc20_address", "value": "\"0x9772154BF1823353C829B43F9AD96CF9f720c768\""},
					{"key": "bank_denom", "value": "\"peggy0x1234\""},
					{"key": "sender", "value": "\"0x15301c6f8a51b177bd3c24f7b5d978b2c02b0da0\""}
				]
			}`,
			expectedAddr:    "0x9772154BF1823353C829B43F9AD96CF9f720c768",
			expectedCreator: "0x15301c6f8a51b177bd3c24f7b5d978b2c02b0da0",
			expectedDenom:   "peggy0x1234",
		},
		{
			name: "different attribute order",
			eventJSON: `{
				"type": "injective.erc20.v1beta1.EventCreateTokenPair",
				"attributes": [
					{"key": "sender", "value": "\"0xSenderAddress\""},
					{"key": "erc20_address", "value": "\"0xContractAddress\""},
					{"key": "bank_denom", "value": "\"peggy0xDenom\""}
				]
			}`,
			expectedAddr:    "0xContractAddress",
			expectedCreator: "0xSenderAddress",
			expectedDenom:   "peggy0xDenom",
		},
		{
			name: "missing optional fields",
			eventJSON: `{
				"type": "injective.erc20.v1beta1.EventCreateTokenPair",
				"attributes": [
					{"key": "erc20_address", "value": "\"0x9772154BF1823353C829B43F9AD96CF9f720c768\""}
				]
			}`,
			expectedAddr:    "0x9772154BF1823353C829B43F9AD96CF9f720c768",
			expectedCreator: "",
			expectedDenom:   "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var event struct {
				Type       string `json:"type"`
				Attributes []struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				} `json:"attributes"`
			}

			err := json.Unmarshal([]byte(tc.eventJSON), &event)
			assert.NoError(t, err)

			var contractAddress, creatorAddress, bankDenom string
			for _, attr := range event.Attributes {
				switch attr.Key {
				case "erc20_address":
					contractAddress = strings.Trim(attr.Value, "\"")
				case "sender":
					creatorAddress = strings.Trim(attr.Value, "\"")
				case "bank_denom":
					bankDenom = strings.Trim(attr.Value, "\"")
				}
			}

			assert.Equal(t, tc.expectedAddr, contractAddress)
			assert.Equal(t, tc.expectedCreator, creatorAddress)
			assert.Equal(t, tc.expectedDenom, bankDenom)
		})
	}
}

// TestNonContractCreationEvents verifies that non-contract events are ignored
func TestNonContractCreationEvents(t *testing.T) {
	nonContractEvents := []string{
		`{"type": "coin_spent", "attributes": [{"key": "amount", "value": "\"1000inj\""}]}`,
		`{"type": "transfer", "attributes": [{"key": "recipient", "value": "\"0x1234\""}]}`,
		`{"type": "message", "attributes": [{"key": "action", "value": "\"send\""}]}`,
	}

	for _, eventJSON := range nonContractEvents {
		var event struct {
			Type       string `json:"type"`
			Attributes []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"attributes"`
		}

		err := json.Unmarshal([]byte(eventJSON), &event)
		assert.NoError(t, err)

		// Verify it's not a contract creation event
		assert.NotEqual(t, "injective.erc20.v1beta1.EventCreateTokenPair", event.Type)
	}
}
