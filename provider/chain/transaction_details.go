package chain

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

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
	BaseFee         string `json:"base_fee,omitempty"`
	MaxFeePerGas    string `json:"max_fee_per_gas,omitempty"`
	MaxPriorityFee  string `json:"max_priority_fee,omitempty"`

	// Other fields
	Nonce            uint64    `json:"nonce"`
	Input            string    `json:"input"` // Transaction data
	TransactionIndex uint      `json:"transaction_index"`

	// Logs
	Logs []LogInfo `json:"logs"`
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
			// Success
			return details, nil
		}

		lastErr = err

		// Check if error is retryable
		if !retryableRPCError(err) {
			// Non-retryable error, return immediately
			return nil, errors.Wrapf(err, "failed to get transaction details (non-retryable error)")
		}

		// If this is the last attempt, don't continue
		if attempt == maxRetries-1 {
			break
		}

		// Log the retry attempt
		delay := time.Duration(1<<uint(attempt)) * initialDelay
		fmt.Printf("Retrying transaction details fetch for %s in %v (attempt %d/%d): %v\n",
			txHash.Hex(), delay, attempt+1, maxRetries, err)
	}

	// All retries exhausted
	return nil, errors.Wrapf(lastErr, "failed to get transaction details after %d attempts", maxRetries)
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
	signer := types.NewEIP155Signer(c.chainID)
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
		if block.BaseFee() != nil {
			baseFee = block.BaseFee()
			// Actual gas price paid = baseFee + priorityFee
			gasPrice = new(big.Int).Add(baseFee, maxPriorityFee)
		}
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
			Address: log.Address.Hex(),
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
		contractAddress = receipt.ContractAddress.Hex()
	}

	// 11. Build result
	details := &TransactionDetails{
		Hash:             txHash.Hex(),
		Status:           getStatusString(receipt.Status),
		Method:           method,
		BlockNumber:      receipt.BlockNumber.Uint64(),
		BlockHash:        receipt.BlockHash.Hex(),
		Confirmations:    currentBlockNumber - receipt.BlockNumber.Uint64(),
		Timestamp:        time.Unix(int64(block.Time()), 0),
		From:             from.Hex(),
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

// getToAddress gets the To address.
func getToAddress(tx *types.Transaction, receipt *types.Receipt) string {
	// For contract creation, return contract address
	if receipt.ContractAddress != (common.Address{}) {
		return receipt.ContractAddress.Hex()
	}
	// Otherwise return To address
	if tx.To() != nil {
		return tx.To().Hex()
	}
	return ""
}

