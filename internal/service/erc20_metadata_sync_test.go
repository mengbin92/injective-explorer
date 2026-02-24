// Package service provides unit tests for ERC20 metadata sync logic.
package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateERC20Metadata(t *testing.T) {
	tests := []struct {
		name      string
		nameIn    string
		symbolIn  string
		decimals  int
		wantErr   bool
		wantSym   string
	}{
		{"valid", "Tether USD", "USDT", 6, false, "USDT"},
		{"valid lowercase symbol", "USD Coin", "usdc", 6, false, "USDC"},
		{"empty name", "", "EXT", 18, true, ""},
		{"empty symbol", "Example Token", "", 18, true, ""},
		{"null char in name", "Bad\x00Name", "BN", 18, true, ""},
		{"null char in symbol", "Bad", "B\x00N", 18, true, ""},
		{"long name truncated", strings.Repeat("a", 300), "X", 18, false, "X"},
		{"unusual decimals", "Token", "TKN", 24, false, "TKN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotSym, err := validateERC20Metadata(tt.nameIn, tt.symbolIn, tt.decimals)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantSym, gotSym)
			if tt.nameIn != "" && len(tt.nameIn) <= 255 {
				assert.Equal(t, tt.nameIn, gotName)
			}
		})
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantType    string
		wantRetry   bool
	}{
		{"nil", nil, "", false},
		{"timeout", errors.New("context deadline exceeded"), "rpc_timeout", true},
		{"execution reverted", errors.New("execution reverted"), "contract_not_erc20", false},
		{"no data", errors.New("no data"), "contract_not_erc20", false},
		{"connection refused", errors.New("connection refused"), "rpc_connection_failed", true},
		{"dial tcp", errors.New("dial tcp failed"), "rpc_connection_failed", true},
		{"rate limit", errors.New("429 too many requests"), "rate_limit", true},
		{"unknown", errors.New("something else"), "unknown", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyError(tt.err)
			if tt.err == nil {
				assert.Nil(t, got)
				return
			}
			assert.NotNil(t, got)
			assert.Equal(t, tt.wantType, got.Type)
			assert.Equal(t, tt.wantRetry, got.ShouldRetry)
		})
	}
}

func TestExtractSymbolFromDenom(t *testing.T) {
	tests := []struct {
		denom   string
		want    string
	}{
		{"peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca", "PEGGY"},
		{"peggyUSDT", "USDT"},
		{"peggy", "PEGGY"},
		{"peggy0x", "PEGGY"},
	}
	for _, tt := range tests {
		t.Run(tt.denom, func(t *testing.T) {
			got := extractSymbolFromDenom(tt.denom)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetPeggyTokenDefaults(t *testing.T) {
	tests := []struct {
		denom      string
		wantName   string
		wantSymbol string
		wantDec    int
	}{
		{"peggy0x87aB3B4C8661e07D6372361211B96ed4Dc36B1B5", "Bridge Token", "PEGGY", 18},
		{"peggy0x...usdt...", "Tether USD", "USDT", 6},
		{"peggy0x...USDT...", "Tether USD", "USDT", 6},
		{"peggy0x...usdc...", "USD Coin", "USDC", 6},
	}
	for _, tt := range tests {
		t.Run(tt.denom, func(t *testing.T) {
			name, sym, dec := getPeggyTokenDefaults(tt.denom)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantSymbol, sym)
			assert.Equal(t, tt.wantDec, dec)
		})
	}
}
