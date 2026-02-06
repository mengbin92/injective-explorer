// Package converter provides utilities for converting between different address formats
// and normalizing blockchain-related data.
//
// This package is responsible for:
//   - Converting between Bech32 (Injective) and EVM (0x) address formats
//   - Validating address formats
//   - Normalizing hashes and addresses for consistent API responses
package converter

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcutil/bech32"
	github_com_cosmos_cosmos_sdk_types "github.com/cosmos/cosmos-sdk/types"
)

// Common errors for address conversion.
var (
	ErrInvalidAddress = fmt.Errorf("invalid address")
	ErrConversion     = fmt.Errorf("conversion error")
)

// InjectiveToEthAddress converts a Injective address (bech32) to Ethereum address (0x...).
//
// This function handles three cases:
//  1. If the input is already a valid Ethereum address (0x...), it validates and returns it as-is
//  2. If the input is a valid Injective bech32 address, it converts it to Ethereum format
//  3. Otherwise, it returns an error
//
// The conversion process:
//  1. Decode the bech32 address to get the raw bytes
//  2. Convert from 5-bit to 8-bit encoding
//  3. Encode as hexadecimal with 0x prefix
//
// Parameters:
//   - injAddr: The Injective address string or Ethereum address
//
// Returns:
//   - string: The Ethereum address (0x...) in lowercase
//   - error: Error if conversion fails
func InjectiveToEthAddress(injAddr string) (string, error) {
	if injAddr == "" {
		return "", fmt.Errorf("%w: empty address", ErrInvalidAddress)
	}

	// If it's already an Ethereum address, validate and return as-is
	if strings.HasPrefix(injAddr, "0x") {
		// Basic validation: should be 42 characters (0x + 40 hex chars)
		if len(injAddr) != 42 {
			return "", fmt.Errorf("%w: invalid Ethereum address length", ErrInvalidAddress)
		}
		// Validate hex characters
		for i := 2; i < len(injAddr); i++ {
			c := injAddr[i]
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return "", fmt.Errorf("%w: invalid hex character in Ethereum address", ErrInvalidAddress)
			}
		}
		return strings.ToLower(injAddr), nil
	}

	// Validate Injective address format
	if !IsValidInjectiveAddress(injAddr) {
		return "", fmt.Errorf("%w: invalid Injective address format: %s", ErrInvalidAddress, injAddr)
	}

	// 1. bech32 decode
	hrp, data, err := bech32.Decode(injAddr)
	if err != nil {
		return "", fmt.Errorf("%w: failed to decode bech32 address: %v", ErrConversion, err)
	}
	expectedHrp := github_com_cosmos_cosmos_sdk_types.GetConfig().GetBech32AccountAddrPrefix()
	if hrp != expectedHrp {
		return "", fmt.Errorf("%w: invalid HRP, expected '%s', got '%s'", ErrConversion, expectedHrp, hrp)
	}

	// 2. 5-bit → 8-bit
	conv, err := bech32.ConvertBits(data, 5, 8, false)
	if err != nil {
		return "", fmt.Errorf("%w: failed to convert bits: %v", ErrConversion, err)
	}
	if len(conv) != 20 {
		return "", fmt.Errorf("%w: invalid address length: expected 20 bytes, got %d", ErrConversion, len(conv))
	}

	// 3. bytes → hex with 0x prefix (lowercase for consistency)
	return fmt.Sprintf("0x%s", strings.ToLower(hex.EncodeToString(conv))), nil
}

// EthToInjectiveAddress converts an Ethereum address (0x...) to Injective address.
//
// Parameters:
//   - ethAddr: The Ethereum address string (0x...)
//
// Returns:
//   - string: The Injective address (with configured account prefix)
//   - error: Error if conversion fails
func EthToInjectiveAddress(ethAddr string) (string, error) {
	if ethAddr == "" {
		return "", fmt.Errorf("%w: empty address", ErrInvalidAddress)
	}

	// Remove 0x prefix if present
	ethAddr = strings.TrimPrefix(ethAddr, "0x")

	// Validate hex string length (should be 40 characters for 20 bytes)
	if len(ethAddr) != 40 {
		return "", fmt.Errorf("%w: invalid Ethereum address length", ErrInvalidAddress)
	}

	// Decode hex to bytes
	addrBytes, err := hex.DecodeString(ethAddr)
	if err != nil {
		return "", fmt.Errorf("%w: invalid hex string: %v", ErrInvalidAddress, err)
	}

	// Convert from 8-bit to 5-bit encoding
	conv, err := bech32.ConvertBits(addrBytes, 8, 5, true)
	if err != nil {
		return "", fmt.Errorf("%w: failed to convert bits: %v", ErrConversion, err)
	}

	// Encode as bech32 with the configured account prefix
	prefix := github_com_cosmos_cosmos_sdk_types.GetConfig().GetBech32AccountAddrPrefix()
	bech32Addr, err := bech32.Encode(prefix, conv)
	if err != nil {
		return "", fmt.Errorf("%w: failed to encode bech32: %v", ErrConversion, err)
	}

	return bech32Addr, nil
}

// IsValidInjectiveAddress checks if the given address is a valid Injective address.
//
// A valid Injective address must:
//  1. Not be empty
//  2. Start with the configured account prefix + "1"
//  3. Be a valid bech32 encoded string
//
// Parameters:
//   - addr: The address string to validate
//
// Returns:
//   - bool: True if the address is valid, false otherwise
func IsValidInjectiveAddress(addr string) bool {
	if addr == "" {
		return false
	}
	prefix := github_com_cosmos_cosmos_sdk_types.GetConfig().GetBech32AccountAddrPrefix()
	if len(addr) < 10 || !strings.HasPrefix(addr, prefix+"1") {
		return false
	}
	_, _, err := bech32.Decode(addr)
	return err == nil
}

// NormalizeHash ensures the hash has 0x prefix and is lowercase (EVM format).
// This is used for cross-chain transaction hashes and addresses to maintain
// consistent EVM-style formatting across the API.
//
// Parameters:
//   - hash: The hash or address string to normalize
//
// Returns:
//   - string: The normalized hash with 0x prefix in lowercase
func NormalizeHash(hash string) string {
	if hash == "" {
		return ""
	}
	// Remove any existing 0x prefix
	hash = strings.TrimPrefix(strings.ToLower(hash), "0x")
	// Add 0x prefix
	return "0x" + hash
}
