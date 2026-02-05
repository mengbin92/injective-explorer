# Token Market Overview Enhancement Design

**Date**: 2026-02-05
**Status**: Approved
**Author**: Claude & User

## Overview

Enhance the token market overview functionality by:
1. Adding 24h high/low price fields to market data
2. Providing admin test interfaces for manual data modification
3. Updating Postman test collections

## Requirements

### 1. Data Model Enhancement
- Add `high_24h` and `low_24h` fields to `token_market_cache` table
- Support these fields in API responses for token detail and token list

### 2. Test Interface Requirements
- Admin-level API for modifying token market data in test environments
- Full data override capability for maximum testing flexibility
- Data source tracking to prevent auto-sync from overwriting manual test data
- Reset capability to return test data back to auto-sync mode

### 3. Postman Collection Updates
- Add test cases for new 24h high/low price fields
- Add test cases for new admin test interfaces
- Follow existing collection style and structure

## Architecture

### 1. Database Schema Changes

```sql
-- Migration: Add 24h high/low price fields
ALTER TABLE token_market_cache
ADD COLUMN high_24h DECIMAL(30,18) DEFAULT 0 COMMENT '24小时最高价(USD)',
ADD COLUMN low_24h DECIMAL(30,18) DEFAULT 0 COMMENT '24小时最低价(USD)';

-- data_source field (already exists) values:
-- 'coingecko' - Auto-synced from CoinGecko
-- 'manual_test' - Manually set via test interface
```

### 2. Model Changes

**File**: `internal/models/token_market_cache.go`

```go
type TokenMarketCache struct {
    // ... existing fields ...
    High24h  float64 `json:"high_24h" gorm:"column:high_24h;type:decimal(30,18)"`
    Low24h   float64 `json:"low_24h" gorm:"column:low_24h;type:decimal(30,18)"`
}

type TokenMarketCacheUpdate struct {
    // ... existing fields ...
    High24h  float64
    Low24h   float64
}
```

### 3. Service Layer Changes

#### 3.1 CoinGecko Data Sync Enhancement

**File**: `internal/service/price.go`

```go
type TokenMarketData struct {
    // ... existing fields ...
    High24h  float64  // 24h highest price
    Low24h   float64  // 24h lowest price
}
```

Update `GetTokenFullMarketData()` to fetch 24h high/low from CoinGecko API.

**File**: `internal/service/token_market_cache_sync.go`

Modify `updateTokenCache()` to:
- Only update records where `data_source = 'coingecko'`
- Include `high_24h` and `low_24h` in updates

#### 3.2 Token Detail Service Enhancement

**File**: `internal/service/token_detail.go`

Update `GetTokenDetail()` to include high_24h and low_24h in response.

#### 3.3 Token List Service Enhancement

**File**: `internal/service/token_list.go`

Update `ListTokens()` to include high_24h and low_24h in response.

### 4. API Changes

#### 4.1 Proto Updates

**File**: `api/explorer/v1/token_detail.proto`

```protobuf
message TokenMarketInfo {
    string price = 1;
    double price_change_24h = 2;
    string volume_24h = 3;
    int64 holders_count = 4;
    int64 total_transfers = 5;
    string high_24h = 6;   // New: 24h highest price (formatted)
    string low_24h = 7;    // New: 24h lowest price (formatted)
}
```

**File**: `api/explorer/v1/token_list.proto`

```protobuf
message TokenListItemDTO {
    // ... existing fields ...
    double high_24h = 18;   // New: 24h highest price
    double low_24h = 19;    // New: 24h lowest price
}
```

**File**: `api/explorer/v1/admin.proto`

```protobuf
service AdminService {
    // ... existing RPCs ...

    // Update token market cache for testing
    rpc UpdateTokenMarketCache(UpdateTokenMarketCacheRequest) returns (Reply) {
        option (google.api.http) = {
            post: "/api/v1/admin/token-market-cache"
            body: "*"
        };
    }

    // Reset token market cache to auto-sync mode
    rpc ResetTokenMarketCache(ResetTokenMarketCacheRequest) returns (Reply) {
        option (google.api.http) = {
            post: "/api/v1/admin/token-market-cache/reset"
            body: "*"
        };
    }

    // Get token market cache (for verification)
    rpc GetTokenMarketCache(GetTokenMarketCacheRequest) returns (Reply) {
        option (google.api.http) = {
            get: "/api/v1/admin/token-market-cache"
        };
    }
}

message UpdateTokenMarketCacheRequest {
    string token_symbol = 1 [json_name = "token_symbol"];
    string chain_id = 2 [json_name = "chain_id"];

    // Market data fields (all optional, only provided fields will be updated)
    optional double price_usd = 3 [json_name = "price_usd"];
    optional double price_change_24h = 4 [json_name = "price_change_24h"];
    optional double high_24h = 5 [json_name = "high_24h"];
    optional double low_24h = 6 [json_name = "low_24h"];
    optional double market_cap_usd = 7 [json_name = "market_cap_usd"];
    optional double fully_diluted_market_cap = 8 [json_name = "fully_diluted_market_cap"];
    optional double total_volume_24h = 9 [json_name = "total_volume_24h"];
    optional int64 holders_count = 10 [json_name = "holders_count"];
    optional string total_supply = 11 [json_name = "total_supply"];
    optional string circulating_supply = 12 [json_name = "circulating_supply"];
    optional string max_supply = 13 [json_name = "max_supply"];
}

message UpdateTokenMarketCacheResponse {
    bool success = 1;
    string message = 2;
    TokenMarketCacheDTO data = 3;
}

message ResetTokenMarketCacheRequest {
    string token_symbol = 1 [json_name = "token_symbol"];
    string chain_id = 2 [json_name = "chain_id"];
}

message ResetTokenMarketCacheResponse {
    bool success = 1;
    string message = 2;
}

message GetTokenMarketCacheRequest {
    string token_symbol = 1 [json_name = "token_symbol"];
    string chain_id = 2 [json_name = "chain_id"];
}

message TokenMarketCacheDTO {
    string token_symbol = 1 [json_name = "token_symbol"];
    string contract_address = 2 [json_name = "contract_address"];
    string chain_id = 3 [json_name = "chain_id"];
    double price_usd = 4 [json_name = "price_usd"];
    double price_change_24h = 5 [json_name = "price_change_24h"];
    double high_24h = 6 [json_name = "high_24h"];
    double low_24h = 7 [json_name = "low_24h"];
    double market_cap_usd = 8 [json_name = "market_cap_usd"];
    double fully_diluted_market_cap = 9 [json_name = "fully_diluted_market_cap"];
    double total_volume_24h = 10 [json_name = "total_volume_24h"];
    int64 holders_count = 11 [json_name = "holders_count"];
    string total_supply = 12 [json_name = "total_supply"];
    string circulating_supply = 13 [json_name = "circulating_supply"];
    string max_supply = 14 [json_name = "max_supply"];
    int64 last_updated = 15 [json_name = "last_updated"];
    string data_source = 16 [json_name = "data_source"];
}
```

#### 4.2 Admin Service Implementation

**File**: `internal/service/admin.go`

Implement three new methods:
- `UpdateTokenMarketCache()` - Set market data manually for testing
- `ResetTokenMarketCache()` - Reset data source back to 'coingecko' for auto-sync
- `GetTokenMarketCache()` - Retrieve current cache data for verification

## Data Flow

### Auto-Sync Flow (Existing + Enhanced)
```
CoinGecko API
  → GetTokenFullMarketData() [fetch price, volume, supply, high_24h, low_24h]
  → updateTokenCache() [only update where data_source='coingecko']
  → token_market_cache table
```

### Manual Test Flow (New)
```
Admin API Request
  → UpdateTokenMarketCache() [set data_source='manual_test']
  → token_market_cache table [upsert with manual data]
  → Response with updated data
```

### Reset Flow (New)
```
Admin API Request
  → ResetTokenMarketCache() [change data_source back to 'coingecko']
  → token_market_cache table [mark for auto-sync]
  → Next sync will update with real CoinGecko data
```

## Testing Strategy

### Unit Tests
- Test `updateTokenCache()` skips manual_test records
- Test admin service CRUD operations
- Test data source filtering logic

### Integration Tests
- Test full flow: set manual data → verify not overwritten by sync → reset → verify updated by sync
- Test Postman collection with all new endpoints

### Postman Test Cases
1. **Token Detail API**: Verify high_24h and low_24h in response
2. **Token List API**: Verify high_24h and low_24h in response
3. **Admin - Update Market Cache**: Set manual test data
4. **Admin - Get Market Cache**: Verify manual data
5. **Admin - Reset Market Cache**: Reset to auto-sync
6. **Admin - Verify Reset**: Confirm data source changed

## Migration Plan

### Phase 1: Database & Model
1. Create migration file for schema changes
2. Update Go models
3. Run migration on test database

### Phase 2: API & Service
1. Update proto files (token_detail, token_list, admin)
2. Generate proto code: `make api`
3. Implement admin service methods
4. Update token detail/list services
5. Update CoinGecko sync service

### Phase 3: Testing
1. Update Postman collections
2. Write unit tests
3. Run integration tests
4. Verify on test environment

### Phase 4: Deployment
1. Deploy to staging
2. Verify admin interfaces work correctly
3. Verify auto-sync respects data_source field
4. Deploy to production (with admin features controlled by config)

## Security Considerations

1. **Admin-only Access**: All test interfaces require admin authentication
2. **Environment Control**: Consider adding config flag `ENABLE_TEST_ADMIN_API` to disable in production
3. **Audit Logging**: Log all manual data modifications with user/timestamp
4. **Data Source Visibility**: Always expose `data_source` in admin responses so operators know what's manual vs auto

## API Documentation Updates

Update API documentation to include:
- New fields in token detail/list responses
- Admin endpoints for test data management
- Data source field meaning and behavior

## Success Criteria

- [ ] 24h high/low price fields added to database and models
- [ ] Token detail API returns high_24h and low_24h
- [ ] Token list API returns high_24h and low_24h
- [ ] Admin API can set manual test data
- [ ] Admin API can reset data to auto-sync mode
- [ ] Auto-sync service respects data_source field
- [ ] Postman collections updated with test cases
- [ ] All tests passing
- [ ] Documentation updated
