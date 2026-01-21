// Package models provides data models for blockchain entities.
package models

import (
	"gorm.io/gorm"
)

// IPBlacklist represents an IP address in the blacklist.
type IPBlacklist struct {
	gorm.Model

	// IPAddress is the IP address (supports IPv4 and IPv6)
	IPAddress string `gorm:"column:ip_address;type:varchar(45);not null;uniqueIndex" json:"ip_address"`
	// IPLocation is the IP location/region
	IPLocation string `gorm:"column:ip_location;type:varchar(100)" json:"ip_location"`
	// Reason is the reason for blacklisting: batch_register/api_abuse/proxy/spam/other
	Reason string `gorm:"column:reason;type:varchar(50);not null" json:"reason"`
	// ReasonText is the detailed reason description
	ReasonText string `gorm:"column:reason_text;type:varchar(255)" json:"reason_text"`
	// BlockCount is the number of times this IP was blocked
	BlockCount uint64 `gorm:"column:block_count;type:bigint unsigned;not null" json:"block_count"`
	// OperatorID is the ID of the operator who added this IP
	OperatorID string `gorm:"column:operator_id;type:varchar(64)" json:"operator_id"`
	// OperatorName is the name of the operator
	OperatorName string `gorm:"column:operator_name;type:varchar(100)" json:"operator_name"`
	// BlockType is the restriction type: all/register/api
	BlockType string `gorm:"column:block_type;type:varchar(20);not null" json:"block_type"`
}

// TableName specifies the table name for IPBlacklist.
func (IPBlacklist) TableName() string {
	return "ip_blacklist"
}

// IPRegistrationRecord represents an IP registration record.
type IPRegistrationRecord struct {
	gorm.Model

	// IPAddress is the IP address
	IPAddress string `gorm:"column:ip_address;type:varchar(45);not null;index" json:"ip_address"`
	// IPLocation is the IP location/region
	IPLocation string `gorm:"column:ip_location;type:varchar(100)" json:"ip_location"`
	// UserID is the user ID who registered from this IP
	UserID string `gorm:"column:user_id;type:varchar(64);not null;index" json:"user_id"`
	// Email is the registration email
	Email string `gorm:"column:email;type:varchar(255)" json:"email"`
}

// TableName specifies the table name for IPRegistrationRecord.
func (IPRegistrationRecord) TableName() string {
	return "ip_registration_records"
}

