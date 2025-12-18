// Package models provides unit tests for protobuf utility functions.
package models

import (
	"testing"

	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

// TestMarshalProtoToBase64 tests the MarshalProtoToBase64 function.
func TestMarshalProtoToBase64(t *testing.T) {
	tests := []struct {
		name    string
		msg     proto.Message
		wantErr bool
	}{
		{
			name:    "nil message",
			msg:     nil,
			wantErr: true,
		},
		{
			name:    "valid message",
			msg:     &injective_explorer_rpcpb.TxData{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := MarshalProtoToBase64(tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalProtoToBase64() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && encoded == "" {
				t.Error("MarshalProtoToBase64() returned empty string for valid message")
			}
		})
	}
}

// TestUnmarshalProtoFromBase64 tests the UnmarshalProtoFromBase64 function.
func TestUnmarshalProtoFromBase64(t *testing.T) {
	// Create a test message
	testMsg := &injective_explorer_rpcpb.TxData{
		Hash: "test_hash",
	}

	// Marshal it first
	encoded, err := MarshalProtoToBase64(testMsg)
	assert.NoError(t, err, "Failed to marshal test message")

	// Test unmarshaling
	var unmarshaled injective_explorer_rpcpb.TxData
	err = UnmarshalProtoFromBase64(encoded, &unmarshaled)
	if err != nil {
		t.Errorf("UnmarshalProtoFromBase64() error = %v", err)
		return
	}

	if unmarshaled.Hash != testMsg.Hash {
		t.Errorf("UnmarshalProtoFromBase64() hash = %v, want %v", unmarshaled.Hash, testMsg.Hash)
	}

	// Test with empty string
	err = UnmarshalProtoFromBase64("", &unmarshaled)
	if err == nil {
		t.Error("UnmarshalProtoFromBase64() should return error for empty string")
	}

	// Test with invalid base64
	err = UnmarshalProtoFromBase64("invalid_base64!!!", &unmarshaled)
	if err == nil {
		t.Error("UnmarshalProtoFromBase64() should return error for invalid base64")
	}
}

// TestMarshalProtoSliceToBase64 tests the MarshalProtoSliceToBase64 function.
func TestMarshalProtoSliceToBase64(t *testing.T) {
	tests := []struct {
		name    string
		msgs    []proto.Message
		wantErr bool
	}{
		{
			name:    "nil slice",
			msgs:    nil,
			wantErr: false,
		},
		{
			name:    "empty slice",
			msgs:    []proto.Message{},
			wantErr: false,
		},
		{
			name: "valid messages",
			msgs: []proto.Message{
				&injective_explorer_rpcpb.TxData{Hash: "hash1"},
				&injective_explorer_rpcpb.TxData{Hash: "hash2"},
			},
			wantErr: false,
		},
		{
			name: "slice with nil message",
			msgs: []proto.Message{
				&injective_explorer_rpcpb.TxData{Hash: "hash1"},
				nil,
			},
			wantErr: false, // Should handle nil gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := MarshalProtoSliceToBase64(tt.msgs)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalProtoSliceToBase64() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.msgs != nil && len(encoded) != len(tt.msgs) {
				t.Errorf("MarshalProtoSliceToBase64() length = %v, want %v", len(encoded), len(tt.msgs))
			}
		})
	}
}

// TestRoundTrip tests round-trip conversion (marshal then unmarshal).
func TestRoundTrip(t *testing.T) {
	original := &injective_explorer_rpcpb.TxData{
		Hash: "test_hash_123",
		Code: 0,
	}

	// Marshal
	encoded, err := MarshalProtoToBase64(original)
	assert.NoError(t, err, "MarshalProtoToBase64() error")

	// Unmarshal
	var unmarshaled injective_explorer_rpcpb.TxData
	err = UnmarshalProtoFromBase64(encoded, &unmarshaled)
	assert.NoError(t, err, "UnmarshalProtoFromBase64() error")

	// Compare
	if unmarshaled.Hash != original.Hash {
		t.Errorf("RoundTrip() hash = %v, want %v", unmarshaled.Hash, original.Hash)
	}
	if unmarshaled.Code != original.Code {
		t.Errorf("RoundTrip() code = %v, want %v", unmarshaled.Code, original.Code)
	}
}
