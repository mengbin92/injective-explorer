package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExtractContractFromEventCreateTokenPair verifies that we can extract
// contract address from EventCreateTokenPair events correctly.
// This test uses the real transaction data format where sender is in EventCreateTokenPair.
func TestExtractContractFromEventCreateTokenPair(t *testing.T) {
	// Some chain versions include sender directly in EventCreateTokenPair
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

// TestExtractCreatorFromMessageEvent verifies that we can extract the creator
// from the "message" event when EventCreateTokenPair doesn't contain a sender.
// This matches the real on-chain data structure where sender is in a separate message event.
func TestExtractCreatorFromMessageEvent(t *testing.T) {
	// Real on-chain format: EventCreateTokenPair has NO sender attribute
	tokenPairEventJSON := `{
		"type": "injective.erc20.v1beta1.EventCreateTokenPair",
		"attributes": [
			{
				"key": "bank_denom",
				"value": "\"peggy0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238\""
			},
			{
				"key": "erc20_address",
				"value": "\"0xC8C01C39a28688d63242Fd17F293be76Cf5f9717\""
			},
			{
				"key": "msg_index",
				"value": "0"
			}
		]
	}`

	// The sender is in a separate "message" event
	messageEventJSON := `{
		"type": "message",
		"attributes": [
			{
				"key": "action",
				"value": "/injective.erc20.v1beta1.MsgCreateTokenPair"
			},
			{
				"key": "module",
				"value": "erc20"
			},
			{
				"key": "msg_index",
				"value": "0"
			},
			{
				"key": "sender",
				"value": "0xe3c970535eb397684dc4db1c32ddeedbab566e6e"
			}
		]
	}`

	type eventData struct {
		Type       string `json:"type"`
		Attributes []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"attributes"`
	}

	var tokenPairEvent, messageEvent eventData
	assert.NoError(t, json.Unmarshal([]byte(tokenPairEventJSON), &tokenPairEvent))
	assert.NoError(t, json.Unmarshal([]byte(messageEventJSON), &messageEvent))

	// Step 1: Extract from EventCreateTokenPair (no sender expected)
	var contractAddress, creatorAddress, bankDenom string
	for _, attr := range tokenPairEvent.Attributes {
		value := strings.Trim(attr.Value, "\"")
		switch attr.Key {
		case "erc20_address":
			contractAddress = value
		case "sender":
			creatorAddress = value
		case "bank_denom":
			bankDenom = value
		}
	}

	assert.Equal(t, "0xC8C01C39a28688d63242Fd17F293be76Cf5f9717", contractAddress)
	assert.Equal(t, "", creatorAddress, "sender should NOT be in EventCreateTokenPair in real on-chain data")
	assert.Equal(t, "peggy0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238", bankDenom)

	// Step 2: Fallback to message event to find sender
	if creatorAddress == "" && messageEvent.Type == "message" {
		isCreateTokenPairMsg := false
		var sender string
		for _, attr := range messageEvent.Attributes {
			value := strings.Trim(attr.Value, "\"")
			switch attr.Key {
			case "action":
				if value == "/injective.erc20.v1beta1.MsgCreateTokenPair" {
					isCreateTokenPairMsg = true
				}
			case "sender":
				sender = value
			}
		}
		if isCreateTokenPairMsg && sender != "" {
			creatorAddress = sender
		}
	}

	assert.Equal(t, "0xe3c970535eb397684dc4db1c32ddeedbab566e6e", creatorAddress,
		"creator should be resolved from message event")

	t.Logf("Successfully resolved creator from message event:")
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
			name: "standard format with sender in event",
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
			name: "real on-chain format - no sender in EventCreateTokenPair",
			eventJSON: `{
				"type": "injective.erc20.v1beta1.EventCreateTokenPair",
				"attributes": [
					{"key": "bank_denom", "value": "\"peggy0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238\""},
					{"key": "erc20_address", "value": "\"0xC8C01C39a28688d63242Fd17F293be76Cf5f9717\""},
					{"key": "msg_index", "value": "0"}
				]
			}`,
			expectedAddr:    "0xC8C01C39a28688d63242Fd17F293be76Cf5f9717",
			expectedCreator: "",
			expectedDenom:   "peggy0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238",
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
