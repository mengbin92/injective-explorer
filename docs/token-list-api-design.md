# 代币列表 API 实现方案（优化版）

## 需求概述

实现一个代币列表页面，展示 Injective Chain 上部署的所有代币合约信息，包括：
- 代币基本信息（名称、符号、标签、合约地址）
- 价格信息（当前价格、24h涨跌幅）
- 市场数据（24h交易量、市值、持有者数量）
- 元数据（首次部署时间）
- 支持搜索、筛选、排序、分页

### UI 需求分析（基于设计稿）

根据前端设计稿，代币列表页面需要展示以下字段：

| 列名 | 字段 | 数据类型 | 格式要求 | 说明 |
|------|------|---------|---------|------|
| # | 序号 | number | - | 自动递增 |
| 代币 | token_name, token_symbol, tags, contract_address | string, string, array, string | 地址需缩略显示（0x8a3d...7f2e） | 支持多标签显示 |
| 价格 | price_usd, price_change_24h | number, number | $X.XX, ±X.XX% | 涨跌用颜色区分 |
| 24h 交易量 | total_volume_24h | number | $X.XM / $X.XB | 自动格式化 |
| 市值 | market_cap_usd | number | $X.XM / $X.XB | 自动格式化 |
| 持有者 | holders_count | number | X,XXX | 千分位格式化 |
| 首次部署 | first_deployment_time | string | YYYY/MM/DD | 日期格式 |

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

#### 2. 价格/市值/涨跌幅（已有，需扩展）
- **来源**：CoinGecko API（通过现有 `GetTokenPriceAndMarketCap` 服务）
- **当前获取**：
  - `price_usd` - 当前价格（USD）
  - `market_cap_usd` - 市值
  - `price_change_24h` - 24h价格变化百分比
  - `total_supply` - 总供应量
- **需扩展**：
  - `total_volume_24h` - 24h交易量（CoinGecko API 的 `market_data.total_volume.usd`）
  - `holders_count` - 持有者数量（CoinGecko API 的 `market_data.holders`，可选）

**实现要点**：
- CoinGecko 的 `/coins/{id}` 接口返回的 `market_data` 包含：
  - `total_volume.usd` - 24h交易量（USD）
  - `holders` - 持有者数量（部分代币支持）
- 需要在 `GetTokenTotalSupplyFromCoinGecko` 中同时获取这些数据，避免多次调用

#### 3. 持有者数量（新增）
**方案A（推荐）：CoinGecko API**
- CoinGecko 的 `/coins/{id}` 接口在 `market_data` 中提供 `holders` 字段
- 优点：已有集成，无需额外服务，数据更新及时
- 缺点：并非所有代币都有此数据（部分小币种可能缺失）
- **实现**：在 `GetTokenTotalSupplyFromCoinGecko` 中扩展，同时获取 `market_data.holders`

**方案B：链上分析服务（长期方案）**
- 扫描 ERC-20/CW20 Transfer 事件，统计唯一地址数
- 实现步骤：
  1. 监听链上 Transfer 事件（通过区块扫描或事件订阅）
  2. 使用数据库表 `token_holders` 存储：`(chain_id, contract_address, holder_address, balance, updated_at)`
  3. 定期更新统计（每小时或每天）
  4. 提供 API 查询接口：`SELECT COUNT(DISTINCT holder_address) FROM token_holders WHERE ...`
- 优点：数据完整准确，覆盖所有代币
- 缺点：需要额外开发工作，需要存储空间

**方案C：第三方 API（备选）**
- 使用专业的链上数据分析服务（如 Dune Analytics、Nansen、The Graph）
- 优点：数据专业准确，无需维护
- 缺点：可能需要付费，API调用限制

**建议**：
1. **第一阶段**：使用方案A快速上线，对于缺失数据的代币返回 `0` 或不显示
2. **第二阶段**：实现方案B，逐步迁移到链上数据
3. **降级策略**：如果 CoinGecko 数据不可用，持有者数量字段返回 `null`，前端显示为 "-"

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

**注意**：此步骤已在"数据源策略"章节详细说明，这里提供具体的实现代码。

#### 1.1 扩展 Proto 定义（已完成，见上文）

#### 1.2 扩展 PriceAndSupplyData 结构（已完成，见上文）

#### 1.3 修改 GetTokenTotalSupplyFromCoinGecko 方法

在 `internal/service/price.go` 中修改 `GetTokenTotalSupplyFromCoinGecko`，同时获取交易量和持有者数据：

```go
var coinDetailResp struct {
    MarketData struct {
        TotalSupply       *float64 `json:"total_supply"`
        CirculatingSupply *float64 `json:"circulating_supply"`
        TotalVolume       struct {
            USD float64 `json:"usd"`
        } `json:"total_volume"`
        Holders *int64 `json:"holders"`  // 注意：可能为 null
    } `json:"market_data"`
}

// 在解析后，提取交易量和持有者数据
totalVolume24H := coinDetailResp.MarketData.TotalVolume.USD
holdersCount := int64(0)
if coinDetailResp.MarketData.Holders != nil {
    holdersCount = *coinDetailResp.MarketData.Holders
}
```

**建议**：创建一个新的方法 `GetTokenMarketDataFromCoinGecko` 来统一获取所有市场数据（价格、供应量、交易量、持有者），避免多次调用 CoinGecko API。

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

// batchGetMarketData 批量获取市场数据（并发优化，带限流）
func (s *TokenListService) batchGetMarketData(ctx context.Context, symbols []string) map[string]*TokenMarketData {
    result := make(map[string]*TokenMarketData)
    var wg sync.WaitGroup
    var mu sync.Mutex
    
    // 限制并发数量，避免过多请求导致 CoinGecko API 限流
    maxConcurrency := 10
    semaphore := make(chan struct{}, maxConcurrency)

    for _, symbol := range symbols {
        wg.Add(1)
        go func(sym string) {
            defer wg.Done()
            
            // 获取信号量
            semaphore <- struct{}{}
            defer func() { <-semaphore }()

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

**问题**：每个代币都调用 CoinGecko API 会导致大量请求和性能瓶颈

**解决方案**：

#### 方案A：并发请求 + 限流（当前实现）
- 使用 goroutine 并发请求
- 限制并发数量（10-20 个并发）
- 添加请求速率限制（避免触发 CoinGecko API 限流）
- **优点**：实现简单，复用现有服务
- **缺点**：仍需要多次 API 调用

#### 方案B：CoinGecko 批量查询 API（推荐优化）
- CoinGecko 支持批量查询多个代币：`/api/v3/simple/price?ids=bitcoin,ethereum&vs_currencies=usd&include_24hr_change=true&include_market_cap=true`
- 对于详细信息，可以使用 `/api/v3/coins/markets` 接口，支持批量查询
- **优点**：大幅减少 API 调用次数，提高性能
- **缺点**：需要修改现有服务逻辑

**推荐实现**（方案B）：

```go
// 批量获取市场数据（使用 CoinGecko 批量 API）
func batchGetMarketDataFromCoinGecko(ctx context.Context, coinGeckoIDs []string) (map[string]*TokenMarketData, error) {
    // 构建批量查询 URL（最多支持 100 个 ID）
    ids := strings.Join(coinGeckoIDs[:min(len(coinGeckoIDs), 100)], ",")
    apiURL := fmt.Sprintf(
        "https://api.coingecko.com/api/v3/coins/markets?ids=%s&vs_currency=usd&order=market_cap_desc&per_page=100&page=1&sparkline=false&price_change_percentage=24h",
        ids,
    )
    
    // 调用 API 并解析响应
    // 返回 map[coinGeckoID]*TokenMarketData
}
```

**注意**：
- CoinGecko 免费版限制：每分钟 10-50 次请求
- 建议添加请求缓存（Redis），缓存时间 5 分钟
- 对于超过 100 个代币的情况，需要分批查询

### 2. 缓存策略

#### 2.1 多层缓存架构

```
请求 → Redis 列表缓存 → 数据库查询 → CoinGecko API
       (5分钟)          (实时)        (5分钟缓存)
```

- **列表缓存**：
  - Key: `token_list:{chain_id}:{filters_hash}:{page}:{page_size}`
  - TTL: 5 分钟
  - 包含分页和过滤参数，确保不同查询条件独立缓存
  
- **市场数据缓存**：
  - Key: `token_market:{token_symbol}` 或 `token_market:{coinGeckoID}`
  - TTL: 5 分钟（价格数据变化频繁）
  - 复用现有的 `GetTokenPriceAndMarketCap` 缓存逻辑
  
- **标签缓存**：
  - Key: `contract_tags:{chain_id}:{contract_address}`
  - TTL: 1 小时（标签变化不频繁）
  - 复用现有的标签缓存

#### 2.2 缓存失效策略

- **主动失效**：当合约信息更新时，清除相关缓存
- **被动失效**：依赖 TTL，自动过期
- **部分失效**：只清除受影响代币的市场数据缓存，不影响整个列表缓存

#### 2.3 缓存预热（可选）

对于热门代币（如 USDT、USDC、WBTC），可以定期预热缓存：
```go
// 定时任务：每 4 分钟更新一次热门代币的市场数据
func warmupPopularTokensCache(ctx context.Context) {
    popularSymbols := []string{"USDT", "USDC", "WBTC", "WETH", "LINK"}
    for _, symbol := range popularSymbols {
        // 异步更新缓存
        go updateTokenMarketCache(ctx, symbol)
    }
}
```

### 3. 数据库查询优化

#### 3.1 索引优化

```sql
-- 基础查询索引
CREATE INDEX IF NOT EXISTS idx_contract_infos_token_symbol 
  ON contract_infos(token_symbol) 
  WHERE token_symbol IS NOT NULL AND token_symbol != '';

-- 组合索引（优化多条件查询）
CREATE INDEX IF NOT EXISTS idx_contract_infos_chain_type 
  ON contract_infos(chain_id, contract_type);

-- 排序索引
CREATE INDEX IF NOT EXISTS idx_contract_infos_created_at 
  ON contract_infos(created_at DESC);

-- 搜索索引（优化模糊查询）
CREATE INDEX IF NOT EXISTS idx_contract_infos_name_search 
  ON contract_infos USING gin(to_tsvector('english', COALESCE(token_name, '') || ' ' || COALESCE(token_symbol, '')));

-- 标签关联查询索引
CREATE INDEX IF NOT EXISTS idx_contract_tags_contract 
  ON contract_tags(chain_id, contract_address, tag_type);
```

#### 3.2 查询优化建议

1. **避免 N+1 查询**：使用 `Preload("Tags")` 一次性加载标签
2. **分页优化**：对于大数据量，使用游标分页（cursor-based pagination）代替偏移分页
3. **条件过滤**：在数据库层面完成过滤，减少内存占用
4. **只查询必要字段**：使用 `Select()` 指定需要的字段

```go
// 优化后的查询示例
query := db.Get().WithContext(ctx).
    Model(&models.ContractInfo{}).
    Select("id, contract_address, token_name, token_symbol, created_at").
    Preload("Tags", func(db *gorm.DB) *gorm.DB {
        return db.Select("tag_type, tag_value, tag_color")
    }).
    Where("token_symbol IS NOT NULL AND token_symbol != ''").
    Where("chain_id = ?", chainID)
```

### 4. 排序优化

#### 4.1 排序策略

**数据库字段排序**（快速）：
- `created_at` - 首次部署时间
- `token_symbol` - 代币符号（字母序）
- 这些字段可以直接在数据库层面排序

**市场数据排序**（需要应用层处理）：
- `market_cap` - 市值
- `volume_24h` - 24h交易量
- `price` - 价格
- `holders` - 持有者数量
- 这些数据来自 CoinGecko API，需要在应用层排序

#### 4.2 排序实现方案

**方案A：应用层排序（当前实现）**
- 先查询数据库获取代币列表
- 批量获取市场数据
- 在内存中排序
- **优点**：实现简单
- **缺点**：对于大数据量，需要获取更多数据才能保证排序准确

**方案B：预计算排序字段（推荐优化）**
- 创建 `token_market_cache` 表，定期更新市场数据
- 在数据库层面完成排序
- **优点**：性能好，支持大数据量
- **缺点**：需要额外的数据同步逻辑

```sql
-- 预计算表结构
CREATE TABLE token_market_cache (
    id BIGSERIAL PRIMARY KEY,
    chain_id VARCHAR(50) NOT NULL,
    contract_address VARCHAR(66) NOT NULL,
    token_symbol VARCHAR(50) NOT NULL,
    coin_gecko_id VARCHAR(100),
    price_usd DECIMAL(30, 6),
    market_cap_usd DECIMAL(30, 6),
    volume_24h_usd DECIMAL(30, 6),
    price_change_24h DECIMAL(10, 4),
    holders_count BIGINT,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(chain_id, contract_address),
    INDEX idx_market_cap (market_cap_usd DESC),
    INDEX idx_volume_24h (volume_24h_usd DESC),
    INDEX idx_price (price_usd DESC),
    INDEX idx_holders (holders_count DESC)
);
```

#### 4.3 混合排序策略

对于按市场数据排序的请求：
1. 如果 `token_market_cache` 表存在且数据较新（< 5分钟），使用数据库排序
2. 否则，使用应用层排序（降级方案）

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

## 错误处理和边界情况

### 1. CoinGecko API 失败处理

**场景**：CoinGecko API 不可用或返回错误

**处理策略**：
```go
// 在 batchGetMarketData 中
if err != nil {
    global.Logger.Warnf("failed to get market data for %s: %v", sym, err)
    // 返回空的市场数据，前端显示为 "-" 或 "N/A"
    return &TokenMarketData{
        PriceUsd:       0,
        MarketCapUsd:   0,
        PriceChange24H: 0,
        TotalVolume24H: 0,
        HoldersCount:   0,
    }
}
```

**降级方案**：
- 如果 CoinGecko API 完全不可用，返回基础代币信息（名称、符号、地址、标签）
- 市场数据字段返回 `null` 或 `0`，前端显示为 "-"
- 记录错误日志，便于后续排查

### 2. 数据缺失处理

**持有者数量缺失**：
- CoinGecko 可能不提供某些代币的持有者数据
- 返回 `holders_count: 0` 或 `null`
- 前端判断：如果为 `0` 或 `null`，显示为 "-"

**交易量缺失**：
- 新代币或小币种可能没有交易量数据
- 返回 `total_volume_24h: 0`
- 前端显示为 "$0" 或 "-"

**价格数据缺失**：
- 如果价格获取失败，返回 `price_usd: 0`
- 前端显示为 "-" 或 "N/A"

### 3. 分页边界情况

**空结果**：
- 返回空数组 `items: []`
- `total: 0`
- `is_exact_count: true`

**超出最大页数**：
- 返回最后一页的数据
- `total` 保持不变
- 前端根据 `total` 和 `page_size` 计算最大页数

**大数据量排序**：
- 如果按市场数据排序，需要获取更多数据（3倍 pageSize）
- 如果数据量仍然不足，记录警告日志
- 返回可用数据，并在响应中添加 `warning` 字段

### 4. 搜索和筛选边界情况

**搜索无结果**：
- 返回空数组
- `total: 0`
- 前端显示"未找到匹配的代币"

**标签筛选无结果**：
- 返回空数组
- 前端提示"该标签下暂无代币"

**组合筛选**：
- 多个筛选条件组合时，使用 AND 逻辑
- 如果所有条件都不匹配，返回空结果

### 5. 并发和限流

**CoinGecko API 限流**：
- 免费版限制：每分钟 10-50 次请求
- 实现请求队列和限流器
- 如果触发限流，返回 429 错误，前端提示"请求过于频繁，请稍后重试"

**数据库连接池**：
- 设置合理的连接池大小
- 如果连接池耗尽，返回 503 错误
- 记录错误日志，监控连接池使用情况

### 6. 缓存失效处理

**缓存未命中**：
- 正常查询数据库和 CoinGecko API
- 异步更新缓存
- 不影响响应时间

**缓存数据过期**：
- 使用 TTL 自动过期
- 过期后重新获取数据
- 可以设置较短的 TTL（5分钟）确保数据新鲜度

**缓存服务不可用**：
- 如果 Redis 不可用，跳过缓存
- 直接查询数据库和 CoinGecko API
- 记录警告日志

---

## 监控和日志

### 关键指标

- **API 响应时间**：P50, P95, P99 延迟
- **CoinGecko API 调用次数和成功率**：监控 API 调用频率和错误率
- **缓存命中率**：列表缓存和市场数据缓存的命中率
- **数据库查询性能**：慢查询监控
- **错误率**：按错误类型统计（API 错误、数据库错误、缓存错误）

### 日志记录

```go
// 请求日志
global.Logger.Infof("token list request: chain_id=%s, search=%s, sort_by=%s, page=%d, page_size=%d",
    chainID, req.Search, req.SortBy, page, pageSize)

// 数据获取日志
global.Logger.Debugf("fetched %d tokens from database, %d with market data from CoinGecko",
    len(contracts), len(marketDataMap))

// 性能日志
global.Logger.Infof("token list query took %v, market data fetch took %v",
    dbQueryDuration, marketDataFetchDuration)

// 错误日志
global.Logger.Errorf("failed to get market data for token %s: %v", symbol, err)

// 缓存日志
global.Logger.Debugf("cache hit for token list: key=%s", cacheKey)
global.Logger.Debugf("cache miss for token list: key=%s", cacheKey)
```

### 告警规则

1. **API 响应时间 > 2s**：告警
2. **CoinGecko API 错误率 > 10%**：告警
3. **缓存命中率 < 50%**：告警
4. **数据库慢查询 > 1s**：告警
5. **错误率 > 5%**：告警

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

### 短期优化（1-2周）

1. **CoinGecko 批量查询优化**
   - 使用 `/coins/markets` 接口批量获取市场数据
   - 减少 API 调用次数，提高性能

2. **缓存优化**
   - 实现多层缓存架构
   - 添加缓存预热机制
   - 优化缓存失效策略

3. **数据库索引优化**
   - 添加全文搜索索引
   - 优化组合索引

### 中期优化（1-2月）

1. **预计算排序字段**
   - 创建 `token_market_cache` 表
   - 定时任务更新市场数据
   - 支持数据库层面排序

2. **链上持有者统计**
   - 实现链上事件监听
   - 统计代币持有者数量
   - 提供更准确的数据

3. **更多筛选条件**
   - 价格区间筛选
   - 市值区间筛选
   - 持有者数量筛选
   - 标签组合筛选

4. **导出功能**
   - 导出代币列表为 CSV/Excel
   - 支持自定义字段选择

### 长期优化（3-6月）

1. **实时更新**
   - 使用 WebSocket 推送价格更新
   - 实时更新交易量和市值

2. **历史数据**
   - 代币价格历史图表
   - 交易量趋势分析
   - 持有者变化趋势

3. **智能推荐**
   - 根据用户行为推荐代币
   - 热门代币推荐
   - 相似代币推荐

4. **多语言支持**
   - i18n 国际化
   - 支持多语言代币名称

5. **高级功能**
   - 代币对比功能
   - 收藏夹功能
   - 价格提醒功能

---

## 总结

### 方案概述

本方案通过新增 `/api/v1/tokens/list` 接口，聚合多个数据源，实现了完整的代币列表功能。基于 UI 设计稿和现有代码实现，本方案进行了以下优化：

### 核心优化点

1. **数据完整性**
   - ✅ 扩展 CoinGecko API 获取 24h 交易量（`total_volume_24h`）
   - ✅ 扩展 CoinGecko API 获取持有者数量（`holders_count`）
   - ✅ 完善数据降级策略（缺失数据返回 `null` 或 `0`）

2. **性能优化**
   - ✅ 批量获取市场数据（使用 CoinGecko 批量 API）
   - ✅ 并发请求 + 限流（避免 API 限流）
   - ✅ 多层缓存架构（列表缓存 + 市场数据缓存 + 标签缓存）
   - ✅ 数据库索引优化（全文搜索、组合索引）

3. **排序优化**
   - ✅ 数据库字段排序（`created_at`, `token_symbol`）
   - ✅ 应用层市场数据排序（市值、交易量、价格、持有者）
   - ✅ 预计算排序字段方案（长期优化）

4. **错误处理和降级**
   - ✅ CoinGecko API 失败时的降级策略
   - ✅ 持有者数据缺失时的处理（返回 `0` 或不显示）
   - ✅ 缓存失效时的降级方案

### 实施优先级

**P0（核心功能，1-2天）**：
- 创建新接口 `/api/v1/tokens/list`
- 扩展 CoinGecko API 获取交易量和持有者数据
- 实现基础列表查询 + 分页
- 批量获取市场数据（并发优化）

**P1（增强功能，1-2天）**：
- 搜索和筛选功能
- 排序功能（数据库字段 + 市场数据）
- 缓存优化

**P2（数据完善，2-3天）**：
- 链上持有者统计（长期方案）
- 首次部署时间优化（链上查询）
- 性能优化和测试

### 关键技术决策

1. **数据源选择**：
   - 短期：CoinGecko API（快速上线）
   - 长期：链上数据 + CoinGecko（数据准确）

2. **性能优化**：
   - 优先使用 CoinGecko 批量 API
   - 多层缓存减少 API 调用
   - 预计算排序字段（大数据量场景）

3. **用户体验**：
   - 支持多种排序方式
   - 灵活的筛选条件
   - 数据缺失时的友好提示

### 预期效果

- **响应时间**：< 500ms（缓存命中）或 < 2s（缓存未命中）
- **数据完整性**：> 90% 代币有完整市场数据
- **API 调用**：减少 80% 的 CoinGecko API 调用（通过批量查询和缓存）
- **用户体验**：支持快速搜索、筛选、排序，满足前端 UI 需求

通过分阶段实施，可以快速上线核心功能，然后逐步完善数据准确性和性能。
