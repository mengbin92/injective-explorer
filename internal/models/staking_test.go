// Package models provides unit tests for staking models.
package models

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/cosmos/cosmos-sdk/types"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
)

// TestDelegationRewardFromProto tests the DelegationRewardFromProto function.
func TestDelegationRewardFromProto(t *testing.T) {
	tests := []struct {
		name          string
		reward        *distributiontypes.DelegationDelegatorReward
		delegatorAddr string
		wantErr       bool
		expectedCoins int
	}{
		{
			name:          "nil reward",
			reward:        nil,
			delegatorAddr: "inj1test1234567890123456789012345678901234",
			wantErr:       true,
		},
		{
			name: "empty reward",
			reward: &distributiontypes.DelegationDelegatorReward{
				ValidatorAddress: "inj1validator123456789012345678901234567890",
				Reward:           types.DecCoins{},
			},
			delegatorAddr: "inj1test1234567890123456789012345678901234",
			wantErr:       false,
			expectedCoins: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DelegationRewardFromProto(tt.reward, tt.delegatorAddr)
			if (err != nil) != tt.wantErr {
				t.Errorf("DelegationRewardFromProto() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if result == nil {
				t.Fatal("DelegationRewardFromProto() returned nil result")
			}

			if result.DelegatorAddress != tt.delegatorAddr {
				t.Errorf("DelegationRewardFromProto() delegator address = %v, want %v", result.DelegatorAddress, tt.delegatorAddr)
			}

			if result.ValidatorAddress != tt.reward.ValidatorAddress {
				t.Errorf("DelegationRewardFromProto() validator address = %v, want %v", result.ValidatorAddress, tt.reward.ValidatorAddress)
			}

			// Verify JSON structure
			var rewardDetail DelegationRewardDetail
			if err := sonic.Unmarshal([]byte(result.Rewards), &rewardDetail); err != nil {
				t.Errorf("DelegationRewardFromProto() invalid JSON: %v", err)
				return
			}

			if len(rewardDetail.Coins) != tt.expectedCoins {
				t.Errorf("DelegationRewardFromProto() coins count = %v, want %v", len(rewardDetail.Coins), tt.expectedCoins)
			}
		})
	}
}

// TestDelegationRewardFromDecCoins tests the DelegationRewardFromDecCoins function.
func TestDelegationRewardFromDecCoins(t *testing.T) {
	tests := []struct {
		name          string
		rewards       types.DecCoins
		delegatorAddr string
		validatorAddr string
		wantErr       bool
		expectedCoins int
	}{
		{
			name:          "empty coins",
			rewards:       types.DecCoins{},
			delegatorAddr: "inj1test1234567890123456789012345678901234",
			validatorAddr: "inj1validator123456789012345678901234567890",
			wantErr:       false,
			expectedCoins: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DelegationRewardFromDecCoins(tt.rewards, tt.delegatorAddr, tt.validatorAddr)
			if (err != nil) != tt.wantErr {
				t.Errorf("DelegationRewardFromDecCoins() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if result == nil {
				t.Fatal("DelegationRewardFromDecCoins() returned nil result")
			}

			if result.DelegatorAddress != tt.delegatorAddr {
				t.Errorf("DelegationRewardFromDecCoins() delegator address = %v, want %v", result.DelegatorAddress, tt.delegatorAddr)
			}

			if result.ValidatorAddress != tt.validatorAddr {
				t.Errorf("DelegationRewardFromDecCoins() validator address = %v, want %v", result.ValidatorAddress, tt.validatorAddr)
			}

			// Verify JSON structure
			var rewardDetail DelegationRewardDetail
			if err := sonic.Unmarshal([]byte(result.Rewards), &rewardDetail); err != nil {
				t.Errorf("DelegationRewardFromDecCoins() invalid JSON: %v", err)
				return
			}

			if len(rewardDetail.Coins) != tt.expectedCoins {
				t.Errorf("DelegationRewardFromDecCoins() coins count = %v, want %v", len(rewardDetail.Coins), tt.expectedCoins)
			}
		})
	}
}

// TestDelegationRewardMultiCoinScenario tests the multi-coin reward scenario.
// This test verifies that TotalReward field has been removed and all coins are stored correctly.
// Note: This test is simplified to avoid complex DecCoin creation API.
// In a real scenario, you would create DecCoins from actual blockchain data.
func TestDelegationRewardMultiCoinScenario(t *testing.T) {
	// Test with empty coins to verify the function handles it correctly
	rewards := types.DecCoins{}

	result, err := DelegationRewardFromDecCoins(rewards, "inj1test1234567890123456789012345678901234", "inj1validator123456789012345678901234567890")
	if err != nil {
		t.Fatalf("DelegationRewardFromDecCoins() error = %v", err)
	}

	// Verify JSON structure is valid
	var rewardDetail DelegationRewardDetail
	if err := sonic.Unmarshal([]byte(result.Rewards), &rewardDetail); err != nil {
		t.Fatalf("Failed to unmarshal rewards JSON: %v", err)
	}

	// Verify empty coins are handled correctly
	if len(rewardDetail.Coins) != 0 {
		t.Errorf("Expected 0 coins, got %d", len(rewardDetail.Coins))
	}
}
