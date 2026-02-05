# Token Market Overview Enhancement Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add 24h high/low price fields to token market data and create admin test interfaces for manual data modification

**Architecture:** Extend token_market_cache table with high_24h/low_24h fields, update CoinGecko sync to fetch these values, expose in token detail/list APIs, and provide admin endpoints for test data manipulation with data_source tracking to prevent sync conflicts

**Tech Stack:** Go, gRPC, Protocol Buffers, PostgreSQL, GORM, CoinGecko API, Postman

---

## Task 1: Database Migration - Add 24h High/Low Price Fields

**Files:**
- Create: `database/migrations/add_token_market_24h_high_low.sql`

**Step 1: Create migration file**

Create the SQL migration file with the following content:

```sql
-- Migration: Add 24h high/low price fields to token_market_cache
-- Date: 2026-02-05
-- Purpose: Support displaying 24h high/low prices in token market overview

-- Add high_24h and low_24h columns
ALTER TABLE token_market_cache
ADD COLUMN IF NOT EXISTS high_24h DECIMAL(30,18) DEFAULT 0 COMMENT '24小时最高价(USD)',
ADD COLUMN IF NOT EXISTS low_24h DECIMAL(30,18) DEFAULT 0 COMMENT '24小时最低价(USD)';

-- Create index for better query performance
CREATE INDEX IF NOT EXISTS idx_token_market_cache_high_24h ON token_market_cache(high_24h);
CREATE INDEX IF NOT EXISTS idx_token_market_cache_low_24h ON token_market_cache(low_24h);

-- Comments for documentation
COMMENT ON COLUMN token_market_cache.high_24h IS '24小时最高价格(USD),来自CoinGecko API或手动测试设置';
COMMENT ON COLUMN token_market_cache.low_24h IS '24小时最低价格(USD),来自CoinGecko API或手动测试设置';
```

**Step 2: Verify migration file syntax**

Check the SQL syntax is correct:
```bash
cat database/migrations/add_token_market_24h_high_low.sql
```

Expected: File displays correctly with proper SQL syntax

**Step 3: Commit migration**

```bash
git add database/migrations/add_token_market_24h_high_low.sql
git commit -m "feat(db): add 24h high/low price columns to token_market_cache

- Add high_24h and low_24h columns with DECIMAL(30,18) type
- Add indexes for query performance
- Add column comments for documentation

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 2: Update Go Models - TokenMarketCache

**Files:**
- Modify: `internal/models/token_market_cache.go`

**Step 1: Add fields to TokenMarketCache struct**

Add high_24h and low_24h fields after the existing market data fields (around line 26):

```go
// TokenMarketCache represents pre-computed token market data for fast queries.
type TokenMarketCache struct {
	gorm.Model

	// Token identification
	TokenSymbol     string `json:"token_symbol" gorm:"column:token_symbol;size:50;not null;uniqueIndex:uk_token_symbol_chain"`
	ContractAddress string `json:"contract_address" gorm:"column:contract_address;size:128;index"`
	ChainID         string `json:"chain_id" gorm:"column:chain_id;size:50;default:'injective-1';uniqueIndex:uk_token_symbol_chain"`

	// Market data (from CoinGecko)
	PriceUSD             float64 `json:"price_usd" gorm:"column:price_usd;type:decimal(30,18)"`
	PriceChange24h       float64 `json:"price_change_24h" gorm:"column:price_change_24h;type:decimal(10,4)"`
	High24h              float64 `json:"high_24h" gorm:"column:high_24h;type:decimal(30,18)"`           // NEW: 24h highest price
	Low24h               float64 `json:"low_24h" gorm:"column:low_24h;type:decimal(30,18)"`             // NEW: 24h lowest price
	MarketCapUSD         float64 `json:"market_cap_usd" gorm:"column:market_cap_usd;type:decimal(40,2);index"`
	FullyDilutedMarketCap float64 `json:"fully_diluted_market_cap" gorm:"column:fully_diluted_market_cap;type:decimal(40,2)"`
	TotalVolume24h       float64 `json:"total_volume_24h" gorm:"column:total_volume_24h;type:decimal(40,2)"`

	// ... rest of fields
}
```

**Step 2: Add fields to TokenMarketCacheUpdate struct**

Add high_24h and low_24h fields to the update struct (around line 73):

```go
// TokenMarketCacheUpdate represents fields that can be updated.
type TokenMarketCacheUpdate struct {
	PriceUSD              float64
	PriceChange24h        float64
	High24h               float64  // NEW
	Low24h                float64  // NEW
	MarketCapUSD          float64
	FullyDilutedMarketCap float64
	TotalVolume24h        float64
	HoldersCount          int64
	TotalSupply           *string
	CirculatingSupply     *string
	MaxSupply             *string
	LastUpdated           int64
	DataSource            string
}
```

**Step 3: Verify code compiles**

```bash
cd /Users/mac/vscode/mengbin/biya/biya-explorer
go build ./internal/models/...
```

Expected: Compiles successfully with no errors

**Step 4: Commit model changes**

```bash
git add internal/models/token_market_cache.go
git commit -m "feat(model): add high_24h and low_24h to TokenMarketCache

- Add High24h and Low24h fields to TokenMarketCache struct
- Add High24h and Low24h to TokenMarketCacheUpdate struct
- Support storing 24h price range data

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 3: Update Service Layer - TokenMarketData

**Files:**
- Modify: `internal/service/price.go`

**Step 1: Add fields to TokenMarketData struct**

Find the TokenMarketData struct (around line 121) and add high_24h and low_24h fields:

```go
// TokenMarketData contains market data for a token from CoinGecko.
type TokenMarketData struct {
	PriceUSD              float64
	PriceChange24h        float64
	High24h               float64  // NEW: 24h highest price
	Low24h                float64  // NEW: 24h lowest price
	MarketCapUSD          float64
	TotalVolume24h        float64
	HoldersCount          int64
	MaxSupply             *big.Int
	CirculatingSupply     *big.Int
	TotalSupply           *big.Int
	FullyDilutedMarketCap float64
}
```

**Step 2: Find GetTokenFullMarketData function**

Search for the function that fetches full market data:

```bash
grep -n "func GetTokenFullMarketData" internal/service/price.go
```

**Step 3: Update GetTokenFullMarketData to fetch high/low from CoinGecko**

Locate the function (likely around line 300-400) and update the CoinGecko API call to include high_24h and low_24h. The CoinGecko API returns these in the `usd_24h_high` and `usd_24h_low` fields.

Update the response parsing section to include:

```go
// Parse market data from CoinGecko response
if data, ok := response[tokenConfig.CoinGeckoID]; ok {
	marketData := &TokenMarketData{
		PriceUSD:       data.USD,
		PriceChange24h: data.USD24hChange,
		High24h:        data.USD24hHigh,    // NEW
		Low24h:         data.USD24hLow,     // NEW
		// ... rest of fields
	}
	// ...
}
```

**Step 4: Update CoinGeckoDetailResponse struct**

Find the CoinGeckoDetailResponse struct (around line 105) and add the high/low fields:

```go
// CoinGeckoDetailResponse represents the detailed response from CoinGecko API with supply data.
type CoinGeckoDetailResponse map[string]struct {
	USD               float64  `json:"usd"`
	USD24hChange      float64  `json:"usd_24h_change"`
	USD24hHigh        float64  `json:"usd_24h_high"`   // NEW
	USD24hLow         float64  `json:"usd_24h_low"`    // NEW
	TotalSupply       *float64 `json:"total_supply,omitempty"`
	CirculatingSupply *float64 `json:"circulating_supply,omitempty"`
	MaxSupply         *float64 `json:"max_supply,omitempty"`
}
```

**Step 5: Verify code compiles**

```bash
go build ./internal/service/...
```

Expected: Compiles successfully

**Step 6: Commit service changes**

```bash
git add internal/service/price.go
git commit -m "feat(service): add high_24h and low_24h to TokenMarketData

- Add High24h and Low24h fields to TokenMarketData struct
- Update CoinGeckoDetailResponse to parse usd_24h_high and usd_24h_low
- Update GetTokenFullMarketData to fetch 24h price range from CoinGecko

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 4: Update Sync Service - Respect Data Source

**Files:**
- Modify: `internal/service/token_market_cache_sync.go`

**Step 1: Update updateTokenCache to include high_24h and low_24h**

Find the updateTokenCache function (around line 241) and update the cache record creation to include high_24h and low_24h:

```go
// Prepare cache record
cache := &models.TokenMarketCache{
	TokenSymbol:           symbol,
	ContractAddress:       contractAddress,
	ChainID:               DefaultChainID,
	PriceUSD:              data.PriceUSD,
	PriceChange24h:        data.PriceChange24h,
	High24h:               data.High24h,              // NEW
	Low24h:                data.Low24h,               // NEW
	MarketCapUSD:          data.MarketCapUSD,
	FullyDilutedMarketCap: data.FullyDilutedMarketCap,
	TotalVolume24h:        data.TotalVolume24h,
	HoldersCount:          data.HoldersCount,
	LastUpdated:           time.Now().Unix(),
	DataSource:            "coingecko",
}
```

**Step 2: Update upsert logic to only update coingecko records**

Modify the database transaction to only update records where data_source = 'coingecko':

```go
// Upsert to database - only update if data_source is 'coingecko' or doesn't exist
return db.Get().Transaction(func(tx *gorm.DB) error {
	// Check if record exists
	var existing models.TokenMarketCache
	err := tx.Where("token_symbol = ? AND chain_id = ?", symbol, DefaultChainID).
		First(&existing).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Record doesn't exist, create new one
			return tx.Create(cache).Error
		}
		return err
	}

	// Record exists - only update if data_source is 'coingecko'
	if existing.DataSource != "coingecko" {
		global.Logger.Debugf("Skipping sync for %s: data_source is '%s' (manual override)",
			symbol, existing.DataSource)
		return nil
	}

	// Update existing coingecko record
	return tx.Model(&models.TokenMarketCache{}).
		Where("token_symbol = ? AND chain_id = ? AND data_source = ?",
			symbol, DefaultChainID, "coingecko").
		Updates(cache).Error
})
```

**Step 3: Verify code compiles**

```bash
go build ./internal/service/...
```

Expected: Compiles successfully

**Step 4: Commit sync service changes**

```bash
git add internal/service/token_market_cache_sync.go
git commit -m "feat(service): sync respects data_source field

- Include high_24h and low_24h in cache sync
- Only update records where data_source='coingecko'
- Skip manual_test records to prevent overwriting test data
- Add debug logging when skipping manual records

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 5: Update Proto - Token Detail API

**Files:**
- Modify: `api/explorer/v1/token_detail.proto`

**Step 1: Add high_24h and low_24h to TokenMarketInfo message**

Find the TokenMarketInfo message (around line 91) and add the new fields:

```protobuf
// TokenMarketInfo contains market data
message TokenMarketInfo {
  string price = 1;                    // Formatted: "$1.00" or "$0.00000123"
  double price_change_24h = 2 [ json_name = "price_change_24h" ]; // For badge display
  string volume_24h = 3 [ json_name = "volume_24h" ];          // Formatted: "$53,200,000"
  int64 holders_count = 4 [ json_name = "holders_count" ];     // Integer count
  int64 total_transfers = 5 [ json_name = "total_transfers" ]; // Total transfer count
  string high_24h = 6 [ json_name = "high_24h" ];              // NEW: 24h highest price (formatted)
  string low_24h = 7 [ json_name = "low_24h" ];                // NEW: 24h lowest price (formatted)
}
```

**Step 2: Generate proto code**

```bash
make api
```

Expected: Proto files regenerated successfully

**Step 3: Commit proto changes**

```bash
git add api/explorer/v1/token_detail.proto api/explorer/v1/token_detail.pb.go api/explorer/v1/token_detail_http.pb.go
git commit -m "feat(api): add high_24h and low_24h to TokenMarketInfo

- Add high_24h field (formatted string) to token market response
- Add low_24h field (formatted string) to token market response
- Regenerate proto code

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 6: Update Proto - Token List API

**Files:**
- Modify: `api/explorer/v1/token_list.proto`

**Step 1: Add high_24h and low_24h to TokenListItemDTO**

Find the TokenListItemDTO message (around line 51) and add the new fields at the end:

```protobuf
// TokenListItemDTO represents a single token in the list
message TokenListItemDTO {
  // Basic information
  uint64 id = 1;
  string contract_address = 2 [ json_name = "contract_address" ];
  string contract_address_short = 3 [ json_name = "contract_address_short" ];
  string token_name = 4 [ json_name = "token_name" ];
  string token_symbol = 5 [ json_name = "token_symbol" ];
  repeated ContractTagInfoDTO tags = 6 [ json_name = "tags" ];

  // Token icon (optional)
  string icon_url = 7 [ json_name = "icon_url" ];
  string icon_gradient = 8 [ json_name = "icon_gradient" ];

  // Market data
  double price_usd = 9 [ json_name = "price_usd" ];
  double price_change_24h = 10 [ json_name = "price_change_24h" ];
  double total_volume_24h = 11 [ json_name = "total_volume_24h" ];
  double market_cap_usd = 12 [ json_name = "market_cap_usd" ];
  int64 holders_count = 13 [ json_name = "holders_count" ];

  // Metadata
  string first_deployment_time = 14 [ json_name = "first_deployment_time" ];
  string created_at = 15 [ json_name = "created_at" ];

  // Token technical details
  string denom = 16 [ json_name = "denom" ];
  int32 decimals = 17 [ json_name = "decimals" ];

  // NEW: 24h price range
  double high_24h = 18 [ json_name = "high_24h" ];   // 24h highest price
  double low_24h = 19 [ json_name = "low_24h" ];     // 24h lowest price
}
```

**Step 2: Generate proto code**

```bash
make api
```

Expected: Proto files regenerated successfully

**Step 3: Commit proto changes**

```bash
git add api/explorer/v1/token_list.proto api/explorer/v1/token_list.pb.go api/explorer/v1/token_list_http.pb.go
git commit -m "feat(api): add high_24h and low_24h to TokenListItemDTO

- Add high_24h field (double) to token list response
- Add low_24h field (double) to token list response
- Regenerate proto code

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 7: Update Proto - Admin API

**Files:**
- Modify: `api/explorer/v1/admin.proto`

**Step 1: Add token market cache management RPCs to AdminService**

Find the AdminService definition and add the new RPCs at the end (before the closing brace, around line 870):

```protobuf
service AdminService {
  // ... existing RPCs ...

  // ========== Token Market Cache Management (Testing) ==========
  // UpdateTokenMarketCache updates token market cache for testing
  rpc UpdateTokenMarketCache(UpdateTokenMarketCacheRequest) returns (Reply) {
    option (google.api.http) = {
      post: "/api/v1/admin/token-market-cache"
      body: "*"
    };
  }

  // ResetTokenMarketCache resets token market cache to auto-sync mode
  rpc ResetTokenMarketCache(ResetTokenMarketCacheRequest) returns (Reply) {
    option (google.api.http) = {
      post: "/api/v1/admin/token-market-cache/reset"
      body: "*"
    };
  }

  // GetTokenMarketCache gets token market cache for verification
  rpc GetTokenMarketCache(GetTokenMarketCacheRequest) returns (Reply) {
    option (google.api.http) = {
      get: "/api/v1/admin/token-market-cache"
    };
  }
}
```

**Step 2: Add request/response messages at the end of the file**

Add these message definitions at the end of admin.proto (around line 872):

```protobuf
// ========== Token Market Cache Messages ==========

// UpdateTokenMarketCacheRequest updates token market data for testing
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

// UpdateTokenMarketCacheResponse response for update
message UpdateTokenMarketCacheResponse {
  bool success = 1;
  string message = 2;
  TokenMarketCacheDTO data = 3;
}

// ResetTokenMarketCacheRequest resets to auto-sync mode
message ResetTokenMarketCacheRequest {
  string token_symbol = 1 [json_name = "token_symbol"];
  string chain_id = 2 [json_name = "chain_id"];
}

// ResetTokenMarketCacheResponse response for reset
message ResetTokenMarketCacheResponse {
  bool success = 1;
  string message = 2;
}

// GetTokenMarketCacheRequest gets cache data
message GetTokenMarketCacheRequest {
  string token_symbol = 1 [json_name = "token_symbol"];
  string chain_id = 2 [json_name = "chain_id"];
}

// TokenMarketCacheDTO complete cache data transfer object
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

**Step 3: Generate proto code**

```bash
make api
```

Expected: Proto files regenerated successfully

**Step 4: Commit proto changes**

```bash
git add api/explorer/v1/admin.proto api/explorer/v1/admin.pb.go api/explorer/v1/admin_grpc.pb.go api/explorer/v1/admin_http.pb.go
git commit -m "feat(api): add token market cache admin endpoints

- Add UpdateTokenMarketCache RPC for manual test data
- Add ResetTokenMarketCache RPC to reset to auto-sync
- Add GetTokenMarketCache RPC for verification
- Include high_24h and low_24h in all messages
- Regenerate proto code

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 8: Implement Admin Service Methods

**Files:**
- Modify: `internal/service/admin.go`

**Step 1: Add GetTokenMarketCache method**

Add this method to the AdminService (find a good location near other Get methods):

```go
// GetTokenMarketCache retrieves token market cache data for verification
func (s *AdminService) GetTokenMarketCache(ctx context.Context, req *pb.GetTokenMarketCacheRequest) (*pb.Reply, error) {
	if req == nil || req.TokenSymbol == "" {
		return ErrorReply(http.StatusBadRequest, "token_symbol is required", errors.New("missing token_symbol")), nil
	}

	chainID := req.ChainId
	if chainID == "" {
		chainID = DefaultChainID
	}

	// Query cache from database
	var cache models.TokenMarketCache
	err := db.Get().WithContext(ctx).
		Where("token_symbol = ? AND chain_id = ?", req.TokenSymbol, chainID).
		First(&cache).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrorReply(http.StatusNotFound, "token market cache not found", err), nil
		}
		global.Logger.Errorf("failed to query token market cache: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to query cache", err), err
	}

	// Convert to DTO
	response := &pb.TokenMarketCacheDTO{
		TokenSymbol:           cache.TokenSymbol,
		ContractAddress:       cache.ContractAddress,
		ChainId:               cache.ChainID,
		PriceUsd:              cache.PriceUSD,
		PriceChange_24H:       cache.PriceChange24h,
		High_24H:              cache.High24h,
		Low_24H:               cache.Low24h,
		MarketCapUsd:          cache.MarketCapUSD,
		FullyDilutedMarketCap: cache.FullyDilutedMarketCap,
		TotalVolume_24H:       cache.TotalVolume24h,
		HoldersCount:          cache.HoldersCount,
		LastUpdated:           cache.LastUpdated,
		DataSource:            cache.DataSource,
	}

	// Add supply data if available
	if cache.TotalSupply != nil {
		response.TotalSupply = *cache.TotalSupply
	}
	if cache.CirculatingSupply != nil {
		response.CirculatingSupply = *cache.CirculatingSupply
	}
	if cache.MaxSupply != nil {
		response.MaxSupply = *cache.MaxSupply
	}

	return MarshalAndReply(response, "marshal token market cache error")
}
```

**Step 2: Add UpdateTokenMarketCache method**

```go
// UpdateTokenMarketCache updates token market cache data for testing
func (s *AdminService) UpdateTokenMarketCache(ctx context.Context, req *pb.UpdateTokenMarketCacheRequest) (*pb.Reply, error) {
	if req == nil || req.TokenSymbol == "" {
		return ErrorReply(http.StatusBadRequest, "token_symbol is required", errors.New("missing token_symbol")), nil
	}

	chainID := req.ChainId
	if chainID == "" {
		chainID = DefaultChainID
	}

	// Build update map with only provided fields
	updates := make(map[string]interface{})

	if req.PriceUsd != nil {
		updates["price_usd"] = *req.PriceUsd
	}
	if req.PriceChange_24H != nil {
		updates["price_change_24h"] = *req.PriceChange_24H
	}
	if req.High_24H != nil {
		updates["high_24h"] = *req.High_24H
	}
	if req.Low_24H != nil {
		updates["low_24h"] = *req.Low_24H
	}
	if req.MarketCapUsd != nil {
		updates["market_cap_usd"] = *req.MarketCapUsd
	}
	if req.FullyDilutedMarketCap != nil {
		updates["fully_diluted_market_cap"] = *req.FullyDilutedMarketCap
	}
	if req.TotalVolume_24H != nil {
		updates["total_volume_24h"] = *req.TotalVolume_24H
	}
	if req.HoldersCount != nil {
		updates["holders_count"] = *req.HoldersCount
	}
	if req.TotalSupply != nil {
		updates["total_supply"] = *req.TotalSupply
	}
	if req.CirculatingSupply != nil {
		updates["circulating_supply"] = *req.CirculatingSupply
	}
	if req.MaxSupply != nil {
		updates["max_supply"] = *req.MaxSupply
	}

	// Always update data_source and last_updated
	updates["data_source"] = "manual_test"
	updates["last_updated"] = time.Now().Unix()

	if len(updates) == 2 {
		return ErrorReply(http.StatusBadRequest, "no fields to update", errors.New("no update fields provided")), nil
	}

	// Perform upsert
	err := db.Get().Transaction(func(tx *gorm.DB) error {
		// Try to find existing record
		var existing models.TokenMarketCache
		err := tx.Where("token_symbol = ? AND chain_id = ?", req.TokenSymbol, chainID).
			First(&existing).Error

		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Create new record with minimal required fields
				newCache := &models.TokenMarketCache{
					TokenSymbol: req.TokenSymbol,
					ChainID:     chainID,
					DataSource:  "manual_test",
					LastUpdated: time.Now().Unix(),
				}

				// Apply updates to new record
				if err := tx.Create(newCache).Error; err != nil {
					return err
				}

				// Now update with provided fields
				return tx.Model(&models.TokenMarketCache{}).
					Where("token_symbol = ? AND chain_id = ?", req.TokenSymbol, chainID).
					Updates(updates).Error
			}
			return err
		}

		// Update existing record
		return tx.Model(&models.TokenMarketCache{}).
			Where("token_symbol = ? AND chain_id = ?", req.TokenSymbol, chainID).
			Updates(updates).Error
	})

	if err != nil {
		global.Logger.Errorf("failed to update token market cache: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to update cache", err), err
	}

	// Retrieve updated data
	var updated models.TokenMarketCache
	db.Get().WithContext(ctx).
		Where("token_symbol = ? AND chain_id = ?", req.TokenSymbol, chainID).
		First(&updated)

	// Build response
	response := &pb.UpdateTokenMarketCacheResponse{
		Success: true,
		Message: fmt.Sprintf("Token market cache updated successfully for %s", req.TokenSymbol),
		Data: &pb.TokenMarketCacheDTO{
			TokenSymbol:           updated.TokenSymbol,
			ContractAddress:       updated.ContractAddress,
			ChainId:               updated.ChainID,
			PriceUsd:              updated.PriceUSD,
			PriceChange_24H:       updated.PriceChange24h,
			High_24H:              updated.High24h,
			Low_24H:               updated.Low24h,
			MarketCapUsd:          updated.MarketCapUSD,
			FullyDilutedMarketCap: updated.FullyDilutedMarketCap,
			TotalVolume_24H:       updated.TotalVolume24h,
			HoldersCount:          updated.HoldersCount,
			LastUpdated:           updated.LastUpdated,
			DataSource:            updated.DataSource,
		},
	}

	if updated.TotalSupply != nil {
		response.Data.TotalSupply = *updated.TotalSupply
	}
	if updated.CirculatingSupply != nil {
		response.Data.CirculatingSupply = *updated.CirculatingSupply
	}
	if updated.MaxSupply != nil {
		response.Data.MaxSupply = *updated.MaxSupply
	}

	global.Logger.Infof("Admin: Updated token market cache for %s (data_source=manual_test)", req.TokenSymbol)

	return MarshalAndReply(response, "marshal update response error")
}
```

**Step 3: Add ResetTokenMarketCache method**

```go
// ResetTokenMarketCache resets token market cache to auto-sync mode
func (s *AdminService) ResetTokenMarketCache(ctx context.Context, req *pb.ResetTokenMarketCacheRequest) (*pb.Reply, error) {
	if req == nil || req.TokenSymbol == "" {
		return ErrorReply(http.StatusBadRequest, "token_symbol is required", errors.New("missing token_symbol")), nil
	}

	chainID := req.ChainId
	if chainID == "" {
		chainID = DefaultChainID
	}

	// Update data_source back to 'coingecko' so next sync will update it
	err := db.Get().WithContext(ctx).
		Model(&models.TokenMarketCache{}).
		Where("token_symbol = ? AND chain_id = ?", req.TokenSymbol, chainID).
		Update("data_source", "coingecko").Error

	if err != nil {
		global.Logger.Errorf("failed to reset token market cache: %v", err)
		return ErrorReply(http.StatusInternalServerError, "failed to reset cache", err), err
	}

	response := &pb.ResetTokenMarketCacheResponse{
		Success: true,
		Message: fmt.Sprintf("Token market cache reset to auto-sync mode for %s", req.TokenSymbol),
	}

	global.Logger.Infof("Admin: Reset token market cache for %s (data_source=coingecko)", req.TokenSymbol)

	return MarshalAndReply(response, "marshal reset response error")
}
```

**Step 4: Verify code compiles**

```bash
go build ./internal/service/...
```

Expected: Compiles successfully

**Step 5: Commit admin service implementation**

```bash
git add internal/service/admin.go
git commit -m "feat(admin): implement token market cache management

- Add GetTokenMarketCache for retrieving cache data
- Add UpdateTokenMarketCache for manual test data
- Add ResetTokenMarketCache to reset to auto-sync
- Include high_24h and low_24h in all operations
- Add logging for admin actions

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 9: Update Token Detail Service

**Files:**
- Modify: `internal/service/token_detail.go`

**Step 1: Update GetTokenDetail to include high_24h and low_24h**

Find the GetTokenDetail method (around line 48) and locate where the response.Market is built (around line 117). Add high_24h and low_24h fields:

```go
response := &pb.TokenDetailResponse{
	Basic: &pb.TokenBasicInfo{
		// ... existing fields
	},
	Overview: &pb.TokenOverviewInfo{},
	Market: &pb.TokenMarketInfo{
		Price:          formatPrice(0),
		Volume_24H:     formatLargeNumber(0),
		HoldersCount:   0,
		TotalTransfers: totalTransfers,
		High_24H:       "-",  // NEW: default value
		Low_24H:        "-",  // NEW: default value
	},
	Additional: &pb.TokenAdditionalInfo{
		// ... existing fields
	},
}

// Fill market data
if marketData != nil {
	response.Market.Price = formatPrice(marketData.PriceUSD)
	response.Market.PriceChange_24H = marketData.PriceChange24h
	response.Market.Volume_24H = formatLargeNumber(marketData.TotalVolume24h)
	response.Market.HoldersCount = marketData.HoldersCount
	response.Market.High_24H = formatPrice(marketData.High24h)  // NEW
	response.Market.Low_24H = formatPrice(marketData.Low24h)    // NEW

	response.Overview.MarketCap = formatLargeNumber(marketData.MarketCapUSD)
}
```

**Step 2: Verify code compiles**

```bash
go build ./internal/service/...
```

Expected: Compiles successfully

**Step 3: Commit changes**

```bash
git add internal/service/token_detail.go
git commit -m "feat(service): add high_24h and low_24h to token detail

- Include High24h and Low24h in TokenMarketInfo response
- Format prices using existing formatPrice helper
- Provide default '-' when data not available

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 10: Update Token List Service

**Files:**
- Modify: `internal/service/token_list.go`

**Step 1: Find the ListTokens method and locate DTO building**

Search for where TokenListItemDTO is populated (likely in a loop building response items).

**Step 2: Add high_24h and low_24h to token list items**

Update the DTO building to include high_24h and low_24h from the cache or market data:

```go
// When building token list items
item := &pb.TokenListItemDTO{
	// ... existing basic fields ...

	// Market data
	PriceUsd:        cache.PriceUSD,
	PriceChange_24H: cache.PriceChange24h,
	High_24H:        cache.High24h,        // NEW
	Low_24H:         cache.Low24h,         // NEW
	TotalVolume_24H: cache.TotalVolume24h,
	MarketCapUsd:    cache.MarketCapUSD,
	HoldersCount:    cache.HoldersCount,

	// ... rest of fields
}
```

**Step 3: Verify code compiles**

```bash
go build ./internal/service/...
```

Expected: Compiles successfully

**Step 4: Commit changes**

```bash
git add internal/service/token_list.go
git commit -m "feat(service): add high_24h and low_24h to token list

- Include High24h and Low24h in TokenListItemDTO
- Fetch from token_market_cache table
- Expose in list response

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 11: Update Postman Collection - Add Admin Test Cases

**Files:**
- Modify: `postman/Injective_Explorer_Complete_API.postman_collection.json`

**Step 1: Read existing collection structure**

```bash
head -100 postman/Injective_Explorer_Complete_API.postman_collection.json
```

Expected: See the JSON structure with folders and items

**Step 2: Add a new folder for Admin Token Market Cache tests**

Insert a new folder after the existing Admin sections with the following structure (adapt to match existing style):

```json
{
  "name": "Admin - Token Market Cache",
  "item": [
    {
      "name": "Get Token Market Cache",
      "request": {
        "method": "GET",
        "header": [],
        "url": {
          "raw": "{{base_url}}/api/v1/admin/token-market-cache?token_symbol={{token_symbol}}&chain_id={{chain_id}}",
          "host": ["{{base_url}}"],
          "path": ["api", "v1", "admin", "token-market-cache"],
          "query": [
            {
              "key": "token_symbol",
              "value": "{{token_symbol}}",
              "description": "Token symbol (e.g., 'inj', 'usdt')"
            },
            {
              "key": "chain_id",
              "value": "{{chain_id}}",
              "description": "Chain ID (default: injective-1)"
            }
          ]
        },
        "description": "Retrieve token market cache data for verification"
      }
    },
    {
      "name": "Update Token Market Cache",
      "request": {
        "method": "POST",
        "header": [
          {
            "key": "Content-Type",
            "value": "application/json"
          }
        ],
        "body": {
          "mode": "raw",
          "raw": "{\n  \"token_symbol\": \"inj\",\n  \"chain_id\": \"injective-1\",\n  \"price_usd\": 25.50,\n  \"price_change_24h\": 5.25,\n  \"high_24h\": 26.80,\n  \"low_24h\": 24.20,\n  \"market_cap_usd\": 2000000000,\n  \"total_volume_24h\": 150000000,\n  \"holders_count\": 50000\n}"
        },
        "url": {
          "raw": "{{base_url}}/api/v1/admin/token-market-cache",
          "host": ["{{base_url}}"],
          "path": ["api", "v1", "admin", "token-market-cache"]
        },
        "description": "Manually set token market data for testing. Sets data_source to 'manual_test'."
      }
    },
    {
      "name": "Reset Token Market Cache",
      "request": {
        "method": "POST",
        "header": [
          {
            "key": "Content-Type",
            "value": "application/json"
          }
        ],
        "body": {
          "mode": "raw",
          "raw": "{\n  \"token_symbol\": \"inj\",\n  \"chain_id\": \"injective-1\"\n}"
        },
        "url": {
          "raw": "{{base_url}}/api/v1/admin/token-market-cache/reset",
          "host": ["{{base_url}}"],
          "path": ["api", "v1", "admin", "token-market-cache", "reset"]
        },
        "description": "Reset token market cache to auto-sync mode (data_source='coingecko')"
      }
    }
  ]
}
```

**Step 3: Update existing Token Detail test to check high_24h and low_24h**

Find the "Get Token Detail" test case and add assertions for the new fields in the Tests tab:

```javascript
pm.test("Response includes high_24h and low_24h", function () {
    var jsonData = pm.response.json();
    pm.expect(jsonData.data.market).to.have.property('high_24h');
    pm.expect(jsonData.data.market).to.have.property('low_24h');
});
```

**Step 4: Update existing Token List test to check high_24h and low_24h**

Find the "List Tokens" test case and add assertions:

```javascript
pm.test("Token items include high_24h and low_24h", function () {
    var jsonData = pm.response.json();
    if (jsonData.data.items && jsonData.data.items.length > 0) {
        pm.expect(jsonData.data.items[0]).to.have.property('high_24h');
        pm.expect(jsonData.data.items[0]).to.have.property('low_24h');
    }
});
```

**Step 5: Verify JSON syntax**

```bash
python3 -m json.tool postman/Injective_Explorer_Complete_API.postman_collection.json > /dev/null
```

Expected: No syntax errors

**Step 6: Commit Postman updates**

```bash
git add postman/Injective_Explorer_Complete_API.postman_collection.json
git commit -m "test(postman): add token market cache admin tests

- Add folder for Admin Token Market Cache tests
- Add Get Token Market Cache test case
- Add Update Token Market Cache test case
- Add Reset Token Market Cache test case
- Update Token Detail test to check high_24h/low_24h
- Update Token List test to check high_24h/low_24h

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 12: Update Postman README

**Files:**
- Modify: `postman/README.md`

**Step 1: Add documentation for new admin endpoints**

Find the Admin API section in the README and add documentation for the new endpoints:

```markdown
### Admin - Token Market Cache Management

- **Get Token Market Cache**: Retrieve token market cache data for verification
  - Method: `GET /api/v1/admin/token-market-cache`
  - Query params: `token_symbol`, `chain_id`
  - Description: Get current market cache data including data_source field

- **Update Token Market Cache**: Manually set token market data for testing
  - Method: `POST /api/v1/admin/token-market-cache`
  - Body: JSON with token_symbol, chain_id, and market data fields
  - Description: Set manual test data. All market fields are optional. Sets data_source to 'manual_test' to prevent auto-sync override.

- **Reset Token Market Cache**: Reset to auto-sync mode
  - Method: `POST /api/v1/admin/token-market-cache/reset`
  - Body: JSON with token_symbol and chain_id
  - Description: Reset data_source to 'coingecko' so next sync will update with real market data
```

**Step 2: Add environment variable documentation**

Update the environment variables section to mention the new fields:

```markdown
## Environment Variables

- `token_symbol`: Token symbol for testing (default: `inj`)
  - Used in token detail, list, and market cache admin endpoints
  - Examples: `inj`, `usdt`, `usdc`
```

**Step 3: Commit README updates**

```bash
git add postman/README.md
git commit -m "docs(postman): document token market cache admin endpoints

- Add Admin Token Market Cache Management section
- Document Get, Update, and Reset endpoints
- Add usage examples for manual test data
- Explain data_source field behavior

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 13: Run Migration and Test

**Step 1: Check if migration has been run**

```bash
psql -h localhost -U your_user -d injective_explorer -c "SELECT column_name FROM information_schema.columns WHERE table_name='token_market_cache' AND column_name IN ('high_24h', 'low_24h');"
```

Expected: Empty result (columns don't exist yet) or columns already exist

**Step 2: Run migration if needed**

```bash
psql -h localhost -U your_user -d injective_explorer -f database/migrations/add_token_market_24h_high_low.sql
```

Expected: ALTER TABLE and CREATE INDEX commands execute successfully

**Step 3: Verify migration**

```bash
psql -h localhost -U your_user -d injective_explorer -c "\d token_market_cache" | grep "24h"
```

Expected: Shows high_24h and low_24h columns

**Step 4: Build the application**

```bash
make build
```

Expected: Builds successfully with no errors

**Step 5: Run the application (if possible)**

```bash
./bin/injective-explorer
```

Expected: Starts successfully, no errors related to missing fields

---

## Task 14: Integration Testing

**Step 1: Test Admin - Update Market Cache**

Using Postman or curl:

```bash
curl -X POST http://localhost:8000/api/v1/admin/token-market-cache \
  -H "Content-Type: application/json" \
  -d '{
    "token_symbol": "inj",
    "chain_id": "injective-1",
    "price_usd": 25.50,
    "price_change_24h": 5.25,
    "high_24h": 26.80,
    "low_24h": 24.20,
    "market_cap_usd": 2000000000,
    "total_volume_24h": 150000000
  }'
```

Expected: Success response with data showing data_source='manual_test'

**Step 2: Test Admin - Get Market Cache**

```bash
curl "http://localhost:8000/api/v1/admin/token-market-cache?token_symbol=inj&chain_id=injective-1"
```

Expected: Returns the manually set data with high_24h=26.80 and low_24h=24.20

**Step 3: Test Token Detail includes high/low**

```bash
curl "http://localhost:8000/api/v1/tokens/detail?contract_address=<address>&chain_id=injective-1"
```

Expected: Response includes market.high_24h and market.low_24h fields

**Step 4: Test Token List includes high/low**

```bash
curl "http://localhost:8000/api/v1/tokens/list?page=1&page_size=10"
```

Expected: Each token in items array has high_24h and low_24h fields

**Step 5: Test Admin - Reset Market Cache**

```bash
curl -X POST http://localhost:8000/api/v1/admin/token-market-cache/reset \
  -H "Content-Type: application/json" \
  -d '{
    "token_symbol": "inj",
    "chain_id": "injective-1"
  }'
```

Expected: Success response, subsequent Get shows data_source='coingecko'

**Step 6: Verify sync respects data_source**

- Set manual test data for a token
- Trigger or wait for auto-sync
- Verify manual data is NOT overwritten

```bash
# Set manual data
curl -X POST http://localhost:8000/api/v1/admin/token-market-cache \
  -H "Content-Type: application/json" \
  -d '{"token_symbol": "inj", "price_usd": 999.99, "high_24h": 1000, "low_24h": 900}'

# Wait for sync or trigger it
# Then check data is still manual
curl "http://localhost:8000/api/v1/admin/token-market-cache?token_symbol=inj"
```

Expected: price_usd is still 999.99 and data_source='manual_test'

---

## Task 15: Final Commit and Documentation

**Step 1: Create a summary commit**

```bash
git add .
git commit -m "feat: complete token market overview enhancement

This completes the token market overview enhancement with:

Phase 1 - Database & Model:
- Added high_24h and low_24h columns to token_market_cache
- Updated Go models with new fields

Phase 2 - API & Service:
- Updated proto files for token_detail, token_list, and admin
- Implemented admin endpoints for test data management
- Enhanced CoinGecko sync to fetch 24h high/low
- Modified sync service to respect data_source field

Phase 3 - Testing:
- Updated Postman collection with admin test cases
- Added test assertions for new fields
- Updated documentation

Features:
- Token detail API returns high_24h and low_24h
- Token list API returns high_24h and low_24h
- Admin can manually set test market data
- Admin can reset to auto-sync mode
- Auto-sync respects manual test data

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

**Step 2: Update main README if exists**

Check if there's a main README that needs updating:

```bash
ls -la README.md
```

If exists and has API documentation section, add notes about:
- New high_24h and low_24h fields in token responses
- Admin endpoints for test data management

**Step 3: Generate or update API documentation**

If the project uses automated API doc generation:

```bash
# Example commands (adjust based on project)
make docs
# or
swag init
```

---

## Completion Checklist

Verify all success criteria from the design document:

- [ ] 24h high/low price fields added to database and models
- [ ] Token detail API returns high_24h and low_24h
- [ ] Token list API returns high_24h and low_24h
- [ ] Admin API can set manual test data
- [ ] Admin API can reset data to auto-sync mode
- [ ] Auto-sync service respects data_source field
- [ ] Postman collections updated with test cases
- [ ] All tests passing
- [ ] Documentation updated

## Notes for Deployment

1. **Database Migration**: Run `add_token_market_24h_high_low.sql` before deploying new code
2. **Backward Compatibility**: Existing API clients will receive new fields (non-breaking change)
3. **Admin Access**: Ensure admin endpoints are properly authenticated in production
4. **CoinGecko API**: Verify CoinGecko API includes `usd_24h_high` and `usd_24h_low` fields
5. **Data Source**: Monitor logs for "Skipping sync" messages to verify manual override works

## Rollback Plan

If issues occur:
1. Admin endpoints can be disabled via routing without code changes
2. New fields default to 0, won't break existing queries
3. To remove fields: `ALTER TABLE token_market_cache DROP COLUMN high_24h, DROP COLUMN low_24h;`
