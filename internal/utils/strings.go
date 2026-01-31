// Package utils provides utility functions used across the application.
package utils

import (
	"regexp"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	// injAddressRegex matches Injective addresses (inj1 followed by 38 base58 characters)
	// Note: This regex is updated at runtime via init() to match the configured prefix
	injAddressRegex *regexp.Regexp
)

func init() {
	// Initialize regex with the configured account address prefix
	prefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
	injAddressRegex = regexp.MustCompile(prefix + `1[0-9a-z]{38}`)
}

// ExtractAddressesFromString extracts Injective addresses from a string.
// Injective addresses typically start with the configured prefix (e.g., "inj1") and are 42 characters long.
//
// Parameters:
//   - s: The string to extract addresses from
//
// Returns:
//   - []string: A slice of unique Injective addresses found in the string
func ExtractAddressesFromString(s string) []string {
	// Pattern for Injective addresses: configured prefix + "1" followed by 38 characters (base58 encoded)
	// Total length: 42 characters
	matches := injAddressRegex.FindAllString(s, -1)

	// Remove duplicates
	seen := make(map[string]bool)
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		if !seen[match] {
			seen[match] = true
			result = append(result, match)
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
	// Injective addresses start with the configured prefix + "1" and are 42 characters long
	prefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
	return strings.HasPrefix(addr, prefix+"1") && len(addr) == 42
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
