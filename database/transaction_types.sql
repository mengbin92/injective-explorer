-- ============================================
-- 交易类型数据库表结构
-- 默认使用 MySQL 语法
-- 
-- 如需适配其他数据库：
-- PostgreSQL: 将 AUTO_INCREMENT 改为 SERIAL，将 TINYINT(1) 改为 BOOLEAN，移除 COMMENT
-- SQLite3: 将 AUTO_INCREMENT 改为 AUTOINCREMENT，将 TINYINT(1) 改为 INTEGER，移除 COMMENT
-- ============================================

-- 创建交易类型表
CREATE TABLE IF NOT EXISTS transaction_types (
    id INT AUTO_INCREMENT PRIMARY KEY,
    
    full_type_name VARCHAR(255) NOT NULL UNIQUE COMMENT '完整类型名，如 injective.exchange.v1beta1.MsgDeposit',
    module VARCHAR(100) NOT NULL COMMENT '模块名，如 exchange, auction, erc20 等',
    version VARCHAR(50) DEFAULT '' COMMENT '版本号，如 v1beta1, v2, v1 等',
    message_type VARCHAR(100) NOT NULL COMMENT '消息类型，如 MsgDeposit, MsgWithdraw 等',
    category VARCHAR(100) DEFAULT '' COMMENT '分类，如 账户操作、现货订单、衍生品订单 等',
    description_zh TEXT COMMENT '中文描述',
    description_en TEXT COMMENT '英文描述',
    is_active TINYINT(1) DEFAULT 1 COMMENT '是否激活，1=激活，0=停用',
    requires_admin TINYINT(1) DEFAULT 0 COMMENT '是否需要管理员权限',
    is_deprecated TINYINT(1) DEFAULT 0 COMMENT '是否已废弃',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='交易类型表';

-- 创建索引
-- MySQL 5.7+ 支持 IF NOT EXISTS，旧版本需要先检查索引是否存在
CREATE INDEX idx_module ON transaction_types(module);
CREATE INDEX idx_version ON transaction_types(version);
CREATE INDEX idx_message_type ON transaction_types(message_type);
CREATE INDEX idx_category ON transaction_types(category);
CREATE INDEX idx_is_active ON transaction_types(is_active);
CREATE INDEX idx_full_type_name ON transaction_types(full_type_name);

-- ============================================
-- 插入交易类型数据
-- ============================================

-- Exchange v1beta1 模块
INSERT INTO transaction_types (full_type_name, module, version, message_type, category, description_zh, is_active) VALUES
-- 账户操作
('injective.exchange.v1beta1.MsgDeposit', 'exchange', 'v1beta1', 'MsgDeposit', '账户操作', '存入资金到子账户', 1),
('injective.exchange.v1beta1.MsgWithdraw', 'exchange', 'v1beta1', 'MsgWithdraw', '账户操作', '从子账户提取资金', 1),
('injective.exchange.v1beta1.MsgSubaccountTransfer', 'exchange', 'v1beta1', 'MsgSubaccountTransfer', '账户操作', '子账户间转账', 1),
('injective.exchange.v1beta1.MsgExternalTransfer', 'exchange', 'v1beta1', 'MsgExternalTransfer', '账户操作', '外部转账', 1),

-- 现货订单
('injective.exchange.v1beta1.MsgCreateSpotLimitOrder', 'exchange', 'v1beta1', 'MsgCreateSpotLimitOrder', '现货订单', '创建现货限价单', 1),
('injective.exchange.v1beta1.MsgBatchCreateSpotLimitOrders', 'exchange', 'v1beta1', 'MsgBatchCreateSpotLimitOrders', '现货订单', '批量创建现货限价单', 1),
('injective.exchange.v1beta1.MsgCreateSpotMarketOrder', 'exchange', 'v1beta1', 'MsgCreateSpotMarketOrder', '现货订单', '创建现货市价单', 1),
('injective.exchange.v1beta1.MsgCancelSpotOrder', 'exchange', 'v1beta1', 'MsgCancelSpotOrder', '现货订单', '取消现货订单', 1),
('injective.exchange.v1beta1.MsgBatchCancelSpotOrders', 'exchange', 'v1beta1', 'MsgBatchCancelSpotOrders', '现货订单', '批量取消现货订单', 1),

-- 衍生品订单
('injective.exchange.v1beta1.MsgCreateDerivativeLimitOrder', 'exchange', 'v1beta1', 'MsgCreateDerivativeLimitOrder', '衍生品订单', '创建衍生品限价单', 1),
('injective.exchange.v1beta1.MsgBatchCreateDerivativeLimitOrders', 'exchange', 'v1beta1', 'MsgBatchCreateDerivativeLimitOrders', '衍生品订单', '批量创建衍生品限价单', 1),
('injective.exchange.v1beta1.MsgCreateDerivativeMarketOrder', 'exchange', 'v1beta1', 'MsgCreateDerivativeMarketOrder', '衍生品订单', '创建衍生品市价单', 1),
('injective.exchange.v1beta1.MsgCancelDerivativeOrder', 'exchange', 'v1beta1', 'MsgCancelDerivativeOrder', '衍生品订单', '取消衍生品订单', 1),
('injective.exchange.v1beta1.MsgBatchCancelDerivativeOrders', 'exchange', 'v1beta1', 'MsgBatchCancelDerivativeOrders', '衍生品订单', '批量取消衍生品订单', 1),

-- 二元期权订单
('injective.exchange.v1beta1.MsgCreateBinaryOptionsLimitOrder', 'exchange', 'v1beta1', 'MsgCreateBinaryOptionsLimitOrder', '二元期权订单', '创建二元期权限价单', 1),
('injective.exchange.v1beta1.MsgCreateBinaryOptionsMarketOrder', 'exchange', 'v1beta1', 'MsgCreateBinaryOptionsMarketOrder', '二元期权订单', '创建二元期权市价单', 1),
('injective.exchange.v1beta1.MsgCancelBinaryOptionsOrder', 'exchange', 'v1beta1', 'MsgCancelBinaryOptionsOrder', '二元期权订单', '取消二元期权订单', 1),
('injective.exchange.v1beta1.MsgBatchCancelBinaryOptionsOrders', 'exchange', 'v1beta1', 'MsgBatchCancelBinaryOptionsOrders', '二元期权订单', '批量取消二元期权订单', 1),

-- 市场管理
('injective.exchange.v1beta1.MsgInstantSpotMarketLaunch', 'exchange', 'v1beta1', 'MsgInstantSpotMarketLaunch', '市场管理', '即时启动现货市场', 1),
('injective.exchange.v1beta1.MsgInstantBinaryOptionsMarketLaunch', 'exchange', 'v1beta1', 'MsgInstantBinaryOptionsMarketLaunch', '市场管理', '即时启动二元期权市场', 1),
('injective.exchange.v1beta1.MsgUpdateSpotMarket', 'exchange', 'v1beta1', 'MsgUpdateSpotMarket', '市场管理', '更新现货市场', 1),
('injective.exchange.v1beta1.MsgUpdateDerivativeMarket', 'exchange', 'v1beta1', 'MsgUpdateDerivativeMarket', '市场管理', '更新衍生品市场', 1),
('injective.exchange.v1beta1.MsgAdminUpdateBinaryOptionsMarket', 'exchange', 'v1beta1', 'MsgAdminUpdateBinaryOptionsMarket', '市场管理', '管理员更新二元期权市场', 1),
('injective.exchange.v1beta1.MsgBatchExchangeModification', 'exchange', 'v1beta1', 'MsgBatchExchangeModification', '市场管理', '批量交易所修改', 1),

-- 仓位管理
('injective.exchange.v1beta1.MsgIncreasePositionMargin', 'exchange', 'v1beta1', 'MsgIncreasePositionMargin', '仓位管理', '增加仓位保证金', 1),
('injective.exchange.v1beta1.MsgDecreasePositionMargin', 'exchange', 'v1beta1', 'MsgDecreasePositionMargin', '仓位管理', '减少仓位保证金', 1),
('injective.exchange.v1beta1.MsgLiquidatePosition', 'exchange', 'v1beta1', 'MsgLiquidatePosition', '仓位管理', '清算仓位', 1),
('injective.exchange.v1beta1.MsgEmergencySettleMarket', 'exchange', 'v1beta1', 'MsgEmergencySettleMarket', '仓位管理', '紧急结算市场', 1),

-- 批量操作
('injective.exchange.v1beta1.MsgBatchUpdateOrders', 'exchange', 'v1beta1', 'MsgBatchUpdateOrders', '批量操作', '批量更新订单', 1),

-- 其他
('injective.exchange.v1beta1.MsgPrivilegedExecuteContract', 'exchange', 'v1beta1', 'MsgPrivilegedExecuteContract', '其他', '特权执行合约', 1),
('injective.exchange.v1beta1.MsgRewardsOptOut', 'exchange', 'v1beta1', 'MsgRewardsOptOut', '其他', '退出奖励', 1),
('injective.exchange.v1beta1.MsgReclaimLockedFunds', 'exchange', 'v1beta1', 'MsgReclaimLockedFunds', '其他', '回收锁定资金', 1),
('injective.exchange.v1beta1.MsgAuthorizeStakeGrants', 'exchange', 'v1beta1', 'MsgAuthorizeStakeGrants', '其他', '授权质押授权', 1),
('injective.exchange.v1beta1.MsgActivateStakeGrant', 'exchange', 'v1beta1', 'MsgActivateStakeGrant', '其他', '激活质押授权', 1),
('injective.exchange.v1beta1.MsgUpdateParams', 'exchange', 'v1beta1', 'MsgUpdateParams', '其他', '更新参数', 1),
('injective.exchange.v1beta1.MsgSignData', 'exchange', 'v1beta1', 'MsgSignData', '其他', '签名数据', 1),
('injective.exchange.v1beta1.MsgSignDoc', 'exchange', 'v1beta1', 'MsgSignDoc', '其他', '签名文档', 1);

-- Exchange v2 模块
INSERT INTO transaction_types (full_type_name, module, version, message_type, category, description_zh, is_active) VALUES
-- 市场启动
('injective.exchange.v2.MsgSpotMarketLaunch', 'exchange', 'v2', 'MsgSpotMarketLaunch', '市场启动', '启动现货市场', 1),
('injective.exchange.v2.MsgPerpetualMarketLaunch', 'exchange', 'v2', 'MsgPerpetualMarketLaunch', '市场启动', '启动永续市场', 1),
('injective.exchange.v2.MsgExpiryFuturesMarketLaunch', 'exchange', 'v2', 'MsgExpiryFuturesMarketLaunch', '市场启动', '启动到期期货市场', 1),
('injective.exchange.v2.MsgBinaryOptionsMarketLaunch', 'exchange', 'v2', 'MsgBinaryOptionsMarketLaunch', '市场启动', '启动二元期权市场', 1),
('injective.exchange.v2.MsgInstantPerpetualMarketLaunch', 'exchange', 'v2', 'MsgInstantPerpetualMarketLaunch', '市场启动', '即时启动永续市场', 1),
('injective.exchange.v2.MsgInstantExpiryFuturesMarketLaunch', 'exchange', 'v2', 'MsgInstantExpiryFuturesMarketLaunch', '市场启动', '即时启动到期期货市场', 1),
('injective.exchange.v2.MsgInstantSpotMarketLaunch', 'exchange', 'v2', 'MsgInstantSpotMarketLaunch', '市场启动', '即时启动现货市场', 1),
('injective.exchange.v2.MsgInstantBinaryOptionsMarketLaunch', 'exchange', 'v2', 'MsgInstantBinaryOptionsMarketLaunch', '市场启动', '即时启动二元期权市场', 1),

-- 市场参数更新
('injective.exchange.v2.MsgSpotMarketParamUpdate', 'exchange', 'v2', 'MsgSpotMarketParamUpdate', '市场参数更新', '现货市场参数更新', 1),
('injective.exchange.v2.MsgDerivativeMarketParamUpdate', 'exchange', 'v2', 'MsgDerivativeMarketParamUpdate', '市场参数更新', '衍生品市场参数更新', 1),
('injective.exchange.v2.MsgBinaryOptionsMarketParamUpdate', 'exchange', 'v2', 'MsgBinaryOptionsMarketParamUpdate', '市场参数更新', '二元期权市场参数更新', 1),

-- 交易奖励
('injective.exchange.v2.MsgTradingRewardCampaignLaunch', 'exchange', 'v2', 'MsgTradingRewardCampaignLaunch', '交易奖励', '启动交易奖励活动', 1),
('injective.exchange.v2.MsgTradingRewardCampaignUpdate', 'exchange', 'v2', 'MsgTradingRewardCampaignUpdate', '交易奖励', '更新交易奖励活动', 1),
('injective.exchange.v2.MsgTradingRewardPendingPointsUpdate', 'exchange', 'v2', 'MsgTradingRewardPendingPointsUpdate', '交易奖励', '更新待处理奖励积分', 1),

-- 费用管理
('injective.exchange.v2.MsgFeeDiscount', 'exchange', 'v2', 'MsgFeeDiscount', '费用管理', '费用折扣', 1),
('injective.exchange.v2.MsgAtomicMarketOrderFeeMultiplierSchedule', 'exchange', 'v2', 'MsgAtomicMarketOrderFeeMultiplierSchedule', '费用管理', '原子市价单费用乘数计划', 1),

-- 其他
('injective.exchange.v2.MsgMarketForcedSettlement', 'exchange', 'v2', 'MsgMarketForcedSettlement', '其他', '市场强制结算', 1),
('injective.exchange.v2.MsgExchangeEnable', 'exchange', 'v2', 'MsgExchangeEnable', '其他', '启用交易所', 1),
('injective.exchange.v2.MsgBatchCommunityPoolSpend', 'exchange', 'v2', 'MsgBatchCommunityPoolSpend', '其他', '批量社区池支出', 1),
('injective.exchange.v2.MsgCancelPostOnlyMode', 'exchange', 'v2', 'MsgCancelPostOnlyMode', '其他', '取消仅限挂单模式', 1),
('injective.exchange.v2.MsgSetDelegationTransferReceivers', 'exchange', 'v2', 'MsgSetDelegationTransferReceivers', '其他', '设置委托转账接收者', 1),
('injective.exchange.v2.MsgUpdateParams', 'exchange', 'v2', 'MsgUpdateParams', '其他', '更新参数', 1);

-- Auction 模块
INSERT INTO transaction_types (full_type_name, module, version, message_type, category, description_zh, is_active) VALUES
('injective.auction.v1beta1.MsgBid', 'auction', 'v1beta1', 'MsgBid', '拍卖', '出价', 1),
('injective.auction.v1beta1.MsgUpdateParams', 'auction', 'v1beta1', 'MsgUpdateParams', '参数管理', '更新参数', 1);

-- ERC20 模块
INSERT INTO transaction_types (full_type_name, module, version, message_type, category, description_zh, is_active) VALUES
('injective.erc20.v1beta1.MsgCreateTokenPair', 'erc20', 'v1beta1', 'MsgCreateTokenPair', '代币对管理', '创建代币对', 1),
('injective.erc20.v1beta1.MsgDeleteTokenPair', 'erc20', 'v1beta1', 'MsgDeleteTokenPair', '代币对管理', '删除代币对', 1),
('injective.erc20.v1beta1.MsgUpdateParams', 'erc20', 'v1beta1', 'MsgUpdateParams', '参数管理', '更新参数', 1);

-- EVM 模块
INSERT INTO transaction_types (full_type_name, module, version, message_type, category, description_zh, is_active) VALUES
('injective.evm.v1.MsgEthereumTx', 'evm', 'v1', 'MsgEthereumTx', '以太坊交易', '以太坊交易', 1),
('injective.evm.v1.LegacyTx', 'evm', 'v1', 'LegacyTx', '以太坊交易类型', '传统交易', 1),
('injective.evm.v1.AccessListTx', 'evm', 'v1', 'AccessListTx', '以太坊交易类型', '访问列表交易', 1),
('injective.evm.v1.DynamicFeeTx', 'evm', 'v1', 'DynamicFeeTx', '以太坊交易类型', '动态费用交易', 1),
('injective.evm.v1.ExtensionOptionsEthereumTx', 'evm', 'v1', 'ExtensionOptionsEthereumTx', '以太坊交易类型', '以太坊扩展选项交易', 1),
('injective.evm.v1.MsgUpdateParams', 'evm', 'v1', 'MsgUpdateParams', '参数管理', '更新参数', 1);

-- Insurance 模块
INSERT INTO transaction_types (full_type_name, module, version, message_type, category, description_zh, is_active) VALUES
('injective.insurance.v1beta1.MsgCreateInsuranceFund', 'insurance', 'v1beta1', 'MsgCreateInsuranceFund', '保险基金', '创建保险基金', 1),
('injective.insurance.v1beta1.MsgUnderwrite', 'insurance', 'v1beta1', 'MsgUnderwrite', '保险', '承保', 1),
('injective.insurance.v1beta1.MsgRequestRedemption', 'insurance', 'v1beta1', 'MsgRequestRedemption', '保险', '请求赎回', 1),
('injective.insurance.v1beta1.MsgUpdateParams', 'insurance', 'v1beta1', 'MsgUpdateParams', '参数管理', '更新参数', 1);

-- OCR 模块
INSERT INTO transaction_types (full_type_name, module, version, message_type, category, description_zh, is_active) VALUES
('injective.ocr.v1beta1.MsgCreateFeed', 'ocr', 'v1beta1', 'MsgCreateFeed', '数据源管理', '创建数据源', 1),
('injective.ocr.v1beta1.MsgUpdateFeed', 'ocr', 'v1beta1', 'MsgUpdateFeed', '数据源管理', '更新数据源', 1),
('injective.ocr.v1beta1.MsgTransmit', 'ocr', 'v1beta1', 'MsgTransmit', '数据传输', '传输数据', 1),
('injective.ocr.v1beta1.MsgFundFeedRewardPool', 'ocr', 'v1beta1', 'MsgFundFeedRewardPool', '奖励池', '为数据源奖励池注资', 1),
('injective.ocr.v1beta1.MsgWithdrawFeedRewardPool', 'ocr', 'v1beta1', 'MsgWithdrawFeedRewardPool', '奖励池', '从数据源奖励池提取', 1),
('injective.ocr.v1beta1.MsgSetPayees', 'ocr', 'v1beta1', 'MsgSetPayees', '权限管理', '设置收款人', 1),
('injective.ocr.v1beta1.MsgTransferPayeeship', 'ocr', 'v1beta1', 'MsgTransferPayeeship', '权限管理', '转移收款权', 1),
('injective.ocr.v1beta1.MsgAcceptPayeeship', 'ocr', 'v1beta1', 'MsgAcceptPayeeship', '权限管理', '接受收款权', 1),
('injective.ocr.v1beta1.MsgUpdateParams', 'ocr', 'v1beta1', 'MsgUpdateParams', '参数管理', '更新参数', 1);

-- Oracle 模块
INSERT INTO transaction_types (full_type_name, module, version, message_type, category, description_zh, is_active) VALUES
('injective.oracle.v1beta1.MsgRelayProviderPrices', 'oracle', 'v1beta1', 'MsgRelayProviderPrices', '价格中继', '中继提供商价格', 1),
('injective.oracle.v1beta1.MsgRelayPriceFeedPrice', 'oracle', 'v1beta1', 'MsgRelayPriceFeedPrice', '价格中继', '中继价格源价格', 1),
('injective.oracle.v1beta1.MsgRelayBandRates', 'oracle', 'v1beta1', 'MsgRelayBandRates', '价格中继', '中继Band汇率', 1),
('injective.oracle.v1beta1.MsgRelayCoinbaseMessages', 'oracle', 'v1beta1', 'MsgRelayCoinbaseMessages', '价格中继', '中继Coinbase消息', 1),
('injective.oracle.v1beta1.MsgRelayPythPrices', 'oracle', 'v1beta1', 'MsgRelayPythPrices', '价格中继', '中继Pyth价格', 1),
('injective.oracle.v1beta1.MsgRelayStorkPrices', 'oracle', 'v1beta1', 'MsgRelayStorkPrices', '价格中继', '中继Stork价格', 1),
('injective.oracle.v1beta1.MsgRequestBandIBCRates', 'oracle', 'v1beta1', 'MsgRequestBandIBCRates', '价格请求', '请求Band IBC汇率', 1),
('injective.oracle.v1beta1.MsgUpdateParams', 'oracle', 'v1beta1', 'MsgUpdateParams', '参数管理', '更新参数', 1);

-- Permissions 模块
INSERT INTO transaction_types (full_type_name, module, version, message_type, category, description_zh, is_active) VALUES
('injective.permissions.v1beta1.MsgCreateNamespace', 'permissions', 'v1beta1', 'MsgCreateNamespace', '命名空间', '创建命名空间', 1),
('injective.permissions.v1beta1.MsgUpdateNamespace', 'permissions', 'v1beta1', 'MsgUpdateNamespace', '命名空间', '更新命名空间', 1),
('injective.permissions.v1beta1.MsgUpdateActorRoles', 'permissions', 'v1beta1', 'MsgUpdateActorRoles', '角色管理', '更新角色', 1),
('injective.permissions.v1beta1.MsgClaimVoucher', 'permissions', 'v1beta1', 'MsgClaimVoucher', '凭证', '领取凭证', 1),
('injective.permissions.v1beta1.MsgUpdateParams', 'permissions', 'v1beta1', 'MsgUpdateParams', '参数管理', '更新参数', 1);

-- TokenFactory 模块
INSERT INTO transaction_types (full_type_name, module, version, message_type, category, description_zh, is_active) VALUES
('injective.tokenfactory.v1beta1.MsgCreateDenom', 'tokenfactory', 'v1beta1', 'MsgCreateDenom', '代币管理', '创建代币', 1),
('injective.tokenfactory.v1beta1.MsgMint', 'tokenfactory', 'v1beta1', 'MsgMint', '代币管理', '铸造代币', 1),
('injective.tokenfactory.v1beta1.MsgBurn', 'tokenfactory', 'v1beta1', 'MsgBurn', '代币管理', '销毁代币', 1),
('injective.tokenfactory.v1beta1.MsgChangeAdmin', 'tokenfactory', 'v1beta1', 'MsgChangeAdmin', '权限管理', '更改管理员', 1),
('injective.tokenfactory.v1beta1.MsgSetDenomMetadata', 'tokenfactory', 'v1beta1', 'MsgSetDenomMetadata', '代币管理', '设置代币元数据', 1),
('injective.tokenfactory.v1beta1.MsgUpdateParams', 'tokenfactory', 'v1beta1', 'MsgUpdateParams', '参数管理', '更新参数', 1);

-- TxFees 模块
INSERT INTO transaction_types (full_type_name, module, version, message_type, category, description_zh, is_active) VALUES
('injective.txfees.v1beta1.MsgUpdateParams', 'txfees', 'v1beta1', 'MsgUpdateParams', '参数管理', '更新参数', 1);

-- WasmX 模块
INSERT INTO transaction_types (full_type_name, module, version, message_type, category, description_zh, is_active) VALUES
('injective.wasmx.v1.MsgExecuteContractCompat', 'wasmx', 'v1', 'MsgExecuteContractCompat', '合约执行', '执行合约(兼容模式)', 1),
('injective.wasmx.v1.MsgRegisterContract', 'wasmx', 'v1', 'MsgRegisterContract', '合约管理', '注册合约', 1),
('injective.wasmx.v1.MsgUpdateContract', 'wasmx', 'v1', 'MsgUpdateContract', '合约管理', '更新合约', 1),
('injective.wasmx.v1.MsgActivateContract', 'wasmx', 'v1', 'MsgActivateContract', '合约管理', '激活合约', 1),
('injective.wasmx.v1.MsgDeactivateContract', 'wasmx', 'v1', 'MsgDeactivateContract', '合约管理', '停用合约', 1),
('injective.wasmx.v1.MsgUpdateParams', 'wasmx', 'v1', 'MsgUpdateParams', '参数管理', '更新参数', 1);

-- ============================================
-- 数据库特定语法说明
-- ============================================

-- MySQL 特定语法:
-- 1. 将 AUTOINCREMENT 改为 AUTO_INCREMENT
-- 2. 将 BOOLEAN 改为 TINYINT(1)
-- 3. 将 IF NOT EXISTS 改为 IF NOT EXISTS (MySQL 5.7+)
-- 4. 将 COMMENT 保留（MySQL 支持）

-- PostgreSQL 特定语法:
-- 1. 将 AUTOINCREMENT 改为 SERIAL
-- 2. 将 BOOLEAN 保留（PostgreSQL 支持）
-- 3. 将 IF NOT EXISTS 保留（PostgreSQL 9.5+）
-- 4. 将 COMMENT 改为 COMMENT ON (需要单独执行)

-- SQLite3 特定语法:
-- 1. 将 AUTOINCREMENT 保留（SQLite3 支持）
-- 2. 将 BOOLEAN 改为 INTEGER (0/1)
-- 3. 将 IF NOT EXISTS 保留（SQLite3 支持）
-- 4. 将 COMMENT 移除（SQLite3 不支持）

-- ============================================
-- 查询示例
-- ============================================

-- 查询所有激活的交易类型
-- SELECT * FROM transaction_types WHERE is_active = 1;

-- 按模块查询
-- SELECT * FROM transaction_types WHERE module = 'exchange' ORDER BY version, message_type;

-- 查询特定版本
-- SELECT * FROM transaction_types WHERE module = 'exchange' AND version = 'v2';

-- 按分类查询
-- SELECT * FROM transaction_types WHERE category = '账户操作';

-- 搜索交易类型
-- SELECT * FROM transaction_types WHERE message_type LIKE '%Order%';

-- 统计各模块的交易类型数量
-- SELECT module, version, COUNT(*) as count FROM transaction_types GROUP BY module, version ORDER BY module, version;

