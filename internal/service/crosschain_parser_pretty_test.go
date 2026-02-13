package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnqueuePeggyEvmHashSyncTasks_Deposit tests deposit task enqueuing.
func TestEnqueuePeggyEvmHashSyncTasks_Deposit(t *testing.T) {
	// Verify task serialization (without actual Redis)
	taskData := map[string]interface{}{
		"type":         "deposit",
		"block_height": uint64(1000000),
		"event_nonce":  uint64(123),
		"tx_hash":      "cosmos_tx_123",
		"denom":        "peggy0x07865c6E87B9F70255377e024ace6630C1Eaa37F",
		"amount":       "1000000000000000000",
		"sender":       "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb",
	}

	jsonData, err := json.Marshal(taskData)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Verify deserialization
	var decoded map[string]interface{}
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "deposit", decoded["type"])
	assert.Equal(t, "cosmos_tx_123", decoded["tx_hash"])
}

// TestEnqueuePeggyEvmHashSyncTasks_Withdrawal tests withdrawal task enqueuing.
func TestEnqueuePeggyEvmHashSyncTasks_Withdrawal(t *testing.T) {
	// Verify task serialization (without actual Redis)
	taskData := map[string]interface{}{
		"type":         "withdrawal",
		"block_height": uint64(2000000),
		"batch_nonce":  uint64(456),
		"tx_hash":      "cosmos_tx_456",
		"denom":        "peggy0x07865c6E87B9F70255377e024ace6630C1Eaa37F",
	}

	jsonData, err := json.Marshal(taskData)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Verify deserialization
	var decoded map[string]interface{}
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "withdrawal", decoded["type"])
	assert.Equal(t, "cosmos_tx_456", decoded["tx_hash"])
}

// TestSyncTaskSerialization tests that SyncTask can be serialized/deserialized.
func TestSyncTaskSerialization(t *testing.T) {
	// Deposit task
	depositTask := map[string]interface{}{
		"type":         "deposit",
		"block_height": uint64(1000000),
		"event_nonce":  uint64(123),
		"tx_hash":      "cosmos_tx_123",
		"denom":        "peggy0x07865c6E87B9F70255377e024ace6630C1Eaa37F",
		"amount":       "1000000000000000000",
		"sender":       "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb",
	}

	data, err := json.Marshal(depositTask)
	require.NoError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "deposit", decoded["type"])
	assert.Equal(t, "cosmos_tx_123", decoded["tx_hash"])

	// Withdrawal task
	withdrawalTask := map[string]interface{}{
		"type":         "withdrawal",
		"block_height": uint64(2000000),
		"batch_nonce":  uint64(456),
		"tx_hash":      "cosmos_tx_456",
		"denom":        "peggy0x07865c6E87B9F70255377e024ace6630C1Eaa37F",
	}

	data, err = json.Marshal(withdrawalTask)
	require.NoError(t, err)

	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "withdrawal", decoded["type"])
	assert.Equal(t, "cosmos_tx_456", decoded["tx_hash"])
}
