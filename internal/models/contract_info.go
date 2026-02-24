// Package models provides data models for blockchain entities.
package models

import (
	"time"

	"gorm.io/gorm"
)

// ContractInfo represents contract information in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores contract information including contract address, name, contract type,
// chain ID, and verification status. It supports various contract types (token contracts,
// NFT contracts, etc.), with token contracts being the primary use case currently.
// TokenName and TokenSymbol are optional fields specifically for token contracts.
//
// Note: The table name is "token_contracts" for historical reasons, but this model
// is designed to support all contract types, not just token contracts.
type ContractInfo struct {
	gorm.Model

	// Basic information
	ContractAddress string `json:"contract_address" gorm:"column:contract_address;size:128;not null"`
	Name            string `json:"name" gorm:"column:name;size:255"`                      // Generic contract name (optional)
	TokenName       string `json:"token_name" gorm:"column:token_name;size:255"`           // Token name (optional, for token contracts)
	TokenSymbol     string `json:"token_symbol" gorm:"column:token_symbol;size:50"`        // Token symbol (optional, for token contracts)
	ContractType    string `json:"contract_type" gorm:"column:contract_type;size:50"`
	ChainID         string `json:"chain_id" gorm:"column:chain_id;size:50"`

	// Verification related (reserved for future extension)
	VerificationStatus string `json:"verification_status" gorm:"column:verification_status;size:50"`
	IsVerified         bool `json:"is_verified" gorm:"column:is_verified"`
	VerifiedAt         *time.Time `json:"verified_at" gorm:"column:verified_at"`
	VerifiedBy         string     `json:"verified_by" gorm:"column:verified_by;size:128"`

	// Creator/Deployer information
	Creator string `json:"creator" gorm:"column:creator;size:128"` // Contract deployer/creator address

	// Token decimals for ERC20 tokens (default 18)
	Decimals int `json:"decimals" gorm:"column:decimals;default:18"`

	// ERC20 metadata sync fields
	MetadataNotApplicable bool       `json:"metadata_not_applicable" gorm:"column:metadata_not_applicable;default:false"` // 是否非 ERC20 合约
	Version              int        `json:"version" gorm:"column:version;default:0"`                                   // 乐观锁版本
	MetadataUpdatedAt    *time.Time `json:"metadata_updated_at" gorm:"column:metadata_updated_at"`                      // 元数据最后更新时间

	// Peggy bridge token fields
	IsPeggyToken        bool   `json:"is_peggy_token" gorm:"column:is_peggy_token;default:false"`
	PeggyDenom          string `json:"peggy_denom" gorm:"column:peggy_denom;size:255"`
	PeggyOriginalAddress string `json:"peggy_original_address" gorm:"column:peggy_original_address;size:42"`
	MetadataSource      string `json:"metadata_source" gorm:"column:metadata_source;size:50"` // evm/ethereum/coingecko/manual

	// Extended fields (JSON format for storing additional information)
	Metadata map[string]interface{} `json:"metadata" gorm:"column:metadata;serializer:json"`

	// Wasm-specific fields (for CosmWasm contracts)
	CodeID          *int64      `json:"code_id" gorm:"column:code_id"`
	Label           string      `json:"label" gorm:"column:label;size:255"`
	InitMsg         string      `json:"init_msg" gorm:"column:init_msg;type:text"`
	Admin           string      `json:"admin" gorm:"column:admin;size:128"`
	ExecuteCount    int64       `json:"execute_count" gorm:"column:execute_count;default:0"`
	LastExecutedAt  *time.Time  `json:"last_executed_at" gorm:"column:last_executed_at"`
	InstantiatedAt  *time.Time  `json:"instantiated_at" gorm:"column:instantiated_at"`
	BlockHeight     *int64      `json:"block_height" gorm:"column:block_height"`
	TxHash          string      `json:"tx_hash" gorm:"column:tx_hash;size:128"`
	TotalSupply     string      `json:"total_supply" gorm:"column:total_supply;size:255"`

	// Relationships
	Tags []ContractTag `json:"tags" gorm:"foreignKey:ContractAddress,ChainID;references:ContractAddress,ChainID"`
}

// TableName specifies the table name for ContractInfo.
func (ContractInfo) TableName() string {
	return "contract_infos"
}
