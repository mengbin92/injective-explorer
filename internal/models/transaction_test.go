package models

import (
	"testing"

	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionFromProto_Basic(t *testing.T) {
	detail := &injective_explorer_rpcpb.TxDetailData{
		Id:          "test-id",
		Hash:        "0x1234567890abcdef",
		BlockNumber: 1000,
		Code:        0,
		Data:        []byte("test data"),
		Messages:    []byte("test messages"),
		Logs:        []byte("test logs"),
		Events: []*injective_explorer_rpcpb.Event{
			{
				Type: "transfer",
				Attributes: map[string]string{
					"sender":    "inj1sender",
					"recipient": "inj1recipient",
					"amount":    "100inj",
				},
			},
		},
		Signatures: []*injective_explorer_rpcpb.Signature{
			{Address: "inj1signer"},
		},
	}

	tx := TransactionFromProto(detail, 0)

	require.NotNil(t, tx)
	assert.Equal(t, "test-id", tx.Id)
	assert.Equal(t, "0x1234567890abcdef", tx.Hash)
	assert.Equal(t, uint64(1000), tx.BlockNumber)
	assert.Equal(t, "inj1signer", tx.Signer)
	assert.Equal(t, "inj1recipient", tx.To)
	assert.Equal(t, []string{"100inj"}, tx.Values)
}

func TestTransactionFromProto_NilInput(t *testing.T) {
	tx := TransactionFromProto(nil, 0)
	assert.Nil(t, tx)
}

func TestTransactionFromProto_ExtractToAndValue(t *testing.T) {
	detail := &injective_explorer_rpcpb.TxDetailData{
		Hash: "0xtest",
		Events: []*injective_explorer_rpcpb.Event{
			{
				Type: "transfer",
				Attributes: map[string]string{
					"sender":    "inj1from",
					"recipient": "inj1to",
					"amount":    "500inj",
				},
			},
		},
		Signatures: []*injective_explorer_rpcpb.Signature{
			{Address: "inj1from"},
		},
	}

	tx := TransactionFromProto(detail, 0)

	require.NotNil(t, tx)
	assert.Equal(t, "inj1to", tx.To)
	assert.Equal(t, []string{"500inj"}, tx.Values)
}

func TestTransactionFromProto_Compression(t *testing.T) {
	// Create large data that should be compressed
	largeData := make([]byte, 2048) // 2KB
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	detail := &injective_explorer_rpcpb.TxDetailData{
		Hash: "0xtest",
		Data: largeData,
	}

	tx := TransactionFromProto(detail, 0)

	require.NotNil(t, tx)
	// Data should be compressed (starts with CompressionPrefix)
	assert.Contains(t, tx.Data, CompressionPrefix)
}

func TestTransactionToProto_Basic(t *testing.T) {
	tx := &Transaction{
		Id:          "test-id",
		Hash:        "0x1234567890abcdef",
		BlockNumber: 1000,
		Code:        0,
		Data:        "test data",
		Messages:    "test messages",
		Logs:        "test logs",
		To:          "inj1recipient",
		Values:      []string{"100inj"},
		Signer:      "inj1signer",
	}

	detail := TransactionToProto(tx)

	require.NotNil(t, detail)
	assert.Equal(t, "test-id", detail.Id)
	assert.Equal(t, "0x1234567890abcdef", detail.Hash)
	assert.Equal(t, uint64(1000), detail.BlockNumber)
	assert.Equal(t, []byte("test data"), detail.Data)
	assert.Equal(t, []byte("test messages"), detail.Messages)
	assert.Equal(t, []byte("test logs"), detail.Logs)
}

func TestTransactionToProto_NilInput(t *testing.T) {
	detail := TransactionToProto(nil)
	assert.Nil(t, detail)
}

func TestTransactionToProto_CompressedData(t *testing.T) {
	// Create compressed data
	originalData := []byte("test data for compression")
	compressed, err := CompressData(originalData)
	require.NoError(t, err)

	tx := &Transaction{
		Hash: "0xtest",
		Data: compressed,
	}

	detail := TransactionToProto(tx)

	require.NotNil(t, detail)
	// Should decompress correctly
	assert.Equal(t, originalData, detail.Data)
}

func TestTransactionToProto_WithStorageKeys(t *testing.T) {
	// This test would require mocking the storage client
	// For now, we test the fallback path when storage key exists but storage is disabled
	tx := &Transaction{
		Hash:           "0xtest",
		DataStorageKey: "data/test-key",
		Data:           "fallback data", // Fallback data in database
	}

	detail := TransactionToProto(tx)

	require.NotNil(t, detail)
	// Should use fallback data when storage is not available
	// Note: This assumes NoOpStorage is used (storage disabled)
	assert.NotNil(t, detail.Data)
}

func TestTransactionFromProto_ToValueExtraction(t *testing.T) {
	testCases := []struct {
		name      string
		detail    *injective_explorer_rpcpb.TxDetailData
		wantTo    string
		wantValue []string
	}{
		{
			name: "transfer event",
			detail: &injective_explorer_rpcpb.TxDetailData{
				Events: []*injective_explorer_rpcpb.Event{
					{
						Type: "transfer",
						Attributes: map[string]string{
							"recipient": "inj1to",
							"amount":    "100inj",
						},
					},
				},
			},
			wantTo:    "inj1to",
			wantValue: []string{"100inj"},
		},
		{
			name: "coin_received event",
			detail: &injective_explorer_rpcpb.TxDetailData{
				Events: []*injective_explorer_rpcpb.Event{
					{
						Type: "coin_received",
						Attributes: map[string]string{
							"receiver": "inj1receiver",
							"amount":   "200inj",
						},
					},
				},
				Signatures: []*injective_explorer_rpcpb.Signature{
					{Address: "inj1sender"},
				},
			},
			wantTo:    "inj1receiver",
			wantValue: []string{"200inj"},
		},
		{
			name: "no transfer events",
			detail: &injective_explorer_rpcpb.TxDetailData{
				Events: []*injective_explorer_rpcpb.Event{
					{
						Type: "message",
						Attributes: map[string]string{
							"action": "send",
						},
					},
				},
			},
			wantTo:    "",
			wantValue: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tx := TransactionFromProto(tc.detail, 0)
			require.NotNil(t, tx)
			assert.Equal(t, tc.wantTo, tx.To)
			assert.Equal(t, tc.wantValue, tx.Values)
		})
	}
}

func TestTransactionFromProto_SignerExtraction(t *testing.T) {
	testCases := []struct {
		name       string
		detail     *injective_explorer_rpcpb.TxDetailData
		wantSigner string
	}{
		{
			name: "from signature",
			detail: &injective_explorer_rpcpb.TxDetailData{
				Signatures: []*injective_explorer_rpcpb.Signature{
					{Address: "inj1signer"},
				},
			},
			wantSigner: "inj1signer",
		},
		{
			name: "from gasFee.payer",
			detail: &injective_explorer_rpcpb.TxDetailData{
				GasFee: &injective_explorer_rpcpb.GasFee{
					Payer: "inj1payer",
				},
			},
			wantSigner: "inj1payer",
		},
		{
			name: "signature overrides gasFee",
			detail: &injective_explorer_rpcpb.TxDetailData{
				Signatures: []*injective_explorer_rpcpb.Signature{
					{Address: "inj1signer"},
				},
				GasFee: &injective_explorer_rpcpb.GasFee{
					Payer: "inj1payer",
				},
			},
			wantSigner: "inj1signer",
		},
		{
			name: "no signer",
			detail: &injective_explorer_rpcpb.TxDetailData{
				Hash: "0xtest",
			},
			wantSigner: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tx := TransactionFromProto(tc.detail, 0)
			require.NotNil(t, tx)
			assert.Equal(t, tc.wantSigner, tx.Signer)
		})
	}
}
