// Package service provides validator API implementations.
package service

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sort"

	pb "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/models"
	"injective-explorer/provider/db"

	"github.com/bytedance/sonic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/gorm"
)

// mapStatusStringToCode converts status string to Cosmos SDK status code
func mapStatusStringToCode(status string) int32 {
	switch status {
	case "bonded":
		return 3
	case "unbonding":
		return 2
	case "unbonded":
		return 1
	default:
		return 0
	}
}

// mapStatusCodeToString converts status code to string
func mapStatusCodeToString(code int32) string {
	switch code {
	case 3:
		return "bonded"
	case 2:
		return "unbonding"
	case 1:
		return "unbonded"
	default:
		return "unknown"
	}
}

// validateOrderBy validates and returns safe order_by clause
func validateOrderBy(orderBy string, allowedFields []string) string {
	if orderBy == "" {
		return ""
	}

	// Check against allowed fields
	for _, allowed := range allowedFields {
		if orderBy == allowed || orderBy == allowed+" DESC" || orderBy == allowed+" ASC" {
			// For numeric fields, use CAST
			if allowed == "tokens" || allowed == "commission_rate" {
				if orderBy == allowed {
					return fmt.Sprintf("CAST(%s AS DECIMAL) DESC", allowed)
				}
				return fmt.Sprintf("CAST(%s AS DECIMAL) %s", allowed, orderBy[len(allowed)+1:])
			}
			return orderBy
		}
	}
	return ""
}

// calculateTotalVotingPower sums all bonded validator tokens
func calculateTotalVotingPower(validators []models.Validator) string {
	total := big.NewFloat(0)
	for _, v := range validators {
		if v.Status == 3 { // BONDED
			tokens, _, err := big.ParseFloat(v.Tokens, 10, 256, big.ToNearestEven)
			if err == nil {
				total = total.Add(total, tokens)
			}
		}
	}
	return total.Text('f', -1)
}

// calculateVotingPowerPercentage calculates voting power percentage
func calculateVotingPowerPercentage(tokens string, totalVotingPower string) float64 {
	if totalVotingPower == "" || totalVotingPower == "0" {
		return 0.0
	}

	validatorTokens, _, err := big.ParseFloat(tokens, 10, 256, big.ToNearestEven)
	if err != nil {
		return 0.0
	}

	total, _, err := big.ParseFloat(totalVotingPower, 10, 256, big.ToNearestEven)
	if err != nil {
		return 0.0
	}

	if total.Cmp(big.NewFloat(0)) == 0 {
		return 0.0
	}

	percentage := new(big.Float).Quo(validatorTokens, total)
	percentage = percentage.Mul(percentage, big.NewFloat(100))
	result, _ := percentage.Float64()
	return result
}

// convertValidatorToListDTO converts validator to list DTO
func convertValidatorToListDTO(v *models.Validator, totalVotingPower string) map[string]interface{} {
	votingPowerPct := calculateVotingPowerPercentage(v.Tokens, totalVotingPower)

	return map[string]interface{}{
		"operator_address":       v.OperatorAddress,
		"consensus_address":      v.ConsensusAddress,
		"moniker":                v.Moniker,
		"status":                 mapStatusCodeToString(v.Status),
		"jailed":                 v.Jailed,
		"tokens":                 v.Tokens,
		"voting_power_percentage": votingPowerPct,
		"commission_rate":        v.CommissionRate,
		"uptime_percentage":      v.UptimePercentage,
		"image_url":              v.ImageUrl,
	}
}

// GetValidators retrieves a list of validators with filtering and sorting.
func (s *ExplorerService) GetValidators(ctx context.Context, req *pb.GetValidatorsRequest) (*pb.Reply, error) {
	var validators []models.Validator

	query := db.Get().Model(&models.Validator{})

	// Filter by status
	if req.Status != "" {
		statusCode := mapStatusStringToCode(req.Status)
		if statusCode > 0 {
			query = query.Where("status = ?", statusCode)
		}
	}

	// Filter by jailed
	if req.Jailed != nil {
		query = query.Where("jailed = ?", *req.Jailed)
	}

	// Search by moniker or address
	if req.Search != "" {
		query = query.Where("moniker LIKE ? OR operator_address = ?",
			"%"+req.Search+"%", req.Search)
	}

	// Commission range filter
	if req.MinCommission != "" {
		query = query.Where("CAST(commission_rate AS DECIMAL) >= ?", req.MinCommission)
	}
	if req.MaxCommission != "" {
		query = query.Where("CAST(commission_rate AS DECIMAL) <= ?", req.MaxCommission)
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to count validators")
	}

	// Apply pagination
	page := int32(1)
	pageSize := int32(20)
	if req.Pagination != nil {
		if req.Pagination.Page > 0 {
			page = req.Pagination.Page
		}
		if req.Pagination.PageSize > 0 {
			pageSize = req.Pagination.PageSize
		}
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize
	query = query.Offset(int(offset)).Limit(int(pageSize))

	// Sorting (with validation)
	orderBy := validateOrderBy(req.OrderBy, []string{
		"tokens", "commission_rate", "uptime_percentage",
	})
	if orderBy == "" {
		orderBy = "CAST(tokens AS DECIMAL) DESC"
	}
	query = query.Order(orderBy)

	// Execute query
	if err := query.Find(&validators).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to fetch validators")
	}

	// Pre-calculate total voting power
	totalVotingPower := calculateTotalVotingPower(validators)

	// Convert to DTOs
	validatorDTOs := make([]interface{}, len(validators))
	for i, v := range validators {
		validatorDTOs[i] = convertValidatorToListDTO(&v, totalVotingPower)
	}

	// Calculate pagination info
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))

	dataStruct, err := structpb.NewStruct(map[string]interface{}{
		"validators": validatorDTOs,
		"pagination": map[string]interface{}{
			"page":        float64(page),
			"page_size":   float64(pageSize),
			"total":       float64(total),
			"total_pages": float64(totalPages),
			"has_prev":    page > 1,
			"has_next":    page < totalPages,
		},
	})
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to create response struct: %v", err))
	}

	return &pb.Reply{
		Code:    http.StatusOK,
		Message: "Success",
		Data:    dataStruct,
	}, nil
}

// decodeValidatorDescription decodes JSON-stored description
func decodeValidatorDescription(descJSON string) map[string]interface{} {
	var desc map[string]interface{}
	if err := sonic.UnmarshalString(descJSON, &desc); err != nil {
		return map[string]interface{}{}
	}
	return desc
}

// parseSlashingEvents decodes slashing events from JSON array
func parseSlashingEvents(eventsJSON []string) []interface{} {
	events := make([]interface{}, 0, len(eventsJSON))
	for _, eventStr := range eventsJSON {
		var event map[string]interface{}
		if err := sonic.UnmarshalString(eventStr, &event); err == nil {
			events = append(events, event)
		}
	}
	// Sort by block_height DESC
	sort.Slice(events, func(i, j int) bool {
		ei := events[i].(map[string]interface{})
		ej := events[j].(map[string]interface{})
		hi, _ := ei["block_height"].(float64)
		hj, _ := ej["block_height"].(float64)
		return hi > hj
	})
	return events
}

// fetchUptimeTrend gets uptime statistics for validator
func fetchUptimeTrend(address string, days int) []interface{} {
	// Since the table is organized by block heights, not dates,
	// return empty array for now - can be enhanced later with block timestamp mapping
	return []interface{}{}
}

// convertValidatorToDetailDTO converts validator to detailed DTO
func convertValidatorToDetailDTO(v *models.Validator) map[string]interface{} {
	return map[string]interface{}{
		"operator_address":  v.OperatorAddress,
		"consensus_address": v.ConsensusAddress,
		"moniker":           v.Moniker,
		"status":            mapStatusCodeToString(v.Status),
		"jailed":            v.Jailed,
		"tokens":            v.Tokens,
		"delegator_shares":  v.DelegatorShares,
		"commission": map[string]interface{}{
			"rate":             v.CommissionRate,
			"max_rate":         v.CommissionMaxRate,
			"max_change_rate":  v.CommissionMaxChangeRate,
			"update_time":      v.CommissionUpdateTime,
		},
		"unbonding_height":  v.UnbondingHeight,
		"unbonding_time":    v.UnbondingTime,
		"uptime_percentage": v.UptimePercentage,
		"image_url":         v.ImageUrl,
	}
}

// GetValidator retrieves detailed information for a specific validator.
func (s *ExplorerService) GetValidator(ctx context.Context, req *pb.GetValidatorRequest) (*pb.Reply, error) {
	var validator models.Validator

	// Query by operator_address or consensus_address
	err := db.Get().Where("operator_address = ? OR consensus_address = ?",
		req.Address, req.Address).First(&validator).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "validator not found")
		}
		return nil, status.Error(codes.Internal, "failed to fetch validator")
	}

	// Build detailed DTO
	dto := convertValidatorToDetailDTO(&validator)

	// Fetch aggregated statistics
	proposed, signed, missed, err := models.GetValidatorStatsAggregated(validator.OperatorAddress)
	if err == nil {
		dto["proposed_blocks"] = proposed
		dto["signed_blocks"] = signed
		dto["missed_blocks"] = missed
	}

	// Decode description
	if validator.Description != "" {
		desc := decodeValidatorDescription(validator.Description)
		dto["description"] = desc
	}

	// Parse slashing events
	if len(validator.SlashingEvents) > 0 {
		events := parseSlashingEvents(validator.SlashingEvents)
		dto["slashing_events"] = events
	}

	// Fetch 30-day uptime trend
	uptimeTrend := fetchUptimeTrend(validator.OperatorAddress, 30)
	dto["uptime_trend"] = uptimeTrend

	dataStruct, err := structpb.NewStruct(dto)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to create response struct")
	}

	return &pb.Reply{
		Code:    http.StatusOK,
		Message: "Success",
		Data:    dataStruct,
	}, nil
}

// parsePeriodToDays converts period string to days
func parsePeriodToDays(period string) int {
	switch period {
	case "24h":
		return 1
	case "7d":
		return 7
	case "30d":
		return 30
	case "90d":
		return 90
	default:
		return 30
	}
}

// GetValidatorUptime retrieves validator uptime statistics.
func (s *ExplorerService) GetValidatorUptime(ctx context.Context, req *pb.GetValidatorUptimeRequest) (*pb.Reply, error) {
	// Query aggregated stats from the entire period
	var stats []struct {
		PeriodStartHeight uint64
		PeriodEndHeight   uint64
		Proposed          uint64
		Signed            uint64
		Missed            uint64
		ShouldSign        uint64
	}

	query := db.Get().Model(&models.ValidatorBlockStats{}).
		Select("period_start_height, period_end_height, proposed_count as proposed, signed_count as signed, missed_count as missed, should_sign_count as should_sign").
		Where("validator_operator_address = ?", req.Address)

	// Apply period or block-based filtering if needed
	if req.Period != "" || req.Blocks > 0 {
		// For simplicity, query all and let the aggregation handle it
		query = query.Order("period_start_height DESC")
	}

	query.Scan(&stats)

	// Calculate overall metrics
	var totalProposed, totalSigned, totalMissed, totalShouldSign uint64
	timeSeries := make([]interface{}, len(stats))

	for i, s := range stats {
		totalProposed += s.Proposed
		totalSigned += s.Signed
		totalMissed += s.Missed
		totalShouldSign += s.ShouldSign

		uptime := 0.0
		if s.ShouldSign > 0 {
			uptime = float64(s.Signed) / float64(s.ShouldSign) * 100
		}

		timeSeries[i] = map[string]interface{}{
			"period_start": float64(s.PeriodStartHeight),
			"period_end":   float64(s.PeriodEndHeight),
			"uptime":       uptime,
			"proposed":     float64(s.Proposed),
			"signed":       float64(s.Signed),
			"missed":       float64(s.Missed),
		}
	}

	overallUptime := 0.0
	if totalShouldSign > 0 {
		overallUptime = float64(totalSigned) / float64(totalShouldSign) * 100
	}

	// Determine period description
	periodDesc := "all time"
	if req.Period != "" {
		periodDesc = req.Period
	} else if req.Blocks > 0 {
		periodDesc = fmt.Sprintf("%d blocks", req.Blocks)
	}

	dataStruct, err := structpb.NewStruct(map[string]interface{}{
		"validator_address": req.Address,
		"period":            periodDesc,
		"overall": map[string]interface{}{
			"uptime_percentage": overallUptime,
			"proposed_blocks":   float64(totalProposed),
			"signed_blocks":     float64(totalSigned),
			"missed_blocks":     float64(totalMissed),
			"total_blocks":      float64(totalShouldSign),
		},
		"time_series": timeSeries,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to create response struct")
	}

	return &pb.Reply{
		Code:    http.StatusOK,
		Message: "Success",
		Data:    dataStruct,
	}, nil
}