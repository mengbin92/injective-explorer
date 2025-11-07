package models

import (
	"encoding/base64"

	injective_explorer_rpcpb "github.com/InjectiveLabs/sdk-go/exchange/explorer_rpc/pb"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

type Validator struct {
	gorm.Model

	Id                      string   `json:"id" gorm:"column:validator_id"`
	Moniker                 string   `json:"moniker" gorm:"column:moniker"`
	OperatorAddress         string   `json:"operator_address" gorm:"column:operator_address"`
	ConsensusAddress        string   `json:"consensus_address" gorm:"column:consensus_address"`
	Jailed                  bool     `json:"jailed" gorm:"column:jailed"`
	Status                  int32    `json:"status" gorm:"column:status"`
	Tokens                  string   `json:"tokens" gorm:"column:tokens"`
	DelegatorShares         string   `json:"delegator_shares" gorm:"column:delegator_shares"`
	Description             string   `json:"description" gorm:"column:description"`
	UnbondingHeight         int64    `json:"unbonding_height" gorm:"column:unbonding_height"`
	UnbondingTime           string   `json:"unbonding_time" gorm:"column:unbonding_time"`
	CommissionRate          string   `json:"commission_rate" gorm:"column:commission_rate"`
	CommissionMaxRate       string   `json:"commission_max_rate" gorm:"column:commission_max_rate"`
	CommissionMaxChangeRate string   `json:"commission_max_change_rate" gorm:"column:commission_max_change_rate"`
	CommissionUpdateTime    string   `json:"commission_update_time" gorm:"column:commission_update_time"`
	Proposed                uint64   `json:"proposed" gorm:"column:proposed"`
	Signed                  uint64   `json:"signed" gorm:"column:signed"`
	Missed                  uint64   `json:"missed" gorm:"column:missed"`
	Timestamp               string   `json:"timestamp" gorm:"column:timestamp"`
	Uptimes                 []string `json:"uptimes" gorm:"column:uptimes;serializer:json"`
	SlashingEvents          []string `json:"slashing_events" gorm:"column:slashing_events;serializer:json"`
	UptimePercentage        float64  `json:"uptime_percentage" gorm:"column:uptime_percentage"`
	ImageUrl                string   `json:"image_url" gorm:"column:image_url"`
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
	description := ""
	if validator.Description != nil {
		// Marshal protobuf to binary
		if data, err := proto.Marshal(validator.Description); err == nil {
			// Encode as base64 for storage
			description = base64.StdEncoding.EncodeToString(data)
		} else {
			// Fallback to string representation if marshaling fails
			description = validator.Description.String()
		}
	}

	// Store Uptimes as base64-encoded protobuf binary for reversible conversion
	uptimes := make([]string, 0, len(validator.Uptimes))
	for _, uptime := range validator.Uptimes {
		if uptime != nil {
			// Marshal protobuf to binary
			if data, err := proto.Marshal(uptime); err == nil {
				// Encode as base64 for storage
				uptimes = append(uptimes, base64.StdEncoding.EncodeToString(data))
			} else {
				// Fallback to string representation if marshaling fails
				uptimes = append(uptimes, uptime.String())
			}
		}
	}

	// Store SlashingEvents as base64-encoded protobuf binary for reversible conversion
	slashingEvents := make([]string, 0, len(validator.SlashingEvents))
	for _, slashingEvent := range validator.SlashingEvents {
		if slashingEvent != nil {
			// Marshal protobuf to binary
			if data, err := proto.Marshal(slashingEvent); err == nil {
				// Encode as base64 for storage
				slashingEvents = append(slashingEvents, base64.StdEncoding.EncodeToString(data))
			} else {
				// Fallback to string representation if marshaling fails
				slashingEvents = append(slashingEvents, slashingEvent.String())
			}
		}
	}

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
	var description *injective_explorer_rpcpb.ValidatorDescription
	if validator.Description != "" {
		// Try to decode from base64
		data, err := base64.StdEncoding.DecodeString(validator.Description)
		if err == nil {
			// Successfully decoded, try to unmarshal protobuf
			var desc injective_explorer_rpcpb.ValidatorDescription
			if err := proto.Unmarshal(data, &desc); err == nil {
				description = &desc
			}
		}
		// If base64 decode or unmarshal fails, the string might be in text format
		// from old data. We'll leave description as nil to maintain compatibility
		// Note: This is a fallback for backward compatibility with old data format
	}

	// Convert Uptimes from base64-encoded protobuf binary back to protobuf format
	uptimes := make([]*injective_explorer_rpcpb.ValidatorUptime, 0, len(validator.Uptimes))
	for _, uptimeStr := range validator.Uptimes {
		if uptimeStr == "" {
			continue
		}

		// Try to decode from base64
		data, err := base64.StdEncoding.DecodeString(uptimeStr)
		if err == nil {
			// Successfully decoded, try to unmarshal protobuf
			var uptime injective_explorer_rpcpb.ValidatorUptime
			if err := proto.Unmarshal(data, &uptime); err == nil {
				uptimes = append(uptimes, &uptime)
				continue
			}
		}

		// If base64 decode or unmarshal fails, the string might be in text format
		// from old data. We'll create an empty ValidatorUptime to maintain compatibility
		// Note: This is a fallback for backward compatibility with old data format
		uptimes = append(uptimes, &injective_explorer_rpcpb.ValidatorUptime{})
	}

	// Convert SlashingEvents from base64-encoded protobuf binary back to protobuf format
	slashingEvents := make([]*injective_explorer_rpcpb.SlashingEvent, 0, len(validator.SlashingEvents))
	for _, slashingEventStr := range validator.SlashingEvents {
		if slashingEventStr == "" {
			continue
		}

		// Try to decode from base64
		data, err := base64.StdEncoding.DecodeString(slashingEventStr)
		if err == nil {
			// Successfully decoded, try to unmarshal protobuf
			var slashingEvent injective_explorer_rpcpb.SlashingEvent
			if err := proto.Unmarshal(data, &slashingEvent); err == nil {
				slashingEvents = append(slashingEvents, &slashingEvent)
				continue
			}
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
