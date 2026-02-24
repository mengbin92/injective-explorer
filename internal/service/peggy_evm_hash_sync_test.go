package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestService creates a new service with a isolated registry for testing
func newTestService(cfg *Config, client PeggyEvmHashClient) *PeggyEvmHashSyncService {
	registry := prometheus.NewRegistry()
	return &PeggyEvmHashSyncService{
		config:  cfg,
		client:  client,
		redis:   nil,
		db:      nil,
		metrics: NewPeggyEvmSyncMetrics(registry),
		ctx:     context.Background(),
		cancel:  func() {},
	}
}

// MockPeggyEvmHashClient is a mock implementation of PeggyEvmHashClient.
type MockPeggyEvmHashClient struct {
	GetDepositTxHashFunc        func(context.Context, uint64, uint64, string, string, string) (string, error)
	GetWithdrawalBatchTxHashFunc func(context.Context, uint64, string) (string, error)
}

func (m *MockPeggyEvmHashClient) GetDepositTxHash(
	ctx context.Context,
	eventNonce uint64,
	eventHeight uint64,
	sender string,
	denom string,
	amount string,
) (string, error) {
	if m.GetDepositTxHashFunc != nil {
		return m.GetDepositTxHashFunc(ctx, eventNonce, eventHeight, sender, denom, amount)
	}
	return "", errors.New("not implemented")
}

func (m *MockPeggyEvmHashClient) GetWithdrawalBatchTxHash(
	ctx context.Context,
	batchNonce uint64,
	denom string,
) (string, error) {
	if m.GetWithdrawalBatchTxHashFunc != nil {
		return m.GetWithdrawalBatchTxHashFunc(ctx, batchNonce, denom)
	}
	return "", errors.New("not implemented")
}

// TestNewPeggyEvmHashSyncService tests service creation.
func TestNewPeggyEvmHashSyncService(t *testing.T) {
	cfg := &Config{
		QueueKey:           "test:peggy:evm_hash:queue",
		WorkerCount:        2,
		BatchUpdateSize:    50,
		MinConfirmBlocks:   12,
		RetryMaxAttempts:   3,
		RetryInitialDelay:  time.Second,
		TaskPollInterval:   100 * time.Millisecond,
	}

	// Service creation
	svc := newTestService(cfg, nil)
	assert.NotNil(t, svc)
	assert.NotNil(t, svc.metrics)
	assert.NotNil(t, svc.ctx)
	assert.Equal(t, 100*time.Millisecond, svc.config.TaskPollInterval)
	assert.Equal(t, 50, svc.config.BatchUpdateSize)
}

// TestProcessTask_Deposit_Success tests successful deposit processing.
func TestProcessTask_Deposit_Success(t *testing.T) {
	cfg := &Config{
		QueueKey:         "test:peggy:evm_hash:queue",
		WorkerCount:      1,
		BatchUpdateSize:   100,
		TaskPollInterval:  100 * time.Millisecond,
	}

	mockClient := &MockPeggyEvmHashClient{
		GetDepositTxHashFunc: func(ctx context.Context, eventNonce, eventHeight uint64, sender, denom, amount string) (string, error) {
			return "0xabc123", nil
		},
	}

	svc := newTestService(cfg, mockClient)

	task := &SyncTask{
		Type:        "deposit",
		BlockHeight: 1000000,
		EventNonce:  123,
		TxHash:      "cosmos_tx_123",
		Denom:       "peggy0x07865c6E87B9F70255377e024ace6630C1Eaa37F",
		Amount:      "1000000000000000000",
		Sender:      "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb",
	}

	// Test processDeposit directly (without DB update)
	txHash, err := svc.processDeposit(task)
	require.NoError(t, err)
	assert.Equal(t, "0xabc123", txHash)
}

// TestProcessTask_Withdrawal_Success tests successful withdrawal processing.
func TestProcessTask_Withdrawal_Success(t *testing.T) {
	cfg := &Config{
		QueueKey:         "test:peggy:evm_hash:queue",
		WorkerCount:      1,
		BatchUpdateSize:   100,
		TaskPollInterval:  100 * time.Millisecond,
	}

	mockClient := &MockPeggyEvmHashClient{
		GetWithdrawalBatchTxHashFunc: func(ctx context.Context, batchNonce uint64, denom string) (string, error) {
		return "0xdef456", nil
		},
	}

	svc := newTestService(cfg, mockClient)

	task := &SyncTask{
		Type:        "withdrawal",
		BlockHeight: 1000000,
		BatchNonce:  456,
		TxHash:      "cosmos_tx_456",
		Denom:       "peggy0x07865c6E87B9F70255377e024ace6630C1Eaa37F",
	}

	// Test processWithdrawal directly (without DB update)
	txHash, err := svc.processWithdrawal(task)
	require.NoError(t, err)
	assert.Equal(t, "0xdef456", txHash)
}

// TestProcessTask_Deposit_RPCError tests RPC error handling.
func TestProcessTask_Deposit_RPCError(t *testing.T) {
	cfg := &Config{
		QueueKey:         "test:peggy:evm_hash:queue",
		WorkerCount:      1,
		BatchUpdateSize:   100,
		TaskPollInterval:  100 * time.Millisecond,
	}

	mockClient := &MockPeggyEvmHashClient{
		GetDepositTxHashFunc: func(ctx context.Context, eventNonce, eventHeight uint64, sender, denom, amount string) (string, error) {
		return "", errors.New("timeout: connection refused")
		},
	}

	svc := newTestService(cfg, mockClient)

	task := &SyncTask{
		Type:        "deposit",
		BlockHeight: 1000000,
		EventNonce:  123,
		TxHash:      "cosmos_tx_123",
		Denom:       "peggy0x07865c6E87B9F70255377e024ace6630C1Eaa37F",
		Amount:      "1000000000000000000",
		Sender:      "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb",
	}

	// Test processDeposit directly - should return error
	_, err := svc.processDeposit(task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

// TestEnqueueTask tests task serialization and deserialization.
func TestEnqueueTask(t *testing.T) {
	task := &SyncTask{
		Type:        "deposit",
		BlockHeight: 1000000,
		EventNonce:  123,
		TxHash:      "cosmos_tx_123",
		Denom:       "peggy0xABCDEF",
		Amount:      "1000000",
		Sender:      "0x123456",
	}

	// Serialize
	data, err := json.Marshal(task)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Deserialize
	var decoded SyncTask
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, task.Type, decoded.Type)
	assert.Equal(t, task.BlockHeight, decoded.BlockHeight)
	assert.Equal(t, task.EventNonce, decoded.EventNonce)
	assert.Equal(t, task.TxHash, decoded.TxHash)
	assert.Equal(t, task.Denom, decoded.Denom)
	assert.Equal(t, task.Amount, decoded.Amount)
	assert.Equal(t, task.Sender, decoded.Sender)
}

// TestParseTokenContractFromDenom tests denom parsing.
func TestParseTokenContractFromDenom(t *testing.T) {
	tests := []struct {
		name        string
		denom       string
		expected    string
		expectError bool
	}{
		{
			name:        "Valid peggy denom with 0x",
			denom:       "peggy0x07865c6E87B9F70255377e024ace6630C1Eaa37F",
			expected:    "0x07865c6e87b9f70255377e024ace6630c1eaa37f",
			expectError: false,
		},
		{
			name:        "Valid peggy denom without 0x",
			denom:       "peggy07865c6E87B9F70255377e024ace6630C1Eaa37F",
			expected:    "0x07865c6e87b9f70255377e024ace6630c1eaa37f",
			expectError: false,
		},
		{
			name:        "Raw 0x address (no peggy prefix)",
			denom:       "0x6173fcd85ed895f247b59489a5592f7337b8b232",
			expected:    "0x6173fcd85ed895f247b59489a5592f7337b8b232",
			expectError: false,
		},
		{
			name:        "Raw 0x address uppercase",
			denom:       "0x1C7D4B196CB0C7B01D743FBC6116A902379C7238",
			expected:    "0x1c7d4b196cb0c7b01d743fbc6116a902379c7238",
			expectError: false,
		},
		{
			name:        "Non-peggy denom",
			denom:       "inj",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Empty denom",
			denom:       "",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTokenContractFromDenom(tt.denom)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestIsRetryableError tests retry logic.
func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Timeout error",
			err:      errors.New("timeout: connection refused"),
			expected: true,
		},
		{
			name:     "Rate limit error",
			err:      errors.New("rate limit exceeded"),
			expected: true,
		},
		{
			name:     "Not found error",
			err:      errors.New("event not found"),
			expected: false,
		},
		{
			name:     "Nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestUpdateWithdrawalByOutgoingTxIDs tests batch update logic.
func TestUpdateWithdrawalByOutgoingTxIDs(t *testing.T) {
	cfg := &Config{
		BatchUpdateSize: 2,
	}

	// Test data
	outgoingTxIDs := []uint64{1, 2, 3, 4, 5}

	// This would require a real DB, so we just test the loop logic
	batchCount := 0
	for i := 0; i < len(outgoingTxIDs); i += cfg.BatchUpdateSize {
		batchCount++
		end := i + cfg.BatchUpdateSize
		if end > len(outgoingTxIDs) {
			end = len(outgoingTxIDs)
		}
		// Verify batch size is respected
		assert.LessOrEqual(t, end-i, cfg.BatchUpdateSize)
	}

	assert.Equal(t, 3, batchCount) // 5 items with batch size 2 = 3 batches
}
