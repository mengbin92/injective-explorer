// Package service provides validation utilities for service layer.
package service

import (
	"fmt"
	"regexp"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	// ethAddressRegex validates Ethereum addresses (0x followed by 40 hex characters)
	ethAddressRegex = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
	// txHashRegex validates transaction hashes (hex string, typically 64 characters)
	txHashRegex = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
)

// getAccountAddressRegex returns the regex for validating account addresses
func getAccountAddressRegex() *regexp.Regexp {
	prefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
	return regexp.MustCompile(`^` + prefix + `1[0-9a-z]{38}$`)
}

// getValidatorAddressRegex returns the regex for validating validator addresses
func getValidatorAddressRegex() *regexp.Regexp {
	prefix := sdk.GetConfig().GetBech32ValidatorAddrPrefix()
	return regexp.MustCompile(`^` + prefix + `1[0-9a-z]{38}$`)
}

// ValidateAddress validates a blockchain address format.
// It supports injective chain (configured prefix...), validator (configured valoper prefix...) and Ethereum (0x...) addresses.
//
// Parameters:
//   - address: The address string to validate
//
// Returns:
//   - error: Validation error if address format is invalid
func ValidateAddress(address string) error {
	if address == "" {
		return fmt.Errorf("address cannot be empty")
	}

	// Check injective chain address format
	accPrefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
	if strings.HasPrefix(address, accPrefix+"1") {
		if !getAccountAddressRegex().MatchString(address) {
			return fmt.Errorf("invalid injective chain address format: %s", address)
		}
		return nil
	}

	// Check injective chain validator address format
	valPrefix := sdk.GetConfig().GetBech32ValidatorAddrPrefix()
	if strings.HasPrefix(address, valPrefix+"1") {
		if !getValidatorAddressRegex().MatchString(address) {
			return fmt.Errorf("invalid injective chain validator address format: %s", address)
		}
		return nil
	}

	// Check Ethereum address format
	if strings.HasPrefix(address, "0x") {
		if !ethAddressRegex.MatchString(address) {
			return fmt.Errorf("invalid Ethereum address format: %s", address)
		}
		return nil
	}

	// Unknown address format
	return fmt.Errorf("unknown address format: %s (expected %s1..., %s1... or 0x...)", address, accPrefix, valPrefix)
}

// ValidateTransactionHash validates a transaction hash format.
//
// Parameters:
//   - hash: The transaction hash string to validate
//
// Returns:
//   - error: Validation error if hash format is invalid
func ValidateTransactionHash(hash string) error {
	if hash == "" {
		return fmt.Errorf("transaction hash cannot be empty")
	}

	if !txHashRegex.MatchString(hash) {
		return fmt.Errorf("invalid transaction hash format: %s (expected 64 hex characters)", hash)
	}

	return nil
}

// ValidateBlockHeight validates a block height value.
// It accepts both int64 and uint64 for flexibility.
//
// Parameters:
//   - height: The block height to validate (can be int64 or uint64)
//
// Returns:
//   - error: Validation error if height is invalid
func ValidateBlockHeight(height interface{}) error {
	var h uint64
	switch v := height.(type) {
	case int64:
		if v < 0 {
			return fmt.Errorf("block height cannot be negative")
		}
		h = uint64(v)
	case uint64:
		h = v
	case int32:
		if v < 0 {
			return fmt.Errorf("block height cannot be negative")
		}
		h = uint64(v)
	case uint32:
		h = uint64(v)
	default:
		return fmt.Errorf("invalid block height type: %T", height)
	}

	if h == 0 {
		return fmt.Errorf("block height cannot be zero")
	}

	// Maximum reasonable block height (can be adjusted based on chain)
	const maxBlockHeight = 1 << 50 // 2^50 is a very large number
	if h > maxBlockHeight {
		return fmt.Errorf("block height %d exceeds maximum allowed value", h)
	}

	return nil
}

// ValidateLimit validates a limit parameter for pagination.
// It accepts both int32 and uint32 for flexibility.
//
// Parameters:
//   - limit: The limit value to validate (can be int32 or uint32)
//   - maxLimit: The maximum allowed limit value (uint32)
//
// Returns:
//   - error: Validation error if limit is invalid
func ValidateLimit(limit interface{}, maxLimit uint32) error {
	var l uint32
	switch v := limit.(type) {
	case int32:
		if v < 0 {
			return fmt.Errorf("limit cannot be negative")
		}
		l = uint32(v)
	case uint32:
		l = v
	case int64:
		if v < 0 {
			return fmt.Errorf("limit cannot be negative")
		}
		if v > int64(maxLimit) {
			return fmt.Errorf("limit %d exceeds maximum allowed value %d", v, maxLimit)
		}
		l = uint32(v)
	case uint64:
		if v > uint64(maxLimit) {
			return fmt.Errorf("limit %d exceeds maximum allowed value %d", v, maxLimit)
		}
		l = uint32(v)
	default:
		return fmt.Errorf("invalid limit type: %T", limit)
	}

	if l == 0 {
		return fmt.Errorf("limit cannot be zero")
	}

	if l > maxLimit {
		return fmt.Errorf("limit %d exceeds maximum allowed value %d", l, maxLimit)
	}

	return nil
}

// SanitizeAddress sanitizes an address string by removing whitespace and converting to lowercase.
//
// Parameters:
//   - address: The address string to sanitize
//
// Returns:
//   - string: The sanitized address
func SanitizeAddress(address string) string {
	address = strings.TrimSpace(address)
	address = strings.ToLower(address)
	return address
}

// SanitizeHash sanitizes a hash string by removing whitespace, converting to lowercase,
// and removing the 0x prefix if present.
//
// Parameters:
//   - hash: The hash string to sanitize
//
// Returns:
//   - string: The sanitized hash
func SanitizeHash(hash string) string {
	hash = strings.TrimSpace(hash)
	hash = strings.ToLower(hash)
	// Remove 0x prefix if present
	hash = strings.TrimPrefix(hash, "0x")
	return hash
}
