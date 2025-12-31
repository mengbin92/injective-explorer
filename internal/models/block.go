// Package models provides data models for blocks and transactions.
package models

import (
	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// Block represents a blockchain block in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct is designed to store block information from the Injective blockchain explorer,
// including block height, proposer information, hash values, transaction counts, and timestamps.
type Block struct {
	gorm.Model

	Height             uint64   `json:"height" gorm:"column:height"`               // Height is the block height on the blockchain
	Proposer           string   `json:"proposer" gorm:"column:proposer"`                                                  // Proposer is the address of the block proposer
	Moniker            string   `json:"moniker" gorm:"column:moniker"`                                                    // Moniker is the moniker (name) of the block proposer
	BlockHash          string   `json:"block_hash" gorm:"column:block_hash"`                                        // BlockHash is the hash of the current block
	ParentHash         string   `json:"parent_hash" gorm:"column:parent_hash"`                                            // ParentHash is the hash of the parent block
	NumPrecommits      int64    `json:"num_precommits" gorm:"column:num_precommits"`                                      // NumPrecommits is the number of precommits for this block
	NumTxs             int64    `json:"num_txs" gorm:"column:num_txs"`                                                    // NumTxs is the number of transactions in this block
	TotalTxs           int64    `json:"total_txs" gorm:"column:total_txs"`                                                // TotalTxs is the total number of transactions in the blockchain up to this block
	Txs                []string `json:"txs" gorm:"column:txs;serializer:json"`                                            // Txs is a list of transactions in this block (not stored in database)
	Timestamp          string   `json:"timestamp" gorm:"column:timestamp"`                                                // Timestamp is the human-readable timestamp of the block
	BlockUnixTimestamp uint64   `json:"block_unix_timestamp" gorm:"column:block_unix_timestamp"` // BlockUnixTimestamp is the Unix timestamp of the block
}

// BlockFromProto converts a protobuf BlockDetailInfo to a Block model.
//
// Parameters:
//   - detail: The protobuf block detail information (note: parameter name corrected from "deatil")
//
// Returns:
//   - *Block: A new Block instance with data from the protobuf message
func BlockFromProto(detail *injective_explorer_rpcpb.BlockDetailInfo) *Block {
	return BlockFromProtoWithPool(detail, nil)
}

// BlockFromProtoWithPool converts a protobuf BlockDetailInfo to a Block model,
// optionally reusing a Block from the object pool to reduce allocations.
//
// Parameters:
//   - detail: The protobuf block detail information
//   - poolBlock: Optional Block instance from object pool (can be nil)
//
// Returns:
//   - *Block: A Block instance with data from the protobuf message
func BlockFromProtoWithPool(detail *injective_explorer_rpcpb.BlockDetailInfo, poolBlock *Block) *Block {
	if detail == nil {
		return nil
	}

	var block *Block
	if poolBlock != nil {
		// Reuse block from pool
		block = poolBlock
		// Reset to zero values (pool block may contain old data)
		*block = Block{}
	} else {
		// Allocate new block
		block = &Block{}
	}

	// Store Txs as JSON strings for reversible conversion
	// Use shared utility function to eliminate code duplication
	txMessages := make([]proto.Message, 0, len(detail.Txs))
	for _, tx := range detail.Txs {
		txMessages = append(txMessages, tx)
	}
	txs, _ := MarshalProtoSliceToJSON(txMessages)

	// Populate block fields
	block.Height = detail.Height
	// Convert proposer address to Ethereum format for storage
	// Database stores addresses in Ethereum format (0x...) for consistency with browser display
	block.Proposer = ConvertAddressToEthForStorage(detail.Proposer)
	block.Moniker = detail.Moniker
	block.BlockHash = detail.BlockHash
	block.ParentHash = detail.ParentHash
	block.NumPrecommits = detail.NumPreCommits
	block.NumTxs = detail.NumTxs
	block.TotalTxs = detail.TotalTxs
	block.Timestamp = detail.Timestamp
	block.BlockUnixTimestamp = detail.BlockUnixTimestamp
	block.Txs = txs

	return block
}

// BlockToProto converts a Block model back to a protobuf BlockDetailInfo.
//
// Parameters:
//   - block: The Block model from database
//
// Returns:
//   - *injective_explorer_rpcpb.BlockDetailInfo: A new BlockDetailInfo instance with data from the Block model
func BlockToProto(block *Block) *injective_explorer_rpcpb.BlockDetailInfo {
	if block == nil {
		return nil
	}

	// Convert Txs from JSON strings back to protobuf format
	// Use shared utility function to eliminate code duplication
	txs := make([]*injective_explorer_rpcpb.TxData, 0, len(block.Txs))
	for _, txStr := range block.Txs {
		if txStr == "" {
			continue
		}

		var txData injective_explorer_rpcpb.TxData
		if err := UnmarshalProtoFromJSON(txStr, &txData); err == nil {
			txs = append(txs, &txData)
			continue
		}

		// If JSON unmarshal fails, the string might be in old format
		// from old data. We'll create an empty TxData to maintain compatibility
		// Note: This is a fallback for backward compatibility with old data format
		txs = append(txs, &injective_explorer_rpcpb.TxData{})
	}

	// Convert proposer address back to Injective format if needed (for protobuf compatibility)
	// Note: This is only needed if the protobuf expects Injective format
	// For now, we'll use the stored Ethereum format as-is
	return &injective_explorer_rpcpb.BlockDetailInfo{
		Height:             block.Height,
		Proposer:           block.Proposer,
		Moniker:            block.Moniker,
		BlockHash:          block.BlockHash,
		ParentHash:         block.ParentHash,
		NumPreCommits:      block.NumPrecommits,
		NumTxs:             block.NumTxs,
		TotalTxs:           block.TotalTxs,
		Timestamp:          block.Timestamp,
		BlockUnixTimestamp: block.BlockUnixTimestamp,
		Txs:                txs,
	}
}
