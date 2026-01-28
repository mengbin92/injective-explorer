# 代币列表 API 实现方案（优化版）

## 需求概述

实现一个代币列表页面，展示 Injective Chain 上部署的所有代币合约信息，包括：
- 代币基本信息（名称、符号、标签、合约地址）
- 价格信息（当前价格、24h涨跌幅）
- 市场数据（24h交易量、市值、持有者数量）
- 元数据（首次部署时间）
- 支持搜索、筛选、排序、分页

### UI 需求分析（基于实际 HTML 文件）

根据前端 HTML 文件（`代币列表.html`），代币列表页面需要展示以下字段：

| 列名 | 字段 | 数据类型 | 格式要求 | 说明 |
|------|------|---------|---------|------|
| # | 序号 | number | - | 自动递增（从1开始） |
| 代币 | token_name, token_symbol, tags, contract_address | string, string, array, string | 地址需缩略显示（0x8a3d...7f2e） | 支持多标签显示（官方、稳定币、DeFi、Meme、跨链资产、预言机、原生代币等） |
| 价格 | price_usd, price_change_24h | number, number | $X.XX 或 $X.XXXXX（小价格），±X.XX% | 涨跌用颜色区分（绿色上涨，红色下跌） |
| 24h 交易量 | total_volume_24h | number | $X.XM / $X.XB | 自动格式化（M=百万，B=十亿） |
| 市值 | market_cap_usd | number | $X.XM / $X.XB | 自动格式化 |
| 持有者 | holders_count | number | X,XXX | 千分位格式化，带图标（👥） |
| 首次部署 | first_deployment_time | string | YYYY/MM/DD | 日期格式 |

**UI 交互功能：**
- 搜索框：支持搜索代币名称、符号或合约地址（模糊匹配）
- 分页：每页显示 20/50/100 条（可配置）
- 排序：虽然 UI 中没有显示排序按钮，但应支持按市值、交易量、价格、持有者、首次部署时间排序
- 筛选：支持按标签类型筛选（虽然 UI 中没有显示，但应支持）
- 代币点击：点击代币行跳转到代币详情页

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

## 代币详情页 API 设计

基于前端 UI 设计稿（`代币详情.html`），代币详情页需要展示代币的完整信息，包括概览、市场数据、转账记录、持有者列表和合约代码。

### UI 需求分析（基于实际 HTML 文件）

#### 页面结构

代币详情页包含以下主要部分：

1. **代币头部信息**
   - 代币图标（56x56px，支持 icon_url 或 icon_gradient）
   - 代币名称（24px，font-weight: 700）
   - 代币符号（18px，font-weight: 600）
   - 代币类型徽章（ERC-20 / ERC-721 / etc.）
   - 标签行（官方、稳定币、DeFi、Meme、跨链资产、预言机、原生代币等）
   - 合约地址（带复制按钮，完整地址显示）

2. **三列信息卡片**
   - **概览卡片**：
     - 最大供应量（格式化显示，无限制时显示"无限制"）
     - 流通量（格式化显示）
     - 完全稀释市值（格式化显示，无限制时显示"-"）
     - 流通市值（格式化显示）
   - **市场概览卡片**：
     - 价格（格式化显示，带涨跌徽章）
     - 24h 交易量（格式化显示）
     - 持有者（整数显示）
     - 总转账次数（整数显示）
   - **更多信息卡片**：
     - 代币标准（ERC-20, ERC-721 等）
     - 精度（decimals）
     - 首次部署（日期格式：YYYY-MM-DD）
     - 创建者（地址缩略显示：0x1234...5678）

3. **标签页**
   - **转账记录**（默认显示）：
     - 交易哈希（缩略显示，可点击跳转）
     - 方法（Transfer, TransferFrom, Approve, Mint 等）
     - 时间（相对时间显示：X 分钟前）
     - 发送方（地址缩略显示，可点击跳转）
     - 接收方（地址缩略显示，可点击跳转）
     - 数量（格式化显示，带代币符号）
     - 支持地址筛选（发送方或接收方）
   - **合约代码**：
     - 合约名称
     - 字节码展示（代码查看器，支持复制）
   - **注意**：HTML 中虽然生成了持仓列表的 JavaScript 函数，但 UI 中没有显示"持仓统计"标签页，该功能可作为可选功能保留

### API 设计

#### Proto 定义

```protobuf
// api/explorer/v1/token_detail.proto

syntax = "proto3";

package explorer.v1;

import "google/api/annotations.proto";
import "explorer/v1/common.proto";
import "explorer/v1/contract_info.proto";

option go_package = "injective-explorer/api/explorer/v1";

// TokenDetailService provides detailed token information
service TokenDetailService {
  // GetTokenDetail returns detailed information for a specific token
  rpc GetTokenDetail(GetTokenDetailRequest) returns (Reply) {
    option (google.api.http) = {
      get : "/api/v1/tokens/detail"
    };
  }

  // GetTokenTransfers returns transfer transactions for a token
  rpc GetTokenTransfers(GetTokenTransfersRequest) returns (Reply) {
    option (google.api.http) = {
      get : "/api/v1/tokens/transfers"
    };
  }

  // GetTokenHolders returns holder rankings for a token
  rpc GetTokenHolders(GetTokenHoldersRequest) returns (Reply) {
    option (google.api.http) = {
      get : "/api/v1/tokens/holders"
    };
  }

  // GetTokenHoldersStats returns holder statistics
  rpc GetTokenHoldersStats(GetTokenHoldersStatsRequest) returns (Reply) {
    option (google.api.http) = {
      get : "/api/v1/tokens/holders-stats"
    };
  }
}

// GetTokenDetailRequest specifies which token to query
message GetTokenDetailRequest {
  string contract_address = 1 [ json_name = "contract_address" ]; // Contract address (required)
  string chain_id = 2 [ json_name = "chain_id" ];                 // Chain ID (optional)
}

// TokenDetailResponse contains complete token information
message TokenDetailResponse {
  // Basic information
  TokenBasicInfo basic = 1 [ json_name = "basic" ];

  // Overview (supply and market cap)
  TokenOverviewInfo overview = 2 [ json_name = "overview" ];

  // Market information
  TokenMarketInfo market = 3 [ json_name = "market" ];

  // Additional information
  TokenAdditionalInfo additional = 4 [ json_name = "additional" ];
}

// TokenBasicInfo contains basic token information
message TokenBasicInfo {
  string name = 1 [ json_name = "name" ];
  string symbol = 2 [ json_name = "symbol" ];
  string icon_url = 3 [ json_name = "icon_url" ];        // Optional token icon URL
  string icon_gradient = 4 [ json_name = "icon_gradient" ]; // Fallback gradient CSS
  string contract_address = 5 [ json_name = "contract_address" ];
  string token_type = 6 [ json_name = "token_type" ];   // ERC-20, ERC-721, etc.
  repeated ContractTagInfoDTO tags = 7 [ json_name = "tags" ];
}

// TokenOverviewInfo contains supply and market cap data
message TokenOverviewInfo {
  string max_supply = 1 [ json_name = "max_supply" ];           // Formatted string, "无限制" if unlimited
  string circulating_supply = 2 [ json_name = "circulating_supply" ];
  string fully_diluted_market_cap = 3 [ json_name = "fully_diluted_market_cap" ]; // "$0" if unlimited
  string market_cap = 4 [ json_name = "market_cap" ];
}

// TokenMarketInfo contains market data
message TokenMarketInfo {
  string price = 1 [ json_name = "price" ];                    // Formatted: "$1.00" or "$0.00000123"
  double price_change_24h = 2 [ json_name = "price_change_24h" ]; // For badge display
  string volume_24h = 3 [ json_name = "volume_24h" ];          // Formatted: "$53,200,000"
  int64 holders_count = 4 [ json_name = "holders_count" ];     // Integer count
  int64 total_transfers = 5 [ json_name = "total_transfers" ]; // Total transfer count
}

// TokenAdditionalInfo contains token technical details
message TokenAdditionalInfo {
  string token_standard = 1 [ json_name = "token_standard" ]; // ERC-20, ERC-721, etc.
  int32 decimals = 2 [ json_name = "decimals" ];
  string deploy_date = 3 [ json_name = "deploy_date" ];       // "2024-03-15"
  string creator_address = 4 [ json_name = "creator_address" ]; // Full address
  string creator_short = 5 [ json_name = "creator_short" ];    // "0x1234...5678"
}

// GetTokenTransfersRequest queries token transfers
message GetTokenTransfersRequest {
  string contract_address = 1 [ json_name = "contract_address" ];
  string chain_id = 2 [ json_name = "chain_id" ];

  // Optional filters
  string address = 3 [ json_name = "address" ]; // Filter by sender or receiver address
  string method = 4 [ json_name = "method" ];   // Filter by method (Transfer, Mint, etc.)

  // Pagination
  PaginationRequest pagination = 5 [ json_name = "pagination" ];
}

// TokenTransferInfo represents a single transfer
message TokenTransferInfo {
  string tx_hash = 1 [ json_name = "tx_hash" ];
  string method = 2 [ json_name = "method" ];     // Transfer, Mint, Burn, etc.
  int64 timestamp = 3 [ json_name = "timestamp" ];
  string time_ago = 4 [ json_name = "time_ago" ]; // "5 分钟前"
  string from_address = 5 [ json_name = "from_address" ];
  string to_address = 6 [ json_name = "to_address" ];
  string amount = 7 [ json_name = "amount" ];     // Formatted with symbol: "1,234.56 USDT"
  string value_usd = 8 [ json_name = "value_usd" ]; // Optional USD value
}

// TokenTransfersResponse contains paginated transfers
message TokenTransfersResponse {
  repeated TokenTransferInfo items = 1 [ json_name = "items" ];
  int32 page = 2 [ json_name = "page" ];
  int32 page_size = 3 [ json_name = "page_size" ];
  int64 total = 4 [ json_name = "total" ];
  bool is_exact_count = 5 [ json_name = "is_exact_count" ];
}

// GetTokenHoldersRequest queries token holders
message GetTokenHoldersRequest {
  string contract_address = 1 [ json_name = "contract_address" ];
  string chain_id = 2 [ json_name = "chain_id" ];

  // Pagination
  PaginationRequest pagination = 3 [ json_name = "pagination" ];

  // Sorting
  string sort_by = 4 [ json_name = "sort_by" ]; // "balance" (default), "percentage"
  string sort_order = 5 [ json_name = "sort_order" ]; // "desc" (default), "asc"
}

// TokenHolderInfo represents a single holder
message TokenHolderInfo {
  int32 rank = 1 [ json_name = "rank" ];
  string address = 2 [ json_name = "address" ];
  string label = 3 [ json_name = "label" ];        // Optional label (e.g., "Binance Hot Wallet")
  string label_type = 4 [ json_name = "label_type" ]; // "exchange", "burn", "whale", etc.
  string balance = 5 [ json_name = "balance" ];    // Formatted: "1,234.56 USDT"
  string percentage = 6 [ json_name = "percentage" ]; // "18.5"
  string value_usd = 7 [ json_name = "value_usd" ]; // "$123,456.78"
}

// TokenHoldersResponse contains paginated holders
message TokenHoldersResponse {
  repeated TokenHolderInfo items = 1 [ json_name = "items" ];
  int32 page = 2 [ json_name = "page" ];
  int32 page_size = 3 [ json_name = "page_size" ];
  int64 total = 4 [ json_name = "total" ];
  bool is_exact_count = 5 [ json_name = "is_exact_count" ];
}

// GetTokenHoldersStatsRequest queries holder statistics
message GetTokenHoldersStatsRequest {
  string contract_address = 1 [ json_name = "contract_address" ];
  string chain_id = 2 [ json_name = "chain_id" ];
}

// TokenHoldersStatsResponse contains holder statistics
message TokenHoldersStatsResponse {
  int64 total_holders = 1 [ json_name = "total_holders" ];       // Total unique holders
  string top10_percentage = 2 [ json_name = "top10_percentage" ]; // "45.2"
  string top100_percentage = 3 [ json_name = "top100_percentage" ]; // "68.7"
  int64 circulating_supply = 4 [ json_name = "circulating_supply" ]; // For percentage calc
}
```

### API 响应示例

#### GetTokenDetail 响应

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "basic": {
      "name": "Tether USD",
      "symbol": "USDT",
      "icon_url": "https://assets.coingecko.com/coins/images/325/small/Tether.png",
      "icon_gradient": "linear-gradient(135deg, #26a17b 0%, #1a7a5c 100%)",
      "contract_address": "0x8a3d5E2F4B7C9D1A6E8F3B2C7D4E9F1A2B3C7f2e",
      "token_type": "ERC-20",
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
      ]
    },
    "overview": {
      "max_supply": "100,000,000,000",
      "circulating_supply": "85,043,216,789",
      "fully_diluted_market_cap": "$100,000,000,000",
      "market_cap": "$85,043,216,789"
    },
    "market": {
      "price": "$1.00",
      "price_change_24h": 0.02,
      "volume_24h": "$53,200,000",
      "holders_count": 125832,
      "total_transfers": 1256789
    },
    "additional": {
      "token_standard": "ERC-20",
      "decimals": 6,
      "deploy_date": "2024-03-15",
      "creator_address": "0x1a2b3c4d5e6f7890abcdef1234567890abcd5678",
      "creator_short": "0x1a2b...5678"
    }
  }
}
```

#### GetTokenTransfers 响应

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "items": [
      {
        "tx_hash": "0x8a3d5E2F4B7C9D1A6E8F3B2C7D4E9F1A2B3C7f2e",
        "method": "Transfer",
        "timestamp": 1708052400,
        "time_ago": "5 分钟前",
        "from_address": "0x5b2a4C8E9F1D3A7B6E2F8C4D9A1B3E7F2C8D3c8d",
        "to_address": "0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599",
        "amount": "1,234.56 USDT",
        "value_usd": "$1,234.56"
      }
    ],
    "page": 1,
    "page_size": 20,
    "total": 1256789,
    "is_exact_count": false
  }
}
```

#### GetTokenHolders 响应

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "items": [
      {
        "rank": 1,
        "address": "0x0000000000000000000000000000000000000000",
        "label": "销毁地址",
        "label_type": "burn",
        "balance": "18,500,000,000 USDT",
        "percentage": "18.5",
        "value_usd": "$18,500,000,000"
      },
      {
        "rank": 2,
        "address": "0x5b2a4C8E9F1D3A7B6E2F8C4D9A1B3E7F2C8D3c8d",
        "label": "Binance Hot Wallet",
        "label_type": "exchange",
        "balance": "12,300,000,000 USDT",
        "percentage": "12.3",
        "value_usd": "$12,300,000,000"
      }
    ],
    "page": 1,
    "page_size": 20,
    "total": 125832,
    "is_exact_count": true
  }
}
```

#### GetTokenHoldersStats 响应

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "total_holders": 125832,
    "top10_percentage": "45.2",
    "top100_percentage": "68.7",
    "circulating_supply": "85,043,216,789"
  }
}
```

---

## 总结

### 方案概述

本方案通过新增 `/api/v1/tokens/list` 和 `/api/v1/tokens/*` 系列接口，聚合多个数据源，实现了完整的代币列表和详情功能。基于实际 HTML UI 文件和现有代码实现，本方案进行了以下优化升级：

### 核心优化点

1. **数据完整性**
   - ✅ 扩展 CoinGecko API 获取 24h 交易量（`total_volume_24h`）
   - ✅ 扩展 CoinGecko API 获取持有者数量（`holders_count`）
   - ✅ 完善数据降级策略（缺失数据返回 `null` 或 `0`）
   - ✅ 新增总转账次数统计（`total_transfers`）
   - ✅ 新增代币图标支持（`icon_url` 和 `icon_gradient`）

2. **性能优化**
   - ✅ 批量获取市场数据（使用 CoinGecko 批量 API `/coins/markets`）
   - ✅ 并发请求 + 限流（避免 API 限流，建议并发数 10-20）
   - ✅ 多层缓存架构（列表缓存 + 市场数据缓存 + 标签缓存）
   - ✅ 数据库索引优化（全文搜索、组合索引）
   - ✅ 预计算排序字段方案（长期优化，使用 `token_market_cache` 表）

3. **排序优化**
   - ✅ 数据库字段排序（`created_at`, `token_symbol`）
   - ✅ 应用层市场数据排序（市值、交易量、价格、持有者）
   - ✅ 支持多字段排序（默认按市值降序）
   - ✅ 预计算排序字段方案（长期优化）

4. **错误处理和降级**
   - ✅ CoinGecko API 失败时的降级策略（返回基础信息，市场数据为 0）
   - ✅ 持有者数据缺失时的处理（返回 `0` 或 `null`，前端显示为 "-"）
   - ✅ 缓存失效时的降级方案（直接查询数据库和 API）
   - ✅ 数据格式化容错（处理 null/undefined 值）

5. **UI 适配优化**
   - ✅ 日期格式统一（列表页 YYYY/MM/DD，详情页 YYYY-MM-DD）
   - ✅ 地址缩略显示（0x8a3d...7f2e）
   - ✅ 价格格式化（小价格显示更多小数位）
   - ✅ 大数字格式化（M/B 单位）
   - ✅ 标签颜色映射（官方、稳定币、DeFi 等不同颜色）

### 实施优先级

**P0（核心功能，1-2天）**：
- ✅ 创建新接口 `/api/v1/tokens/list`
- ✅ 创建代币详情接口 `/api/v1/tokens/detail`
- ✅ 扩展 CoinGecko API 获取交易量和持有者数据
- ✅ 实现基础列表查询 + 分页
- ✅ 批量获取市场数据（并发优化）
- ✅ 实现搜索功能（代币名称、符号、合约地址）

**P1（增强功能，1-2天）**：
- ✅ 搜索和筛选功能（标签筛选、合约类型筛选）
- ✅ 排序功能（数据库字段 + 市场数据）
- ✅ 缓存优化（Redis 多层缓存）
- ✅ 转账记录查询接口 `/api/v1/tokens/transfers`（支持地址筛选）
- ✅ 合约代码查询接口（字节码展示）

**P2（数据完善，2-3天）**：
- ✅ 持有者列表查询接口 `/api/v1/tokens/holders`（可选功能，UI 中未显示）
- ✅ 持有者统计接口 `/api/v1/tokens/holders-stats`（可选功能）
- ✅ 总转账次数统计（`total_transfers`）
- ✅ 代币图标支持（`icon_url` 和 `icon_gradient`）
- ✅ 链上持有者统计（长期方案）
- ✅ 首次部署时间优化（链上查询）
- ✅ 性能优化和测试

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
- **UI 适配度**：100% 匹配 HTML 设计稿的显示需求

### 基于 HTML 分析的优化建议

1. **代币列表页优化**：
   - 支持按列排序（点击表头排序）
   - 支持标签筛选（虽然 UI 中没有显示筛选器，但应支持）
   - 优化搜索体验（支持代币符号、名称、合约地址的模糊匹配）
   - 分页大小可配置（20/50/100）

2. **代币详情页优化**：
   - 转账记录支持地址筛选（发送方或接收方）
   - 时间显示使用相对时间（X 分钟前）
   - 合约代码支持复制功能
   - 持仓统计作为可选功能（虽然 HTML 中有代码，但 UI 中未显示）

3. **数据格式化优化**：
   - 价格格式化：小于 0.01 时显示更多小数位
   - 大数字格式化：使用 M/B 单位
   - 地址格式化：统一使用 0x8a3d...7f2e 格式
   - 日期格式化：列表页 YYYY/MM/DD，详情页 YYYY-MM-DD

4. **性能优化**：
   - 使用 CoinGecko `/coins/markets` 批量 API（一次请求获取多个代币数据）
   - 实现智能缓存策略（热门代币缓存时间更长）
   - 数据库查询优化（使用索引，避免全表扫描）

通过分阶段实施，可以快速上线核心功能，然后逐步完善数据准确性和性能。

---

## 基于 HTML 文件分析的详细优化建议

### 1. 代币列表页（`代币列表.html`）优化

#### 1.1 数据字段补充

根据 HTML 分析，需要补充以下字段：

```protobuf
// TokenListItemDTO 需要添加的字段
message TokenListItemDTO {
  // ... 现有字段 ...
  
  // 新增字段
  string icon_url = 13 [ json_name = "icon_url" ];        // 代币图标 URL
  string icon_gradient = 14 [ json_name = "icon_gradient" ]; // 图标渐变 CSS（备用）
  string contract_address_short = 15 [ json_name = "contract_address_short" ]; // 缩略地址（0x8a3d...7f2e）
}
```

#### 1.2 排序功能增强

虽然 HTML 中没有显示排序按钮，但应支持以下排序方式：

- **默认排序**：按市值降序（`market_cap DESC`）
- **可选排序**：
  - 市值（`market_cap`）
  - 24h 交易量（`volume_24h`）
  - 价格（`price`）
  - 持有者数量（`holders_count`）
  - 首次部署时间（`first_deployment_time`）
  - 代币符号（`token_symbol`，字母序）

#### 1.3 搜索功能优化

HTML 中的搜索框支持：
- 代币名称模糊匹配
- 代币符号精确/模糊匹配
- 合约地址精确/模糊匹配

**实现建议**：
```go
// 搜索逻辑优化
if req.Search != "" {
    searchPattern := "%" + req.Search + "%"
    // 支持地址格式（0x开头）
    if strings.HasPrefix(req.Search, "0x") {
        query = query.Where("contract_address LIKE ?", searchPattern)
    } else {
        // 名称和符号模糊匹配
        query = query.Where(
            "token_name LIKE ? OR token_symbol LIKE ? OR contract_address LIKE ?",
            searchPattern, searchPattern, searchPattern,
        )
    }
}
```

### 2. 代币详情页（`代币详情.html`）优化

#### 2.1 转账记录接口优化

HTML 中的转账记录支持地址筛选：

```protobuf
// GetTokenTransfersRequest 优化
message GetTokenTransfersRequest {
  string contract_address = 1 [ json_name = "contract_address" ];
  string chain_id = 2 [ json_name = "chain_id" ];
  
  // 地址筛选（发送方或接收方）
  string address = 3 [ json_name = "address" ]; // 支持发送方或接收方筛选
  
  // 方法筛选（可选）
  string method = 4 [ json_name = "method" ];   // Transfer, Mint, Burn 等
  
  // 时间范围筛选（可选，未来扩展）
  int64 start_time = 5 [ json_name = "start_time" ];
  int64 end_time = 6 [ json_name = "end_time" ];
  
  PaginationRequest pagination = 7 [ json_name = "pagination" ];
}
```

#### 2.2 时间格式化

HTML 中使用相对时间显示（"X 分钟前"），需要后端或前端处理：

```go
// 时间格式化函数
func formatTimeAgo(timestamp int64) string {
    now := time.Now().Unix()
    diff := now - timestamp
    
    if diff < 60 {
        return fmt.Sprintf("%d 秒前", diff)
    } else if diff < 3600 {
        return fmt.Sprintf("%d 分钟前", diff/60)
    } else if diff < 86400 {
        return fmt.Sprintf("%d 小时前", diff/3600)
    } else if diff < 2592000 {
        return fmt.Sprintf("%d 天前", diff/86400)
    } else {
        return time.Unix(timestamp, 0).Format("2006-01-02")
    }
}
```

#### 2.3 合约代码接口

HTML 中显示合约字节码，需要新增接口：

```protobuf
// GetTokenContractCode 获取合约代码
rpc GetTokenContractCode(GetTokenContractCodeRequest) returns (Reply) {
  option (google.api.http) = {
    get : "/api/v1/tokens/contract-code"
  };
}

message GetTokenContractCodeRequest {
  string contract_address = 1 [ json_name = "contract_address" ];
  string chain_id = 2 [ json_name = "chain_id" ];
}

message TokenContractCodeResponse {
  string contract_name = 1 [ json_name = "contract_name" ];
  string bytecode = 2 [ json_name = "bytecode" ];        // 合约字节码
  string source_code = 3 [ json_name = "source_code" ];   // 源代码（如果已验证）
  bool verified = 4 [ json_name = "verified" ];          // 是否已验证
}
```

### 3. 数据格式化优化

#### 3.1 价格格式化

根据 HTML 分析，价格格式化规则：

```go
func formatPrice(price float64) string {
    if price >= 1000 {
        return fmt.Sprintf("$%s", formatLargeNumber(price))
    } else if price >= 1 {
        return fmt.Sprintf("$%.2f", price)
    } else if price >= 0.01 {
        return fmt.Sprintf("$%.4f", price)
    } else {
        return fmt.Sprintf("$%.8f", price)
    }
}
```

#### 3.2 大数字格式化

```go
func formatLargeNumber(value float64) string {
    if value >= 1e9 {
        return fmt.Sprintf("%.1fB", value/1e9)
    } else if value >= 1e6 {
        return fmt.Sprintf("%.1fM", value/1e6)
    } else if value >= 1e3 {
        return fmt.Sprintf("%.1fK", value/1e3)
    }
    return fmt.Sprintf("%.0f", value)
}
```

#### 3.3 地址缩略显示

```go
func formatAddressShort(address string) string {
    if len(address) <= 10 {
        return address
    }
    return fmt.Sprintf("%s...%s", address[:6], address[len(address)-4:])
}
```

### 4. 标签颜色映射

根据 HTML CSS 分析，标签颜色映射：

```go
var tagColorMap = map[string]string{
    "official":  "#28a745",  // 绿色
    "stablecoin": "#1a73e8",  // 蓝色
    "defi":      "#845ef7",  // 紫色
    "meme":      "#ff6b6b",  // 红色
    "wrapped":   "#fd7e14",  // 橙色
    "oracle":    "#375bd2",  // 深蓝色
    "native":    "#fab005",  // 黄色
}
```

### 5. 性能优化建议

#### 5.1 CoinGecko 批量 API 使用

优先使用 `/coins/markets` 接口批量获取市场数据：

```go
// 批量获取市场数据（优化版）
func batchGetMarketDataFromCoinGecko(ctx context.Context, coinGeckoIDs []string) (map[string]*TokenMarketData, error) {
    // 分批处理（每批最多 100 个）
    batchSize := 100
    result := make(map[string]*TokenMarketData)
    
    for i := 0; i < len(coinGeckoIDs); i += batchSize {
        end := i + batchSize
        if end > len(coinGeckoIDs) {
            end = len(coinGeckoIDs)
        }
        
        ids := strings.Join(coinGeckoIDs[i:end], ",")
        apiURL := fmt.Sprintf(
            "https://api.coingecko.com/api/v3/coins/markets?ids=%s&vs_currency=usd&order=market_cap_desc&per_page=100&page=1&sparkline=false&price_change_percentage=24h",
            ids,
        )
        
        // 调用 API 并解析
        // ...
    }
    
    return result, nil
}
```

#### 5.2 缓存策略优化

```go
// 缓存键设计
const (
    // 列表缓存（包含查询参数）
    CacheKeyTokenList = "token_list:%s:%s:%d:%d"  // chain_id:filters_hash:page:page_size
    
    // 市场数据缓存（按代币符号）
    CacheKeyTokenMarket = "token_market:%s"        // token_symbol
    
    // 详情缓存（按合约地址）
    CacheKeyTokenDetail = "token_detail:%s:%s"    // chain_id:contract_address
)

// 缓存时间
const (
    CacheExpireTokenList   = 5 * time.Minute   // 列表缓存 5 分钟
    CacheExpireTokenMarket = 5 * time.Minute   // 市场数据 5 分钟
    CacheExpireTokenDetail = 10 * time.Minute  // 详情缓存 10 分钟
)
```

### 6. 错误处理优化

#### 6.1 数据缺失处理

```go
// 处理缺失的市场数据
func fillMissingMarketData(item *pb.TokenListItemDTO) {
    if item.PriceUsd == 0 {
        item.PriceUsd = 0  // 前端显示为 "-"
    }
    if item.MarketCapUsd == 0 {
        item.MarketCapUsd = 0  // 前端显示为 "-"
    }
    if item.HoldersCount == 0 {
        item.HoldersCount = 0  // 前端显示为 "-" 或 "0"
    }
}
```

#### 6.2 API 降级策略

```go
// CoinGecko API 失败时的降级处理
func getMarketDataWithFallback(ctx context.Context, symbol string) (*TokenMarketData, error) {
    // 1. 尝试从 CoinGecko 获取
    data, err := getMarketDataFromCoinGecko(ctx, symbol)
    if err == nil {
        return data, nil
    }
    
    // 2. 尝试从缓存获取（即使过期）
    cachedData := getCachedMarketData(ctx, symbol)
    if cachedData != nil {
        global.Logger.Warnf("using cached market data for %s (CoinGecko unavailable)", symbol)
        return cachedData, nil
    }
    
    // 3. 返回空数据（前端显示为 "-"）
    return &TokenMarketData{
        PriceUsd:       0,
        MarketCapUsd:   0,
        PriceChange24H: 0,
        TotalVolume24H: 0,
        HoldersCount:   0,
    }, nil
}
```

### 7. 数据库优化

#### 7.1 索引建议

```sql
-- 代币列表查询优化索引
CREATE INDEX IF NOT EXISTS idx_contract_infos_token_symbol 
  ON contract_infos(token_symbol) 
  WHERE token_symbol IS NOT NULL AND token_symbol != '';

-- 搜索优化索引（全文搜索）
CREATE INDEX IF NOT EXISTS idx_contract_infos_search 
  ON contract_infos USING gin(to_tsvector('english', 
    COALESCE(token_name, '') || ' ' || COALESCE(token_symbol, '') || ' ' || COALESCE(contract_address, '')
  ));

-- 排序优化索引
CREATE INDEX IF NOT EXISTS idx_contract_infos_created_at 
  ON contract_infos(created_at DESC);

-- 标签关联查询索引
CREATE INDEX IF NOT EXISTS idx_contract_tags_lookup 
  ON contract_tags(chain_id, contract_address, tag_type, tag_value);
```

#### 7.2 预计算表（可选）

对于大数据量场景，可以创建预计算表：

```sql
-- 市场数据预计算表
CREATE TABLE IF NOT EXISTS token_market_cache (
    id BIGSERIAL PRIMARY KEY,
    chain_id VARCHAR(50) NOT NULL,
    contract_address VARCHAR(66) NOT NULL,
    token_symbol VARCHAR(50) NOT NULL,
    coin_gecko_id VARCHAR(100),
    price_usd DECIMAL(30, 8),
    market_cap_usd DECIMAL(30, 2),
    volume_24h_usd DECIMAL(30, 2),
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

### 8. 测试建议

#### 8.1 单元测试

```go
func TestTokenListService_ListTokens(t *testing.T) {
    tests := []struct {
        name     string
        req      *pb.ListTokensRequest
        wantCode int
    }{
        {
            name: "基础查询",
            req: &pb.ListTokensRequest{
                Pagination: &pb.PaginationRequest{Page: 1, PageSize: 20},
            },
            wantCode: 200,
        },
        {
            name: "搜索代币",
            req: &pb.ListTokensRequest{
                Search: "USDT",
                Pagination: &pb.PaginationRequest{Page: 1, PageSize: 20},
            },
            wantCode: 200,
        },
        {
            name: "按市值排序",
            req: &pb.ListTokensRequest{
                SortBy: "market_cap",
                SortOrder: "desc",
                Pagination: &pb.PaginationRequest{Page: 1, PageSize: 20},
            },
            wantCode: 200,
        },
    }
    // ...
}
```

#### 8.2 性能测试

```bash
# 压力测试
wrk -t4 -c100 -d30s --latency \
  "http://localhost:8080/api/v1/tokens/list?page=1&page_size=20"

# 并发测试
ab -n 1000 -c 10 \
  "http://localhost:8080/api/v1/tokens/list?page=1&page_size=20"
```

---

## 实施检查清单

### 代币列表页
- [ ] 实现 `/api/v1/tokens/list` 接口
- [ ] 支持搜索（名称、符号、地址）
- [ ] 支持排序（市值、交易量、价格、持有者）
- [ ] 支持分页（20/50/100）
- [ ] 支持标签筛选
- [ ] 数据格式化（价格、大数字、地址）
- [ ] 缓存优化

### 代币详情页
- [ ] 实现 `/api/v1/tokens/detail` 接口
- [ ] 实现 `/api/v1/tokens/transfers` 接口（支持地址筛选）
- [ ] 实现 `/api/v1/tokens/contract-code` 接口
- [ ] 时间格式化（相对时间）
- [ ] 数据格式化（价格、大数字、地址）
- [ ] 缓存优化

### 数据源
- [ ] 扩展 CoinGecko API 获取交易量
- [ ] 扩展 CoinGecko API 获取持有者数量
- [ ] 实现批量获取市场数据
- [ ] 实现降级策略

### 性能优化
- [ ] 数据库索引优化
- [ ] Redis 缓存实现
- [ ] 批量 API 调用优化
- [ ] 并发控制

### 测试
- [ ] 单元测试
- [ ] 集成测试
- [ ] 性能测试
- [ ] 压力测试
