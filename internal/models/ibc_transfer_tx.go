package models

import (
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/converter"

	"gorm.io/gorm"
)

// IBCTransferTx represents an IBC transfer transaction in the database.
type IBCTransferTx struct {
	ID                  uint64         `gorm:"primaryKey;column:id"`
	CreatedAt           time.Time      `gorm:"column:created_at"`
	UpdatedAt           time.Time      `gorm:"column:updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"column:deleted_at;index"`

	// Core fields
	Sender              string `gorm:"column:sender;size:128;not null;index"`
	Receiver            string `gorm:"column:receiver;size:128;not null;index"`
	SourcePort          string `gorm:"column:source_port;size:100"`
	SourceChannel       string `gorm:"column:source_channel;size:100;index"`
	DestinationPort     string `gorm:"column:destination_port;size:100"`
	DestinationChannel  string `gorm:"column:destination_channel;size:100;index"`
	Amount              string `gorm:"column:amount;size:100"`
	Denom               string `gorm:"column:denom;size:100"`
	TimeoutHeight       string `gorm:"column:timeout_height;size:100"`
	TimeoutTimestamp    uint64 `gorm:"column:timeout_timestamp"`
	PacketSequence      uint64 `gorm:"column:packet_sequence"`
	DataHex             []byte `gorm:"column:data_hex;type:blob"`
	State               string `gorm:"column:state;size:50;default:'completed'"`

	// Block reference fields
	TxHash         string `gorm:"column:tx_hash;size:128;not null;index"`
	BlockHeight    uint64 `gorm:"column:block_height;index"`
	BlockTimestamp uint64 `gorm:"column:block_timestamp;index"`
}

// TableName returns the table name for GORM.
func (IBCTransferTx) TableName() string {
	return "ibc_transfer_txs"
}

// ToProto converts IBCTransferTx to protobuf DTO for API responses.
// Detects and converts addresses and hashes to EVM format for consistent API responses.
func (t *IBCTransferTx) ToProto() *pb.IBCTransferTxDTO {
	// Convert sender to EVM format if it's a Injective address
	senderEVM := t.Sender
	if evmAddr, err := converter.InjectiveToEthAddress(t.Sender); err == nil {
		senderEVM = evmAddr
	}
	// For non-Injective addresses (other chains), just normalize if it looks like an EVM address
	senderEVM = converter.NormalizeHash(senderEVM)

	// Convert receiver to EVM format if it's a Injective address
	receiverEVM := t.Receiver
	if evmAddr, err := converter.InjectiveToEthAddress(t.Receiver); err == nil {
		receiverEVM = evmAddr
	}
	// For non-Injective addresses (other chains), just normalize if it looks like an EVM address
	receiverEVM = converter.NormalizeHash(receiverEVM)

	// Normalize transaction hash to EVM format (0x + lowercase)
	txHash := converter.NormalizeHash(t.TxHash)

	return &pb.IBCTransferTxDTO{
		Sender:             senderEVM,
		Receiver:           receiverEVM,
		SourcePort:         t.SourcePort,
		SourceChannel:      t.SourceChannel,
		DestinationPort:    t.DestinationPort,
		DestinationChannel: t.DestinationChannel,
		Amount:             t.Amount,
		Denom:              t.Denom,
		TimeoutHeight:      t.TimeoutHeight,
		TimeoutTimestamp:   t.TimeoutTimestamp,
		PacketSequence:     t.PacketSequence,
		DataHex:            t.DataHex,
		State:              t.State,
		TxHashes:           []string{txHash},
		CreatedAt:          t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          t.UpdatedAt.Format(time.RFC3339),
	}
}

