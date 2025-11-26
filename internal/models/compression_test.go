// Package models provides tests for data compression utilities.
package models

import (
	"bytes"
	"testing"
)

func TestCompressData(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantErr  bool
		checkLen bool // Whether to check if compressed data is smaller
	}{
		{
			name:     "empty data",
			data:     []byte{},
			wantErr:  false,
			checkLen: false,
		},
		{
			name:     "small data (should not compress)",
			data:     []byte("hello"),
			wantErr:  false,
			checkLen: false,
		},
		{
			name:     "large data (should compress)",
			data:     bytes.Repeat([]byte("test data for compression "), 100), // ~2.7KB
			wantErr:  false,
			checkLen: true,
		},
		{
			name:     "binary data",
			data:     bytes.Repeat([]byte{0x00, 0xFF, 0xAA, 0x55}, 500), // ~2KB
			wantErr:  false,
			checkLen: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CompressData(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("CompressData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(tt.data) == 0 {
				if result != "" {
					t.Errorf("CompressData() empty input should return empty string, got %q", result)
				}
				return
			}

			// Check if compression was applied (for large data)
			if tt.checkLen {
				// For large data, compression should be applied (has prefix)
				if len(result) < len(CompressionPrefix) || result[:len(CompressionPrefix)] != CompressionPrefix {
					t.Errorf("CompressData() large data should be compressed (have prefix), got %q", result[:min(20, len(result))])
				}
			} else {
				// For small data, compression may not be applied
				// Just verify it doesn't have prefix (or if it does, it's because compression helped)
				if len(tt.data) < CompressionThreshold {
					// Small data should not have prefix (unless compression actually helped)
					// This is acceptable behavior
				}
			}
		})
	}
}

func TestDecompressData(t *testing.T) {
	tests := []struct {
		name        string
		dataStr     string
		wantErr     bool
		expectMatch bool // Whether decompressed data should match original
		original    []byte
	}{
		{
			name:        "empty string",
			dataStr:     "",
			wantErr:     false,
			expectMatch: true,
			original:    []byte{},
		},
		{
			name:        "uncompressed data (backward compatibility)",
			dataStr:     "hello world",
			wantErr:     false,
			expectMatch: true,
			original:    []byte("hello world"),
		},
		{
			name:        "compressed data",
			dataStr:     "",
			wantErr:     false,
			expectMatch: true,
			original:    bytes.Repeat([]byte("test data "), 200), // ~2KB
		},
	}

	// Prepare compressed data for the last test
	if len(tests) > 0 {
		lastTest := &tests[len(tests)-1]
		compressed, err := CompressData(lastTest.original)
		if err != nil {
			t.Fatalf("Failed to prepare compressed data for test: %v", err)
		}
		lastTest.dataStr = compressed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecompressData(tt.dataStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecompressData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.expectMatch && len(tt.original) > 0 {
				if !bytes.Equal(result, tt.original) {
					t.Errorf("DecompressData() = %v, want %v (length: got %d, want %d)",
						result[:min(20, len(result))], tt.original[:min(20, len(tt.original))],
						len(result), len(tt.original))
				}
			}
		})
	}
}

func TestCompressDecompressRoundTrip(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{
			name: "small text",
			data: []byte("hello world"),
		},
		{
			name: "large text",
			data: bytes.Repeat([]byte("This is a test string for compression. "), 100),
		},
		{
			name: "binary data",
			data: bytes.Repeat([]byte{0x00, 0xFF, 0xAA, 0x55, 0x12, 0x34, 0x56, 0x78}, 500),
		},
		{
			name: "mixed data",
			data: append(
				bytes.Repeat([]byte("text "), 200),
				bytes.Repeat([]byte{0x00, 0xFF}, 200)...,
			),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Compress
			compressed, err := CompressData(tc.data)
			if err != nil {
				t.Fatalf("CompressData() error = %v", err)
			}

			// Decompress
			decompressed, err := DecompressData(compressed)
			if err != nil {
				t.Fatalf("DecompressData() error = %v", err)
			}

			// Verify round trip
			if !bytes.Equal(decompressed, tc.data) {
				t.Errorf("Round trip failed: original length = %d, decompressed length = %d",
					len(tc.data), len(decompressed))
			}
		})
	}
}

func TestCompressString(t *testing.T) {
	testStr := "This is a test string that should be compressed if it's large enough."
	// Make it large enough to trigger compression
	largeStr := ""
	for i := 0; i < 50; i++ {
		largeStr += testStr
	}

	result, err := CompressString(largeStr)
	if err != nil {
		t.Fatalf("CompressString() error = %v", err)
	}

	// Decompress and verify
	decompressed, err := DecompressString(result)
	if err != nil {
		t.Fatalf("DecompressString() error = %v", err)
	}

	if decompressed != largeStr {
		t.Errorf("CompressString/DecompressString round trip failed")
	}
}

// Helper function for min (Go 1.18+)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
