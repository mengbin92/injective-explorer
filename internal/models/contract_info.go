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
	Name            string `json:"name" gorm:"column:name;size:255"`                                       // Generic contract name (optional)
	TokenName       string `json:"token_name" gorm:"column:token_name;size:255"`                           // Token name (optional, for token contracts)
	TokenSymbol     string `json:"token_symbol" gorm:"column:token_symbol;size:50"` // Token symbol (optional, for token contracts)
	ContractType    string `json:"contract_type" gorm:"column:contract_type;size:50;default:cosmwasm"`
	ChainID         string `json:"chain_id" gorm:"column:chain_id;size:50;default:injective-1"`

	// Verification related (reserved for future extension)
	VerificationStatus string     `json:"verification_status" gorm:"column:verification_status;size:50;default:unverified"`
	IsVerified         bool       `json:"is_verified" gorm:"column:is_verified;default:false"`
	VerifiedAt         *time.Time `json:"verified_at" gorm:"column:verified_at"`
	VerifiedBy         string     `json:"verified_by" gorm:"column:verified_by;size:128"`

	// Extended fields (JSON format for storing additional information)
	Metadata map[string]interface{} `json:"metadata" gorm:"column:metadata;type:json"`

	// Relationships
	Tags []ContractTag `json:"tags" gorm:"foreignKey:ContractAddress,ChainID;references:ContractAddress,ChainID"`
}

// TableName specifies the table name for ContractInfo.
func (ContractInfo) TableName() string {
	return "contract_infos"
}
