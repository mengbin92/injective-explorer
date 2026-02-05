package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"
	"injective-explorer/provider/logger"

	"github.com/bytedance/sonic"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/env"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"go.uber.org/zap"
)

var (
	flagconf = flag.String("conf", "../../configs", "config path, eg: -conf config.yaml")
	limit    = flag.Int("limit", 0, "limit number of transactions to process (0 = all)")
	dryRun   = flag.Bool("dry-run", false, "dry run mode, don't save to database")
)

func main() {
	flag.Parse()

	// Load configuration
	c := config.New(
		config.WithSource(
			file.NewSource(*flagconf),
			env.NewSource(""),
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to scan configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	kratosLogger := logger.NewZapLogger(bc.Log)
	kratosLogger = kratosLogger.With(
		zap.String("tool", "resync-crosschain"),
	)
	logHelper := log.NewHelper(kratosLogger)

	// Initialize global variables (database, storage, etc.)
	global.Init(&bc, kratosLogger)

	logHelper.Info("🔍 Scanning for cross-chain transactions in database...")
	logHelper.Info("=" + string(make([]byte, 60)) + "=")

	// Query transactions with cross-chain message types
	query := db.Get().Model(&models.Transaction{}).
		Where("messages LIKE ? OR messages LIKE ? OR messages LIKE ? OR messages LIKE ?",
			"%/ibc.applications.transfer.v1.MsgTransfer%",
			"%/injective.peggy.v1.MsgDepositClaim%",
			"%/injective.peggy.v1.MsgWithdrawClaim%",
			"%/injective.peggy.v1.MsgSendToEth%").
		Order("block_number DESC")

	if *limit > 0 {
		query = query.Limit(*limit)
	}

	var txs []models.Transaction
	if err := query.Find(&txs).Error; err != nil {
		logHelper.Fatalf("failed to query transactions: %v", err)
	}

	logHelper.Infof("📊 Found %d transactions to process", len(txs))

	if len(txs) == 0 {
		logHelper.Info("✓ No cross-chain transactions found")
		return
	}

	stats := struct {
		Total           int
		IBCTransfer     int
		PeggyDeposit    int
		PeggyWithdrawal int
		SendToEth       int
		Errors          int
	}{
		Total: len(txs),
	}

	for i, tx := range txs {
		logHelper.Infof("[%d/%d] Processing tx %s (block %d)...", i+1, len(txs), tx.Hash, tx.BlockNumber)

		if err := processCrossChainTx(&tx, *dryRun, &stats, logHelper); err != nil {
			logHelper.Warnf("failed to process tx %s: %v", tx.Hash, err)
			stats.Errors++
		}
	}

	// Print summary
	logHelper.Info("")
	logHelper.Info("=" + string(make([]byte, 60)) + "=")
	logHelper.Info("📈 Summary:")
	logHelper.Infof("  Total transactions:    %d", stats.Total)
	logHelper.Infof("  IBC Transfers:         %d", stats.IBCTransfer)
	logHelper.Infof("  Peggy Deposits:        %d", stats.PeggyDeposit)
	logHelper.Infof("  Peggy Withdrawals:     %d", stats.PeggyWithdrawal)
	logHelper.Infof("  Peggy Send to Eth:     %d", stats.SendToEth)
	logHelper.Infof("  Errors:                %d", stats.Errors)
	logHelper.Info("=" + string(make([]byte, 60)) + "=")

	if *dryRun {
		logHelper.Info("")
		logHelper.Info("⚠️  DRY RUN MODE - No data was saved to database")
		logHelper.Info("Run without --dry-run to save the data")
	} else {
		logHelper.Info("")
		logHelper.Info("✅ Done! Cross-chain data saved to database")
	}
}

func processCrossChainTx(tx *models.Transaction, dryRun bool, stats *struct {
	Total           int
	IBCTransfer     int
	PeggyDeposit    int
	PeggyWithdrawal int
	SendToEth       int
	Errors          int
}, logHelper *log.Helper) error {
	// Parse transaction messages
	var txData models.TxJSON
	if err := sonic.UnmarshalString(tx.Messages, &txData); err != nil {
		return fmt.Errorf("failed to unmarshal messages: %w", err)
	}

	// Parse logs
	logsBytes := []byte(tx.Logs)
	if len(logsBytes) > 0 {
		var logs []models.LogRaw
		if err := sonic.Unmarshal(logsBytes, &logs); err == nil {
			txData.Logs = logs
		}
	}

	ctx := context.Background()

	// Process each message
	for msgIndex, msg := range txData.Messages {
		logHelper.Infof("  - Message type: %s", msg.Type)

		switch msg.Type {
		case "/ibc.applications.transfer.v1.MsgTransfer":
			if err := saveIBCTransfer(ctx, msg, tx, msgIndex, txData.Logs, dryRun, logHelper); err != nil {
				return fmt.Errorf("failed to save IBC transfer: %w", err)
			}
			stats.IBCTransfer++
			logHelper.Info("    ✓ Saved IBC transfer")

		case "/injective.peggy.v1.MsgDepositClaim":
			if err := savePeggyDeposit(ctx, msg, tx, msgIndex, txData.Logs, dryRun, logHelper); err != nil {
				return fmt.Errorf("failed to save Peggy deposit: %w", err)
			}
			stats.PeggyDeposit++
			logHelper.Info("    ✓ Saved Peggy deposit")

		case "/injective.peggy.v1.MsgWithdrawClaim":
			if err := savePeggyWithdrawal(ctx, msg, tx, msgIndex, txData.Logs, dryRun, logHelper); err != nil {
				return fmt.Errorf("failed to save Peggy withdrawal: %w", err)
			}
			stats.PeggyWithdrawal++
			logHelper.Info("    ✓ Saved Peggy withdrawal")

		case "/injective.peggy.v1.MsgSendToEth":
			if err := savePeggySendToEth(ctx, msg, tx, msgIndex, txData.Logs, dryRun, logHelper); err != nil {
				return fmt.Errorf("failed to save Peggy send to eth: %w", err)
			}
			stats.SendToEth++
			logHelper.Info("    ✓ Saved Peggy send to eth")
		}
	}

	return nil
}

// Helper functions that mirror the sync service logic
func saveIBCTransfer(ctx context.Context, msg models.MessageRaw, tx *models.Transaction, msgIndex int, logs []models.LogRaw, dryRun bool, logHelper *log.Helper) error {
	var ibcMsg struct {
		SourcePort    string `json:"source_port"`
		SourceChannel string `json:"source_channel"`
		Token         struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"token"`
		Sender        string `json:"sender"`
		Receiver      string `json:"receiver"`
		TimeoutHeight struct {
			RevisionNumber string `json:"revision_number"`
			RevisionHeight string `json:"revision_height"`
		} `json:"timeout_height"`
		TimeoutTimestamp string `json:"timeout_timestamp"`
	}

	if err := sonic.Unmarshal(msg.Message, &ibcMsg); err != nil {
		return fmt.Errorf("failed to unmarshal IBC message: %w", err)
	}

	// Extract additional information from events
	var packetSequence uint64
	var destPort, destChannel string

	for _, log := range logs {
		if log.MsgIndex == fmt.Sprintf("%d", msgIndex) {
			for _, event := range log.Events {
				if event.Type == "send_packet" || event.Type == "fungible_token_packet" {
					for _, attr := range event.Attributes {
						key := attr.Key
						value := stringifyAttrValue(attr.Value)
						switch key {
						case "packet_sequence":
							fmt.Sscanf(value, "%d", &packetSequence)
						case "packet_dst_port":
							destPort = value
						case "packet_dst_channel":
							destChannel = value
						}
					}
				}
			}
		}
	}

	timeoutTimestamp, _ := parseUint64(ibcMsg.TimeoutTimestamp)
	timeoutHeight := ""
	if ibcMsg.TimeoutHeight.RevisionHeight != "" && ibcMsg.TimeoutHeight.RevisionHeight != "0" {
		timeoutHeight = fmt.Sprintf("%s-%s", ibcMsg.TimeoutHeight.RevisionNumber, ibcMsg.TimeoutHeight.RevisionHeight)
	}

	ibcTransfer := &models.IBCTransferTx{
		Sender:             ibcMsg.Sender,
		Receiver:           ibcMsg.Receiver,
		SourcePort:         ibcMsg.SourcePort,
		SourceChannel:      ibcMsg.SourceChannel,
		DestinationPort:    destPort,
		DestinationChannel: destChannel,
		Amount:             ibcMsg.Token.Amount,
		Denom:              ibcMsg.Token.Denom,
		TimeoutHeight:      timeoutHeight,
		TimeoutTimestamp:   timeoutTimestamp,
		PacketSequence:     packetSequence,
		State:              "completed",
		TxHash:             tx.Hash,
		BlockHeight:        tx.BlockNumber,
		BlockTimestamp:     tx.BlockUnixTimestamp,
	}

	if dryRun {
		logHelper.Infof("    [DRY RUN] Would save: %+v", ibcTransfer)
		return nil
	}

	return db.Get().WithContext(ctx).Create(ibcTransfer).Error
}

func savePeggyDeposit(ctx context.Context, msg models.MessageRaw, tx *models.Transaction, msgIndex int, logs []models.LogRaw, dryRun bool, logHelper *log.Helper) error {
	var depositMsg struct {
		EventNonce     string `json:"event_nonce"`
		BlockHeight    string `json:"block_height"`
		TokenContract  string `json:"token_contract"`
		Amount         string `json:"amount"`
		EthereumSender string `json:"ethereum_sender"`
		CosmosReceiver string `json:"cosmos_receiver"`
		Orchestrator   string `json:"orchestrator"`
	}

	if err := sonic.Unmarshal(msg.Message, &depositMsg); err != nil {
		return fmt.Errorf("failed to unmarshal deposit message: %w", err)
	}

	eventNonce, _ := parseUint64(depositMsg.EventNonce)
	eventHeight, _ := parseUint64(depositMsg.BlockHeight)

	peggyDeposit := &models.PeggyDepositTx{
		Sender:              depositMsg.EthereumSender,
		Receiver:            depositMsg.CosmosReceiver,
		Amount:              depositMsg.Amount,
		Denom:               depositMsg.TokenContract,
		EventNonce:          eventNonce,
		EventHeight:         eventHeight,
		OrchestratorAddress: depositMsg.Orchestrator,
		State:               "completed",
		ClaimType:           1,
		TxHash:              tx.Hash,
		BlockHeight:         tx.BlockNumber,
		BlockTimestamp:      tx.BlockUnixTimestamp,
	}

	if dryRun {
		logHelper.Infof("    [DRY RUN] Would save: %+v", peggyDeposit)
		return nil
	}

	return db.Get().WithContext(ctx).Create(peggyDeposit).Error
}

func savePeggyWithdrawal(ctx context.Context, msg models.MessageRaw, tx *models.Transaction, msgIndex int, logs []models.LogRaw, dryRun bool, logHelper *log.Helper) error {
	var withdrawMsg struct {
		EventNonce    string `json:"event_nonce"`
		BlockHeight   string `json:"block_height"`
		BatchNonce    string `json:"batch_nonce"`
		TokenContract string `json:"token_contract"`
		Orchestrator  string `json:"orchestrator"`
	}

	if err := sonic.Unmarshal(msg.Message, &withdrawMsg); err != nil {
		return fmt.Errorf("failed to unmarshal withdrawal message: %w", err)
	}

	eventNonce, _ := parseUint64(withdrawMsg.EventNonce)
	eventHeight, _ := parseUint64(withdrawMsg.BlockHeight)
	batchNonce, _ := parseUint64(withdrawMsg.BatchNonce)

	peggyWithdrawal := &models.PeggyWithdrawalTx{
		Denom:               withdrawMsg.TokenContract,
		BatchNonce:          batchNonce,
		OrchestratorAddress: withdrawMsg.Orchestrator,
		EventNonce:          eventNonce,
		EventHeight:         eventHeight,
		State:               "completed",
		ClaimType:           2,
		TxHash:              tx.Hash,
		BlockHeight:         tx.BlockNumber,
		BlockTimestamp:      tx.BlockUnixTimestamp,
	}

	if dryRun {
		logHelper.Infof("    [DRY RUN] Would save: %+v", peggyWithdrawal)
		return nil
	}

	return db.Get().WithContext(ctx).Create(peggyWithdrawal).Error
}

func savePeggySendToEth(ctx context.Context, msg models.MessageRaw, tx *models.Transaction, msgIndex int, logs []models.LogRaw, dryRun bool, logHelper *log.Helper) error {
	var sendToEthMsg struct {
		Sender    string `json:"sender"`
		EthDest   string `json:"eth_dest"`
		Amount    struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"amount"`
		BridgeFee struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"bridge_fee"`
	}

	if err := sonic.Unmarshal(msg.Message, &sendToEthMsg); err != nil {
		return fmt.Errorf("failed to unmarshal send to eth message: %w", err)
	}

	var outgoingTxId uint64
	for _, log := range logs {
		if log.MsgIndex == fmt.Sprintf("%d", msgIndex) {
			for _, event := range log.Events {
				for _, attr := range event.Attributes {
					if attr.Key == "outgoing_tx_id" {
						outgoingTxId, _ = parseUint64(stringifyAttrValue(attr.Value))
					}
				}
			}
		}
	}

	peggyWithdrawal := &models.PeggyWithdrawalTx{
		Sender:       sendToEthMsg.Sender,
		Receiver:     sendToEthMsg.EthDest,
		Amount:       sendToEthMsg.Amount.Amount,
		Denom:        sendToEthMsg.Amount.Denom,
		BridgeFee:    sendToEthMsg.BridgeFee.Amount,
		OutgoingTxId: outgoingTxId,
		State:        "pending",
		ClaimType:    2,
		TxHash:       tx.Hash,
		BlockHeight:  tx.BlockNumber,
		BlockTimestamp: tx.BlockUnixTimestamp,
	}

	if dryRun {
		logHelper.Infof("    [DRY RUN] Would save: %+v", peggyWithdrawal)
		return nil
	}

	return db.Get().WithContext(ctx).Create(peggyWithdrawal).Error
}

func parseUint64(s string) (uint64, error) {
	if s == "" {
		return 0, nil
	}
	var result uint64
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

func stringifyAttrValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case []interface{}:
		var parts []string
		for _, item := range val {
			if s := stringifyAttrValue(item); s != "" {
				parts = append(parts, s)
			}
		}
		return fmt.Sprintf("[%s]", fmt.Sprint(parts))
	default:
		return fmt.Sprintf("%v", val)
	}
}
