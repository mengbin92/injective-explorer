// Package models provides data models for Wasm code storage.
package models

import (
	"gorm.io/gorm"
)

// WasmCode represents uploaded Wasm code in the database.
// It stores information about uploaded Wasm bytecode, including creator,
// hash, instantiation permissions, and verification status.
type WasmCode struct {
	gorm.Model

	// Code identification
	CodeID                 int64  `json:"code_id" gorm:"column:code_id;not null;uniqueIndex:idx_wasm_codes_code_id"`
	Creator                string `json:"creator" gorm:"column:creator;size:128;not null;index:idx_wasm_codes_creator"`
	DataHash               string `json:"data_hash" gorm:"column:data_hash;size:128;not null"`
	InstantiatePermission  string `json:"instantiate_permission" gorm:"column:instantiate_permission;type:text"`

	// Transaction info
	TxHash      *string `json:"tx_hash" gorm:"column:tx_hash;size:128"`
	BlockHeight int64   `json:"block_height" gorm:"column:block_height;not null;index:idx_wasm_codes_block_height"`

	// Contract metadata
	ContractType string  `json:"contract_type" gorm:"column:contract_type;size:100"`
	Version      *string `json:"version" gorm:"column:version;size:50"`
	SourceURL    *string `json:"source_url" gorm:"column:source_url;type:text"`

	// Verification
	Verified         bool  `json:"verified" gorm:"column:verified;default:0"`
	InstantiateCount int64 `json:"instantiate_count" gorm:"column:instantiate_count;default:0"`
}

// TableName returns the table name for WasmCode model.
func (WasmCode) TableName() string {
	return "wasm_codes"
}
