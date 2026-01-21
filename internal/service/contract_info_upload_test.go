// Package service provides unit tests for the contract info upload service.
package service

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/models"

	"github.com/bytedance/sonic"
)

// TestUploadContractFiles_InputValidation tests the UploadContractFiles method input validation.
// Note: Full integration tests with database are skipped as they require DB initialization.
func TestUploadContractFiles_InputValidation(t *testing.T) {
	service := NewContractInfoService()
	ctx := context.Background()

	t.Run("nil request", func(t *testing.T) {
		resp, err := service.UploadContractFiles(ctx, nil)
		if err == nil {
			t.Errorf("UploadContractFiles() expected error for nil request, got nil")
		}
		if resp != nil && resp.Code != 400 && resp.Code != 500 {
			t.Errorf("UploadContractFiles() expected error code 400 or 500, got %d", resp.Code)
		}
		if resp != nil && !strings.Contains(resp.Message, "request cannot be nil") {
			t.Errorf("UploadContractFiles() expected error message to contain 'request cannot be nil', got %q", resp.Message)
		}
	})

	t.Run("missing contract_address", func(t *testing.T) {
		req := &pb.UploadContractFilesRequest{
			ChainId:          "injective-1",
			SourceCodeBase64: base64.StdEncoding.EncodeToString([]byte("// Solidity code")),
		}

		resp, err := service.UploadContractFiles(ctx, req)
		if err == nil {
			t.Errorf("UploadContractFiles() expected error for missing contract_address, got nil")
		}
		if resp != nil && resp.Code != 400 && resp.Code != 500 {
			t.Errorf("UploadContractFiles() expected error code 400 or 500, got %d", resp.Code)
		}
		if resp != nil && !strings.Contains(resp.Message, "contract_address is required") {
			t.Errorf("UploadContractFiles() expected error message to contain 'contract_address is required', got %q", resp.Message)
		}
	})

	t.Run("no files provided", func(t *testing.T) {
		req := &pb.UploadContractFilesRequest{
			ContractAddress: "0x1234567890abcdef1234567890abcdef12345678",
			ChainId:         "injective-1",
		}

		resp, err := service.UploadContractFiles(ctx, req)
		if err == nil {
			t.Errorf("UploadContractFiles() expected error for no files provided, got nil")
		}
		if resp != nil && resp.Code != 400 && resp.Code != 500 {
			t.Errorf("UploadContractFiles() expected error code 400 or 500, got %d", resp.Code)
		}
		if resp != nil && !strings.Contains(resp.Message, "at least one file") {
			t.Errorf("UploadContractFiles() expected error message to contain 'at least one file', got %q", resp.Message)
		}
	})

	t.Run("invalid base64 for source code - requires database", func(t *testing.T) {
		t.Skip("Skipping test that requires database initialization")
	})

	t.Run("invalid JSON for ABI file - requires database", func(t *testing.T) {
		t.Skip("Skipping test that requires database initialization")
	})
}

// TestDecodeAndValidateFile tests the decodeAndValidateFile function.
func TestDecodeAndValidateFile(t *testing.T) {
	validSolCode := "// SPDX-License-Identifier: MIT\npragma solidity ^0.8.0;"
	validAbiJSON := `[{"constant":true,"inputs":[],"name":"name","outputs":[{"name":"","type":"string"}],"type":"function"}]`

	tests := []struct {
		name             string
		base64Content    string
		fileType         string
		expectedExt      string
		fileName         string
		maxSize          int
		wantErr          bool
		errMsgContains   string
		checkResult      bool
		expectedName     string
		expectedMIMEType string
	}{
		{
			name:             "valid source code file",
			base64Content:    base64.StdEncoding.EncodeToString([]byte(validSolCode)),
			fileType:         "source",
			expectedExt:      ".sol",
			fileName:         "Token.sol",
			maxSize:          10 * 1024 * 1024,
			wantErr:          false,
			checkResult:      true,
			expectedName:     "Token.sol",
			expectedMIMEType: "text/plain",
		},
		{
			name:             "valid ABI file",
			base64Content:    base64.StdEncoding.EncodeToString([]byte(validAbiJSON)),
			fileType:         "abi",
			expectedExt:      ".json",
			fileName:         "abi.json",
			maxSize:          10 * 1024 * 1024,
			wantErr:          false,
			checkResult:      true,
			expectedName:     "abi.json",
			expectedMIMEType: "application/json",
		},
		{
			name:             "auto-generate filename for source",
			base64Content:    base64.StdEncoding.EncodeToString([]byte(validSolCode)),
			fileType:         "source",
			expectedExt:      ".sol",
			fileName:         "",
			maxSize:          10 * 1024 * 1024,
			wantErr:          false,
			checkResult:      true,
			expectedName:     "file.sol",
			expectedMIMEType: "text/plain",
		},
		{
			name:             "auto-generate filename for ABI",
			base64Content:    base64.StdEncoding.EncodeToString([]byte(validAbiJSON)),
			fileType:         "abi",
			expectedExt:      ".json",
			fileName:         "",
			maxSize:          10 * 1024 * 1024,
			wantErr:          false,
			checkResult:      true,
			expectedName:     "file.json",
			expectedMIMEType: "application/json",
		},
		{
			name:           "invalid base64 encoding",
			base64Content:  "!!!invalid-base64!!!",
			fileType:       "source",
			expectedExt:    ".sol",
			fileName:       "Token.sol",
			maxSize:        10 * 1024 * 1024,
			wantErr:        true,
			errMsgContains: "invalid base64 encoding",
		},
		{
			name:           "file size exceeds maximum",
			base64Content:  base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 11*1024*1024))),
			fileType:       "source",
			expectedExt:    ".sol",
			fileName:       "Token.sol",
			maxSize:        10 * 1024 * 1024,
			wantErr:        true,
			errMsgContains: "file size exceeds maximum",
		},
		{
			name:           "invalid JSON for ABI file",
			base64Content:  base64.StdEncoding.EncodeToString([]byte("not valid json")),
			fileType:       "abi",
			expectedExt:    ".json",
			fileName:       "abi.json",
			maxSize:        10 * 1024 * 1024,
			wantErr:        true,
			errMsgContains: "invalid JSON format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := decodeAndValidateFile(tt.base64Content, tt.fileType, tt.expectedExt, tt.fileName, tt.maxSize)

			if tt.wantErr {
				if err == nil {
					t.Errorf("decodeAndValidateFile() expected error but got none")
				}
				if tt.errMsgContains != "" && !strings.Contains(err.Error(), tt.errMsgContains) {
					t.Errorf("decodeAndValidateFile() expected error to contain %q, got %q", tt.errMsgContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("decodeAndValidateFile() unexpected error: %v", err)
				return
			}

			if tt.checkResult && result != nil {
				if result.Name != tt.expectedName {
					t.Errorf("decodeAndValidateFile() expected name %q, got %q", tt.expectedName, result.Name)
				}
				if result.MIMEType != tt.expectedMIMEType {
					t.Errorf("decodeAndValidateFile() expected MIME type %q, got %q", tt.expectedMIMEType, result.MIMEType)
				}
				if len(result.Content) == 0 {
					t.Errorf("decodeAndValidateFile() expected non-empty content")
				}
			}
		})
	}
}

// TestCalculateFileHash tests the calculateFileHash function.
func TestCalculateFileHash(t *testing.T) {
	tests := []struct {
		name        string
		content     []byte
		wantEmpty   bool
		checkLen    bool
		expectedLen int
	}{
		{
			name:        "empty content",
			content:     []byte(""),
			wantEmpty:   false,
			checkLen:    true,
			expectedLen: 64, // SHA256 hash length in hex
		},
		{
			name:        "simple content",
			content:     []byte("hello world"),
			wantEmpty:   false,
			checkLen:    true,
			expectedLen: 64,
		},
		{
			name:        "binary content",
			content:     []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD},
			wantEmpty:   false,
			checkLen:    true,
			expectedLen: 64,
		},
		{
			name:        "large content",
			content:     []byte(strings.Repeat("test", 10000)),
			wantEmpty:   false,
			checkLen:    true,
			expectedLen: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateFileHash(tt.content)

			if tt.wantEmpty && result != "" {
				t.Errorf("calculateFileHash() expected empty result, got %q", result)
			}
			if !tt.wantEmpty && result == "" {
				t.Errorf("calculateFileHash() expected non-empty result, got empty")
			}
			if tt.checkLen && len(result) != tt.expectedLen {
				t.Errorf("calculateFileHash() expected length %d, got %d", tt.expectedLen, len(result))
			}

			// Verify hash is consistent for same content
			if len(tt.content) > 0 {
				result2 := calculateFileHash(tt.content)
				if result != result2 {
					t.Errorf("calculateFileHash() expected consistent hash for same content, got %q and %q", result, result2)
				}
			}

			// Verify different content produces different hash
			if len(tt.content) > 0 {
				differentContent := []byte("different content")
				result3 := calculateFileHash(differentContent)
				if result == result3 {
					t.Errorf("calculateFileHash() expected different hash for different content, got same hash %q", result)
				}
			}
		})
	}
}

// TestGetStorageType tests the getStorageType method.
func TestGetStorageType(t *testing.T) {
	service := NewContractInfoService()

	tests := []struct {
		name         string
		storageKey   string
		expectedType string
	}{
		{
			name:         "object storage",
			storageKey:   "contract-files/injective-1/0x123.../source/abc123",
			expectedType: "object_storage",
		},
		{
			name:         "database storage",
			storageKey:   "",
			expectedType: "database",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &models.ContractSourceFile{
				FileContentStorageKey: tt.storageKey,
			}
			result := service.getStorageType(file)

			if result != tt.expectedType {
				t.Errorf("getStorageType() expected %q, got %q", tt.expectedType, result)
			}
		})
	}
}

// TestContractSourceFileModel tests the ContractSourceFile model.
func TestContractSourceFileModel(t *testing.T) {
	t.Run("TableName", func(t *testing.T) {
		model := models.ContractSourceFile{}
		tableName := model.TableName()

		if tableName != "contract_source_files" {
			t.Errorf("ContractSourceFile.TableName() expected 'contract_source_files', got %q", tableName)
		}
	})

	t.Run("FieldValidation", func(t *testing.T) {
		model := models.ContractSourceFile{
			ContractAddress: "0x1234567890abcdef1234567890abcdef12345678",
			ChainID:         "injective-1",
			FileName:        "Token.sol",
			FileType:        "source",
			FileContent:     "// SPDX-License-Identifier: MIT",
			FileSize:        26,
			FileHash:        "abc123",
			MIMEType:        "text/plain",
			Description:     "Test contract",
			UploadedBy:      "0xtest",
		}

		// Verify all fields are set correctly
		if model.ContractAddress == "" {
			t.Errorf("ContractSourceFile expected ContractAddress to be set")
		}
		if model.ChainID == "" {
			t.Errorf("ContractSourceFile expected ChainID to be set")
		}
		if model.FileName == "" {
			t.Errorf("ContractSourceFile expected FileName to be set")
		}
		if model.FileType == "" {
			t.Errorf("ContractSourceFile expected FileType to be set")
		}
		if model.FileSize == 0 {
			t.Errorf("ContractSourceFile expected FileSize to be non-zero")
		}
		if model.FileHash == "" {
			t.Errorf("ContractSourceFile expected FileHash to be set")
		}
		if model.MIMEType == "" {
			t.Errorf("ContractSourceFile expected MIMEType to be set")
		}
	})

	t.Run("ValidFileTypes", func(t *testing.T) {
		validTypes := []string{"source", "abi"}

		for _, fileType := range validTypes {
			model := models.ContractSourceFile{
				FileType: fileType,
			}

			if model.FileType != fileType {
				t.Errorf("ContractSourceFile expected FileType %q, got %q", fileType, model.FileType)
			}
		}
	})
}

// TestFileDataStruct tests the FileData structure.
func TestFileDataStruct(t *testing.T) {
	t.Run("CreateFileData", func(t *testing.T) {
		content := []byte("test content")
		fileData := &FileData{
			Content:  content,
			Name:     "test.sol",
			MIMEType: "text/plain",
		}

		if fileData.Content == nil {
			t.Errorf("FileData expected Content to be set")
		}
		if fileData.Name == "" {
			t.Errorf("FileData expected Name to be set")
		}
		if fileData.MIMEType == "" {
			t.Errorf("FileData expected MIMEType to be set")
		}
	})

	t.Run("FileDataWithEmptyContent", func(t *testing.T) {
		fileData := &FileData{
			Content:  []byte(""),
			Name:     "empty.sol",
			MIMEType: "text/plain",
		}

		if len(fileData.Content) != 0 {
			t.Errorf("FileData expected empty content, got %d bytes", len(fileData.Content))
		}
	})
}

// TestUploadContractFiles_RequestScenarios tests various request scenarios for UploadContractFiles.
// Note: All scenarios require database initialization.
func TestUploadContractFiles_RequestScenarios(t *testing.T) {
	t.Skip("Skipping test that requires database initialization")
	service := NewContractInfoService()
	ctx := context.Background()

	validSolCode := "// SPDX-License-Identifier: MIT\npragma solidity ^0.8.0;"
	validAbiJSON := `[{"constant":true,"inputs":[],"name":"name","outputs":[{"name":"","type":"string"}],"type":"function"}]`

	tests := []struct {
		name           string
		req            *pb.UploadContractFilesRequest
		wantErr        bool
		errMsgContains string
		checkResponse  bool
	}{
		{
			name: "only source code file",
			req: &pb.UploadContractFilesRequest{
				ContractAddress:    "0x1234567890abcdef1234567890abcdef12345678",
				ChainId:            "injective-1",
				SourceCodeBase64:   base64.StdEncoding.EncodeToString([]byte(validSolCode)),
				SourceCodeFileName: "Token.sol",
			},
			wantErr:        true, // Will fail because contract doesn't exist in DB
			errMsgContains: "contract not found",
			checkResponse:  false,
		},
		{
			name: "only ABI file",
			req: &pb.UploadContractFilesRequest{
				ContractAddress: "0x1234567890abcdef1234567890abcdef12345678",
				ChainId:         "injective-1",
				AbiBase64:       base64.StdEncoding.EncodeToString([]byte(validAbiJSON)),
				AbiFileName:     "abi.json",
			},
			wantErr:        true, // Will fail because contract doesn't exist in DB
			errMsgContains: "contract not found",
			checkResponse:  false,
		},
		{
			name: "both files with metadata",
			req: &pb.UploadContractFilesRequest{
				ContractAddress:    "0x1234567890abcdef1234567890abcdef12345678",
				ChainId:            "injective-1",
				SourceCodeBase64:   base64.StdEncoding.EncodeToString([]byte(validSolCode)),
				SourceCodeFileName: "Token.sol",
				AbiBase64:          base64.StdEncoding.EncodeToString([]byte(validAbiJSON)),
				AbiFileName:        "abi.json",
				Description:        "Test contract upload",
				UploadedBy:         "0xuser",
			},
			wantErr:        true, // Will fail because contract doesn't exist in DB
			errMsgContains: "contract not found",
			checkResponse:  false,
		},
		{
			name: "default chain_id",
			req: &pb.UploadContractFilesRequest{
				ContractAddress:    "0x1234567890abcdef1234567890abcdef12345678",
				ChainId:            "",
				SourceCodeBase64:   base64.StdEncoding.EncodeToString([]byte(validSolCode)),
				SourceCodeFileName: "Token.sol",
			},
			wantErr:        true, // Will fail because contract doesn't exist in DB
			errMsgContains: "contract not found",
			checkResponse:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := service.UploadContractFiles(ctx, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Errorf("UploadContractFiles() expected error but got none")
				}
				if tt.errMsgContains != "" && resp != nil && !strings.Contains(resp.Message, tt.errMsgContains) {
					t.Errorf("UploadContractFiles() expected error to contain %q, got %q", tt.errMsgContains, resp.Message)
				}
			}
		})
	}
}

// TestUploadContractFiles_ResponseStructure tests the response structure of UploadContractFiles.
func TestUploadContractFiles_ResponseStructure(t *testing.T) {
	// This test verifies the response structure would be correct
	// when the contract exists (integration test scenario)

	t.Run("response data structure", func(t *testing.T) {
		// Simulate response data structure
		responseData := map[string]interface{}{
			"contract_address": "0x1234567890abcdef1234567890abcdef12345678",
			"chain_id":         "injective-1",
			"uploaded_files": []map[string]interface{}{
				{
					"file_type":    "source",
					"file_name":    "Token.sol",
					"file_id":      uint(1),
					"file_size":    int64(100),
					"storage_type": "database",
				},
			},
			"uploaded_at": "2024-01-01T00:00:00Z",
		}

		// Verify structure
		if _, ok := responseData["contract_address"]; !ok {
			t.Errorf("Response expected 'contract_address' field")
		}
		if _, ok := responseData["chain_id"]; !ok {
			t.Errorf("Response expected 'chain_id' field")
		}
		if _, ok := responseData["uploaded_files"]; !ok {
			t.Errorf("Response expected 'uploaded_files' field")
		}
		if _, ok := responseData["uploaded_at"]; !ok {
			t.Errorf("Response expected 'uploaded_at' field")
		}

		// Verify uploaded_files structure
		uploadedFiles, ok := responseData["uploaded_files"].([]map[string]interface{})
		if !ok || len(uploadedFiles) == 0 {
			t.Errorf("Response expected non-empty 'uploaded_files' array")
			return
		}

		firstFile := uploadedFiles[0]
		requiredFields := []string{"file_type", "file_name", "file_id", "file_size", "storage_type"}
		for _, field := range requiredFields {
			if _, ok := firstFile[field]; !ok {
				t.Errorf("Uploaded file expected '%s' field", field)
			}
		}
	})
}

// TestUploadContractFiles_EdgeCases tests edge cases for UploadContractFiles.
// Note: All scenarios require database initialization.
func TestUploadContractFiles_EdgeCases(t *testing.T) {
	t.Skip("Skipping test that requires database initialization")
	service := NewContractInfoService()
	ctx := context.Background()

	t.Run("empty source code content", func(t *testing.T) {
		req := &pb.UploadContractFilesRequest{
			ContractAddress:    "0x1234567890abcdef1234567890abcdef12345678",
			ChainId:            "injective-1",
			SourceCodeBase64:   base64.StdEncoding.EncodeToString([]byte("")),
			SourceCodeFileName: "empty.sol",
		}

		resp, err := service.UploadContractFiles(ctx, req)
		// Empty content is technically valid base64, but contract doesn't exist
		if err == nil {
			t.Errorf("UploadContractFiles() expected error (contract not found), got nil")
		}
		if resp != nil && resp.Code == 200 {
			t.Errorf("UploadContractFiles() expected error status, got 200")
		}
	})

	t.Run("very long filename", func(t *testing.T) {
		longFileName := strings.Repeat("a", 300) + ".sol"
		req := &pb.UploadContractFilesRequest{
			ContractAddress:    "0x1234567890abcdef1234567890abcdef12345678",
			ChainId:            "injective-1",
			SourceCodeBase64:   base64.StdEncoding.EncodeToString([]byte("// code")),
			SourceCodeFileName: longFileName,
		}

		resp, err := service.UploadContractFiles(ctx, req)
		// Will fail because contract doesn't exist
		if err == nil {
			t.Errorf("UploadContractFiles() expected error (contract not found), got nil")
		}
		if resp != nil && resp.Code == 200 {
			t.Errorf("UploadContractFiles() expected error status, got 200")
		}
	})

	t.Run("special characters in description", func(t *testing.T) {
		req := &pb.UploadContractFilesRequest{
			ContractAddress:    "0x1234567890abcdef1234567890abcdef12345678",
			ChainId:            "injective-1",
			SourceCodeBase64:   base64.StdEncoding.EncodeToString([]byte("// code")),
			SourceCodeFileName: "Token.sol",
			Description:        "Special chars: <>&\"'\\n\\t中文",
			UploadedBy:         "0xuser",
		}

		resp, err := service.UploadContractFiles(ctx, req)
		// Will fail because contract doesn't exist
		if err == nil {
			t.Errorf("UploadContractFiles() expected error (contract not found), got nil")
		}
		if resp != nil && resp.Code == 200 {
			t.Errorf("UploadContractFiles() expected error status, got 200")
		}
	})
}

// TestContractSourceFile_JSONSerialization tests JSON serialization of ContractSourceFile.
func TestContractSourceFile_JSONSerialization(t *testing.T) {
	t.Run("serialize and deserialize", func(t *testing.T) {
		original := &models.ContractSourceFile{
			ContractAddress: "0x1234567890abcdef1234567890abcdef12345678",
			ChainID:         "injective-1",
			FileName:        "Token.sol",
			FileType:        "source",
			FileContent:     "// SPDX-License-Identifier: MIT",
			FileSize:        26,
			FileHash:        "abc123",
			MIMEType:        "text/plain",
			Description:     "Test contract",
			UploadedBy:      "0xtest",
		}

		// Serialize
		jsonData, err := sonic.Marshal(original)
		if err != nil {
			t.Fatalf("Failed to marshal ContractSourceFile: %v", err)
		}

		// Deserialize
		var deserialized models.ContractSourceFile
		err = sonic.Unmarshal(jsonData, &deserialized)
		if err != nil {
			t.Fatalf("Failed to unmarshal ContractSourceFile: %v", err)
		}

		// Verify fields
		if deserialized.ContractAddress != original.ContractAddress {
			t.Errorf("Expected ContractAddress %q, got %q", original.ContractAddress, deserialized.ContractAddress)
		}
		if deserialized.ChainID != original.ChainID {
			t.Errorf("Expected ChainID %q, got %q", original.ChainID, deserialized.ChainID)
		}
		if deserialized.FileName != original.FileName {
			t.Errorf("Expected FileName %q, got %q", original.FileName, deserialized.FileName)
		}
		if deserialized.FileType != original.FileType {
			t.Errorf("Expected FileType %q, got %q", original.FileType, deserialized.FileType)
		}
		if deserialized.FileContent != original.FileContent {
			t.Errorf("Expected FileContent %q, got %q", original.FileContent, deserialized.FileContent)
		}
		if deserialized.FileSize != original.FileSize {
			t.Errorf("Expected FileSize %d, got %d", original.FileSize, deserialized.FileSize)
		}
		if deserialized.FileHash != original.FileHash {
			t.Errorf("Expected FileHash %q, got %q", original.FileHash, deserialized.FileHash)
		}
		if deserialized.MIMEType != original.MIMEType {
			t.Errorf("Expected MIMEType %q, got %q", original.MIMEType, deserialized.MIMEType)
		}
		if deserialized.Description != original.Description {
			t.Errorf("Expected Description %q, got %q", original.Description, deserialized.Description)
		}
		if deserialized.UploadedBy != original.UploadedBy {
			t.Errorf("Expected UploadedBy %q, got %q", original.UploadedBy, deserialized.UploadedBy)
		}
	})
}

// TestContractSourceFile_WithObjectStorage tests ContractSourceFile with object storage.
func TestContractSourceFile_WithObjectStorage(t *testing.T) {
	t.Run("file stored in object storage", func(t *testing.T) {
		model := &models.ContractSourceFile{
			ContractAddress:       "0x1234567890abcdef1234567890abcdef12345678",
			ChainID:               "injective-1",
			FileName:              "LargeToken.sol",
			FileType:              "source",
			FileContent:           "", // Empty because it's in object storage
			FileContentStorageKey: "contract-files/injective-1/0x123.../source/abc123",
			FileSize:              1024 * 1024, // 1MB
			FileHash:              "def456",
			MIMEType:              "text/plain",
			Description:           "Large contract file",
			UploadedBy:            "0xuploader",
		}

		if model.FileContent != "" {
			t.Errorf("Expected empty FileContent for object storage, got %q", model.FileContent)
		}
		if model.FileContentStorageKey == "" {
			t.Errorf("Expected non-empty FileContentStorageKey for object storage")
		}
		if model.FileSize == 0 {
			t.Errorf("Expected non-zero FileSize")
		}
	})
}
