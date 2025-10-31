下面是 Injective 区块链交易消息类型 (`tx_msg_types`) 支持的消息类型

## 支持的消息类型

### 交易相关 (exchange)

- `/injective.exchange.v1beta1.MsgCreateSpotMarketOrder` - 创建现货市场订单
- `/injective.exchange.v1beta1.MsgCreateDerivativeMarketOrder` - 创建衍生品市场订单
- `/injective.exchange.v1beta1.MsgCancelSpotOrder` - 取消现货订单
- `/injective.exchange.v1beta1.MsgCancelDerivativeOrder` - 取消衍生品订单
- `/injective.exchange.v1beta1.MsgBatchUpdateOrders` - 批量更新订单

### 转账相关 (bank)

- `/cosmos.bank.v1beta1.MsgSend` - 发送代币
- `/cosmos.bank.v1beta1.MsgMultiSend` - 批量发送代币

### 质押相关 (staking)

- `/cosmos.staking.v1beta1.MsgDelegate` - 委托质押
- `/cosmos.staking.v1beta1.MsgUndelegate` - 取消委托
- `/cosmos.staking.v1beta1.MsgBeginRedelegate` - 重新委托

### 智能合约相关 (wasm)

- `/cosmwasm.wasm.v1.MsgExecuteContract` - 执行智能合约
- `/cosmwasm.wasm.v1.MsgInstantiateContract` - 实例化智能合约
- `/injective.wasmx.v1.MsgExecuteContractCompat` - 执行合约（兼容模式）

### 授权相关 (authz)

- `/cosmos.authz.v1beta1.MsgExec` - 授权执行
- `/cosmos.authz.v1beta1.MsgGrant` - 授权授予
- `/cosmos.authz.v1beta1.MsgRevoke` - 授权撤销

### 奖励相关 (distribution)

- `/cosmos.distribution.v1beta1.MsgWithdrawDelegatorReward` - 提取委托奖励
- `/cosmos.distribution.v1beta1.MsgWithdrawValidatorCommission` - 提取验证者佣金

### 跨链相关 (peggy/ibc)

- `/injective.peggy.v1.MsgSendToEth` - 发送到以太坊
- `/injective.peggy.v1.MsgRequestBatch` - 请求批量处理
- `/ibc.core.client.v1.MsgCreateClient` - 创建客户端

### 治理相关 (gov)

- `/cosmos.gov.v1beta1.MsgSubmitProposal` - 提交提案
- `/cosmos.gov.v1beta1.MsgVote` - 投票
