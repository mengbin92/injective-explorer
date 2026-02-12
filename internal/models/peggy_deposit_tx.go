package models

import (
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/converter"

	"gorm.io/gorm"
)

// PeggyDepositTx represents a Peggy deposit transaction (Ethereum → Injective).
type PeggyDepositTx struct {
	ID                  uint64         `gorm:"primaryKey;column:id"`
	CreatedAt           time.Time      `gorm:"column:created_at"`
	UpdatedAt           time.Time      `gorm:"column:updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"column:deleted_at;index"`

	// Core fields
	Sender              string `gorm:"column:sender;size:128;not null;index"`       // Ethereum 0x...
	Receiver            string `gorm:"column:receiver;size:128;not null;index"`     // Injective address
	Amount              string `gorm:"column:amount;size:100"`
	Denom               string `gorm:"column:denom;size:100"`
	EventNonce          uint64 `gorm:"column:event_nonce;index"`                                // Changed to regular index
	EventHeight         uint64 `gorm:"column:event_height"`
	OrchestratorAddress string `gorm:"column:orchestrator_address;size:128"`
	State               string `gorm:"column:state;size:50;default:'completed'"`
	ClaimType           int32  `gorm:"column:claim_type;default:1"` // DepositClaim = 1

	// Block reference fields
	TxHash         string `gorm:"column:tx_hash;size:128;not null;index"`
	BlockHeight    uint64 `gorm:"column:block_height;index"`
	BlockTimestamp uint64 `gorm:"column:block_timestamp;index"`
	EthTxHash      string `gorm:"column:eth_tx_hash;size:128"` // Original Ethereum tx
}

// TableName returns the table name for GORM.
func (PeggyDepositTx) TableName() string {
	return "peggy_deposit_txs"
}

// ToProto converts PeggyDepositTx to protobuf DTO for API responses.
// Detects and converts addresses and hashes to EVM format for consistent API responses.
func (t *PeggyDepositTx) ToProto() *pb.PeggyDepositTxDTO {
	// Convert sender to EVM format if needed (should already be EVM format, but check for backward compatibility)
	senderEVM := t.Sender
	if evmAddr, err := converter.InjectiveToEthAddress(t.Sender); err == nil {
		senderEVM = evmAddr
	}
	senderEVM = converter.NormalizeHash(senderEVM)

	// Convert receiver to EVM format if needed
	receiverEVM := t.Receiver
	if evmAddr, err := converter.InjectiveToEthAddress(t.Receiver); err == nil {
		receiverEVM = evmAddr
	}
	receiverEVM = converter.NormalizeHash(receiverEVM)

	// Convert orchestrator address to EVM format if needed
	orchestratorEVM := t.OrchestratorAddress
	if t.OrchestratorAddress != "" {
		if evmAddr, err := converter.InjectiveToEthAddress(t.OrchestratorAddress); err == nil {
			orchestratorEVM = evmAddr
		}
		orchestratorEVM = converter.NormalizeHash(orchestratorEVM)
	}

	// Normalize transaction hash to EVM format (0x + lowercase)
	txHash := converter.NormalizeHash(t.TxHash)
	ethTxHash := converter.NormalizeHash(t.EthTxHash)

	return &pb.PeggyDepositTxDTO{
		Sender:              senderEVM,
		Receiver:            receiverEVM,
		EventNonce:          t.EventNonce,
		EventHeight:         t.EventHeight,
		Amount:              t.Amount,
		Denom:               t.Denom,
		OrchestratorAddress: orchestratorEVM,
		State:               t.State,
		ClaimType:           t.ClaimType,
		TxHashes:            []string{txHash},
		CreatedAt:           t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           t.UpdatedAt.Format(time.RFC3339),
		EthTxHash:           ethTxHash,
	}
}

