// Package utils provides shared utility functions used across the application.
// It includes helpers for string manipulation, validation, pagination, caching,
// database operations, and context management.
package utils

import (
	"fmt"
	"regexp"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	// injAddressRegex matches Injective addresses (inj1 followed by 38 base58 characters)
	// Note: This regex is updated at runtime via init() to match the configured prefix
	injAddressRegex *regexp.Regexp
	// evmAddressRegex matches EVM addresses (0x followed by 40 hex characters)
	// Events and Signatures often store addresses in EVM format after convertEventAddressesToEVM
	evmAddressRegex = regexp.MustCompile(`0x[0-9a-fA-F]{40}`)
)

func init() {
	// Initialize regex with the configured account address prefix
	prefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
	injAddressRegex = regexp.MustCompile(prefix + `1[0-9a-z]{38}`)
}

// ExtractAddressesFromString extracts Injective and EVM addresses from a string.
// Supports both formats:
//   - Injective: configured prefix + "1" + 38 chars (e.g., inj1...)
//   - EVM: 0x + 40 hex chars (e.g., 0x1234...)
//
// Events and Signatures often use EVM format after convertEventAddressesToEVM,
// so both formats must be extracted for account table population.
//
// Parameters:
//   - s: The string to extract addresses from
//
// Returns:
//   - []string: A slice of unique addresses found in the string (deduplicated)
func ExtractAddressesFromString(s string) []string {
	seen := make(map[string]bool)

	// Pre-find all matches to estimate capacity and avoid slice reallocation
	matchesInj := injAddressRegex.FindAllString(s, -1)
	matchesEvm := evmAddressRegex.FindAllString(s, -1)

	// Pre-allocate with estimated capacity to avoid multiple slice growth
	result := make([]string, 0, len(matchesInj)+len(matchesEvm))

	// Extract Injective addresses
	for _, match := range matchesInj {
		if !seen[match] {
			seen[match] = true
			result = append(result, match)
		}
	}

	// Extract EVM addresses (normalize to lowercase for dedup)
	for _, match := range matchesEvm {
		normalized := strings.ToLower(match)
		if !seen[normalized] {
			seen[normalized] = true
			result = append(result, normalized)
		}
	}

	return result
}

// IsValidAddress checks if a string is a valid Injective address.
//
// Parameters:
//   - addr: The address string to validate
//
// Returns:
//   - bool: True if the address is valid, false otherwise
func IsValidAddress(addr string) bool {
	// Check for EVM address format (0x prefix + 40 hex characters)
	if strings.HasPrefix(addr, "0x") && len(addr) == 42 {
		// Validate hex characters
		hexPart := addr[2:]
		for _, c := range hexPart {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
		return true
	}

	// Injective addresses start with the configured prefix + "1" and are 42 characters long
	prefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
	return strings.HasPrefix(addr, prefix+"1") && len(addr) == 42
}

// NormalizeEVMAddress normalizes an EVM address to standard format (0x prefix + lowercase hex)
// Returns empty string if the address is not a valid EVM address
func NormalizeEVMAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}

	// Add 0x prefix if missing
	if !strings.HasPrefix(addr, "0x") && !strings.HasPrefix(addr, "0X") {
		addr = "0x" + addr
	}

	// Convert to lowercase
	addr = strings.ToLower(addr)

	// Validate format (0x + 40 hex characters)
	if len(addr) != 42 {
		return ""
	}

	hexPart := addr[2:]
	for _, c := range hexPart {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return ""
		}
	}

	return addr
}

// NormalizeEVMHash normalizes an EVM transaction hash to standard format (0x prefix + lowercase hex)
// Returns empty string if the hash is not a valid EVM hash
func NormalizeEVMHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return ""
	}

	// Add 0x prefix if missing
	if !strings.HasPrefix(hash, "0x") && !strings.HasPrefix(hash, "0X") {
		hash = "0x" + hash
	}

	// Convert to lowercase
	hash = strings.ToLower(hash)

	// Validate format (0x + 64 hex characters)
	if len(hash) != 66 {
		return ""
	}

	hexPart := hash[2:]
	for _, c := range hexPart {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return ""
		}
	}

	return hash
}

// ReverseString reverses a string.
//
// Parameters:
//   - s: The string to reverse
//
// Returns:
//   - string: The reversed string
func ReverseString(s string) string {
	b := []byte(s)
	i := 0
	j := len(b) - 1
	for i < j {
		b[i], b[j] = b[j], b[i]
		i++
		j--
	}
	return string(b)
}

// FirstNonEmpty returns the first non-empty string from the given arguments.
//
// Parameters:
//   - a: First string
//   - b: Second string
//
// Returns:
//   - string: The first non-empty string, or empty string if both are empty
func FirstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Max returns the maximum of two integers.
//
// Parameters:
//   - a: First integer
//   - b: Second integer
//
// Returns:
//   - int: The maximum of the two integers
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Uint64ToString converts a uint64 to string.
//
// Parameters:
//   - v: The uint64 value to convert
//
// Returns:
//   - string: The string representation of the value
func Uint64ToString(v uint64) string {
	return fmt.Sprintf("%d", v)
}

// Int64ToString converts an int64 to string.
//
// Parameters:
//   - v: The int64 value to convert
//
// Returns:
//   - string: The string representation of the value
func Int64ToString(v int64) string {
	return fmt.Sprintf("%d", v)
}
