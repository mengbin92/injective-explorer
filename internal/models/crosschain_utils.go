package models

import "strings"

// normalizeHash ensures the hash has 0x prefix and is lowercase (EVM format).
// This is used for cross-chain transaction hashes and addresses to maintain
// consistent EVM-style formatting across the API.
func normalizeHash(hash string) string {
	if hash == "" {
		return ""
	}
	// Remove any existing 0x prefix
	hash = strings.TrimPrefix(strings.ToLower(hash), "0x")
	// Add 0x prefix
	return "0x" + hash
}
