// Package service provides the contract verification service.
package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/chain"
	"injective-explorer/provider/db"
	"injective-explorer/provider/storage"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// ContractVerificationService handles contract verification logic.
// It manages the full lifecycle of contract verification: submitting,
// compiling, comparing bytecodes, and updating verification status.
type ContractVerificationService struct {
	evmClient *chain.EVMClient
	compiler  ContractCompiler
	storage   storage.Storage
}

// ContractVerificationConfig holds configuration for the verification service.
type ContractVerificationConfig struct {
	// SolidityCompiler config
	SolidityCompilerConfig *SolidityCompilerConfig
}

// NewContractVerificationService creates a new contract verification service instance.
func NewContractVerificationService(
	evmClient *chain.EVMClient,
	storageProvider storage.Storage,
	config *ContractVerificationConfig,
) *ContractVerificationService {
	var compilerConfig *SolidityCompilerConfig
	if config != nil && config.SolidityCompilerConfig != nil {
		compilerConfig = config.SolidityCompilerConfig
	}

	return &ContractVerificationService{
		evmClient: evmClient,
		compiler:  NewSolidityCompiler(compilerConfig),
		storage:   storageProvider,
	}
}

// SubmitVerificationRequest holds the parameters for submitting a contract verification.
type SubmitVerificationRequest struct {
	ContractAddress     string `json:"contract_address"`
	ChainID             string `json:"chain_id"`
	ContractType        string `json:"contract_type"` // solidity / cosmwasm
	SourceCode          string `json:"source_code"`   // Solidity source or JSON standard input
	CompilerVersion     string `json:"compiler_version"`
	OptimizationEnabled bool   `json:"optimization_enabled"`
	OptimizationRuns    int    `json:"optimization_runs"`
	EVMVersion          string `json:"evm_version"`
	License             string `json:"license"`
	ConstructorArgs     string `json:"constructor_args"`
	ContractName        string `json:"contract_name"`
	VerifiedBy          string `json:"verified_by"`
}

// SubmitVerificationResponse holds the response for a verification submission.
type SubmitVerificationResponse struct {
	VerificationID uint   `json:"verification_id"`
	Status         string `json:"status"`
	Message        string `json:"message"`
}

// SubmitVerification creates a verification record and triggers async verification.
func (s *ContractVerificationService) SubmitVerification(ctx context.Context, req *SubmitVerificationRequest) (*SubmitVerificationResponse, error) {
	// Validate request
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.ContractAddress == "" {
		return nil, errors.New("contract_address is required")
	}
	if req.SourceCode == "" {
		return nil, errors.New("source_code is required")
	}

	// Default values
	if req.ChainID == "" {
		req.ChainID = "injective-1"
	}
	if req.ContractType == "" {
		req.ContractType = models.ContractTypeSolidity
	}

	// Validate contract type
	if req.ContractType != models.ContractTypeSolidity && req.ContractType != models.ContractTypeCosmWasm {
		return nil, errors.Errorf("unsupported contract_type: %s (expected: solidity or cosmwasm)", req.ContractType)
	}

	// Only Solidity is supported for now
	if req.ContractType == models.ContractTypeCosmWasm {
		return nil, errors.New("cosmwasm verification is not yet supported")
	}

	// Check if there's already a pending or verifying verification
	var existingVerification models.ContractVerification
	err := db.Get().WithContext(ctx).
		Where("contract_address = ? AND chain_id = ? AND verification_status IN (?)",
			req.ContractAddress, req.ChainID,
			[]string{models.VerificationStatusPending, models.VerificationStatusVerifying}).
		First(&existingVerification).Error
	if err == nil {
		return &SubmitVerificationResponse{
			VerificationID: existingVerification.ID,
			Status:         existingVerification.VerificationStatus,
			Message:        "verification is already in progress",
		}, nil
	}

	// Store source code to object storage
	sourceKey := fmt.Sprintf("contracts/%s/verification/%d/source.json", req.ContractAddress, time.Now().Unix())
	if s.storage != nil {
		if err := s.storage.PutObject(ctx, sourceKey, []byte(req.SourceCode)); err != nil {
			if global.Logger != nil {
				global.Logger.Warnf("Failed to store source code to object storage: %v", err)
			}
			// Continue without object storage - store inline
			sourceKey = ""
		}
	} else {
		sourceKey = ""
	}

	// Create verification record
	verification := &models.ContractVerification{
		ContractAddress:      req.ContractAddress,
		ChainID:              req.ChainID,
		ContractType:         req.ContractType,
		VerificationType:     models.VerificationTypeFullMatch,
		CompilerVersion:      req.CompilerVersion,
		OptimizationEnabled:  req.OptimizationEnabled,
		OptimizationRuns:     req.OptimizationRuns,
		EVMVersion:           req.EVMVersion,
		License:              req.License,
		SourceCode:           req.SourceCode,
		SourceCodeStorageKey: sourceKey,
		ConstructorArgs:      req.ConstructorArgs,
		VerificationStatus:   models.VerificationStatusPending,
		VerifiedBy:           req.VerifiedBy,
	}

	if err := db.Get().WithContext(ctx).Create(verification).Error; err != nil {
		return nil, errors.Wrap(err, "failed to create verification record")
	}

	// Trigger async verification
	go s.executeVerification(context.Background(), verification.ID, req.ContractName)

	return &SubmitVerificationResponse{
		VerificationID: verification.ID,
		Status:         models.VerificationStatusPending,
		Message:        "verification submitted successfully",
	}, nil
}

// executeVerification performs the actual verification process asynchronously.
func (s *ContractVerificationService) executeVerification(ctx context.Context, verificationID uint, contractName string) {
	if global.Logger != nil {
		global.Logger.Infof("Starting contract verification ID=%d", verificationID)
	}

	// Load verification record
	var verification models.ContractVerification
	if err := db.Get().WithContext(ctx).First(&verification, verificationID).Error; err != nil {
		if global.Logger != nil {
			global.Logger.Errorf("Failed to load verification record ID=%d: %v", verificationID, err)
		}
		return
	}

	// Update status to verifying
	s.updateVerificationStatus(ctx, verificationID, models.VerificationStatusVerifying, "")

	// Step 1: Get on-chain bytecode
	onchainBytecode, err := s.getOnchainBytecode(ctx, verification.ContractAddress)
	if err != nil {
		s.updateVerificationStatus(ctx, verificationID, models.VerificationStatusFailed,
			fmt.Sprintf("failed to get on-chain bytecode: %v", err))
		return
	}

	if onchainBytecode == "" {
		s.updateVerificationStatus(ctx, verificationID, models.VerificationStatusFailed,
			"no bytecode found at contract address (may not be a contract)")
		return
	}

	// Step 2: Compile source code
	compileResult, err := s.compiler.Compile(ctx, &CompileRequest{
		SourceCode:          verification.SourceCode,
		CompilerVersion:     verification.CompilerVersion,
		OptimizationEnabled: verification.OptimizationEnabled,
		OptimizationRuns:    verification.OptimizationRuns,
		EVMVersion:          verification.EVMVersion,
		ContractName:        contractName,
	})
	if err != nil {
		s.updateVerificationStatus(ctx, verificationID, models.VerificationStatusFailed,
			fmt.Sprintf("compilation failed: %v", err))
		return
	}

	if len(compileResult.Errors) > 0 {
		s.updateVerificationStatus(ctx, verificationID, models.VerificationStatusFailed,
			fmt.Sprintf("compilation errors: %s", strings.Join(compileResult.Errors, "; ")))
		return
	}

	if compileResult.Bytecode == "" {
		s.updateVerificationStatus(ctx, verificationID, models.VerificationStatusFailed,
			"compilation produced no bytecode")
		return
	}

	// Step 3: Compare bytecodes
	compareResult := CompareBytecode(compileResult.Bytecode, onchainBytecode)

	switch compareResult.MatchType {
	case BytecodeMatchFull:
		verification.VerificationType = models.VerificationTypeFullMatch
	case BytecodeMatchPartial:
		verification.VerificationType = models.VerificationTypePartialMatch
	default:
		s.updateVerificationStatus(ctx, verificationID, models.VerificationStatusFailed,
			fmt.Sprintf("bytecode mismatch: %s", compareResult.Message))
		return
	}

	// Step 4: Save ABI to object storage and update records
	now := time.Now()
	verification.VerificationStatus = models.VerificationStatusVerified
	verification.VerifiedAt = &now
	verification.ABI = compileResult.ABI

	// Store ABI
	if s.storage != nil && compileResult.ABI != "" {
		abiKey := fmt.Sprintf("contracts/%s/abi/%d.json", verification.ContractAddress, now.Unix())
		if err := s.storage.PutObject(ctx, abiKey, []byte(compileResult.ABI)); err != nil {
			if global.Logger != nil {
				global.Logger.Warnf("Failed to store ABI: %v", err)
			}
		} else {
			verification.ABIStorageKey = abiKey
		}
	}

	// Save verification record
	if err := db.Get().WithContext(ctx).Save(&verification).Error; err != nil {
		if global.Logger != nil {
			global.Logger.Errorf("Failed to save verification result: %v", err)
		}
		return
	}

	// Step 5: Update ContractInfo
	s.updateContractInfoAfterVerification(ctx, &verification, compileResult)

	// Step 6: Save source files
	s.saveSourceFiles(ctx, &verification)

	if global.Logger != nil {
		global.Logger.Infof("Contract verification completed: address=%s, type=%s, match=%s",
			verification.ContractAddress, verification.ContractType, verification.VerificationType)
	}
}

// getOnchainBytecode retrieves the runtime bytecode of a contract from the chain.
func (s *ContractVerificationService) getOnchainBytecode(ctx context.Context, address string) (string, error) {
	if s.evmClient == nil {
		return "", errors.New("EVM client is not initialized")
	}

	addr := common.HexToAddress(address)
	code, err := s.evmClient.GetCode(ctx, addr, nil)
	if err != nil {
		return "", errors.Wrap(err, "failed to get code from chain")
	}

	if len(code) == 0 {
		return "", nil
	}

	return hex.EncodeToString(code), nil
}

// updateVerificationStatus updates the verification status and error message.
func (s *ContractVerificationService) updateVerificationStatus(ctx context.Context, id uint, status, errMsg string) {
	updates := map[string]interface{}{
		"verification_status": status,
		"error_message":       errMsg,
	}
	if status == models.VerificationStatusVerified {
		now := time.Now()
		updates["verified_at"] = &now
	}

	if err := db.Get().WithContext(ctx).
		Model(&models.ContractVerification{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		if global.Logger != nil {
			global.Logger.Errorf("Failed to update verification status: id=%d, status=%s, err=%v", id, status, err)
		}
	}
}

// updateContractInfoAfterVerification updates the ContractInfo record after successful verification.
func (s *ContractVerificationService) updateContractInfoAfterVerification(
	ctx context.Context,
	verification *models.ContractVerification,
	compileResult *CompileResult,
) {
	updates := map[string]interface{}{
		"is_verified":         true,
		"verification_status": models.VerificationStatusVerified,
		"verified_at":         verification.VerifiedAt,
		"verified_by":         verification.VerifiedBy,
	}

	result := db.Get().WithContext(ctx).
		Model(&models.ContractInfo{}).
		Where("contract_address = ? AND chain_id = ?", verification.ContractAddress, verification.ChainID).
		Updates(updates)

	if result.Error != nil {
		if global.Logger != nil {
			global.Logger.Errorf("Failed to update ContractInfo: address=%s, err=%v",
				verification.ContractAddress, result.Error)
		}
		return
	}

	// If no record was updated, the ContractInfo may not exist yet - skip
	if result.RowsAffected == 0 {
		if global.Logger != nil {
			global.Logger.Warnf("No ContractInfo found for address=%s, skipping update", verification.ContractAddress)
		}
	}

	// Update the contract verification_status tag
	s.updateVerificationTag(ctx, verification.ContractAddress, verification.ChainID, true)
}

// updateVerificationTag updates the verification status tag on a contract.
func (s *ContractVerificationService) updateVerificationTag(ctx context.Context, address, chainID string, verified bool) {
	tagValue := "未验证"
	tagColor := "#808080" // gray
	if verified {
		tagValue = "已验证"
		tagColor = "#22c55e" // green
	}

	// Try to update existing tag first
	result := db.Get().WithContext(ctx).
		Model(&models.ContractTag{}).
		Where("contract_address = ? AND chain_id = ? AND tag_type = ?",
			address, chainID, "verification_status").
		Updates(map[string]interface{}{
			"tag_value": tagValue,
			"tag_color": tagColor,
		})

	if result.Error != nil {
		if global.Logger != nil {
			global.Logger.Warnf("Failed to update verification tag: %v", result.Error)
		}
		return
	}

	// If no tag exists, create one
	if result.RowsAffected == 0 {
		tag := &models.ContractTag{
			ContractAddress: address,
			ChainID:         chainID,
			TagType:         "verification_status",
			TagValue:        tagValue,
			TagColor:        tagColor,
			Description:     "合约验证状态",
			Visible:         true,
			IsPreset:        true,
			CreatedBy:       "system",
		}
		if err := db.Get().WithContext(ctx).Create(tag).Error; err != nil {
			if global.Logger != nil {
				global.Logger.Warnf("Failed to create verification tag: %v", err)
			}
		}
	}
}

// saveSourceFiles saves source code files to the contract_source_files table.
func (s *ContractVerificationService) saveSourceFiles(ctx context.Context, verification *models.ContractVerification) {
	// Try to parse source code as JSON standard input format
	var standardInput struct {
		Sources map[string]struct {
			Content string `json:"content"`
		} `json:"sources"`
	}

	if err := json.Unmarshal([]byte(verification.SourceCode), &standardInput); err == nil && len(standardInput.Sources) > 0 {
		// Multi-file source code
		for filePath, source := range standardInput.Sources {
			s.createSourceFile(ctx, verification, filePath, source.Content, "source")
		}
	} else {
		// Single file source code
		fileName := "Contract.sol"
		s.createSourceFile(ctx, verification, fileName, verification.SourceCode, "source")
	}

	// Save ABI as a separate file
	if verification.ABI != "" {
		s.createSourceFile(ctx, verification, "abi.json", verification.ABI, "abi")
	}
}

// createSourceFile creates a source file record.
func (s *ContractVerificationService) createSourceFile(
	ctx context.Context,
	verification *models.ContractVerification,
	filePath, content, fileType string,
) {
	// Extract filename from path
	fileName := filePath
	if idx := strings.LastIndex(filePath, "/"); idx >= 0 {
		fileName = filePath[idx+1:]
	}

	file := &models.ContractSourceFile{
		ContractAddress: verification.ContractAddress,
		ChainID:         verification.ChainID,
		FileName:        fileName,
		FileType:        fileType,
		FilePath:        filePath,
		FileContent:     content,
		FileSize:        int64(len(content)),
		UploadedBy:      verification.VerifiedBy,
	}

	if err := db.Get().WithContext(ctx).Create(file).Error; err != nil {
		if global.Logger != nil {
			global.Logger.Warnf("Failed to save source file %s: %v", fileName, err)
		}
	}
}

// GetVerificationStatus retrieves the verification status of a contract.
func (s *ContractVerificationService) GetVerificationStatus(ctx context.Context, address, chainID string) (*models.ContractVerification, error) {
	if address == "" {
		return nil, errors.New("address is required")
	}
	if chainID == "" {
		chainID = "injective-1"
	}

	var verification models.ContractVerification
	err := db.Get().WithContext(ctx).
		Where("contract_address = ? AND chain_id = ?", address, chainID).
		Order("id DESC").
		First(&verification).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // No verification record
		}
		return nil, errors.Wrap(err, "failed to get verification status")
	}

	return &verification, nil
}

// GetContractSourceCode retrieves the source code files for a verified contract.
func (s *ContractVerificationService) GetContractSourceCode(ctx context.Context, address, chainID string) ([]models.ContractSourceFile, error) {
	if address == "" {
		return nil, errors.New("address is required")
	}
	if chainID == "" {
		chainID = "injective-1"
	}

	var files []models.ContractSourceFile
	err := db.Get().WithContext(ctx).
		Where("contract_address = ? AND chain_id = ? AND file_type = ?", address, chainID, "source").
		Find(&files).Error

	if err != nil {
		return nil, errors.Wrap(err, "failed to get source code files")
	}

	return files, nil
}

// GetContractABI retrieves the ABI for a verified contract.
func (s *ContractVerificationService) GetContractABI(ctx context.Context, address, chainID string) (string, error) {
	if address == "" {
		return "", errors.New("address is required")
	}
	if chainID == "" {
		chainID = "injective-1"
	}

	// Try to get from the latest verification record
	var verification models.ContractVerification
	err := db.Get().WithContext(ctx).
		Where("contract_address = ? AND chain_id = ? AND verification_status = ?",
			address, chainID, models.VerificationStatusVerified).
		Order("id DESC").
		First(&verification).Error

	if err == nil && verification.ABI != "" {
		return verification.ABI, nil
	}

	// Try to get from source files
	var file models.ContractSourceFile
	err = db.Get().WithContext(ctx).
		Where("contract_address = ? AND chain_id = ? AND file_type = ?", address, chainID, "abi").
		Order("id DESC").
		First(&file).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", errors.Wrap(err, "failed to get ABI")
	}

	return file.FileContent, nil
}

// MarkContractAsUnverified marks a newly deployed contract as unverified.
// This is called during contract deployment detection in the sync pipeline.
func (s *ContractVerificationService) MarkContractAsUnverified(ctx context.Context, address, chainID, creator string) {
	if address == "" {
		return
	}
	if chainID == "" {
		chainID = "injective-1"
	}

	// Update ContractInfo if it exists
	db.Get().WithContext(ctx).
		Model(&models.ContractInfo{}).
		Where("contract_address = ? AND chain_id = ?", address, chainID).
		Updates(map[string]interface{}{
			"verification_status": models.VerificationStatusPending,
			"is_verified":         false,
		})

	// Add unverified tag
	s.updateVerificationTag(ctx, address, chainID, false)
}
