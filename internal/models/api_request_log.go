// Package models provides data models for blockchain entities.
package models

import (
	"gorm.io/gorm"
)

// APIRequestLog represents an API request log in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores API request logs including token ID, user ID, IP address,
// endpoint, method, status code, response time, and error messages.
type APIRequestLog struct {
	gorm.Model

	// TokenID is the API token ID used for this request (indexed for faster lookups)
	TokenID uint `gorm:"column:token_id;type:bigint unsigned;not null;index" json:"token_id"`
	// UserID is the user ID (indexed for faster lookups)
	UserID string `gorm:"column:user_id;type:varchar(64);not null;index" json:"user_id"`
	// IPAddress is the client IP address (supports IPv6, indexed for faster lookups)
	IPAddress string `gorm:"column:ip_address;type:varchar(45);not null;index" json:"ip_address"`
	// IPLocation is the IP geolocation (may be empty if lookup failed)
	IPLocation string `gorm:"column:ip_location;type:varchar(100)" json:"ip_location"`
	// Endpoint is the API endpoint path (indexed for faster lookups)
	Endpoint string `gorm:"column:endpoint;type:varchar(255);not null;index" json:"endpoint"`
	// Method is the HTTP method (GET, POST, etc.)
	Method string `gorm:"column:method;type:varchar(10);not null" json:"method"`
	// StatusCode is the HTTP status code (indexed for faster lookups)
	StatusCode int `gorm:"column:status_code;type:int;not null;index" json:"status_code"`
	// ResponseTime is the response time in milliseconds
	ResponseTime int `gorm:"column:response_time;type:int;not null" json:"response_time"`
	// ErrorMessage is the error message if the request failed
	ErrorMessage string `gorm:"column:error_message;type:text" json:"error_message"`
}

// TableName specifies the table name for APIRequestLog.
func (APIRequestLog) TableName() string {
	return "api_request_logs"
}
