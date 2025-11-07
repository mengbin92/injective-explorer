// Package models provides data models for blockchain entities.
package models

import (
	"encoding/json"

	"github.com/cosmos/cosmos-sdk/types"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"gorm.io/gorm"
)

// DelegationReward represents a delegation reward for a specific delegator-validator pair in the database.
// It embeds gorm.Model which provides ID, CreatedAt, UpdatedAt, and DeletedAt fields.
//
// The struct stores delegation reward information from the Injective blockchain,
// including the delegator address, validator address, and reward details.
type DelegationReward struct {
	gorm.Model

	// DelegatorAddress is the delegator's blockchain address (indexed for faster lookups)
	DelegatorAddress string `json:"delegator_address" gorm:"column:delegator_address;index"`
	// ValidatorAddress is the validator's blockchain address (indexed for faster lookups)
	ValidatorAddress string `json:"validator_address" gorm:"column:validator_address;index"`
	// Rewards contains the reward details stored as JSON
	Rewards string `json:"rewards" gorm:"column:rewards;type:text"`
	// TotalReward is the total reward amount as a string (for quick access)
	TotalReward string `json:"total_reward" gorm:"column:total_reward"`
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
func DelegationRewardFromProto(reward *distributiontypes.DelegationDelegatorReward, delegatorAddress string) *DelegationReward {
	if reward == nil {
		return nil
	}

	// Convert DecCoins to RewardCoin list
	coins := make([]RewardCoin, 0, len(reward.Reward))
	var totalAmount string
	for _, coin := range reward.Reward {
		coins = append(coins, RewardCoin{
			Denom:  coin.Denom,
			Amount: coin.Amount.String(),
		})
		// For simplicity, we'll store the first coin's amount as total
		// In practice, you might want to sum all coins or store them separately
		if totalAmount == "" {
			totalAmount = coin.Amount.String()
		}
	}

	// Create reward detail
	rewardDetail := DelegationRewardDetail{
		Coins: coins,
	}

	// Marshal to JSON
	rewardsJSON, err := json.Marshal(rewardDetail)
	if err != nil {
		// If marshaling fails, store empty string
		rewardsJSON = []byte("{}")
	}

	return &DelegationReward{
		DelegatorAddress: delegatorAddress,
		ValidatorAddress: reward.ValidatorAddress,
		Rewards:          string(rewardsJSON),
		TotalReward:      totalAmount,
	}
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
func DelegationRewardFromDecCoins(rewards types.DecCoins, delegatorAddress, validatorAddress string) *DelegationReward {
	// Convert DecCoins to RewardCoin list
	coins := make([]RewardCoin, 0, len(rewards))
	var totalAmount string
	for _, coin := range rewards {
		coins = append(coins, RewardCoin{
			Denom:  coin.Denom,
			Amount: coin.Amount.String(),
		})
		if totalAmount == "" {
			totalAmount = coin.Amount.String()
		}
	}

	// Create reward detail
	rewardDetail := DelegationRewardDetail{
		Coins: coins,
	}

	// Marshal to JSON
	rewardsJSON, err := json.Marshal(rewardDetail)
	if err != nil {
		rewardsJSON = []byte("{}")
	}

	return &DelegationReward{
		DelegatorAddress: delegatorAddress,
		ValidatorAddress: validatorAddress,
		Rewards:          string(rewardsJSON),
		TotalReward:      totalAmount,
	}
}
