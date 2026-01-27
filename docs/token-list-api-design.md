# 代币列表 API 实现方案

## 需求概述

实现一个代币列表页面，展示 Injective Chain 上部署的所有代币合约信息，包括：
- 代币基本信息（名称、符号、标签、合约地址）
- 价格信息（当前价格、24h涨跌幅）
- 市场数据（24h交易量、市值、持有者数量）
- 元数据（首次部署时间）
- 支持搜索、筛选、排序、分页

## 现有系统分析

### 现有接口

| 接口 | 路径 | 提供数据 |
|------|------|----------|
| `ListContractInfos` | `/api/v1/contract/info/list` | 代币基础信息 |
| `GetTokenPriceAndMarketCap` | `/api/v1/token/price-marketcap` | 价格、市值、涨跌幅 |

### 数据可用性对比

| 字段 | 需求 | 现有数据源 | 状态 |
|------|------|-----------|------|
| 代币名称 | ✅ | `token_name` (ContractInfo) | ✅ 已有 |
| 代币符号 | ✅ | `token_symbol` (ContractInfo) | ✅ 已有 |
| 标签（官方/稳定币/跨链资产） | ✅ | `tags` (ContractInfo) | ✅ 已有 |
| 合约地址 | ✅ | `contract_address` (ContractInfo) | ✅ 已有 |
| 当前价格 | ✅ | `price_usd` (CoinGecko) | ✅ 已有 |
| 24h涨跌幅 | ✅ | `price_change_24h` (CoinGecko) | ✅ 已有 |
| 市值 | ✅ | `market_cap_usd` (CoinGecko) | ✅ 已有 |
| 24h交易量 | ✅ | `total_volume` (CoinGecko) | ❌ 需扩展 |
| 持有者数量 | ✅ | - | ❌ 需新增 |
| 首次部署时间 | ✅ | `created_at` (临时) | ⚠️ 需优化 |
| 搜索/分页 | ✅ | ListContractInfos | ✅ 已有 |

### 结论

**需要新增接口**。现有接口无法满足需求的原因：
1. 缺少关键数据：交易量、持有者数量需要新增数据源
2. 需要数据聚合：前端需要一个接口返回完整数据，避免多次调用
3. 性能考虑：组合调用会导致 N+1 查询问题

---

## 技术方案

### 方案概述

**新增接口：`GET /api/v1/tokens/list`**

这个接口会聚合多个数据源，一次性返回代币列表所需的所有数据。

### 数据源策略

#### 1. 基础信息（已有）
- **来源**：`contract_infos` 表
- **数据**：代币名称、符号、合约地址、标签、创建时间
- **查询**：ORM 查询，支持模糊搜索和标签过滤

#### 2. 价格/市值/涨跌幅（已有）
- **来源**：CoinGecko API（通过现有 `GetTokenPriceAndMarketCap` 服务）
- **当前获取**：
  - `price_usd` - 当前价格（USD）
  - `market_cap_usd` - 市值
  - `price_change_24h` - 24h价格变化百分比
  - `total_supply` - 总供应量
- **需扩展**：
  - `total_volume` - 24h交易量（CoinGecko 原生支持）

#### 3. 持有者数量（新增）
**方案A（推荐）：CoinGecko API**
- CoinGecko 提供部分代币的持有者数据
- 字段：`market_data.total_holders` 或类似字段
- 优点：已有集成，无需额外服务
- 缺点：并非所有代币都有此数据

**方案B：链上分析服务**
- 扫描 ERC-20 Transfer 事件，统计唯一地址数
- 实现：
  1. 监听链上 Transfer 事件
  2. 使用 Redis Set 或数据库表存储持有者地址
  3. 定期更新统计（每小时或每天）
- 优点：数据完整准确
- 缺点：需要额外开发工作

**方案C：第三方 API**
- 使用专业的链上数据分析服务（如 Dune Analytics、Nansen）
- 优点：数据专业准确
- 缺点：可能需要付费

**建议**：先使用方案A快速上线，逐步实现方案B作为补充。

#### 4. 首次部署时间
**方案A（临时）：使用 `created_at`**
- 使用 `contract_infos.created_at` 作为首次部署时间
- 优点：无需额外开发
- 缺点：不准确（创建时间 ≠ 部署时间）

**方案B（准确）：链上查询**
- 查询合约创建交易的时间戳
- 实现：通过 `eth_getTransactionByContract` 或类似 RPC 方法
- 优点：数据准确
- 缺点：需要额外的 RPC 调用

**建议**：先用方案A，有时间再优化为方案B。

---

## API 设计

### Proto 定义

```protobuf
// api/explorer/v1/token_list.proto

syntax = "proto3";

package explorer.v1;

import "google/api/annotations.proto";
import "explorer/v1/contract_info.proto"; // 复用现有类型

option go_package = "injective-explorer/api/explorer/v1";

// TokenListService provides token list with market data
service TokenListService {
  // ListTokens returns a paginated list of tokens with market data
  rpc ListTokens(ListTokensRequest) returns (Reply) {
    option (google.api.http) = {
      get : "/api/v1/tokens/list"
    };
  }
}

// ListTokensRequest defines request parameters for listing tokens
message ListTokensRequest {
  // Chain ID filter (optional, default: injective-1)
  string chain_id = 1 [ json_name = "chain_id" ];

  // Contract type filter (optional, e.g., "erc20", "erc721")
  string contract_type = 2 [ json_name = "contract_type" ];

  // Search by token name, symbol, or contract address (optional, fuzzy match)
  string search = 3 [ json_name = "search" ];

  // Sort field: market_cap, volume_24h, price, holders, deployment_time (optional)
  string sort_by = 4 [ json_name = "sort_by" ];

  // Sort order: asc or desc (optional, default: desc)
  string sort_order = 5 [ json_name = "sort_order" ];

  // Filter by tag type (optional, e.g., "stablecoin", "official")
  string tag_type = 6 [ json_name = "tag_type" ];

  // Filter by verification status (optional, e.g., "verified", "unverified")
  string verification_status = 7 [ json_name = "verification_status" ];

  // Pagination parameters
  PaginationRequest pagination = 8 [ json_name = "pagination" ];
}

// TokenListItemDTO represents a single token in the list
message TokenListItemDTO {
  // Basic information
  uint64 id = 1 [ json_name = "id" ];
  string contract_address = 2 [ json_name = "contract_address" ];
  string token_name = 3 [ json_name = "token_name" ];
  string token_symbol = 4 [ json_name = "token_symbol" ];
  repeated ContractTagInfoDTO tags = 5 [ json_name = "tags" ];

  // Market data
  double price_usd = 6 [ json_name = "price_usd" ];
  double price_change_24h = 7 [ json_name = "price_change_24h" ];
  double total_volume_24h = 8 [ json_name = "total_volume_24h" ];
  double market_cap_usd = 9 [ json_name = "market_cap_usd" ];
  int64 holders_count = 10 [ json_name = "holders_count" ];

  // Metadata
  string first_deployment_time = 11 [ json_name = "first_deployment_time" ]; // Format: "2024/03/15"
  string created_at = 12 [ json_name = "created_at" ];                      // ISO 8601 format
}

// TokenListResponse represents the paginated response
message TokenListResponse {
  repeated TokenListItemDTO items = 1 [ json_name = "items" ];
  int32 page = 2 [ json_name = "page" ];
  int32 page_size = 3 [ json_name = "page_size" ];
  int64 total = 4 [ json_name = "total" ];
  bool is_exact_count = 5 [ json_name = "is_exact_count" ];
}
```

### API 请求示例

```bash
# 基础请求
GET /api/v1/tokens/list?page=1&page_size=20

# 搜索 USDT
GET /api/v1/tokens/list?search=USDT&page=1&page_size=20

# 按市值排序
GET /api/v1/tokens/list?sort_by=market_cap&sort_order=desc&page=1&page_size=20

# 筛选稳定币
GET /api/v1/tokens/list?tag_type=stablecoin&page=1&page_size=20

# 组合查询：搜索、排序、筛选
GET /api/v1/tokens/list?search=USD&sort_by=volume_24h&sort_order=desc&tag_type=stablecoin&page=1&page_size=20
```

### API 响应示例

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "items": [
      {
        "id": 1,
        "contract_address": "0x8a3d...7f2e",
        "token_name": "Tether USD",
        "token_symbol": "USDT",
        "tags": [
          {
            "tag_type": "category",
            "tag_value": "官方",
            "tag_color": "#28a745"
          },
          {
            "tag_type": "category",
            "tag_value": "稳定币",
            "tag_color": "#007bff"
          }
        ],
        "price_usd": 1.00,
        "price_change_24h": 0.02,
        "total_volume_24h": 53200000.00,
        "market_cap_usd": 850500000.00,
        "holders_count": 125832,
        "first_deployment_time": "2024/03/15",
        "created_at": "2024-03-15T10:30:00Z"
      },
      {
        "id": 2,
        "contract_address": "0x5b2a...3c8d",
        "token_name": "USD Coin",
        "token_symbol": "USDC",
        "tags": [
          {
            "tag_type": "category",
            "tag_value": "官方",
            "tag_color": "#28a745"
          },
          {
            "tag_type": "category",
            "tag_value": "稳定币",
            "tag_color": "#007bff"
          }
        ],
        "price_usd": 0.9998,
        "price_change_24h": -0.01,
        "total_volume_24h": 42800000.00,
        "market_cap_usd": 620300000.00,
        "holders_count": 98456,
        "first_deployment_time": "2024/03/18",
        "created_at": "2024-03-18T14:20:00Z"
      }
    ],
    "page": 1,
    "page_size": 20,
    "total": 156,
    "is_exact_count": true
  }
}
```

---

## 实现步骤

### Step 1: 扩展 CoinGecko API 数据获取

修改 `internal/service/explorer.go` 中的 `GetTokenPriceAndMarketCap` 方法，额外获取交易量数据：

```go
// 扩展响应结构
message TokenPriceAndMarketCapResponse {
  string token_symbol = 1 [ json_name = "token_symbol" ];
  string token_name = 2 [ json_name = "token_name" ];
  double price_usd = 3 [ json_name = "price_usd" ];
  string total_supply = 4 [ json_name = "total_supply" ];
  string total_supply_formatted = 5 [ json_name = "total_supply_formatted" ];
  double market_cap_usd = 6 [ json_name = "market_cap_usd" ];
  double price_change_24h = 7 [ json_name = "price_change_24h" ];

  // 新增字段
  double total_volume_24h = 8 [ json_name = "total_volume_24h" ];    // 24h交易量
  int64 holders_count = 9 [ json_name = "holders_count" ];          // 持有者数量

  int64 last_updated = 10 [ json_name = "last_updated" ];
  string price_source = 11 [ json_name = "price_source" ];
  string supply_source = 12 [ json_name = "supply_source" ];
  string total_supply_from_chain = 13 [ json_name = "total_supply_from_chain" ];
  string total_supply_from_chain_formatted = 14 [ json_name = "total_supply_from_chain_formatted" ];
}
```

修改 CoinGecko API 调用：

```go
// 在 fetchTokenPriceFromCoinGecko 方法中
type CoinGeckoMarketData struct {
    CurrentPrice              map[string]float64 `json:"current_price"`
    MarketCap                 map[string]float64 `json:"market_cap"`
    TotalVolume               map[string]float64 `json:"total_volume"`               // 新增
    PriceChangePercentage24h  map[string]float64 `json:"price_change_percentage_24h"`
    TotalSupply               float64            `json:"total_supply"`
    Holders                   int64              `json:"total_holders"`              // 新增
    LastUpdated               string            `json:"last_updated"`
}

// 填充响应
response.TotalVolume24H = marketData.TotalVolume["usd"]
if marketData.Holders > 0 {
    response.HoldersCount = marketData.Holders
} else {
    response.HoldersCount = 0 // 或者不返回此字段
}
```

### Step 2: 创建 Proto 文件并生成代码

```bash
# 创建 proto 文件
vi api/explorer/v1/token_list.proto

# 生成代码
make api
```

### Step 3: 实现服务层

创建 `internal/service/token_list.go`：

```go
package service

import (
    "context"
    "net/http"
    "sort"
    "sync"
    "time"

    pb "injective-explorer/api/explorer/v1"
    "injective-explorer/internal/global"
    "injective-explorer/internal/models"
    "injective-explorer/provider/db"

    "github.com/bytedance/sonic"
    "github.com/pkg/errors"
    "gorm.io/gorm"
)

type TokenListService struct {
    pb.UnimplementedTokenListServiceServer
    explorerService *ExplorerService
}

func NewTokenListService(explorerService *ExplorerService) *TokenListService {
    return &TokenListService{
        explorerService: explorerService,
    }
}

func (s *TokenListService) ListTokens(ctx context.Context, req *pb.ListTokensRequest) (*pb.Reply, error) {
    // 1. 参数验证
    if req == nil {
        return ErrorReply(http.StatusBadRequest, "request cannot be nil", errors.New("nil request")), nil
    }

    // 2. 设置默认值
    chainID := req.ChainId
    if chainID == "" {
        chainID = DefaultChainID
    }

    sortField := req.SortBy
    if sortField == "" {
        sortField = "market_cap" // 默认按市值排序
    }

    sortOrder := req.SortOrder
    if sortOrder != "asc" && sortOrder != "desc" {
        sortOrder = "desc" // 默认降序
    }

    // 3. 归一化分页参数
    var pagination *pb.PaginationRequest
    if req.Pagination != nil {
        pagination = req.Pagination
    } else {
        pagination = &pb.PaginationRequest{
            Page:     1,
            PageSize: DefaultPageSize,
        }
    }

    page, pageSize, offset, _, _ := normalizePagination(
        pagination.Page,
        pagination.PageSize,
        pagination.Cursor,
        MaxPageSize,
    )

    // 4. 构建查询
    queryCtx, queryCancel := context.WithTimeout(context.Background(), DBQueryTimeout)
    defer queryCancel()

    query := db.Get().WithContext(queryCtx).
        Model(&models.ContractInfo{}).
        Preload("Tags").
        Where("token_symbol IS NOT NULL AND token_symbol != ''")

    // 应用过滤条件
    if req.ChainId != "" {
        query = query.Where("chain_id = ?", req.ChainId)
    }

    if req.ContractType != "" {
        query = query.Where("contract_type = ?", req.ContractType)
    }

    if req.VerificationStatus != "" {
        query = query.Where("verification_status = ?", req.VerificationStatus)
    }

    if req.Search != "" {
        searchPattern := "%" + req.Search + "%"
        query = query.Where(
            "token_name LIKE ? OR token_symbol LIKE ? OR contract_address LIKE ?",
            searchPattern, searchPattern, searchPattern,
        )
    }

    if req.TagType != "" {
        query = query.Joins(
            "INNER JOIN contract_tags ON contract_tags.contract_address = contract_infos.contract_address AND contract_tags.chain_id = contract_infos.chain_id",
        ).Where("contract_tags.tag_type = ?", req.TagType)
    }

    // 5. 获取总数
    var total int64
    var isExactCount bool
    var countErr error
    total, isExactCount, countErr = getApproximateCountWithError(query, &models.ContractInfo{}, queryCtx)
    if countErr != nil {
        global.Logger.Warnf("failed to get count for tokens: %v", countErr)
        total = ApproximateCountFallback
        isExactCount = false
    }

    // 6. 执行查询（先获取更多数据用于排序）
    // 如果按市场数据排序，需要先获取更多数据
    fetchSize := pageSize
    if sortField == "market_cap" || sortField == "volume_24h" || sortField == "price" || sortField == "holders" {
        fetchSize = pageSize * 3 // 获取3倍数据用于排序
    }

    query = query.Offset(offset).Limit(int(fetchSize)).Order("created_at DESC")

    var contracts []models.ContractInfo
    err := query.Find(&contracts).Error
    if err != nil {
        global.Logger.Errorf("failed to query tokens: %v", err)
        return ErrorReply(http.StatusInternalServerError, "failed to query tokens", err), err
    }

    // 7. 批量获取市场数据
    tokenSymbols := extractTokenSymbols(contracts)
    marketDataMap := s.batchGetMarketData(ctx, tokenSymbols)

    // 8. 组装返回数据
    items := make([]*pb.TokenListItemDTO, 0, len(contracts))
    for _, contract := range contracts {
        marketData := marketDataMap[contract.TokenSymbol]

        item := &pb.TokenListItemDTO{
            Id:                 uint64(contract.ID),
            ContractAddress:    contract.ContractAddress,
            TokenName:          contract.TokenName,
            TokenSymbol:        contract.TokenSymbol,
            Tags:               models.ToContractTagInfoDTOs(contract.Tags),
            FirstDeploymentTime: formatDeploymentTime(contract.CreatedAt),
            CreatedAt:          contract.CreatedAt.Format(time.RFC3339),
        }

        // 填充市场数据
        if marketData != nil {
            item.PriceUsd = marketData.PriceUsd
            item.PriceChange24H = marketData.PriceChange24H
            item.TotalVolume24H = marketData.TotalVolume24H
            item.MarketCapUsd = marketData.MarketCapUsd
            item.HoldersCount = marketData.HoldersCount
        }

        items = append(items, item)
    }

    // 9. 排序（如果按市场数据排序）
    if sortField == "market_cap" || sortField == "volume_24h" || sortField == "price" || sortField == "holders" {
        items = sortTokenItems(items, sortField, sortOrder)
        // 截取到请求的页面大小
        if len(items) > int(pageSize) {
            items = items[:pageSize]
        }
    }

    // 10. 构建分页响应
    response := &pb.TokenListResponse{
        Items:         items,
        Page:          page,
        PageSize:      pageSize,
        Total:         total,
        IsExactCount:  isExactCount,
    }

    return MarshalAndReply(response, "marshal token list error")
}

// extractTokenSymbols 提取代币符号列表
func extractTokenSymbols(contracts []models.ContractInfo) []string {
    symbols := make([]string, 0, len(contracts))
    seen := make(map[string]bool)

    for _, contract := range contracts {
        if contract.TokenSymbol != "" && !seen[contract.TokenSymbol] {
            symbols = append(symbols, contract.TokenSymbol)
            seen[contract.TokenSymbol] = true
        }
    }

    return symbols
}

// TokenMarketData 市场数据
type TokenMarketData struct {
    PriceUsd       float64
    MarketCapUsd   float64
    PriceChange24H float64
    TotalVolume24H float64
    HoldersCount   int64
}

// batchGetMarketData 批量获取市场数据（并发优化）
func (s *TokenListService) batchGetMarketData(ctx context.Context, symbols []string) map[string]*TokenMarketData {
    result := make(map[string]*TokenMarketData)
    var wg sync.WaitGroup
    var mu sync.Mutex

    for _, symbol := range symbols {
        wg.Add(1)
        go func(sym string) {
            defer wg.Done()

            // 调用现有的 GetTokenPriceAndMarketCap 服务
            req := &pb.GetTokenPriceAndMarketCapRequest{TokenSymbol: sym}
            reply, err := s.explorerService.GetTokenPriceAndMarketCap(ctx, req)

            if err != nil {
                global.Logger.Warnf("failed to get market data for %s: %v", sym, err)
                return
            }

            if reply.Code != 200 {
                global.Logger.Warnf("non-200 response for %s: %d", sym, reply.Code)
                return
            }

            var data pb.TokenPriceAndMarketCapResponse
            if err := sonic.Unmarshal([]byte(reply.Data), &data); err != nil {
                global.Logger.Warnf("failed to unmarshal market data for %s: %v", sym, err)
                return
            }

            mu.Lock()
            result[sym] = &TokenMarketData{
                PriceUsd:       data.PriceUsd,
                MarketCapUsd:   data.MarketCapUsd,
                PriceChange24H: data.PriceChange24H,
                TotalVolume24H: data.TotalVolume24H,
                HoldersCount:   data.HoldersCount,
            }
            mu.Unlock()
        }(symbol)
    }

    wg.Wait()
    return result
}

// sortTokenItems 排序代币列表
func sortTokenItems(items []*pb.TokenListItemDTO, sortField, sortOrder string) []*pb.TokenListItemDTO {
    sort.Slice(items, func(i, j int) bool {
        var iValue, jValue float64

        switch sortField {
        case "market_cap":
            iValue = items[i].MarketCapUsd
            jValue = items[j].MarketCapUsd
        case "volume_24h":
            iValue = items[i].TotalVolume24H
            jValue = items[j].TotalVolume24H
        case "price":
            iValue = items[i].PriceUsd
            jValue = items[j].PriceUsd
        case "holders":
            iValue = float64(items[i].HoldersCount)
            jValue = float64(items[j].HoldersCount)
        default:
            return false
        }

        if sortOrder == "asc" {
            return iValue < jValue
        }
        return iValue > jValue
    })

    return items
}

// formatDeploymentTime 格式化部署时间为 "YYYY/MM/DD" 格式
func formatDeploymentTime(t time.Time) string {
    return t.Format("2006/01/02")
}
```

### Step 4: 注册服务

修改 `internal/server/grpc.go`：

```go
import (
    pb "injective-explorer/api/explorer/v1"
    "injective-explorer/internal/service"
)

func RegisterGRPCServer(s *grpc.Server) {
    // ... 现有注册代码 ...

    // 注册 TokenListService
    explorerService := service.NewExplorerService()
    tokenListService := service.NewTokenListService(explorerService)
    pb.RegisterTokenListServiceServer(s, tokenListService)
}
```

### Step 5: 添加缓存（可选）

为了提高性能，可以添加 Redis 缓存：

```go
// 在 token_list.go 中添加缓存逻辑

const (
    CacheKeyTokenList      = "token_list:%s:%s:%s"  // chain_id:filters:page
    CacheExpireTokenList   = 5 * time.Minute
)

func (s *TokenListService) ListTokens(ctx context.Context, req *pb.ListTokensRequest) (*pb.Reply, error) {
    // ... 现有代码 ...

    // 尝试从缓存获取
    cacheKey := fmt.Sprintf(CacheKeyTokenList, chainID, buildCacheKeySuffix(req), page)
    if cachedData := s.getFromCache(ctx, cacheKey); cachedData != nil {
        return cachedData, nil
    }

    // ... 查询数据 ...

    // 缓存结果
    go s.setToCache(context.Background(), cacheKey, response)

    return MarshalAndReply(response, "marshal token list error")
}
```

---

## 实现优先级

| 优先级 | 功能 | 实现方式 | 工作量 | 说明 |
|--------|------|----------|--------|------|
| P0 | 创建新接口 `/api/v1/tokens/list` | Proto + Service | 1天 | 基础框架 |
| P0 | 扩展 CoinGecko API 获取交易量 | 修改现有服务 | 0.5天 | 扩展响应字段 |
| P0 | 实现基础列表查询 | ORM 查询 + 分页 | 0.5天 | 核心功能 |
| P0 | 批量获取市场数据 | 并发调用 | 0.5天 | 性能优化 |
| P1 | 搜索和筛选 | 模糊搜索 + 标签过滤 | 0.5天 | 用户体验 |
| P1 | 排序功能 | 市场数据排序 | 0.5天 | 用户体验 |
| P2 | 持有者数量 | CoinGecko API | 1天 | 数据完整 |
| P2 | 持有者统计（链上） | 新增服务 | 2-3天 | 数据准确 |
| P3 | 首次部署时间（准确） | 链上查询 | 1-2天 | 数据优化 |
| P3 | 缓存优化 | Redis | 0.5天 | 性能优化 |

**总计工作量：3-5天（P0-P1），7-10天（包含 P2-P3）**

---

## 快速启动方案

如果需要快速上线，建议按以下步骤实施：

### 第一阶段（1-2天）：核心功能

1. 创建 `token_list.proto` 并生成代码
2. 实现 `TokenListService.ListTokens` 基础功能
3. 扩展 CoinGecko API 获取 `total_volume`
4. 实现基础查询 + 分页

**临时方案**：
- 持有者数量：返回 `0` 或不显示
- 首次部署时间：使用 `created_at`

### 第二阶段（1-2天）：增强功能

1. 实现搜索和筛选功能
2. 实现排序功能
3. 添加缓存优化

### 第三阶段（2-3天）：数据完善

1. 从 CoinGecko 获取持有者数据
2. 优化首次部署时间（链上查询）
3. 性能优化和测试

### 第四阶段（可选）：高级功能

1. 实现链上持有者统计服务
2. 实时数据更新
3. 更多排序和筛选选项

---

## 性能考虑

### 1. 批量查询市场数据

**问题**：每个代币都调用 CoinGecko API 会导致大量请求

**解决方案**：
- 使用 goroutine 并发请求
- 限制并发数量（如 10 个并发）
- 添加请求速率限制

```go
// 使用带缓冲的 channel 限制并发
maxConcurrency := 10
semaphore := make(chan struct{}, maxConcurrency)

for _, symbol := range symbols {
    semaphore <- struct{}{} // 获取信号量
    go func(sym string) {
        defer func() { <-semaphore }() // 释放信号量
        // 获取市场数据
    }(symbol)
}
```

### 2. 缓存策略

- **列表缓存**：缓存 5 分钟（带分页和过滤参数）
- **市场数据缓存**：复用现有的 `GetTokenPriceAndMarketCap` 缓存
- **标签缓存**：复用现有的标签缓存

### 3. 数据库查询优化

- 添加适当的索引：
  - `token_symbol` 索引
  - `contract_address` 索引
  - `(chain_id, contract_type)` 组合索引
  - `created_at` 索引（用于排序）

```sql
CREATE INDEX idx_contract_infos_token_symbol ON contract_infos(token_symbol);
CREATE INDEX idx_contract_infos_chain_type ON contract_infos(chain_id, contract_type);
CREATE INDEX idx_contract_infos_created_at ON contract_infos(created_at DESC);
```

### 4. 排序优化

- 如果按市场数据排序，需要在应用层排序
- 对于大数据量，考虑预计算排序字段并存储

---

## 数据库优化

### 建议的索引

```sql
-- 基础查询优化
CREATE INDEX IF NOT EXISTS idx_contract_infos_token_symbol
  ON contract_infos(token_symbol)
  WHERE token_symbol IS NOT NULL AND token_symbol != '';

CREATE INDEX IF NOT EXISTS idx_contract_infos_chain_type
  ON contract_infos(chain_id, contract_type);

CREATE INDEX IF NOT EXISTS idx_contract_infos_verification
  ON contract_infos(verification_status);

-- 搜索优化（全文搜索）
-- CREATE INDEX idx_contract_infos_fulltext
--   ON contract_infos USING gin(to_tsvector('english', token_name || ' ' || token_symbol));

-- 标签关联查询优化
CREATE INDEX IF NOT EXISTS idx_contract_tags_contract
  ON contract_tags(chain_id, contract_address, tag_type);
```

### 预计算选项（可选）

对于高频查询，可以定期预计算市场数据并缓存：

```sql
-- 创建市场数据缓存表
CREATE TABLE token_market_cache (
    id BIGSERIAL PRIMARY KEY,
    chain_id VARCHAR(50) NOT NULL,
    token_symbol VARCHAR(50) NOT NULL,
    price_usd DECIMAL(30, 6),
    market_cap_usd DECIMAL(30, 6),
    volume_24h_usd DECIMAL(30, 6),
    price_change_24h DECIMAL(10, 4),
    holders_count BIGINT,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(chain_id, token_symbol)
);

-- 定时任务更新缓存（每5分钟）
```

---

## 监控和日志

### 关键指标

- API 响应时间
- CoinGecko API 调用次数和成功率
- 缓存命中率
- 数据库查询性能

### 日志记录

```go
global.Logger.Infof("token list request: chain_id=%s, search=%s, sort_by=%s, page=%d",
    chainID, req.Search, req.SortBy, page)

global.Logger.Debugf("fetched %d tokens, %d with market data",
    len(contracts), len(marketDataMap))
```

---

## 前端集成

### API 调用示例

```typescript
interface TokenListItem {
  id: number;
  contract_address: string;
  token_name: string;
  token_symbol: string;
  tags: ContractTagInfoDTO[];
  price_usd: number;
  price_change_24h: number;
  total_volume_24h: number;
  market_cap_usd: number;
  holders_count: number;
  first_deployment_time: string;
  created_at: string;
}

interface TokenListResponse {
  items: TokenListItem[];
  page: number;
  page_size: number;
  total: number;
  is_exact_count: boolean;
}

async function getTokenList(params: {
  page?: number;
  page_size?: number;
  search?: string;
  sort_by?: string;
  sort_order?: string;
  tag_type?: string;
}): Promise<TokenListResponse> {
  const queryParams = new URLSearchParams();
  if (params.page) queryParams.append('page', params.page.toString());
  if (params.page_size) queryParams.append('page_size', params.page_size.toString());
  if (params.search) queryParams.append('search', params.search);
  if (params.sort_by) queryParams.append('sort_by', params.sort_by);
  if (params.sort_order) queryParams.append('sort_order', params.sort_order);
  if (params.tag_type) queryParams.append('tag_type', params.tag_type);

  const response = await fetch(`/api/v1/tokens/list?${queryParams.toString()}`);
  const data = await response.json();

  if (data.code !== 200) {
    throw new Error(data.message);
  }

  return data.data;
}
```

### 显示格式化

```typescript
// 价格格式化
function formatPrice(price: number): string {
  if (price >= 1000) {
    return `$${price.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
  } else if (price >= 1) {
    return `$${price.toFixed(2)}`;
  } else {
    return `$${price.toFixed(4)}`;
  }
}

// 涨跌幅显示
function formatPriceChange(change: number): { text: string; color: string } {
  const text = `${change >= 0 ? '+' : ''}${change.toFixed(2)}%`;
  const color = change >= 0 ? '#28a745' : '#dc3545';
  return { text, color };
}

// 市值/交易量格式化
function formatLargeNumber(value: number): string {
  if (value >= 1e9) {
    return `$${(value / 1e9).toFixed(1)}B`;
  } else if (value >= 1e6) {
    return `$${(value / 1e6).toFixed(1)}M`;
  } else if (value >= 1e3) {
    return `$${(value / 1e3).toFixed(1)}K`;
  }
  return `$${value.toLocaleString()}`;
}

// 持有者数量格式化
function formatHolders(count: number): string {
  return count.toLocaleString();
}

// 合约地址缩略
function formatAddress(address: string): string {
  if (address.length <= 10) return address;
  return `${address.slice(0, 6)}...${address.slice(-4)}`;
}
```

---

## 测试计划

### 单元测试

```go
func TestTokenListService_ListTokens(t *testing.T) {
    // 测试用例：
    // 1. 基础查询
    // 2. 带搜索的查询
    // 3. 带排序的查询
    // 4. 带标签过滤的查询
    // 5. 分页功能
}
```

### 集成测试

```go
func TestTokenListAPI(t *testing.T) {
    // 启动测试服务器
    // 调用 API
    // 验证响应格式
    // 验证数据正确性
}
```

### 性能测试

```bash
# 使用 ab 或 wrk 进行压力测试
ab -n 1000 -c 10 "http://localhost:8080/api/v1/tokens/list?page=1&page_size=20"

wrk -t4 -c100 -d30s --latency "http://localhost:8080/api/v1/tokens/list?page=1&page_size=20"
```

---

## 后续优化方向

1. **实时更新**：使用 WebSocket 推送价格更新
2. **更多筛选条件**：价格区间、市值区间等
3. **导出功能**：导出代币列表为 CSV/Excel
4. **历史数据**：代币价格历史图表
5. **更多排序选项**：按创建时间、按价格变化等
6. **智能推荐**：根据用户行为推荐代币
7. **多语言支持**：i18n 国际化

---

## 总结

本方案通过新增 `/api/v1/tokens/list` 接口，聚合多个数据源，实现了完整的代币列表功能。主要工作包括：

1. **扩展 CoinGecko API**：获取 24h 交易量数据
2. **新增 TokenListService**：实现代币列表查询
3. **批量获取市场数据**：使用并发优化性能
4. **实现排序和筛选**：提供良好的用户体验
5. **添加缓存**：优化 API 响应速度

通过分阶段实施，可以快速上线核心功能，然后逐步完善数据准确性和性能。
