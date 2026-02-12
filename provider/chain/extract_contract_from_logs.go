package chain

import (
	"bytes"
	"strings"
	"sync"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// PairCreatedEventSignature is the keccak256 hash of "PairCreated(address,address,address,uint256)"
// This is the standard Uniswap V2 Factory event for pair creation
var PairCreatedEventSignature = crypto.Keccak256Hash([]byte("PairCreated(address,address,address,uint256)"))

// ContractCreationEventInfo holds information about a contract creation event
type ContractCreationEventInfo struct {
	Name                string
	Signature           string
	ContractParamType   string
	ParamPosition       int  // Position in data field (for non-indexed params)
	ParamIndexed        bool // Whether the param is in topics (indexed) or data
	TotalIndexedParams  int  // Total number of indexed params in the event
}

// ConfigurableContractCreationEvents is a thread-safe store for contract creation events
// It can be populated from configuration at runtime
type ConfigurableContractCreationEvents struct {
	mu     sync.RWMutex
	events map[common.Hash]ContractCreationEventInfo
}

// Global configurable events store
var (
	configurableEvents     *ConfigurableContractCreationEvents
	configurableEventsOnce sync.Once
)

// GetConfigurableEvents returns the global configurable events store (singleton)
func GetConfigurableEvents() *ConfigurableContractCreationEvents {
	configurableEventsOnce.Do(func() {
		configurableEvents = &ConfigurableContractCreationEvents{
			events: make(map[common.Hash]ContractCreationEventInfo),
		}
		// Initialize with default events
		configurableEvents.initDefaultEvents()
	})
	return configurableEvents
}

// initDefaultEvents initializes the store with default contract creation events
// These are used as fallback when no configuration is provided
func (c *ConfigurableContractCreationEvents) initDefaultEvents() {
	// Uniswap V2 PairCreated event
	c.events[PairCreatedEventSignature] = ContractCreationEventInfo{
		Name:               "PairCreated",
		Signature:          "PairCreated(address,address,address,uint256)",
		ContractParamType:  "address",
		ParamPosition:      0, // pair address is the first 32-byte word in data (offset 0)
		ParamIndexed:       false,
		TotalIndexedParams: 2, // token0, token1 are indexed
	}
}

// LoadFromConfig loads contract creation events from configuration
// This allows runtime configuration of supported factory patterns
//
// Parameters:
//   - configEvents: List of contract creation event configurations from config file
func (c *ConfigurableContractCreationEvents) LoadFromConfig(configEvents []*conf.Chain_EVM_ContractCreationEvent) {
	if len(configEvents) == 0 {
		if global.Logger != nil {
			global.Logger.Debug("No contract creation events configured, using defaults")
		}
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Clear existing events (except we keep defaults if no valid config provided)
	newEvents := make(map[common.Hash]ContractCreationEventInfo)

	for _, ce := range configEvents {
		if ce.Name == "" || ce.Signature == "" {
			if global.Logger != nil {
				global.Logger.Warnf("Skipping invalid contract creation event config: name=%s, signature=%s", ce.Name, ce.Signature)
			}
			continue
		}

		// Calculate topic0 hash from signature
		topic0 := crypto.Keccak256Hash([]byte(ce.Signature))

		// Count total indexed parameters from signature
		totalIndexed := countIndexedParams(ce.Signature)

		eventInfo := ContractCreationEventInfo{
			Name:               ce.Name,
			Signature:          ce.Signature,
			ContractParamType:  ce.ContractParamType,
			ParamPosition:      int(ce.ContractParamIndex),
			ParamIndexed:       ce.ContractParamIndexed,
			TotalIndexedParams: totalIndexed,
		}

		newEvents[topic0] = eventInfo
		if global.Logger != nil {
			global.Logger.Infof("Registered contract creation event: name=%s, signature=%s, topic0=%s, paramIndex=%d, indexed=%v",
				ce.Name, ce.Signature, topic0.Hex(), ce.ContractParamIndex, ce.ContractParamIndexed)
		}
	}

	// Only replace if we have valid configured events
	if len(newEvents) > 0 {
		c.events = newEvents
		if global.Logger != nil {
			global.Logger.Infof("Loaded %d contract creation events from configuration", len(newEvents))
		}
	} else {
		if global.Logger != nil {
			global.Logger.Warn("No valid contract creation events in configuration, using defaults")
		}
	}
}

// RegisterEvent registers a new contract creation event dynamically
// This allows runtime registration without config file changes
//
// Parameters:
//   - signature: The event signature string (e.g., "PairCreated(address,address,address,uint256)")
//   - paramType: The type of the contract address parameter ("address")
//   - paramIndex: Index of the contract address parameter (0-indexed)
//   - paramIndexed: Whether the contract address parameter is indexed (in topics)
func (c *ConfigurableContractCreationEvents) RegisterEvent(signature string, paramType string, paramIndex int, paramIndexed bool) {
	if signature == "" {
		return
	}

	topic0 := crypto.Keccak256Hash([]byte(signature))
	name := extractEventName(signature)
	totalIndexed := countIndexedParams(signature)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.events[topic0] = ContractCreationEventInfo{
		Name:               name,
		Signature:          signature,
		ContractParamType:  paramType,
		ParamPosition:      paramIndex,
		ParamIndexed:       paramIndexed,
		TotalIndexedParams: totalIndexed,
	}

	if global.Logger != nil {
		global.Logger.Infof("Dynamically registered contract creation event: name=%s, topic0=%s", name, topic0.Hex())
	}
}

// GetEvent returns event info for a given topic0
func (c *ConfigurableContractCreationEvents) GetEvent(topic0 common.Hash) (ContractCreationEventInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	event, ok := c.events[topic0]
	return event, ok
}

// HasEvent checks if an event is registered
func (c *ConfigurableContractCreationEvents) HasEvent(topic0 common.Hash) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, ok := c.events[topic0]
	return ok
}

// GetAllEvents returns a copy of all registered events
func (c *ConfigurableContractCreationEvents) GetAllEvents() map[common.Hash]ContractCreationEventInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[common.Hash]ContractCreationEventInfo, len(c.events))
	for k, v := range c.events {
		result[k] = v
	}
	return result
}

// countIndexedParams counts the number of indexed parameters in an event signature
// e.g., "PairCreated(address indexed token0, address indexed token1, address pair, uint)" -> 2
func countIndexedParams(signature string) int {
	// This is a simplified counter - it counts "indexed" keywords
	// For more complex signatures, this may need adjustment
	// Extract parameter list from signature
	start := strings.IndexByte(signature, '(')
	end := strings.LastIndexByte(signature, ')')
	if start == -1 || end == -1 || end <= start {
		return 0
	}

	paramList := signature[start+1 : end]
	// Count occurrences of "indexed"
	return strings.Count(paramList, "indexed")
}

// ExtractContractAddressFromLogs extracts contract addresses from event logs.
// This handles Factory pattern contract creation (e.g., Uniswap V2 PairCreated events)
// when receipt.ContractAddress is empty (which happens for Factory-created contracts).
//
// Parameters:
//   - receipt: The transaction receipt containing event logs
//
// Returns:
//   - The contract address if found in logs, or empty string if not found
func ExtractContractAddressFromLogs(receipt *types.Receipt) string {
	if receipt == nil || len(receipt.Logs) == 0 {
		return ""
	}

	events := GetConfigurableEvents()

	// Search through all logs for known contract creation events
	for _, log := range receipt.Logs {
		if len(log.Topics) == 0 {
			continue
		}

		topic0 := log.Topics[0]

		// Check if this is a known contract creation event
		if eventInfo, known := events.GetEvent(topic0); known {
			contractAddr := extractAddressFromEvent(log, eventInfo)
			if contractAddr != (common.Address{}) {
				// Return lowercase address with 0x prefix (EVM standard format)
				return strings.ToLower(contractAddr.Hex())
			}
		}
	}

	return ""
}

// extractAddressFromEvent extracts the contract address from a specific event log
// based on the event configuration (parameter position and indexed status)
//
// For indexed parameters: they are in topics[1+paramIndex]
// For non-indexed parameters: they are in data[paramIndex*32:(paramIndex+1)*32]
func extractAddressFromEvent(log *types.Log, eventInfo ContractCreationEventInfo) common.Address {
	if eventInfo.ParamIndexed {
		// Indexed parameter is in topics
		// topics[0] is event signature, so indexed params start at topics[1]
		topicIndex := 1 + eventInfo.ParamPosition
		if topicIndex < len(log.Topics) {
			return common.BytesToAddress(log.Topics[topicIndex].Bytes())
		}
	} else {
		// Non-indexed parameter is in data
		// Each parameter takes 32 bytes
		dataStart := eventInfo.ParamPosition * 32
		dataEnd := dataStart + 32
		if len(log.Data) >= dataEnd {
			// Address is the last 20 bytes of the 32-byte word
			return common.BytesToAddress(log.Data[dataStart+12 : dataEnd])
		}
	}

	return common.Address{}
}

// HasKnownContractCreationEvent checks if the receipt contains a known contract creation event
// This can be used to quickly determine if logs parsing is needed
func HasKnownContractCreationEvent(receipt *types.Receipt) bool {
	if receipt == nil || len(receipt.Logs) == 0 {
		return false
	}

	events := GetConfigurableEvents()

	for _, log := range receipt.Logs {
		if len(log.Topics) == 0 {
			continue
		}

		if events.HasEvent(log.Topics[0]) {
			return true
		}
	}

	return false
}

// RegisterContractCreationEvent registers a new contract creation event signature
// This allows extending support for different DEX or Factory patterns
//
// Parameters:
//   - signature: The event signature string (e.g., "PairCreated(address,address,address,uint256)")
//   - paramType: The type of the contract address parameter ("address" or "indexed")
//   - position: For non-indexed params, the position in data field (0, 1, 2, etc.)
//
// Deprecated: Use GetConfigurableEvents().RegisterEvent() instead for more control
func RegisterContractCreationEvent(signature string, paramType string, position int) {
	GetConfigurableEvents().RegisterEvent(signature, paramType, position, false)
}

// extractEventName extracts the event name from a signature string
// e.g., "PairCreated(address,address,address,uint256)" -> "PairCreated"
func extractEventName(signature string) string {
	idx := strings.IndexByte(signature, '(')
	if idx == -1 {
		return signature
	}
	return signature[:idx]
}

// IsContractCreationTransaction determines if a transaction is a contract creation transaction
// by checking both direct CREATE (receipt.ContractAddress) and Factory pattern (logs)
func IsContractCreationTransaction(receipt *types.Receipt) bool {
	if receipt == nil {
		return false
	}

	// Direct contract creation
	if receipt.ContractAddress != (common.Address{}) {
		return true
	}

	// Factory pattern contract creation
	return HasKnownContractCreationEvent(receipt)
}

// GetContractCreationAddress returns the contract creation address from a receipt
// It checks both direct CREATE and Factory pattern
//
// Returns:
//   - The contract address if found, or empty string if not a contract creation tx
func GetContractCreationAddress(receipt *types.Receipt) string {
	if receipt == nil {
		return ""
	}

	// Check direct contract creation first
	if receipt.ContractAddress != (common.Address{}) {
		return receipt.ContractAddress.Hex()
	}

	// Check Factory pattern from logs
	return ExtractContractAddressFromLogs(receipt)
}

// PairCreatedData represents the data structure for PairCreated event data
// This is useful for debugging and testing
type PairCreatedData struct {
	Token0 common.Address
	Token1 common.Address
	Pair   common.Address
	// uint parameter is ignored for our purposes
}

// ParsePairCreatedEvent parses a PairCreated event log into structured data
// Returns nil if the log is not a valid PairCreated event
func ParsePairCreatedEvent(log *types.Log) *PairCreatedData {
	if len(log.Topics) < 3 || len(log.Data) < 32 {
		return nil
	}

	// Verify topic0 matches PairCreated signature
	if log.Topics[0] != PairCreatedEventSignature {
		return nil
	}

	return &PairCreatedData{
		Token0: common.BytesToAddress(log.Topics[1].Bytes()),
		Token1: common.BytesToAddress(log.Topics[2].Bytes()),
		Pair:   common.BytesToAddress(log.Data[12:32]),
	}
}

// ParseEventByConfig parses an event log using configuration-based rules
// This is a generic parser that can handle any configured event type
//
// Parameters:
//   - log: The event log to parse
//   - eventInfo: The event configuration
//
// Returns:
//   - The contract address if found, or empty address if not found
func ParseEventByConfig(log *types.Log, eventInfo ContractCreationEventInfo) common.Address {
	return extractAddressFromEvent(log, eventInfo)
}

// bytesToAddress converts bytes to common.Address, handling padding
// If the input is longer than 20 bytes, it takes the last 20 bytes
func bytesToAddress(data []byte) common.Address {
	if len(data) == 0 {
		return common.Address{}
	}

	// If data is exactly 20 bytes, convert directly
	if len(data) == 20 {
		return common.BytesToAddress(data)
	}

	// If data is 32 bytes (padded), take the last 20 bytes
	if len(data) == 32 {
		return common.BytesToAddress(data[12:])
	}

	// For other lengths, use the standard conversion
	return common.BytesToAddress(data)
}

// DebugLogPrint prints log information for debugging purposes
// This is useful for identifying unknown contract creation events
func DebugLogPrint(log *types.Log) string {
	if log == nil {
		return "nil log"
	}

	var sb strings.Builder
	sb.WriteString("Log:\n")
	sb.WriteString("  Address: " + log.Address.Hex() + "\n")
	sb.WriteString("  Topics:\n")
	for i, topic := range log.Topics {
		sb.WriteString("    [" + string(rune('0'+i)) + "] " + topic.Hex() + "\n")
	}
	sb.WriteString("  Data: 0x" + common.Bytes2Hex(log.Data) + "\n")
	return sb.String()
}

// TrimLeadingZeros removes leading zeros from a byte slice
// This is useful when comparing addresses that may have different padding
func TrimLeadingZeros(data []byte) []byte {
	return bytes.TrimLeft(data, "\x00")
}

// InitContractCreationEventsFromConfig initializes contract creation events from application configuration
// This should be called during application startup after configuration is loaded
//
// Parameters:
//   - evmConfig: EVM configuration containing contract creation event definitions
func InitContractCreationEventsFromConfig(evmConfig *conf.Chain_EVM) {
	if evmConfig == nil {
		if global.Logger != nil {
			global.Logger.Debug("No EVM config provided, using default contract creation events")
		}
		return
	}

	events := GetConfigurableEvents()
	events.LoadFromConfig(evmConfig.GetContractCreationEvents())
}
