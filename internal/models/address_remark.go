// Package models provides data models for blockchain entities.
package models

import (
	"time"

	pb "injective-explorer/api/explorer/v1"
	"gorm.io/gorm"
)

// AddressRemark represents a remark for an address in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// This table stores address-level remarks that are only visible in the backend admin system.
// Unlike labels (which are multiple per address), each address can have at most one remark.
type AddressRemark struct {
	gorm.Model

	// Address is the blockchain address (Ethereum format: 0x...)
	Address string `json:"address" gorm:"column:address;size:128;not null;uniqueIndex:uk_address_remark,priority:2;index:idx_address,priority:2"`

	// ChainID identifies the blockchain network
	ChainID string `json:"chain_id" gorm:"column:chain_id;size:50;not null;uniqueIndex:uk_address_remark,priority:1;index:idx_address,priority:1"`

	// Remark is the note content (only visible in backend admin system)
	Remark string `json:"remark" gorm:"column:remark;type:text;not null"`

	// CreatedBy is the user who created this remark
	CreatedBy string `json:"created_by" gorm:"column:created_by;size:128"`
}

// TableName specifies the table name for AddressRemark.
func (AddressRemark) TableName() string {
	return "address_remarks"
}

// ToAddressRemarkDTO converts AddressRemark to proto AddressRemarkDTO.
func (r *AddressRemark) ToAddressRemarkDTO() *pb.AddressRemarkDTO {
	if r == nil {
		return nil
	}

	var createdAt string
	if !r.CreatedAt.IsZero() {
		createdAt = r.CreatedAt.Format(time.RFC3339)
	}

	var updatedAt string
	if !r.UpdatedAt.IsZero() {
		updatedAt = r.UpdatedAt.Format(time.RFC3339)
	}

	return &pb.AddressRemarkDTO{
		Id:        uint64(r.ID),
		Address:   r.Address,
		ChainId:   r.ChainID,
		Remark:    r.Remark,
		CreatedBy: r.CreatedBy,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}
