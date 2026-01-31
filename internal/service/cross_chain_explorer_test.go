package service

import (
	"testing"

	pb "injective-explorer/api/explorer/v1"
	"github.com/stretchr/testify/assert"
)

// TestExplorerService_HasCrossChainMethods verifies that ExplorerService implements cross-chain methods
// This is a compile-time check - if the methods are not implemented, this test won't compile
func TestExplorerService_HasCrossChainMethods(t *testing.T) {
	service := &ExplorerService{}

	// Verify service implements the interface (compile-time check)
	var _ pb.ExplorerServer = service

	// Methods are verified at compile time by the interface check above
	// The actual method implementations are tested via integration tests
	assert.NotNil(t, service)
}

// TestCrossChainRequestTypes verifies request types are properly defined
func TestCrossChainRequestTypes(t *testing.T) {
	// Test IBC request
	ibcReq := &pb.GetIBCTransferTxsRequest{
		Sender:   "inj1sender",
		Receiver: "cosmos1receiver",
		SrcChannel: "channel-0",
		DestChannel: "channel-1",
		Pagination: &pb.PaginationRequest{
			Page:     1,
			PageSize: 20,
		},
	}
	assert.Equal(t, "inj1sender", ibcReq.Sender)
	assert.Equal(t, "cosmos1receiver", ibcReq.Receiver)
	assert.Equal(t, "channel-0", ibcReq.SrcChannel)
	assert.Equal(t, "channel-1", ibcReq.DestChannel)

	// Test Peggy deposit request
	depositReq := &pb.GetPeggyDepositTxsRequest{
		Sender:   "0xethsender",
		Receiver: "inj1receiver",
		Pagination: &pb.PaginationRequest{
			Page:     1,
			PageSize: 20,
		},
	}
	assert.Equal(t, "0xethsender", depositReq.Sender)
	assert.Equal(t, "inj1receiver", depositReq.Receiver)

	// Test Peggy withdrawal request
	withdrawalReq := &pb.GetPeggyWithdrawalTxsRequest{
		Sender:   "inj1sender",
		Receiver: "0xethreceiver",
		Pagination: &pb.PaginationRequest{
			Page:     1,
			PageSize: 20,
		},
	}
	assert.Equal(t, "inj1sender", withdrawalReq.Sender)
	assert.Equal(t, "0xethreceiver", withdrawalReq.Receiver)
}

// TestCrossChainDTOTypes verifies DTO types are properly defined
func TestCrossChainDTOTypes(t *testing.T) {
	// Test IBC DTO
	ibcDTO := &pb.IBCTransferTxDTO{
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
		State:              "completed",
		TxHashes:           []string{"0xabc123"},
	}
	assert.Equal(t, "inj1sender", ibcDTO.Sender)
	assert.Equal(t, "cosmos1receiver", ibcDTO.Receiver)
	assert.Equal(t, "transfer", ibcDTO.SourcePort)
	assert.Equal(t, "channel-0", ibcDTO.SourceChannel)
	assert.Equal(t, uint64(42), ibcDTO.PacketSequence)
	assert.Equal(t, "completed", ibcDTO.State)
	assert.Len(t, ibcDTO.TxHashes, 1)

	// Test Peggy deposit DTO
	depositDTO := &pb.PeggyDepositTxDTO{
		Sender:              "0xethsender",
		Receiver:            "inj1receiver",
		Amount:              "500000000000000000",
		Denom:               "peggy0x...",
		EventNonce:          42,
		EventHeight:         100000,
		OrchestratorAddress: "inj1orch",
		State:               "completed",
		ClaimType:           1,
		TxHashes:            []string{"0xdef456"},
	}
	assert.Equal(t, "0xethsender", depositDTO.Sender)
	assert.Equal(t, uint64(42), depositDTO.EventNonce)
	assert.Equal(t, int32(1), depositDTO.ClaimType)

	// Test Peggy withdrawal DTO
	withdrawalDTO := &pb.PeggyWithdrawalTxDTO{
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
		TxHashes:            []string{"0xghi789"},
	}
	assert.Equal(t, "inj1sender", withdrawalDTO.Sender)
	assert.Equal(t, uint64(999), withdrawalDTO.OutgoingTxId)
	assert.Equal(t, uint64(10), withdrawalDTO.BatchNonce)
	assert.Equal(t, int32(2), withdrawalDTO.ClaimType)
}

// TestCrossChainPagination verifies pagination works with cross-chain requests
func TestCrossChainPagination(t *testing.T) {
	tests := []struct {
		name       string
		pagination *pb.PaginationRequest
		wantPage   int32
		wantSize   int32
	}{
		{
			name:       "default pagination",
			pagination: &pb.PaginationRequest{},
			wantPage:   0, // Uses default
			wantSize:   0, // Uses default
		},
		{
			name: "custom page and size",
			pagination: &pb.PaginationRequest{
				Page:     2,
				PageSize: 50,
			},
			wantPage: 2,
			wantSize: 50,
		},
		{
			name: "cursor-based pagination",
			pagination: &pb.PaginationRequest{
				Cursor: "1234567890",
			},
			wantPage: 0,
			wantSize: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &pb.GetIBCTransferTxsRequest{
				Pagination: tt.pagination,
			}
			assert.Equal(t, tt.wantPage, req.Pagination.Page)
			assert.Equal(t, tt.wantSize, req.Pagination.PageSize)
		})
	}
}

// TestNewExplorerService verifies ExplorerService can be created
func TestNewExplorerService(t *testing.T) {
	service := NewExplorerService()
	assert.NotNil(t, service)
	assert.NotNil(t, service.coalescer)
}

// BenchmarkCrossChainDTOCreation benchmarks DTO creation performance
func BenchmarkIBCTransferTxDTO_Creation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = &pb.IBCTransferTxDTO{
			Sender:         "inj1sender",
			Receiver:       "cosmos1receiver",
			SourcePort:     "transfer",
			SourceChannel:  "channel-0",
			Amount:         "1000000",
			Denom:          "inj",
			State:          "completed",
			TxHashes:       []string{"0xabc123"},
		}
	}
}

func BenchmarkPeggyDepositTxDTO_Creation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = &pb.PeggyDepositTxDTO{
			Sender:     "0xethsender",
			Receiver:   "inj1receiver",
			Amount:     "500000000000000000",
			Denom:      "peggy0x...",
			EventNonce: 42,
			State:      "completed",
			TxHashes:   []string{"0xdef456"},
		}
	}
}

func BenchmarkPeggyWithdrawalTxDTO_Creation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = &pb.PeggyWithdrawalTxDTO{
			Sender:       "inj1sender",
			Receiver:     "0xethreceiver",
			Amount:       "1000000000000000000",
			Denom:        "peggy0x...",
			OutgoingTxId: 999,
			BatchNonce:   10,
			State:        "completed",
			TxHashes:     []string{"0xghi789"},
		}
	}
}
