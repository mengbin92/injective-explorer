package models

import (
	"time"

	pb "injective-explorer/api/explorer/v1"

	"gorm.io/gorm"
)

// PeggyWithdrawalTx represents a Peggy withdrawal transaction (Injective → Ethereum).
type PeggyWithdrawalTx struct {
	ID        uint64         `gorm:"primaryKey;column:id"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`

	// Core fields
	Sender              string `gorm:"column:sender;size:128;not null;index"`   // Injective
	Receiver            string `gorm:"column:receiver;size:128;not null;index"` // Ethereum 0x...
	Amount              string `gorm:"column:amount;size:100"`
	Denom               string `gorm:"column:denom;size:100"`
	BridgeFee           string `gorm:"column:bridge_fee;size:100"`
	OutgoingTxId        uint64 `gorm:"column:outgoing_tx_id;uniqueIndex"`
	BatchTimeout        uint64 `gorm:"column:batch_timeout"`
	BatchNonce          uint64 `gorm:"column:batch_nonce"`
	OrchestratorAddress string `gorm:"column:orchestrator_address;size:128"`
	EventNonce          uint64 `gorm:"column:event_nonce"`
	EventHeight         uint64 `gorm:"column:event_height"`
	State               string `gorm:"column:state;size:50;default:'pending'"` // pending, batched, completed, failed
	ClaimType           int32  `gorm:"column:claim_type;default:2"`            // WithdrawalClaim = 2

	// Block reference fields
	TxHash         string `gorm:"column:tx_hash;size:128;not null;index"`
	BlockHeight    uint64 `gorm:"column:block_height;index"`
	BlockTimestamp uint64 `gorm:"column:block_timestamp;index"`
	EthTxHash      string `gorm:"column:eth_tx_hash;size:128"` // Ethereum tx when completed
}

// TableName returns the table name for GORM.
func (PeggyWithdrawalTx) TableName() string {
	return "peggy_withdrawal_txs"
}

// ToProto converts PeggyWithdrawalTx to protobuf DTO for API responses.
// Converts Bech32 addresses to EVM format (0x...) and normalizes to lowercase for consistent display.
func (t *PeggyWithdrawalTx) ToProto() *pb.PeggyWithdrawalTxDTO {
	// Convert sender address from Bech32 to EVM format (Injective → Ethereum withdrawal)
	senderEVM := t.Sender
	if converted, err := InjectiveToEthAddress(t.Sender); err == nil {
		senderEVM = converted
	}

	// Convert orchestrator address from Bech32 to EVM format
	orchestratorEVM := t.OrchestratorAddress
	if t.OrchestratorAddress != "" {
		if converted, err := InjectiveToEthAddress(t.OrchestratorAddress); err == nil {
			orchestratorEVM = converted
		}
	}

	// Normalize all addresses and hashes to lowercase with 0x prefix (EVM convention)
	return &pb.PeggyWithdrawalTxDTO{
		Sender:              normalizeHash(senderEVM),
		Receiver:            normalizeHash(t.Receiver), // Already in EVM format
		Amount:              t.Amount,
		Denom:               t.Denom,
		BridgeFee:           t.BridgeFee,
		OutgoingTxId:        t.OutgoingTxId,
		BatchTimeout:        t.BatchTimeout,
		BatchNonce:          t.BatchNonce,
		OrchestratorAddress: normalizeHash(orchestratorEVM),
		EventNonce:          t.EventNonce,
		EventHeight:         t.EventHeight,
		State:               t.State,
		ClaimType:           t.ClaimType,
		TxHashes:            []string{normalizeHash(t.TxHash)},
		CreatedAt:           t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           t.UpdatedAt.Format(time.RFC3339),
		EthTxHash:           normalizeHash(t.EthTxHash),
	}
}

