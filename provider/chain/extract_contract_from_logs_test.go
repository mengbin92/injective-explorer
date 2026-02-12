package chain

import (
	"bytes"
	"testing"

	"injective-explorer/internal/conf"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestExtractContractAddressFromLogs(t *testing.T) {
	// Create a mock PairCreated event log
	// Event: PairCreated(address indexed token0, address indexed token1, address pair, uint)
	// topic0 = keccak256("PairCreated(address,address,address,uint256)")

	token0 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	token1 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	pair := common.HexToAddress("0x9772154BF1823353C829B43F9AD96CF9f720c768") // Expected pair address

	// Build data field: pair address (32 bytes) + uint (32 bytes)
	data := make([]byte, 64)
	copy(data[12:32], pair.Bytes()) // Address is right-padded in 32 bytes
	// Leave the uint field as 0

	log := &types.Log{
		Address: common.HexToAddress("0x5C69bEe701ef814a2B6a3EDD4B1652CB9cc5aA6f"), // Factory address
		Topics: []common.Hash{
			PairCreatedEventSignature,
			common.BytesToHash(token0.Bytes()),
			common.BytesToHash(token1.Bytes()),
		},
		Data: data,
	}

	receipt := &types.Receipt{
		Logs: []*types.Log{log},
	}

	// Test extraction
	extracted := ExtractContractAddressFromLogs(receipt)
	expected := pair.Hex()

	if extracted == "" {
		t.Fatal("Expected to extract contract address from PairCreated event, got empty string")
	}

	if extracted != expected {
		t.Errorf("Extracted address mismatch: got %s, want %s", extracted, expected)
	}
}

func TestExtractContractAddressFromLogs_EmptyReceipt(t *testing.T) {
	// Test with nil receipt
	result := ExtractContractAddressFromLogs(nil)
	if result != "" {
		t.Errorf("Expected empty string for nil receipt, got %s", result)
	}

	// Test with empty logs
	receipt := &types.Receipt{Logs: []*types.Log{}}
	result = ExtractContractAddressFromLogs(receipt)
	if result != "" {
		t.Errorf("Expected empty string for empty logs, got %s", result)
	}
}

func TestExtractContractAddressFromLogs_UnknownEvent(t *testing.T) {
	// Log with unknown event signature
	unknownTopic := common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")

	log := &types.Log{
		Address: common.HexToAddress("0x5C69bEe701ef814a2B6a3EDD4B1652CB9cc5aA6f"),
		Topics:  []common.Hash{unknownTopic},
		Data:    []byte{},
	}

	receipt := &types.Receipt{
		Logs: []*types.Log{log},
	}

	result := ExtractContractAddressFromLogs(receipt)
	if result != "" {
		t.Errorf("Expected empty string for unknown event, got %s", result)
	}
}

func TestHasKnownContractCreationEvent(t *testing.T) {
	// Test with known event
	log := &types.Log{
		Topics: []common.Hash{PairCreatedEventSignature},
	}
	receipt := &types.Receipt{Logs: []*types.Log{log}}

	if !HasKnownContractCreationEvent(receipt) {
		t.Error("Expected HasKnownContractCreationEvent to return true for PairCreated event")
	}

	// Test with unknown event
	unknownTopic := common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
	log2 := &types.Log{
		Topics: []common.Hash{unknownTopic},
	}
	receipt2 := &types.Receipt{Logs: []*types.Log{log2}}

	if HasKnownContractCreationEvent(receipt2) {
		t.Error("Expected HasKnownContractCreationEvent to return false for unknown event")
	}
}

func TestIsContractCreationTransaction(t *testing.T) {
	// Test with direct contract creation (ContractAddress set)
	receipt1 := &types.Receipt{
		ContractAddress: common.HexToAddress("0x1234567890123456789012345678901234567890"),
	}
	if !IsContractCreationTransaction(receipt1) {
		t.Error("Expected IsContractCreationTransaction to return true for direct contract creation")
	}

	// Test with Factory pattern (PairCreated event)
	log := &types.Log{
		Topics: []common.Hash{PairCreatedEventSignature},
		Data:   make([]byte, 64),
	}
	receipt2 := &types.Receipt{
		Logs:            []*types.Log{log},
		ContractAddress: common.Address{}, // Empty direct address
	}
	if !IsContractCreationTransaction(receipt2) {
		t.Error("Expected IsContractCreationTransaction to return true for Factory pattern")
	}

	// Test with non-contract transaction
	receipt3 := &types.Receipt{
		Logs:            []*types.Log{},
		ContractAddress: common.Address{},
	}
	if IsContractCreationTransaction(receipt3) {
		t.Error("Expected IsContractCreationTransaction to return false for non-contract transaction")
	}
}

func TestGetContractCreationAddress(t *testing.T) {
	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	pairAddr := common.HexToAddress("0x9772154BF1823353C829B43F9AD96CF9f720c768")

	// Test with direct contract creation
	receipt1 := &types.Receipt{
		ContractAddress: contractAddr,
	}
	result1 := GetContractCreationAddress(receipt1)
	if result1 != contractAddr.Hex() {
		t.Errorf("Expected %s, got %s", contractAddr.Hex(), result1)
	}

	// Test with Factory pattern (should return pair address)
	data := make([]byte, 64)
	copy(data[12:32], pairAddr.Bytes())
	log := &types.Log{
		Topics: []common.Hash{PairCreatedEventSignature, common.Hash{}, common.Hash{}},
		Data:   data,
	}
	receipt2 := &types.Receipt{
		Logs:            []*types.Log{log},
		ContractAddress: common.Address{},
	}
	result2 := GetContractCreationAddress(receipt2)
	if result2 != pairAddr.Hex() {
		t.Errorf("Expected %s from Factory pattern, got %s", pairAddr.Hex(), result2)
	}

	// Test with non-contract transaction
	receipt3 := &types.Receipt{
		Logs:            []*types.Log{},
		ContractAddress: common.Address{},
	}
	result3 := GetContractCreationAddress(receipt3)
	if result3 != "" {
		t.Errorf("Expected empty string for non-contract tx, got %s", result3)
	}
}

func TestParsePairCreatedEvent(t *testing.T) {
	token0 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	token1 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	pair := common.HexToAddress("0x9772154BF1823353C829B43F9AD96CF9f720c768")

	data := make([]byte, 64)
	copy(data[12:32], pair.Bytes())

	log := &types.Log{
		Topics: []common.Hash{
			PairCreatedEventSignature,
			common.BytesToHash(token0.Bytes()),
			common.BytesToHash(token1.Bytes()),
		},
		Data: data,
	}

	result := ParsePairCreatedEvent(log)
	if result == nil {
		t.Fatal("Expected ParsePairCreatedEvent to return non-nil result")
	}

	if result.Token0 != token0 {
		t.Errorf("Token0 mismatch: got %s, want %s", result.Token0.Hex(), token0.Hex())
	}
	if result.Token1 != token1 {
		t.Errorf("Token1 mismatch: got %s, want %s", result.Token1.Hex(), token1.Hex())
	}
	if result.Pair != pair {
		t.Errorf("Pair mismatch: got %s, want %s", result.Pair.Hex(), pair.Hex())
	}

	// Test with invalid log
	invalidLog := &types.Log{
		Topics: []common.Hash{common.HexToHash("0x1234")}, // Wrong topic
	}
	result2 := ParsePairCreatedEvent(invalidLog)
	if result2 != nil {
		t.Error("Expected ParsePairCreatedEvent to return nil for invalid log")
	}
}

func TestConfigurableContractCreationEvents(t *testing.T) {
	// Get singleton instance
	events := GetConfigurableEvents()

	// Test default events exist
	if !events.HasEvent(PairCreatedEventSignature) {
		t.Error("Expected PairCreated event to be registered by default")
	}

	// Test RegisterEvent
	testSignature := "TestEvent(address,uint256)"
	events.RegisterEvent(testSignature, "address", 0, false)

	// Note: We can't easily get the topic0 without calculating keccak256, but we can verify it doesn't panic

	// Test GetAllEvents
	allEvents := events.GetAllEvents()
	if len(allEvents) == 0 {
		t.Error("Expected non-empty events list")
	}
}

func TestLoadFromConfig(t *testing.T) {
	// Create fresh instance for testing
	events := &ConfigurableContractCreationEvents{
		events: make(map[common.Hash]ContractCreationEventInfo),
	}

	// Test loading from config
	configEvents := []*conf.Chain_EVM_ContractCreationEvent{
		{
			Name:                 "PairCreated",
			Signature:            "PairCreated(address,address,address,uint256)",
			ContractParamIndex:   2,
			ContractParamIndexed: false,
			ContractParamType:    "address",
		},
	}

	events.LoadFromConfig(configEvents)

	// Verify events were loaded
	if !events.HasEvent(PairCreatedEventSignature) {
		t.Error("Expected PairCreated event to be loaded from config")
	}

	// Verify we can get event info
	eventInfo, ok := events.GetEvent(PairCreatedEventSignature)
	if !ok {
		t.Fatal("Expected to get PairCreated event info")
	}
	if eventInfo.Name != "PairCreated" {
		t.Errorf("Expected event name 'PairCreated', got '%s'", eventInfo.Name)
	}
	if eventInfo.ParamPosition != 2 {
		t.Errorf("Expected param position 2, got %d", eventInfo.ParamPosition)
	}
	if eventInfo.ParamIndexed {
		t.Error("Expected ParamIndexed to be false")
	}

	// Test empty config
	events2 := &ConfigurableContractCreationEvents{
		events: make(map[common.Hash]ContractCreationEventInfo),
	}
	events2.LoadFromConfig([]*conf.Chain_EVM_ContractCreationEvent{})
	// Should not panic with empty config
}

func TestExtractAddressFromEventWithConfig(t *testing.T) {
	// Test indexed parameter
	indexedEventInfo := ContractCreationEventInfo{
		Name:               "IndexedEvent",
		ParamPosition:      0, // First indexed param
		ParamIndexed:       true,
		TotalIndexedParams: 1,
	}

	contractAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	log := &types.Log{
		Topics: []common.Hash{
			common.HexToHash("0xabcdef"), // topic0 - event signature
			common.BytesToHash(contractAddr.Bytes()),
		},
	}

	result := extractAddressFromEvent(log, indexedEventInfo)
	if result != contractAddr {
		t.Errorf("Indexed param extraction failed: got %s, want %s", result.Hex(), contractAddr.Hex())
	}

	// Test non-indexed parameter
	nonIndexedEventInfo := ContractCreationEventInfo{
		Name:          "NonIndexedEvent",
		ParamPosition: 0,
		ParamIndexed:  false,
	}

	data := make([]byte, 32)
	copy(data[12:], contractAddr.Bytes())
	log2 := &types.Log{
		Topics: []common.Hash{common.HexToHash("0xabcdef")},
		Data:   data,
	}

	result2 := extractAddressFromEvent(log2, nonIndexedEventInfo)
	if result2 != contractAddr {
		t.Errorf("Non-indexed param extraction failed: got %s, want %s", result2.Hex(), contractAddr.Hex())
	}
}

func TestCountIndexedParams(t *testing.T) {
	tests := []struct {
		signature string
		expected  int
	}{
		{"PairCreated(address indexed token0, address indexed token1, address pair, uint)", 2},
		{"Transfer(address indexed from, address indexed to, uint256 value)", 2},
		{"Approval(address indexed owner, address indexed spender, uint256 value)", 2},
		{"EventWithoutIndexed(address from, uint value)", 0},
		{"InvalidSignature", 0},
	}

	for _, test := range tests {
		result := countIndexedParams(test.signature)
		if result != test.expected {
			t.Errorf("countIndexedParams(%q) = %d, want %d", test.signature, result, test.expected)
		}
	}
}

func TestRegisterContractCreationEvent(t *testing.T) {
	// Test deprecated function (backward compatibility)
	RegisterContractCreationEvent("TestLegacyEvent(address,uint256)", "address", 0)

	// Verify it was registered
	events := GetConfigurableEvents()
	// Should not panic
	_ = events.GetAllEvents()
}

func TestBytesToAddress(t *testing.T) {
	addr := common.HexToAddress("0x9772154BF1823353C829B43F9AD96CF9f720c768")

	// Test with 20 bytes
	result1 := bytesToAddress(addr.Bytes())
	if result1 != addr {
		t.Errorf("Expected %s from 20 bytes, got %s", addr.Hex(), result1.Hex())
	}

	// Test with 32 bytes (padded)
	paddedData := make([]byte, 32)
	copy(paddedData[12:], addr.Bytes())
	result2 := bytesToAddress(paddedData)
	if result2 != addr {
		t.Errorf("Expected %s from 32 bytes, got %s", addr.Hex(), result2.Hex())
	}

	// Test with empty data
	result3 := bytesToAddress([]byte{})
	if result3 != (common.Address{}) {
		t.Error("Expected empty address from empty data")
	}
}

func TestTrimLeadingZeros(t *testing.T) {
	data := []byte{0, 0, 0, 1, 2, 3}
	result := TrimLeadingZeros(data)
	expected := []byte{1, 2, 3}
	if !bytes.Equal(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}

	// Test with no leading zeros
	data2 := []byte{1, 2, 3}
	result2 := TrimLeadingZeros(data2)
	if !bytes.Equal(result2, data2) {
		t.Errorf("Expected %v, got %v", data2, result2)
	}
}

func TestInitContractCreationEventsFromConfig(t *testing.T) {
	// Test with nil config (should not panic)
	InitContractCreationEventsFromConfig(nil)

	// Test with empty config
	evmConfig := &conf.Chain_EVM{
		ContractCreationEvents: []*conf.Chain_EVM_ContractCreationEvent{},
	}
	InitContractCreationEventsFromConfig(evmConfig)

	// Test with valid config
	evmConfig2 := &conf.Chain_EVM{
		ContractCreationEvents: []*conf.Chain_EVM_ContractCreationEvent{
			{
				Name:                 "TestEvent",
				Signature:            "TestEvent(address,address)",
				ContractParamIndex:   0,
				ContractParamIndexed: true,
				ContractParamType:    "address",
			},
		},
	}
	InitContractCreationEventsFromConfig(evmConfig2)
}
