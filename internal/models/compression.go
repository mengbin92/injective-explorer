// Package models provides data compression utilities for reducing storage space.
// This file contains functions for compressing and decompressing large data fields.
package models

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"unicode/utf8"
)

const (
	// CompressionThreshold is the minimum size (in bytes) before compression is applied.
	// Data smaller than this threshold will not be compressed to avoid overhead.
	CompressionThreshold = 1024 // 1KB

	// CompressionPrefix is the prefix used to identify compressed data.
	// This allows backward compatibility with uncompressed data.
	CompressionPrefix = "gzip:"

	// CompressionLevel is the gzip compression level (0-9).
	// Level 6 provides a good balance between compression ratio and speed.
	CompressionLevel = 6
)

// CompressData compresses data using gzip if it exceeds the compression threshold.
// Returns the compressed data (base64 encoded) with a prefix, or base64 encoded original data if compression is not beneficial.
// This ensures binary data can be safely stored in UTF-8 string columns.
//
// Parameters:
//   - data: The data to compress (can be nil or empty)
//
// Returns:
//   - string: Compressed data (base64) with prefix, or base64 encoded original data if not compressed
//   - error: Error if compression fails
func CompressData(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	// Check if data is valid UTF-8 text
	isText := isValidUTF8Text(data)

	// Don't compress small data - compression overhead may not be worth it
	if len(data) < CompressionThreshold {
		if isText {
			// For small text data, return as-is
			return string(data), nil
		}
		// For small binary data, encode to base64 for safe storage
		return base64.StdEncoding.EncodeToString(data), nil
	}

	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, CompressionLevel)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip writer: %w", err)
	}

	if _, err := writer.Write(data); err != nil {
		writer.Close()
		return "", fmt.Errorf("failed to write data to gzip writer: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close gzip writer: %w", err)
	}

	compressed := buf.Bytes()

	// Only use compression if it actually reduces size (accounting for prefix and base64 overhead)
	// Base64 encoding increases size by ~33%, so we need to account for that
	base64Overhead := (len(compressed) * 4 / 3) - len(compressed) // Approximate base64 overhead
	prefixOverhead := len(CompressionPrefix)
	totalOverhead := prefixOverhead + base64Overhead

	if len(compressed)+totalOverhead >= len(data) {
		// Compression didn't help, return original (base64 encoded if binary)
		if isText {
			return string(data), nil
		}
		return base64.StdEncoding.EncodeToString(data), nil
	}

	// Encode compressed binary data to base64 for safe storage in database
	// This ensures the data can be stored in UTF-8 string columns
	compressedBase64 := base64.StdEncoding.EncodeToString(compressed)

	// Return compressed data with prefix (base64 encoded)
	return CompressionPrefix + compressedBase64, nil
}

// DecompressData decompresses data if it has the compression prefix.
// Returns the decompressed data, or the original data if it's not compressed.
// This function is backward compatible with uncompressed data.
//
// Parameters:
//   - dataStr: The data string (may be compressed with prefix or uncompressed)
//
// Returns:
//   - []byte: Decompressed data, or original data if not compressed
//   - error: Error if decompression fails
func DecompressData(dataStr string) ([]byte, error) {
	if dataStr == "" {
		return nil, nil
	}

	// Check if data is compressed (has prefix)
	if len(dataStr) < len(CompressionPrefix) || dataStr[:len(CompressionPrefix)] != CompressionPrefix {
		// Not compressed, try to decode as base64 (might be base64 encoded binary data)
		// If base64 decode fails, return as-is (plain text)
		if decoded, err := base64.StdEncoding.DecodeString(dataStr); err == nil {
			return decoded, nil
		}
		return []byte(dataStr), nil
	}

	// Remove prefix and decode base64
	compressedBase64 := dataStr[len(CompressionPrefix):]
	compressedData, err := base64.StdEncoding.DecodeString(compressedBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 compressed data: %w", err)
	}

	// Decompress the binary data
	reader, err := gzip.NewReader(bytes.NewReader(compressedData))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read decompressed data: %w", err)
	}

	return decompressed, nil
}

// CompressString compresses a string using CompressData.
// This is a convenience function for string data.
//
// Parameters:
//   - data: The string to compress
//
// Returns:
//   - string: Compressed data with prefix, or original string if not compressed
//   - error: Error if compression fails
func CompressString(data string) (string, error) {
	return CompressData([]byte(data))
}

// DecompressString decompresses a string using DecompressData.
// This is a convenience function for string data.
//
// Parameters:
//   - dataStr: The compressed or uncompressed string
//
// Returns:
//   - string: Decompressed string, or original string if not compressed
//   - error: Error if decompression fails
func DecompressString(dataStr string) (string, error) {
	data, err := DecompressData(dataStr)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// isValidUTF8Text checks if the byte slice contains valid UTF-8 encoded text.
// This is used to determine if data can be safely stored as a string without base64 encoding.
func isValidUTF8Text(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	return utf8.Valid(data)
}
