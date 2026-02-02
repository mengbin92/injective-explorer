package models

import (
	"encoding/json"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/stretchr/testify/assert"
)

// TestIBCMsgTransferExtractor_CrossChainDetail tests IBC transfer detail extraction
func TestIBCMsgTransferExtractor_CrossChainDetail(t *testing.T) {
	extractor := &ibcMsgTransferExtractor{}

	msg := json.RawMessage(`{
		"sender": "inj1sender...",
		"receiver": "cosmos1receiver...",
		"token": {
			"denom": "inj",
			"amount": "1000000"
		},
		"source_port": "transfer",
		"source_channel": "channel-0",
		"timeout_height": {
			"revision_number": "1",
			"revision_height": "1234567"
		},
		"timeout_timestamp": "1699999999000000000"
	}`)

	res := &ExtractResult{}
	ok := extractor.Extract(msg, res)

	assert.True(t, ok, "extraction should succeed")
	assert.Equal(t, "inj1sender...", res.From)
	assert.Equal(t, "cosmos1receiver...", res.To)
	assert.Len(t, res.Values, 1)
	assert.Equal(t, "inj", res.Values[0].Denom)
	assert.Equal(t, "1000000", res.Values[0].Amount)

	// Check cross-chain details
	assert.NotNil(t, res.IBCTransfer, "IBC transfer detail should be extracted")
	assert.Equal(t, "transfer", res.IBCTransfer.SourcePort)
	assert.Equal(t, "channel-0", res.IBCTransfer.SourceChannel)
	assert.Equal(t, uint64(1699999999000000000), res.IBCTransfer.TimeoutTimestamp)
	assert.NotEmpty(t, res.IBCTransfer.TimeoutHeight)
}

// TestPeggyMsgSendToEthExtractor_CrossChainDetail tests Peggy withdrawal detail extraction
func TestPeggyMsgSendToEthExtractor_CrossChainDetail(t *testing.T) {
	extractor := &injectivePeggyMsgSendToEthExtractor{}

	msg := json.RawMessage(`{
		"sender": "inj1sender...",
		"eth_dest": "0xabc123...",
		"amount": {
			"denom": "peggy0x...",
			"amount": "1000000000000000000"
		},
		"bridge_fee": {
			"denom": "peggy0x...",
			"amount": "1000000000000000"
		}
	}`)

	res := &ExtractResult{}
	ok := extractor.Extract(msg, res)

	assert.True(t, ok, "extraction should succeed")
	assert.Equal(t, "inj1sender...", res.From)
	assert.Equal(t, "0xabc123...", res.To)
	assert.Len(t, res.Values, 1)
	assert.Equal(t, "peggy0x...", res.Values[0].Denom)
	assert.Equal(t, "1000000000000000000", res.Values[0].Amount)

	// Check cross-chain details
	assert.NotNil(t, res.PeggyWithdrawal, "Peggy withdrawal detail should be extracted")
	assert.Equal(t, "1000000000000000", res.PeggyWithdrawal.BridgeFee)
	assert.Equal(t, int32(2), res.PeggyWithdrawal.ClaimType)
}

// TestPeggyMsgDepositClaimExtractor_CrossChainDetail tests Peggy deposit detail extraction
func TestPeggyMsgDepositClaimExtractor_CrossChainDetail(t *testing.T) {
	extractor := &injectivePeggyMsgDepositClaimExtractor{}

	msg := json.RawMessage(`{
		"event_nonce": "42",
		"eth_sender": "0xethsender...",
		"injective_receiver": "inj1receiver...",
		"amount": {
			"denom": "peggy0x...",
			"amount": "500000000000000000"
		}
	}`)

	res := &ExtractResult{}
	ok := extractor.Extract(msg, res)

	assert.True(t, ok, "extraction should succeed")
	assert.Equal(t, "0xethsender...", res.From)
	assert.Equal(t, "inj1receiver...", res.To)
	assert.Len(t, res.Values, 1)
	assert.Equal(t, "peggy0x...", res.Values[0].Denom)
	assert.Equal(t, "500000000000000000", res.Values[0].Amount)

	// Check cross-chain details
	assert.NotNil(t, res.PeggyDeposit, "Peggy deposit detail should be extracted")
	assert.Equal(t, uint64(42), res.PeggyDeposit.EventNonce)
	assert.Equal(t, int32(1), res.PeggyDeposit.ClaimType)
}

// TestExtractIBCInfoFromEvents tests IBC event extraction
func TestExtractIBCInfoFromEvents(t *testing.T) {
	events := []EventRaw{
		{
			Type: "send_packet",
			Attributes: []AttributeRaw{
				{Key: "packet_sequence", Value: "123"},
				{Key: "packet_timeout_height", Value: "1-1000000"},
				{Key: "packet_timeout_timestamp", Value: "1699999999000000000"},
				{Key: "packet_data", Value: `{"receiver":"cosmos1...","amount":"1000"}`},
			},
		},
		{
			Type: "recv_packet",
			Attributes: []AttributeRaw{
				{Key: "packet_dst_port", Value: "transfer"},
				{Key: "packet_dst_channel", Value: "channel-1"},
			},
		},
	}

	detail := &IBCTransferDetail{}
	extractIBCInfoFromEvents(events, detail)

	assert.Equal(t, uint64(123), detail.PacketSequence)
	assert.Equal(t, "1-1000000", detail.TimeoutHeight)
	assert.Equal(t, uint64(1699999999000000000), detail.TimeoutTimestamp)
	assert.Equal(t, "transfer", detail.DestinationPort)
	assert.Equal(t, "channel-1", detail.DestinationChannel)
	assert.NotNil(t, detail.DataHex)
}

// TestExtractPeggyDepositFromEvents tests Peggy deposit event extraction
func TestExtractPeggyDepositFromEvents(t *testing.T) {
	events := []EventRaw{
		{
			Type: "deposit_claim",
			Attributes: []AttributeRaw{
				{Key: "event_nonce", Value: "42"},
				{Key: "event_height", Value: "100000"},
				{Key: "orchestrator_address", Value: "inj1orch..."},
				{Key: "eth_tx_hash", Value: "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"},
			},
		},
	}

	detail := &PeggyDepositDetail{}
	extractPeggyDepositFromEvents(events, detail)

	assert.Equal(t, uint64(42), detail.EventNonce)
	assert.Equal(t, uint64(100000), detail.EventHeight)
	assert.Equal(t, "inj1orch...", detail.OrchestratorAddress)
	assert.Equal(t, "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890", detail.EthTxHash)
}

// TestExtractPeggyWithdrawalFromEvents tests Peggy withdrawal event extraction
func TestExtractPeggyWithdrawalFromEvents(t *testing.T) {
	events := []EventRaw{
		{
			Type: "send_to_eth",
			Attributes: []AttributeRaw{
				{Key: "outgoing_tx_id", Value: "999"},
				{Key: "batch_timeout", Value: "1700000000"},
			},
		},
		{
			Type: "withdrawal_claim",
			Attributes: []AttributeRaw{
				{Key: "event_nonce", Value: "55"},
				{Key: "event_height", Value: "100001"},
				{Key: "batch_nonce", Value: "10"},
				{Key: "orchestrator_address", Value: "inj1orch..."},
				{Key: "eth_tx_hash", Value: "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"},
			},
		},
	}

	detail := &PeggyWithdrawalDetail{}
	extractPeggyWithdrawalFromEvents(events, detail)

	assert.Equal(t, uint64(999), detail.OutgoingTxId)
	assert.Equal(t, uint64(1700000000), detail.BatchTimeout)
	assert.Equal(t, uint64(55), detail.EventNonce)
	assert.Equal(t, uint64(100001), detail.EventHeight)
	assert.Equal(t, uint64(10), detail.BatchNonce)
	assert.Equal(t, "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", detail.EthTxHash)
	assert.Equal(t, "inj1orch...", detail.OrchestratorAddress)
}

// TestParseUint64 tests uint64 parsing
func TestParseUint64(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
		wantErr  bool
	}{
		{"123", 123, false},
		{"0", 0, false},
		{"", 0, false},
		{"9999999999", 9999999999, false},
	}

	for _, tt := range tests {
		result, err := parseUint64(tt.input)
		if tt.wantErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		}
	}
}

// TestStringifyAttrValue tests attribute value stringification
func TestStringifyAttrValue(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected string
	}{
		{"string", "string"},
		{[]byte("bytes"), "\"Ynl0ZXM=\""}, // base64 encoded
		{nil, "null"},
		{123, "123"},
		{[]interface{}{"a", "b"}, "[\"a\",\"b\"]"}, // JSON array
	}

	for _, tt := range tests {
		result := stringifyAttrValue(tt.input)
		assert.Equal(t, tt.expected, result)
	}
}

// TestIBCTransferTx_ToProto tests conversion to proto DTO
func TestIBCTransferTx_ToProto(t *testing.T) {
	tx := &IBCTransferTx{
		Sender:             "inj1sender",
		Receiver:           "cosmos1receiver",
		SourcePort:         "transfer",
		SourceChannel:      "channel-0",
		DestinationPort:    "transfer",
		DestinationChannel: "channel-1",
		Amount:             "1000000",
		Denom:              "inj",
		TimeoutHeight:      "1-1234567",
		TimeoutTimestamp:   1699999999000000000,
		PacketSequence:     42,
		DataHex:            []byte("test"),
		State:              "completed",
		TxHash:             "0xabc123",
	}

	dto := tx.ToProto()

	assert.Equal(t, tx.Sender, dto.Sender)
	assert.Equal(t, tx.Receiver, dto.Receiver)
	assert.Equal(t, tx.SourcePort, dto.SourcePort)
	assert.Equal(t, tx.SourceChannel, dto.SourceChannel)
	assert.Equal(t, tx.DestinationPort, dto.DestinationPort)
	assert.Equal(t, tx.DestinationChannel, dto.DestinationChannel)
	assert.Equal(t, tx.Amount, dto.Amount)
	assert.Equal(t, tx.Denom, dto.Denom)
	assert.Equal(t, tx.TimeoutHeight, dto.TimeoutHeight)
	assert.Equal(t, tx.TimeoutTimestamp, dto.TimeoutTimestamp)
	assert.Equal(t, tx.PacketSequence, dto.PacketSequence)
	assert.Equal(t, tx.State, dto.State)
	assert.Len(t, dto.TxHashes, 1)
	assert.Equal(t, tx.TxHash, dto.TxHashes[0])
}

// TestPeggyDepositTx_ToProto tests conversion to proto DTO
func TestPeggyDepositTx_ToProto(t *testing.T) {
	tx := &PeggyDepositTx{
		Sender:              "0xethsender",
		Receiver:            "inj1receiver",
		Amount:              "500000000000000000",
		Denom:               "peggy0x...",
		EventNonce:          42,
		EventHeight:         100000,
		OrchestratorAddress: "inj1orch",
		State:               "completed",
		ClaimType:           1,
		TxHash:              "0xdef456",
	}

	dto := tx.ToProto()

	assert.Equal(t, tx.Sender, dto.Sender)
	assert.Equal(t, tx.Receiver, dto.Receiver)
	assert.Equal(t, tx.Amount, dto.Amount)
	assert.Equal(t, tx.Denom, dto.Denom)
	assert.Equal(t, tx.EventNonce, dto.EventNonce)
	assert.Equal(t, tx.EventHeight, dto.EventHeight)
	assert.Equal(t, tx.OrchestratorAddress, dto.OrchestratorAddress)
	assert.Equal(t, tx.State, dto.State)
	assert.Equal(t, tx.ClaimType, dto.ClaimType)
	assert.Len(t, dto.TxHashes, 1)
	assert.Equal(t, tx.TxHash, dto.TxHashes[0])
}

// TestPeggyWithdrawalTx_ToProto tests conversion to proto DTO
func TestPeggyWithdrawalTx_ToProto(t *testing.T) {
	tx := &PeggyWithdrawalTx{
		Sender:              "inj1sender",
		Receiver:            "0xethreceiver",
		Amount:              "1000000000000000000",
		Denom:               "peggy0x...",
		BridgeFee:           "1000000000000000",
		OutgoingTxId:        999,
		BatchTimeout:        1700000000,
		BatchNonce:          10,
		OrchestratorAddress: "inj1orch",
		EventNonce:          55,
		EventHeight:         100001,
		State:               "completed",
		ClaimType:           2,
		TxHash:              "0xghi789",
	}

	dto := tx.ToProto()

	assert.Equal(t, tx.Sender, dto.Sender)
	assert.Equal(t, tx.Receiver, dto.Receiver)
	assert.Equal(t, tx.Amount, dto.Amount)
	assert.Equal(t, tx.Denom, dto.Denom)
	assert.Equal(t, tx.BridgeFee, dto.BridgeFee)
	assert.Equal(t, tx.OutgoingTxId, dto.OutgoingTxId)
	assert.Equal(t, tx.BatchTimeout, dto.BatchTimeout)
	assert.Equal(t, tx.BatchNonce, dto.BatchNonce)
	assert.Equal(t, tx.OrchestratorAddress, dto.OrchestratorAddress)
	assert.Equal(t, tx.EventNonce, dto.EventNonce)
	assert.Equal(t, tx.EventHeight, dto.EventHeight)
	assert.Equal(t, tx.State, dto.State)
	assert.Equal(t, tx.ClaimType, dto.ClaimType)
	assert.Len(t, dto.TxHashes, 1)
	assert.Equal(t, tx.TxHash, dto.TxHashes[0])
}

// TestExtractDestinationFromPacketData tests packet data parsing
func TestExtractDestinationFromPacketData(t *testing.T) {
	packetData := `{"receiver":"cosmos1receiver","amount":"1000","denom":"inj"}`
	detail := &IBCTransferDetail{}
	extractDestinationFromPacketData(packetData, detail)
	// Currently doesn't extract anything, but should not panic
}

// BenchmarkIBCExtractor benchmarks IBC extractor performance
func BenchmarkIBCExtractor(b *testing.B) {
	extractor := &ibcMsgTransferExtractor{}
	msg := json.RawMessage(`{
		"sender": "inj1sender...",
		"receiver": "cosmos1receiver...",
		"token": {"denom": "inj", "amount": "1000000"},
		"source_port": "transfer",
		"source_channel": "channel-0"
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := &ExtractResult{}
		extractor.Extract(msg, res)
	}
}

// BenchmarkToProto benchmarks DTO conversion performance
func BenchmarkIBCTransferTx_ToProto(b *testing.B) {
	tx := &IBCTransferTx{
		Sender:        "inj1sender",
		Receiver:      "cosmos1receiver",
		SourcePort:    "transfer",
		SourceChannel: "channel-0",
		Amount:        "1000000",
		Denom:         "inj",
		State:         "completed",
		TxHash:        "0xabc123",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx.ToProto()
	}
}

// Helper function to parse JSON safely
func mustMarshal(v interface{}) []byte {
	data, err := sonic.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
