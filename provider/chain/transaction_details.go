package chain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"injective-explorer/internal/global"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"
)

// TransactionDetails represents complete transaction details.
type TransactionDetails struct {
	// Basic information
	Hash   string `json:"hash"`
	Status string `json:"status"` // "Success" or "Failed"
	Method string `json:"method"` // Method signature (e.g., "0xb9256d28")

	// Block information
	BlockNumber   uint64    `json:"block_number"`
	BlockHash     string    `json:"block_hash"`
	Confirmations uint64    `json:"confirmations"`
	Timestamp     time.Time `json:"timestamp"`

	// Address information
	From            string `json:"from"`
	To              string `json:"to"`               // Contract address or recipient address
	ContractAddress string `json:"contract_address"` // For contract creation

	// Amount and fees
	Value          string  `json:"value"`           // Transaction amount (wei)
	TransactionFee string  `json:"transaction_fee"` // Transaction fee
	GasPrice       string  `json:"gas_price"`       // Gas price (wei)
	GasLimit       uint64  `json:"gas_limit"`
	GasUsed        uint64  `json:"gas_used"`
	GasUsedPercent float64 `json:"gas_used_percent"`

	// Gas fee details (EIP-1559)
	BaseFee        string `json:"base_fee,omitempty"`
	MaxFeePerGas   string `json:"max_fee_per_gas,omitempty"`
	MaxPriorityFee string `json:"max_priority_fee,omitempty"`

	// Other fields
	Nonce            uint64 `json:"nonce"`
	Input            string `json:"input"` // Transaction data
	TransactionIndex uint   `json:"transaction_index"`

	// Logs
	Logs []LogInfo `json:"logs"`

	// Raw JSON data (fallback when parsing fails)
	// These are the original RPC responses stored as JSON strings
	RawTransactionJSON json.RawMessage `json:"raw_transaction_json,omitempty"` // eth_getTransactionByHash response
	RawReceiptJSON     json.RawMessage `json:"raw_receipt_json,omitempty"`     // eth_getTransactionReceipt response
	ParseStatus        string          `json:"parse_status,omitempty"`         // "success", "partial", "failed"
	ParseError         string          `json:"parse_error,omitempty"`          // Error message if parsing failed
}

// LogInfo represents event log information.
type LogInfo struct {
	Address string   `json:"address"`
	Topics  []string `json:"topics"`
	Data    string   `json:"data"`
	Index   uint     `json:"index"`
	Removed bool     `json:"removed"`
}

// retryableRPCError checks if an error is retryable
func retryableRPCError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Retryable errors
	retryableErrors := []string{
		"context canceled",
		"context deadline exceeded",
		"parse error",
		"invalid json",
		"connection refused",
		"timeout",
		"temporary failure",
		"network error",
	}

	for _, retryableErr := range retryableErrors {
		if strings.Contains(errStr, retryableErr) {
			return true
		}
	}

	return false
}

// GetTransactionDetails gets complete transaction details with retry logic.
// This implementation uses TransactionByHash and TransactionReceipt with
// exponential backoff retry and circuit breaker pattern.
// If parsing fails, it falls back to storing raw JSON responses.
func (c *EVMClient) GetTransactionDetails(ctx context.Context, txHash common.Hash) (*TransactionDetails, error) {
	var details *TransactionDetails
	var lastErr error

	maxRetries := 3
	initialDelay := 1 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			delay := time.Duration(1<<uint(attempt-1)) * initialDelay
			time.Sleep(delay)
		}

		// Use circuit breaker to execute the RPC call
		err := c.circuitBreaker.Execute(func() error {
			var innerErr error
			details, innerErr = c.getTransactionDetailsInternal(ctx, txHash)
			return innerErr
		})

		if err == nil {
			// Success - mark as successfully parsed
			details.ParseStatus = "success"
			return details, nil
		}

		lastErr = err

		// Check if error is retryable
		if !retryableRPCError(err) {
			// Non-retryable error - try to get raw JSON as fallback
			break
		}

		// If this is the last attempt, don't continue
		if attempt == maxRetries-1 {
			break
		}

		// Log the retry attempt
		delay := time.Duration(1<<uint(attempt)) * initialDelay
		global.Logger.Warnf("Retrying transaction details fetch for %s in %v (attempt %d/%d): %v",
			txHash.Hex(), delay, attempt+1, maxRetries, err)
	}

	// All retries exhausted - try fallback to raw JSON
	global.Logger.Warnf("All retries failed for %s, attempting to fetch raw JSON", txHash.Hex())
	rawDetails, rawErr := c.getRawTransactionDetailsFallback(ctx, txHash, lastErr)
	if rawErr != nil {
		return nil, errors.Wrapf(lastErr, "failed to get transaction details after %d attempts and also failed to get raw JSON: %v", maxRetries, rawErr)
	}

	return rawDetails, nil
}

// getTransactionDetailsInternal performs the actual RPC calls to fetch transaction details
func (c *EVMClient) getTransactionDetailsInternal(ctx context.Context, txHash common.Hash) (*TransactionDetails, error) {
	// 1. Get transaction
	tx, isPending, err := c.client.TransactionByHash(ctx, txHash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get transaction")
	}

	if isPending {
		return nil, fmt.Errorf("transaction is still pending")
	}

	// 2. Get transaction receipt
	receipt, err := c.client.TransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get transaction receipt")
	}

	// 3. Get block information (for timestamp and confirmations)
	block, err := c.client.BlockByNumber(ctx, receipt.BlockNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get block")
	}

	// 4. Get current block number (for confirmation calculation)
	currentBlockNumber, err := c.client.BlockNumber(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get current block number")
	}

	// 5. Recover From address
	signer := types.LatestSignerForChainID(tx.ChainId())
	from, err := types.Sender(signer, tx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to recover sender")
	}

	// 6. Extract method signature
	method := ""
	if len(tx.Data()) >= 4 {
		method = "0x" + hex.EncodeToString(tx.Data()[:4])
	}

	// 7. Calculate gas price and fee
	gasPrice := tx.GasPrice()
	var baseFee, maxFeePerGas, maxPriorityFee *big.Int

	// Check if this is an EIP-1559 transaction
	if tx.Type() == types.DynamicFeeTxType {
		maxFeePerGas = tx.GasFeeCap()
		maxPriorityFee = tx.GasTipCap()
		// Base fee must be obtained from block header
		// Use recovery to handle corrupted block objects with nil headers
		SafeExecuteWithTxHash(
			txHash,
			"accessing block.BaseFee()",
			"using tx.GasPrice()",
			func() {
				if block != nil && block.BaseFee() != nil {
					baseFee = block.BaseFee()
					// Actual gas price paid = baseFee + priorityFee
					gasPrice = new(big.Int).Add(baseFee, maxPriorityFee)
				}
			},
		)
	}

	// Calculate transaction fee
	transactionFee := new(big.Int).Mul(
		big.NewInt(int64(receipt.GasUsed)),
		gasPrice,
	)

	// 8. Calculate gas usage percentage
	var gasUsedPercent float64
	if tx.Gas() > 0 {
		gasUsedPercent = float64(receipt.GasUsed) / float64(tx.Gas()) * 100
	}

	// 9. Build log information
	logs := make([]LogInfo, len(receipt.Logs))
	for i, log := range receipt.Logs {
		topics := make([]string, len(log.Topics))
		for j, topic := range log.Topics {
			topics[j] = topic.Hex()
		}
		logs[i] = LogInfo{
			Address: strings.ToLower(log.Address.Hex()),
			Topics:  topics,
			Data:    hex.EncodeToString(log.Data),
			Index:   log.Index,
			Removed: log.Removed,
		}
	}

	// 10. Determine To address
	to := getToAddress(tx, receipt)
	contractAddress := ""
	if receipt.ContractAddress != (common.Address{}) {
		contractAddress = strings.ToLower(receipt.ContractAddress.Hex())
	}

	// 10a. If no direct contract creation address, try to extract from event logs
	// This handles Factory pattern contract creation (e.g., Uniswap V2 PairCreated events)
	// Note: ExtractContractAddressFromLogs already returns lowercase
	if contractAddress == "" {
		contractAddress = ExtractContractAddressFromLogs(receipt)
	}

	// 11. Build result
	// Use recovery to handle corrupted block objects with nil headers
	var timestamp time.Time
	SafeExecuteWithTxHash(
		txHash,
		"accessing block.Time()",
		"using zero time value",
		func() {
			if block != nil {
				timestamp = time.Unix(int64(block.Time()), 0)
			}
		},
	)

	details := &TransactionDetails{
		Hash:             txHash.Hex(),
		Status:           getStatusString(receipt.Status),
		Method:           method,
		BlockNumber:      receipt.BlockNumber.Uint64(),
		BlockHash:        receipt.BlockHash.Hex(),
		Confirmations:    currentBlockNumber - receipt.BlockNumber.Uint64(),
		Timestamp:        timestamp,
		From:             strings.ToLower(from.Hex()),
		To:               to,
		ContractAddress:  contractAddress,
		Value:            tx.Value().String(),
		TransactionFee:   transactionFee.String(),
		GasPrice:         gasPrice.String(),
		GasLimit:         tx.Gas(),
		GasUsed:          receipt.GasUsed,
		GasUsedPercent:   gasUsedPercent,
		Nonce:            tx.Nonce(),
		Input:            hex.EncodeToString(tx.Data()),
		TransactionIndex: receipt.TransactionIndex,
		Logs:             logs,
	}

	// Add EIP-1559 related fields
	if baseFee != nil {
		details.BaseFee = baseFee.String()
	}
	if maxFeePerGas != nil {
		details.MaxFeePerGas = maxFeePerGas.String()
	}
	if maxPriorityFee != nil {
		details.MaxPriorityFee = maxPriorityFee.String()
	}

	return details, nil
}

// getStatusString converts status code to string.
func getStatusString(status uint64) string {
	if status == 1 {
		return "Success"
	}
	return "Failed"
}

// getToAddress gets the To address (EVM format: 0x + lowercase).
func getToAddress(tx *types.Transaction, receipt *types.Receipt) string {
	// For contract creation, return contract address
	if receipt.ContractAddress != (common.Address{}) {
		return strings.ToLower(receipt.ContractAddress.Hex())
	}
	// Otherwise return To address
	if tx.To() != nil {
		return strings.ToLower(tx.To().Hex())
	}
	return ""
}

// getRawTransactionDetailsFallback fetches raw JSON data when normal parsing fails.
// This allows us to store the original data even for unsupported transaction types.
// It uses ethclient methods and serializes the results to JSON.
//
// Parameters:
//   - ctx: Context for cancellation
//   - txHash: Transaction hash
//   - parseErr: The parsing error that occurred
//
// Returns:
//   - *TransactionDetails: Details with raw JSON populated
//   - error: Error if even fetching raw data fails
func (c *EVMClient) getRawTransactionDetailsFallback(ctx context.Context, txHash common.Hash, parseErr error) (*TransactionDetails, error) {
	// Create details object with fallback status
	details := &TransactionDetails{
		Hash:        txHash.Hex(),
		ParseStatus: "failed",
		ParseError:  parseErr.Error(),
	}

	// Try to get transaction data (even if sender recovery fails)
	tx, isPending, err := c.client.TransactionByHash(ctx, txHash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get transaction (even for fallback)")
	}

	if isPending {
		return nil, fmt.Errorf("transaction is still pending")
	}

	// Get receipt (may fail for pending tx)
	receipt, err := c.client.TransactionReceipt(ctx, txHash)
	if err != nil {
		// Continue without receipt
		global.Logger.Warnf("Fallback: Failed to get receipt for %s: %v", txHash.Hex(), err)
	}

	// Serialize transaction to JSON (even if we can't recover sender)
	txJSON, err := tx.MarshalJSON()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal transaction to JSON")
	}

	// Serialize receipt to JSON if available
	var receiptJSON []byte
	if receipt != nil {
		receiptJSON, err = receipt.MarshalJSON()
		if err != nil {
			global.Logger.Errorf("Fallback: Failed to marshal receipt to JSON: %v", err)
			// Continue without receipt
		}
	}

	// Store raw JSON
	details.RawTransactionJSON = txJSON
	if len(receiptJSON) > 0 {
		details.RawReceiptJSON = receiptJSON
	}

	// Try to recover sender address even in fallback mode
	// This is critical for updating signer/signatures in the transactions table
	signer := types.LatestSignerForChainID(tx.ChainId())
	from, err := types.Sender(signer, tx)
	if err != nil {
		// Sender recovery failed - this is the original parse error
		global.Logger.Warnf("Fallback: Failed to recover sender for %s: %v", txHash.Hex(), err)
		// Continue without from address - downstream will handle missing signer
	} else {
		details.From = strings.ToLower(from.Hex())
	}

	// Try to extract basic information from transaction
	if tx.To() != nil {
		details.To = strings.ToLower(tx.To().Hex())
	}
	details.Value = tx.Value().String()
	details.Nonce = tx.Nonce()
	details.Input = "0x" + hex.EncodeToString(tx.Data())

	// Extract gas info
	details.GasLimit = tx.Gas()
	details.GasPrice = tx.GasPrice().String()

	// Try block hash from receipt
	if receipt != nil {
		details.BlockHash = receipt.BlockHash.Hex()
		details.BlockNumber = receipt.BlockNumber.Uint64()
		details.TransactionIndex = uint(receipt.TransactionIndex)

		// Extract status
		if receipt.Status == 1 {
			details.Status = "Success"
		} else {
			details.Status = "Failed"
		}

		details.GasUsed = receipt.GasUsed

		// Extract logs
		logs := make([]LogInfo, len(receipt.Logs))
		for i, log := range receipt.Logs {
			topics := make([]string, len(log.Topics))
			for j, topic := range log.Topics {
				topics[j] = topic.Hex()
			}
			logs[i] = LogInfo{
				Address: strings.ToLower(log.Address.Hex()),
				Topics:  topics,
				Data:    hex.EncodeToString(log.Data),
				Index:   uint(log.Index),
				Removed: log.Removed,
			}
		}
		details.Logs = logs

		// Extract contract address from receipt or logs (Fallback enhancement - 方案B)
		// This ensures contract address is captured even when main parsing fails
		if receipt.ContractAddress != (common.Address{}) {
			details.ContractAddress = strings.ToLower(receipt.ContractAddress.Hex())
		} else {
			// Try to extract from event logs (Factory pattern)
			// Note: ExtractContractAddressFromLogs already returns lowercase
			details.ContractAddress = ExtractContractAddressFromLogs(receipt)
		}
	}

	// Get block info for timestamp
	if receipt != nil {
		block, err := c.client.BlockByNumber(ctx, receipt.BlockNumber)
		if err == nil {
			details.Timestamp = time.Unix(int64(block.Time()), 0)
		}
	}

	// Get current block for confirmations
	if receipt != nil {
		currentBlock, err := c.client.BlockNumber(ctx)
		if err == nil {
			details.Confirmations = currentBlock - receipt.BlockNumber.Uint64()
		}
	}

	// Mark as partial since we have some data but not from address
	details.ParseStatus = "partial"

	global.Logger.Infof("Fallback: Stored transaction data for %s (parse_status: partial)", txHash.Hex())

	return details, nil
}
