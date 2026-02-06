package models

import (
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/converter"
	"injective-explorer/internal/global"

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
	EventNonce          uint64 `gorm:"column:event_nonce;uniqueIndex"`
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
// Converts Bech32 addresses to EVM format (0x...) and normalizes to lowercase for consistent display.
func (t *PeggyDepositTx) ToProto() *pb.PeggyDepositTxDTO {
	// Convert receiver address from Bech32 to EVM format
	receiverEVM := t.Receiver
	if converted, err := converter.InjectiveToEthAddress(t.Receiver); err == nil {
		receiverEVM = converted
	} else {
		// Log conversion failure for debugging, but continue with original address
		global.Logger.Debugf("Failed to convert receiver address %s to EVM format: %v", t.Receiver, err)
	}

	// Convert orchestrator address from Bech32 to EVM format
	orchestratorEVM := t.OrchestratorAddress
	if t.OrchestratorAddress != "" {
		if converted, err := converter.InjectiveToEthAddress(t.OrchestratorAddress); err == nil {
			orchestratorEVM = converted
		} else {
			// Log conversion failure for debugging, but continue with original address
			global.Logger.Debugf("Failed to convert orchestrator address %s to EVM format: %v", t.OrchestratorAddress, err)
		}
	}

	// Normalize all addresses and hashes to lowercase with 0x prefix (EVM convention)
	return &pb.PeggyDepositTxDTO{
		Sender:              converter.NormalizeHash(t.Sender),
		Receiver:            converter.NormalizeHash(receiverEVM),
		EventNonce:          t.EventNonce,
		EventHeight:         t.EventHeight,
		Amount:              t.Amount,
		Denom:               t.Denom,
		OrchestratorAddress: converter.NormalizeHash(orchestratorEVM),
		State:               t.State,
		ClaimType:           t.ClaimType,
		TxHashes:            []string{converter.NormalizeHash(t.TxHash)},
		CreatedAt:           t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           t.UpdatedAt.Format(time.RFC3339),
		EthTxHash:           converter.NormalizeHash(t.EthTxHash),
	}
}

