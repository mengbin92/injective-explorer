package models

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil/bech32"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
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

func TestInjectiveToEthAddress(t *testing.T) {
	tests := []struct {
		name      string
		injAddr   string
		wantEth   string
		wantError bool
	}{
		{
			name:      "valid Injective address",
			injAddr:   "inj1w9tzkuvenpeakkegdhu40tcenmy5v9lh2yzahv",
			wantEth:   "0x71562b71999873db5b286df957af199ec94617f7",
			wantError: false,
		},
		{
			name:      "already Ethereum address",
			injAddr:   "0x71562b71999873db5b286df957af199ec94617f7",
			wantEth:   "0x71562b71999873db5b286df957af199ec94617f7",
			wantError: false,
		},
		{
			name:      "invalid address format",
			injAddr:   "invalid",
			wantEth:   "",
			wantError: true,
		},
		{
			name:      "empty address",
			injAddr:   "",
			wantEth:   "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InjectiveToEthAddress(tt.injAddr)
			if (err != nil) != tt.wantError {
				t.Errorf("InjectiveToEthAddress() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if got != tt.wantEth {
				t.Errorf("InjectiveToEthAddress() = %v, want %v", got, tt.wantEth)
			}
		})
	}
}

func TestAddressConversionRoundTrip(t *testing.T) {
	// Test round-trip conversion: Ethereum -> Injective -> Ethereum
	ethAddr := "0x71562b71999873db5b286df957af199ec94617f7"

	// Convert Ethereum to Injective
	injAddr, err := ethToInjectiveAddressForTest(ethAddr)
	assert.NoError(t, err, "failed to convert Ethereum to Injective")

	// Convert back to Ethereum
	ethAddr2, err := InjectiveToEthAddress(injAddr)
	assert.NoError(t, err, "failed to convert Injective to Ethereum")

	// Compare (case-insensitive for hex)
	if ethAddr2 != ethAddr {
		t.Errorf("round-trip conversion failed: %s -> %s -> %s (expected %s)",
			ethAddr, injAddr, ethAddr2, ethAddr)
	}
}

// Helper function for testing (simplified version of EthToInjectiveAddress)
func ethToInjectiveAddressForTest(ethAddr string) (string, error) {
	ethAddr = strings.TrimPrefix(ethAddr, "0x")
	addrBytes, err := hex.DecodeString(ethAddr)
	if err != nil {
		return "", err
	}
	if len(addrBytes) != 20 {
		return "", fmt.Errorf("invalid eth address length")
	}
	conv, err := bech32.ConvertBits(addrBytes, 8, 5, true)
	if err != nil {
		return "", err
	}
	prefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
	return bech32.Encode(prefix, conv)
}
