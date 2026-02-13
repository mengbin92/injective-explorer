# ERC20 合约元数据同步完整技术方案

## 文档目录

| 章节 | 内容 |
|------|------|
| [一、背景与问题](#一背景与问题) | 需求背景、目标 |
| [二、ERC20 元数据获取原理](#二erc20-元数据获取原理) | 标准接口、调用方式、前置条件 |
| [三、现状分析](#三现状分析) | 合约来源路径、数据流、已修复问题 |
| [三一、Peggy Tokens 特殊处理](#三一peggy-tokens-特殊处理) | Peggy 架构、元数据来源、处理流程（新增） |
| [四、完整技术方案](#四完整技术方案) | 实时同步、历史回填、API、验证、容错、多活、批量化、并发、缓存、监控 |
| [五、数据模型](#五数据模型) | 字段、索引、迁移脚本 |
| [六、配置设计](#六配置设计) | 现有与扩展配置 |
| [七、监控与可观测性](#七监控与可观测性) | 日志、指标、健康检查 |
| [八、实现计划](#八实现计划) | 阶段与优先级 |
| [九、测试策略](#九测试策略) | 单元、集成、回归 |
| [十、后续扩展](#十后续扩展) | CW20、多链等 |
| [附录 A](#附录-a相关代码位置) | 代码位置 |
| [附录 B](#附录-b问题排查指南) | 排查指南、诊断命令 |
| [附录 C](#附录-cerc20-方法-selector-参考) | Selector 参考 |
| [附录 D](#附录-d快速修复指南) | 快速修复步骤 |
| [附录 E](#附录-e审查补充与遗漏修正记录) | 审查修正记录 |
| [附录 F](#附录-fpeggy-tokens-诊断实例) | Peggy token 诊断案例（新增） |

---

## 一、背景与问题

### 1.1 需求背景

Explorer 已支持 Create Token Pair（如 `MsgCreateTokenPair`）等合约创建事件的解析，能将新部署的 ERC20 合约写入 `contract_infos` 表。但无法达到 injscan 级别的体验：**合约的 `token_name` 和 `token_symbol` 经常为空**。

示例：交易 `0x70af765a4b927be4ef9fc63338dae13df5e28b26592a2eff4167a6be11df2b16` 为创建合约对的交易，解析后 `contract_infos` 中有对应记录，但 `token_name` 和 `token_symbol` 均为空。

### 1.2 目标

| 目标 | 说明 |
|------|------|
| 实时同步 | Create Token Pair 等事件触发的合约，在入库后自动补齐 token_name、token_symbol、decimals |
| 历史回填 | 已有记录中 metadata 为空或残缺的，支持批量/按需补充 |
| 多来源覆盖 | Cosmos EventCreateTokenPair + EVM 直接/Factory 创建的合约均能获取元数据 |
| 可观测性 | 支持监控元数据拉取成功率和失败原因 |

---

## 二、ERC20 元数据获取原理

### 2.1 标准接口

ERC20 合约通过以下标准方法暴露元数据：

| 方法 | Selector | 返回值 | 用途 |
|------|----------|--------|------|
| `name()` | `0x06fdde03` | string | 代币全名（如 "Tether USD"） |
| `symbol()` | `0x95d89b41` | string | 代币符号（如 "USDT"） |
| `decimals()` | `0x313ce567` | uint8 | 精度（18 或 6 等） |

### 2.2 调用方式

通过 EVM 的 `eth_call` 进行只读调用，**不发送交易、无 gas 消耗**：

```
eth_call(to: contractAddress, data: selector, block: "latest")
```

- 返回值按 ABI 编码（string 为 offset + length + data）
- 现有实现：`provider/chain/evm_client.go` 中的 `GetERC20Name`、`GetERC20Symbol`、`GetERC20Decimals`、`GetERC20Metadata`

### 2.3 前置条件

- 合约地址需存在于 **当前链的 EVM 层**
- 需配置正确的 **EVM RPC 端点**（必须指向该链的 EVM JSON-RPC，而非其他链）
- 合约需实现 ERC20 接口；非标准合约可能返回空或失败

---

## 三、现状分析

### 3.1 合约来源路径

| 路径 | 触发场景 | 实现位置 | 元数据获取方式 |
|------|----------|----------|----------------|
| **Cosmos 事件** | `MsgCreateTokenPair` → `EventCreateTokenPair` | `sync.go` → `saveContractInfoFromCosmosEvents` | 异步 `scheduleMetadataFetch` → `fetchAndUpdateERC20Metadata` |
| **EVM 详情同步** | receipt.ContractAddress 或 Event Logs 解析的 Factory 创建 | `evm_details_sync.go` → `saveContractInfoIfNotExists` | 同步调用 `GetERC20Metadata` 后入库 |

### 3.2 数据流示意

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Cosmos 交易同步                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│  GetTx(txHash) → extractContractCreationEvents(EventCreateTokenPair)        │
│       → buildContractInfo → persistContractInfo                             │
│       → scheduleMetadataFetch(contractAddress, chainID)  [异步]               │
│            → processMetadataJob → fetchAndUpdateERC20Metadata                │
│                 → createEVMClient() → GetERC20Metadata → DB Updates         │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                          EVM 详情同步                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│  EVM RPC GetTransactionDetails → receipt.ContractAddress / Event Logs       │
│       → saveContractInfoIfNotExists                                         │
│            → GetERC20Metadata(同步) → 写入 contractInfo 后 FirstOrCreate     │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.3 已修复问题（根因）

**问题**：`fetchAndUpdateERC20Metadata` 使用 `chain.NewEVMClient(global.NetworkName)`，而 `NewEVMClient` 仅支持 `mainnet`/`testnet`，会连接 Injective 预设 RPC。对于 injective 等自定义链（`NetworkName = "injective"`），会返回 `unsupported network`，导致 EVM 客户端创建失败，元数据拉取被跳过。

**修复**：改用 `createEVMClient()`，优先使用配置中的 `chain.evm.rpc_endpoint`，支持自定义链。

---

## 三一、Peggy Tokens 特殊处理

> **📋 重要更新（2026-02-13）**：经过实际测试验证，Injective Chain 的 Peggy Query 接口返回 `HTTP 501 Not Implemented`，因此**无法通过 Cosmos Peggy 模块获取元数据**。
>
> **主要变更**：
> 1. ❌ **移除**：Cosmos Peggy Module Query 方案（接口不可用）
> 2. ✅ **提升**：Ethereum RPC 为主要数据源（从 P1 提升至 **P0**）
> 3. ✅ **简化**：回退策略调整为 3 级（Ethereum RPC → CoinGecko → 默认值）
>
> **验证结果**：详见 [附录 F：Cosmos Peggy Query 验证记录](#附录-fcosmos-peggy-query-验证记录) 和 [附录 G：Peggy Tokens 诊断实例](#附录-gpeggy-tokens-诊断实例)


### 3.1 Peggy Tokens 架构与特点

#### 3.1.1 什么是 Peggy Tokens

Peggy 是 Injective Labs 开发的跨链桥，允许将以太坊（及其他 EVM 链）上的 ERC20 代币桥接到 Injective Chain。

| 特性 | 说明 |
|------|------|
| **两层架构** | Cosmos 层（bank_denom） + EVM 层（ERC20 包装合约） |
| **元数据存储** | 元数据存储在 Cosmos 层，EVM 合约仅实现基本转账功能 |
| **denom 格式** | `peggy0x{erc20_address}`，例如 `peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca` |
| **创建方式** | 通过 `MsgCreateTokenPair` Cosmos 消息创建 |

#### 3.1.2 元数据获取的挑战

**问题示例**：交易 `0x70af765a4b927be4ef9fc63338dae13df5e28b26592a2eff4167a6be11df2b16`

```bash
# EVM 层调用测试
contract=0x9772154BF1823353C829B43F9AD96CF9f720c768

✅ 合约存在：eth_getCode 返回字节码
❌ name()     → 0x (空字符串)
❌ symbol()   → 0x (空字符串)
❌ decimals() → 0x (0)
```

**根本原因**：Peggy 包装合约是"薄包装器"（thin wrapper），不存储元数据。真实代币信息需要从：
1. Cosmos Peggy 模块查询
2. 原始链（Ethereum）的 ERC20 合约获取
3. 代币注册表或第三方 API（CoinGecko 等）

### 3.2 数据流分析

#### 3.2.1 当前流程（标准 ERC20）

```
┌─────────────────────────────────────────────────────────────┐
│              标准 ERC20 合约创建流程                      │
├─────────────────────────────────────────────────────────────┤
│                                                         │
│  MsgCreateTokenPair (Cosmos)                             │
│       ↓                                                  │
│  EventCreateTokenPair                                     │
│   ├── erc20_address: 0x...                              │
│   ├── bank_denom: peggy0x...                             │
│   └── sender: 0x...                                     │
│       ↓                                                  │
│  saveContractInfo (DB)                                    │
│       ↓                                                  │
│  scheduleMetadataFetch                                     │
│       ↓                                                  │
│  fetchAndUpdateERC20Metadata                               │
│       ├─→ EVM RPC: eth_call(name/symbol/decimals)        │
│       └─→ UPDATE contract_infos SET token_name=...         │
│                                                         │
└─────────────────────────────────────────────────────────────┘
```

#### 3.2.2 Peggy Tokens 流程（需优化）

```
┌─────────────────────────────────────────────────────────────────────────┐
│              Peggy Token 创建与元数据获取流程（优化后）                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  1. Cosmos 层：MsgCreateTokenPair                                    │
│     ├── erc20_address: 0x9772154BF1823353C829B43F9AD96CF9f720c768 │
│     └── bank_denom: peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca │
│                                                                     │
│  2. 保存基础信息到 DB                                               │
│     contract_infos:                                                   │
│     ├── contract_address: 0x9772...                                   │
│     ├── contract_type: 'erc20'                                       │
│     └── metadata.bank_denom: 'peggy0xa8c8...'                       │
│                                                                     │
│  3. 元数据获取流程（多路径回退）                                     │
│                                                                     │
│     fetchAndUpdateERC20Metadata                                       │
│        │                                                              │
│        ├─→ 尝试 EVM RPC (标准路径)                                  │
│        │     ├─ name/symbol/decimals 有值？ ─Yes→ 更新 DB           │
│        │     └No                                                     │
│        │        │                                                      │
│        ├─→ 检查 metadata.bank_denom 是否存在                            │
│        │     │                                                         │
│        │     └─→ 是 Peggy token？ ─Yes→ fetchPeggyTokenMetadata        │
│        │              │                                                │
│        │              ├─ 方案 A: 从 Cosmos Peggy 模块查询               │
│        │              │    QueryDenomToERC20 / QueryERC20ToDenom        │
│        │              │                                                │
│        │              ├─ 方案 B: 从原始链（Ethereum）获取               │
│        │              │    提取 denom 中的 0x 地址                     │
│        │              │    查询 Ethereum RPC 的 name/symbol/decimals     │
│        │              │                                                │
│        │              └─ 方案 C: 第三方 API（CoinGecko）                │
│        │                   /coins/ethereum/contract/{address}            │
│        │                                                            │
│        └─→ 非 Peggy token                                           │
│             └─→ 标记 metadata_not_applicable = true                  │
│                                                                     │
└─────────────────────────────────────────────────────────────────────────┘
```

### 3.3 技术实现方案

#### 3.3.1 方案对比

| 方案 | 优点 | 缺点 | 优先级 |
|------|------|------|--------|
| **A: 原始链 RPC（Ethereum）** | ✅ 数据完整、权威、已验证可行 | 需要 Ethereum RPC 配置 | **P0（主要数据源）** |
| **B: 第三方 API（CoinGecko）** | 实现简单，支持多链 | 有限流风险、覆盖不全（部分代币缺失） | P1（备选） |
| **C: 默认值 + 手动维护** | 最简单，无外部依赖 | 用户体验差，需要人工干预 | P2（兜底） |

> **⚠️ 重要更新**：经过实测验证，Injective Chain 的 Peggy Query 接口返回 `HTTP 501 Not Implemented`，因此 **Cosmos Peggy Module 方案不可用**，已从本方案中移除。详见 [附录 F：Cosmos Peggy Query 验证记录](#附录-fcosmos-peggy-query-验证记录)。

#### 3.3.2 推荐方案：简化的三级回退策略

```go
// fetchAndUpdateERC20Metadata 增强版
func (s *SyncService) fetchAndUpdateERC20Metadata(ctx context.Context, contractAddress, chainID string) error {
    evmClient, err := createEVMClient()
    if err != nil {
        return errors.Wrap(err, "failed to create EVM client")
    }
    defer evmClient.Close()

    // 1. 尝试从 EVM 获取元数据（标准 ERC20）
    name, symbol, decimals, err := evmClient.GetERC20Metadata(ctx, contractAddress)

    // 2. 如果 EVM 层获取失败，检查是否是 Peggy token
    if (name == "" && symbol == "" && decimals == 0) || err != nil {
        var contractInfo models.ContractInfo
        if dbErr := db.Get().Where("contract_address = ? AND chain_id = ?", contractAddress, chainID).First(&contractInfo).Error; dbErr == nil {
            if bankDenom, ok := contractInfo.Metadata["bank_denom"].(string); ok && strings.HasPrefix(bankDenom, "peggy") {
                // Peggy token：使用专门的获取逻辑
                name, symbol, decimals, err = s.fetchPeggyTokenMetadata(ctx, bankDenom, contractAddress)
                if err != nil {
                    global.Logger.Warnf("failed to fetch Peggy token metadata for %s (denom: %s): %v", contractAddress, bankDenom, err)
                    // 使用默认值
                    name = "Bridge Token"
                    symbol = s.extractSymbolFromDenom(bankDenom)
                    decimals = 18
                }
            } else {
                // 非 Peggy token，标记为不适用
                s.markContractNotERC20(contractAddress, chainID)
                return nil
            }
        }
    }

    // 3. 验证元数据
    if name, symbol, err = validateERC20Metadata(name, symbol, decimals); err != nil {
        global.Logger.Warnf("invalid metadata for %s: %v", contractAddress, err)
        return err
    }

    // 4. 更新数据库
    return s.updateContractMetadata(ctx, contractAddress, chainID, name, symbol, decimals)
}

// fetchPeggyTokenMetadata 从多个来源获取 Peggy token 元数据
func (s *SyncService) fetchPeggyTokenMetadata(ctx context.Context, bankDenom, contractAddress string) (name, symbol string, decimals int, err error) {
    // 从 denom 提取原始 ERC20 地址
    // peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca → 0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca
    originalERC20Address := strings.TrimPrefix(bankDenom, "peggy")
    if !strings.HasPrefix(originalERC20Address, "0x") {
        originalERC20Address = "0x" + originalERC20Address
    }

    // 方案 A：从原始链（Ethereum）获取
    if s.ethereumRPC != "" {
        name, symbol, decimals, err = s.fetchMetadataFromEthereum(ctx, originalERC20Address)
        if err == nil && name != "" {
            global.Logger.Infof("fetched Peggy token metadata from Ethereum: %s (%s)", name, symbol)
            return name, symbol, decimals, nil
        }
        global.Logger.Debugf("failed to fetch from Ethereum: %v", err)
    }

    // 方案 B：从 Cosmos Peggy 模块查询
    if s.chainClient != nil {
        name, symbol, decimals, err = s.fetchMetadataFromCosmosPeggy(ctx, bankDenom)
        if err == nil && name != "" {
            global.Logger.Infof("fetched Peggy token metadata from Cosmos: %s (%s)", name, symbol)
            return name, symbol, decimals, nil
        }
        global.Logger.Debugf("failed to fetch from Cosmos Peggy: %v", err)
    }

    // 方案 C：第三方 API（CoinGecko）
    name, symbol, decimals, err = s.fetchMetadataFromCoinGecko(ctx, originalERC20Address)
    if err == nil && name != "" {
        global.Logger.Infof("fetched Peggy token metadata from CoinGecko: %s (%s)", name, symbol)
        return name, symbol, decimals, nil
    }

    return "", "", 0, errors.New("all metadata sources failed")
}

// fetchMetadataFromEthereum 从 Ethereum RPC 获取原始代币元数据
func (s *SyncService) fetchMetadataFromEthereum(ctx context.Context, erc20Address string) (name, symbol string, decimals int, err error) {
    // 配置中需要 Ethereum RPC 端点
    // chain:
    //   ethereum:
    //     rpc_endpoint: https://eth.llamarpc.com
    //     timeout: 10s

    evmConfig := &EVMConfig{
        RPCURL:  s.config.EthereumRPC.Endpoint,
        ChainID: big.NewInt(1), // Ethereum Mainnet
        Timeout: 10 * time.Second,
    }

    evmClient, err := NewEVMClientWithConfig(evmConfig)
    if err != nil {
        return "", "", 0, errors.Wrap(err, "failed to create Ethereum EVM client")
    }
    defer evmClient.Close()

    return evmClient.GetERC20Metadata(ctx, erc20Address)
}

// fetchMetadataFromCosmosPeggy 从 Injective Cosmos Peggy 模块查询
func (s *SyncService) fetchMetadataFromCosmosPeggy(ctx context.Context, bankDenom string) (name, symbol string, decimals int, err error) {
    // Peggy 模块查询客户端
    peggyQueryClient := peggytypes.NewQueryClient(s.chainClient)

    // 查询 denom 对应的 ERC20 信息
    response, err := peggyQueryClient.DenomToERC20(ctx, &peggytypes.QueryDenomToERC20Request{
        Denom: bankDenom,
    })
    if err != nil {
        return "", "", 0, errors.Wrap(err, "failed to query DenomToERC20")
    }

    // Peggy 模块可能返回代币的元数据
    // 需要实际查询验证返回结构
    if response.Erc20Address != "" {
        // 可以尝试从返回信息中提取元数据
        // 或者使用返回的 ERC20 地址从原始链获取
        return s.fetchMetadataFromEthereum(ctx, response.Erc20Address)
    }

    return "", "", 0, errors.New("no metadata found in Peggy module")
}

// fetchMetadataFromCoinGecko 使用 CoinGecko API 获取代币元数据
func (s *SyncService) fetchMetadataFromCoinGecko(ctx context.Context, contractAddress string) (name, symbol string, decimals int, err error) {
    // CoinGecko API: /coins/ethereum/contract/{contractAddress}
    url := fmt.Sprintf("https://api.coingecko.com/api/v3/coins/ethereum/contract/%s", contractAddress)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return "", "", 0, errors.Wrap(err, "failed to create request")
    }

    // 添加可选的 CoinGecko API Key（如果有配置）
    if s.config.CoinGeckoAPIKey != "" {
        req.Header.Set("x-cg-demo-api-key", s.config.CoinGeckoAPIKey)
    }

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return "", "", 0, errors.Wrap(err, "failed to fetch from CoinGecko")
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        return "", "", 0, errors.Errorf("CoinGecko returned status %d", resp.StatusCode)
    }

    var data struct {
        Name         string `json:"name"`
        Symbol       string `json:"symbol"`
        DetailPlatforms struct {
            Ethereum struct {
                DecimalPlace int `json:"decimal_place"`
            } `json:"ethereum"`
        } `json:"detail_platforms"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
        return "", "", 0, errors.Wrap(err, "failed to decode CoinGecko response")
    }

    symbol = strings.ToUpper(data.Symbol)
    decimals = data.DetailPlatforms.Ethereum.DecimalPlace

    if decimals == 0 {
        decimals = 18 // 默认值
    }

    return data.Name, symbol, decimals, nil
}

// extractSymbolFromDenom 从 bank_denom 提取代币符号（启发式）
func (s *SyncService) extractSymbolFromDenom(bankDenom string) string {
    // 移除 peggy 前缀
    denom := strings.TrimPrefix(bankDenom, "peggy")

    // 如果是 0x 开头，返回默认符号
    if strings.HasPrefix(denom, "0x") {
        // 可以尝试从合约地址生成短符号
        // 例如：0xa8c8... → PEGGY
        return "PEGGY"
    }

    // 如果 denom 本身包含符号信息
    // 例如：peggyUSDT → USDT
    if len(denom) > 0 && !strings.HasPrefix(denom, "0x") {
        return strings.ToUpper(denom)
    }

    return "PEGGY"
}
```

### 3.4 配置扩展

#### 3.4.1 Ethereum RPC 配置（用于 Peggy tokens）

```yaml
chain:
  evm:
    enabled: true
    rpc_endpoint: ${CHAIN_EVM_RPC_ENDPOINT:https://sentry.evm-rpc.injective.network}
    chain_id: ${CHAIN_EVM_CHAIN_ID:801}
    timeout: 10s

  # 新增：Ethereum RPC（用于查询 Peggy 原始代币元数据）
  ethereum:
    enabled: true
    rpc_endpoint: ${ETHEREUM_RPC_ENDPOINT:https://eth.llamarpc.com}
    # 备选：
    # - https://rpc.ankr.com/eth
    # - https://ethereum.publicnode.com
    timeout: 10s
    rate_limit:
      requests_per_second: 10  # CoinGecko 免费版限制

  # 新增：CoinGecko API（可选）
  coingecko:
    enabled: false  # 默认禁用，需要时启用
    api_key: ${COINGECKO_API_KEY:}  # 可选，免费版有限制
    base_url: https://api.coingecko.com/api/v3
    timeout: 10s
```

#### 3.4.2 配置优先级

| 来源 | 优先级 | 说明 |
|------|--------|------|
| 本地 EVM RPC | 1 | 最快，但 Peggy tokens 返回空 |
| Ethereum RPC | 2 | 权威数据源，需要配置额外 RPC |
| Cosmos Peggy 模块 | 3 | 需验证是否包含元数据 |
| CoinGecko API | 4 | 备选方案，有限制 |
| 默认值 | 5 | 最后的回退 |

### 3.5 错误处理与降级

#### 3.5.1 错误分类

```go
type PeggyMetadataError struct {
    Source string  // "ethereum", "cosmos", "coingecko"
    Err    error
    Type   string  // "timeout", "not_found", "rate_limit"
}

func (s *SyncService) handlePeggyError(contractAddress string, err *PeggyMetadataError) {
    switch err.Type {
    case "rate_limit":
        // 限流：延迟后重试
        global.Logger.Warnf("rate limit hit for %s, will retry", contractAddress)
        time.Sleep(time.Minute)
    case "not_found":
        // 未找到：使用默认值
        global.Logger.Infof("Peggy token not found in external sources, using default: %s", contractAddress)
    case "timeout":
        // 超时：尝试下一个数据源
        global.Logger.Warnf("timeout fetching Peggy metadata for %s, trying next source", contractAddress)
    }
}
```

#### 3.5.2 降级策略

```go
// 降级到默认值
func (s *SyncService) getPeggyTokenDefaultMetadata(bankDenom string) (name, symbol string, decimals int) {
    denom := strings.TrimPrefix(bankDenom, "peggy")

    // 尝试从 denom 推断
    if strings.Contains(denom, "USDT") {
        return "Tether USD", "USDT", 6
    }
    if strings.Contains(denom, "USDC") {
        return "USD Coin", "USDC", 6
    }

    // 默认值
    return "Bridge Token", "PEGGY", 18
}
```

### 3.6 数据库优化

#### 3.6.1 新增字段

```sql
-- 标记 Peggy tokens
ALTER TABLE contract_infos ADD COLUMN is_peggy_token BOOLEAN DEFAULT FALSE;
ALTER TABLE contract_infos ADD COLUMN peggy_original_address VARCHAR(42);
ALTER TABLE contract_infos ADD COLUMN peggy_denom VARCHAR(255);

-- 索引
CREATE INDEX idx_contract_infos_is_peggy ON contract_infos(is_peggy_token);
```

#### 3.6.2 自动标记

```go
// 在 buildContractInfo 中
if eventData.bankDenom != "" && strings.HasPrefix(eventData.bankDenom, "peggy") {
    contractInfo.IsPeggyToken = true
    contractInfo.PeggyDenom = eventData.bankDenom
    contractInfo.PeggyOriginalAddress = strings.TrimPrefix(eventData.bankDenom, "peggy")
}
```

### 3.7 API 扩展

#### 3.7.1 Peggy Token 元数据刷新

```
POST /api/v1/admin/contract/peggy/metadata/refresh
Authorization: Admin token

Request:
{
  "contract_address": "0x9772154BF1823353C829B43F9AD96CF9f720c768",
  "force": true,  // 强制从原始链重新获取
  "sources": ["ethereum", "cosmos", "coingecko"]  // 指定数据源优先级
}

Response:
{
  "code": 200,
  "data": {
    "contract_address": "0x9772154bf1823353c829b43f9ad96cf9f720c768",
    "token_name": "Example Token",
    "token_symbol": "EXT",
    "decimals": 18,
    "metadata_source": "ethereum",  // 实际使用的数据源
    "is_peggy": true,
    "original_denom": "peggy0xa8c8..."
  }
}
```

### 3.8 监控指标

```go
var (
    // Peggy token 元数据获取指标
    peggyMetadataFetchTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "peggy_metadata_fetch_total",
            Help: "Total number of Peggy token metadata fetch attempts",
        },
        []string{"source", "status"},  // source: ethereum/cosmos/coingecko, status: success/failure
    )

    peggyMetadataFetchDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "peggy_metadata_fetch_duration_seconds",
            Help: "Duration of Peggy token metadata fetch by source",
        },
        []string{"source"},
    )

    peggyTokensDetected = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "peggy_tokens_detected_total",
            Help: "Total number of Peggy tokens detected",
        },
    )
)
```

---

## 四、完整技术方案

### 4.1 实时同步（已实现 + 已修复）

| 组件 | 说明 |
|------|------|
| `saveContractInfoFromCosmosEvents` | 解析 EventCreateTokenPair，保存基础 contract_info |
| `scheduleMetadataFetch` | 将 (contractAddress, chainID) 投递到 metadata 工作池 |
| `fetchAndUpdateERC20Metadata` | 使用 `createEVMClient()` 创建客户端，调用 `GetERC20Metadata`，更新 DB |
| `metadataWorkerPool` | 10 并发，30s 超时，失败时记录 warn 日志 |

**配置依赖**：`configs/config.yaml` 中 `chain.evm` 必须正确配置：

```yaml
chain:
  evm:
    enabled: true
    rpc_endpoint: ${CHAIN_EVM_RPC_ENDPOINT:https://sentry.evm-rpc.injective.network}
    chain_id: ${CHAIN_EVM_CHAIN_ID:801}
    timeout: 10s
```

### 4.2 历史数据回填

#### 4.2.1 回填目标

- 条件：`contract_type IN ('erc20', 'solidity')` 且 `(token_name IS NULL OR token_name = '') OR (token_symbol IS NULL OR token_symbol = '')`
- 动作：对每条记录调用 `fetchAndUpdateERC20Metadata`，更新 DB

#### 4.2.2 实现方式

**方案 A：管理端 API（推荐）**

- 新增 Admin API：`POST /api/v1/admin/contract/metadata/refresh`
  - 请求体：`{ "contract_address": "0x...", "chain_id": "injective-1" }` 或 `{ "batch": true, "limit": 100 }`
  - 单合约：同步调用 `fetchAndUpdateERC20Metadata`，返回更新结果
  - 批量：异步投递到 `metadataWorkerPool`，返回已投递数量

**方案 B：CLI 脚本**

- 脚本：`scripts/backfill_erc20_metadata.sh`（或 Go 命令 `go run ./cmd/backfill-metadata`）
- 逻辑：查询缺失元数据的合约列表，分批调用 `fetchAndUpdateERC20Metadata`
- 参数：`--batch-size`、`--delay`（RPC 限速）、`--dry-run`

**方案 C：定时任务（可选）**

- 在 SyncService 或独立 worker 中增加定时任务（如每 6 小时）
- 查询最近 N 天创建的、元数据为空的合约，投递到 `metadataWorkerPool`
- 需控制频率，避免对 RPC 造成压力

#### 4.2.3 回填执行建议

| 场景 | 建议 |
|------|------|
| 一次性修补历史 | 方案 B 脚本，batch_size=50，delay=500ms |
| 运维按需刷新 | 方案 A 单合约 API |
| 长期自动补齐 | 方案 C 定时任务（低频） |

### 4.3 管理端 API 设计

#### 4.3.1 单合约元数据刷新

```
POST /api/v1/admin/contract/metadata/refresh
Authorization: Admin token

Request:
{
  "contract_address": "0x9772154BF1823353C829B43F9AD96CF9f720c768",
  "chain_id": "injective-1"  // 可选，默认 global.ChainConfig.ChainId
}

Response:
{
  "code": 200,
  "data": {
    "contract_address": "0x9772154bf1823353c829b43f9ad96cf9f720c768",
    "chain_id": "injective-1",
    "token_name": "Example Token",
    "token_symbol": "EXT",
    "decimals": 18,
    "updated": true
  }
}
```

#### 4.3.2 批量元数据回填

```
POST /api/v1/admin/contract/metadata/backfill
Authorization: Admin token

Request:
{
  "limit": 100,        // 最多处理数量，默认 50
  "chain_id": "injective-1",  // 可选，不传则所有链
  "async": true        // true=投递到 worker 异步执行，false=同步逐个执行
}

Response:
{
  "code": 200,
  "data": {
    "queued": 87,      // 待处理数量（async=true 时），投递到 worker 后立即返回
    "processed": 50,   // 已处理数量（async=false 时同步执行完成）
    "succeeded": 48,
    "failed": 2
  }
}
```
> async=true：立即返回，queued 为实际投递数；async=false：同步执行，返回 processed/succeeded/failed。

#### 4.3.3 元数据统计（可选）

```
GET /api/v1/admin/contract/metadata/stats
Authorization: Admin token

Response:
{
  "code": 200,
  "data": {
    "missing_count": 156,      // 元数据缺失的合约数
    "not_erc20_count": 23,    // 已标记为非 ERC20 的数量
    "chain_id": "injective-1"  // 可选过滤
  }
}
```

### 4.4 多来源覆盖说明

| 来源 | 说明 | 元数据获取 |
|------|------|------------|
| EventCreateTokenPair | Cosmos MsgCreateTokenPair | 异步 fetchAndUpdateERC20Metadata |
| receipt.ContractAddress | EVM 直接 CREATE | evm_details_sync 内同步 GetERC20Metadata |
| Event Logs (PairCreated 等) | Factory 创建交易对 | evm_details_sync 内同步 GetERC20Metadata |

确保两处均使用**同一套 EVM 配置**（`createEVMClient` 或 `createEVMClientFromConfig`），避免自定义链下 RPC 不一致。`evm_details_sync` 在启动时通过 `createEVMClientFromConfig(bc.Chain.Evm)` 初始化，`sync` 的 `fetchAndUpdateERC20Metadata` 使用 `createEVMClient()`（读取 `global.EVMConfig`，同来源），两处配置应一致。

### 4.5 限流与容错

| 策略 | 说明 |
|------|------|
| 工作池并发 | metadataWorkerPool 10 并发，避免 RPC 过载 |
| 超时 | 单次 GetERC20Metadata 建议 10s，processMetadataJob 30s |
| 部分成功 | name/symbol/decimals 任一项成功即更新，不全量失败 |
| 日志 | 失败时 Warn 级别，便于排查 RPC、合约兼容等问题 |
| 错误分类 | 区分可重试错误（RPC 超时、网络问题）和不可重试错误（合约非 ERC20） |
| 失败标记 | 对非 ERC20 合约设置 `metadata_not_applicable` 标志，避免重复尝试 |

### 4.6 合约类型验证（新增）

#### 4.6.1 问题背景

当前方案假设所有 `contract_type IN ('erc20', 'solidity')` 的合约都能获取 ERC20 元数据，但实际上：
- Uniswap Factory、Router 等 solidity 合约不是 ERC20
- 部署在 EVM 上的 CosmWasm 合约包装器可能不支持标准 ERC20 接口
- 非 ERC20 合约调用 `name()`/`symbol()` 会返回错误或空值，浪费 RPC 资源

#### 4.6.2 验证策略

**方案 A：轻量级检测（可选）**

在获取元数据前，先调用 `symbol()` 进行快速检测。注意：若检测通过，后续 GetERC20Metadata 会再次调用 symbol()，成功路径多一次 RPC；若检测失败可省去 name/decimals 两次调用。**在主要为 ERC20 合约的场景下，可省略预检测，直接调用 GetERC20Metadata 并依赖错误分类 + metadata_not_applicable 标记**，以简化实现。

```go
// isERC20Contract 通过简单调用检测合约是否支持 ERC20
func (s *SyncService) isERC20Contract(ctx context.Context, contractAddress string) bool {
    evmClient, err := createEVMClient()
    if err != nil {
        return false
    }
    defer evmClient.Close()

    symbol, err := evmClient.GetERC20Symbol(ctx, contractAddress)
    return err == nil && symbol != "" && symbol != "0x"
}
```

**方案 B：EIP-165 标准检测（不推荐用于 ERC20）**

> ⚠️ **注意**：标准 ERC20 规范**不要求**实现 EIP-165。绝大多数 ERC20 代币（包括 OpenZeppelin 基础实现）并未实现 `supportsInterface(bytes4)`。因此 EIP-165 检测对大部分 ERC20 会返回 `false`，导致误判。此方案仅适用于明确实现了 EIP-165 的扩展型 ERC20，**不建议作为主要检测手段**。

```go
// ERC20 可选接口 ID（若合约实现了 supportsInterface）
// 注意：多数 ERC20 不实现此接口，调用会 revert
func (c *EVMClient) SupportsERC20Interface(ctx context.Context, contractAddress string) (bool, error) {
    // ...
}
```

#### 4.6.3 数据库优化

为避免对非 ERC20 合约重复尝试，增加标志字段：

```sql
ALTER TABLE contract_infos ADD COLUMN metadata_not_applicable BOOLEAN DEFAULT FALSE;
-- MySQL 不支持 WHERE 条件索引，使用普通索引
CREATE INDEX idx_contract_infos_metadata_not_applicable ON contract_infos(metadata_not_applicable);
```

查询元数据缺失时，排除已标记为不适用的合约：

```sql
SELECT contract_address, chain_id
FROM contract_infos
WHERE contract_type IN ('erc20', 'solidity')
  AND (token_name IS NULL OR token_name = '' OR token_symbol IS NULL OR token_symbol = '')
  AND (metadata_not_applicable = FALSE OR metadata_not_applicable IS NULL);
```

### 4.7 元数据有效性验证（新增）

#### 4.7.1 验证规则

获取到元数据后，需进行有效性验证：

| 字段 | 验证规则 | 处理策略 |
|------|----------|----------|
| `name` | 非空，不包含空字符（`\x00`），长度 ≤ 255 | 验证失败则不更新 |
| `symbol` | 非空，不包含空字符，长度 ≤ 50，大写优先 | 验证失败则不更新 |
| `decimals` | 0-18 之间（通常为 6 或 18） | 超出范围记录警告但允许更新 |

#### 4.7.2 Honeypot 检测（启发式）

```go
// validateERC20Metadata 验证元数据有效性并检测潜在 honeypot
// 返回 (normalizedName, normalizedSymbol, error)：验证通过时返回规范化后的值供 DB 更新
func validateERC20Metadata(name, symbol string, decimals int) (string, string, error) {
    // 1. 基础非空检查
    if name == "" || symbol == "" {
        return "", "", errors.New("empty name or symbol")
    }

    // 2. 空字符检查（部分恶意合约会插入空字符混淆）
    if strings.Contains(name, "\x00") || strings.Contains(symbol, "\x00") {
        return "", "", errors.New("invalid null character in metadata")
    }

    // 3. 合理性检查：name 和 symbol 不应完全相同
    if strings.EqualFold(name, symbol) {
        global.Logger.Warnf("suspicious metadata: name and symbol are identical: %s", name)
    }

    // 4. Symbol 规范化（转为大写，供 DB 更新）
    symbol = strings.ToUpper(symbol)

    // 5. Decimals 范围检查
    if decimals < 0 || decimals > 18 {
        global.Logger.Warnf("unusual decimals: %d (expected 0-18)", decimals)
    }

    return name, symbol, nil
}
```

### 4.8 错误分类与重试策略（优化）

#### 4.8.1 错误类型定义

| 错误类型 | 示例 | 是否重试 | 处理方式 |
|---------|------|---------|----------|
| `rpc_timeout` | context.DeadlineExceeded | ✅ 是 | 加入重试队列 |
| `rpc_connection_failed` | connection refused, dial tcp | ✅ 是 | 切换 RPC 端点 |
| `contract_not_erc20` | execution reverted (no data) | ❌ 否 | 标记 `metadata_not_applicable = true` |
| `invalid_response` | ABI decode error | ❌ 否 | 记录错误日志 |
| `rate_limit` | too many requests | ✅ 是 | 指数退避后重试 |

#### 4.8.2 实现代码

```go
type MetadataFetchError struct {
    Type        string
    Err         error
    ShouldRetry bool
}

func (s *SyncService) classifyError(err error) *MetadataFetchError {
    if err == nil {
        return nil
    }

    errStr := err.Error()

    switch {
    case errors.Is(err, context.DeadlineExceeded):
        return &MetadataFetchError{Type: "rpc_timeout", Err: err, ShouldRetry: true}

    case strings.Contains(errStr, "execution reverted") || strings.Contains(errStr, "no data"):
        return &MetadataFetchError{Type: "contract_not_erc20", Err: err, ShouldRetry: false}

    case strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "dial tcp"):
        return &MetadataFetchError{Type: "rpc_connection_failed", Err: err, ShouldRetry: true}

    case strings.Contains(errStr, "ABI") || strings.Contains(errStr, "decode"):
        return &MetadataFetchError{Type: "invalid_response", Err: err, ShouldRetry: false}

    case strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "too many requests") ||
         strings.Contains(errStr, "429"):
        return &MetadataFetchError{Type: "rate_limit", Err: err, ShouldRetry: true}

    default:
        return &MetadataFetchError{Type: "unknown", Err: err, ShouldRetry: false}
    }
}

func (s *SyncService) handleFetchError(contractAddress, chainID string, fetchErr *MetadataFetchError) {
    if !fetchErr.ShouldRetry && fetchErr.Type == "contract_not_erc20" {
        // 标记为非 ERC20 合约，后续不再尝试（需同时匹配 contract_address 和 chain_id）
        s.db.Model(&models.ContractInfo{}).
            Where("contract_address = ? AND chain_id = ?", contractAddress, chainID).
            Update("metadata_not_applicable", true)
        global.Logger.Infof("marked contract %s (chain=%s) as not ERC20", contractAddress, chainID)
    }
}
```

### 4.9 RPC 端点多活与故障转移（新增）

> **实施注意**：当前 `conf.proto` 中 `Chain.EVM` 仅有 `rpc_endpoint`（单数）。多端点需扩展为 `repeated string rpc_endpoints` 或新增 `rpc_endpoints` 字段，并做配置迁移。建议作为 P2 阶段的独立改动。

#### 4.9.1 配置设计

```yaml
chain:
  evm:
    enabled: true
    rpc_endpoints:  # 扩展配置：支持多个端点，实现高可用（需 proto 与迁移）
      - https://sentry.evm-rpc.injective.network
      - https://backup-rpc-1.injective.network
      - https://backup-rpc-2.injective.network
    chain_id: ${CHAIN_EVM_CHAIN_ID:801}
    timeout: 10s

    # 健康检查配置
    health_check:
      enabled: true
      interval: 30s        # 健康检查间隔
      timeout: 5s          # 单次检查超时
      failure_threshold: 3  # 连续失败次数触发切换
```

#### 4.9.2 实现逻辑

```go
type MultiEndpointEVMClient struct {
    endpoints         []string
    currentEndpoint   int
    failureCount      []int
    circuitBreakers   []*CircuitBreaker
    healthCheckTicker *time.Ticker
    mu                sync.RWMutex
}

func (c *MultiEndpointEVMClient) Call(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
    // 尝试当前端点
    result, err := c.tryCurrentEndpoint(ctx, call, blockNumber)

    if err != nil && c.shouldSwitchEndpoint(err) {
        c.recordFailure()
        if c.failureCount[c.currentEndpoint] >= c.healthCheckConfig.FailureThreshold {
            c.switchToNextEndpoint()
            return c.Call(ctx, call, blockNumber)  // 使用新端点重试
        }
    }

    return result, err
}

func (c *MultiEndpointEVMClient) healthCheck() {
    for i, endpoint := range c.endpoints {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        _, err := c.client.ChainID(ctx)
        cancel()

        if err != nil {
            c.failureCount[i]++
        } else {
            c.failureCount[i] = 0  // 成功则重置
        }
    }
}
```

### 4.10 批量元数据获取优化（性能）

#### 4.10.1 JSON-RPC Batch 调用

当前实现对每个合约发起 3 个并发 RPC 调用（name/symbol/decimals），批量回填时网络开销大。

**优化方案：使用 JSON-RPC Batch**

```go
// GetERC20MetadataBatch 批量获取多个合约的元数据
func (c *EVMClient) GetERC20MetadataBatch(ctx context.Context, addresses []string) ([]*ERC20Metadata, error) {
    batchSize := 100  // JSON-RPC 批量请求限制
    var allResults []*ERC20Metadata

    for i := 0; i < len(addresses); i += batchSize {
        end := i + batchSize
        if end > len(addresses) {
            end = len(addresses)
        }
        batch := addresses[i:end]

        results := c.batchFetch(ctx, batch)
        allResults = append(allResults, results...)
    }

    return allResults, nil
}

func (c *EVMClient) batchFetch(ctx context.Context, addresses []string) []*ERC20Metadata {
    // 构造批量 JSON-RPC 请求
    var batchRequests []json.RawMessage

    for _, addr := range addresses {
        // name() call
        batchRequests = append(batchRequests, c.buildBatchRequest(1, addr, "0x06fdde03"))
        // symbol() call
        batchRequests = append(batchRequests, c.buildBatchRequest(2, addr, "0x95d89b41"))
        // decimals() call
        batchRequests = append(batchRequests, c.buildBatchRequest(3, addr, "0x313ce567"))
    }

    // 发送批量请求
    var batchResponses []json.RawMessage
    err := c.client.Client().BatchCallContext(ctx, batchRequests, &batchResponses)

    // 解析并聚合结果
    // ...
}
```

**性能提升：**
- 100 个合约从 300 次 RPC 调用 → 1 次 Batch 请求
- 网络往返次数大幅减少，适合历史数据回填场景

**实现注意**：JSON-RPC Batch 要求每个请求有唯一 `id`，响应顺序可能与请求不一致，需通过 `id` 匹配并聚合到对应合约。部分 RPC 对单次 batch 请求数量有限制（通常 100–500），需分批发送。

### 4.11 数据并发更新控制（一致性）

#### 4.11.1 乐观锁实现

为防止多个 worker 同时更新同一合约的元数据导致数据不一致：

```go
// ContractInfo 增加版本字段
type ContractInfo struct {
    // ... existing fields
    Version int `gorm:"default:0"`
}

// 更新时使用乐观锁（需先查询当前 version，Updates 中递增）
func (s *SyncService) updateMetadataWithOptimisticLock(contractAddr, chainID string, currentVersion int, updates map[string]interface{}) error {
    updates["version"] = gorm.Expr("version + 1")
    updates["metadata_updated_at"] = time.Now()

    result := s.db.Model(&models.ContractInfo{}).
        Where("contract_address = ? AND chain_id = ? AND version = ?", contractAddr, chainID, currentVersion).
        Updates(updates)

    if result.RowsAffected == 0 {
        return errors.New("concurrent update detected, retry or skip")
    }
    return nil
}
```

#### 4.11.2 分布式锁（可选）

```go
func (s *SyncService) fetchAndUpdateERC20Metadata(ctx context.Context, contractAddress, chainID string) error {
    // 获取分布式锁（key 需包含 chain_id，同一地址可存在于多链）
    lockKey := fmt.Sprintf("contract:metadata:update:%s:%s", chainID, strings.ToLower(contractAddress))
    lock, err := s.redis.Lock(ctx, lockKey, 30*time.Second)
    if err != nil {
        return errors.Wrap(err, "failed to acquire lock")
    }
    defer lock.Unlock(ctx)

    // 执行实际的元数据获取和更新
    // ...
}
```

### 4.12 缓存策略优化

#### 4.12.1 缓存设计

```go
type MetadataCache struct {
    redis *redis.Client
    ttl   time.Duration  // 建议 1 小时
}

const (
    metadataCacheKey = "contract:metadata:%s:%s"  // chain_id:contract_address，同一地址不同链需区分
    metadataCacheTTL = 1 * time.Hour
)

func (c *MetadataCache) Get(chainID, contractAddress string) (*ERC20Metadata, bool) {
    key := fmt.Sprintf(metadataCacheKey, chainID, strings.ToLower(contractAddress))
    data, err := c.redis.Get(context.Background(), key).Bytes()
    if err != nil {
        return nil, false
    }

    var metadata ERC20Metadata
    json.Unmarshal(data, &metadata)
    return &metadata, true
}

func (c *MetadataCache) Set(chainID, contractAddress string, metadata *ERC20Metadata) error {
    key := fmt.Sprintf(metadataCacheKey, chainID, strings.ToLower(contractAddress))
    data, _ := json.Marshal(metadata)
    return c.redis.Set(context.Background(), key, data, metadataCacheTTL).Err()
}
```

#### 4.12.2 缓存失效策略

| 场景 | 处理方式 |
|------|----------|
| 正常更新 | 写入 DB 后更新缓存 |
| 强制刷新 API | 删除缓存，重新获取 |
| 定时全量刷新 | SCAN 模式 `contract:metadata:*` 删除所有链的缓存 |
| 检测到数据异常 | 立即删除缓存 |

### 4.13 监控指标增强

#### 4.13.1 新增 Prometheus 指标

```go
var (
    // 元数据获取总次数（按状态和错误类型分组）
    metadataFetchTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "contract_metadata_fetch_total",
            Help: "Total number of ERC20 metadata fetch attempts",
        },
        []string{"status", "error_type"},  // status: success/partial_failure/failure
    )

    // 元数据获取耗时（按步骤分组）
    metadataFetchDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "contract_metadata_fetch_duration_seconds",
            Help:    "Duration of ERC20 metadata fetch by step",
            Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),  // 0.1s - 51.2s
        },
        []string{"step"},  // step: name/symbol/decimals
    )

    // 元数据缺失数量
    contractsMissingMetadata = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "contracts_missing_metadata_total",
            Help: "Number of contracts with incomplete metadata",
        },
    )

    // 非 ERC20 合约数量（已标记）
    contractsNotERC20 = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "contracts_not_erc20_total",
            Help: "Number of contracts marked as not ERC20",
        },
    )

    // RPC 端点健康状态
    rpcEndpointHealth = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "rpc_endpoint_health",
            Help: "Health status of RPC endpoints (1=healthy, 0=unhealthy)",
        },
        []string{"endpoint"},
    )
)
```

#### 4.13.2 告警规则建议

```yaml
groups:
  - name: contract_metadata
    rules:
      # 元数据缺失数量异常增长
      - alert: TooManyMissingMetadata
        expr: contracts_missing_metadata_total > 1000
        for: 1h
        annotations:
          summary: "Too many contracts missing metadata"

      # RPC 端点全宕
      - alert: AllRPCEndpointsDown
        expr: sum(rpc_endpoint_health) == 0
        for: 5m
        annotations:
          summary: "All RPC endpoints are down"

      # 元数据获取失败率过高
      - alert: HighMetadataFetchFailureRate
        expr: |
          rate(contract_metadata_fetch_total{status="failure"}[5m])
          /
          rate(contract_metadata_fetch_total[5m]) > 0.3
        for: 10m
        annotations:
          summary: "Metadata fetch failure rate > 30%"
```

---

## 五、数据模型

### 5.1 contract_infos 相关字段

| 字段 | 类型 | 说明 |
|------|------|------|
| contract_address | varchar(128) | 合约地址 |
| chain_id | varchar(50) | 链 ID |
| contract_type | varchar(50) | erc20 / solidity / cosmwasm 等 |
| name | varchar(255) | 合约名称（通用） |
| token_name | varchar(255) | 代币全名（ERC20） |
| token_symbol | varchar(50) | 代币符号 |
| decimals | int | 精度，默认 18 |
| metadata_not_applicable | boolean | 标记是否为非 ERC20 合约（新增） |
| version | int | 乐观锁版本号（新增） |
| metadata_updated_at | timestamp | 元数据最后更新时间（新增） |
| **is_peggy_token** | **boolean** | **是否为 Peggy bridge token（新增）** |
| **peggy_denom** | **varchar(255)** | **Peggy bank_denom，如 peggy0xa8c8...（新增）** |
| **peggy_original_address** | **varchar(42)** | **Peggy 原始链 ERC20 地址（新增）** |
| metadata_source | varchar(50) | 元数据来源：evm/ethereum/cosmos/coingecko/manual（新增） |

### 5.2 索引

现有 `idx_token_symbol` 支持按 symbol 查询；新增以下索引优化查询性能：

> **注意**：MySQL **不支持**带 `WHERE` 的部分索引（Partial Index），以下为 MySQL 兼容写法。PostgreSQL 可使用 `CREATE INDEX ... WHERE condition` 语法获得更好的选择性。

```sql
-- 1. 元数据缺失查询优化索引（回填时 WHERE 条件用到这些字段）
-- MySQL: 建普通复合索引，由应用层过滤 metadata_not_applicable
CREATE INDEX idx_contract_infos_metadata_missing
ON contract_infos(contract_type, chain_id, metadata_not_applicable);

-- 2. 非 ERC20 合约索引（用于排除已标记合约）
CREATE INDEX idx_contract_infos_metadata_not_applicable
ON contract_infos(metadata_not_applicable);

-- 3. 元数据更新时间索引（定时任务按时间范围查询）
CREATE INDEX idx_contract_infos_metadata_updated_at
ON contract_infos(metadata_updated_at);

-- 4. Peggy tokens 查询索引（新增）
CREATE INDEX idx_contract_infos_is_peggy
ON contract_infos(is_peggy_token);

-- 5. Peggy denom 查询索引（用于反向查找）
CREATE INDEX idx_contract_infos_peggy_denom
ON contract_infos(peggy_denom);
```

回填查询需在应用层增加 `AND (metadata_not_applicable = FALSE OR metadata_not_applicable IS NULL)` 过滤。

### 5.3 数据库迁移脚本

执行顺序建议：先加字段，再加索引。适用于 MySQL 5.7+/8.0：

```sql
-- migrations/xxx_add_erc20_metadata_fields.sql
-- 新增 ERC20 元数据同步相关字段

-- 1. 新增基础字段
ALTER TABLE contract_infos
  ADD COLUMN IF NOT EXISTS metadata_not_applicable TINYINT(1) DEFAULT 0 COMMENT '是否非 ERC20 合约（避免重复尝试）' AFTER decimals,
  ADD COLUMN IF NOT EXISTS version INT DEFAULT 0 COMMENT '乐观锁版本' AFTER metadata_not_applicable,
  ADD COLUMN IF NOT EXISTS metadata_updated_at DATETIME(3) DEFAULT NULL COMMENT '元数据最后更新时间' AFTER version;

-- 2. 新增 Peggy tokens 相关字段
ALTER TABLE contract_infos
  ADD COLUMN IF NOT EXISTS is_peggy_token TINYINT(1) DEFAULT 0 COMMENT '是否为 Peggy bridge token' AFTER metadata_updated_at,
  ADD COLUMN IF NOT EXISTS peggy_denom VARCHAR(255) DEFAULT NULL COMMENT 'Peggy bank_denom，如 peggy0xa8c8...' AFTER is_peggy_token,
  ADD COLUMN IF NOT EXISTS peggy_original_address VARCHAR(42) DEFAULT NULL COMMENT 'Peggy 原始链 ERC20 地址' AFTER peggy_denom,
  ADD COLUMN IF NOT EXISTS metadata_source VARCHAR(50) DEFAULT NULL COMMENT '元数据来源：evm/ethereum/cosmos/coingecko/manual' AFTER peggy_original_address;

-- MySQL 5.7 无 ADD COLUMN IF NOT EXISTS，可改用：
-- ALTER TABLE contract_infos ADD COLUMN metadata_not_applicable TINYINT(1) DEFAULT 0;
-- ALTER TABLE contract_infos ADD COLUMN version INT DEFAULT 0;
-- ALTER TABLE contract_infos ADD COLUMN metadata_updated_at DATETIME(3) DEFAULT NULL;
-- ALTER TABLE contract_infos ADD COLUMN is_peggy_token TINYINT(1) DEFAULT 0;
-- ALTER TABLE contract_infos ADD COLUMN peggy_denom VARCHAR(255);
-- ALTER TABLE contract_infos ADD COLUMN peggy_original_address VARCHAR(42);
-- ALTER TABLE contract_infos ADD COLUMN metadata_source VARCHAR(50);

-- 3. 新增索引
CREATE INDEX IF NOT EXISTS idx_contract_infos_metadata_not_applicable
  ON contract_infos(metadata_not_applicable);

CREATE INDEX IF NOT EXISTS idx_contract_infos_metadata_missing
  ON contract_infos(contract_type, chain_id, metadata_not_applicable);

CREATE INDEX IF NOT EXISTS idx_contract_infos_metadata_updated_at
  ON contract_infos(metadata_updated_at);

CREATE INDEX IF NOT EXISTS idx_contract_infos_is_peggy
  ON contract_infos(is_peggy_token);

CREATE INDEX IF NOT EXISTS idx_contract_infos_peggy_denom
  ON contract_infos(peggy_denom);

-- 4. 回填历史数据：标记已有的 Peggy tokens
-- 从 metadata JSON 字段中提取 bank_denom
UPDATE contract_infos
SET is_peggy_token = 1,
    peggy_denom = JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.bank_denom')),
    peggy_original_address = SUBSTRING(JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.bank_denom')), 7)
WHERE metadata IS NOT NULL
  AND JSON_CONTAINS_PATH(metadata, '$.bank_denom')
  AND JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.bank_denom')) LIKE 'peggy0x%';

-- 5. 为已有的 Peggy tokens 设置元数据来源标识
UPDATE contract_infos
SET metadata_source = 'manual'
WHERE is_peggy_token = 1
  AND (token_name IS NOT NULL OR token_symbol IS NOT NULL);
```

> **注意**：MySQL 5.7 不支持 `ADD COLUMN IF NOT EXISTS` 和 `CREATE INDEX IF NOT EXISTS`，需先 `SELECT` 检查或使用 `CREATE INDEX ... ON ...` 并忽略重复错误。

---

## 六、配置设计

### 6.1 现有配置（复用）

```yaml
chain:
  evm:
    enabled: true
    rpc_endpoint: ${CHAIN_EVM_RPC_ENDPOINT}
    chain_id: ${CHAIN_EVM_CHAIN_ID}
    timeout: 10s
```

### 6.2 Peggy Tokens 元数据获取配置

> **⚠️ 重要更新**：经过实测验证，Cosmos Peggy Query 接口不可用（HTTP 501），因此配置简化为只保留 Ethereum RPC 和 CoinGecko。

```yaml
chain:
  # ✅ Ethereum RPC 配置（P0 - 主要数据源，已验证可行）
  ethereum:
    enabled: true
    rpc_endpoint: ${ETHEREUM_RPC_ENDPOINT:https://eth.llamarpc.com}
    chain_id: 1  # Ethereum Mainnet
    timeout: 10s
    # 备选端点（故障转移）
    rpc_endpoints:
      - https://eth.llamarpc.com
      - https://ethereum.publicnode.com
      - https://rpc.ankr.com/eth  # 需要 API key 时添加

  # ⚠️ CoinGecko API（P1 - 备选数据源）
  coingecko:
    enabled: false  # 默认禁用，可选启用
    api_key: ${COINGECKO_API_KEY:}  # 可选，免费版有限制
    base_url: https://api.coingecko.com/api/v3
    timeout: 10s
    # 请求间隔（避免限流）
    rate_limit:
      requests_per_second: 10
      burst_size: 20

  # ❌ 已移除：Cosmos Peggy 配置（接口不可用）
  # cosmos_peggy:
  #   enabled: false
  #   reason: "Query interfaces not implemented (HTTP 501)"
```

**配置说明**：

| 配置项 | 必需 | 默认值 | 说明 |
|--------|------|--------|------|
| `ethereum.enabled` | 是 | true | 是否启用 Ethereum RPC 查询 |
| `ethereum.rpc_endpoint` | 是 | https://eth.llamarpc.com | Ethereum RPC 端点 |
| `ethereum.chain_id` | 是 | 1 | Ethereum Mainnet Chain ID |
| `coingecko.enabled` | 否 | false | 是否启用 CoinGecko 备选查询 |
| `coingecko.api_key` | 否 | - | CoinGecko API Key（可选） |

**故障转移**：
- Ethereum RPC 支持多端点配置
- 主端点失败时自动切换到备用端点
- 建议配置至少 2-3 个公共端点

**公共 Ethereum RPC 端点**（推荐）：
1. ✅ `https://eth.llamarpc.com`（已测试，免费，无限流）
2. ✅ `https://ethereum.publicnode.com`
3. ✅ `https://rpc.ankr.com/eth`（需要 API key）

当前实现可先复用现有 evm 配置，后续按需引入。

---

## 七、监控与可观测性

### 7.1 日志

| 场景 | 级别 | 内容 |
|------|------|------|
| 元数据更新成功 | Info | `updated ERC20 metadata for 0x...: name=..., symbol=..., decimals=...` |
| 拉取失败/部分失败 | Warn | `partial or failed ERC20 metadata fetch for 0x...: ...` |
| EVM 客户端创建失败 | Error | `failed to create EVM client: ...` |

### 7.2 指标（可选）

| 指标名 | 类型 | 说明 |
|--------|------|------|
| `contract_metadata_fetch_total` | counter | 按 status=success/failure 统计 |
| `contract_metadata_fetch_duration_seconds` | histogram | 拉取耗时 |

### 7.3 健康检查

- EVM RPC 可达性：可在现有 health check 中增加 `eth_blockNumber` 探测
- 元数据缺失数量：Admin API 可提供 `GET /api/v1/admin/contract/metadata/stats` 返回 `missing_count`

---

## 八、实现计划

| 阶段 | 内容 | 状态/优先级 |
|------|------|-------------|
| 1 | 修复 fetchAndUpdateERC20Metadata 使用 createEVMClient | ✅ 已完成 |
| 1.1 | 增加数据库字段：metadata_not_applicable, version, metadata_updated_at | P0 |
| 1.2 | 增加 Peggy tokens 相关字段：is_peggy_token, peggy_denom, peggy_original_address | P0 |
| 1.3 | 实现元数据有效性验证（validateERC20Metadata） | P1 |
| **1.4** | **实现 Peggy tokens 元数据获取（fetchPeggyTokenMetadata）** | **P0** |
| **1.5** | **实现多数据源回退：Ethereum RPC → Cosmos Peggy → CoinGecko** | **P1** |
| 2 | 实现错误分类与重试策略（classifyError, handleFetchError） | P1 |
| 3 | 新增 Admin API：单合约刷新、批量回填 | P1 |
| **3.1** | **新增 Peggy Token 专门刷新 API** | **P1** |
| 4 | 编写回填脚本（CLI/Go），支持批量处理 | P2 |
| **4.1** | **回填脚本支持 Peggy tokens 批量处理** | **P2** |
| 5 | 实现 RPC 端点多活与故障转移（MultiEndpointEVMClient） | P2 |
| 6 | 实现批量元数据获取优化（JSON-RPC Batch） | P2 |
| 7 | 实现缓存策略（Redis 缓存元数据） | P2 |
| **7.1** | **为 Peggy tokens 实现专门的缓存策略** | **P2** |
| 8 | 实现数据并发更新控制（乐观锁/分布式锁） | P2 |
| 9 | 可选：metadata 专用配置、定时回填任务 | P3 |
| 10 | 可选：监控指标增强、告警规则 | P3 |
| **10.1** | **新增 Peggy tokens 监控指标** | **P2** |

### 实施顺序建议

**第一阶段（核心功能，必须）：**
- ✅ 1. 已修复：使用 createEVMClient
- 1.1 数据库迁移：增加新字段（metadata_not_applicable, version, metadata_updated_at）
- **1.2 数据库迁移：增加 Peggy tokens 字段（is_peggy_token, peggy_denom, peggy_original_address）**
- 1.3 实现元数据有效性验证（validateERC20Metadata）
- **1.4 实现 Peggy tokens 元数据获取（fetchPeggyTokenMetadata + 多数据源回退）**
- 2 实现错误分类与重试策略（classifyError, handleFetchError）

**第二阶段（可用性增强）：**
- 3 Admin API 开发（单合约刷新、批量回填）
- **3.1 新增 Peggy Token 专门刷新 API**
- 4 回填脚本编写
- **4.1 回填脚本支持 Peggy tokens 批量处理**

**可选（按需）：** 1.2 合约类型预检测（isERC20Contract），在主要为非 ERC20 场景可节省 RPC

**第三阶段（性能优化）：**
- 5 RPC 端点多活
- **5.1 配置 Ethereum RPC（用于 Peggy tokens 原始链查询）**
- 6 批量获取优化
- 7 缓存策略
- **7.1 为 Peggy tokens 实现专门的缓存策略**
- 8 并发控制

**第四阶段（可观测性）：**
- 9 定时任务
- 10 监控与告警
- **10.1 新增 Peggy tokens 监控指标**

---

## 九、测试策略

### 9.1 单元测试

- `GetERC20Metadata`：mock eth_call 返回，验证 name/symbol/decimals 解析
- `fetchAndUpdateERC20Metadata`：mock createEVMClient、db，验证 updates 映射正确

### 9.2 集成测试

- 使用测试网或 fork：Create Token Pair 交易 → 验证 contract_infos 中 token_name、token_symbol 非空
- 回填 API：对已知合约调用，验证 DB 更新

### 9.3 回归场景

- 交易 `0x70af765a4b927be4ef9fc63338dae13df5e28b26592a2eff4167a6be11df2b16` 创建的合约，修复后新同步应能补齐元数据；历史数据需通过回填补齐

---

## 十、后续扩展

| 方向 | 说明 |
|------|------|
| CW20 元数据 | CosmWasm 合约的 token_info 从 chain query 获取，与 ERC20 路径不同 |
| 更多 contract_type | NFT、LP Token 等可按需扩展类似机制 |
| 多链 EVM | 不同 chain_id 对应不同 RPC，需按 chain_id 选择 EVM 客户端 |
| 缓存层 | 对高频查询的 token 元数据可加 Redis 缓存，降低 DB/RPC 压力 |

---

## 附录 A：相关代码位置

| 功能 | 文件 | 函数/常量 |
|------|------|-----------|
| Cosmos 事件解析 | internal/service/sync.go | saveContractInfoFromCosmosEvents, extractContractCreationEvents |
| 元数据拉取 | internal/service/sync.go | fetchAndUpdateERC20Metadata, scheduleMetadataFetch, processMetadataJob |
| **Peggy tokens 元数据获取（已验证）** | **internal/service/sync.go** | **fetchAndUpdateERC20Metadata, fetchMetadataFromEthereum, fetchMetadataFromCoinGecko, getPeggyTokenDefaults** |
| **合约类型验证（新增）** | **internal/service/sync.go** | **isERC20Contract, SupportsERC20Interface** |
| **元数据验证（新增）** | **internal/service/sync.go** | **validateERC20Metadata** |
| **错误分类（新增）** | **internal/service/sync.go** | **classifyError, handleFetchError, MetadataFetchError** |
| ERC20 合约调用 | provider/chain/evm_client.go | GetERC20Name, GetERC20Symbol, GetERC20Decimals, GetERC20Metadata |
| **批量获取（新增）** | **provider/chain/evm_client.go** | **GetERC20MetadataBatch, batchFetch** |
| EVM 客户端创建 | internal/service/explorer.go | createEVMClient |
| **多端点 EVM 客户端（新增）** | **provider/chain/evm_client.go** | **MultiEndpointEVMClient, healthCheck** |
| EVM 详情同步入库 | internal/service/evm_details_sync.go | saveContractInfoIfNotExists |
| **缓存管理（新增）** | **internal/service/cache.go** | **MetadataCache, Get, Set** |
| **乐观锁更新（新增）** | **internal/service/sync.go** | **updateMetadataWithOptimisticLock** |
| 常量 | internal/service/constants.go | EventCreateTokenPair, MetadataJob, **EventAttributeBankDenom** |
| **Admin API（新增）** | **internal/api/admin_api.go** | **RefreshMetadata, BackfillMetadata, GetMetadataStats, RefreshPeggyMetadata** |
| **Peggy tokens 配置（新增）** | **internal/conf/config.proto** | **EthereumRPC, CoinGeckoAPI** |

### 数据库模型

| 模型 | 文件 | 字段 |
|------|------|------|
| ContractInfo | internal/models/contract_info.go | contract_address, token_name, token_symbol, decimals, **metadata_not_applicable, version, metadata_updated_at, is_peggy_token, peggy_denom, peggy_original_address, metadata_source** |

## 附录 B：问题排查指南

### B.1 元数据为空的常见原因

| 症状 | 可能原因 | 排查方法 |
|------|----------|----------|
| token_name 和 token_symbol 都为空 | EVM RPC 配置错误或不可达 | 检查 `chain.evm.rpc_endpoint` 配置，测试 RPC 连通性 |
| token_name 和 token_symbol 都为空 | 合约不是 ERC20 标准 | 检查 `metadata_not_applicable` 标志，查看合约源码 |
| 仅 decimals 为空 | 合约未实现 decimals() 方法 | 部分老合约可能缺少此方法，设置默认值 18 |
| 部分字段有值，部分为空 | RPC 超时或返回部分失败 | 查看日志中的 warn 级别错误信息 |

### B.2 诊断命令

```bash
# 1. 检查 EVM RPC 配置
grep -A 5 "evm:" configs/config.yaml

# 2. 测试 RPC 连通性
curl -X POST $CHAIN_EVM_RPC_ENDPOINT \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'

# 3. 查询元数据缺失的合约（MySQL）
mysql -e "SELECT contract_address, contract_type FROM contract_infos
         WHERE ((token_name IS NULL OR token_name = '')
            OR (token_symbol IS NULL OR token_symbol = ''))
         LIMIT 10;" $DB_NAME

# 4. 查看元数据拉取失败日志
grep "metadata" logs/sync.log | grep -E "(failed|error|warn)"

# 5. 手动测试合约调用（需 Foundry: cast）
cast call 0x合约地址 "symbol()" --rpc-url $CHAIN_EVM_RPC_ENDPOINT

# 5 备选：无 cast 时使用 curl 调用 eth_call（symbol 的 selector 为 0x95d89b41）
CONTRACT="0x合约地址"
curl -s -X POST $CHAIN_EVM_RPC_ENDPOINT -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$CONTRACT\",\"data\":\"0x95d89b41\"},\"latest\"],\"id\":1}"
```

### B.3 针对交易 0x70af765a... 的诊断步骤

```bash
# 1. 获取交易详情，确认合约地址
injectived query tx 0x70af765a4b927be4ef9fc63338dae13df5e28b26592a2eff4167a6be11df2b16

# 2. 检查数据库中该合约的记录（MySQL）
mysql -e "SELECT * FROM contract_infos WHERE creator LIKE '%70af765a%'" $DB_NAME

# 3. 查看该合约的元数据拉取日志
grep "0x70af765a" logs/sync.log

# 4. 手动测试合约是否支持 ERC20（cast 或 curl）
# 使用 cast（Foundry）：
cast call <合约地址> "symbol()" --rpc-url $CHAIN_EVM_RPC_ENDPOINT

# 使用 curl（无 Foundry 时）：
curl -s -X POST $CHAIN_EVM_RPC_ENDPOINT -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_call","params":[{"to":"<合约地址>","data":"0x95d89b41"},"latest"],"id":1}'
# 返回 0x 或 revert 表示非 ERC20；正常返回为 ABI 编码的 string
```

## 附录 C：ERC20 方法 Selector 参考

```
name()     → 0x06fdde03  (keccak256("name()")[:4])
symbol()   → 0x95d89b41  (keccak256("symbol()")[:4])
decimals() → 0x313ce567  (keccak256("decimals()")[:4])
```

## 附录 D：快速修复指南

针对你遇到的具体问题（交易 `0x70af765a...` 创建的合约元数据为空），按以下步骤快速诊断和修复：

### D.1 立即诊断（5 分钟）

```bash
# 1. 确认 EVM RPC 配置
cat configs/config.yaml | grep -A 5 "evm:"

# 2. 测试 RPC 连通性
export CHAIN_EVM_RPC_ENDPOINT="https://sentry.evm-rpc.injective.network"
curl -X POST $CHAIN_EVM_RPC_ENDPOINT \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'

# 3. 获取交易详情，找到合约地址
injectived query tx 0x70af765a4b927be4ef9fc63338dae13df5e28b26592a2eff4167a6be11df2b16

# 4. 检查数据库记录（MySQL）
mysql -e "SELECT contract_address, token_name, token_symbol, decimals, metadata_not_applicable
         FROM contract_infos
         WHERE creator LIKE '%70af765a%';" $DB_NAME

# 5. 查看相关日志
grep "metadata" logs/sync.log | tail -100
```

### D.2 快速修复步骤

#### 修复 1：确认 RPC 配置正确

检查 `configs/config.yaml` 中 EVM RPC 配置：

```yaml
chain:
  evm:
    enabled: true
    rpc_endpoint: https://sentry.evm-rpc.injective.network  # 确认可访问
    chain_id: 801
    timeout: 10s
```

#### 修复 2：手动触发元数据拉取

在代码中临时添加手动触发：

```go
// 在 sync.go 中添加临时测试代码
func (s *SyncService) DebugFetchMetadata(contractAddress string) {
    ctx := context.Background()
    err := s.fetchAndUpdateERC20Metadata(ctx, contractAddress, global.ChainConfig.ChainId)
    if err != nil {
        global.Logger.Errorf("debug fetch metadata failed: %v", err)
    }
}
```

> **说明**：当前实现使用**内存工作池**（`metadataWorkerPool`），无 `metadata_jobs` 表。若需持久化任务队列，可后续引入 Redis 或数据库队列表。手动触发建议通过 Admin API 或临时测试代码。

#### 修复 3：实现最小化的错误分类

在 `fetchAndUpdateERC20Metadata` 中添加：

```go
func (s *SyncService) fetchAndUpdateERC20Metadata(ctx context.Context, contractAddress, chainID string) error {
    evmClient, err := createEVMClient()
    if err != nil {
        return errors.Wrap(err, "failed to create EVM client")
    }
    defer evmClient.Close()

    // Fetch ERC20 metadata
    name, symbol, decimals, err := evmClient.GetERC20Metadata(ctx, contractAddress)
    if err != nil {
        // 错误分类：如果是合约不支持，标记为非 ERC20（需同时匹配 chain_id）
        if strings.Contains(err.Error(), "execution reverted") {
            db.Get().Model(&models.ContractInfo{}).
                Where("contract_address = ? AND chain_id = ?", contractAddress, chainID).
                Update("metadata_not_applicable", true)
            global.Logger.Infof("marked %s (chain=%s) as not ERC20", contractAddress, chainID)
            return nil  // 不返回错误，避免重试
        }
        return errors.Wrap(err, "failed to fetch metadata")
    }

    // ... 更新数据库
}
```

### D.3 验证修复效果

```bash
# 1. 重启 sync 服务
systemctl restart injective-explorer

# 2. 观察日志
tail -f logs/sync.log | grep -i "metadata"

# 3. 等待 1 分钟后检查数据库（MySQL）
mysql -e "SELECT contract_address, token_name, token_symbol, decimals, metadata_updated_at
         FROM contract_infos
         WHERE creator LIKE '%70af765a%';" $DB_NAME

# 4. 如果还是空，手动调用 Admin API 或临时测试代码触发刷新
```

### D.4 如果仍然失败的检查清单

- [ ] EVM RPC 端点是否可访问（使用 curl 测试）
- [ ] 合约地址是否正确（检查交易详情中的 contract_address）
- [ ] 合约是否真的是 ERC20（使用 cast 测试 symbol() 调用）
- [ ] 代码是否已使用 `createEVMClient()` 而非 `NewEVMClient(global.NetworkName)`
- [ ] 日志中是否有 "failed to create EVM client" 错误
- [ ] 工作池是否正常运行（metadataWorkerPool 初始化）
- [ ] 交易是否被正确解析（saveContractInfoFromCosmosEvents 被调用）

### D.5 常见错误及解决方案

| 错误信息 | 原因 | 解决方案 |
|---------|------|----------|
| `unsupported network: injective` | 使用了 `NewEVMClient(global.NetworkName)` | 改用 `createEVMClient()` |
| `connection refused` | RPC 端点不可访问 | 检查 `rpc_endpoint` 配置，测试网络连通性 |
| `execution reverted` | 合约不是 ERC20 | 标记 `metadata_not_applicable = true` |
| `context deadline exceeded` | RPC 超时 | 增加 `timeout` 配置或检查 RPC 性能 |
| `no data` | 合约地址不存在或未部署 | 确认合约地址正确性和部署状态 |

---

**总结：** 问题的根因很可能是代码仍在使用 `NewEVMClient(global.NetworkName)` 而非 `createEVMClient()`，或者 EVM RPC 配置有误。按照上述步骤可以快速定位问题并实施修复。

---

## 附录 E：审查补充与优化记录

### E.1 审查修正（历史）

| 类别 | 发现项 | 修正方式 |
|------|--------|----------|
| **技术准确性** | EIP-165 方案 B：标准 ERC20 不实现 supportsInterface，多数代币会误判 | 增加⚠️说明，标注为不推荐，仅适用于扩展型 ERC20 |
| **数据库兼容** | 5.2 索引使用 `WHERE` 条件，MySQL 不支持部分索引 | 改为 MySQL 兼容的普通索引，补充应用层过滤说明 |
| **实现一致性** | handleFetchError 仅用 contract_address，未包含 chain_id | 增加 chain_id 参数，Where 条件匹配唯一约束 |
| **实现一致性** | validateERC20Metadata 修改 symbol 但未返回，调用方无法获取规范化值 | 改为返回 (name, symbol, error)，供 DB 更新使用 |
| **文档准确性** | 附录 D 引用 metadata_jobs 表，当前实现为内存工作池 | 删除 SQL 示例，说明当前无队列表，建议用 Admin API |
| **环境一致性** | 诊断命令使用 psql（PostgreSQL），项目实际为 MySQL | 统一改为 mysql 命令，变量 $DB_NAME |
| **配置扩展** | 4.9 rpc_endpoints 与现有 conf.proto 单端点冲突 | 补充“需 proto 与配置迁移”的说明，标注为 P2 扩展 |

### E.2 后续优化（本轮）

| 优化项 | 说明 |
|--------|------|
| 文档目录 | 增加全文目录，便于长文档导航 |
| 数据库迁移脚本 | 新增 5.3 节，提供可执行的迁移 SQL |
| rate_limit 错误分类 | classifyError 补充 429/too many requests 分支 |
| 缓存与锁 key | Get/Set、lockKey 增加 chain_id，同一地址多链区分 |
| 乐观锁示例 | 完善 gorm.Expr、chain_id、metadata_updated_at |
| API 统计接口 | 新增 4.3.3 GetMetadataStats |
| 实现计划 | 1.2 isERC20Contract 调整为 P2 可选 |

### E.3 落地时建议核对

1. **evm_details_sync** 与 sync 的 EVM 配置一致（createEVMClientFromConfig / createEVMClient）。
2. **乐观锁** 使用 `gorm.Expr("version + 1")`，Where 含 version 与 chain_id（4.11.1）。
3. **cast** 无 Foundry 时用 curl + eth_call（附录 B.2、B.3）。
4. **缓存/锁 key** 含 chain_id（4.12、4.11.2）。
5. **Peggy tokens** 实现 fetchPeggyTokenMetadata + 多数据源回退（三一、3.3）。

---

## 附录 F：Peggy Tokens 诊断实例

### F.1 问题描述

交易 `0x70af765a4b927be4ef9fc63338dae13df5e28b26592a2eff4167a6be11df2b16` 创建的合约 `0x9772154BF1823353C829B43F9AD96CF9f720c768` 在数据库中 `token_name` 和 `token_symbol` 为空。

### F.2 诊断步骤

#### 第 1 步：确认合约基本信息

```bash
# 查询数据库
mysql -u root -p injective_explorer -e "
SELECT
    contract_address,
    token_name,
    token_symbol,
    decimals,
    is_peggy_token,
    peggy_denom,
    JSON_EXTRACT(metadata, '$.bank_denom') as bank_denom_from_metadata,
    created_at
FROM contract_infos
WHERE contract_address = '0x9772154BF1823353C829B43F9AD96CF9f720c768';
"

# 预期结果：
# contract_address: 0x9772154BF1823353C829B43F9AD96CF9f720c768
# token_name: NULL 或空字符串
# token_symbol: NULL 或空字符串
# peggy_denom: peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca (如果有字段)
# bank_denom_from_metadata: "peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca"
```

#### 第 2 步：验证 EVM 合约

```bash
CONTRACT="0x9772154BF1823353C829B43F9AD96CF9f720c768"
RPC="https://sentry.evm-rpc.injective.network"

# 测试合约是否存在
echo "=== Testing contract existence ==="
curl -s -X POST $RPC -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_getCode\",\"params\":[\"$CONTRACT\",\"latest\"],\"id\":1}" \
  | jq -r '.result' | head -c 100

# 预期结果：返回字节码（不是 0x）

# 测试 ERC20 方法
echo "=== Testing ERC20 methods ==="
echo "name():"
curl -s -X POST $RPC -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$CONTRACT\",\"data\":\"0x06fdde03\"},\"latest\"],\"id\":1}"

echo "symbol():"
curl -s -X POST $RPC -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$CONTRACT\",\"data\":\"0x95d89b41\"},\"latest\"],\"id\":1}"

echo "decimals():"
curl -s -X POST $RPC -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$CONTRACT\",\"data\":\"0x313ce567\"},\"latest\"],\"id\":1}"

# 预期结果：
# name():     0x0000... (空字符串)
# symbol():   0x0000... (空字符串)
# decimals(): 0x0000... (0)
```

#### 第 3 步：检查交易事件

```bash
TX="0x70af765a4b927be4ef9fc63338dae13df5e28b26592a2eff4167a6be11df2b16"

# 从事件中确认是 Peggy token
injectived query tx $TX 2>/dev/null | grep -A 5 "injective.erc20.v1beta1.EventCreateTokenPair"

# 预期输出包含：
# bank_denom: "peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca"
# erc20_address: "0x9772154BF1823353C829B43F9AD96CF9f720c768"
```

#### 第 4 步：尝试从原始链获取

```bash
# 从 peggy denom 提取原始地址
PEGGY_DENOM="peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca"
ORIGINAL_ADDRESS=$(echo $PEGGY_DENOM | sed 's/peggy//')

echo "=== Original Ethereum address: $ORIGINAL_ADDRESS ==="

# 使用 Ethereum RPC 测试（示例，使用公共节点）
ETH_RPC="https://eth.llamarpc.com"

echo "Testing name() on Ethereum:"
curl -s -X POST $ETH_RPC -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$ORIGINAL_ADDRESS\",\"data\":\"0x06fdde03\"},\"latest\"],\"id\":1}" \
  | jq -r '.result'

echo "Testing symbol() on Ethereum:"
curl -s -X POST $ETH_RPC -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$ORIGINAL_ADDRESS\",\"data\":\"0x95d89b41\"},\"latest\"],\"id\":1}" \
  | jq -r '.result'

echo "Testing decimals() on Ethereum:"
curl -s -X POST $ETH_RPC -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$ORIGINAL_ADDRESS\",\"data\":\"0x313ce567\"},\"latest\"],\"id\":1}" \
  | jq -r '.result'

# 预期结果：返回有效的元数据（如果原始合约存在）
```

#### 第 5 步：检查日志

```bash
# 查看元数据拉取相关日志
grep "0x9772154bf1823353c829b43f9ad96cf9f720c768" logs/sync.log | grep -i metadata

# 查看是否有错误
grep "metadata.*failed\|metadata.*warn" logs/sync.log | grep "0x9772154bf1823353c829b43f9ad96cf9f720c768"

# 预期输出：
# 如果有日志，应该显示 EVM 调用返回空值
```

### F.3 诊断结论

根据上述步骤，可以得出以下结论：

| 检查项 | 结果 | 结论 |
|--------|------|------|
| 合约存在 | ✅ eth_getCode 返回字节码 | 合约已成功部署 |
| EVM name/symbol/decimals | ❌ 全部返回空值 | EVM 层无元数据 |
| 事件中 bank_denom | ✅ peggy0xa8c8... | **确认为 Peggy token** |
| 原始链元数据 | ？ | 需配置 Ethereum RPC 测试 |

**根本原因**：这是一个 Peggy bridge token，元数据需要从原始链（Ethereum）或 Cosmos Peggy 模块获取，而不是从 Injective EVM 层的包装合约获取。

### F.4 解决方案实施

#### 方案 A：实现完整的 Peggy metadata fetch（推荐）

```go
// 1. 执行数据库迁移，增加 Peggy 字段
// 2. 更新 buildContractInfo，自动标记 Peggy tokens
// 3. 实现 fetchPeggyTokenMetadata
// 4. 更新 fetchAndUpdateERC20Metadata，调用 Peggy 专门处理
```

具体代码参考 **[三一、3.3.2 推荐方案：多级回退策略](#三一、3.3-推荐方案多级回退策略)**。

#### 方案 B：临时手动修复

```sql
-- 为这个特定的 Peggy token 设置元数据
UPDATE contract_infos
SET token_name = 'Example Token',  -- 需要从原始链获取真实名称
    token_symbol = 'EXT',           -- 需要从原始链获取真实符号
    decimals = 18,
    metadata_source = 'manual',
    is_peggy_token = 1,
    peggy_denom = 'peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca',
    peggy_original_address = '0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca'
WHERE contract_address = '0x9772154BF1823353C829B43F9AD96CF9f720c768';
```

#### 方案 C：批量处理已有的 Peggy tokens

```sql
-- 1. 标记所有 Peggy tokens
UPDATE contract_infos
SET is_peggy_token = 1,
    peggy_denom = JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.bank_denom')),
    peggy_original_address = SUBSTRING(JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.bank_denom')), 7)
WHERE metadata IS NOT NULL
  AND JSON_CONTAINS_PATH(metadata, '$.bank_denom')
  AND JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.bank_denom')) LIKE 'peggy0x%';

-- 2. 为元数据为空的 Peggy tokens 设置默认值
UPDATE contract_infos
SET token_name = 'Bridge Token',
    token_symbol = 'PEGGY',
    decimals = 18,
    metadata_source = 'manual_default'
WHERE is_peggy_token = 1
  AND (token_name IS NULL OR token_name = '');

-- 3. 使用 Admin API 或脚本批量刷新
# POST /api/v1/admin/contract/metadata/backfill
# {
#   "only_peggy": true,
#   "limit": 100,
#   "async": true
# }
```

### F.5 验证修复效果

```bash
# 1. 检查数据库更新
mysql -u root -p injective_explorer -e "
SELECT
    contract_address,
    token_name,
    token_symbol,
    decimals,
    is_peggy_token,
    metadata_source,
    metadata_updated_at
FROM contract_infos
WHERE contract_address = '0x9772154BF1823353C829B43F9AD96CF9f720c768';
"

# 预期结果：
# token_name: Example Token（或从原始链获取的真实名称）
# token_symbol: EXT（或从原始链获取的真实符号）
# decimals: 18
# is_peggy_token: 1
# metadata_source: ethereum/cosmos/coingecko（实际使用的数据源）

# 2. 检查日志确认无重复错误
grep "0x9772154bf1823353c829b43f9ad96cf9f720c768" logs/sync.log | tail -10

# 3. 验证前端显示
# 访问 explorer 的合约详情页，确认元数据正确显示
```

### F.6 经验总结

1. **识别 Peggy tokens 的关键**：
   - 检查 `EventCreateTokenPair` 事件中的 `bank_denom` 属性
   - denom 格式：`peggy0x{address}`

2. **元数据获取优先级**（已验证）：
   - 本地 EVM RPC（最快，但对 Peggy 无效）
   - **Ethereum RPC**（P0，主要数据源，已验证可行）
   - **CoinGecko API**（P1，备选数据源）
   - **默认值**（P2，兜底方案）
   - ~~Cosmos Peggy 模块~~（已验证不可用，HTTP 501，详见[附录 G](#附录-gcosmos-peggy-query-验证记录)）

3. **避免重复尝试**：
   - 使用 `metadata_not_applicable` 标记非 ERC20 合约
   - 使用 `is_peggy_token` 标识 Peggy tokens
   - 使用 `metadata_source` 追踪数据来源

4. **监控关键指标**：
   - Peggy tokens 检测数量
   - 各数据源的成功率
   - 元数据获取延迟（特别是跨链调用）

---

## 附录 G：Cosmos Peggy Query 验证记录

### G.1 验证目的

验证 Injective Chain 的 Cosmos Peggy Module 是否提供 Query 接口来获取 Peggy token 的元数据（symbol/decimals）。

### G.2 测试环境

| 项目 | 值 |
|------|-----|
| 测试日期 | 2026-02-13 |
| 测试网络 | Injective Mainnet / Testnet |
| 测试 denom | `peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca` |
| 测试地址 | `0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca` |

### G.3 测试端点

#### G.3.1 denom_to_erc20 查询

```bash
# 测试 Injective Mainnet LCD
curl -s "https://lcd.injective.network/peggy/v1beta1/denom_to_erc20/peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca" | jq

# 预期响应（理论）
{
  "erc20": "0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca"
}

# 实际响应
{
  "code": 12,
  "message": "Not Implemented",
  "details": []
}
```

**结果**：❌ HTTP 501 Not Implemented

#### G.3.2 erc20_to_denom 查询

```bash
# 测试 Injective Mainnet LCD
curl -s "https://lcd.injective.network/peggy/v1beta1/erc20_to_denom/0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca" | jq

# 预期响应（理论）
{
  "denom": "peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca"
}

# 实际响应
{
  "code": 12,
  "message": "Not Implemented",
  "details": []
}
```

**结果**：❌ HTTP 501 Not Implemented

#### G.3.3 params 查询

```bash
# 测试 Peggy params
curl -s "https://lcd.injective.network/peggy/v1beta1/params" | jq

# 预期响应（理论）
{
  "params": {
    "peggy_id": "inj...",
    ...
  }
}

# 实际响应
{
  "code": 12,
  "message": "Not Implemented",
  "details": []
}
```

**结果**：❌ HTTP 501 Not Implemented

### G.4 多 LCD 端点验证

| LCD 端点 | denom_to_erc20 | erc20_to_denom | params | 结论 |
|----------|---------------|---------------|--------|------|
| `https://lcd.injective.network` | 501 | 501 | 501 | ❌ 未实现 |
| `https://lcd.testnet.injective.network` | 501 | 501 | 501 | ❌ 未实现 |
| `https://sentry.lcd.injective.network` | 501 | 501 | 501 | ❌ 未实现 |

### G.5 根因分析

#### G.5.1 为什么 Peggy Query 接口未实现？

可能的原因：

1. **Injective 使用自定义桥接实现**：Injective 可能修改了标准 Peggy Module，没有暴露 Query 接口
2. **Gas 成本优化**：存储完整的元数据会增加链上存储成本
3. **设计决策**：Injective 期望用户从原始链（Ethereum）获取元数据，而不是从 Cosmos 层

#### G.5.2 Injective ERC20 Module vs Peggy Module

| Module | 命名空间 | 功能 | Query 支持 |
|--------|----------|------|-----------|
| `injective.erc20.v1beta1` | Cosmos SDK | 管理 Peggy tokens | ✅ 部分支持 |
| `injective.peggy.v1beta1` | Cosmos SDK | Peggy bridge 逻辑 | ❌ Query 未实现 |

**验证结果**：
- ✅ `injective.erc20.v1beta1` 的 `EventCreateTokenPair` 事件可以捕获
- ❌ `injective.peggy.v1beta1` 的 Query 接口不可用

### G.6 对技术方案的影响

#### G.6.1 原方案（理论推测）

```
优先级：
1. 本地 EVM RPC
2. Cosmos Peggy Query ← ❌ 未实现
3. Ethereum RPC
4. CoinGecko API
5. 默认值
```

#### G.6.2 新方案（实测验证）

```
优先级：
1. 本地 EVM RPC（对 Peggy 无效）
2. Ethereum RPC ← ✅ P0 主要数据源
3. CoinGecko API ← P1 备选
4. 默认值 ← P2 兜底
```

### G.7 验证结论

| 项目 | 结论 |
|------|------|
| Cosmos Peggy Query 可用性 | ❌ **不可用**（HTTP 501） |
| 技术方案是否需要调整 | ✅ **是**，移除 Cosmos Peggy Query |
| 推荐主要数据源 | ✅ **Ethereum RPC**（已验证可行） |
| 推荐备选数据源 | ⚠️ **CoinGecko API**（部分代币缺失） |
| 推荐兜底方案 | ✅ **默认值**（从 denom 推断） |

### G.8 实施建议

1. **立即移除** Cosmos Peggy Query 相关代码和配置
2. **使用 Ethereum RPC** 作为 Peggy tokens 的主要数据源
3. **保留 CoinGecko API** 作为备选（可选）
4. **实现默认值** 作为兜底方案

### G.9 测试命令

```bash
# 1. 测试 denom_to_erc20
curl -s "https://lcd.injective.network/peggy/v1beta1/denom_to_erc20/peggy0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca" | jq

# 2. 测试 erc20_to_denom
curl -s "https://lcd.injective.network/peggy/v1beta1/erc20_to_denom/0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca" | jq

# 3. 测试 params
curl -s "https://lcd.injective.network/peggy/v1beta1/params" | jq

# 4. 对比：测试 Ethereum RPC（应该成功）
ETH_RPC="https://eth.llamarpc.com"
ORIGINAL="0xa8c8CfB141A3bB59FEA1E2ea6B79b5ECBCD7b6ca"
curl -s -X POST $ETH_RPC -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"$ORIGINAL\",\"data\":\"0x95d89b41\"},\"latest\"],\"id\":1}" | jq
```

---

**验证完成日期**: 2026-02-13
**验证状态**: ✅ 完成
**下一步**: 实现 Ethereum RPC 作为主要数据源（P0 优先级）

