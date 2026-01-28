package service

import (
	"math/big"
	"testing"
	"time"

	"injective-explorer/internal/models"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
)

// TestParseTransferLogs tests the parsing of EVM Transfer event logs
func TestParseTransferLogs(t *testing.T) {
	service := &TokenTransferSyncService{}

	tests := []struct {
		name     string
		logs     []ethtypes.Log
		expected int
	}{
		{
			name:     "Empty logs",
			logs:     []ethtypes.Log{},
			expected: 0,
		},
		{
			name: "Valid Transfer event",
			logs: []ethtypes.Log{
				{
					Address: ethcommon.HexToAddress("0x1234567890123456789012345678901234567890"),
					Topics: []ethcommon.Hash{
						ethcommon.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"), // Transfer event signature
						ethcommon.HexToHash("0x0000000000000000000000001111111111111111111111111111111111111111"), // from
						ethcommon.HexToHash("0x0000000000000000000000002222222222222222222222222222222222222222"), // to
					},
					Data:        append(make([]byte, 32-8), []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x64}...), // 100
					BlockNumber: 12345,
					TxHash:      ethcommon.HexToHash("0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"),
					Index:       0,
				},
			},
			expected: 1,
		},
		{
			name: "Mint event (from zero address)",
			logs: []ethtypes.Log{
				{
					Address: ethcommon.HexToAddress("0x1234567890123456789012345678901234567890"),
					Topics: []ethcommon.Hash{
						ethcommon.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"),
						ethcommon.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), // from zero address
						ethcommon.HexToHash("0x0000000000000000000000002222222222222222222222222222222222222222"), // to
					},
					Data:        append(make([]byte, 32-8), []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0xE8}...), // 1000
					BlockNumber: 12346,
					TxHash:      ethcommon.HexToHash("0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567891"),
					Index:       1,
				},
			},
			expected: 1,
		},
		{
			name: "Burn event (to zero address)",
			logs: []ethtypes.Log{
				{
					Address: ethcommon.HexToAddress("0x1234567890123456789012345678901234567890"),
					Topics: []ethcommon.Hash{
						ethcommon.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"),
						ethcommon.HexToHash("0x0000000000000000000000001111111111111111111111111111111111111111"), // from
						ethcommon.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"), // to zero address
					},
					Data:        append(make([]byte, 32-8), []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x27, 0x10}...), // 10000
					BlockNumber: 12347,
					TxHash:      ethcommon.HexToHash("0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567892"),
					Index:       2,
				},
			},
			expected: 1,
		},
		{
			name: "Log with insufficient topics",
			logs: []ethtypes.Log{
				{
					Address: ethcommon.HexToAddress("0x1234567890123456789012345678901234567890"),
					Topics: []ethcommon.Hash{
						ethcommon.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"),
						// Missing from and to topics
					},
					BlockNumber: 12348,
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.parseTransferLogs(tt.logs)
			assert.Len(t, result, tt.expected)

			if tt.expected > 0 && len(tt.logs) > 0 {
				// Verify the parsed transfer
				transfer := result[0]
				assert.NotEmpty(t, transfer.ContractAddress)
				assert.NotEmpty(t, transfer.FromAddress)
				assert.NotEmpty(t, transfer.ToAddress)
				assert.NotEmpty(t, transfer.Amount)
				assert.Greater(t, transfer.BlockNumber, uint64(0))

				// Check method type
				if transfer.FromAddress == "0x0000000000000000000000000000000000000000" {
					assert.Equal(t, "Mint", transfer.Method)
				} else if transfer.ToAddress == "0x0000000000000000000000000000000000000000" {
					assert.Equal(t, "Burn", transfer.Method)
				} else {
					assert.Equal(t, "Transfer", transfer.Method)
				}
			}
		})
	}
}

// TestFormatTokenAmount tests the token amount formatting
func TestFormatTokenAmount(t *testing.T) {
	tests := []struct {
		name     string
		amount   string
		decimals int
		want     string
	}{
		{
			name:     "No decimals",
			amount:   "1000000000000000000",
			decimals: 0,
			want:     "1000000000000000000",
		},
		{
			name:     "18 decimals - whole number",
			amount:   "1000000000000000000",
			decimals: 18,
			want:     "1",
		},
		{
			name:     "18 decimals - with remainder",
			amount:   "1500000000000000000",
			decimals: 18,
			want:     "1.5",
		},
		{
			name:     "18 decimals - complex",
			amount:   "1234567890123456789",
			decimals: 18,
			want:     "1.234567890123456789",
		},
		{
			name:     "6 decimals",
			amount:   "1000000",
			decimals: 6,
			want:     "1",
		},
		{
			name:     "6 decimals - with remainder",
			amount:   "1500000",
			decimals: 6,
			want:     "1.5",
		},
		{
			name:     "Invalid amount",
			amount:   "invalid",
			decimals: 18,
			want:     "invalid",
		},
		{
			name:     "Zero amount",
			amount:   "0",
			decimals: 18,
			want:     "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTokenAmount(tt.amount, tt.decimals)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestTokenTransferConfig tests the configuration loading
func TestTokenTransferConfig(t *testing.T) {
	t.Run("Default config", func(t *testing.T) {
		config := DefaultTokenTransferSyncConfig()
		assert.NotNil(t, config)
		assert.Equal(t, 2, config.WorkerCount)
		assert.Equal(t, 100, config.BatchSize)
		assert.Equal(t, 30*time.Second, config.ProcessInterval)
		assert.Equal(t, 60*time.Second, config.Timeout)
		assert.Equal(t, uint64(0), config.StartBlock)
		assert.True(t, config.EnableRealtimeSync)
	})

	t.Run("Load config from nil bootstrap", func(t *testing.T) {
		config := LoadTokenTransferSyncConfig(nil)
		assert.NotNil(t, config)
		// Should return default config
		assert.Equal(t, 2, config.WorkerCount)
	})
}

// BenchmarkParseTransferLogs benchmarks the log parsing performance
func BenchmarkParseTransferLogs(b *testing.B) {
	service := &TokenTransferSyncService{}

	// Create a batch of logs
	logs := make([]ethtypes.Log, 100)
	for i := range logs {
		logs[i] = ethtypes.Log{
			Address: ethcommon.HexToAddress("0x1234567890123456789012345678901234567890"),
			Topics: []ethcommon.Hash{
				ethcommon.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"),
				ethcommon.HexToHash("0x0000000000000000000000001111111111111111111111111111111111111111"),
				ethcommon.HexToHash("0x0000000000000000000000002222222222222222222222222222222222222222"),
			},
			Data:        append(make([]byte, 32-8), big.NewInt(int64(i+1)).Bytes()...),
			BlockNumber: uint64(1000 + i),
			TxHash:      ethcommon.HexToHash("0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"),
			Index:       uint(i),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = service.parseTransferLogs(logs)
	}
}

// TestTokenTransferMethodDetection tests the correct detection of transfer methods
func TestTokenTransferMethodDetection(t *testing.T) {
	service := &TokenTransferSyncService{}

	tests := []struct {
		name           string
		from           string
		to             string
		expectedMethod string
	}{
		{
			name:           "Regular transfer",
			from:           "0x1111111111111111111111111111111111111111",
			to:             "0x2222222222222222222222222222222222222222",
			expectedMethod: "Transfer",
		},
		{
			name:           "Mint from zero address",
			from:           "0x0000000000000000000000000000000000000000",
			to:             "0x2222222222222222222222222222222222222222",
			expectedMethod: "Mint",
		},
		{
			name:           "Burn to zero address",
			from:           "0x1111111111111111111111111111111111111111",
			to:             "0x0000000000000000000000000000000000000000",
			expectedMethod: "Burn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := ethtypes.Log{
				Address: ethcommon.HexToAddress("0x1234567890123456789012345678901234567890"),
				Topics: []ethcommon.Hash{
					ethcommon.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"),
					ethcommon.HexToHash(tt.from),
					ethcommon.HexToHash(tt.to),
				},
				Data:        append(make([]byte, 32-8), []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x64}...),
				BlockNumber: 12345,
				Index:       0,
			}

			result := service.parseTransferLogs([]ethtypes.Log{log})
			assert.Len(t, result, 1)
			assert.Equal(t, tt.expectedMethod, result[0].Method)
		})
	}
}

// TestTokenModelHelpers tests the helper methods on TokenTransfer model
func TestTokenModelHelpers(t *testing.T) {
	t.Run("IsMint", func(t *testing.T) {
		mint := &models.TokenTransfer{
			FromAddress: "0x0000000000000000000000000000000000000000",
			ToAddress:   "0x1111111111111111111111111111111111111111",
		}
		assert.True(t, mint.IsMint())

		transfer := &models.TokenTransfer{
			FromAddress: "0x2222222222222222222222222222222222222222",
			ToAddress:   "0x1111111111111111111111111111111111111111",
		}
		assert.False(t, transfer.IsMint())
	})

	t.Run("IsBurn", func(t *testing.T) {
		burn := &models.TokenTransfer{
			FromAddress: "0x1111111111111111111111111111111111111111",
			ToAddress:   "0x0000000000000000000000000000000000000000",
		}
		assert.True(t, burn.IsBurn())

		transfer := &models.TokenTransfer{
			FromAddress: "0x2222222222222222222222222222222222222222",
			ToAddress:   "0x1111111111111111111111111111111111111111",
		}
		assert.False(t, transfer.IsBurn())
	})

	t.Run("GetMethod", func(t *testing.T) {
		transfer := &models.TokenTransfer{
			Method: "Transfer",
		}
		assert.Equal(t, "Transfer", transfer.GetMethod())
	})
}
