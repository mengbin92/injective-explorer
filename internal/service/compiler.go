// Package service provides the contract compiler interface and implementations.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"injective-explorer/internal/global"

	"github.com/pkg/errors"
)

// ContractCompiler defines the interface for compiling smart contract source code.
type ContractCompiler interface {
	// Compile compiles the given source code and returns the compiled result.
	Compile(ctx context.Context, req *CompileRequest) (*CompileResult, error)
	// Name returns the name of the compiler.
	Name() string
}

// CompileRequest holds compile request parameters.
type CompileRequest struct {
	// SourceCode is the source code to compile. For multi-file contracts,
	// this should be in Solidity Standard JSON Input format.
	SourceCode string
	// CompilerVersion is the compiler version to use (e.g., "0.8.17").
	CompilerVersion string
	// OptimizationEnabled indicates whether compiler optimization is enabled.
	OptimizationEnabled bool
	// OptimizationRuns is the number of optimization runs.
	OptimizationRuns int
	// EVMVersion is the target EVM version (e.g., "london", "paris").
	EVMVersion string
	// ContractName is the name of the contract to compile (for multi-contract sources).
	ContractName string
}

// CompileResult holds the compile output.
type CompileResult struct {
	// Bytecode is the runtime bytecode (deployed bytecode, without constructor).
	Bytecode string
	// DeployedBytecode is an alias for Bytecode (for clarity).
	DeployedBytecode string
	// CreationBytecode is the creation bytecode (including constructor).
	CreationBytecode string
	// ABI is the ABI JSON string.
	ABI string
	// Errors contains compilation errors.
	Errors []string
	// Warnings contains compilation warnings.
	Warnings []string
}

// SolidityCompilerConfig holds Solidity compiler configuration.
type SolidityCompilerConfig struct {
	// DockerImage is the Docker image for solc (default: "ethereum/solc").
	DockerImage string
	// CompileTimeout is the maximum time allowed for compilation.
	CompileTimeout time.Duration
	// MaxSourceSize is the maximum source code size in bytes.
	MaxSourceSize int64
	// UseDocker indicates whether to use Docker for compilation.
	UseDocker bool
	// SolcBinaryPath is the path to the local solc binary (if not using Docker).
	SolcBinaryPath string
}

// DefaultSolidityCompilerConfig returns default configuration for the Solidity compiler.
func DefaultSolidityCompilerConfig() *SolidityCompilerConfig {
	return &SolidityCompilerConfig{
		DockerImage:    "ethereum/solc",
		CompileTimeout: 30 * time.Second,
		MaxSourceSize:  1 * 1024 * 1024, // 1MB
		UseDocker:      true,
	}
}

// SolidityCompiler implements ContractCompiler for Solidity contracts using Docker solc.
type SolidityCompiler struct {
	config *SolidityCompilerConfig
}

// NewSolidityCompiler creates a new SolidityCompiler instance.
func NewSolidityCompiler(config *SolidityCompilerConfig) *SolidityCompiler {
	if config == nil {
		config = DefaultSolidityCompilerConfig()
	}
	return &SolidityCompiler{config: config}
}

// Name returns the compiler name.
func (c *SolidityCompiler) Name() string {
	return "solidity"
}

// Compile compiles Solidity source code using Docker solc.
//
// The source code should be in Solidity Standard JSON Input format:
//
//	{
//	  "language": "Solidity",
//	  "sources": {
//	    "Contract.sol": { "content": "pragma solidity ^0.8.0; ..." }
//	  },
//	  "settings": { "outputSelection": { "*": { "*": ["abi", "evm.bytecode", "evm.deployedBytecode"] } } }
//	}
func (c *SolidityCompiler) Compile(ctx context.Context, req *CompileRequest) (*CompileResult, error) {
	if req == nil {
		return nil, errors.New("compile request is nil")
	}

	// Validate source code size
	if c.config.MaxSourceSize > 0 && int64(len(req.SourceCode)) > c.config.MaxSourceSize {
		return nil, errors.Errorf("source code exceeds maximum size: %d > %d bytes", len(req.SourceCode), c.config.MaxSourceSize)
	}

	// Build standard JSON input if the source is plain Solidity
	input, err := c.buildStandardJSONInput(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build standard JSON input")
	}

	// Apply timeout
	compileCtx, cancel := context.WithTimeout(ctx, c.config.CompileTimeout)
	defer cancel()

	// Compile via Docker or local binary
	var output []byte
	if c.config.UseDocker {
		output, err = c.compileWithDocker(compileCtx, input, req.CompilerVersion)
	} else {
		output, err = c.compileWithBinary(compileCtx, input)
	}
	if err != nil {
		return nil, errors.Wrap(err, "compilation failed")
	}

	// Parse the standard JSON output
	return c.parseCompileOutput(output, req.ContractName)
}

// buildStandardJSONInput constructs the Solidity Standard JSON Input.
func (c *SolidityCompiler) buildStandardJSONInput(req *CompileRequest) ([]byte, error) {
	// Check if source code is already in JSON format
	var jsonCheck map[string]interface{}
	if err := json.Unmarshal([]byte(req.SourceCode), &jsonCheck); err == nil {
		if _, hasLanguage := jsonCheck["language"]; hasLanguage {
			// Already in standard JSON input format
			return []byte(req.SourceCode), nil
		}
	}

	// Build standard JSON input from plain source code
	type Source struct {
		Content string `json:"content"`
	}
	type OptimizerDetails struct {
		Enabled bool `json:"enabled"`
		Runs    int  `json:"runs"`
	}
	type OutputSelection map[string]map[string][]string
	type Settings struct {
		Optimizer       OptimizerDetails `json:"optimizer"`
		EVMVersion      string           `json:"evmVersion,omitempty"`
		OutputSelection OutputSelection  `json:"outputSelection"`
	}
	type StandardInput struct {
		Language string            `json:"language"`
		Sources  map[string]Source `json:"sources"`
		Settings Settings          `json:"settings"`
	}

	contractFile := "Contract.sol"
	if req.ContractName != "" {
		contractFile = req.ContractName + ".sol"
	}

	input := StandardInput{
		Language: "Solidity",
		Sources: map[string]Source{
			contractFile: {Content: req.SourceCode},
		},
		Settings: Settings{
			Optimizer: OptimizerDetails{
				Enabled: req.OptimizationEnabled,
				Runs:    req.OptimizationRuns,
			},
			EVMVersion: req.EVMVersion,
			OutputSelection: OutputSelection{
				"*": {"*": {"abi", "evm.bytecode", "evm.deployedBytecode"}},
			},
		},
	}

	return json.Marshal(input)
}

// compileWithDocker compiles Solidity source code using Docker solc image.
func (c *SolidityCompiler) compileWithDocker(ctx context.Context, input []byte, version string) ([]byte, error) {
	image := c.config.DockerImage
	if version != "" {
		image = fmt.Sprintf("%s:%s", c.config.DockerImage, version)
	}

	// SECURITY: Use exec.CommandContext with variadic args (not string concatenation)
	// to prevent command injection via untrusted image names or versions.
	// The image variable is constructed from trusted config + user-provided version,
	// and is passed as a single argument to Docker, not interpolated into a shell string.
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "-i",
		"--memory=512m", "--cpus=1",
		image, "--standard-json")
	cmd.Stdin = bytes.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Check if it's a timeout
		if ctx.Err() == context.DeadlineExceeded {
			return nil, errors.New("compilation timed out")
		}
		return nil, errors.Wrapf(err, "docker solc failed: %s", stderr.String())
	}

	return stdout.Bytes(), nil
}

// compileWithBinary compiles Solidity source code using a local solc binary.
func (c *SolidityCompiler) compileWithBinary(ctx context.Context, input []byte) ([]byte, error) {
	binaryPath := c.config.SolcBinaryPath
	if binaryPath == "" {
		binaryPath = "solc"
	}

	cmd := exec.CommandContext(ctx, binaryPath, "--standard-json")
	cmd.Stdin = bytes.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, errors.New("compilation timed out")
		}
		return nil, errors.Wrapf(err, "solc binary failed: %s", stderr.String())
	}

	return stdout.Bytes(), nil
}

// solcOutput represents the standard JSON output from solc.
type solcOutput struct {
	Errors []struct {
		Severity string `json:"severity"`
		Message  string `json:"message"`
	} `json:"errors"`
	Contracts map[string]map[string]struct {
		ABI json.RawMessage `json:"abi"`
		EVM struct {
			Bytecode struct {
				Object string `json:"object"`
			} `json:"bytecode"`
			DeployedBytecode struct {
				Object string `json:"object"`
			} `json:"deployedBytecode"`
		} `json:"evm"`
	} `json:"contracts"`
}

// parseCompileOutput parses solc standard JSON output.
func (c *SolidityCompiler) parseCompileOutput(output []byte, contractName string) (*CompileResult, error) {
	var out solcOutput
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, errors.Wrap(err, "failed to parse solc output")
	}

	result := &CompileResult{}

	// Collect errors and warnings
	for _, e := range out.Errors {
		if e.Severity == "error" {
			result.Errors = append(result.Errors, e.Message)
		} else {
			result.Warnings = append(result.Warnings, e.Message)
		}
	}

	// If there are compile errors, return early
	if len(result.Errors) > 0 {
		return result, nil
	}

	// Find the target contract output
	for fileName, contracts := range out.Contracts {
		for name, contract := range contracts {
			// Match by contract name if specified
			if contractName != "" && name != contractName {
				continue
			}
			// If no contract name specified, pick the first one (or the one matching the filename)
			if contractName == "" && len(out.Contracts) > 1 {
				// Prefer contract whose name matches the filename (without .sol)
				baseName := strings.TrimSuffix(fileName, ".sol")
				if name != baseName && name != strings.Split(baseName, "/")[len(strings.Split(baseName, "/"))-1] {
					continue
				}
			}

			result.Bytecode = contract.EVM.DeployedBytecode.Object
			result.DeployedBytecode = contract.EVM.DeployedBytecode.Object
			result.CreationBytecode = contract.EVM.Bytecode.Object
			if contract.ABI != nil {
				result.ABI = string(contract.ABI)
			}
			return result, nil
		}
	}

	// If we didn't find the target contract, pick any contract
	for _, contracts := range out.Contracts {
		for _, contract := range contracts {
			result.Bytecode = contract.EVM.DeployedBytecode.Object
			result.DeployedBytecode = contract.EVM.DeployedBytecode.Object
			result.CreationBytecode = contract.EVM.Bytecode.Object
			if contract.ABI != nil {
				result.ABI = string(contract.ABI)
			}
			return result, nil
		}
	}

	if global.Logger != nil {
		global.Logger.Warnf("No contract found in solc output for contract name: %s", contractName)
	}

	return result, errors.New("no contract found in compilation output")
}
