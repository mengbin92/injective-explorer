// Package models provides data models for blockchain entities.
package models

import (
	"time"

	"gorm.io/gorm"
)

// ContractVerification represents a contract verification record in the database.
// It stores the details and history of each verification attempt, including
// compiler settings, source code references, bytecode comparison results, and
// the final verification status.
//
// Supports both Solidity (EVM) contracts and CosmWasm contracts.
type ContractVerification struct {
	gorm.Model

	// Basic identification
	ContractAddress string `json:"contract_address" gorm:"column:contract_address;size:128;not null;index"`
	ChainID         string `json:"chain_id" gorm:"column:chain_id;size:50;default:injective-1;index"`
	ContractType    string `json:"contract_type" gorm:"column:contract_type;size:50;not null"` // solidity / cosmwasm
	VerificationType string `json:"verification_type" gorm:"column:verification_type;size:50"` // full_match / partial_match

	// Solidity-specific compiler settings
	CompilerVersion     string `json:"compiler_version" gorm:"column:compiler_version;size:100"`
	OptimizationEnabled bool   `json:"optimization_enabled" gorm:"column:optimization_enabled;default:false"`
	OptimizationRuns    int    `json:"optimization_runs" gorm:"column:optimization_runs;default:0"`
	EVMVersion          string `json:"evm_version" gorm:"column:evm_version;size:50"`

	// CosmWasm-specific fields
	CargoVersion string `json:"cargo_version" gorm:"column:cargo_version;size:100"`
	CodeID       *int64 `json:"code_id" gorm:"column:code_id"`

	// Common fields
	License             string `json:"license" gorm:"column:license;size:100"`
	SourceCode          string `json:"source_code" gorm:"column:source_code;type:mediumtext"`
	SourceCodeStorageKey string `json:"source_code_storage_key" gorm:"column:source_code_storage_key;size:512"`
	ABI                 string `json:"abi" gorm:"column:abi;type:json"`
	ABIStorageKey       string `json:"abi_storage_key" gorm:"column:abi_storage_key;size:512"`
	ConstructorArgs     string `json:"constructor_args" gorm:"column:constructor_args;type:text"`

	// Verification status
	VerificationStatus string     `json:"verification_status" gorm:"column:verification_status;size:50;default:pending;index"`
	ErrorMessage       string     `json:"error_message" gorm:"column:error_message;type:text"`
	VerifiedAt         *time.Time `json:"verified_at" gorm:"column:verified_at"`
	VerifiedBy         string     `json:"verified_by" gorm:"column:verified_by;size:128"`
}

// TableName specifies the table name for ContractVerification.
func (ContractVerification) TableName() string {
	return "contract_verifications"
}

// Verification status constants
const (
	VerificationStatusPending   = "pending"
	VerificationStatusVerifying = "verifying"
	VerificationStatusVerified  = "verified"
	VerificationStatusFailed    = "failed"
)

// Verification type constants
const (
	VerificationTypeFullMatch    = "full_match"
	VerificationTypePartialMatch = "partial_match"
)

// Contract type constants for verification
const (
	ContractTypeSolidity = "solidity"
	ContractTypeCosmWasm = "cosmwasm"
)
