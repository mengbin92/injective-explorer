package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"

	"github.com/go-kratos/kratos/v2/log"
)

func TestMain(m *testing.M) {
	// Initialize global logger if not already initialized
	if global.Logger == nil {
		global.Logger = log.NewHelper(log.DefaultLogger)
	}
	m.Run()
}

// TestExportContractInfos_InputValidation tests the ExportContractInfos method input validation.
// Note: Full integration tests with database are skipped as they require DB initialization.
func TestExportContractInfos_InputValidation(t *testing.T) {
	service := NewContractInfoService()

	tests := []struct {
		name    string
		req     *pb.ExportContractInfosRequest
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil request",
			req:     nil,
			wantErr: true,
			errMsg:  "request cannot be nil",
		},
		{
			name: "empty format",
			req: &pb.ExportContractInfosRequest{
				Format: "",
			},
			wantErr: true,
			errMsg:  "format is required",
		},
		{
			name: "invalid format",
			req: &pb.ExportContractInfosRequest{
				Format: "xml",
			},
			wantErr: true,
			errMsg:  "format must be csv or json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			reply, err := service.ExportContractInfos(ctx, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ExportContractInfos() expected error but got none")
				}
				if reply != nil && reply.Code != 400 && reply.Code != 500 {
					t.Errorf("ExportContractInfos() expected error code 400 or 500, got %d", reply.Code)
				}
				if tt.errMsg != "" && reply != nil && !strings.Contains(reply.Message, tt.errMsg) {
					t.Errorf("ExportContractInfos() expected error message to contain %q, got %q", tt.errMsg, reply.Message)
				}
			}
		})
	}
}

// TestImportContractInfos_InputValidation tests the ImportContractInfos method input validation.
// Note: Full integration tests with database are skipped as they require DB initialization.
func TestImportContractInfos_InputValidation(t *testing.T) {
	service := NewContractInfoService()

	tests := []struct {
		name    string
		req     *pb.ImportContractInfosRequest
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil request",
			req:     nil,
			wantErr: true,
			errMsg:  "request cannot be nil",
		},
		{
			name: "empty format",
			req: &pb.ImportContractInfosRequest{
				Format: "",
			},
			wantErr: true,
			errMsg:  "format is required",
		},
		{
			name: "empty file content",
			req: &pb.ImportContractInfosRequest{
				Format:            "csv",
				FileContentBase64: "",
			},
			wantErr: true,
			errMsg:  "file_content_base64 is required",
		},
		{
			name: "invalid base64",
			req: &pb.ImportContractInfosRequest{
				Format:            "csv",
				FileContentBase64: "!!!invalid-base64!!!",
			},
			wantErr: true,
			errMsg:  "invalid base64 encoding",
		},
		{
			name: "invalid format",
			req: &pb.ImportContractInfosRequest{
				Format:            "xml",
				FileContentBase64: base64.StdEncoding.EncodeToString([]byte("test")),
			},
			wantErr: true,
			errMsg:  "format must be csv or json",
		},
		{
			name: "invalid import mode",
			req: &pb.ImportContractInfosRequest{
				Format:            "csv",
				FileContentBase64: base64.StdEncoding.EncodeToString([]byte("test")),
				ImportMode:        "invalid",
			},
			wantErr: true,
			errMsg:  "import_mode must be create, update, or upsert",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			reply, err := service.ImportContractInfos(ctx, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ImportContractInfos() expected error but got none")
				}
				if reply != nil && reply.Code != 400 && reply.Code != 500 {
					t.Errorf("ImportContractInfos() expected error code 400 or 500, got %d", reply.Code)
				}
				if tt.errMsg != "" && reply != nil && !strings.Contains(reply.Message, tt.errMsg) {
					t.Errorf("ImportContractInfos() expected error message to contain %q, got %q", tt.errMsg, reply.Message)
				}
			}
		})
	}
}

// TestExportToCSV tests the exportToCSV method.
func TestExportToCSV(t *testing.T) {
	service := NewContractInfoService()

	now := time.Now()
	contracts := []models.ContractInfo{
		{
			ContractAddress:    "0x1234567890abcdef1234567890abcdef12345678",
			ChainID:            "injective-1",
			Name:               "Test Contract",
			TokenName:          "Test Token",
			TokenSymbol:        "TEST",
			ContractType:       "cosmwasm",
			VerificationStatus: "verified",
			IsVerified:         true,
			VerifiedAt:         &now,
			VerifiedBy:         "admin",
			Metadata:           map[string]interface{}{"key": "value"},
		},
	}

	tests := []struct {
		name            string
		contracts       []models.ContractInfo
		includeTags     bool
		includeMetadata bool
		wantErr         bool
		checkContent    bool
	}{
		{
			name:            "basic csv export",
			contracts:       contracts,
			includeTags:     false,
			includeMetadata: false,
			wantErr:         false,
			checkContent:    true,
		},
		{
			name:            "with tags",
			contracts:       contracts,
			includeTags:     true,
			includeMetadata: false,
			wantErr:         false,
			checkContent:    true,
		},
		{
			name:            "with metadata",
			contracts:       contracts,
			includeTags:     false,
			includeMetadata: true,
			wantErr:         false,
			checkContent:    true,
		},
		{
			name:            "with tags and metadata",
			contracts:       contracts,
			includeTags:     true,
			includeMetadata: true,
			wantErr:         false,
			checkContent:    true,
		},
		{
			name:            "empty contracts",
			contracts:       []models.ContractInfo{},
			includeTags:     false,
			includeMetadata: false,
			wantErr:         false,
			checkContent:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.exportToCSV(tt.contracts, tt.includeTags, tt.includeMetadata)

			if tt.wantErr && err == nil {
				t.Errorf("exportToCSV() expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("exportToCSV() unexpected error: %v", err)
			}

			if tt.checkContent && result != nil {
				// Check for UTF-8 BOM
				if !strings.HasPrefix(string(result), "\xEF\xBB\xBF") {
					t.Errorf("exportToCSV() expected UTF-8 BOM prefix")
				}

				// Check for CSV headers
				content := string(result)
				expectedHeaders := []string{
					"contract_address",
					"chain_id",
					"name",
					"token_name",
					"token_symbol",
				}

				for _, header := range expectedHeaders {
					if !strings.Contains(content, header) {
						t.Errorf("exportToCSV() expected header %q not found", header)
					}
				}

				if tt.includeMetadata && !strings.Contains(content, "metadata") {
					t.Errorf("exportToCSV() expected metadata column when includeMetadata is true")
				}

				if tt.includeTags && !strings.Contains(content, "tags") {
					t.Errorf("exportToCSV() expected tags column when includeTags is true")
				}
			}
		})
	}
}

// TestExportToJSON tests the exportToJSON method.
func TestExportToJSON(t *testing.T) {
	service := NewContractInfoService()

	now := time.Now()
	contracts := []models.ContractInfo{
		{
			ContractAddress:    "0x1234567890abcdef1234567890abcdef12345678",
			ChainID:            "injective-1",
			Name:               "Test Contract",
			TokenName:          "Test Token",
			TokenSymbol:        "TEST",
			ContractType:       "cosmwasm",
			VerificationStatus: "verified",
			IsVerified:         true,
			VerifiedAt:         &now,
			VerifiedBy:         "admin",
			Metadata:           map[string]interface{}{"key": "value"},
		},
	}

	tests := []struct {
		name            string
		contracts       []models.ContractInfo
		includeTags     bool
		includeMetadata bool
		wantErr         bool
	}{
		{
			name:            "basic json export",
			contracts:       contracts,
			includeTags:     false,
			includeMetadata: false,
			wantErr:         false,
		},
		{
			name:            "with tags",
			contracts:       contracts,
			includeTags:     true,
			includeMetadata: false,
			wantErr:         false,
		},
		{
			name:            "with metadata",
			contracts:       contracts,
			includeTags:     false,
			includeMetadata: true,
			wantErr:         false,
		},
		{
			name:            "empty contracts",
			contracts:       []models.ContractInfo{},
			includeTags:     false,
			includeMetadata: false,
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.exportToJSON(tt.contracts, tt.includeTags, tt.includeMetadata)

			if tt.wantErr && err == nil {
				t.Errorf("exportToJSON() expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("exportToJSON() unexpected error: %v", err)
			}

			if result != nil {
				var jsonData map[string]interface{}
				if err := json.Unmarshal(result, &jsonData); err != nil {
					t.Errorf("exportToJSON() result is not valid JSON: %v", err)
				}

				// Check for required fields
				if _, ok := jsonData["version"]; !ok {
					t.Errorf("exportToJSON() expected 'version' field in JSON")
				}
				if _, ok := jsonData["exported_at"]; !ok {
					t.Errorf("exportToJSON() expected 'exported_at' field in JSON")
				}
				if _, ok := jsonData["contracts"]; !ok {
					t.Errorf("exportToJSON() expected 'contracts' field in JSON")
				}

				if tt.includeMetadata {
					// Check that metadata is included
					if contractsArray, ok := jsonData["contracts"].([]interface{}); ok && len(contractsArray) > 0 {
						if firstContract, ok := contractsArray[0].(map[string]interface{}); ok {
							if _, hasMetadata := firstContract["metadata"]; !hasMetadata {
								t.Errorf("exportToJSON() expected metadata in contracts when includeMetadata is true")
							}
						}
					}
				}
			}
		})
	}
}

// TestParseCSVFile tests the parseCSVFile method.
func TestParseCSVFile(t *testing.T) {
	service := NewContractInfoService()

	validCSV := `contract_address,chain_id,name,token_name,token_symbol,contract_type,verification_status
0x1234567890abcdef1234567890abcdef12345678,injective-1,Test Contract,Test Token,TEST,cosmwasm,verified
0xabcdefabcdefabcdefabcdefabcdefabcdefabcd,injective-1,Another,Another,ACCT,cosmwasm,unverified`

	csvWithMetadata := `contract_address,chain_id,name,metadata
0x1234567890abcdef1234567890abcdef12345678,injective-1,Test,"{""key"":""value""}"`

	csvWithTags := `contract_address,chain_id,name,tags
0x1234567890abcdef1234567890abcdef12345678,injective-1,Test,"[{""tag_type"":""verified"",""tag_value"":""yes""}]"`

	tests := []struct {
		name                 string
		content              []byte
		defaultChainID       string
		defaultContractType  string
		wantErr              bool
		checkResult          bool
		expectedContractCount int
	}{
		{
			name:                 "valid csv",
			content:              []byte(validCSV),
			defaultChainID:       "injective-1",
			defaultContractType:  "cosmwasm",
			wantErr:              false,
			checkResult:          true,
			expectedContractCount: 2,
		},
		{
			name:                 "csv with metadata",
			content:              []byte(csvWithMetadata),
			defaultChainID:       "injective-1",
			defaultContractType:  "cosmwasm",
			wantErr:              false,
			checkResult:          true,
			expectedContractCount: 1,
		},
		{
			name:                 "csv with tags",
			content:              []byte(csvWithTags),
			defaultChainID:       "injective-1",
			defaultContractType:  "cosmwasm",
			wantErr:              false,
			checkResult:          true,
			expectedContractCount: 1,
		},
		{
			name:                "empty csv",
			content:             []byte(""),
			defaultChainID:      "injective-1",
			defaultContractType: "cosmwasm",
			wantErr:             true,
		},
		{
			name:                "only headers",
			content:             []byte("contract_address,chain_id,name"),
			defaultChainID:      "injective-1",
			defaultContractType: "cosmwasm",
			wantErr:             false,
			checkResult:         true,
			expectedContractCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.parseCSVFile(tt.content, tt.defaultChainID, tt.defaultContractType)

			if tt.wantErr && err == nil {
				t.Errorf("parseCSVFile() expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("parseCSVFile() unexpected error: %v", err)
			}

			if tt.checkResult && result != nil {
				if len(result) != tt.expectedContractCount {
					t.Errorf("parseCSVFile() expected %d contracts, got %d", tt.expectedContractCount, len(result))
				}
			}
		})
	}
}

// TestParseJSONFile tests the parseJSONFile method.
func TestParseJSONFile(t *testing.T) {
	service := NewContractInfoService()

	// Versioned format
	versionedJSON := `{
		"version": "1.0",
		"contracts": [
			{
				"contract_address": "0x1234567890abcdef1234567890abcdef12345678",
				"chain_id": "injective-1",
				"name": "Test Contract",
				"token_name": "Test Token",
				"token_symbol": "TEST"
			}
		]
	}`

	// Array format
	arrayJSON := `[
		{
			"contract_address": "0x1234567890abcdef1234567890abcdef12345678",
			"chain_id": "injective-1",
			"name": "Test Contract"
		}
	]`

	// JSON with metadata
	jsonWithMetadata := `[
		{
			"contract_address": "0x1234567890abcdef1234567890abcdef12345678",
			"chain_id": "injective-1",
			"metadata": {"key": "value"}
		}
	]`

	tests := []struct {
		name                 string
		content              []byte
		defaultChainID       string
		defaultContractType  string
		wantErr              bool
		checkResult          bool
		expectedContractCount int
	}{
		{
			name:                 "versioned json format",
			content:              []byte(versionedJSON),
			defaultChainID:       "injective-1",
			defaultContractType:  "cosmwasm",
			wantErr:              false,
			checkResult:          true,
			expectedContractCount: 1,
		},
		{
			name:                 "array json format",
			content:              []byte(arrayJSON),
			defaultChainID:       "injective-1",
			defaultContractType:  "cosmwasm",
			wantErr:              false,
			checkResult:          true,
			expectedContractCount: 1,
		},
		{
			name:                 "json with metadata",
			content:              []byte(jsonWithMetadata),
			defaultChainID:       "injective-1",
			defaultContractType:  "cosmwasm",
			wantErr:              false,
			checkResult:          true,
			expectedContractCount: 1,
		},
		{
			name:                "invalid json",
			content:             []byte("{invalid json}"),
			defaultChainID:      "injective-1",
			defaultContractType: "cosmwasm",
			wantErr:             true,
		},
		{
			name:                "empty json object",
			content:             []byte("{}"),
			defaultChainID:      "injective-1",
			defaultContractType: "cosmwasm",
			wantErr:             true, // Empty object is not a valid format
		},
		{
			name:                "empty json array",
			content:             []byte("[]"),
			defaultChainID:      "injective-1",
			defaultContractType: "cosmwasm",
			wantErr:             false,
			checkResult:         true,
			expectedContractCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.parseJSONFile(tt.content, tt.defaultChainID, tt.defaultContractType)

			if tt.wantErr && err == nil {
				t.Errorf("parseJSONFile() expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("parseJSONFile() unexpected error: %v", err)
			}

			if tt.checkResult && result != nil {
				if len(result) != tt.expectedContractCount {
					t.Errorf("parseJSONFile() expected %d contracts, got %d", tt.expectedContractCount, len(result))
				}
			}
		})
	}
}

// TestValidateContractInfo tests the validateContractInfo method.
func TestValidateContractInfo(t *testing.T) {
	service := NewContractInfoService()

	validContract := &ContractInfoImport{
		ContractAddress:    "0x1234567890abcdef1234567890abcdef12345678",
		ChainID:            "injective-1",
		Name:               "Test Contract",
		ContractType:       "cosmwasm",
	}

	contractWithoutAddress := &ContractInfoImport{
		ChainID:      "injective-1",
		Name:         "Test Contract",
		ContractType: "cosmwasm",
	}

	contractWithoutChainID := &ContractInfoImport{
		ContractAddress: "0x1234567890abcdef1234567890abcdef12345678",
		Name:            "Test Contract",
		ContractType:    "cosmwasm",
	}

	contractWithInvalidAddress := &ContractInfoImport{
		ContractAddress: "invalid-address",
		ChainID:         "injective-1",
		Name:            "Test Contract",
	}

	contractWithLongChainID := &ContractInfoImport{
		ContractAddress: "0x1234567890abcdef1234567890abcdef12345678",
		ChainID:         strings.Repeat("a", 51), // 51 characters, exceeds limit
		Name:            "Test Contract",
	}

	contractWithLongContractType := &ContractInfoImport{
		ContractAddress: "0x1234567890abcdef1234567890abcdef12345678",
		ChainID:         "injective-1",
		Name:            "Test Contract",
		ContractType:    strings.Repeat("a", 51), // 51 characters, exceeds limit
	}

	injectiveAddressContract := &ContractInfoImport{
		ContractAddress: "inj1abc123",
		ChainID:         "injective-1",
		Name:            "Test Contract",
	}

	tests := []struct {
		name       string
		info       *ContractInfoImport
		rowNumber  int32
		wantErrors bool
		errorCount int
	}{
		{
			name:       "valid contract",
			info:       validContract,
			rowNumber:  1,
			wantErrors: false,
			errorCount: 0,
		},
		{
			name:       "missing contract address",
			info:       contractWithoutAddress,
			rowNumber:  1,
			wantErrors: true,
			errorCount: 1,
		},
		{
			name:       "missing chain id",
			info:       contractWithoutChainID,
			rowNumber:  1,
			wantErrors: true,
			errorCount: 1,
		},
		{
			name:       "invalid address format",
			info:       contractWithInvalidAddress,
			rowNumber:  1,
			wantErrors: true,
			errorCount: 1,
		},
		{
			name:       "chain id exceeds max length",
			info:       contractWithLongChainID,
			rowNumber:  1,
			wantErrors: true,
			errorCount: 1,
		},
		{
			name:       "contract type exceeds max length",
			info:       contractWithLongContractType,
			rowNumber:  1,
			wantErrors: true,
			errorCount: 1,
		},
		{
			name:       "injective chain address format",
			info:       injectiveAddressContract,
			rowNumber:  1,
			wantErrors: false,
			errorCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := service.validateContractInfo(tt.info, tt.rowNumber)

			if tt.wantErrors && len(errors) == 0 {
				t.Errorf("validateContractInfo() expected errors but got none")
			}
			if !tt.wantErrors && len(errors) > 0 {
				t.Errorf("validateContractInfo() unexpected errors: %v", errors)
			}
			if tt.errorCount > 0 && len(errors) != tt.errorCount {
				t.Errorf("validateContractInfo() expected %d errors, got %d", tt.errorCount, len(errors))
			}

			// Verify error fields are set correctly
			for _, err := range errors {
				if err.RowNumber != tt.rowNumber {
					t.Errorf("validateContractInfo() expected row number %d, got %d", tt.rowNumber, err.RowNumber)
				}
				if err.ErrorMessage == "" {
					t.Errorf("validateContractInfo() expected error message, got empty string")
				}
				if err.ErrorCode == "" {
					t.Errorf("validateContractInfo() expected error code, got empty string")
				}
			}
		})
	}
}

// TestParseContractFromMap tests the parseContractFromMap method.
func TestParseContractFromMap(t *testing.T) {
	service := NewContractInfoService()

	completeMap := map[string]interface{}{
		"contract_address":     "0x1234567890abcdef1234567890abcdef12345678",
		"chain_id":             "injective-1",
		"name":                 "Test Contract",
		"token_name":           "Test Token",
		"token_symbol":         "TEST",
		"contract_type":        "cosmwasm",
		"verification_status":  "verified",
		"is_verified":          true,
		"verified_at":          "2024-01-01T00:00:00Z",
		"verified_by":          "admin",
		"metadata":             map[string]interface{}{"key": "value"},
		"tags":                 []interface{}{map[string]interface{}{"tag_type": "verified", "tag_value": "yes"}},
	}

	minimalMap := map[string]interface{}{
		"contract_address": "0x1234567890abcdef1234567890abcdef12345678",
	}

	tests := []struct {
		name                 string
		contractMap          map[string]interface{}
		defaultChainID       string
		defaultContractType  string
		checkFields          bool
		expectedAddress      string
		expectedChainID      string
		expectedContractType string
	}{
		{
			name:                 "complete map",
			contractMap:          completeMap,
			defaultChainID:       "default-chain",
			defaultContractType:  "default-type",
			checkFields:          true,
			expectedAddress:      "0x1234567890abcdef1234567890abcdef12345678",
			expectedChainID:      "injective-1",
			expectedContractType: "cosmwasm",
		},
		{
			name:                 "minimal map with defaults",
			contractMap:          minimalMap,
			defaultChainID:       "default-chain",
			defaultContractType:  "default-type",
			checkFields:          true,
			expectedAddress:      "0x1234567890abcdef1234567890abcdef12345678",
			expectedChainID:      "default-chain",
			expectedContractType: "default-type",
		},
		{
			name:                 "empty map",
			contractMap:          map[string]interface{}{},
			defaultChainID:       "default-chain",
			defaultContractType:  "default-type",
			checkFields:          true,
			expectedAddress:      "",
			expectedChainID:      "default-chain",
			expectedContractType: "default-type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.parseContractFromMap(tt.contractMap, tt.defaultChainID, tt.defaultContractType)

			if result == nil {
				t.Errorf("parseContractFromMap() expected result but got nil")
				return
			}

			if tt.checkFields {
				if result.ContractAddress != tt.expectedAddress {
					t.Errorf("parseContractFromMap() expected address %s, got %s", tt.expectedAddress, result.ContractAddress)
				}
				if result.ChainID != tt.expectedChainID {
					t.Errorf("parseContractFromMap() expected chain_id %s, got %s", tt.expectedChainID, result.ChainID)
				}
				if result.ContractType != tt.expectedContractType {
					t.Errorf("parseContractFromMap() expected contract_type %s, got %s", tt.expectedContractType, result.ContractType)
				}

				// Check metadata is initialized
				if result.Metadata == nil {
					t.Errorf("parseContractFromMap() expected metadata to be initialized")
				}

				// Check tags is initialized
				if result.Tags == nil {
					t.Errorf("parseContractFromMap() expected tags to be initialized")
				}
			}
		})
	}
}
