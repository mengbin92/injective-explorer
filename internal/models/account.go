// Package models provides data models for blockchain entities.
// It defines structures for accounts, blocks, transactions, and their conversions
// between protobuf and database representations.
package models

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/InjectiveLabs/sdk-go/chain/types"
	"github.com/btcsuite/btcd/btcutil/bech32"
	"github.com/bytedance/sonic"
	"github.com/cosmos/cosmos-sdk/codec"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	github_com_cosmos_cosmos_sdk_types "github.com/cosmos/cosmos-sdk/types"
	"gorm.io/gorm"
)

// recoveryContext provides context for recovery operations in account processing.
type recoveryContext struct {
	operation   string // e.g., "extracting pub key information"
	identifier  string // e.g., account address
	fallback    string // e.g., "using empty pub key values"
}

// safeExecuteRecovery executes a function with panic recovery for account processing.
//
// This local helper centralizes panic recovery logic for account operations,
// following the same pattern as the chain package recovery helpers.
//
// IMPORTANT: This function recovers from panics and continues execution with fallback values.
// This is acceptable for:
//   - Handling corrupted protobuf data from external sources
//   - Defensive programming against nil pointer access in optional pub key fields
//   - Graceful degradation when non-critical data is unavailable
//
// This is NOT appropriate for:
//   - Expected errors that should be returned to the caller
//   - Critical failures where continuing would produce incorrect data
//
// Parameters:
//   - ctx: Recovery context for logging
//   - fn: Function to execute safely
//
// Returns:
//   - bool: true if a panic was recovered, false otherwise
func safeExecuteRecovery(ctx *recoveryContext, fn func()) (recovered bool) {
	if ctx == nil {
		ctx = &recoveryContext{
			operation:  "unknown operation",
			identifier: "unknown",
			fallback:   "continuing with default values",
		}
	}

	defer func() {
		if r := recover(); r != nil {
			recovered = true
			// Log at warn level for corrupted data (not error, as this is recoverable)
			fmt.Printf(
				"[WARN] Recovered from panic during %s for %s: %v. Fallback: %s\n",
				ctx.operation,
				ctx.identifier,
				r,
				ctx.fallback,
			)
		}
	}()

	fn()
	return false
}

// Account represents a blockchain account in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores account information from the Injective blockchain,
// including the account address, base account details, and code hash.
type Account struct {
	gorm.Model

	// Address is the blockchain address
	Address string `json:"address" gorm:"column:address"`
	// BaseAccount contains serialized base account information (stored as JSON)
	BaseAccount string `json:"base_account" gorm:"column:base_account"`
	// CodeHash is the hash of the account's code (if it's a contract account)
	CodeHash string `json:"code_hash" gorm:"column:code_hash"`
}

// BaseAccount represents the base account information.
// It contains the core account details that are stored as JSON in the Account model.
type BaseAccount struct {
	// Address is the account address
	Address string `json:"address" gorm:"column:address"`
	// PubKey is the public key of the account
	PubKey string `json:"pub_key" gorm:"column:pub_key"`
	// AccountNumber is the account number assigned by the blockchain
	AccountNumber uint64 `json:"account_number" gorm:"column:account_number"`
	// Sequence is the account sequence number for transaction ordering
	Sequence uint64 `json:"sequence" gorm:"column:sequence"`
}

type PubKey struct {
	Type    string `json:"type" gorm:"column:type"`
	Value   string `json:"value" gorm:"column:value"`
	Address string `json:"evm_address" gorm:"column:evm_address"`
}

// AccountFromProto converts a protobuf EthAccount to an Account model.
//
// Parameters:
//   - account: The protobuf account object to convert
//   - address: The account address string
//
// Returns:
//   - *Account: A new Account instance with data from the protobuf message
func AccountFromProto(account *types.EthAccount, address string, codec codec.Codec) *Account {
	if account == nil {
		return nil
	}

	// Handle case where PubKey might be nil
	var pub *PubKey
	if account.PubKey != nil {
		var pubKey cryptotypes.PubKey
		err := codec.UnpackAny(account.PubKey, &pubKey)
		if err != nil {
			return nil
		}

		// Safely extract pub key information with recovery for corrupted/nil values
		var pubType string
		var pubValue string
		var pubAddress string

		safeExecuteRecovery(
			&recoveryContext{
				operation:  "extracting pub key information",
				identifier: address,
				fallback:   "using empty pub key values",
			},
			func() {
				pubType = pubKey.Type()
				pubValue = hex.EncodeToString(pubKey.Bytes())
				// Pre-calculate lowercase address to avoid repeated ToLower calls
				addrStr := pubKey.Address().String()
				lowerAddr := strings.ToLower(addrStr)
				pubAddress = fmt.Sprintf("0x%s", lowerAddr)
			},
		)

		pub = &PubKey{
			Type:    pubType,
			Value:   pubValue,
			Address: pubAddress,
		}
	} else {
		// Create a PubKey with empty values when PubKey is nil
		pub = &PubKey{
			Type:    "",
			Value:   "",
			Address: "",
		}
	}

	pubKeyJSON, _ := sonic.Marshal(pub)
	baseAccount := &BaseAccount{
		Address:       address,
		PubKey:        string(pubKeyJSON),
		AccountNumber: account.AccountNumber,
		Sequence:      account.Sequence,
	}
	baseAccountJSON, _ := sonic.Marshal(baseAccount)
	// Convert address to Ethereum format for storage
	// Database stores addresses in Ethereum format (0x...) for consistency with browser display
	ethAddress := ConvertAddressToEthForStorage(address)
	return &Account{
		Address:     ethAddress,
		BaseAccount: string(baseAccountJSON),
		CodeHash:    hex.EncodeToString(account.CodeHash),
	}
}

// AccountBalance represents an account balance for a specific denomination in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores balance information for accounts, including the address,
// denomination (token type), and amount.
type AccountBalance struct {
	gorm.Model

	// Address is the account address
	Address string `json:"address" gorm:"column:address"`
	// Denom is the denomination (token type) of the balance
	Denom string `json:"denom" gorm:"column:denom"`
	// Amount is the balance amount as a string (supports large numbers)
	Amount string `json:"amount" gorm:"column:amount"`
}

// AccountBalanceFromProto converts a Cosmos SDK Coin to an AccountBalance model.
//
// Parameters:
//   - coin: The Cosmos SDK coin object to convert
//   - address: The account address string
//
// Returns:
//   - *AccountBalance: A new AccountBalance instance with data from the coin
func AccountBalanceFromProto(coin github_com_cosmos_cosmos_sdk_types.Coin, address string) *AccountBalance {
	// Convert address to Ethereum format for storage
	// Database stores addresses in Ethereum format (0x...) for consistency with browser display
	ethAddress := ConvertAddressToEthForStorage(address)
	return &AccountBalance{
		Address: ethAddress,
		Denom:   coin.Denom,
		Amount:  coin.Amount.String(),
	}
}

// ConvertAddressToEthForStorage converts an address to Ethereum format (0x...) for database storage.
// This function ensures all addresses are stored in a consistent Ethereum format (0x...) in the database,
// regardless of the input format (Injective inj1... or Ethereum 0x...).
//
// Storage Policy:
//   - All addresses in the database are stored in Ethereum format (0x...) for consistency
//   - This simplifies queries and ensures uniform display in the browser
//   - Injective addresses (inj1...) are converted to Ethereum format (0x...)
//   - Ethereum addresses are validated and normalized to lowercase
//
// Parameters:
//   - address: The address to convert (can be inj1... or 0x... format)
//
// Returns:
//   - string: The address in Ethereum format (0x...), or original if conversion fails
//
// Note: If conversion fails, the original address is returned for backward compatibility
// with existing data that might be in unexpected formats.
func ConvertAddressToEthForStorage(address string) string {
	if address == "" {
		return address
	}

	// If already Ethereum format, validate and normalize (lowercase)
	if strings.HasPrefix(address, "0x") {
		// Normalize to lowercase for consistency
		return strings.ToLower(address)
	}

	// Try to convert from Injective format
	ethAddr, err := InjToEthAddress(address)
	if err != nil {
		// If conversion fails, return original address (for backward compatibility)
		// This handles edge cases where address might be in an unexpected format
		// In production, you might want to log this error for monitoring
		return address
	}

	return ethAddr
}

// Error definitions for address conversion
var (
	ErrInvalidAddress = fmt.Errorf("invalid address format")
	ErrConversion     = fmt.Errorf("address conversion failed")
)

// IsValidInjectiveAddress validates a Injective address format.
// Checks if the address conforms to the inj1... format and can be correctly decoded.
//
// Parameters:
//   - addr: The address string to validate
//
// Returns:
//   - bool: True if the address is valid, false otherwise
func IsValidInjectiveAddress(addr string) bool {
	if addr == "" {
		return false
	}
	if len(addr) < 10 || !strings.HasPrefix(addr, "inj1") {
		return false
	}
	_, _, err := bech32.Decode(addr)
	return err == nil
}

// IsValidInjectiveAddress is an alias for IsValidInjectiveAddress for backward compatibility.
// Deprecated: Use IsValidInjectiveAddress instead.
func IsValidInjectiveAddress(addr string) bool {
	return IsValidInjectiveAddress(addr)
}

// EthToInjectiveAddress converts an Ethereum address (0x...) to Injective address (inj1...).
//
// Parameters:
//   - ethAddr: The Ethereum address string (0x...)
//
// Returns:
//   - string: The Injective address (inj1...)
//   - error: Error if conversion fails
func EthToInjectiveAddress(ethAddr string) (string, error) {
	if ethAddr == "" {
		return "", fmt.Errorf("%w: empty address", ErrInvalidAddress)
	}

	// Validate Ethereum address format
	if !strings.HasPrefix(ethAddr, "0x") {
		return "", fmt.Errorf("%w: invalid Ethereum address format, must start with 0x", ErrInvalidAddress)
	}

	// Remove 0x prefix and convert to lowercase
	hexStr := strings.ToLower(strings.TrimPrefix(ethAddr, "0x"))

	// Validate hex string length and characters
	if len(hexStr) != 40 {
		return "", fmt.Errorf("%w: invalid Ethereum address length", ErrInvalidAddress)
	}

	// Validate hex characters
	for _, c := range hexStr {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return "", fmt.Errorf("%w: invalid hex character in Ethereum address", ErrInvalidAddress)
		}
	}

	// Convert hex string to bytes
	addrBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return "", fmt.Errorf("%w: failed to decode hex string: %v", ErrConversion, err)
	}

	// 8-bit → 5-bit conversion for bech32
	conv, err := bech32.ConvertBits(addrBytes, 8, 5, true)
	if err != nil {
		return "", fmt.Errorf("%w: failed to convert bits: %v", ErrConversion, err)
	}

	// Encode as bech32 with "inj" prefix
	injAddr, err := bech32.Encode("inj", conv)
	if err != nil {
		return "", fmt.Errorf("%w: failed to encode bech32 address: %v", ErrConversion, err)
	}

	return injAddr, nil
}

// EthToInjAddress is an alias for EthToInjectiveAddress for backward compatibility.
// Deprecated: Use EthToInjectiveAddress instead.
func EthToInjAddress(ethAddr string) (string, error) {
	return EthToInjectiveAddress(ethAddr)
}

// InjectiveToEthAddress converts a Injective address (inj1...) to Ethereum address (0x...).
// This is the reverse operation of EthToInjectiveAddress.
//
// Parameters:
//   - injAddr: The Injective address string (inj1...) or Ethereum address (0x...)
//
// Returns:
//   - string: The Ethereum address (0x...)
//   - error: Error if conversion fails
func InjectiveToEthAddress(injAddr string) (string, error) {
	if injAddr == "" {
		return "", fmt.Errorf("%w: empty address", ErrInvalidAddress)
	}

	// If it's already an Ethereum address, validate and return as-is
	if strings.HasPrefix(injAddr, "0x") {
		// Basic validation: should be 42 characters (0x + 40 hex chars)
		if len(injAddr) != 42 {
			return "", fmt.Errorf("%w: invalid Ethereum address length", ErrInvalidAddress)
		}
		// Validate hex characters
		for i := 2; i < len(injAddr); i++ {
			c := injAddr[i]
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return "", fmt.Errorf("%w: invalid hex character in Ethereum address", ErrInvalidAddress)
			}
		}
		return strings.ToLower(injAddr), nil
	}

	// Validate Injective address format
	if !IsValidInjectiveAddress(injAddr) {
		return "", fmt.Errorf("%w: invalid Injective address format: %s", ErrInvalidAddress, injAddr)
	}

	// 1. bech32 decode
	hrp, data, err := bech32.Decode(injAddr)
	if err != nil {
		return "", fmt.Errorf("%w: failed to decode bech32 address: %v", ErrConversion, err)
	}
	if hrp != "inj" {
		return "", fmt.Errorf("%w: invalid HRP, expected 'inj', got '%s'", ErrConversion, hrp)
	}

	// 2. 5-bit → 8-bit
	conv, err := bech32.ConvertBits(data, 5, 8, false)
	if err != nil {
		return "", fmt.Errorf("%w: failed to convert bits: %v", ErrConversion, err)
	}
	if len(conv) != 20 {
		return "", fmt.Errorf("%w: invalid address length: expected 20 bytes, got %d", ErrConversion, len(conv))
	}

	// 3. bytes → hex with 0x prefix (lowercase for consistency)
	return fmt.Sprintf("0x%s", strings.ToLower(hex.EncodeToString(conv))), nil
}

// InjToEthAddress is an alias for InjectiveToEthAddress for backward compatibility.
// Deprecated: Use InjectiveToEthAddress instead.
func InjToEthAddress(injAddr string) (string, error) {
	return InjectiveToEthAddress(injAddr)
}

// HexToValconsAddress converts a hex string address to Cosmos validator consensus address (injvalcons1...).
// This is used to convert Tendermint proposer addresses to the bech32 format.
//
// Parameters:
//   - hexAddr: The hex string address (e.g., "29F697D164538C3BDF7969F238E2097FE745D73C")
//
// Returns:
//   - string: The validator consensus address in bech32 format (e.g., "injvalcons1eh644nvhkvdxwe3t6zltcee7td064mqy2p2d2n")
//   - error: Error if conversion fails
func HexToValconsAddress(hexAddr string) (string, error) {
	if hexAddr == "" {
		return "", fmt.Errorf("%w: empty address", ErrInvalidAddress)
	}

	// Remove any 0x prefix if present
	hexStr := strings.TrimPrefix(strings.ToLower(hexAddr), "0x")

	// Convert hex string to bytes
	addrBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return "", fmt.Errorf("%w: failed to decode hex string: %v", ErrConversion, err)
	}

	// 8-bit → 5-bit conversion for bech32
	conv, err := bech32.ConvertBits(addrBytes, 8, 5, true)
	if err != nil {
		return "", fmt.Errorf("%w: failed to convert bits: %v", ErrConversion, err)
	}

	// Encode as bech32 with "injvalcons" prefix
	valconsAddr, err := bech32.Encode("injvalcons", conv)
	if err != nil {
		return "", fmt.Errorf("%w: failed to encode bech32 address: %v", ErrConversion, err)
	}

	return valconsAddr, nil
}

// ValoperToAccAddress converts a validator operator address (injvaloper1...) to an account address (inj1...).
// In Cosmos SDK, both addresses share the same 20-byte data, only the bech32 HRP differs.
//
// Parameters:
//   - valoperAddr: The validator operator address (injvaloper1...)
//
// Returns:
//   - string: The account address (inj1...)
//   - error: Error if conversion fails
func ValoperToAccAddress(valoperAddr string) (string, error) {
	if valoperAddr == "" {
		return "", fmt.Errorf("%w: empty validator address", ErrInvalidAddress)
	}

	// Validate prefix
	if !strings.HasPrefix(valoperAddr, "injvaloper1") {
		return "", fmt.Errorf("%w: invalid validator address format, expected injvaloper1...", ErrInvalidAddress)
	}

	// bech32 decode
	hrp, data, err := bech32.Decode(valoperAddr)
	if err != nil {
		return "", fmt.Errorf("%w: failed to decode bech32 address: %v", ErrConversion, err)
	}
	if hrp != "injvaloper" {
		return "", fmt.Errorf("%w: invalid HRP, expected 'injvaloper', got '%s'", ErrConversion, hrp)
	}

	// Re-encode with account HRP
	accAddr, err := bech32.Encode("inj", data)
	if err != nil {
		return "", fmt.Errorf("%w: failed to encode bech32 address: %v", ErrConversion, err)
	}

	return accAddr, nil
}

// AccToValoperAddress converts an account address (inj1...) to a validator operator address (injvaloper1...).
//
// Parameters:
//   - accAddr: The account address (inj1...)
//
// Returns:
//   - string: The validator operator address (injvaloper1...)
//   - error: Error if conversion fails
func AccToValoperAddress(accAddr string) (string, error) {
	if accAddr == "" {
		return "", fmt.Errorf("%w: empty account address", ErrInvalidAddress)
	}

	// Validate prefix
	if !strings.HasPrefix(accAddr, "inj1") {
		return "", fmt.Errorf("%w: invalid account address format, expected inj1...", ErrInvalidAddress)
	}

	// bech32 decode
	hrp, data, err := bech32.Decode(accAddr)
	if err != nil {
		return "", fmt.Errorf("%w: failed to decode bech32 address: %v", ErrConversion, err)
	}
	if hrp != "inj" {
		return "", fmt.Errorf("%w: invalid HRP, expected 'inj', got '%s'", ErrConversion, hrp)
	}

	// Re-encode with validator HRP
	valoperAddr, err := bech32.Encode("injvaloper", data)
	if err != nil {
		return "", fmt.Errorf("%w: failed to encode bech32 address: %v", ErrConversion, err)
	}

	return valoperAddr, nil
}

// ValoperToEthAddress converts a validator operator address (injvaloper1...) to an Ethereum address (0x...).
// Conversion path: injvaloper1... → inj1... → 0x...
//
// Parameters:
//   - valoperAddr: The validator operator address (injvaloper1...)
//
// Returns:
//   - string: The Ethereum address (0x...)
//   - error: Error if conversion fails
func ValoperToEthAddress(valoperAddr string) (string, error) {
	// First convert to account address
	accAddr, err := ValoperToAccAddress(valoperAddr)
	if err != nil {
		return "", err
	}

	// Then convert to Ethereum address
	return InjToEthAddress(accAddr)
}
