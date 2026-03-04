// Package service provides bytecode comparison utilities for contract verification.
package service

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// BytecodeMatchType represents the type of bytecode match.
type BytecodeMatchType string

const (
	// BytecodeMatchFull indicates a full bytecode match.
	BytecodeMatchFull BytecodeMatchType = "full_match"
	// BytecodeMatchPartial indicates a partial match (ignoring metadata hash).
	BytecodeMatchPartial BytecodeMatchType = "partial_match"
	// BytecodeMatchNone indicates no match.
	BytecodeMatchNone BytecodeMatchType = "no_match"

	// MaxSolidityMetadataSize is the maximum expected size (in bytes) of the CBOR-encoded
	// metadata section appended by the Solidity compiler. Typical metadata is ~50 bytes,
	// but larger values can occur with extended metadata (source maps, experimental features).
	// 200 bytes provides a generous upper bound while filtering out invalid length values.
	MaxSolidityMetadataSize = 200
)

// CompareBytecodeResult holds the result of a bytecode comparison.
type CompareBytecodeResult struct {
	// MatchType is the type of match found.
	MatchType BytecodeMatchType
	// Message provides additional details about the comparison.
	Message string
}

// CompareBytecode compares compiled bytecode with on-chain bytecode.
// It first tries a full match, then falls back to partial match (ignoring Solidity metadata hash).
//
// Parameters:
//   - compiledBytecode: the bytecode produced by the compiler (hex string, with or without "0x" prefix)
//   - onchainBytecode: the bytecode retrieved from the chain (hex string, with or without "0x" prefix)
//
// Returns:
//   - CompareBytecodeResult: the comparison result
func CompareBytecode(compiledBytecode, onchainBytecode string) CompareBytecodeResult {
	compiled := normalizeBytecode(compiledBytecode)
	onchain := normalizeBytecode(onchainBytecode)

	if compiled == "" || onchain == "" {
		return CompareBytecodeResult{
			MatchType: BytecodeMatchNone,
			Message:   "one or both bytecodes are empty",
		}
	}

	// Try full match first
	if compiled == onchain {
		return CompareBytecodeResult{
			MatchType: BytecodeMatchFull,
			Message:   "bytecodes match exactly",
		}
	}

	// Try partial match: strip Solidity metadata hash from both
	compiledStripped, err1 := stripSolidityMetadata(compiled)
	onchainStripped, err2 := stripSolidityMetadata(onchain)

	// If stripping failed for either, we can still try the comparison
	// but only if at least one succeeded
	if err1 == nil && err2 == nil && compiledStripped != "" && onchainStripped != "" && compiledStripped == onchainStripped {
		return CompareBytecodeResult{
			MatchType: BytecodeMatchPartial,
			Message:   "bytecodes match after stripping metadata hash",
		}
	}

	message := "bytecodes do not match"
	if err1 != nil {
		message = fmt.Sprintf("bytecodes do not match (compiled bytecode error: %v)", err1)
	} else if err2 != nil {
		message = fmt.Sprintf("bytecodes do not match (on-chain bytecode error: %v)", err2)
	}

	return CompareBytecodeResult{
		MatchType: BytecodeMatchNone,
		Message:   message,
	}
}

// normalizeBytecode normalizes a bytecode hex string by removing "0x" prefix
// and converting to lowercase.
func normalizeBytecode(bytecode string) string {
	bytecode = strings.TrimSpace(bytecode)
	bytecode = strings.TrimPrefix(bytecode, "0x")
	bytecode = strings.TrimPrefix(bytecode, "0X")
	return strings.ToLower(bytecode)
}

// stripSolidityMetadata removes the Solidity metadata hash from the end of bytecode.
//
// Solidity appends a CBOR-encoded metadata section at the end of the deployed bytecode.
// The format is:
//   - For solc >= 0.6.0: `<metadata_hash><2-byte length>` ending with the length indicator
//   - The metadata starts with CBOR marker 'a2' (or 'a1') followed by key-value pairs
//   - The last 2 bytes contain the length of the CBOR encoded metadata in big-endian
//
// Example: ...a26469706673582212...64736f6c634300080400330033
//
// By stripping this metadata, we can compare bytecodes compiled in different environments
// (which may produce different metadata hashes due to different file paths etc.).
func stripSolidityMetadata(bytecode string) (string, error) {
	if len(bytecode) < 4 {
		return bytecode, nil
	}

	// Decode to bytes for easier processing
	data, err := hex.DecodeString(bytecode)
	if err != nil {
		return "", fmt.Errorf("invalid hex bytecode: %w", err)
	}

	if len(data) < 2 {
		return bytecode, nil
	}

	// Read the last 2 bytes as the CBOR metadata length
	metadataLength := int(data[len(data)-2])<<8 | int(data[len(data)-1])

	// Validate: metadata length should be reasonable and the total
	// should not exceed the bytecode length
	if metadataLength <= 0 || metadataLength > MaxSolidityMetadataSize || metadataLength+2 > len(data) {
		return bytecode, nil
	}

	// Check if the metadata section starts with a valid CBOR map marker
	metadataStart := len(data) - 2 - metadataLength
	if metadataStart < 0 || metadataStart >= len(data) {
		return bytecode, nil
	}

	// CBOR map markers: 0xa1 (1 pair), 0xa2 (2 pairs), ..., 0xa5 (5 pairs)
	marker := data[metadataStart]
	if marker < 0xa1 || marker > 0xa5 {
		return bytecode, nil
	}

	// Strip the metadata and length bytes
	strippedData := data[:metadataStart]
	return hex.EncodeToString(strippedData), nil
}
