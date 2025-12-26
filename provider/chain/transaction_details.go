package chain

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"
)

// TransactionDetails 表示完整的交易详情信息
type TransactionDetails struct {
	// 基本信息
	Hash   string `json:"hash"`
	Status string `json:"status"` // "Success" or "Failed"
	Method string `json:"method"` // 方法签名（如 "0xb9256d28"）

	// 区块信息
	BlockNumber   uint64    `json:"block_number"`
	BlockHash     string    `json:"block_hash"`
	Confirmations uint64    `json:"confirmations"`
	Timestamp     time.Time `json:"timestamp"`

	// 地址信息
	From            string `json:"from"`
	To              string `json:"to"`               // 合约地址或接收地址
	ContractAddress string `json:"contract_address"` // 如果是合约创建

	// 金额和费用
	Value          string  `json:"value"`           // 交易金额（wei）
	TransactionFee string  `json:"transaction_fee"` // 交易手续费
	GasPrice       string  `json:"gas_price"`       // Gas 价格（wei）
	GasLimit       uint64  `json:"gas_limit"`
	GasUsed        uint64  `json:"gas_used"`
	GasUsedPercent float64 `json:"gas_used_percent"`

	// Gas 费用详情（EIP-1559）
	BaseFee       string `json:"base_fee,omitempty"`
	MaxFeePerGas  string `json:"max_fee_per_gas,omitempty"`
	MaxPriorityFee string `json:"max_priority_fee,omitempty"`

	// 其他
	Nonce           uint64    `json:"nonce"`
	Input           string    `json:"input"` // 交易数据
	TransactionIndex uint     `json:"transaction_index"`

	// 日志
	Logs []LogInfo `json:"logs"`
}

// LogInfo 表示事件日志信息
type LogInfo struct {
	Address string   `json:"address"`
	Topics  []string `json:"topics"`
	Data    string   `json:"data"`
	Index   uint     `json:"index"`
	Removed bool     `json:"removed"`
}

// GetTransactionDetails 获取完整的交易详情
// 这是使用 TransactionByHash 和 TransactionReceipt 实现 Blockscout 页面展示的示例
func (c *EVMClient) GetTransactionDetails(ctx context.Context, txHash common.Hash) (*TransactionDetails, error) {
	// 1. 获取交易
	tx, isPending, err := c.client.TransactionByHash(ctx, txHash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get transaction")
	}

	if isPending {
		return nil, fmt.Errorf("transaction is still pending")
	}

	// 2. 获取交易回执
	receipt, err := c.client.TransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get transaction receipt")
	}

	// 3. 获取区块信息（用于时间戳和确认数）
	block, err := c.client.BlockByNumber(ctx, receipt.BlockNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get block")
	}

	// 4. 获取当前区块号（用于计算确认数）
	currentBlockNumber, err := c.client.BlockNumber(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get current block number")
	}

	// 5. 恢复 From 地址
	signer := types.NewEIP155Signer(c.chainID)
	from, err := types.Sender(signer, tx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to recover sender")
	}

	// 6. 提取方法签名
	method := ""
	if len(tx.Data()) >= 4 {
		method = "0x" + hex.EncodeToString(tx.Data()[:4])
	}

	// 7. 计算 Gas 价格和手续费
	gasPrice := tx.GasPrice()
	var baseFee, maxFeePerGas, maxPriorityFee *big.Int

	// 检查是否是 EIP-1559 交易
	if tx.Type() == types.DynamicFeeTxType {
		maxFeePerGas = tx.GasFeeCap()
		maxPriorityFee = tx.GasTipCap()
		// Base fee 需要从区块头获取
		if block.BaseFee() != nil {
			baseFee = block.BaseFee()
			// 实际支付的 gas price = baseFee + priorityFee
			gasPrice = new(big.Int).Add(baseFee, maxPriorityFee)
		}
	}

	// 计算交易手续费
	transactionFee := new(big.Int).Mul(
		big.NewInt(int64(receipt.GasUsed)),
		gasPrice,
	)

	// 8. 计算 Gas 使用率
	var gasUsedPercent float64
	if tx.Gas() > 0 {
		gasUsedPercent = float64(receipt.GasUsed) / float64(tx.Gas()) * 100
	}

	// 9. 构建日志信息
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

	// 10. 确定 To 地址
	to := getToAddress(tx, receipt)
	contractAddress := ""
	if receipt.ContractAddress != (common.Address{}) {
		contractAddress = receipt.ContractAddress.Hex()
	}

	// 11. 构建结果
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

	// 添加 EIP-1559 相关字段
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

// getStatusString 将状态码转换为字符串
func getStatusString(status uint64) string {
	if status == 1 {
		return "Success"
	}
	return "Failed"
}

// getToAddress 获取 To 地址
func getToAddress(tx *types.Transaction, receipt *types.Receipt) string {
	// 如果是合约创建，返回合约地址
	if receipt.ContractAddress != (common.Address{}) {
		return receipt.ContractAddress.Hex()
	}
	// 否则返回 To 地址
	if tx.To() != nil {
		return tx.To().Hex()
	}
	return ""
}

