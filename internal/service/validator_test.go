// Package service provides unit tests for validation utilities.
package service

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func init() {
	// Initialize SDK config for tests (if not already initialized by another package)
	defer func() {
		// Recover if config is already sealed
		_ = recover()
	}()
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount("inj", "injpub")
	config.SetBech32PrefixForValidator("injvaloper", "injvaloperpub")
	config.SetBech32PrefixForConsensusNode("injvalcons", "injvalconspub")
	config.Seal()
}

// TestValidateAddress tests the ValidateAddress function with various inputs.
func TestValidateAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{
			name:    "valid injective chain address",
			address: "inj1test1234567890123456789012345678901234",
			wantErr: false,
		},
		{
			name:    "valid validator address",
			address: "injvaloper1wcezdkvet2klmgg3jygc3qq5s52n4zwkh204f4",
			wantErr: false,
		},
		{
			name:    "valid Ethereum address",
			address: "0x1234567890123456789012345678901234567890",
			wantErr: false,
		},
		{
			name:    "empty address",
			address: "",
			wantErr: true,
		},
		{
			name:    "invalid Injective address (too short)",
			address: "inj1test",
			wantErr: true,
		},
		{
			name:    "invalid Ethereum address (wrong prefix)",
			address: "0X1234567890123456789012345678901234567890",
			wantErr: true,
		},
		{
			name:    "invalid validator address (too short)",
			address: "injvaloper1test",
			wantErr: true,
		},
		{
			name:    "unknown address format",
			address: "unknown_format",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAddress(tt.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAddress() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateTransactionHash tests the ValidateTransactionHash function.
func TestValidateTransactionHash(t *testing.T) {
	tests := []struct {
		name    string
		hash    string
		wantErr bool
	}{
		{
			name:    "valid transaction hash",
			hash:    "1234567890123456789012345678901234567890123456789012345678901234",
			wantErr: false,
		},
		{
			name:    "empty hash",
			hash:    "",
			wantErr: true,
		},
		{
			name:    "invalid hash (too short)",
			hash:    "1234567890",
			wantErr: true,
		},
		{
			name:    "invalid hash (invalid characters)",
			hash:    "123456789012345678901234567890123456789012345678901234567890123g",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransactionHash(tt.hash)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransactionHash() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateBlockHeight tests the ValidateBlockHeight function.
func TestValidateBlockHeight(t *testing.T) {
	tests := []struct {
		name    string
		height  interface{}
		wantErr bool
	}{
		{
			name:    "valid height (int64)",
			height:  int64(1000),
			wantErr: false,
		},
		{
			name:    "valid height (uint64)",
			height:  uint64(1000),
			wantErr: false,
		},
		{
			name:    "zero height",
			height:  uint64(0),
			wantErr: true,
		},
		{
			name:    "negative height",
			height:  int64(-1),
			wantErr: true,
		},
		{
			name:    "invalid type",
			height:  "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBlockHeight(tt.height)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBlockHeight() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateLimit tests the ValidateLimit function.
func TestValidateLimit(t *testing.T) {
	tests := []struct {
		name     string
		limit    interface{}
		maxLimit uint32
		wantErr  bool
	}{
		{
			name:     "valid limit (int32)",
			limit:    int32(100),
			maxLimit: 1000,
			wantErr:  false,
		},
		{
			name:     "valid limit (uint32)",
			limit:    uint32(100),
			maxLimit: 1000,
			wantErr:  false,
		},
		{
			name:     "zero limit",
			limit:    uint32(0),
			maxLimit: 1000,
			wantErr:  true,
		},
		{
			name:     "limit exceeds max",
			limit:    uint32(2000),
			maxLimit: 1000,
			wantErr:  true,
		},
		{
			name:     "negative limit",
			limit:    int32(-1),
			maxLimit: 1000,
			wantErr:  true,
		},
		{
			name:     "invalid type",
			limit:    "invalid",
			maxLimit: 1000,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLimit(tt.limit, tt.maxLimit)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLimit() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestSanitizeAddress tests the SanitizeAddress function.
func TestSanitizeAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    string
	}{
		{
			name:    "address with whitespace",
			address: "  inj1test1234567890123456789012345678901234  ",
			want:    "inj1test1234567890123456789012345678901234",
		},
		{
			name:    "uppercase address",
			address: "INJ1TEST1234567890123456789012345678901234",
			want:    "inj1test1234567890123456789012345678901234",
		},
		{
			name:    "mixed case address",
			address: "Inj1Test1234567890123456789012345678901234",
			want:    "inj1test1234567890123456789012345678901234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeAddress(tt.address)
			if got != tt.want {
				t.Errorf("SanitizeAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSanitizeHash tests the SanitizeHash function.
func TestSanitizeHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
		want string
	}{
		{
			name: "hash with whitespace",
			hash: "  1234567890123456789012345678901234567890123456789012345678901234  ",
			want: "1234567890123456789012345678901234567890123456789012345678901234",
		},
		{
			name: "uppercase hash",
			hash: "ABCDEF123456789012345678901234567890123456789012345678901234567890",
			want: "abcdef123456789012345678901234567890123456789012345678901234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeHash(tt.hash)
			if got != tt.want {
				t.Errorf("SanitizeHash() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNormalizeHashForCache tests the NormalizeHashForCache function.
func TestNormalizeHashForCache(t *testing.T) {
	tests := []struct {
		name string
		hash string
		want string
	}{
		{
			name: "hash with 0x prefix",
			hash: "0xabc123",
			want: "0xabc123",
		},
		{
			name: "hash without 0x",
			hash: "abc123",
			want: "0xabc123",
		},
		{
			name: "double 0x prefix",
			hash: "0x0xabc123",
			want: "0xabc123",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeHashForCache(tt.hash)
			if got != tt.want {
				t.Errorf("NormalizeHashForCache() = %v, want %v", got, tt.want)
			}
		})
	}
}

