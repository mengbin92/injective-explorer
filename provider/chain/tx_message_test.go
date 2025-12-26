package chain

import (
	"context"
	"fmt"
	"testing"

	"github.com/InjectiveLabs/sdk-go/client/common"
	explorerclient "github.com/InjectiveLabs/sdk-go/client/explorer"
	"github.com/bytedance/sonic"
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

func TestGetTxMessageTypes(t *testing.T) {
	network := common.LoadNetwork("mainnet", "lb")
	explorerClient, err := explorerclient.NewExplorerClient(network)
	assert.NoError(t, err, "failed to create explorer client")

	tx, err := explorerClient.GetTxByTxHash(context.Background(), "981DCED9D977E9A3AB7727637CC005DEF643C73B4C4676A30CD671D56C931C19")
	assert.NoError(t, err, "failed to get transaction by hash")

	txMessages := tx.Data.GetMessages()

	// Check if Messages is empty
	if len(txMessages) == 0 {
		t.Logf("Messages is empty for transaction")
		return
	}

	t.Logf("Messages length: %d bytes", len(txMessages))

	// Try parsing as TxRaw first (most common format)
	// TxRaw contains BodyBytes which is the protobuf-encoded TxBody
	var txRaw cosmostxpb.TxRaw
	if err := txRaw.Unmarshal(txMessages); err == nil && len(txRaw.BodyBytes) > 0 {
		t.Logf("Successfully parsed as TxRaw, BodyBytes length: %d", len(txRaw.BodyBytes))

		// Parse BodyBytes as TxBody
		var txBody cosmostxpb.TxBody
		if err := txBody.Unmarshal(txRaw.BodyBytes); err == nil {
			t.Logf("Successfully parsed TxBody with %d messages", len(txBody.Messages))

			messageTypes := make([]string, 0, len(txBody.Messages))
			for i, msgAny := range txBody.Messages {
				if msgAny != nil && msgAny.TypeUrl != "" {
					messageTypes = append(messageTypes, msgAny.TypeUrl)
					t.Logf("Message[%d] TypeUrl: %s", i, msgAny.TypeUrl)
				} else if msgAny != nil {
					t.Logf("Message[%d] has empty TypeUrl", i)
				}
			}

			if len(messageTypes) > 0 {
				t.Logf("Extracted %d message types: %v", len(messageTypes), messageTypes)
				fmt.Println("Message Types:", messageTypes)
			} else {
				t.Logf("No message types extracted from %d messages", len(txBody.Messages))
			}
			return
		} else {
			t.Logf("Failed to parse BodyBytes as TxBody: %v", err)
		}
	} else {
		t.Logf("Failed to parse as TxRaw: %v", err)
	}

	// Fallback: try parsing as TxBody directly
	txBody := &cosmostxpb.TxBody{}
	if err := txBody.Unmarshal(txMessages); err == nil {
		t.Logf("Successfully parsed as TxBody directly with %d messages", len(txBody.Messages))

		messageTypes := make([]string, 0, len(txBody.Messages))
		for i, msgAny := range txBody.Messages {
			if msgAny != nil && msgAny.TypeUrl != "" {
				messageTypes = append(messageTypes, msgAny.TypeUrl)
				t.Logf("Message[%d] TypeUrl: %s", i, msgAny.TypeUrl)
			}
		}

		if len(messageTypes) > 0 {
			t.Logf("Extracted %d message types: %v", len(messageTypes), messageTypes)
			fmt.Println("Message Types:", messageTypes)
		}
		return
	}

	// Both parsing attempts failed, try JSON format
	// Check if it looks like JSON (starts with '[' or '{')
	if len(txMessages) > 0 && (txMessages[0] == '[' || txMessages[0] == '{') {
		t.Logf("Messages appears to be JSON format, attempting to parse...")

		var jsonMessages []map[string]interface{}
		if err := sonic.Unmarshal(txMessages, &jsonMessages); err == nil {
			t.Logf("Successfully parsed as JSON array with %d messages", len(jsonMessages))

			messageTypes := make([]string, 0, len(jsonMessages))
			for i, msg := range jsonMessages {
				if msgType, ok := msg["type"].(string); ok && msgType != "" {
					messageTypes = append(messageTypes, msgType)
					t.Logf("Message[%d] TypeUrl: %s", i, msgType)
				}
			}

			if len(messageTypes) > 0 {
				t.Logf("Extracted %d message types from JSON: %v", len(messageTypes), messageTypes)
				fmt.Println("Message Types (from JSON):", messageTypes)
			} else {
				t.Logf("No message types extracted from JSON messages")
			}
			return
		} else {
			t.Logf("Failed to parse as JSON array: %v", err)
		}
	}

	// All parsing attempts failed
	t.Logf("WARN: failed to parse Messages as TxRaw, TxBody, or JSON")
	t.Logf("Transaction TxType: %s", tx.Data.GetTxType())
	t.Logf("First 50 bytes: %s", string(txMessages[:min(50, len(txMessages))]))
}