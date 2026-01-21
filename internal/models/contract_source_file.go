// Package models provides data models for blockchain entities.
package models

import (
	"gorm.io/gorm"
)

// ContractSourceFile represents a contract source code file in the database.
// It stores source code files (.sol files) and ABI files (.json files) for contracts.
type ContractSourceFile struct {
	gorm.Model

	// Basic information
	ContractAddress string `json:"contract_address" gorm:"column:contract_address;size:128;not null;index"`
	ChainID         string `json:"chain_id" gorm:"column:chain_id;size:50;default:injective-1;index"`

	// File information
	FileName              string `json:"file_name" gorm:"column:file_name;size:255;not null"`                      // Original filename (e.g., "Token.sol" or "abi.json")
	FileType              string `json:"file_type" gorm:"column:file_type;size:50;not null;index"`                 // File type: "source" or "abi"
	FilePath              string `json:"file_path" gorm:"column:file_path;size:512"`                               // File path (optional, for multi-file contracts)
	FileContent           string `json:"file_content" gorm:"column:file_content;type:mediumtext"`                  // File content (for small files)
	FileContentStorageKey string `json:"file_content_storage_key" gorm:"column:file_content_storage_key;size:512"` // Storage key for large files
	FileSize              int64  `json:"file_size" gorm:"column:file_size"`                                        // File size in bytes
	FileHash              string `json:"file_hash" gorm:"column:file_hash;size:128;index"`                         // SHA256 hash of file content
	MIMEType              string `json:"mime_type" gorm:"column:mime_type;size:100"`                               // MIME type (e.g., "text/plain", "application/json")

	// Metadata
	Description string `json:"description" gorm:"column:description;type:text"` // Optional description
	UploadedBy  string `json:"uploaded_by" gorm:"column:uploaded_by;size:128"`  // User/address who uploaded the file
}

// TableName specifies the table name for ContractSourceFile.
func (ContractSourceFile) TableName() string {
	return "contract_source_files"
}
