// Package utils provides shared utility functions used across the application.
// It includes helpers for string manipulation, validation, pagination, caching,
// database operations, and context management.
package utils

import (
	"fmt"
	"strconv"
	"time"

	pb "injective-explorer/api/explorer/v1"
)

// Pagination constants
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
	MinPage         = 1
	MinPageSize     = 1
)

// PaginationResult holds the normalized pagination parameters.
type PaginationResult struct {
	Page        int32  // Normalized page number (1-based)
	PageSize    int32  // Normalized page size
	Offset      int    // Calculated offset (0-based)
	UseCursor   bool   // Whether to use cursor-based pagination
	CursorValue uint64 // Parsed cursor value (0 if not using cursor)
}

// NormalizePagination normalizes pagination parameters for consistent handling.
// It validates and adjusts page and page size values, and supports cursor-based pagination.
//
// Parameters:
//   - page: Requested page number (1-based, will be normalized to at least 1)
//   - pageSize: Requested page size (will be clamped between 1 and maxPageSize)
//   - cursor: Optional cursor string for cursor-based pagination
//   - maxPageSize: Maximum allowed page size (use MaxPageSize for standard limit)
//
// Returns:
//   - PaginationResult: Struct containing all normalized pagination parameters
func NormalizePagination(page, pageSize int32, cursor string, maxPageSize int32) PaginationResult {
	result := PaginationResult{}

	// Normalize page size (must be between min and max)
	result.PageSize = pageSize
	if result.PageSize < MinPageSize {
		result.PageSize = DefaultPageSize
	}
	if result.PageSize > maxPageSize {
		result.PageSize = maxPageSize
	}

	// Check if using cursor-based pagination
	if cursor != "" {
		// Parse cursor (assuming it's a numeric value representing height or tx_number)
		cursorValue, err := strconv.ParseUint(cursor, 10, 64)
		if err == nil {
			result.UseCursor = true
			result.CursorValue = cursorValue
			result.Page = 0
			result.Offset = 0
			return result
		}
		// If cursor parsing fails, fall back to page-based
	}

	// Normalize page (must be >= 1)
	result.Page = max(page, MinPage)

	// Calculate offset (0-based)
	result.Offset = int((result.Page - 1) * result.PageSize)

	return result
}

// NormalizePaginationRequest normalizes pagination from a PaginationRequest proto message.
// This is a convenience wrapper for NormalizePagination when working with protobuf requests.
//
// Parameters:
//   - req: The PaginationRequest from protobuf
//   - maxPageSize: Maximum allowed page size
//
// Returns:
//   - PaginationResult: Struct containing all normalized pagination parameters
func NormalizePaginationRequest(req *pb.PaginationRequest, maxPageSize int32) PaginationResult {
	if req == nil {
		return PaginationResult{
			Page:     MinPage,
			PageSize: DefaultPageSize,
			Offset:   0,
		}
	}
	return NormalizePagination(req.Page, req.PageSize, req.Cursor, maxPageSize)
}

// PaginationInfo holds pagination metadata for responses.
type PaginationInfo struct {
	Page         int32 `json:"page"`
	PageSize     int32 `json:"page_size"`
	Total        int64 `json:"total"`
	TotalPages   int32 `json:"total_pages"`
	HasNext      bool  `json:"has_next"`
	HasPrevious  bool  `json:"has_previous"`
	Approximate  bool  `json:"approximate,omitempty"`
	Cursor       uint64 `json:"cursor,omitempty"`
	NextCursor   string `json:"next_cursor,omitempty"`
}

// BuildPaginationInfo creates pagination info for responses.
//
// Parameters:
//   - page: Current page number
//   - pageSize: Number of items per page
//   - total: Total number of items
//
// Returns:
//   - PaginationInfo: Pagination metadata
func BuildPaginationInfo(page, pageSize int32, total int64) PaginationInfo {
	totalPages := int32(0)
	if pageSize > 0 {
		totalPages = int32((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return PaginationInfo{
		Page:        page,
		PageSize:    pageSize,
		Total:       total,
		TotalPages:  totalPages,
		HasNext:     page < totalPages,
		HasPrevious: page > MinPage,
	}
}

// BuildPaginatedResponse creates a standardized paginated response map.
// This is used by multiple service handlers to construct consistent API responses.
//
// Parameters:
//   - data: The actual data to return
//   - page: Current page number
//   - pageSize: Number of items per page
//   - total: Total number of items
//   - isExactCount: Whether the total count is exact or approximate
//   - useCursor: Whether using cursor-based pagination
//   - cursorValue: Current cursor value
//   - nextCursor: Next cursor value (empty if no more pages)
//   - resultCount: Number of results in current page
//
// Returns:
//   - map[string]any: Response map with data and pagination info
func BuildPaginatedResponse(data any, page, pageSize int32, total int64, isExactCount bool, useCursor bool, cursorValue uint64, nextCursor string, resultCount int) map[string]any {
	paginationInfo := BuildPaginationInfo(page, pageSize, total)
	if !isExactCount {
		paginationInfo.Approximate = true
	}
	if useCursor {
		paginationInfo.Cursor = cursorValue
		if nextCursor != "" {
			paginationInfo.NextCursor = nextCursor
		}
		paginationInfo.HasNext = resultCount == int(pageSize)
	}

	// Flatten the response structure: put data and pagination at the same level
	response := make(map[string]any)
	response["data"] = data
	response["pagination"] = paginationInfo
	return response
}

// BuildSimplePaginatedResponse creates a simplified paginated response without cursor support.
// This is a convenience function for standard page-based pagination.
//
// Parameters:
//   - data: The actual data to return
//   - page: Current page number
//   - pageSize: Number of items per page
//   - total: Total number of items
//
// Returns:
//   - map[string]any: Response map with data and pagination info
func BuildSimplePaginatedResponse(data any, page, pageSize int32, total int64) map[string]any {
	return BuildPaginatedResponse(data, page, pageSize, total, true, false, 0, "", 0)
}

// CalculateNextCursor calculates the next cursor value for cursor-based pagination.
// It finds the minimum value that is less than the current cursor value.
//
// Parameters:
//   - useCursor: Whether using cursor-based pagination
//   - cursorValue: Current cursor value
//   - items: Slice of items to extract next cursor from
//   - getValue: Function to extract the cursor value from an item
//
// Returns:
//   - string: Next cursor value (empty string if not using cursor or no valid value found)
func CalculateNextCursor[T any](useCursor bool, cursorValue uint64, items []T, getValue func(T) uint64) string {
	if !useCursor || len(items) == 0 {
		return ""
	}

	// Find the minimum value less than cursorValue
	var minValue uint64 = 0
	found := false
	for _, item := range items {
		value := getValue(item)
		if value < cursorValue {
			if !found || value < minValue {
				minValue = value
				found = true
			}
		}
	}

	if found {
		return fmt.Sprintf("%d", minValue)
	}
	return ""
}

// CalculateOffset calculates the offset for database queries based on page and page size.
//
// Parameters:
//   - page: Page number (1-based)
//   - pageSize: Number of items per page
//
// Returns:
//   - int: The offset (0-based) for database queries
func CalculateOffset(page, pageSize int32) int {
	if page < MinPage {
		page = MinPage
	}
	if pageSize < MinPageSize {
		pageSize = DefaultPageSize
	}
	return int((page - 1) * pageSize)
}

// ClampPageSize clamps the page size to valid range.
//
// Parameters:
//   - pageSize: Requested page size
//   - maxPageSize: Maximum allowed page size
//
// Returns:
//   - int32: Clamped page size
func ClampPageSize(pageSize, maxPageSize int32) int32 {
	if pageSize < MinPageSize {
		return DefaultPageSize
	}
	if pageSize > maxPageSize {
		return maxPageSize
	}
	return pageSize
}

// PaginateSlice returns a paginated subset of a slice.
// This is useful for in-memory pagination when all data is already loaded.
//
// Parameters:
//   - slice: The source slice
//   - page: Page number (1-based)
//   - pageSize: Number of items per page
//
// Returns:
//   - []T: Paginated slice
//   - int: Total count
//   - bool: Whether page is valid
func PaginateSlice[T any](slice []T, page, pageSize int32) ([]T, int, bool) {
	total := len(slice)

	// Normalize pagination
	page = max(page, MinPage)
	pageSize = max(pageSize, MinPageSize)
	pageSize = min(pageSize, MaxPageSize)

	// Calculate offset
	offset := int((page - 1) * pageSize)
	if offset >= total {
		return []T{}, total, false
	}

	// Calculate end index
	end := min(offset+int(pageSize), total)

	return slice[offset:end], total, true
}

// DefaultPagination returns default pagination parameters.
//
// Returns:
//   - PaginationResult: Default pagination (page 1, default page size)
func DefaultPagination() PaginationResult {
	return PaginationResult{
		Page:     MinPage,
		PageSize: DefaultPageSize,
		Offset:   0,
		UseCursor: false,
		CursorValue: 0,
	}
}

// OffsetBasedPagination creates pagination parameters for offset-based pagination.
// This is useful when you need to work with offset/limit directly.
//
// Parameters:
//   - offset: Offset value (0-based)
//   - limit: Number of items to return
//   - maxLimit: Maximum allowed limit
//
// Returns:
//   - PaginationResult: Pagination parameters
func OffsetBasedPagination(offset int, limit int32, maxLimit int32) PaginationResult {
	if limit < MinPageSize {
		limit = DefaultPageSize
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}

	return PaginationResult{
		Page:     int32(offset/int(limit)) + 1,
		PageSize: limit,
		Offset:   offset,
		UseCursor: false,
		CursorValue: 0,
	}
}

// CacheTTLForPagination returns appropriate cache TTL based on page number.
// Earlier pages are cached longer as they are accessed more frequently.
//
// Parameters:
//   - page: Page number
//   - baseTTL: Base TTL duration
//
// Returns:
//   - time.Duration: Calculated TTL
func CacheTTLForPagination(page int32, baseTTL time.Duration) time.Duration {
	switch {
	case page <= 1:
		return baseTTL // First page gets full TTL
	case page <= 5:
		return baseTTL / 2 // Pages 2-5 get half TTL
	default:
		return baseTTL / 4 // Later pages get quarter TTL
	}
}
