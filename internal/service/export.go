// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"fmt"
	"io"

	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// ExportService handles data export operations.
// It provides Excel export functionality with memory optimization for large datasets.
type ExportService struct {
	db *gorm.DB
}

// NewExportService creates a new export service instance.
//
// Returns:
//   - *ExportService: A new service instance
func NewExportService() *ExportService {
	return &ExportService{
		db: db.Get(),
	}
}

// ExportUsageLogs exports API usage logs to Excel format with pagination.
// This method uses batch processing to avoid memory issues when exporting large datasets.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - userID: The user ID to export logs for
//   - w: Writer to write Excel file to
//
// Returns:
//   - error: Error if export fails
func (s *ExportService) ExportUsageLogs(ctx context.Context, userID string, w io.Writer) error {
	f := excelize.NewFile()
	defer f.Close()

	sheetName := "Usage Logs"
	f.NewSheet(sheetName)

	// Set headers
	headers := []string{"时间", "IP地址", "IP归属地", "接口", "方法", "状态码", "响应时间(ms)", "错误信息"}
	for i, h := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		if err := f.SetCellValue(sheetName, cell, h); err != nil {
			return fmt.Errorf("set header cell %s: %w", cell, err)
		}
	}

	// Set header style (bold)
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
	})
	if err == nil {
		f.SetCellStyle(sheetName, "A1", fmt.Sprintf("%c1", 'A'+len(headers)-1), headerStyle)
	}

	// Query in batches to avoid memory issues
	batchSize := 1000
	page := 1
	row := 2

	for {
		var logs []models.APIRequestLog
		err := s.db.WithContext(ctx).
			Where("user_id = ?", userID).
			Order("created_at DESC").
			Limit(batchSize).
			Offset((page - 1) * batchSize).
			Find(&logs).Error

		if err != nil {
			return fmt.Errorf("query logs batch at page %d: %w", page, err)
		}

		if len(logs) == 0 {
			break // No more data
		}

		// Write batch to Excel
		for _, log := range logs {
			// Format time
			timeStr := log.CreatedAt.Format("2006-01-02 15:04:05")

			// Set cell values
			if err := f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), timeStr); err != nil {
				return fmt.Errorf("set cell A%d: %w", row, err)
			}
			if err := f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), log.IPAddress); err != nil {
				return fmt.Errorf("set cell B%d: %w", row, err)
			}
			if err := f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), log.IPLocation); err != nil {
				return fmt.Errorf("set cell C%d: %w", row, err)
			}
			if err := f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), log.Endpoint); err != nil {
				return fmt.Errorf("set cell D%d: %w", row, err)
			}
			if err := f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), log.Method); err != nil {
				return fmt.Errorf("set cell E%d: %w", row, err)
			}
			if err := f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), log.StatusCode); err != nil {
				return fmt.Errorf("set cell F%d: %w", row, err)
			}
			if err := f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), log.ResponseTime); err != nil {
				return fmt.Errorf("set cell G%d: %w", row, err)
			}
			if err := f.SetCellValue(sheetName, fmt.Sprintf("H%d", row), log.ErrorMessage); err != nil {
				return fmt.Errorf("set cell H%d: %w", row, err)
			}
			row++
		}

		// Flush to writer periodically (every 10 pages) to reduce memory usage
		if page%10 == 0 {
			if err := f.Write(w); err != nil {
				return fmt.Errorf("flush to writer at page %d: %w", page, err)
			}
			global.Logger.Debugf("flushed export data: %d rows written", row-2)
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if len(logs) < batchSize {
			break // Last page
		}
		page++
	}

	// Final write
	if err := f.Write(w); err != nil {
		return fmt.Errorf("final write: %w", err)
	}

	global.Logger.Infof("export completed: %d rows exported for user %s", row-2, userID)
	return nil
}

