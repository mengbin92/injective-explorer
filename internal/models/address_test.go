package models

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil/bech32"
)

func TestInjToEthAddress(t *testing.T) {
	tests := []struct {
		name      string
		injAddr   string
		wantEth   string
		wantError bool
	}{
		{
			name:      "valid Injective address",
			injAddr:   "inj1w9tzkuvenpeakkegdhu40tcenmy5v9lhhcq6ea",
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
			got, err := InjToEthAddress(tt.injAddr)
			if (err != nil) != tt.wantError {
				t.Errorf("InjToEthAddress() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if got != tt.wantEth {
				t.Errorf("InjToEthAddress() = %v, want %v", got, tt.wantEth)
			}
		})
	}
}

func TestAddressConversionRoundTrip(t *testing.T) {
	// Test round-trip conversion: Ethereum -> Injective -> Ethereum
	ethAddr := "0x71562b71999873db5b286df957af199ec94617f7"

	// Convert Ethereum to Injective
	injAddr, err := ethToInjAddressForTest(ethAddr)
	if err != nil {
		t.Fatalf("failed to convert Ethereum to Injective: %v", err)
	}

	// Convert back to Ethereum
	ethAddr2, err := InjToEthAddress(injAddr)
	if err != nil {
		t.Fatalf("failed to convert Injective to Ethereum: %v", err)
	}

	// Compare (case-insensitive for hex)
	if ethAddr2 != ethAddr {
		t.Errorf("round-trip conversion failed: %s -> %s -> %s (expected %s)",
			ethAddr, injAddr, ethAddr2, ethAddr)
	}
}

// Helper function for testing (simplified version of EthToInjAddress)
func ethToInjAddressForTest(ethAddr string) (string, error) {
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
	return bech32.Encode("inj", conv)
}
