# Validator APIs - Phase 2 Design

**Date**: 2026-02-09
**Status**: Design Complete
**Goal**: Implement validator query APIs (GetValidators, GetValidator, GetValidatorUptime)

---

## Overview

Phase 2 adds three core validator APIs to support staking decisions and validator monitoring. These APIs leverage existing validator models and statistics tables.

**APIs to implement:**
1. `GetValidators` - List validators with filtering and sorting
2. `GetValidator` - Get detailed validator information
3. `GetValidatorUptime` - Get validator uptime statistics

---

## Architecture

### Data Sources

**Existing tables (no schema changes needed):**
- `validators` - Validator basic info and cached stats
- `validator_block_stats` - Daily aggregated statistics (proposed, signed, missed)
- `blocks` - Block metadata for timestamp lookups

**Key design decisions:**
- Reuse existing `models.GetValidatorStatsAggregated()` for performance
- Decode Description and SlashingEvents from JSON storage
- Pre-calculate total voting power for percentage calculations
- Support both block-based and time-based uptime queries

---

## API Definitions

### Proto Messages

**File: `api/explorer/v1/validator.proto` (new)**

```protobuf
syntax = "proto3";
package api.explorer.v1;
import "explorer/v1/common.proto";

option go_package = "injective-explorer/api/explorer/v1;v1";
option java_multiple_files = true;
option java_package = "api.explorer.v1";

// GetValidatorsRequest requests a list of validators
message GetValidatorsRequest {
  // Pagination parameters
  PaginationRequest pagination = 1;

  // Filter by status (optional)
  string status = 2;  // "bonded", "unbonding", "unbonded"

  // Filter by jailed status (optional)
  optional bool jailed = 3 [json_name = "jailed"];

  // Search by moniker or address (optional)
  string search = 4;

  // Commission rate range filter (optional)
  string min_commission = 5 [json_name = "min_commission"];
  string max_commission = 6 [json_name = "max_commission"];

  // Sort order (optional)
  string order_by = 7 [json_name = "order_by"];
}

// GetValidatorRequest requests single validator details
message GetValidatorRequest {
  // Validator address (operator_address or consensus_address)
  string address = 1;
}

// GetValidatorUptimeRequest requests validator uptime statistics
message GetValidatorUptimeRequest {
  // Validator address (required)
  string address = 1;

  // Query by blocks (optional)
  int64 blocks = 2;

  // Query by time period (optional): "24h", "7d", "30d", "90d"
  string period = 3;
}
```

### RPC Methods

**File: `api/explorer/v1/explorer.proto` (modify)**

```protobuf
import "explorer/v1/validator.proto";

service Explorer {
  // ... existing methods ...

  // Validator APIs

  // Get list of validators with filtering and sorting
  rpc GetValidators(GetValidatorsRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/validators"
    };
  }

  // Get detailed validator information
  rpc GetValidator(GetValidatorRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/validator/info"
    };
  }

  // Get validator uptime statistics
  rpc GetValidatorUptime(GetValidatorUptimeRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/validator/uptime"
    };
  }
}
```

---

## Service Implementation

### File Structure

```
internal/service/
├── validator_api.go          (new - Phase 2 APIs)
├── validator_api_test.go     (new - comprehensive tests)
├── validator.go              (existing - validation utilities)
├── validator_stats.go        (existing - background stats updater)
```

### 1. GetValidators Implementation

**File: `internal/service/validator_api.go`**

**Core logic:**

```go
func (s *ExplorerService) GetValidators(ctx context.Context, req *pb.GetValidatorsRequest) (*pb.Reply, error) {
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

    // Count total
    var total int64
    query.Count(&total)

    // Pagination
    page, pageSize := normalizePagination(req.Pagination)
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
    var validators []models.Validator
    if err := query.Find(&validators).Error; err != nil {
        return nil, status.Error(codes.Internal, "failed to fetch validators")
    }

    // Pre-calculate total voting power
    totalVotingPower := calculateTotalVotingPower(validators)

    // Convert to DTOs
    validatorDTOs := make([]map[string]interface{}, len(validators))
    for i, v := range validators {
        validatorDTOs[i] = convertValidatorToListDTO(&v, totalVotingPower)
    }

    return &pb.Reply{
        Code: http.StatusOK,
        Message: "Success",
        Data: map[string]interface{}{
            "validators": validatorDTOs,
            "pagination": buildPaginationInfo(page, pageSize, total),
        },
    }, nil
}
```

**Helper functions:**

```go
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

// calculateTotalVotingPower sums all validator tokens (bonded only)
func calculateTotalVotingPower(validators []models.Validator) string {
    var total sdk.Dec
    for _, v := range validators {
        if v.Status == 3 { // BONDED
            tokens, _ := sdk.NewDecFromStr(v.Tokens)
            total = total.Add(tokens)
        }
    }
    return total.String()
}

// convertValidatorToListDTO converts validator to list DTO
func convertValidatorToListDTO(v *models.Validator, totalVotingPower string) map[string]interface{} {
    votingPowerPct := calculateVotingPowerPercentage(v.Tokens, totalVotingPower)

    return map[string]interface{}{
        "operator_address": v.OperatorAddress,
        "consensus_address": v.ConsensusAddress,
        "moniker": v.Moniker,
        "status": mapStatusCodeToString(v.Status),
        "jailed": v.Jailed,
        "tokens": v.Tokens,
        "voting_power_percentage": votingPowerPct,
        "commission_rate": v.CommissionRate,
        "uptime_percentage": v.UptimePercentage,
        "image_url": v.ImageUrl,
    }
}
```

---

### 2. GetValidator Implementation

**Full validator details with decoded fields:**

```go
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

    return &pb.Reply{
        Code: http.StatusOK,
        Message: "Success",
        Data: dto,
    }, nil
}
```

**Helper functions:**

```go
// decodeValidatorDescription decodes JSON-stored description
func decodeValidatorDescription(descJSON string) map[string]interface{} {
    var desc map[string]interface{}
    if err := sonic.UnmarshalString(descJSON, &desc); err != nil {
        return map[string]interface{}{}
    }
    return desc
}

// parseSlashingEvents decodes slashing events from JSON array
func parseSlashingEvents(eventsJSON []string) []map[string]interface{} {
    events := make([]map[string]interface{}, 0, len(eventsJSON))
    for _, eventStr := range eventsJSON {
        var event map[string]interface{}
        if err := sonic.UnmarshalString(eventStr, &event); err == nil {
            events = append(events, event)
        }
    }
    // Sort by block_height DESC
    sort.Slice(events, func(i, j int) {
        hi, _ := events[i]["block_height"].(float64)
        hj, _ := events[j]["block_height"].(float64)
        return hi > hj
    })
    return events
}

// fetchUptimeTrend gets daily uptime for last N days
func fetchUptimeTrend(address string, days int) []map[string]interface{} {
    startDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

    var stats []struct {
        Date         string
        Proposed     int64
        Signed       int64
        Missed       int64
        TotalBlocks  int64
    }

    db.Get().Raw(`
        SELECT
            date,
            SUM(proposed) as proposed,
            SUM(signed) as signed,
            SUM(missed) as missed,
            SUM(total_blocks) as total_blocks
        FROM validator_block_stats
        WHERE validator_address = ? AND date >= ?
        GROUP BY date
        ORDER BY date DESC
    `, address, startDate).Scan(&stats)

    trend := make([]map[string]interface{}, len(stats))
    for i, s := range stats {
        uptime := 0.0
        if s.TotalBlocks > 0 {
            uptime = float64(s.Signed) / float64(s.TotalBlocks) * 100
        }
        trend[i] = map[string]interface{}{
            "date": s.Date,
            "uptime": uptime,
            "proposed": s.Proposed,
            "signed": s.Signed,
            "missed": s.Missed,
            "total": s.TotalBlocks,
        }
    }
    return trend
}
```

---

### 3. GetValidatorUptime Implementation

**Dual-mode uptime calculation:**

```go
func (s *ExplorerService) GetValidatorUptime(ctx context.Context, req *pb.GetValidatorUptimeRequest) (*pb.Reply, error) {
    // Determine query range
    var startDate string
    var periodDesc string

    if req.Period != "" {
        // Time-based query
        days := parsePeriodToDays(req.Period)
        startDate = time.Now().AddDate(0, 0, -days).Format("2006-01-02")
        periodDesc = req.Period
    } else {
        // Block-based query (convert to approximate date)
        blocks := req.Blocks
        if blocks <= 0 {
            blocks = 10000 // default
        }
        days := int(blocks * 5 / 86400) // 5s per block
        startDate = time.Now().AddDate(0, 0, -days).Format("2006-01-02")
        periodDesc = fmt.Sprintf("%d blocks", blocks)
    }

    // Query aggregated stats
    var stats []struct {
        Date        string
        Proposed    int64
        Signed      int64
        Missed      int64
        TotalBlocks int64
    }

    db.Get().Raw(`
        SELECT
            date,
            SUM(proposed) as proposed,
            SUM(signed) as signed,
            SUM(missed) as missed,
            SUM(total_blocks) as total_blocks
        FROM validator_block_stats
        WHERE validator_address = ? AND date >= ?
        GROUP BY date
        ORDER BY date DESC
    `, req.Address, startDate).Scan(&stats)

    // Calculate overall metrics
    var totalProposed, totalSigned, totalMissed, totalBlocks int64
    timeSeries := make([]map[string]interface{}, len(stats))

    for i, s := range stats {
        totalProposed += s.Proposed
        totalSigned += s.Signed
        totalMissed += s.Missed
        totalBlocks += s.TotalBlocks

        uptime := 0.0
        if s.TotalBlocks > 0 {
            uptime = float64(s.Signed) / float64(s.TotalBlocks) * 100
        }

        timeSeries[i] = map[string]interface{}{
            "date": s.Date,
            "uptime": uptime,
            "proposed": s.Proposed,
            "signed": s.Signed,
            "missed": s.Missed,
            "total": s.TotalBlocks,
        }
    }

    overallUptime := 0.0
    if totalBlocks > 0 {
        overallUptime = float64(totalSigned) / float64(totalBlocks) * 100
    }

    return &pb.Reply{
        Code: http.StatusOK,
        Message: "Success",
        Data: map[string]interface{}{
            "validator_address": req.Address,
            "period": periodDesc,
            "overall": map[string]interface{}{
                "uptime_percentage": overallUptime,
                "proposed_blocks": totalProposed,
                "signed_blocks": totalSigned,
                "missed_blocks": totalMissed,
                "total_blocks": totalBlocks,
            },
            "time_series": timeSeries,
        },
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
```

---

## Testing Strategy

### Unit Tests

**File: `internal/service/validator_api_test.go`**

**Test coverage (target: >90%):**

1. **TestGetValidators** (20+ test cases):
   - Pagination (default, custom, boundaries)
   - Status filter (bonded, unbonding, unbonded)
   - Jailed filter (true, false, nil)
   - Search (moniker exact, moniker partial, address)
   - Commission range (min only, max only, both, invalid)
   - Sorting (tokens DESC, commission ASC, uptime DESC)
   - Combined filters
   - Empty results
   - Voting power percentage calculation

2. **TestGetValidator** (15+ test cases):
   - Query by operator_address
   - Query by consensus_address
   - Validator not found
   - Description decoding (JSON, empty, invalid)
   - SlashingEvents parsing (empty, single, multiple, sorted)
   - Uptime trend (30 days, partial data, no data)
   - Stats aggregation integration

3. **TestGetValidatorUptime** (12+ test cases):
   - Block-based query (10000 blocks, custom blocks)
   - Time-based query (24h, 7d, 30d, 90d)
   - Default behavior (no params)
   - Empty stats (new validator)
   - Uptime calculation accuracy
   - Time series ordering
   - Edge cases (0 total blocks, 100% uptime, 0% uptime)

### Integration Testing

- Use in-memory SQLite for isolated tests
- Seed test validators with various statuses
- Seed validator_block_stats with realistic data
- Test end-to-end DTO conversion

---

## Documentation Updates

### 1. Postman Collection

Add to `postman/Injective_Explorer_Complete_API.postman_collection.json`:

**Folder: "验证者接口"**
- GET /api/v1/validators (with test scripts)
- GET /api/v1/validator/info (with test scripts)
- GET /api/v1/validator/uptime (with test scripts)

### 2. OpenAPI Specification

Auto-generated from proto files via `make api`:
- Endpoints, parameters, response schemas automatically added

---

## Implementation Plan

### Task Breakdown

**Estimated time: 4 days**

1. **Proto definitions** (0.5 day)
   - Create validator.proto
   - Modify explorer.proto
   - Generate code: `make api`

2. **GetValidators implementation** (1 day)
   - Core service method
   - Filtering logic
   - DTO conversion helpers
   - Unit tests (10+ cases)

3. **GetValidator implementation** (1 day)
   - Core service method
   - Description decoding
   - SlashingEvents parsing
   - Uptime trend query
   - Unit tests (8+ cases)

4. **GetValidatorUptime implementation** (1 day)
   - Dual-mode query logic
   - Period parsing
   - Stats aggregation
   - Unit tests (8+ cases)

5. **Integration & Documentation** (0.5 day)
   - Postman collection updates
   - End-to-end testing
   - Code review & optimization

---

## Success Criteria

- ✅ All 3 APIs implemented and working
- ✅ Unit test coverage > 90%
- ✅ All tests passing
- ✅ Postman collection updated
- ✅ OpenAPI spec auto-generated
- ✅ No database schema changes (reuse existing tables)
- ✅ Response times < 200ms for typical queries
- ✅ Code follows existing patterns (Phase 1 Wasm APIs)

---

## Risk Mitigation

**Performance concerns:**
- Pre-calculate total voting power (avoid repeated SUM queries)
- Use existing aggregated stats table (validator_block_stats)
- Add caching for validator list (Redis, 1-minute TTL)

**Data quality:**
- Validate Description/SlashingEvents JSON before parsing
- Graceful degradation if stats unavailable
- Clear error messages for invalid queries

**Compatibility:**
- No breaking changes to existing APIs
- Reuse existing models and utilities
- Follow Phase 1 patterns consistently

---

**Design complete. Ready for implementation.**
