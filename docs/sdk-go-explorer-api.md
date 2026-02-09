# SDK-Go Explorer API 接口文档

本文档详细描述了 `sdk-go/client/explorer` 包中提供的所有接口，包括每个接口的入参和出参结构。

## 目录

- [接口概览](#接口概览)
- [详细接口说明](#详细接口说明)
  - [1. GetAccountTxs - 获取账户交易](#1-getaccounttxs---获取账户交易)
  - [2. FetchContractTxs - 获取合约交易](#2-fetchcontracttxs---获取合约交易)
  - [3. FetchContractTxsV2 - 获取合约交易V2](#3-fetchcontracttxsv2---获取合约交易v2)
  - [4. GetBlocks - 获取区块列表](#4-getblocks---获取区块列表)
  - [5. GetBlock - 获取单个区块](#5-getblock---获取单个区块)
  - [6. FetchValidators - 获取验证者列表](#6-fetchvalidators---获取验证者列表)
  - [7. FetchValidator - 获取单个验证者](#7-fetchvalidator---获取单个验证者)
  - [8. FetchValidatorUptime - 获取验证者在线率](#8-fetchvalidatoruptime---获取验证者在线率)
  - [9. GetTxs - 获取交易列表](#9-gettxs---获取交易列表)
  - [10. GetTxByTxHash - 通过交易哈希获取交易](#10-gettxbytxhash---通过交易哈希获取交易)
  - [11. GetPeggyDeposits - 获取Peggy充值交易](#11-getpeggydeposits---获取peggy充值交易)
  - [12. GetPeggyWithdrawals - 获取Peggy提现交易](#12-getpeggywithdrawals---获取peggy提现交易)
  - [13. GetIBCTransfers - 获取IBC转账交易](#13-getibctransfers---获取ibc转账交易)
  - [14. GetWasmCodes - 获取Wasm代码列表](#14-getwasmcodes---获取wasm代码列表)
  - [15. GetWasmCodeByID - 通过ID获取Wasm代码](#15-getwasmcodebyid---通过id获取wasm代码)
  - [16. GetWasmContracts - 获取Wasm合约列表](#16-getwasmcontracts---获取wasm合约列表)
  - [17. GetWasmContractByAddress - 通过地址获取Wasm合约](#17-getwasmcontractbyaddress---通过地址获取wasm合约)
  - [18. GetCW20Balance - 获取CW20余额](#18-getcw20balance---获取cw20余额)
  - [19. FetchRelayers - 获取中继者信息](#19-fetchrelayers---获取中继者信息)
  - [20. FetchBankTransfers - 获取银行转账](#20-fetchbanktransfers---获取银行转账)
  - [21. StreamTxs - 流式交易订阅](#21-streamtxs---流式交易订阅)
  - [22. StreamBlocks - 流式区块订阅](#22-streamblocks---流式区块订阅)
- [附录：通用数据结构](#附录通用数据结构)

---

## 接口概览

`ExplorerClient` 接口提供了以下方法：

| 方法名 | 功能描述 | 类型 |
|--------|---------|------|
| GetAccountTxs | 获取指定账户的交易记录 | RPC |
| FetchContractTxs | 获取指定合约的交易记录 | RPC |
| FetchContractTxsV2 | 获取指定合约的交易记录(V2版本) | RPC |
| GetBlocks | 获取区块列表 | RPC |
| GetBlock | 获取单个区块详情 | RPC |
| FetchValidators | 获取所有验证者信息 | RPC |
| FetchValidator | 获取单个验证者信息 | RPC |
| FetchValidatorUptime | 获取验证者在线时间统计 | RPC |
| GetTxs | 获取交易列表 | RPC |
| GetTxByTxHash | 通过交易哈希获取交易详情 | RPC |
| GetPeggyDeposits | 获取Peggy充值交易 | RPC |
| GetPeggyWithdrawals | 获取Peggy提现交易 | RPC |
| GetIBCTransfers | 获取IBC跨链转账交易 | RPC |
| GetWasmCodes | 获取Wasm代码列表 | RPC |
| GetWasmCodeByID | 通过ID获取Wasm代码详情 | RPC |
| GetWasmContracts | 获取Wasm合约列表 | RPC |
| GetWasmContractByAddress | 通过地址获取Wasm合约详情 | RPC |
| GetCW20Balance | 获取CW20代币余额 | RPC |
| FetchRelayers | 获取中继者市场信息 | RPC |
| FetchBankTransfers | 获取银行转账记录 | RPC |
| StreamTxs | 订阅实时交易流 | Stream |
| StreamBlocks | 订阅实时区块流 | Stream |

---

## 详细接口说明

### 1. GetAccountTxs - 获取账户交易

获取指定账户的交易记录，支持多种过滤条件。

#### 方法签名

```go
GetAccountTxs(ctx context.Context, req *explorerpb.GetAccountTxsRequest) (*explorerpb.GetAccountTxsResponse, error)
```

#### 请求参数 (GetAccountTxsRequest)

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| Address | string | 是 | 账户地址 |
| Before | uint64 | 否 | 返回此区块高度之前的交易 |
| After | uint64 | 否 | 返回此区块高度之后的交易 |
| Limit | int32 | 否 | 返回结果数量限制 |
| Skip | uint64 | 否 | 跳过的记录数 |
| Type | string | 否 | 交易类型过滤 |
| Module | string | 否 | 模块名称过滤 |
| FromNumber | int64 | 否 | 起始区块号 |
| ToNumber | int64 | 否 | 结束区块号 |
| StartTime | int64 | 否 | 起始时间戳(UNIX毫秒) |
| EndTime | int64 | 否 | 结束时间戳(UNIX毫秒) |
| Status | string | 否 | 交易状态过滤 |

#### 响应参数 (GetAccountTxsResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Paging | *Paging | 分页信息 |
| Data | []*TxDetailData | 交易详情数组 |

---

### 2. FetchContractTxs - 获取合约交易

获取指定智能合约的交易记录。

#### 方法签名

```go
FetchContractTxs(ctx context.Context, req *explorerpb.GetContractTxsRequest) (*explorerpb.GetContractTxsResponse, error)
```

#### 请求参数 (GetContractTxsRequest)

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| Address | string | 是 | 合约地址 |
| Limit | int32 | 否 | 返回结果数量限制 |
| Skip | uint64 | 否 | 跳过的记录数 |
| FromNumber | int64 | 否 | 起始区块号 |
| ToNumber | int64 | 否 | 结束区块号 |

#### 响应参数 (GetContractTxsResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Paging | *Paging | 分页信息 |
| Data | []*TxDetailData | 交易详情数组 |

---

### 3. FetchContractTxsV2 - 获取合约交易V2

获取指定智能合约的交易记录(V2版本，支持更多过滤条件)。

#### 方法签名

```go
FetchContractTxsV2(ctx context.Context, req *explorerpb.GetContractTxsV2Request) (*explorerpb.GetContractTxsV2Response, error)
```

#### 请求参数 (GetContractTxsV2Request)

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| Address | string | 是 | 合约地址 |
| Height | uint64 | 否 | 区块高度 |
| From | int64 | 否 | 起始时间戳(UNIX毫秒) |
| To | int64 | 否 | 结束时间戳(UNIX毫秒) |
| PerPage | int32 | 否 | 每页返回数量 |
| Token | string | 否 | 分页令牌 |
| Status | string | 否 | 交易状态过滤 |

#### 响应参数 (GetContractTxsV2Response)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Paging | *Cursor | 游标分页信息 |
| Data | []*TxDetailData | 交易详情数组 |

---

### 4. GetBlocks - 获取区块列表

获取区块列表，支持时间范围和分页。

#### 方法签名

```go
GetBlocks(ctx context.Context) (*explorerpb.GetBlocksResponse, error)
```

#### 请求参数

此接口在客户端内部创建空请求，不需要传入参数。实际的 GetBlocksRequest 结构支持以下字段：

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| Before | uint64 | 否 | 返回此区块高度之前的区块 |
| After | uint64 | 否 | 返回此区块高度之后的区块 |
| Limit | int32 | 否 | 返回结果数量限制 |
| From | uint64 | 否 | 起始时间戳(UNIX毫秒) |
| To | uint64 | 否 | 结束时间戳(UNIX毫秒) |

#### 响应参数 (GetBlocksResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Paging | *Paging | 分页信息 |
| Data | []*BlockInfo | 区块信息数组 |

---

### 5. GetBlock - 获取单个区块

根据区块高度获取单个区块的详细信息。

#### 方法签名

```go
GetBlock(ctx context.Context, blockHeight string) (*explorerpb.GetBlockResponse, error)
```

#### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| blockHeight | string | 是 | 区块高度 |

#### 响应参数 (GetBlockResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| S | string | 响应状态 |
| Errmsg | string | 错误消息 |
| Data | *BlockDetailInfo | 区块详细信息 |

---

### 6. FetchValidators - 获取验证者列表

获取所有验证者的信息列表。

#### 方法签名

```go
FetchValidators(ctx context.Context) (*explorerpb.GetValidatorsResponse, error)
```

#### 请求参数

无需传入参数。

#### 响应参数 (GetValidatorsResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| S | string | 响应状态 |
| Errmsg | string | 错误消息 |
| Data | []*Validator | 验证者信息数组 |

---

### 7. FetchValidator - 获取单个验证者

根据验证者地址获取单个验证者的详细信息。

#### 方法签名

```go
FetchValidator(ctx context.Context, address string) (*explorerpb.GetValidatorResponse, error)
```

#### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| address | string | 是 | 验证者地址 |

#### 响应参数 (GetValidatorResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| S | string | 响应状态 |
| Errmsg | string | 错误消息 |
| Data | *Validator | 验证者详细信息 |

---

### 8. FetchValidatorUptime - 获取验证者在线率

获取验证者的在线时间统计信息。

#### 方法签名

```go
FetchValidatorUptime(ctx context.Context, address string) (*explorerpb.GetValidatorUptimeResponse, error)
```

#### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| address | string | 是 | 验证者地址 |

#### 响应参数 (GetValidatorUptimeResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| S | string | 响应状态 |
| Errmsg | string | 错误消息 |
| Data | []*ValidatorUptime | 验证者在线时间统计数组 |

---

### 9. GetTxs - 获取交易列表

获取交易列表，支持多种过滤条件。

#### 方法签名

```go
GetTxs(ctx context.Context, req *explorerpb.GetTxsRequest) (*explorerpb.GetTxsResponse, error)
```

#### 请求参数 (GetTxsRequest)

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| Before | uint64 | 否 | 返回此区块高度之前的交易 |
| After | uint64 | 否 | 返回此区块高度之后的交易 |
| Limit | int32 | 否 | 返回结果数量限制 |
| Skip | uint64 | 否 | 跳过的记录数 |
| Type | string | 否 | 交易类型过滤 |
| Module | string | 否 | 模块名称过滤 |
| FromNumber | int64 | 否 | 起始区块号 |
| ToNumber | int64 | 否 | 结束区块号 |
| StartTime | int64 | 否 | 起始时间戳(UNIX毫秒) |
| EndTime | int64 | 否 | 结束时间戳(UNIX毫秒) |
| Status | string | 否 | 交易状态过滤 |

#### 响应参数 (GetTxsResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Paging | *Paging | 分页信息 |
| Data | []*TxData | 交易数据数组 |

---

### 10. GetTxByTxHash - 通过交易哈希获取交易

根据交易哈希获取交易的详细信息。

#### 方法签名

```go
GetTxByTxHash(ctx context.Context, hash string) (*explorerpb.GetTxByTxHashResponse, error)
```

#### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| hash | string | 是 | 交易哈希 |

内部构造的 GetTxByTxHashRequest 还支持：

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| Hash | string | 是 | 交易哈希 |
| IsEvmHash | bool | 否 | 如果提供的哈希可能是EVM交易哈希，设置为true |

#### 响应参数 (GetTxByTxHashResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| S | string | 响应状态 |
| Errmsg | string | 错误消息 |
| Data | *TxDetailData | 交易详细信息 |

---

### 11. GetPeggyDeposits - 获取Peggy充值交易

获取Peggy桥充值交易记录。

#### 方法签名

```go
GetPeggyDeposits(ctx context.Context, req *explorerpb.GetPeggyDepositTxsRequest) (*explorerpb.GetPeggyDepositTxsResponse, error)
```

#### 请求参数 (GetPeggyDepositTxsRequest)

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| Sender | string | 否 | 充值请求发送者地址 |
| Receiver | string | 否 | 充值接收者地址 |
| Limit | int32 | 否 | 返回结果数量限制 |
| Skip | uint64 | 否 | 跳过的记录数 |

#### 响应参数 (GetPeggyDepositTxsResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Field | []*PeggyDepositTx | Peggy充值交易数组 |

---

### 12. GetPeggyWithdrawals - 获取Peggy提现交易

获取Peggy桥提现交易记录。

#### 方法签名

```go
GetPeggyWithdrawals(ctx context.Context, req *explorerpb.GetPeggyWithdrawalTxsRequest) (*explorerpb.GetPeggyWithdrawalTxsResponse, error)
```

#### 请求参数 (GetPeggyWithdrawalTxsRequest)

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| Sender | string | 否 | 提现请求发送者地址 |
| Receiver | string | 否 | 提现接收者地址 |
| Limit | int32 | 否 | 返回结果数量限制 |
| Skip | uint64 | 否 | 跳过的记录数 |

#### 响应参数 (GetPeggyWithdrawalTxsResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Field | []*PeggyWithdrawalTx | Peggy提现交易数组 |

---

### 13. GetIBCTransfers - 获取IBC转账交易

获取IBC跨链转账交易记录。

#### 方法签名

```go
GetIBCTransfers(ctx context.Context, req *explorerpb.GetIBCTransferTxsRequest) (*explorerpb.GetIBCTransferTxsResponse, error)
```

#### 请求参数 (GetIBCTransferTxsRequest)

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| Sender | string | 否 | 发送者地址 |
| Receiver | string | 否 | 接收者地址 |
| SrcChannel | string | 否 | 源通道 |
| SrcPort | string | 否 | 源端口 |
| DestChannel | string | 否 | 目标通道 |
| DestPort | string | 否 | 目标端口 |
| Limit | int32 | 否 | 返回结果数量限制 |
| Skip | uint64 | 否 | 跳过的记录数 |

#### 响应参数 (GetIBCTransferTxsResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Field | []*IBCTransferTx | IBC转账交易数组 |

---

### 14. GetWasmCodes - 获取Wasm代码列表

获取已存储的Wasm代码列表。

#### 方法签名

```go
GetWasmCodes(ctx context.Context, req *explorerpb.GetWasmCodesRequest) (*explorerpb.GetWasmCodesResponse, error)
```

#### 请求参数 (GetWasmCodesRequest)

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| Limit | int32 | 否 | 返回结果数量限制 |
| FromNumber | int64 | 否 | 起始编号 |
| ToNumber | int64 | 否 | 结束编号 |

#### 响应参数 (GetWasmCodesResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Paging | *Paging | 分页信息 |
| Data | []*WasmCode | Wasm代码数组 |

---

### 15. GetWasmCodeByID - 通过ID获取Wasm代码

根据代码ID获取Wasm代码的详细信息。

#### 方法签名

```go
GetWasmCodeByID(ctx context.Context, req *explorerpb.GetWasmCodeByIDRequest) (*explorerpb.GetWasmCodeByIDResponse, error)
```

#### 请求参数 (GetWasmCodeByIDRequest)

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| CodeId | int64 | 是 | 代码ID |

#### 响应参数 (GetWasmCodeByIDResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| CodeId | uint64 | 存储的wasm代码ID，降序排列 |
| TxHash | string | 存储代码交易的哈希 |
| Checksum | *Checksum | CosmWasm代码的校验和 |
| CreatedAt | uint64 | 代码存储时的区块时间戳(毫秒) |
| ContractType | string | Wasm代码的合约类型 |
| Version | string | Wasm代码的版本字符串 |
| Permission | *ContractPermission | 实例化权限描述 |
| CodeSchema | string | 代码schema预览 |
| CodeView | string | 代码仓库预览，可能包含schema文件夹 |
| Instantiates | uint64 | 从此代码实例化的合约数量 |
| Creator | string | 此代码的创建者 |
| CodeNumber | int64 | 代码存储的单调顺序 |
| ProposalId | int64 | 存储此代码的提案ID |

---

### 16. GetWasmContracts - 获取Wasm合约列表

获取已部署的Wasm合约列表。

#### 方法签名

```go
GetWasmContracts(ctx context.Context, req *explorerpb.GetWasmContractsRequest) (*explorerpb.GetWasmContractsResponse, error)
```

#### 请求参数 (GetWasmContractsRequest)

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| Limit | int32 | 否 | 返回结果数量限制 |
| CodeId | int64 | 否 | 按代码ID过滤 |
| FromNumber | int64 | 否 | 起始编号 |
| ToNumber | int64 | 否 | 结束编号 |
| AssetsOnly | bool | 否 | 仅返回资产合约 |
| Skip | int64 | 否 | 跳过的记录数 |
| Label | string | 否 | 合约标签过滤 |
| Token | string | 否 | 按代币名称或符号过滤 |
| Lookup | string | 否 | 按文本查找 |

#### 响应参数 (GetWasmContractsResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Paging | *Paging | 分页信息 |
| Data | []*WasmContract | Wasm合约数组 |

---

### 17. GetWasmContractByAddress - 通过地址获取Wasm合约

根据合约地址获取Wasm合约的详细信息。

#### 方法签名

```go
GetWasmContractByAddress(ctx context.Context, req *explorerpb.GetWasmContractByAddressRequest) (*explorerpb.GetWasmContractByAddressResponse, error)
```

#### 请求参数 (GetWasmContractByAddressRequest)

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| ContractAddress | string | 是 | 合约地址 |

#### 响应参数 (GetWasmContractByAddressResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Label | string | 合约的通用名称 |
| Address | string | 合约地址 |
| TxHash | string | 实例化交易的哈希 |
| Creator | string | 合约创建者地址 |
| Executes | uint64 | 执行合约的次数 |
| InstantiatedAt | uint64 | 合约实例化时的区块时间戳(毫秒) |
| InitMessage | string | 实例化时的初始化消息 |
| LastExecutedAt | uint64 | 最后一次调用合约的区块时间戳(毫秒) |
| Funds | []*ContractFund | 合约资金 |
| CodeId | uint64 | 合约的代码ID |
| Admin | string | 合约的管理员 |
| CurrentMigrateMessage | string | 合约的最新迁移消息 |
| ContractNumber | int64 | 数据库中的单调合约编号 |
| Version | string | 合约版本字符串 |
| Type | string | 合约类型 |
| Cw20Metadata | *Cw20Metadata | CW20元数据 |
| ProposalId | int64 | 实例化此合约的提案ID |

---

### 18. GetCW20Balance - 获取CW20余额

获取指定地址的CW20代币余额。

#### 方法签名

```go
GetCW20Balance(ctx context.Context, req *explorerpb.GetCw20BalanceRequest) (*explorerpb.GetCw20BalanceResponse, error)
```

#### 请求参数 (GetCw20BalanceRequest)

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| Address | string | 是 | 要查询余额的地址 |
| Limit | int32 | 否 | 返回结果数量限制 |

#### 响应参数 (GetCw20BalanceResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Field | []*WasmCw20Balance | CW20余额数组 |

---

### 19. FetchRelayers - 获取中继者信息

获取指定市场的中继者信息。

#### 方法签名

```go
FetchRelayers(ctx context.Context, marketIDs []string) (*explorerpb.RelayersResponse, error)
```

#### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| marketIDs | []string | 否 | 要查询的市场ID数组 |

#### 响应参数 (RelayersResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Field | []*RelayerMarkets | 中继者市场信息数组 |

---

### 20. FetchBankTransfers - 获取银行转账

获取银行转账记录。

#### 方法签名

```go
FetchBankTransfers(ctx context.Context, req *explorerpb.GetBankTransfersRequest) (*explorerpb.GetBankTransfersResponse, error)
```

#### 请求参数 (GetBankTransfersRequest)

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| Senders | []string | 否 | 转账发送者地址数组 |
| Recipients | []string | 否 | 转账接收者地址数组 |
| IsCommunityPoolRelated | bool | 否 | 返回与社区池相关的转账 |
| Limit | int32 | 否 | 返回结果数量限制 |
| Skip | uint64 | 否 | 跳过的记录数 |
| StartTime | int64 | 否 | 起始时间戳(UNIX毫秒) |
| EndTime | int64 | 否 | 结束时间戳(UNIX毫秒) |
| Address | []string | 否 | 发送者或接收者地址数组 |
| PerPage | int32 | 否 | 每页返回数量 |
| Token | string | 否 | 分页令牌 |

#### 响应参数 (GetBankTransfersResponse)

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Paging | *Paging | 分页信息 |
| Data | []*BankTransfer | 银行转账数组 |

---

### 21. StreamTxs - 流式交易订阅

订阅实时交易流。

#### 方法签名

```go
StreamTxs(ctx context.Context) (explorerpb.InjectiveExplorerRPC_StreamTxsClient, error)
```

#### 请求参数

无需传入参数。

#### 响应 (StreamTxsResponse - 流式)

返回一个流客户端，可以持续接收交易更新。每个消息包含：

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Id | string | 交易ID |
| BlockNumber | uint64 | 区块号 |
| BlockTimestamp | string | 区块时间戳 |
| Hash | string | 交易哈希 |
| Codespace | string | 代码空间 |
| Messages | string | 消息内容 |
| TxNumber | uint64 | 交易编号 |
| ErrorLog | string | 错误日志 |
| Code | uint32 | 响应代码 |
| ClaimIds | []int64 | Peggy桥claim ID数组 |

---

### 22. StreamBlocks - 流式区块订阅

订阅实时区块流。

#### 方法签名

```go
StreamBlocks(ctx context.Context) (explorerpb.InjectiveExplorerRPC_StreamBlocksClient, error)
```

#### 请求参数

无需传入参数。

#### 响应 (StreamBlocksResponse - 流式)

返回一个流客户端，可以持续接收区块更新。每个消息包含：

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Height | uint64 | 区块高度 |
| Proposer | string | 出块者 |
| Moniker | string | 验证者名称 |
| BlockHash | string | 区块哈希 |
| ParentHash | string | 父区块哈希 |
| NumPreCommits | int64 | 预提交数量 |
| NumTxs | int64 | 交易数量 |
| Txs | []*TxDataRPC | 交易数据数组 |
| Timestamp | string | 时间戳 |
| BlockUnixTimestamp | uint64 | Unix毫秒时间戳 |

---

## 附录：通用数据结构

### Paging - 分页信息

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Total | int64 | 数据库中保存的总记录数 |
| From | int32 | 起始位置(可以是区块高度或索引号) |
| To | int32 | 结束位置(可以是区块高度或索引号) |
| CountBySubaccount | int64 | 按子账户统计的数量 |
| Next | []string | 导航到下一页的令牌数组 |

### Cursor - 游标分页

用于V2版本接口的游标分页。

### TxDetailData - 交易详情

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Id | string | 交易ID |
| BlockNumber | uint64 | 区块号 |
| BlockTimestamp | string | 区块时间戳 |
| Hash | string | 交易哈希 |
| Code | uint32 | 响应代码 |
| Data | []byte | 交易数据 |
| Info | string | 附加信息 |
| GasWanted | int64 | 期望的Gas |
| GasUsed | int64 | 实际使用的Gas |
| GasFee | *GasFee | Gas费用 |
| Codespace | string | 代码空间 |
| Events | []*Event | 事件数组 |
| TxType | string | 交易类型 |
| Messages | []byte | 消息内容 |
| Signatures | []*Signature | 签名数组 |
| Memo | string | 备注 |
| TxNumber | uint64 | 交易编号 |
| BlockUnixTimestamp | uint64 | Unix毫秒时间戳 |
| ErrorLog | string | 错误日志 |
| Logs | []byte | 事件日志 |
| ClaimIds | []int64 | Peggy桥claim ID数组 |

### BlockInfo - 区块信息

包含区块的基本信息。

### BlockDetailInfo - 区块详细信息

包含区块的详细信息，比BlockInfo包含更多字段。

### Validator - 验证者

包含验证者的基本信息和统计数据。

### ValidatorUptime - 验证者在线时间

包含验证者的在线时间统计。

### TxData - 交易数据

交易的简化数据结构。

### PeggyDepositTx - Peggy充值交易

Peggy桥充值交易的详细信息。

### PeggyWithdrawalTx - Peggy提现交易

Peggy桥提现交易的详细信息。

### IBCTransferTx - IBC转账交易

IBC跨链转账交易的详细信息。

### WasmCode - Wasm代码

存储的Wasm代码信息。

### WasmContract - Wasm合约

已部署的Wasm合约信息。

### WasmCw20Balance - CW20余额

CW20代币余额信息。

### BankTransfer - 银行转账

银行转账记录信息。

### RelayerMarkets - 中继者市场

中继者的市场信息。

---

## 使用示例

### 创建客户端

```go
import (
    "context"
    "github.com/InjectiveLabs/sdk-go/client/common"
    "github.com/InjectiveLabs/sdk-go/client/explorer"
)

// 创建Explorer客户端
network := common.LoadNetwork("testnet", "lb")
explorerClient, err := explorer.NewExplorerClient(network)
if err != nil {
    panic(err)
}
defer explorerClient.Close()
```

### 获取账户交易

```go
ctx := context.Background()
req := &explorerpb.GetAccountTxsRequest{
    Address: "inj1...",
    Limit:   10,
}

res, err := explorerClient.GetAccountTxs(ctx, req)
if err != nil {
    panic(err)
}

for _, tx := range res.Data {
    fmt.Printf("Tx Hash: %s, Block: %d\n", tx.Hash, tx.BlockNumber)
}
```

### 订阅实时交易

```go
ctx := context.Background()
stream, err := explorerClient.StreamTxs(ctx)
if err != nil {
    panic(err)
}

for {
    res, err := stream.Recv()
    if err != nil {
        break
    }
    fmt.Printf("New Tx: %s\n", res.Hash)
}
```

---

## 注意事项

1. **时间戳格式**：所有时间戳字段均使用UNIX毫秒格式
2. **分页**：大多数列表接口支持分页，建议设置合理的Limit值
3. **流式接口**：StreamTxs和StreamBlocks是长连接，需要妥善处理连接断开和重连
4. **错误处理**：部分接口的响应中包含`S`(status)和`Errmsg`字段用于错误处理
5. **上下文管理**：所有接口都需要传入context.Context，建议设置合理的超时时间

---

**文档版本**: 1.0
**最后更新**: 2026-02-08
**基于代码**: sdk-go/client/explorer/explorer.go
