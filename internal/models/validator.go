// Package models provides data models for blockchain entities.
// This file defines the Validator model and its conversion functions.
package models

import (
	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
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

	// Store Description as base64-encoded protobuf binary for reversible conversion
	// Use shared utility function to eliminate code duplication
	description := ""
	if validator.Description != nil {
		encoded, err := MarshalProtoToBase64(validator.Description)
		if err == nil {
			description = encoded
		} else {
			// Fallback to string representation if marshaling fails
			description = validator.Description.String()
		}
	}

	// Store Uptimes as base64-encoded protobuf binary for reversible conversion
	// Use shared utility function to eliminate code duplication
	uptimeMessages := make([]proto.Message, 0, len(validator.Uptimes))
	for _, uptime := range validator.Uptimes {
		uptimeMessages = append(uptimeMessages, uptime)
	}
	uptimes, _ := MarshalProtoSliceToBase64(uptimeMessages)

	// Store SlashingEvents as base64-encoded protobuf binary for reversible conversion
	// Use shared utility function to eliminate code duplication
	slashingEventMessages := make([]proto.Message, 0, len(validator.SlashingEvents))
	for _, slashingEvent := range validator.SlashingEvents {
		slashingEventMessages = append(slashingEventMessages, slashingEvent)
	}
	slashingEvents, _ := MarshalProtoSliceToBase64(slashingEventMessages)

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

	// Parse Description from base64-encoded protobuf binary back to protobuf format
	// Use shared utility function to eliminate code duplication
	var description *injective_explorer_rpcpb.ValidatorDescription
	if validator.Description != "" {
		var desc injective_explorer_rpcpb.ValidatorDescription
		if err := UnmarshalProtoFromBase64(validator.Description, &desc); err == nil {
			description = &desc
		}
		// If base64 decode or unmarshal fails, the string might be in text format
		// from old data. We'll leave description as nil to maintain compatibility
		// Note: This is a fallback for backward compatibility with old data format
	}

	// Convert Uptimes from base64-encoded protobuf binary back to protobuf format
	// Use shared utility function to eliminate code duplication
	uptimes := make([]*injective_explorer_rpcpb.ValidatorUptime, 0, len(validator.Uptimes))
	for _, uptimeStr := range validator.Uptimes {
		if uptimeStr == "" {
			continue
		}

		var uptime injective_explorer_rpcpb.ValidatorUptime
		if err := UnmarshalProtoFromBase64(uptimeStr, &uptime); err == nil {
			uptimes = append(uptimes, &uptime)
			continue
		}

		// If base64 decode or unmarshal fails, the string might be in text format
		// from old data. We'll create an empty ValidatorUptime to maintain compatibility
		// Note: This is a fallback for backward compatibility with old data format
		uptimes = append(uptimes, &injective_explorer_rpcpb.ValidatorUptime{})
	}

	// Convert SlashingEvents from base64-encoded protobuf binary back to protobuf format
	// Use shared utility function to eliminate code duplication
	slashingEvents := make([]*injective_explorer_rpcpb.SlashingEvent, 0, len(validator.SlashingEvents))
	for _, slashingEventStr := range validator.SlashingEvents {
		if slashingEventStr == "" {
			continue
		}

		var slashingEvent injective_explorer_rpcpb.SlashingEvent
		if err := UnmarshalProtoFromBase64(slashingEventStr, &slashingEvent); err == nil {
			slashingEvents = append(slashingEvents, &slashingEvent)
			continue
		}

		// If base64 decode or unmarshal fails, the string might be in text format
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
