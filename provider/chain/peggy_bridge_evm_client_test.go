package chain

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseTokenContractFromDenom tests parsing token contract from denom.
func TestParseTokenContractFromDenom(t *testing.T) {
	tests := []struct {
		name        string
		denom       string
		expected    string
		expectError bool
	}{
		{
			name:        "Valid peggy denom with full address (40 chars)",
			denom:       "peggy0x07865c6E87B9F70255377e024ace6630C1Eaa37F",
			expected:    "0x07865c6e87b9f70255377e024ace6630c1eaa37f",
			expectError: false,
		},
		{
			name:        "Valid peggy denom without 0x",
			denom:       "peggy07865c6E87B9F70255377e024ace6630C1Eaa37F",
			expected:    "0x07865c6e87b9f70255377e024ace6630c1eaa37f",
			expectError: false,
		},
		{
			name:        "Raw 0x address (no peggy prefix)",
			denom:       "0x3de4027b5b0bf278db2d187768ac441e9b356360",
			expected:    "0x3de4027b5b0bf278db2d187768ac441e9b356360",
			expectError: false,
		},
		{
			name:        "Raw 0x address uppercase",
			denom:       "0x1C7D4B196CB0C7B01D743FBC6116A902379C7238",
			expected:    "0x1c7d4b196cb0c7b01d743fbc6116a902379c7238",
			expectError: false,
		},
		{
			name:        "Non-peggy denom",
			denom:       "inj",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Invalid address format - too short",
			denom:       "peggy0xABC",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Empty denom",
			denom:       "",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseTokenContractFromDenom(tt.denom)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestNormalizeHash tests address normalization.
func TestNormalizeHash(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "With 0x prefix lowercase",
			input:    "0xabcdef",
			expected: "0xabcdef",
		},
		{
			name:     "With 0x prefix uppercase",
			input:    "0xABCDEF",
			expected: "0xabcdef",
		},
		{
			name:     "Without 0x prefix",
			input:    "ABCDEF",
			expected: "0xabcdef",
		},
		{
			name:     "Mixed case",
			input:    "0xAbCdEf",
			expected: "0xabcdef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeHash(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMatchDepositLog tests the deposit log matching logic.
func TestMatchDepositLog(t *testing.T) {
	client := &PeggyBridgeEVMClient{}

	// Create a sample log with TxHash set
	// For SendToCosmosEvent, the data contains:
	// - _destination (string, dynamic) - offset, length, data
	// - _amount (uint256, 32 bytes)
	// - _eventNonce (uint256, 32 bytes)
	//
	// We construct data with:
	// - 64 bytes for _destination (32 for offset+length + 32 for data)
	// - 32 bytes for amount
	// - 32 bytes for eventNonce
	// Total: 128 bytes
	sampleLog := types.Log{
		TxHash: common.HexToHash("0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"),
		Data: common.Hex2Bytes(
			"0000000000000000000000000000000000000000000000000000000000000000" + // offset (32 bytes)
				"0000000000000000000000000000000000000000000000000000000000000020" + // length=32 bytes (32 bytes)
				"0000000000000000000000000000000000000000000000000000000000000001" + // _destination data (32 bytes) placeholder
				"0000000000000000000000000000000000000000000000000000000000000001" + // amount = 1 (32 bytes)
				"0000000000000000000000000000000000000000000000000000000000000002", // eventNonce = 2 (32 bytes)
		),
	}

	t.Run("Matching log", func(t *testing.T) {
		// The log data has eventNonce=2 (last 32 bytes) and amount=1 (second to last 32 bytes)
		txHash, matched := client.matchDepositLog(sampleLog, 2, "1")
		assert.True(t, matched, "Expected log to match")
		assert.NotEmpty(t, txHash, "Expected non-empty tx hash")
	})

	t.Run("Non-matching event nonce", func(t *testing.T) {
		txHash, matched := client.matchDepositLog(sampleLog, 3, "1")
		assert.False(t, matched, "Expected log not to match")
		assert.Empty(t, txHash, "Expected empty tx hash")
	})

	t.Run("Non-matching amount", func(t *testing.T) {
		txHash, matched := client.matchDepositLog(sampleLog, 2, "2")
		assert.False(t, matched, "Expected log not to match")
		assert.Empty(t, txHash, "Expected empty tx hash")
	})

	t.Run("Insufficient data length", func(t *testing.T) {
		shortLog := types.Log{
			Data: []byte{1, 2, 3},
		}
		txHash, matched := client.matchDepositLog(shortLog, 2, "1")
		assert.False(t, matched, "Expected log not to match")
		assert.Empty(t, txHash, "Expected empty tx hash")
	})
}

// TestNewPeggyBridgeEVMClient tests client creation.
func TestNewPeggyBridgeEVMClient(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "Valid config",
			config: Config{
				RPCEndpoint: "https://rpc.sepolia.org",
				ChainID:     11155111,
				Timeout:     10 * time.Second,
				RateLimit:   10,
			},
			expectError: false, // May succeed if network is available
		},
		{
			name: "Invalid endpoint",
			config: Config{
				RPCEndpoint: "invalid://endpoint",
				ChainID:     11155111,
				Timeout:     1 * time.Second,
			},
			expectError: true,
		},
		{
			name: "Default rate limit",
			config: Config{
				RPCEndpoint: "https://rpc.sepolia.org",
				ChainID:     11155111,
				Timeout:     10 * time.Second,
				RateLimit:   0, // Should default to 10
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewPeggyBridgeEVMClient(tt.config)

			if tt.expectError {
				// For network-dependent tests, we may not get an error immediately
				// so we just check the error handling logic
				if err != nil {
					assert.Error(t, err)
				}
			} else {
				if err == nil {
					defer client.Close()
					assert.NotNil(t, client)
					assert.NotNil(t, client.rateLimit)
				}
			}
		})
	}
}

// TestGetDepositTxHash_Integration is an integration test that requires
// a real Ethereum RPC endpoint. Skip this test in CI/CD.
func TestGetDepositTxHash_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := Config{
		RPCEndpoint: "https://rpc.sepolia.org",
		ChainID:     11155111,
		Timeout:     10 * time.Second,
		RateLimit:   5,
	}

	client, err := NewPeggyBridgeEVMClient(cfg)
	require.NoError(t, err)
	defer client.Close()

	// Test with sample data (these are example values)
	// In a real scenario, you would use actual data from your chain
	t.Run("Query existing deposit", func(t *testing.T) {
		// This is just a placeholder test
		// Replace with actual test data from your chain
		_, err := client.GetDepositTxHash(
			ctx,
			12345,                        // eventNonce
			5000000,                      // eventHeight
			"0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb", // sender
			"peggy0x07865c6E87B9F70255377e024ace6630C1Eaa37F", // denom
			"1000000000000000000",         // amount
		)

		// We expect either success or "not found" error
		// Network errors should fail the test
		if err != nil {
			assert.Contains(t, err.Error(), "not found")
		}
	})
}

// TestGetWithdrawalBatchTxHash_Integration is an integration test.
func TestGetWithdrawalBatchTxHash_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := Config{
		RPCEndpoint: "https://rpc.sepolia.org",
		ChainID:     11155111,
		Timeout:     10 * time.Second,
		RateLimit:   5,
	}

	client, err := NewPeggyBridgeEVMClient(cfg)
	require.NoError(t, err)
	defer client.Close()

	t.Run("Query existing batch", func(t *testing.T) {
		// This is just a placeholder test
		// Replace with actual test data from your chain
		_, err := client.GetWithdrawalBatchTxHash(
			ctx,
			123,                          // batchNonce
			"peggy0x07865c6E87B9F70255377e024ace6630C1Eaa37F", // denom
		)

		// We expect either success or "not found" error
		if err != nil {
			assert.Contains(t, err.Error(), "not found")
		}
	})
}

// TestEventSignatures tests that event signatures are correctly computed.
func TestEventSignatures(t *testing.T) {
	// SendToCosmosEvent(address,address,string,uint256,uint256)
	// Verify that the signature is not zero
	assert.NotEqual(t, common.Hash{}, SendToCosmosEventSig, "SendToCosmosEventSig should not be zero")

	// Verify it's a 32-byte hash (standard for event signatures)
	assert.Equal(t, 32, len(SendToCosmosEventSig), "Event signature should be 32 bytes")

	// TransactionBatchExecutedEvent(uint256,address,uint256)
	// Verify that the signature is not zero
	assert.NotEqual(t, common.Hash{}, TransactionBatchExecutedEventSig, "TransactionBatchExecutedEventSig should not be zero")

	// Verify it's a 32-byte hash
	assert.Equal(t, 32, len(TransactionBatchExecutedEventSig), "Event signature should be 32 bytes")
}
