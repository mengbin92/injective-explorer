// Package models provides data models for blockchain entities.
// This file defines the Validator model and its conversion functions.
package models

import (
	"injective-explorer/internal/global"
	appconstants "injective-explorer/internal/pkg/constants"
	"fmt"
	"time"

	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"github.com/bytedance/sonic"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// Validator represents a blockchain validator in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores validator information from the Injective blockchain,
// including validator address, status, commission rates, and performance metrics.
type Validator struct {
	gorm.Model

	// Id is the validator's unique identifier
	Id string `json:"id" gorm:"column:validator_id"`
	// Moniker is the validator's display name
	Moniker string `json:"moniker" gorm:"column:moniker"`
	// OperatorAddress is the validator's operator address (indexed for faster lookups)
	OperatorAddress string `json:"operator_address" gorm:"column:operator_address;index"`
	// ConsensusAddress is the validator's consensus address
	ConsensusAddress string `json:"consensus_address" gorm:"column:consensus_address"`
	// Jailed indicates whether the validator is jailed
	Jailed bool `json:"jailed" gorm:"column:jailed"`
	// Status is the validator's status code
	Status int32 `json:"status" gorm:"column:status"`
	// Tokens is the amount of tokens delegated to the validator
	Tokens string `json:"tokens" gorm:"column:tokens"`
	// DelegatorShares is the total delegator shares
	DelegatorShares string `json:"delegator_shares" gorm:"column:delegator_shares"`
	// Description contains the validator's description (stored as base64-encoded protobuf)
	Description string `json:"description" gorm:"column:description"`
	// UnbondingHeight is the height at which unbonding started
	UnbondingHeight int64 `json:"unbonding_height" gorm:"column:unbonding_height"`
	// UnbondingTime is the time when unbonding completes
	UnbondingTime string `json:"unbonding_time" gorm:"column:unbonding_time"`
	// CommissionRate is the validator's commission rate
	CommissionRate string `json:"commission_rate" gorm:"column:commission_rate"`
	// CommissionMaxRate is the maximum commission rate
	CommissionMaxRate string `json:"commission_max_rate" gorm:"column:commission_max_rate"`
	// CommissionMaxChangeRate is the maximum change rate per period
	CommissionMaxChangeRate string `json:"commission_max_change_rate" gorm:"column:commission_max_change_rate"`
	// CommissionUpdateTime is when the commission was last updated
	CommissionUpdateTime string `json:"commission_update_time" gorm:"column:commission_update_time"`
	// Proposed is the number of blocks proposed
	Proposed uint64 `json:"proposed" gorm:"column:proposed"`
	// Signed is the number of blocks signed
	Signed uint64 `json:"signed" gorm:"column:signed"`
	// Missed is the number of blocks missed
	Missed uint64 `json:"missed" gorm:"column:missed"`
	// Timestamp is when the validator information was last updated
	Timestamp string `json:"timestamp" gorm:"column:timestamp"`
	// Uptimes contains validator uptime information (stored as base64-encoded protobuf array)
	Uptimes []string `json:"uptimes" gorm:"column:uptimes;serializer:json"`
	// SlashingEvents contains slashing event information (stored as base64-encoded protobuf array)
	SlashingEvents []string `json:"slashing_events" gorm:"column:slashing_events;serializer:json"`
	// UptimePercentage is the validator's uptime percentage
	UptimePercentage float64 `json:"uptime_percentage" gorm:"column:uptime_percentage"`
	// ImageUrl is the validator's image URL
	ImageUrl string `json:"image_url" gorm:"column:image_url"`
	// RegistrationTime is the timestamp when the validator first proposed a block (registration time)
	RegistrationTime int64 `json:"registration_time" gorm:"column:registration_time"`
	// WalletAddress is the validator's wallet address (typically same as operator_address)
	WalletAddress string `json:"wallet_address" gorm:"column:wallet_address"`
}

// ValidatorFromProto converts a protobuf Validator to a Validator model.
//
// Parameters:
//   - validator: The protobuf validator information
//
// Returns:
//   - *Validator: A new Validator instance with data from the protobuf message
func ValidatorFromProto(validator *injective_explorer_rpcpb.Validator) *Validator {
	if validator == nil {
		return nil
	}

	// Store Description as JSON string for reversible conversion
	// Use shared utility function to eliminate code duplication
	description := ""
	if validator.Description != nil {
		encoded, err := MarshalProtoToJSON(validator.Description)
		if err == nil {
			description = encoded
		} else {
			// Fallback to empty string if marshaling fails
			// In production, you might want to log this error
			description = ""
		}
	}

	// Store Uptimes as JSON strings for reversible conversion
	// Use shared utility function to eliminate code duplication
	// Memory optimization: Limit the number of uptime entries to prevent unbounded growth
	uptimeMessages := make([]proto.Message, 0, len(validator.Uptimes))
	for _, uptime := range validator.Uptimes {
		uptimeMessages = append(uptimeMessages, uptime)
	}
	// Trim to most recent MaxUptimesEntries entries to prevent memory bloat
	if len(uptimeMessages) > appconstants.MaxUptimesEntries {
		// Keep only the most recent entries (last N items)
		uptimeMessages = uptimeMessages[len(uptimeMessages)-appconstants.MaxUptimesEntries:]
	}
	uptimes, _ := MarshalProtoSliceToJSON(uptimeMessages)

	// Store SlashingEvents as JSON strings for reversible conversion
	// Use shared utility function to eliminate code duplication
	// Memory optimization: Limit the number of slashing events to prevent unbounded growth
	slashingEventMessages := make([]proto.Message, 0, len(validator.SlashingEvents))
	for _, slashingEvent := range validator.SlashingEvents {
		slashingEventMessages = append(slashingEventMessages, slashingEvent)
	}
	// Trim to most recent MaxSlashingEventsEntries entries to prevent memory bloat
	if len(slashingEventMessages) > appconstants.MaxSlashingEventsEntries {
		// Keep only the most recent entries (last N items)
		slashingEventMessages = slashingEventMessages[len(slashingEventMessages)-appconstants.MaxSlashingEventsEntries:]
	}
	slashingEvents, _ := MarshalProtoSliceToJSON(slashingEventMessages)

	// Set wallet address: use operator_address as default if not provided
	walletAddress := validator.OperatorAddress
	// Note: The protobuf validator from chain may not have wallet_address field,
	// so we use operator_address as the default wallet address

	// Set registration time: default to 0, will be populated later if needed
	// The actual first block time will be fetched separately or use CreatedAt as fallback
	registrationTime := int64(0)

	return &Validator{
		Id:                      validator.Id,
		Moniker:                 validator.Moniker,
		OperatorAddress:         validator.OperatorAddress,
		ConsensusAddress:        validator.ConsensusAddress,
		Jailed:                  validator.Jailed,
		Status:                  validator.Status,
		Tokens:                  validator.Tokens,
		DelegatorShares:         validator.DelegatorShares,
		Description:             description,
		UnbondingHeight:         validator.UnbondingHeight,
		UnbondingTime:           validator.UnbondingTime,
		CommissionRate:          validator.CommissionRate,
		CommissionMaxRate:       validator.CommissionMaxRate,
		CommissionMaxChangeRate: validator.CommissionMaxChangeRate,
		CommissionUpdateTime:    validator.CommissionUpdateTime,
		Proposed:                validator.Proposed,
		Signed:                  validator.Signed,
		Missed:                  validator.Missed,
		Timestamp:               validator.Timestamp,
		Uptimes:                 uptimes,
		SlashingEvents:          slashingEvents,
		UptimePercentage:        validator.UptimePercentage,
		ImageUrl:                validator.ImageUrl,
		RegistrationTime:        registrationTime,
		WalletAddress:           walletAddress,
	}
}

// ValidatorToProto converts a Validator model back to a protobuf Validator.
//
// Parameters:
//   - validator: The Validator model from database
//
// Returns:
//   - *injective_explorer_rpcpb.Validator: A new Validator instance with data from the Validator model
func ValidatorToProto(validator *Validator) *injective_explorer_rpcpb.Validator {
	if validator == nil {
		return nil
	}

	// Parse Description from JSON string back to protobuf format
	// Use shared utility function to eliminate code duplication
	var description *injective_explorer_rpcpb.ValidatorDescription
	if validator.Description != "" {
		var desc injective_explorer_rpcpb.ValidatorDescription
		if err := UnmarshalProtoFromJSON(validator.Description, &desc); err == nil {
			description = &desc
		}
		// If JSON unmarshal fails, the string might be in old format
		// from old data. We'll leave description as nil to maintain compatibility
		// Note: This is a fallback for backward compatibility with old data format
	}

	// Convert Uptimes from JSON strings back to protobuf format
	// Use shared utility function to eliminate code duplication
	uptimes := make([]*injective_explorer_rpcpb.ValidatorUptime, 0, len(validator.Uptimes))
	for _, uptimeStr := range validator.Uptimes {
		if uptimeStr == "" {
			continue
		}

		var uptime injective_explorer_rpcpb.ValidatorUptime
		if err := UnmarshalProtoFromJSON(uptimeStr, &uptime); err == nil {
			uptimes = append(uptimes, &uptime)
			continue
		}

		// If JSON unmarshal fails, the string might be in old format
		// from old data. We'll create an empty ValidatorUptime to maintain compatibility
		// Note: This is a fallback for backward compatibility with old data format
		uptimes = append(uptimes, &injective_explorer_rpcpb.ValidatorUptime{})
	}

	// Convert SlashingEvents from JSON strings back to protobuf format
	// Use shared utility function to eliminate code duplication
	slashingEvents := make([]*injective_explorer_rpcpb.SlashingEvent, 0, len(validator.SlashingEvents))
	for _, slashingEventStr := range validator.SlashingEvents {
		if slashingEventStr == "" {
			continue
		}

		var slashingEvent injective_explorer_rpcpb.SlashingEvent
		if err := UnmarshalProtoFromJSON(slashingEventStr, &slashingEvent); err == nil {
			slashingEvents = append(slashingEvents, &slashingEvent)
			continue
		}

		// If JSON unmarshal fails, the string might be in old format
		// from old data. We'll create an empty SlashingEvent to maintain compatibility
		// Note: This is a fallback for backward compatibility with old data format
		slashingEvents = append(slashingEvents, &injective_explorer_rpcpb.SlashingEvent{})
	}

	return &injective_explorer_rpcpb.Validator{
		Id:                      validator.Id,
		Moniker:                 validator.Moniker,
		OperatorAddress:         validator.OperatorAddress,
		ConsensusAddress:        validator.ConsensusAddress,
		Jailed:                  validator.Jailed,
		Status:                  validator.Status,
		Tokens:                  validator.Tokens,
		DelegatorShares:         validator.DelegatorShares,
		Description:             description,
		UnbondingHeight:         validator.UnbondingHeight,
		UnbondingTime:           validator.UnbondingTime,
		CommissionRate:          validator.CommissionRate,
		CommissionMaxRate:       validator.CommissionMaxRate,
		CommissionMaxChangeRate: validator.CommissionMaxChangeRate,
		CommissionUpdateTime:    validator.CommissionUpdateTime,
		Proposed:                validator.Proposed,
		Signed:                  validator.Signed,
		Missed:                  validator.Missed,
		Timestamp:               validator.Timestamp,
		Uptimes:                 uptimes,
		SlashingEvents:          slashingEvents,
		UptimePercentage:        validator.UptimePercentage,
		ImageUrl:                validator.ImageUrl,
	}
}

// ValidatorFromStakingTypes converts a cosmos-sdk stakingtypes.Validator to a Validator model.
//
// Parameters:
//   - validator: The cosmos-sdk staking validator
//
// Returns:
//   - *Validator: A new Validator instance with data from the staking validator
func ValidatorFromStakingTypes(validator stakingtypes.Validator) *Validator {
	if validator.OperatorAddress == "" {
		return nil
	}

	// Convert description to JSON string
	description := ""
	if validator.Description.Moniker != "" || validator.Description.Identity != "" || validator.Description.Website != "" || validator.Description.SecurityContact != "" || validator.Description.Details != "" {
		desc := map[string]string{
			"moniker":          validator.Description.Moniker,
			"identity":         validator.Description.Identity,
			"website":          validator.Description.Website,
			"security_contact": validator.Description.SecurityContact,
			"details":          validator.Description.Details,
		}
		// Use sonic to marshal the description map to JSON
		descJSON, err := sonic.Marshal(desc)
		if err == nil {
			description = string(descJSON)
		}
	}

	// Convert tokens to string
	tokens := validator.Tokens.String()

	// Convert delegator shares to string
	delegatorShares := validator.DelegatorShares.String()

	// Convert commission rates to string
	commissionRate := validator.Commission.CommissionRates.Rate.String()
	commissionMaxRate := validator.Commission.CommissionRates.MaxRate.String()
	commissionMaxChangeRate := validator.Commission.CommissionRates.MaxChangeRate.String()

	// Convert commission update time
	commissionUpdateTime := ""
	if !validator.Commission.UpdateTime.IsZero() {
		commissionUpdateTime = validator.Commission.UpdateTime.Format(time.RFC3339)
	}

	// Convert unbonding time
	unbondingTime := ""
	if !validator.UnbondingTime.IsZero() {
		unbondingTime = validator.UnbondingTime.Format(time.RFC3339)
	}

	// Get consensus address from validator using GetConsAddr() method
	consensusAddress := ""
	if validator.ConsensusPubkey != nil {
		consensusPubKey, err := parseConsensusPubKey(validator.ConsensusPubkey)
		if err == nil {
			consensusAddress = consensusPubKey.Address().String()
		} else {
			global.Logger.Errorf("failed to parse consensus pubkey: %v", err)
		}
	}

	// Set wallet address (use operator address as default)
	walletAddress := validator.OperatorAddress

	return &Validator{
		Id:                      "", // stakingtypes.Validator doesn't have an ID field
		Moniker:                 validator.Description.Moniker,
		OperatorAddress:         validator.OperatorAddress,
		ConsensusAddress:        consensusAddress,
		Jailed:                  validator.Jailed,
		Status:                  int32(validator.Status),
		Tokens:                  tokens,
		DelegatorShares:         delegatorShares,
		Description:             description,
		UnbondingHeight:         validator.UnbondingHeight,
		UnbondingTime:           unbondingTime,
		CommissionRate:          commissionRate,
		CommissionMaxRate:       commissionMaxRate,
		CommissionMaxChangeRate: commissionMaxChangeRate,
		CommissionUpdateTime:    commissionUpdateTime,
		Proposed:                0, // Not available in stakingtypes.Validator
		Signed:                  0, // Not available in stakingtypes.Validator
		Missed:                  0, // Not available in stakingtypes.Validator
		Timestamp:               time.Now().Format(time.RFC3339),
		Uptimes:                 []string{}, // Not available in stakingtypes.Validator
		SlashingEvents:          []string{}, // Not available in stakingtypes.Validator
		UptimePercentage:        0.0,        // Not available in stakingtypes.Validator
		ImageUrl:                "",         // Not available in stakingtypes.Validator
		RegistrationTime:        0,
		WalletAddress:           walletAddress,
	}
}

func parseConsensusPubKey(any *codectypes.Any) (cryptotypes.PubKey, error) {
	if any == nil {
		return nil, fmt.Errorf("consensus pubkey is nil")
	}

	// Create ProtoCodec with the InterfaceRegistry
	cdc := codec.NewProtoCodec(makeInterfaceRegistry())

	// Unpack the Any type to PubKey interface
	var pk cryptotypes.PubKey
	err := cdc.UnpackAny(any, &pk)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack consensus pubkey: %w", err)
	}

	return pk, nil
}

func makeInterfaceRegistry() codectypes.InterfaceRegistry {
	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	return registry
}