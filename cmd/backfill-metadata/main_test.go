// Package main provides unit tests for backfill-metadata CLI.
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCLIConfigFields verifies the CLIConfig struct has all expected fields.
func TestCLIConfigFields(t *testing.T) {
	cfg := &CLIConfig{
		ConfPath:    "/test/config",
		Limit:       100,
		ChainID:     "cosmoshub-4",
		DelayMs:     50,
		DryRun:      true,
		ShowVersion: false,
	}

	assert.Equal(t, "/test/config", cfg.ConfPath)
	assert.Equal(t, 100, cfg.Limit)
	assert.Equal(t, "cosmoshub-4", cfg.ChainID)
	assert.Equal(t, 50, cfg.DelayMs)
	assert.True(t, cfg.DryRun)
	assert.False(t, cfg.ShowVersion)
}

// TestCLIConfigDefaults verifies default values for CLIConfig.
func TestCLIConfigDefaults(t *testing.T) {
	cfg := &CLIConfig{}

	assert.Equal(t, "", cfg.ConfPath)
	assert.Equal(t, 0, cfg.Limit)
	assert.Equal(t, "", cfg.ChainID)
	assert.Equal(t, 0, cfg.DelayMs)
	assert.False(t, cfg.DryRun)
	assert.False(t, cfg.ShowVersion)
}

// TestCLIConfigShowVersion tests the ShowVersion flag behavior.
func TestCLIConfigShowVersion(t *testing.T) {
	cfg := &CLIConfig{ShowVersion: true}

	assert.True(t, cfg.ShowVersion)

	cfg.ShowVersion = false
	assert.False(t, cfg.ShowVersion)
}
