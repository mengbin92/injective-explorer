// Package service provides performance metrics and monitoring for services.
package service

import (
	"time"

	"injective-explorer/internal/global"
)

// ArchiveMetrics contains performance metrics for archiving operations.
type ArchiveMetrics struct {
	// TotalTransactions is the total number of transactions found to archive
	TotalTransactions int64
	// ArchivedTransactions is the number of successfully archived transactions
	ArchivedTransactions int64
	// FailedTransactions is the number of transactions that failed to archive
	FailedTransactions int64
	// StartTime is when the archiving operation started
	StartTime time.Time
	// EndTime is when the archiving operation completed
	EndTime time.Time
	// Duration is the total time taken for the operation
	Duration time.Duration
	// BatchesProcessed is the number of batches processed
	BatchesProcessed int
	// AverageBatchTime is the average time per batch
	AverageBatchTime time.Duration
	// TransactionsPerSecond is the archiving rate
	TransactionsPerSecond float64
}

// MigrationMetrics contains performance metrics for migration operations.
type MigrationMetrics struct {
	// TotalTransactions is the total number of transactions found to migrate
	TotalTransactions int64
	// MigratedTransactions is the number of successfully migrated transactions
	MigratedTransactions int64
	// FailedTransactions is the number of transactions that failed to migrate
	FailedTransactions int64
	// StartTime is when the migration operation started
	StartTime time.Time
	// EndTime is when the migration operation completed
	EndTime time.Time
	// Duration is the total time taken for the operation
	Duration time.Duration
	// BatchesProcessed is the number of batches processed
	BatchesProcessed int
	// AverageBatchTime is the average time per batch
	AverageBatchTime time.Duration
	// TransactionsPerSecond is the migration rate
	TransactionsPerSecond float64
	// StorageOperations is the number of storage operations performed
	StorageOperations int64
	// StorageOperationErrors is the number of storage operation errors
	StorageOperationErrors int64
	// AverageStorageLatency is the average latency for storage operations
	AverageStorageLatency time.Duration
}

// CalculateMetrics calculates performance metrics from start and end times.
func (m *ArchiveMetrics) CalculateMetrics() {
	if !m.StartTime.IsZero() && !m.EndTime.IsZero() {
		m.Duration = m.EndTime.Sub(m.StartTime)
		if m.Duration > 0 {
			m.TransactionsPerSecond = float64(m.ArchivedTransactions) / m.Duration.Seconds()
		}
		if m.BatchesProcessed > 0 {
			m.AverageBatchTime = m.Duration / time.Duration(m.BatchesProcessed)
		}
	}
}

// CalculateMetrics calculates performance metrics from start and end times.
func (m *MigrationMetrics) CalculateMetrics() {
	if !m.StartTime.IsZero() && !m.EndTime.IsZero() {
		m.Duration = m.EndTime.Sub(m.StartTime)
		if m.Duration > 0 {
			m.TransactionsPerSecond = float64(m.MigratedTransactions) / m.Duration.Seconds()
		}
		if m.BatchesProcessed > 0 {
			m.AverageBatchTime = m.Duration / time.Duration(m.BatchesProcessed)
		}
	}
}

// LogMetrics logs the metrics using the global logger.
func (m *ArchiveMetrics) LogMetrics() {
	m.CalculateMetrics()
	global.Logger.Infof("archive metrics: total=%d, archived=%d, failed=%d, duration=%v, rate=%.2f tx/s, batches=%d, avg_batch_time=%v",
		m.TotalTransactions, m.ArchivedTransactions, m.FailedTransactions,
		m.Duration, m.TransactionsPerSecond, m.BatchesProcessed, m.AverageBatchTime)
}

// LogMetrics logs the metrics using the global logger.
func (m *MigrationMetrics) LogMetrics() {
	m.CalculateMetrics()
	global.Logger.Infof("migration metrics: total=%d, migrated=%d, failed=%d, duration=%v, rate=%.2f tx/s, batches=%d, avg_batch_time=%v, storage_ops=%d, storage_errors=%d, avg_storage_latency=%v",
		m.TotalTransactions, m.MigratedTransactions, m.FailedTransactions,
		m.Duration, m.TransactionsPerSecond, m.BatchesProcessed, m.AverageBatchTime,
		m.StorageOperations, m.StorageOperationErrors, m.AverageStorageLatency)
}
