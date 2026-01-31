// Package service provides business logic services for the explorer API.
package service

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"
	"injective-explorer/provider/storage"

	"github.com/bytedance/sonic"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// ExportContractInfosResponseData represents the export response data structure.
type ExportContractInfosResponseData struct {
	FileContentBase64 string `json:"file_content_base64,omitempty"` // Base64 encoded file content for small files
	DownloadURL       string `json:"download_url,omitempty"`        // Download URL for large files
	Format            string `json:"format"`                        // File format: csv or json
	TotalCount        int32  `json:"total_count"`                   // Number of exported records
	FileSize          int64  `json:"file_size"`                     // File size in bytes
	ExportedAt        string `json:"exported_at"`                   // Export timestamp
}

// ImportContractInfosResponseData represents the import response data structure.
type ImportContractInfosResponseData struct {
	TotalCount   int32              `json:"total_count"`   // Total number of records
	SuccessCount int32              `json:"success_count"` // Number of successful imports
	FailedCount  int32              `json:"failed_count"`  // Number of failed imports
	CreatedCount int32              `json:"created_count"` // Number of created records
	UpdatedCount int32              `json:"updated_count"` // Number of updated records
	Errors       []*ImportErrorInfo `json:"errors"`        // List of errors
	ImportedAt   string             `json:"imported_at"`   // Import timestamp
}

// ImportErrorInfo represents import error information.
type ImportErrorInfo struct {
	RowNumber       int32  `json:"row_number"`       // Row number in the import file
	ContractAddress string `json:"contract_address"` // Contract address
	ErrorMessage    string `json:"error_message"`    // Error message
	ErrorCode       string `json:"error_code"`       // Error code
}

// ContractInfoImport represents contract information structure for import.
type ContractInfoImport struct {
	ContractAddress    string
	ChainID            string
	Name               string
	TokenName          string
	TokenSymbol        string
	ContractType       string
	VerificationStatus string
	IsVerified         bool
	VerifiedAt         *time.Time
	VerifiedBy         string
	Metadata           map[string]interface{}
	Tags               []*TagImport
}

// TagImport represents tag structure for import.
type TagImport struct {
	TagType     string
	TagValue    string
	TagColor    string
	Description string
	CreatedBy   string
}

// ImportResult represents import operation result.
type ImportResult struct {
	TotalCount   int32
	SuccessCount int32
	FailedCount  int32
	CreatedCount int32
	UpdatedCount int32
	Errors       []*ImportErrorInfo
}

const (
	// MaxExportFileSize is the maximum export file size (10MB) before using object storage.
	MaxExportFileSize = 10 * 1024 * 1024
	// MaxImportFileSize is the maximum import file size (50MB).
	MaxImportFileSize = 50 * 1024 * 1024
	// MaxImportRecords is the maximum number of records per import.
	MaxImportRecords = 10000
	// BatchImportSize is the batch size for bulk import operations.
	BatchImportSize = 500
)

// ExportContractInfos exports contract information to CSV or JSON format.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing export format and filter conditions
//
// Returns:
//   - *pb.Reply: Response containing export result with file content or download URL
//   - error: Error if the operation fails
func (s *ContractInfoService) ExportContractInfos(ctx context.Context, req *pb.ExportContractInfosRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Format == "" {
		return ErrorReply(http.StatusBadRequest, "format is required", errors.New("format is required")), errors.New("format is required")
	}

	format := strings.ToLower(req.Format)
	if format != "csv" && format != "json" {
		return ErrorReply(http.StatusBadRequest, "format must be csv or json", errors.New("invalid format")), errors.New("invalid format")
	}

	// Set default values
	includeTags := req.IncludeTags
	includeMetadata := req.IncludeMetadata

	// Build query (reuse ListContractInfos logic)
	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	query := db.Get().WithContext(queryCtx).Model(&models.ContractInfo{})

	// Apply filters
	if req.ChainId != "" {
		query = query.Where("chain_id = ?", req.ChainId)
	}

	if req.ContractType != "" {
		query = query.Where("contract_type = ?", req.ContractType)
	}

	if req.VerificationStatus != "" {
		query = query.Where("verification_status = ?", req.VerificationStatus)
	}

	if req.Name != "" {
		query = query.Where("name LIKE ? OR token_name LIKE ?", "%"+req.Name+"%", "%"+req.Name+"%")
	}

	if req.Symbol != "" {
		query = query.Where("token_symbol LIKE ?", "%"+req.Symbol+"%")
	}

	// If specific contract addresses are provided, add filter
	if len(req.ContractAddresses) > 0 {
		// Convert address format
		addresses := make([]string, 0, len(req.ContractAddresses))
		for _, addr := range req.ContractAddresses {
			addresses = append(addresses, models.ConvertAddressToEthForStorage(addr))
		}
		query = query.Where("contract_address IN ?", addresses)
	}

	// Query all matching contracts (no pagination)
	var contracts []models.ContractInfo
	if includeTags {
		err := query.Preload("Tags").Find(&contracts).Error
		if err != nil {
			global.Logger.Errorf("failed to query contracts for export: %v", err)
			return ErrorReply(http.StatusInternalServerError, "failed to query contracts", err), err
		}
	} else {
		err := query.Find(&contracts).Error
		if err != nil {
			global.Logger.Errorf("failed to query contracts for export: %v", err)
			return ErrorReply(http.StatusInternalServerError, "failed to query contracts", err), err
		}
	}

	// Export based on format
	var fileContent []byte
	var err error

	if format == "csv" {
		fileContent, err = s.exportToCSV(contracts, includeTags, includeMetadata)
	} else {
		fileContent, err = s.exportToJSON(contracts, includeTags, includeMetadata)
	}

	if err != nil {
		global.Logger.Errorf("failed to export contracts: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to export contracts", err), err
	}

	// Build response data
	responseData := &ExportContractInfosResponseData{
		Format:     format,
		TotalCount: int32(len(contracts)),
		FileSize:   int64(len(fileContent)),
		ExportedAt: time.Now().Format(time.RFC3339),
	}

	// Determine response method based on file size
	if int64(len(fileContent)) > MaxExportFileSize {
		// Upload to object storage
		storageKey := fmt.Sprintf("contract-exports/%s-%d.%s", format, time.Now().Unix(), format)
		storageClient := storage.Get()
		err = storageClient.PutObject(ctx, storageKey, fileContent)
		if err != nil {
			global.Logger.Errorf("failed to upload export file to storage: %v", err)
			return ErrorReply(http.StatusInternalServerError, "failed to upload export file", err), err
		}

		// Generate download URL (24 hour expiry)
		downloadURL, err := storageClient.GetObjectURL(ctx, storageKey, 24*3600)
		if err != nil {
			global.Logger.Warnf("failed to generate download URL: %v", err)
			// If URL generation fails, still return base64 (though file is large)
			responseData.FileContentBase64 = base64.StdEncoding.EncodeToString(fileContent)
		} else {
			responseData.DownloadURL = downloadURL
		}
	} else {
		// Return base64 encoded content directly
		responseData.FileContentBase64 = base64.StdEncoding.EncodeToString(fileContent)
	}

	return MarshalAndReply(responseData, "marshal export response error")
}

// ImportContractInfos imports contract information from CSV or JSON file.
//
// Parameters:
//   - ctx: Context for request cancellation and timeout
//   - req: Request containing file content, format, and import options
//
// Returns:
//   - *pb.Reply: Response containing import result with success/failure counts
//   - error: Error if the operation fails
func (s *ContractInfoService) ImportContractInfos(ctx context.Context, req *pb.ImportContractInfosRequest) (*pb.Reply, error) {
	// Input validation
	if req == nil {
		return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), errors.New("nil request")
	}

	if req.Format == "" {
		return ErrorReply(http.StatusBadRequest, "format is required", errors.New("format is required")), errors.New("format is required")
	}

	if req.FileContentBase64 == "" {
		return ErrorReply(http.StatusBadRequest, "file_content_base64 is required", errors.New("file_content_base64 is required")), errors.New("file_content_base64 is required")
	}

	format := strings.ToLower(req.Format)
	if format != "csv" && format != "json" {
		return ErrorReply(http.StatusBadRequest, "format must be csv or json", errors.New("invalid format")), errors.New("invalid format")
	}

	// Decode base64
	fileContent, err := base64.StdEncoding.DecodeString(req.FileContentBase64)
	if err != nil {
		return ErrorReply(http.StatusBadRequest, "invalid base64 encoding", err), err
	}

	// Check file size
	if len(fileContent) > MaxImportFileSize {
		return ErrorReply(http.StatusBadRequest, fmt.Sprintf("file size exceeds maximum %d bytes", MaxImportFileSize), errors.New("file too large")), errors.New("file too large")
	}

	// Set default values
	importMode := req.ImportMode
	if importMode == "" {
		importMode = "upsert"
	}
	if importMode != "create" && importMode != "update" && importMode != "upsert" {
		return ErrorReply(http.StatusBadRequest, "import_mode must be create, update, or upsert", errors.New("invalid import_mode")), errors.New("invalid import_mode")
	}

	skipErrors := req.SkipErrors
	defaultChainID := req.DefaultChainId
	if defaultChainID == "" {
		defaultChainID = DefaultChainID
	}
	defaultContractType := req.DefaultContractType
	if defaultContractType == "" {
		defaultContractType = DefaultContractType
	}

	// Parse file
	var contracts []*ContractInfoImport
	if format == "csv" {
		contracts, err = s.parseCSVFile(fileContent, defaultChainID, defaultContractType)
	} else {
		contracts, err = s.parseJSONFile(fileContent, defaultChainID, defaultContractType)
	}

	if err != nil {
		global.Logger.Errorf("failed to parse import file: %v", err)
		return ErrorReply(http.StatusBadRequest, "failed to parse file", err), err
	}

	// Check record count limit
	if len(contracts) > MaxImportRecords {
		return ErrorReply(http.StatusBadRequest, fmt.Sprintf("too many records, maximum is %d", MaxImportRecords), errors.New("too many records")), errors.New("too many records")
	}

	// Batch import
	result, err := s.batchImportContracts(ctx, contracts, importMode, skipErrors)
	if err != nil {
		global.Logger.Errorf("failed to import contracts: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to import contracts", err), err
	}

	// Build response data
	responseData := &ImportContractInfosResponseData{
		TotalCount:   result.TotalCount,
		SuccessCount: result.SuccessCount,
		FailedCount:  result.FailedCount,
		CreatedCount: result.CreatedCount,
		UpdatedCount: result.UpdatedCount,
		Errors:       result.Errors,
		ImportedAt:   time.Now().Format(time.RFC3339),
	}

	// If all records failed, return error status code
	if result.SuccessCount == 0 && result.TotalCount > 0 {
		return ErrorReply(http.StatusBadRequest, "all records failed validation", errors.New("import failed")), nil
	}

	return MarshalAndReply(responseData, "marshal import response error")
}

// exportToCSV exports contracts to CSV format.
//
// Parameters:
//   - contracts: List of contracts to export
//   - includeTags: Whether to include tags in export
//   - includeMetadata: Whether to include metadata in export
//
// Returns:
//   - []byte: CSV file content
//   - error: Error if the operation fails
func (s *ContractInfoService) exportToCSV(contracts []models.ContractInfo, includeTags, includeMetadata bool) ([]byte, error) {
	var buf strings.Builder
	writer := csv.NewWriter(&buf)

	// Write UTF-8 BOM for Excel compatibility
	buf.WriteString("\xEF\xBB\xBF")

	// Write headers
	headers := []string{
		"contract_address",
		"chain_id",
		"name",
		"token_name",
		"token_symbol",
		"contract_type",
		"verification_status",
		"is_verified",
		"verified_at",
		"verified_by",
	}
	if includeMetadata {
		headers = append(headers, "metadata")
	}
	if includeTags {
		headers = append(headers, "tags")
	}

	if err := writer.Write(headers); err != nil {
		return nil, errors.Wrap(err, "write CSV header")
	}

	// Write data rows
	for _, contract := range contracts {
		row := []string{
			contract.ContractAddress,
			contract.ChainID,
			contract.Name,
			contract.TokenName,
			contract.TokenSymbol,
			contract.ContractType,
			contract.VerificationStatus,
			fmt.Sprintf("%t", contract.IsVerified),
		}

		// verified_at
		if contract.VerifiedAt != nil {
			row = append(row, contract.VerifiedAt.Format(time.RFC3339))
		} else {
			row = append(row, "")
		}

		// verified_by
		row = append(row, contract.VerifiedBy)

		// metadata
		if includeMetadata {
			if contract.Metadata != nil {
				metadataJSON, err := sonic.Marshal(contract.Metadata)
				if err != nil {
					global.Logger.Warnf("failed to marshal metadata for contract %s: %v", contract.ContractAddress, err)
					row = append(row, "")
				} else {
					row = append(row, string(metadataJSON))
				}
			} else {
				row = append(row, "")
			}
		}

		// tags
		if includeTags {
			if len(contract.Tags) > 0 {
				tagsJSON, err := sonic.Marshal(contract.Tags)
				if err != nil {
					global.Logger.Warnf("failed to marshal tags for contract %s: %v", contract.ContractAddress, err)
					row = append(row, "")
				} else {
					row = append(row, string(tagsJSON))
				}
			} else {
				row = append(row, "")
			}
		}

		if err := writer.Write(row); err != nil {
			return nil, errors.Wrap(err, "write CSV row")
		}
	}

	// Flush writer to ensure all data is written to buffer
	writer.Flush()

	return []byte(buf.String()), nil
}

// exportToJSON exports contracts to JSON format.
//
// Parameters:
//   - contracts: List of contracts to export
//   - includeTags: Whether to include tags in export
//   - includeMetadata: Whether to include metadata in export
//
// Returns:
//   - []byte: JSON file content
//   - error: Error if the operation fails
func (s *ContractInfoService) exportToJSON(contracts []models.ContractInfo, includeTags, includeMetadata bool) ([]byte, error) {
	type ExportData struct {
		Version    string                   `json:"version"`
		ExportedAt string                   `json:"exported_at"`
		TotalCount int                      `json:"total_count"`
		Contracts  []map[string]interface{} `json:"contracts"`
	}

	exportData := ExportData{
		Version:    "1.0",
		ExportedAt: time.Now().Format(time.RFC3339),
		TotalCount: len(contracts),
		Contracts:  make([]map[string]interface{}, 0, len(contracts)),
	}

	for _, contract := range contracts {
		contractMap := make(map[string]interface{})
		contractMap["contract_address"] = contract.ContractAddress
		contractMap["chain_id"] = contract.ChainID
		contractMap["name"] = contract.Name
		contractMap["token_name"] = contract.TokenName
		contractMap["token_symbol"] = contract.TokenSymbol
		contractMap["contract_type"] = contract.ContractType
		contractMap["verification_status"] = contract.VerificationStatus
		contractMap["is_verified"] = contract.IsVerified

		if contract.VerifiedAt != nil {
			contractMap["verified_at"] = contract.VerifiedAt.Format(time.RFC3339)
		}
		contractMap["verified_by"] = contract.VerifiedBy

		if includeMetadata && contract.Metadata != nil {
			contractMap["metadata"] = contract.Metadata
		}

		if includeTags && len(contract.Tags) > 0 {
			tags := make([]map[string]interface{}, 0, len(contract.Tags))
			for _, tag := range contract.Tags {
				tagMap := make(map[string]interface{})
				tagMap["tag_type"] = tag.TagType
				tagMap["tag_value"] = tag.TagValue
				tagMap["tag_color"] = tag.TagColor
				tagMap["description"] = tag.Description
				tagMap["created_by"] = tag.CreatedBy
				if !tag.CreatedAt.IsZero() {
					tagMap["created_at"] = tag.CreatedAt.Format(time.RFC3339)
				}
				tags = append(tags, tagMap)
			}
			contractMap["tags"] = tags
		}

		if !contract.CreatedAt.IsZero() {
			contractMap["created_at"] = contract.CreatedAt.Format(time.RFC3339)
		}
		if !contract.UpdatedAt.IsZero() {
			contractMap["updated_at"] = contract.UpdatedAt.Format(time.RFC3339)
		}

		exportData.Contracts = append(exportData.Contracts, contractMap)
	}

	jsonBytes, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		return nil, errors.Wrap(err, "marshal JSON")
	}

	return jsonBytes, nil
}

// parseCSVFile parses CSV file content.
//
// Parameters:
//   - content: CSV file content as bytes
//   - defaultChainID: Default chain ID to use if not specified
//   - defaultContractType: Default contract type to use if not specified
//
// Returns:
//   - []*ContractInfoImport: List of parsed contract information
//   - error: Error if parsing fails
func (s *ContractInfoService) parseCSVFile(content []byte, defaultChainID, defaultContractType string) ([]*ContractInfoImport, error) {
	reader := csv.NewReader(strings.NewReader(string(content)))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, errors.Wrap(err, "read CSV")
	}

	if len(records) == 0 {
		return nil, errors.New("CSV file is empty")
	}

	// Parse headers
	headers := records[0]
	headerMap := make(map[string]int)
	for i, header := range headers {
		headerMap[strings.ToLower(strings.TrimSpace(header))] = i
	}

	contracts := make([]*ContractInfoImport, 0, len(records)-1)

	// Parse data rows
	for rowNum := 1; rowNum < len(records); rowNum++ {
		record := records[rowNum]
		if len(record) != len(headers) {
			global.Logger.Warnf("CSV row %d has %d columns, expected %d", rowNum+1, len(record), len(headers))
			// Try to continue processing, fill missing columns with empty strings
			for len(record) < len(headers) {
				record = append(record, "")
			}
		}

		contract := &ContractInfoImport{
			ChainID:            defaultChainID,
			ContractType:       defaultContractType,
			VerificationStatus: DefaultVerificationStatus,
			Metadata:           make(map[string]interface{}),
			Tags:               make([]*TagImport, 0),
		}

		// Parse fields
		if idx, ok := headerMap["contract_address"]; ok && idx < len(record) {
			contract.ContractAddress = strings.TrimSpace(record[idx])
		}
		if idx, ok := headerMap["chain_id"]; ok && idx < len(record) && strings.TrimSpace(record[idx]) != "" {
			contract.ChainID = strings.TrimSpace(record[idx])
		}
		if idx, ok := headerMap["name"]; ok && idx < len(record) {
			contract.Name = strings.TrimSpace(record[idx])
		}
		if idx, ok := headerMap["token_name"]; ok && idx < len(record) {
			contract.TokenName = strings.TrimSpace(record[idx])
		}
		if idx, ok := headerMap["token_symbol"]; ok && idx < len(record) {
			contract.TokenSymbol = strings.TrimSpace(record[idx])
		}
		if idx, ok := headerMap["contract_type"]; ok && idx < len(record) && strings.TrimSpace(record[idx]) != "" {
			contract.ContractType = strings.TrimSpace(record[idx])
		}
		if idx, ok := headerMap["verification_status"]; ok && idx < len(record) && strings.TrimSpace(record[idx]) != "" {
			contract.VerificationStatus = strings.TrimSpace(record[idx])
		}
		if idx, ok := headerMap["is_verified"]; ok && idx < len(record) {
			contract.IsVerified = strings.ToLower(strings.TrimSpace(record[idx])) == "true"
		}
		if idx, ok := headerMap["verified_at"]; ok && idx < len(record) && strings.TrimSpace(record[idx]) != "" {
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(record[idx])); err == nil {
				contract.VerifiedAt = &t
			}
		}
		if idx, ok := headerMap["verified_by"]; ok && idx < len(record) {
			contract.VerifiedBy = strings.TrimSpace(record[idx])
		}

		// Parse metadata
		if idx, ok := headerMap["metadata"]; ok && idx < len(record) && strings.TrimSpace(record[idx]) != "" {
			metadataStr := strings.TrimSpace(record[idx])
			if err := sonic.Unmarshal([]byte(metadataStr), &contract.Metadata); err != nil {
				global.Logger.Warnf("failed to parse metadata at row %d: %v", rowNum+1, err)
				contract.Metadata = make(map[string]interface{})
			}
		}

		// Parse tags
		if idx, ok := headerMap["tags"]; ok && idx < len(record) && strings.TrimSpace(record[idx]) != "" {
			tagsStr := strings.TrimSpace(record[idx])
			var tags []map[string]interface{}
			if err := sonic.Unmarshal([]byte(tagsStr), &tags); err == nil {
				for _, tagMap := range tags {
					tag := &TagImport{}
					if tagType, ok := tagMap["tag_type"].(string); ok {
						tag.TagType = tagType
					}
					if tagValue, ok := tagMap["tag_value"].(string); ok {
						tag.TagValue = tagValue
					}
					if tagColor, ok := tagMap["tag_color"].(string); ok {
						tag.TagColor = tagColor
					}
					if description, ok := tagMap["description"].(string); ok {
						tag.Description = description
					}
					if createdBy, ok := tagMap["created_by"].(string); ok {
						tag.CreatedBy = createdBy
					}
					contract.Tags = append(contract.Tags, tag)
				}
			} else {
				global.Logger.Warnf("failed to parse tags at row %d: %v", rowNum+1, err)
			}
		}

		contracts = append(contracts, contract)
	}

	return contracts, nil
}

// parseJSONFile parses JSON file content.
//
// Parameters:
//   - content: JSON file content as bytes
//   - defaultChainID: Default chain ID to use if not specified
//   - defaultContractType: Default contract type to use if not specified
//
// Returns:
//   - []*ContractInfoImport: List of parsed contract information
//   - error: Error if parsing fails
func (s *ContractInfoService) parseJSONFile(content []byte, defaultChainID, defaultContractType string) ([]*ContractInfoImport, error) {
	var contracts []*ContractInfoImport

	// Try to parse as versioned format
	type ExportData struct {
		Version   string                   `json:"version"`
		Contracts []map[string]interface{} `json:"contracts"`
	}

	var exportData ExportData
	if err := sonic.Unmarshal(content, &exportData); err == nil && exportData.Contracts != nil {
		// Parse as export format
		for _, contractMap := range exportData.Contracts {
			contract := s.parseContractFromMap(contractMap, defaultChainID, defaultContractType)
			contracts = append(contracts, contract)
		}
	} else {
		// Try to parse as array format
		var contractArray []map[string]interface{}
		if err := sonic.Unmarshal(content, &contractArray); err == nil {
			for _, contractMap := range contractArray {
				contract := s.parseContractFromMap(contractMap, defaultChainID, defaultContractType)
				contracts = append(contracts, contract)
			}
		} else {
			return nil, errors.Wrap(err, "parse JSON")
		}
	}

	return contracts, nil
}

// parseContractFromMap parses contract information from map.
//
// Parameters:
//   - contractMap: Map containing contract data
//   - defaultChainID: Default chain ID to use if not specified
//   - defaultContractType: Default contract type to use if not specified
//
// Returns:
//   - *ContractInfoImport: Parsed contract information
func (s *ContractInfoService) parseContractFromMap(contractMap map[string]interface{}, defaultChainID, defaultContractType string) *ContractInfoImport {
	contract := &ContractInfoImport{
		ChainID:            defaultChainID,
		ContractType:       defaultContractType,
		VerificationStatus: DefaultVerificationStatus,
		Metadata:           make(map[string]interface{}),
		Tags:               make([]*TagImport, 0),
	}

	if v, ok := contractMap["contract_address"].(string); ok {
		contract.ContractAddress = v
	}
	if v, ok := contractMap["chain_id"].(string); ok && v != "" {
		contract.ChainID = v
	}
	if v, ok := contractMap["name"].(string); ok {
		contract.Name = v
	}
	if v, ok := contractMap["token_name"].(string); ok {
		contract.TokenName = v
	}
	if v, ok := contractMap["token_symbol"].(string); ok {
		contract.TokenSymbol = v
	}
	if v, ok := contractMap["contract_type"].(string); ok && v != "" {
		contract.ContractType = v
	}
	if v, ok := contractMap["verification_status"].(string); ok && v != "" {
		contract.VerificationStatus = v
	}
	if v, ok := contractMap["is_verified"].(bool); ok {
		contract.IsVerified = v
	}
	if v, ok := contractMap["verified_at"].(string); ok && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			contract.VerifiedAt = &t
		}
	}
	if v, ok := contractMap["verified_by"].(string); ok {
		contract.VerifiedBy = v
	}

	// Parse metadata
	if metadata, ok := contractMap["metadata"].(map[string]interface{}); ok {
		contract.Metadata = metadata
	}

	// Parse tags
	if tags, ok := contractMap["tags"].([]interface{}); ok {
		for _, tagInterface := range tags {
			if tagMap, ok := tagInterface.(map[string]interface{}); ok {
				tag := &TagImport{}
				if v, ok := tagMap["tag_type"].(string); ok {
					tag.TagType = v
				}
				if v, ok := tagMap["tag_value"].(string); ok {
					tag.TagValue = v
				}
				if v, ok := tagMap["tag_color"].(string); ok {
					tag.TagColor = v
				}
				if v, ok := tagMap["description"].(string); ok {
					tag.Description = v
				}
				if v, ok := tagMap["created_by"].(string); ok {
					tag.CreatedBy = v
				}
				contract.Tags = append(contract.Tags, tag)
			}
		}
	}

	return contract
}

// validateContractInfo validates contract information.
//
// Parameters:
//   - info: Contract information to validate
//   - rowNumber: Row number in the import file
//
// Returns:
//   - []*ImportErrorInfo: List of validation errors (empty if valid)
func (s *ContractInfoService) validateContractInfo(info *ContractInfoImport, rowNumber int32) []*ImportErrorInfo {
	var errors []*ImportErrorInfo

	// Check required fields
	if info.ContractAddress == "" {
		errors = append(errors, &ImportErrorInfo{
			RowNumber:    rowNumber,
			ErrorMessage: "contract_address is required",
			ErrorCode:    "MISSING_REQUIRED_FIELD",
		})
	}

	if info.ChainID == "" {
		errors = append(errors, &ImportErrorInfo{
			RowNumber:    rowNumber,
			ErrorMessage: "chain_id is required",
			ErrorCode:    "MISSING_REQUIRED_FIELD",
		})
	}

	// Address format validation (basic check)
	if info.ContractAddress != "" {
		accPrefix := types.GetConfig().GetBech32AccountAddrPrefix()
		if !strings.HasPrefix(info.ContractAddress, "0x") && !strings.HasPrefix(info.ContractAddress, accPrefix+"1") {
			errors = append(errors, &ImportErrorInfo{
				RowNumber:       rowNumber,
				ContractAddress: info.ContractAddress,
				ErrorMessage:    "invalid contract_address format",
				ErrorCode:       "INVALID_ADDRESS_FORMAT",
			})
		}
	}

	// Check length limits
	if len(info.ChainID) > 50 {
		errors = append(errors, &ImportErrorInfo{
			RowNumber:       rowNumber,
			ContractAddress: info.ContractAddress,
			ErrorMessage:    "chain_id exceeds maximum length of 50",
			ErrorCode:       "INVALID_FIELD_LENGTH",
		})
	}

	if len(info.ContractType) > 50 {
		errors = append(errors, &ImportErrorInfo{
			RowNumber:       rowNumber,
			ContractAddress: info.ContractAddress,
			ErrorMessage:    "contract_type exceeds maximum length of 50",
			ErrorCode:       "INVALID_FIELD_LENGTH",
		})
	}

	return errors
}

// batchImportContracts batch imports contracts.
//
// Parameters:
//   - ctx: Context for database operations
//   - contracts: List of contracts to import
//   - importMode: Import mode (create/update/upsert)
//   - skipErrors: Whether to skip errors and continue processing
//
// Returns:
//   - *ImportResult: Import result with success/failure counts
//   - error: Error if the operation fails
func (s *ContractInfoService) batchImportContracts(ctx context.Context, contracts []*ContractInfoImport, importMode string, skipErrors bool) (*ImportResult, error) {
	result := &ImportResult{
		TotalCount: int32(len(contracts)),
		Errors:     make([]*ImportErrorInfo, 0),
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
	defer queryCancel()

	// Process in batches
	for i := 0; i < len(contracts); i += BatchImportSize {
		end := i + BatchImportSize
		if end > len(contracts) {
			end = len(contracts)
		}

		batch := contracts[i:end]
		batchResult := s.processBatch(queryCtx, batch, importMode, skipErrors, int32(i+1))

		result.SuccessCount += batchResult.SuccessCount
		result.FailedCount += batchResult.FailedCount
		result.CreatedCount += batchResult.CreatedCount
		result.UpdatedCount += batchResult.UpdatedCount
		result.Errors = append(result.Errors, batchResult.Errors...)
	}

	return result, nil
}

// processBatch processes a batch of contracts.
//
// Parameters:
//   - ctx: Context for database operations
//   - contracts: List of contracts to process
//   - importMode: Import mode (create/update/upsert)
//   - skipErrors: Whether to skip errors and continue processing
//   - startRowNumber: Starting row number for error reporting
//
// Returns:
//   - *ImportResult: Batch processing result
func (s *ContractInfoService) processBatch(ctx context.Context, contracts []*ContractInfoImport, importMode string, skipErrors bool, startRowNumber int32) *ImportResult {
	result := &ImportResult{
		TotalCount: int32(len(contracts)),
		Errors:     make([]*ImportErrorInfo, 0),
	}

	// Use transaction
	tx := db.Get().WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for idx, contract := range contracts {
		rowNumber := startRowNumber + int32(idx)

		// Data validation
		validationErrors := s.validateContractInfo(contract, rowNumber)
		if len(validationErrors) > 0 {
			result.FailedCount++
			result.Errors = append(result.Errors, validationErrors...)
			if !skipErrors {
				tx.Rollback()
				return result
			}
			continue
		}

		// Convert address format
		contractAddress := models.ConvertAddressToEthForStorage(contract.ContractAddress)

		// Check if contract exists
		var existingContract models.ContractInfo
		err := tx.Where("chain_id = ? AND contract_address = ?", contract.ChainID, contractAddress).First(&existingContract).Error

		exists := err == nil
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			result.FailedCount++
			result.Errors = append(result.Errors, &ImportErrorInfo{
				RowNumber:       rowNumber,
				ContractAddress: contract.ContractAddress,
				ErrorMessage:    fmt.Sprintf("failed to check existing contract: %v", err),
				ErrorCode:       "DATABASE_ERROR",
			})
			if !skipErrors {
				tx.Rollback()
				return result
			}
			continue
		}

		// Process based on import_mode
		if importMode == "create" {
			if exists {
				result.FailedCount++
				result.Errors = append(result.Errors, &ImportErrorInfo{
					RowNumber:       rowNumber,
					ContractAddress: contract.ContractAddress,
					ErrorMessage:    "contract already exists",
					ErrorCode:       "CONTRACT_EXISTS",
				})
				if !skipErrors {
					tx.Rollback()
					return result
				}
				continue
			}
			// Create new contract
			if err := s.createContractInTx(tx, contract, contractAddress); err != nil {
				result.FailedCount++
				result.Errors = append(result.Errors, &ImportErrorInfo{
					RowNumber:       rowNumber,
					ContractAddress: contract.ContractAddress,
					ErrorMessage:    fmt.Sprintf("failed to create contract: %v", err),
					ErrorCode:       "CREATE_ERROR",
				})
				if !skipErrors {
					tx.Rollback()
					return result
				}
				continue
			}
			result.SuccessCount++
			result.CreatedCount++

		} else if importMode == "update" {
			if !exists {
				result.FailedCount++
				result.Errors = append(result.Errors, &ImportErrorInfo{
					RowNumber:       rowNumber,
					ContractAddress: contract.ContractAddress,
					ErrorMessage:    "contract does not exist",
					ErrorCode:       "CONTRACT_NOT_FOUND",
				})
				if !skipErrors {
					tx.Rollback()
					return result
				}
				continue
			}
			// Update existing contract
			if err := s.updateContractInTx(tx, contract, contractAddress); err != nil {
				result.FailedCount++
				result.Errors = append(result.Errors, &ImportErrorInfo{
					RowNumber:       rowNumber,
					ContractAddress: contract.ContractAddress,
					ErrorMessage:    fmt.Sprintf("failed to update contract: %v", err),
					ErrorCode:       "UPDATE_ERROR",
				})
				if !skipErrors {
					tx.Rollback()
					return result
				}
				continue
			}
			result.SuccessCount++
			result.UpdatedCount++

		} else { // upsert
			if exists {
				// Update
				if err := s.updateContractInTx(tx, contract, contractAddress); err != nil {
					result.FailedCount++
					result.Errors = append(result.Errors, &ImportErrorInfo{
						RowNumber:       rowNumber,
						ContractAddress: contract.ContractAddress,
						ErrorMessage:    fmt.Sprintf("failed to update contract: %v", err),
						ErrorCode:       "UPDATE_ERROR",
					})
					if !skipErrors {
						tx.Rollback()
						return result
					}
					continue
				}
				result.SuccessCount++
				result.UpdatedCount++
			} else {
				// Create
				if err := s.createContractInTx(tx, contract, contractAddress); err != nil {
					result.FailedCount++
					result.Errors = append(result.Errors, &ImportErrorInfo{
						RowNumber:       rowNumber,
						ContractAddress: contract.ContractAddress,
						ErrorMessage:    fmt.Sprintf("failed to create contract: %v", err),
						ErrorCode:       "CREATE_ERROR",
					})
					if !skipErrors {
						tx.Rollback()
						return result
					}
					continue
				}
				result.SuccessCount++
				result.CreatedCount++
			}
		}
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		global.Logger.Errorf("failed to commit import transaction: %v", err)
		return &ImportResult{
			FailedCount: result.TotalCount,
			Errors: []*ImportErrorInfo{
				{
					ErrorMessage: fmt.Sprintf("failed to commit transaction: %v", err),
					ErrorCode:    "TRANSACTION_ERROR",
				},
			},
		}
	}

	// Clear cache
	for _, contract := range contracts {
		if len(s.validateContractInfo(contract, 0)) == 0 {
			contractAddress := models.ConvertAddressToEthForStorage(contract.ContractAddress)
			s.clearContractCache(context.Background(), contract.ChainID, contractAddress)
		}
	}

	return result
}

// createContractInTx creates contract in transaction.
//
// Parameters:
//   - tx: Database transaction
//   - contract: Contract information to create
//   - contractAddress: Contract address in Ethereum format
//
// Returns:
//   - error: Error if the operation fails
func (s *ContractInfoService) createContractInTx(tx *gorm.DB, contract *ContractInfoImport, contractAddress string) error {
	contractInfo := &models.ContractInfo{
		ContractAddress:    contractAddress,
		Name:               contract.Name,
		TokenName:          contract.TokenName,
		TokenSymbol:        contract.TokenSymbol,
		ContractType:       contract.ContractType,
		ChainID:            contract.ChainID,
		VerificationStatus: contract.VerificationStatus,
		IsVerified:         contract.IsVerified,
		VerifiedAt:         contract.VerifiedAt,
		VerifiedBy:         contract.VerifiedBy,
		Metadata:           contract.Metadata,
	}

	if err := tx.Create(contractInfo).Error; err != nil {
		return err
	}

	// Create tags
	if len(contract.Tags) > 0 {
		for _, tagImport := range contract.Tags {
			tag := &models.ContractTag{
				ContractAddress: contractAddress,
				ChainID:         contract.ChainID,
				TagType:         tagImport.TagType,
				TagValue:        tagImport.TagValue,
				TagColor:        tagImport.TagColor,
				Description:     tagImport.Description,
				CreatedBy:       tagImport.CreatedBy,
			}
			// Use FirstOrCreate to avoid duplicates
			if err := tx.Where("chain_id = ? AND contract_address = ? AND tag_type = ?", contract.ChainID, contractAddress, tagImport.TagType).FirstOrCreate(tag).Error; err != nil {
				global.Logger.Warnf("failed to create tag %s for contract %s: %v", tagImport.TagType, contractAddress, err)
				// Continue processing other tags
			}
		}
	}

	return nil
}

// updateContractInTx updates contract in transaction.
//
// Parameters:
//   - tx: Database transaction
//   - contract: Contract information to update
//   - contractAddress: Contract address in Ethereum format
//
// Returns:
//   - error: Error if the operation fails
func (s *ContractInfoService) updateContractInTx(tx *gorm.DB, contract *ContractInfoImport, contractAddress string) error {
	updates := make(map[string]interface{})

	if contract.Name != "" {
		updates["name"] = contract.Name
	}
	if contract.TokenName != "" {
		updates["token_name"] = contract.TokenName
	}
	if contract.TokenSymbol != "" {
		updates["token_symbol"] = contract.TokenSymbol
	}
	if contract.ContractType != "" {
		updates["contract_type"] = contract.ContractType
	}
	if contract.VerificationStatus != "" {
		updates["verification_status"] = contract.VerificationStatus
		updates["is_verified"] = (contract.VerificationStatus == "verified")
	}
	if contract.VerifiedAt != nil {
		updates["verified_at"] = contract.VerifiedAt
	}
	if contract.VerifiedBy != "" {
		updates["verified_by"] = contract.VerifiedBy
	}
	if contract.Metadata != nil {
		updates["metadata"] = contract.Metadata
	}

	if len(updates) > 0 {
		if err := tx.Model(&models.ContractInfo{}).
			Where("chain_id = ? AND contract_address = ?", contract.ChainID, contractAddress).
			Updates(updates).Error; err != nil {
			return err
		}
	}

	// Update tags
	if len(contract.Tags) > 0 {
		for _, tagImport := range contract.Tags {
			tag := &models.ContractTag{
				ContractAddress: contractAddress,
				ChainID:         contract.ChainID,
				TagType:         tagImport.TagType,
				TagValue:        tagImport.TagValue,
				TagColor:        tagImport.TagColor,
				Description:     tagImport.Description,
				CreatedBy:       tagImport.CreatedBy,
			}
			// Use FirstOrCreate to avoid duplicates
			if err := tx.Where("chain_id = ? AND contract_address = ? AND tag_type = ?", contract.ChainID, contractAddress, tagImport.TagType).FirstOrCreate(tag).Error; err != nil {
				global.Logger.Warnf("failed to update tag %s for contract %s: %v", tagImport.TagType, contractAddress, err)
				// Continue processing other tags
			}
		}
	}

	return nil
}
