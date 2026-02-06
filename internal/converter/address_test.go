package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeHash(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "hash without 0x prefix",
			input: "abc123",
			want:  "0xabc123",
		},
		{
			name:  "hash with 0x prefix",
			input: "0xABC123",
			want:  "0xabc123",
		},
		{
			name:  "uppercase hash",
			input: "DEADBEEF",
			want:  "0xdeadbeef",
		},
		{
			name:  "mixed case with 0x",
			input: "0xDeAdBeEf",
			want:  "0xdeadbeef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeHash(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsValidInjectiveAddress(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want bool
	}{
		{
			name: "empty address",
			addr: "",
			want: false,
		},
		{
			name: "too short",
			addr: "inj1abc",
			want: false,
		},
		{
			name: "ethereum address",
			addr: "0x1234567890123456789012345678901234567890",
			want: false,
		},
		{
			name: "invalid prefix",
			addr: "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqnrql8a",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidInjectiveAddress(tt.addr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInjectiveToEthAddress(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "empty address",
			input:   "",
			wantErr: true,
		},
		{
			name:    "valid ethereum address",
			input:   "0x1234567890123456789012345678901234567890",
			wantErr: false,
		},
		{
			name:    "ethereum address uppercase",
			input:   "0xABCDEF1234567890123456789012345678901234",
			wantErr: false,
		},
		{
			name:    "invalid ethereum address - too short",
			input:   "0x123456",
			wantErr: true,
		},
		{
			name:    "invalid ethereum address - invalid chars",
			input:   "0x12345678901234567890123456789012345678ZZ",
			wantErr: true,
		},
		{
			name:    "invalid injective address",
			input:   "invalidaddress",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InjectiveToEthAddress(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				// Verify output is lowercase and has 0x prefix
				assert.True(t, len(got) == 42)
				assert.True(t, got[:2] == "0x")
				assert.Equal(t, got, NormalizeHash(got))
			}
		})
	}
}

func TestEthToInjectiveAddress(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "empty address",
			input:   "",
			wantErr: true,
		},
		{
			name:    "valid ethereum address with 0x",
			input:   "0x1234567890123456789012345678901234567890",
			wantErr: false,
		},
		{
			name:    "valid ethereum address without 0x",
			input:   "1234567890123456789012345678901234567890",
			wantErr: false,
		},
		{
			name:    "invalid hex - too short",
			input:   "0x12345",
			wantErr: true,
		},
		{
			name:    "invalid hex - bad characters",
			input:   "0x123456789012345678901234567890123456789Z",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EthToInjectiveAddress(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				// Verify output is a valid injective address
				assert.True(t, IsValidInjectiveAddress(got))
			}
		})
	}
}

// TestRoundTrip tests converting from Injective to Eth and back
func TestRoundTrip(t *testing.T) {
	// Start with a known Ethereum address
	ethAddr := "0x1234567890123456789012345678901234567890"

	// Convert to Injective
	injAddr, err := EthToInjectiveAddress(ethAddr)
	require.NoError(t, err)
	require.True(t, IsValidInjectiveAddress(injAddr))

	// Convert back to Ethereum
	ethAddr2, err := InjectiveToEthAddress(injAddr)
	require.NoError(t, err)

	// Should match the original (normalized)
	assert.Equal(t, NormalizeHash(ethAddr), ethAddr2)
}
