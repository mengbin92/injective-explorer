// Package models provides data models for blockchain entities.
package models

import (
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/cosmos/cosmos-sdk/types"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"gorm.io/gorm"
)

// DelegationReward represents a delegation reward for a specific delegator-validator pair in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores delegation reward information from the Injective blockchain,
// including the delegator address, validator address, and reward details.
//
// Note: TotalReward field has been removed as it was misleading in multi-coin reward scenarios.
// Clients should calculate totals from the Rewards JSON field based on their specific needs.
type DelegationReward struct {
	gorm.Model

	// DelegatorAddress is the delegator's blockchain address (indexed for faster lookups)
	DelegatorAddress string `json:"delegator_address" gorm:"column:delegator_address;index"`
	// ValidatorAddress is the validator's blockchain address (indexed for faster lookups)
	ValidatorAddress string `json:"validator_address" gorm:"column:validator_address;index"`
	// Rewards contains the reward details stored as JSON
	// The JSON structure contains a "coins" array with denomination and amount for each reward coin
	Rewards string `json:"rewards" gorm:"column:rewards;type:text"`
}

// DelegationRewardDetail represents the detailed reward information stored as JSON.
type DelegationRewardDetail struct {
	// Coins is a list of reward coins with denomination and amount
	Coins []RewardCoin `json:"coins"`
}

// RewardCoin represents a single reward coin with denomination and amount.
type RewardCoin struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

// DelegationRewardFromProto converts a protobuf DelegationDelegatorReward to a DelegationReward model.
//
// Parameters:
//   - reward: The protobuf delegation reward object to convert
//   - delegatorAddress: The delegator address string
//
// Returns:
//   - *DelegationReward: A new DelegationReward instance with data from the protobuf message
//   - error: Error if JSON marshaling fails
//
// Note: This function returns an error if JSON marshaling fails, allowing the caller
// to handle the error appropriately (e.g., log, retry, or skip the record).
func DelegationRewardFromProto(reward *distributiontypes.DelegationDelegatorReward, delegatorAddress string) (*DelegationReward, error) {
	if reward == nil {
		return nil, fmt.Errorf("reward cannot be nil")
	}

	// Convert DecCoins to RewardCoin list
	coins := make([]RewardCoin, 0, len(reward.Reward))
	for _, coin := range reward.Reward {
		coins = append(coins, RewardCoin{
			Denom:  coin.Denom,
			Amount: coin.Amount.String(),
		})
	}

	// Create reward detail
	rewardDetail := DelegationRewardDetail{
		Coins: coins,
	}

	// Marshal to JSON - return error if marshaling fails
	rewardsJSON, err := sonic.Marshal(rewardDetail)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reward detail to JSON: %w", err)
	}

	return &DelegationReward{
		DelegatorAddress: delegatorAddress,
		ValidatorAddress: reward.ValidatorAddress,
		Rewards:          string(rewardsJSON),
	}, nil
}

// DelegationRewardFromDecCoins converts DecCoins to a DelegationReward model.
// This is used when we have rewards but not a DelegationDelegatorReward object.
//
// Parameters:
//   - rewards: The DecCoins reward object
//   - delegatorAddress: The delegator address string
//   - validatorAddress: The validator address string
//
// Returns:
//   - *DelegationReward: A new DelegationReward instance
//   - error: Error if JSON marshaling fails
//
// Note: This function returns an error if JSON marshaling fails, allowing the caller
// to handle the error appropriately (e.g., log, retry, or skip the record).
func DelegationRewardFromDecCoins(rewards types.DecCoins, delegatorAddress, validatorAddress string) (*DelegationReward, error) {
	// Convert DecCoins to RewardCoin list
	coins := make([]RewardCoin, 0, len(rewards))
	for _, coin := range rewards {
		coins = append(coins, RewardCoin{
			Denom:  coin.Denom,
			Amount: coin.Amount.String(),
		})
	}

	// Create reward detail
	rewardDetail := DelegationRewardDetail{
		Coins: coins,
	}

	// Marshal to JSON - return error if marshaling fails
	rewardsJSON, err := sonic.Marshal(rewardDetail)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reward detail to JSON: %w", err)
	}

	return &DelegationReward{
		DelegatorAddress: delegatorAddress,
		ValidatorAddress: validatorAddress,
		Rewards:          string(rewardsJSON),
	}, nil
}
