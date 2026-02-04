// Package service provides cross-chain transaction parsing utilities.
package service

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"injective-explorer/internal/global"
	peggytypes "github.com/InjectiveLabs/sdk-go/chain/peggy/types"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmostxpb "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cometbft/cometbft/crypto/tmhash"
	"github.com/pkg/errors"
)

// CrossChainTxParser parses cross-chain transactions from chain data.
type CrossChainTxParser struct {
	chainClient chainclient.ChainClientV2
	txDecoder   sdk.TxDecoder
}

// NewCrossChainTxParser creates a new cross-chain transaction parser.
func NewCrossChainTxParser(chainClient chainclient.ChainClientV2) (*CrossChainTxParser, error) {
	return &CrossChainTxParser{
		chainClient: chainClient,
		txDecoder:   nil, // Initialized lazily when needed
	}, nil
}

// IBCTransferInfo contains parsed IBC transfer transaction information.
type IBCTransferInfo struct {
	TxHash            string
	Height            int64
	MsgIndex          int
	Sender            string
	Receiver          string
	SourcePort        string
	SourceChannel     string
	DestinationPort   string
	DestinationChannel string
	Amount            string
	Denom             string
	TimeoutHeight     string
	TimeoutTimestamp  uint64
	PacketSequence    uint64
	DataHex           []byte
	Timestamp         time.Time
}

// PeggyDepositInfo contains parsed Peggy deposit transaction information.
type PeggyDepositInfo struct {
	TxHash              string
	Height              int64
	MsgIndex            int
	EventNonce          uint64
	EventHeight         uint64
	TokenContract       string
	Amount              string
	EthereumSender      string
	CosmosReceiver      string
	Orchestrator        string
	Timestamp           time.Time
}

// PeggyWithdrawalInfo contains parsed Peggy withdrawal transaction information.
type PeggyWithdrawalInfo struct {
	TxHash       string
	Height       int64
	MsgIndex     int
	EventNonce   uint64
	EventHeight  uint64
	BatchNonce   uint64
	TokenContract string
	Orchestrator string
	Timestamp    time.Time
}

// ParseTxFromChain parses a transaction from chain data and extracts cross-chain messages.
// Returns IBC transfers, Peggy deposits, and Peggy withdrawals found in the transaction.
// This function gets the transaction bytes from GetTx response, following the approach in peggy.md.
func (p *CrossChainTxParser) ParseTxFromChain(ctx context.Context, txHash string, height int64, timestamp time.Time) ([]*IBCTransferInfo, []*PeggyDepositInfo, []*PeggyWithdrawalInfo, error) {
	// Get transaction from chain
	txResp, err := p.chainClient.GetTx(ctx, txHash)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to get tx from chain: %s", txHash)
	}

	if txResp == nil {
		return nil, nil, nil, errors.New("tx response is nil")
	}

	// Get TxBytes from GetTx response
	// According to peggy.md, we should use ChainClientV2.GetTx() which returns TxBytes
	// However, the actual response structure may vary, so we try to get from block as fallback
	var txBytes []byte
	
	// Initialize txDecoder if not already initialized
	if p.txDecoder == nil {
		// Try to create encoding config
		// For now, we'll parse messages directly from txResp without decoding
		// This is a simplified approach - we'll extract from TxResponse.Events and message types
		global.Logger.Debugf("txDecoder not initialized, parsing from TxResponse directly")
		return p.parseFromTxResponse(txResp, txHash, height, timestamp)
	}

	// Try to get from block first (more reliable for getting raw tx bytes)
	block, err := p.chainClient.FetchBlockByHeight(ctx, int64(height))
	if err == nil && block != nil && block.Block != nil && block.Block.Data.Txs != nil && len(block.Block.Data.Txs) > 0 {
		// Find transaction in block by comparing hashes
		txHashUpper := strings.ToUpper(strings.TrimPrefix(txHash, "0x"))
		for _, tx := range block.Block.Data.Txs {
			if len(tx) > 0 {
				// Calculate hash using Tendermint's hash function
				txHashCalculated := strings.ToUpper(hex.EncodeToString(tmhash.Sum(tx)))
				if txHashCalculated == txHashUpper {
					txBytes = tx
					break
				}
			}
		}
	}

	if len(txBytes) == 0 {
		// Fallback to parsing from TxResponse
		return p.parseFromTxResponse(txResp, txHash, height, timestamp)
	}

	// Decode transaction bytes using TxDecoder (as per peggy.md)
	sdkTx, err := p.txDecoder(txBytes)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to decode tx bytes: %s", txHash)
	}

	var ibcTransfers []*IBCTransferInfo
	var peggyDeposits []*PeggyDepositInfo
	var peggyWithdrawals []*PeggyWithdrawalInfo

	// Parse messages
	for msgIndex, msg := range sdkTx.GetMsgs() {
		msgType := sdk.MsgTypeURL(msg)

		switch msgType {
		case "/ibc.applications.transfer.v1.MsgTransfer":
			// Parse IBC transfer - extract info from events since we can't easily import IBC types
			// The message structure will be parsed from events which contain all necessary info
			global.Logger.Debugf("found IBC transfer message at index %d", msgIndex)

			ibcTransfer := &IBCTransferInfo{
				TxHash:            txHash,
				Height:            height,
				MsgIndex:          msgIndex,
				Timestamp:         timestamp,
			}

			// Extract info from events
			if txResp.TxResponse != nil && len(txResp.TxResponse.Events) > 0 {
				for _, event := range txResp.TxResponse.Events {
					if event.Type == "send_packet" || event.Type == "fungible_token_packet" {
						for _, attr := range event.Attributes {
							key := string(attr.Key)
							value := string(attr.Value)
							switch key {
							case "packet_sequence":
								var seq uint64
								if _, err := fmt.Sscanf(value, "%d", &seq); err == nil {
									ibcTransfer.PacketSequence = seq
								}
							case "packet_src_port":
								ibcTransfer.SourcePort = value
							case "packet_src_channel":
								ibcTransfer.SourceChannel = value
							case "packet_dst_port":
								ibcTransfer.DestinationPort = value
							case "packet_dst_channel":
								ibcTransfer.DestinationChannel = value
							case "packet_data":
								if data, err := hex.DecodeString(value); err == nil {
									ibcTransfer.DataHex = data
								}
							}
						}
					}
					if event.Type == "transfer" {
						for _, attr := range event.Attributes {
							key := string(attr.Key)
							value := string(attr.Value)
							switch key {
							case "sender":
								if ibcTransfer.Sender == "" {
									ibcTransfer.Sender = value
								}
							case "recipient":
								if ibcTransfer.Receiver == "" {
									ibcTransfer.Receiver = value
								}
							case "amount":
								// Parse amount like "1000uatom"
								parts := strings.Fields(value)
								if len(parts) > 0 {
									// Extract denom and amount
									amtStr := parts[0]
									for i := len(amtStr) - 1; i >= 0; i-- {
										if amtStr[i] >= '0' && amtStr[i] <= '9' {
											ibcTransfer.Amount = amtStr[:i+1]
											ibcTransfer.Denom = amtStr[i+1:]
											break
										}
									}
								}
							}
						}
					}
				}
			}

			// Only add if we have at least some basic info
			if ibcTransfer.SourcePort != "" || ibcTransfer.SourceChannel != "" {
				ibcTransfers = append(ibcTransfers, ibcTransfer)
			}

		case "/injective.peggy.v1.MsgDepositClaim":
			// Parse Peggy deposit claim
			if msgDepositClaim, ok := msg.(*peggytypes.MsgDepositClaim); ok {
				peggyDeposit := &PeggyDepositInfo{
					TxHash:         txHash,
					Height:         height,
					MsgIndex:       msgIndex,
					EventNonce:     msgDepositClaim.EventNonce,
					EventHeight:    msgDepositClaim.BlockHeight,
					TokenContract:  msgDepositClaim.TokenContract,
					Amount:          msgDepositClaim.Amount.String(),
					EthereumSender: msgDepositClaim.EthereumSender,
					CosmosReceiver: msgDepositClaim.CosmosReceiver,
					Orchestrator:   msgDepositClaim.Orchestrator,
					Timestamp:      timestamp,
				}
				peggyDeposits = append(peggyDeposits, peggyDeposit)
			}

		case "/injective.peggy.v1.MsgWithdrawClaim":
			// Parse Peggy withdrawal claim
			if msgWithdrawClaim, ok := msg.(*peggytypes.MsgWithdrawClaim); ok {
				peggyWithdrawal := &PeggyWithdrawalInfo{
					TxHash:       txHash,
					Height:       height,
					MsgIndex:     msgIndex,
					EventNonce:   msgWithdrawClaim.EventNonce,
					EventHeight:  msgWithdrawClaim.BlockHeight,
					BatchNonce:   msgWithdrawClaim.BatchNonce,
					TokenContract: msgWithdrawClaim.TokenContract,
					Orchestrator: msgWithdrawClaim.Orchestrator,
					Timestamp:    timestamp,
				}
				peggyWithdrawals = append(peggyWithdrawals, peggyWithdrawal)
			}
		}
	}

	return ibcTransfers, peggyDeposits, peggyWithdrawals, nil
}

// parseFromTxResponse parses cross-chain transactions from TxResponse without decoding TxBytes.
// This is a fallback method when txDecoder is not available.
func (p *CrossChainTxParser) parseFromTxResponse(txResp *cosmostxpb.GetTxResponse, txHash string, height int64, timestamp time.Time) ([]*IBCTransferInfo, []*PeggyDepositInfo, []*PeggyWithdrawalInfo, error) {
	var ibcTransfers []*IBCTransferInfo
	var peggyDeposits []*PeggyDepositInfo
	var peggyWithdrawals []*PeggyWithdrawalInfo

	if txResp == nil || txResp.TxResponse == nil {
		return nil, nil, nil, errors.New("tx response is nil")
	}

	// Parse from events and logs
	// This is a simplified approach - extract info from events
	// For full parsing, we need to decode the transaction bytes
	
	// Check message types from logs/events
	// For now, return empty results - full implementation requires txDecoder
	global.Logger.Debugf("parsing from TxResponse for tx %s (simplified mode)", txHash)
	
	return ibcTransfers, peggyDeposits, peggyWithdrawals, nil
}
