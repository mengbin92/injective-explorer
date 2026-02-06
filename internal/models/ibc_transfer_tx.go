package models

import (
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/converter"
	"injective-explorer/internal/global"

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
// Converts Bech32 addresses to EVM format (0x...) and normalizes to lowercase for consistent display.
func (t *IBCTransferTx) ToProto() *pb.IBCTransferTxDTO {
	// Convert sender address from Bech32 to EVM format if it's a Injective address
	senderEVM := t.Sender
	if converted, err := converter.InjectiveToEthAddress(t.Sender); err == nil {
		senderEVM = converted
	} else {
		// Log conversion failure for debugging, but continue with original address
		global.Logger.Debugf("Failed to convert sender address %s to EVM format: %v", t.Sender, err)
	}

	// Convert receiver address from Bech32 to EVM format if it's a Injective address
	// Note: For IBC transfers to other chains, receiver might be in a different format
	receiverEVM := t.Receiver
	if converted, err := converter.InjectiveToEthAddress(t.Receiver); err == nil {
		receiverEVM = converted
	} else {
		// Log conversion failure for debugging, but continue with original address
		global.Logger.Debugf("Failed to convert receiver address %s to EVM format: %v", t.Receiver, err)
	}

	// Normalize all addresses and hashes to lowercase with 0x prefix (EVM convention)
	return &pb.IBCTransferTxDTO{
		Sender:             converter.NormalizeHash(senderEVM),
		Receiver:           converter.NormalizeHash(receiverEVM),
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
		TxHashes:           []string{converter.NormalizeHash(t.TxHash)},
		CreatedAt:          t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          t.UpdatedAt.Format(time.RFC3339),
	}
}

