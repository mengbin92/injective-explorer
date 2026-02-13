// Package service provides unit tests for Cosmos event processing functions.
package service

import (
	"errors"
	"testing"

	github_com_cosmos_cosmos_sdk_types "github.com/cosmos/cosmos-sdk/types"
	"injective-explorer/internal/utils"
	"github.com/stretchr/testify/assert"
)

// TestConstants_EventTypes verifies event constant values
func TestConstants_EventTypes(t *testing.T) {
	assert.Equal(t, "injective.erc20.v1beta1.EventCreateTokenPair", EventCreateTokenPair)
	assert.Equal(t, "erc20_address", EventAttributeERC20Address)
	assert.Equal(t, "sender", EventAttributeSender)
	assert.Equal(t, "bank_denom", EventAttributeBankDenom)
	assert.Equal(t, "MsgCreateTokenPair", CreationSourceMsgCreateTokenPair)
	assert.Equal(t, 18, DefaultERC20Decimals)
}

// TestMetadataJob_Struct verifies MetadataJob structure
func TestMetadataJob_Struct(t *testing.T) {
	job := MetadataJob{
		ContractAddress: "0x1234",
		ChainID:         "test-chain-1",
	}
	assert.Equal(t, "0x1234", job.ContractAddress)
	assert.Equal(t, "test-chain-1", job.ChainID)
}

// TestIsDuplicateKeyError tests duplicate key error detection
func TestIsDuplicateKeyError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{"MySQL duplicate", "Error 1062: Duplicate entry 'test' for key 'PRIMARY'", true},
		{"PostgreSQL unique", "unique constraint contract_address_chain_id_key", true},
		{"SQLite conflict", "UNIQUE constraint failed: contract_infos.contract_address", true},
		{"connection error", "connection refused", false},
		{"not found error", "record not found", false},
		{"nil error", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.errMsg != "" {
				err = errors.New(tt.errMsg)
			}
			// Note: isDuplicateKeyError is not exported, testing via utils package
			assert.Equal(t, tt.expected, utils.IsDuplicateKeyError(err))
		})
	}
}

// TestIsRetryableError_Subset tests retryable error detection
func TestIsRetryableError_Subset(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{"connection refused", "connection refused", true},
		{"service unavailable", "code = Unavailable", true},
		{"503 error", "503 Service Unavailable", true},
		{"validation error", "invalid address", false},
		{"not found", "record not found", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.errMsg)
			assert.Equal(t, tt.expected, utils.IsRetryableError(err))
		})
	}
}

// TestBuildContractInfo_InvalidAddress tests error handling for invalid address
func TestBuildContractInfo_InvalidAddress(t *testing.T) {
	service := &SyncService{}

	eventData := &contractEventData{
		contractAddress: "invalid-address",
	}
	txResp := &github_com_cosmos_cosmos_sdk_types.TxResponse{Height: 1, TxHash: "ABC"}

	_, err := service.buildContractInfo(eventData, txResp, "test-chain")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid contract address")
}

// TestBuildContractInfo_EmptyAddress tests error handling for empty address
func TestBuildContractInfo_EmptyAddress(t *testing.T) {
	service := &SyncService{}

	eventData := &contractEventData{
		contractAddress: "",
	}
	txResp := &github_com_cosmos_cosmos_sdk_types.TxResponse{Height: 1, TxHash: "ABC"}

	_, err := service.buildContractInfo(eventData, txResp, "test-chain")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "contract address is required")
}

// TestPersistContractInfo_NilInput tests error handling for nil contract info
func TestPersistContractInfo_NilInput(t *testing.T) {
	service := &SyncService{}

	err := service.persistContractInfo(nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "contract info cannot be nil")
}

// TestContractEventData_Struct verifies contractEventData structure
func TestContractEventData_Struct(t *testing.T) {
	data := &contractEventData{
		contractAddress: "0x1234",
		creatorAddress:  "0x5678",
		bankDenom:       "peggy0xabcd",
	}

	assert.Equal(t, "0x1234", data.contractAddress)
	assert.Equal(t, "0x5678", data.creatorAddress)
	assert.Equal(t, "peggy0xabcd", data.bankDenom)
}
