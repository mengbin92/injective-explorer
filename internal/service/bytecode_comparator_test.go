package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCompareBytecodeFullMatch tests full bytecode match.
func TestCompareBytecodeFullMatch(t *testing.T) {
	testCases := []struct {
		name     string
		compiled string
		onchain  string
		expected BytecodeMatchType
	}{
		{
			name:     "exact match without prefix",
			compiled: "608060405234801561001057600080fd5b50610150806100206000",
			onchain:  "608060405234801561001057600080fd5b50610150806100206000",
			expected: BytecodeMatchFull,
		},
		{
			name:     "exact match with 0x prefix on one side",
			compiled: "0x608060405234801561001057600080fd5b50610150806100206000",
			onchain:  "608060405234801561001057600080fd5b50610150806100206000",
			expected: BytecodeMatchFull,
		},
		{
			name:     "exact match with 0x prefix on both sides",
			compiled: "0x608060405234801561001057600080fd5b50610150806100206000",
			onchain:  "0x608060405234801561001057600080fd5b50610150806100206000",
			expected: BytecodeMatchFull,
		},
		{
			name:     "case insensitive match",
			compiled: "608060405234801561001057600080FD5B50610150806100206000",
			onchain:  "608060405234801561001057600080fd5b50610150806100206000",
			expected: BytecodeMatchFull,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := CompareBytecode(tc.compiled, tc.onchain)
			assert.Equal(t, tc.expected, result.MatchType, "match type mismatch: %s", result.Message)
		})
	}
}

// TestCompareBytecodeNoMatch tests bytecode mismatch.
func TestCompareBytecodeNoMatch(t *testing.T) {
	testCases := []struct {
		name     string
		compiled string
		onchain  string
	}{
		{
			name:     "completely different bytecode",
			compiled: "608060405234801561001057600080fd",
			onchain:  "6080604052348015610020abcdef1234",
		},
		{
			name:     "empty compiled bytecode",
			compiled: "",
			onchain:  "608060405234801561001057600080fd",
		},
		{
			name:     "empty onchain bytecode",
			compiled: "608060405234801561001057600080fd",
			onchain:  "",
		},
		{
			name:     "both empty",
			compiled: "",
			onchain:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := CompareBytecode(tc.compiled, tc.onchain)
			assert.Equal(t, BytecodeMatchNone, result.MatchType, "expected no match: %s", result.Message)
		})
	}
}

// TestNormalizeBytecode tests bytecode normalization.
func TestNormalizeBytecode(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"0x608060", "608060"},
		{"0X608060", "608060"},
		{"  0x608060  ", "608060"},
		{"608060", "608060"},
		{"608060AABB", "608060aabb"},
		{"", ""},
	}

	for _, tc := range testCases {
		result := normalizeBytecode(tc.input)
		assert.Equal(t, tc.expected, result, "normalize failed for input: %q", tc.input)
	}
}

// TestStripSolidityMetadata tests metadata stripping from bytecode.
func TestStripSolidityMetadata(t *testing.T) {
	t.Run("strip metadata from valid bytecode", func(t *testing.T) {
		// Simulated bytecode with CBOR metadata:
		// Main bytecode (some bytes) + CBOR metadata (a2 prefix) + 2-byte length
		// The metadata section: a2 64 69 70 66 73 ... (a2 = CBOR map with 2 entries)
		// Length: 0x0033 = 51 bytes

		// For a realistic test, we construct bytecode with known metadata
		mainCode := "608060405234801561001057600080fd5b50"

		// CBOR metadata (simplified): a2 + some bytes, total 51 bytes
		// a2 = map(2), followed by key-value pairs, ends with solc version
		metadata := "a264697066735822122000000000000000000000000000000000000000000000000000000000000000006473" +
			"6f6c6343000804"
		metadataLen := len(metadata) / 2 // in bytes
		lenHex := "0033"                 // 51 bytes

		fullBytecode := mainCode + metadata + lenHex

		stripped, err := stripSolidityMetadata(fullBytecode)
		assert.NoError(t, err)
		assert.Equal(t, mainCode, stripped, "metadata should be stripped, leaving only main code")
		_ = metadataLen
	})

	t.Run("no metadata to strip", func(t *testing.T) {
		bytecode := "608060"
		result, err := stripSolidityMetadata(bytecode)
		assert.NoError(t, err)
		assert.Equal(t, bytecode, result, "short bytecode should be returned as-is")
	})

	t.Run("empty bytecode", func(t *testing.T) {
		result, err := stripSolidityMetadata("")
		assert.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("invalid hex input", func(t *testing.T) {
		_, err := stripSolidityMetadata("zzzz_not_valid_hex")
		assert.Error(t, err, "invalid hex should return error")
		assert.Contains(t, err.Error(), "invalid hex bytecode")
	})

	t.Run("metadata length exceeds data", func(t *testing.T) {
		// Construct bytecode where last 2 bytes claim a metadata length
		// larger than the actual data (metadataLength+2 > len(data))
		// 4 bytes of data, last 2 bytes = 0x00FF = 255 (way too large)
		bytecode := "aabb00ff"
		result, err := stripSolidityMetadata(bytecode)
		assert.NoError(t, err)
		assert.Equal(t, bytecode, result, "should return original when metadata length exceeds data")
	})

	t.Run("invalid CBOR marker (below 0xa1)", func(t *testing.T) {
		// Build bytecode where metadata length is valid but CBOR marker is 0x90 (not a map)
		// Main code: "6080", metadata: "90" (1 byte), length: "0001"
		bytecode := "6080" + "90" + "0001"
		result, err := stripSolidityMetadata(bytecode)
		assert.NoError(t, err)
		assert.Equal(t, bytecode, result, "should return original when CBOR marker is invalid")
	})

	t.Run("invalid CBOR marker (above 0xa5)", func(t *testing.T) {
		// Main code: "6080", metadata: "b0" (1 byte), length: "0001"
		bytecode := "6080" + "b0" + "0001"
		result, err := stripSolidityMetadata(bytecode)
		assert.NoError(t, err)
		assert.Equal(t, bytecode, result, "should return original when CBOR marker is above 0xa5")
	})

	t.Run("zero metadata length", func(t *testing.T) {
		// Last 2 bytes = 0x0000 (zero length)
		bytecode := "608060405234801561001057600080fd5b500000"
		result, err := stripSolidityMetadata(bytecode)
		assert.NoError(t, err)
		assert.Equal(t, bytecode, result, "should return original when metadata length is zero")
	})

	t.Run("metadata length exceeds MaxSolidityMetadataSize", func(t *testing.T) {
		// Last 2 bytes = 0x00C9 = 201 (exceeds MaxSolidityMetadataSize of 200)
		// Need enough data: 201 + 2 = 203 bytes minimum
		mainCode := "6080604052348015610010"
		padding := ""
		for len(mainCode+padding)/2 < 201 {
			padding += "00"
		}
		bytecode := mainCode + padding + "00c9"
		result, err := stripSolidityMetadata(bytecode)
		assert.NoError(t, err)
		assert.Equal(t, bytecode, result, "should return original when metadata length exceeds max")
	})
}

// TestCompareBytecodePartialMatch tests partial match (ignoring metadata).
func TestCompareBytecodePartialMatch(t *testing.T) {
	// Create two bytecodes that are the same except for the metadata hash
	mainCode := "608060405234801561001057600080fd5b50610150806100206000396000f3fe"

	// Same CBOR structure but different hash values
	metadata1 := "a264697066735822122011111111111111111111111111111111111111111111111111111111111111116473" +
		"6f6c6343000804"
	metadata2 := "a264697066735822122022222222222222222222222222222222222222222222222222222222222222226473" +
		"6f6c6343000804"

	lenHex := "0033" // 51 bytes

	bytecode1 := mainCode + metadata1 + lenHex
	bytecode2 := mainCode + metadata2 + lenHex

	result := CompareBytecode(bytecode1, bytecode2)
	assert.Equal(t, BytecodeMatchPartial, result.MatchType,
		"should be a partial match when only metadata differs: %s", result.Message)
}

// TestCompareBytecodeWithInvalidHex tests that CompareBytecode handles invalid hex gracefully.
func TestCompareBytecodeWithInvalidHex(t *testing.T) {
	result := CompareBytecode("not_valid_hex", "608060405234801561001057600080fd")
	assert.Equal(t, BytecodeMatchNone, result.MatchType)
	assert.Contains(t, result.Message, "compiled bytecode error")
}
