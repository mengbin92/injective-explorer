// Package models provides data models for blockchain entities.
// This file defines archive models for transaction data archiving.
package models

// TransactionDataArchive represents archived transaction detailed data.
// It stores the large fields (Data, Messages, Logs) that are moved to archive storage.
// Large fields can be stored in object storage (MinIO/S3/OSS/COS) to reduce database size.
//
// Optimization: TransactionID directly references Transaction.ID (main table).
// This removes the need for a separate transaction_archives table, reducing data redundancy.
// The main transactions table retains metadata (with archived=true flag) for statistics and queries.
type TransactionDataArchive struct {
	// TransactionID is the primary key, referencing Transaction.ID (main table)
	TransactionID uint `json:"transaction_id" gorm:"primaryKey;column:transaction_id"`

	// Data contains the transaction data (compressed, stored as LONGBLOB/TEXT)
	// If DataStorageKey is set, this field should be empty and data is stored in object storage
	Data string `json:"data" gorm:"column:data;type:longtext"`
	// DataStorageKey is the object storage key for Data field (if stored in MinIO/S3/OSS/COS)
	DataStorageKey string `json:"data_storage_key" gorm:"column:data_storage_key"`

	// Info contains additional transaction information
	Info string `json:"info" gorm:"column:info"`

	// Messages contains the transaction messages (compressed, stored as LONGBLOB/TEXT)
	// If MessagesStorageKey is set, this field should be empty and data is stored in object storage
	Messages string `json:"messages" gorm:"column:messages;type:longtext"`
	// MessagesStorageKey is the object storage key for Messages field (if stored in MinIO/S3/OSS/COS)
	MessagesStorageKey string `json:"messages_storage_key" gorm:"column:messages_storage_key"`

	// Logs contains execution logs (compressed, stored as LONGBLOB/TEXT)
	// If LogsStorageKey is set, this field should be empty and data is stored in object storage
	Logs string `json:"logs" gorm:"column:logs;type:longtext"`
	// LogsStorageKey is the object storage key for Logs field (if stored in MinIO/S3/OSS/COS)
	LogsStorageKey string `json:"logs_storage_key" gorm:"column:logs_storage_key"`

	// ErrorLog contains error information if the transaction failed
	ErrorLog string `json:"error_log" gorm:"column:error_log"`
}

// TableName specifies the table name for TransactionDataArchive.
func (TransactionDataArchive) TableName() string {
	return "transaction_data_archives"
}
