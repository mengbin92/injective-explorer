-- ========================================================
-- 代币列表和代币详情测试数据
-- 表: token_market_cache, contract_infos, token_transfers, account_balances
-- ========================================================

-- --------------------------------------------------------
-- 1. 插入代币市场数据 (token_market_cache)
-- 包含主流代币和INJ链原生代币
-- --------------------------------------------------------
INSERT INTO token_market_cache (
    created_at, updated_at, token_symbol, contract_address, chain_id,
    price_usd, price_change_24h, market_cap_usd, fully_diluted_market_cap,
    total_volume_24h, holders_count, total_supply, circulating_supply, max_supply,
    last_updated, data_source
) VALUES
-- INJ (原生代币)
(NOW(), NOW(), 'INJ', '0x0000000000000000000000000000000000000000', 'injective-1',
 1.25, 5.32, 1250000000.00, 2500000000.00,
 45000000.00, 125000, 2000000000000000000000000000, 1000000000000000000000000000, 2000000000000000000000000000,
 UNIX_TIMESTAMP(), 'coingecko'),

-- USDT (稳定币)
(NOW(), NOW(), 'USDT', '0xdAC17F958D2ee523a2206206994597C13D831ec7', 'injective-1',
 1.00, 0.02, 95000000000.00, 95000000000.00,
 35000000000.00, 8500000, 95000000000000000, 95000000000000000, NULL,
 UNIX_TIMESTAMP(), 'coingecko'),

-- USDC (稳定币)
(NOW(), NOW(), 'USDC', '0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48', 'injective-1',
 1.00, -0.01, 25000000000.00, 25000000000.00,
 8000000000.00, 3200000, 25000000000000000, 25000000000000000, NULL,
 UNIX_TIMESTAMP(), 'coingecko'),

-- ETH (以太坊)
(NOW(), NOW(), 'ETH', '0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2', 'injective-1',
 3250.50, -2.15, 390000000000.00, 325000000000.00,
 15000000000.00, 250000000, 120000000000000000000000000, 120000000000000000000000000, NULL,
 UNIX_TIMESTAMP(), 'coingecko'),

-- WBTC (包装比特币)
(NOW(), NOW(), 'WBTC', '0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599', 'injective-1',
 98500.00, 1.85, 16000000000.00, 16000000000.00,
 500000000.00, 65000, 160000000000000000000000, 160000000000000000000000, 21000000000000000000000000,
 UNIX_TIMESTAMP(), 'coingecko'),

-- LINK (Chainlink)
(NOW(), NOW(), 'LINK', '0x514910771AF9Ca656af840dff83E8264EcF986CA', 'injective-1',
 18.75, 7.25, 11250000000.00, 18750000000.00,
 450000000.00, 180000, 1000000000000000000000000000, 600000000000000000000000000, 1000000000000000000000000000,
 UNIX_TIMESTAMP(), 'coingecko'),

-- UNI (Uniswap)
(NOW(), NOW(), 'UNI', '0x1f9840a85d5aF5bf1D1762F925BDADdC4201F984', 'injective-1',
 9.85, -4.32, 7400000000.00, 9850000000.00,
 180000000.00, 420000, 1000000000000000000000000000, 750000000000000000000000000, 1000000000000000000000000000,
 UNIX_TIMESTAMP(), 'coingecko'),

-- AAVE
(NOW(), NOW(), 'AAVE', '0x7Fc66500c84A76Ad7e9c93437bFc5Ac33E2DDaE9', 'injective-1',
 165.20, 3.15, 2450000000.00, 2650000000.00,
 95000000.00, 125000, 16000000000000000000000000, 14800000000000000000000000, 16000000000000000000000000,
 UNIX_TIMESTAMP(), 'coingecko'),

-- COMP (Compound)
(NOW(), NOW(), 'COMP', '0xc00e94Cb662C3520282E6f5717214004A7f26888', 'injective-1',
 58.45, -1.82, 485000000.00, 584500000.00,
 25000000.00, 68000, 10000000000000000000000000, 8300000000000000000000000, 10000000000000000000000000,
 UNIX_TIMESTAMP(), 'coingecko'),

-- MKR (Maker)
(NOW(), NOW(), 'MKR', '0x9f8F72aA9304c8B593d555F12eF6589cC3A579A2', 'injective-1',
 1450.00, 0.95, 1350000000.00, 1450000000.00,
 65000000.00, 92000, 1000000000000000000000000, 930000000000000000000000, 1000000000000000000000000,
 UNIX_TIMESTAMP(), 'coingecko');

-- --------------------------------------------------------
-- 2. 插入合约信息 (contract_infos)
-- 对应代币的合约详细信息
-- --------------------------------------------------------
INSERT INTO contract_infos (
    created_at, updated_at, contract_address, name, token_name, token_symbol,
    contract_type, chain_id, verification_status, is_verified, verified_at, creator, decimals, metadata
) VALUES
-- INJ 原生代币
(NOW(), NOW(), '0x0000000000000000000000000000000000000000', 'INJ Token', 'INJ Token', 'INJ',
 'native', 'injective-1', 'verified', 1, NOW(), '0x1111111111111111111111111111111111111111', 18,
 '{"website": "https://injective.network", "description": "Injective native token"}'),

-- USDT 合约
(NOW(), NOW(), '0xdAC17F958D2ee523a2206206994597C13D831ec7', 'TetherToken', 'Tether USD', 'USDT',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x2222222222222222222222222222222222222222', 6,
 '{"website": "https://tether.to", "description": "USD Tether stablecoin"}'),

-- USDC 合约
(NOW(), NOW(), '0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48', 'FiatTokenV2', 'USD Coin', 'USDC',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x3333333333333333333333333333333333333333', 6,
 '{"website": "https://circle.com", "description": "USD Coin stablecoin"}'),

-- ETH (WETH) 合约
(NOW(), NOW(), '0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2', 'Wrapped Ether', 'Wrapped Ether', 'WETH',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x4444444444444444444444444444444444444444', 18,
 '{"website": "https://weth.io", "description": "Wrapped Ether"}'),

-- WBTC 合约
(NOW(), NOW(), '0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599', 'Wrapped BTC', 'Wrapped Bitcoin', 'WBTC',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x5555555555555555555555555555555555555555', 8,
 '{"website": "https://wbtc.network", "description": "Wrapped Bitcoin"}'),

-- LINK 合约
(NOW(), NOW(), '0x514910771AF9Ca656af840dff83E8264EcF986CA', 'LinkToken', 'ChainLink Token', 'LINK',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x6666666666666666666666666666666666666666', 18,
 '{"website": "https://chain.link", "description": "ChainLink Token"}'),

-- UNI 合约
(NOW(), NOW(), '0x1f9840a85d5aF5bf1D1762F925BDADdC4201F984', 'Uniswap', 'Uniswap', 'UNI',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x7777777777777777777777777777777777777777', 18,
 '{"website": "https://uniswap.org", "description": "Uniswap Governance Token"}'),

-- AAVE 合约
(NOW(), NOW(), '0x7Fc66500c84A76Ad7e9c93437bFc5Ac33E2DDaE9', 'AaveToken', 'Aave Token', 'AAVE',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x8888888888888888888888888888888888888888', 18,
 '{"website": "https://aave.com", "description": "Aave Governance Token"}'),

-- COMP 合约
(NOW(), NOW(), '0xc00e94Cb662C3520282E6f5717214004A7f26888', 'Compound', 'Compound', 'COMP',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x9999999999999999999999999999999999999999', 18,
 '{"website": "https://compound.finance", "description": "Compound Governance Token"}'),

-- MKR 合约
(NOW(), NOW(), '0x9f8F72aA9304c8B593d555F12eF6589cC3A579A2', 'Maker Token', 'Maker', 'MKR',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA', 18,
 '{"website": "https://makerdao.com", "description": "Maker DAO Governance Token"}');

-- --------------------------------------------------------
-- 3. 插入代币转账记录 (token_transfers)
-- 模拟最近的交易记录
-- --------------------------------------------------------
INSERT INTO token_transfers (
    created_at, updated_at, transaction_hash, evm_hash, contract_address,
    from_address, to_address, amount, method, block_number, block_timestamp, log_index, chain_id
) VALUES
-- INJ 转账记录
(NOW(), NOW(), '0xabc123def4567890123456789012345678901234567890123456789012345678', '0xdef4567890123456789012345678901234567890123456789012345678901234', '0x0000000000000000000000000000000000000000',
 '0x1111111111111111111111111111111111111111', '0x2222222222222222222222222222222222222222',
 '1000000000000000000000', 'Transfer', 15000001, UNIX_TIMESTAMP() - 3600, 0, 'injective-1'),

(NOW(), NOW(), '0xabc124def4567890123456789012345678901234567890123456789012345679', '0xdef4577890123456789012345678901234567890123456789012345678901235', '0x0000000000000000000000000000000000000000',
 '0x2222222222222222222222222222222222222222', '0x3333333333333333333333333333333333333333',
 '500000000000000000000', 'Transfer', 15000002, UNIX_TIMESTAMP() - 3500, 1, 'injective-1'),

(NOW(), NOW(), '0xabc125def4567890123456789012345678901234567890123456789012345680', '0xdef4587890123456789012345678901234567890123456789012345678901236', '0x0000000000000000000000000000000000000000',
 '0x3333333333333333333333333333333333333333', '0x4444444444444444444444444444444444444444',
 '2500000000000000000000', 'Transfer', 15000003, UNIX_TIMESTAMP() - 3400, 2, 'injective-1'),

-- USDT 转账记录
(NOW(), NOW(), '0xabc126def4567890123456789012345678901234567890123456789012345681', '0xdef4597890123456789012345678901234567890123456789012345678901237', '0xdAC17F958D2ee523a2206206994597C13D831ec7',
 '0x1111111111111111111111111111111111111111', '0x5555555555555555555555555555555555555555',
 '1000000000', 'Transfer', 15000004, UNIX_TIMESTAMP() - 3300, 0, 'injective-1'),

(NOW(), NOW(), '0xabc127def4567890123456789012345678901234567890123456789012345682', '0xdef4607890123456789012345678901234567890123456789012345678901238', '0xdAC17F958D2ee523a2206206994597C13D831ec7',
 '0x5555555555555555555555555555555555555555', '0x6666666666666666666666666666666666666666',
 '50000000000', 'Transfer', 15000005, UNIX_TIMESTAMP() - 3200, 1, 'injective-1'),

(NOW(), NOW(), '0xabc128def4567890123456789012345678901234567890123456789012345683', '0xdef4617890123456789012345678901234567890123456789012345678901239', '0xdAC17F958D2ee523a2206206994597C13D831ec7',
 '0x6666666666666666666666666666666666666666', '0x7777777777777777777777777777777777777777',
 '250000000000', 'Transfer', 15000006, UNIX_TIMESTAMP() - 3100, 2, 'injective-1'),

-- ETH (WETH) 转账记录
(NOW(), NOW(), '0xabc129def4567890123456789012345678901234567890123456789012345684', '0xdef4627890123456789012345678901234567890123456789012345678901240', '0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2',
 '0x1111111111111111111111111111111111111111', '0x8888888888888888888888888888888888888888',
 '5000000000000000000', 'Transfer', 15000007, UNIX_TIMESTAMP() - 3000, 0, 'injective-1'),

(NOW(), NOW(), '0xabc130def4567890123456789012345678901234567890123456789012345685', '0xdef4637890123456789012345678901234567890123456789012345678901241', '0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2',
 '0x8888888888888888888888888888888888888888', '0x9999999999999999999999999999999999999999',
 '10000000000000000000', 'Transfer', 15000008, UNIX_TIMESTAMP() - 2900, 1, 'injective-1'),

-- LINK 转账记录
(NOW(), NOW(), '0xabc131def4567890123456789012345678901234567890123456789012345686', '0xdef4647890123456789012345678901234567890123456789012345678901242', '0x514910771AF9Ca656af840dff83E8264EcF986CA',
 '0x2222222222222222222222222222222222222222', '0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA',
 '1000000000000000000000', 'Transfer', 15000009, UNIX_TIMESTAMP() - 2800, 0, 'injective-1'),

-- UNI 转账记录
(NOW(), NOW(), '0xabc132def4567890123456789012345678901234567890123456789012345687', '0xdef4657890123456789012345678901234567890123456789012345678901243', '0x1f9840a85d5aF5bf1D1762F925BDADdC4201F984',
 '0x3333333333333333333333333333333333333333', '0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB',
 '500000000000000000000', 'Transfer', 15000010, UNIX_TIMESTAMP() - 2700, 0, 'injective-1'),

-- AAVE 转账记录
(NOW(), NOW(), '0xabc133def4567890123456789012345678901234567890123456789012345688', '0xdef4667890123456789012345678901234567890123456789012345678901244', '0x7Fc66500c84A76Ad7e9c93437bFc5Ac33E2DDaE9',
 '0x4444444444444444444444444444444444444444', '0xCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC',
 '250000000000000000000', 'Transfer', 15000011, UNIX_TIMESTAMP() - 2600, 0, 'injective-1'),

-- 铸造记录 (Mint)
(NOW(), NOW(), '0xabc134def4567890123456789012345678901234567890123456789012345689', '0xdef4677890123456789012345678901234567890123456789012345678901245', '0xdAC17F958D2ee523a2206206994597C13D831ec7',
 '0x0000000000000000000000000000000000000000', '0xDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD',
 '1000000000000000', 'Mint', 15000012, UNIX_TIMESTAMP() - 2500, 0, 'injective-1'),

-- 销毁记录 (Burn)
(NOW(), NOW(), '0xabc135def4567890123456789012345678901234567890123456789012345690', '0xdef4687890123456789012345678901234567890123456789012345678901246', '0xdAC17F958D2ee523a2206206994597C13D831ec7',
 '0xDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD', '0x0000000000000000000000000000000000000000',
 '500000000000000', 'Burn', 15000013, UNIX_TIMESTAMP() - 2400, 0, 'injective-1');

-- --------------------------------------------------------
-- 4. 插入账户余额 (account_balances)
-- 模拟一些账户的代币持有情况
-- --------------------------------------------------------
INSERT INTO account_balances (
    created_at, updated_at, address, denom, amount
) VALUES
-- 账户1: 持有多种代币
(NOW(), NOW(), '0x1111111111111111111111111111111111111111', 'inj', '10000000000000000000000'),
(NOW(), NOW(), '0x1111111111111111111111111111111111111111', 'usdt', '500000000000'),
(NOW(), NOW(), '0x1111111111111111111111111111111111111111', 'eth', '50000000000000000000'),

-- 账户2: 持有多种代币
(NOW(), NOW(), '0x2222222222222222222222222222222222222222', 'inj', '5000000000000000000000'),
(NOW(), NOW(), '0x2222222222222222222222222222222222222222', 'usdt', '10000000000000'),
(NOW(), NOW(), '0x2222222222222222222222222222222222222222', 'link', '2000000000000000000000'),

-- 账户3: 持有多种代币
(NOW(), NOW(), '0x3333333333333333333333333333333333333333', 'inj', '25000000000000000000000'),
(NOW(), NOW(), '0x3333333333333333333333333333333333333333', 'uni', '1000000000000000000000'),

-- 账户4: 主要持有稳定币
(NOW(), NOW(), '0x5555555555555555555555555555555555555555', 'usdt', '100000000000000'),
(NOW(), NOW(), '0x5555555555555555555555555555555555555555', 'usdc', '50000000000000'),

-- 账户5: 主要持有ETH和WBTC
(NOW(), NOW(), '0x8888888888888888888888888888888888888888', 'eth', '250000000000000000000'),
(NOW(), NOW(), '0x8888888888888888888888888888888888888888', 'wbtc', '15000000000'),

-- 账户6: 主要持有治理代币
(NOW(), NOW(), '0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA', 'aave', '500000000000000000000'),
(NOW(), NOW(), '0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA', 'comp', '250000000000000000000'),
(NOW(), NOW(), '0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA', 'mkr', '10000000000000000000');

-- --------------------------------------------------------
-- 5. 插入合约源代码文件 (contract_source_files)
-- 用于测试合约代码查询接口
-- --------------------------------------------------------
INSERT INTO contract_source_files (
    created_at, updated_at, contract_address, chain_id, file_name, file_type,
    file_path, file_content, file_size, file_hash, mime_type, description, uploaded_by
) VALUES
-- INJ Token 合约源代码
(NOW(), NOW(), '0x0000000000000000000000000000000000000000', 'injective-1',
 'INJToken.sol', 'source', '',
 '// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

contract INJToken {
    string public name = "INJ Token";
    string public symbol = "INJ";
    uint8 public decimals = 18;
    uint256 public totalSupply = 2000000000 * 10**18;
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    event Transfer(address indexed from, address indexed to, uint256 value);
    event Approval(address indexed owner, address indexed spender, uint256 value);

    constructor() {
        balanceOf[msg.sender] = totalSupply;
    }

    function transfer(address to, uint256 value) public returns (bool) {
        require(balanceOf[msg.sender] >= value, "Insufficient balance");
        balanceOf[msg.sender] -= value;
        balanceOf[to] += value;
        emit Transfer(msg.sender, to, value);
        return true;
    }
}',
 1024, '0xabc123def456', 'text/x-solidity', 'INJ Token source code', '0x1111111111111111111111111111111111111111'),

-- USDT 合约源代码
(NOW(), NOW(), '0xdAC17F958D2ee523a2206206994597C13D831ec7', 'injective-1',
 'TetherToken.sol', 'source', '',
 '// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

contract TetherToken {
    string public name = "Tether USD";
    string public symbol = "USDT";
    uint8 public decimals = 6;
    uint256 public totalSupply;
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    constructor(uint256 initialSupply) {
        totalSupply = initialSupply;
        balanceOf[msg.sender] = totalSupply;
    }

    function transfer(address to, uint256 value) public returns (bool) {
        require(balanceOf[msg.sender] >= value);
        balanceOf[msg.sender] -= value;
        balanceOf[to] += value;
        return true;
    }
}',
 896, '0xdef789abc012', 'text/x-solidity', 'USDT Token source code', '0x2222222222222222222222222222222222222222'),

-- WETH 合约源代码
(NOW(), NOW(), '0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2', 'injective-1',
 'WETH9.sol', 'source', '',
 '// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

contract WETH9 {
    string public name = "Wrapped Ether";
    string public symbol = "WETH";
    uint8 public decimals = 18;
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    event Deposit(address indexed dst, uint256 wad);
    event Withdrawal(address indexed src, uint256 wad);

    receive() external payable {
        deposit();
    }

    function deposit() public payable {
        balanceOf[msg.sender] += msg.value;
        emit Deposit(msg.sender, msg.value);
    }

    function withdraw(uint256 wad) public {
        require(balanceOf[msg.sender] >= wad);
        balanceOf[msg.sender] -= wad;
        payable(msg.sender).transfer(wad);
        emit Withdrawal(msg.sender, wad);
    }
}',
 950, '0xfed654cba321', 'text/x-solidity', 'WETH9 source code', '0x4444444444444444444444444444444444444444'),

-- 合约 ABI 文件示例
(NOW(), NOW(), '0x0000000000000000000000000000000000000000', 'injective-1',
 'INJToken.abi', 'abi', '',
 '[{"inputs":[],"stateMutability":"nonpayable","type":"constructor"},{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address","name":"owner","type":"address"},{"indexed":true,"internalType":"address","name":"spender","type":"address"},{"indexed":false,"internalType":"uint256","name":"value","type":"uint256"}],"name":"Approval","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address","name":"from","type":"address"},{"indexed":true,"internalType":"address","name":"to","type":"address"},{"indexed":false,"internalType":"uint256","name":"value","type":"uint256"}],"name":"Transfer","type":"event"}]',
 450, '0xaaa111bbb222', 'application/json', 'INJ Token ABI', '0x1111111111111111111111111111111111111111');

-- --------------------------------------------------------
-- 查询验证数据
-- --------------------------------------------------------

-- 查看所有代币市场数据
-- SELECT * FROM token_market_cache WHERE deleted_at IS NULL ORDER BY market_cap_usd DESC;

-- 查看所有合约信息
-- SELECT contract_address, token_name, token_symbol, creator, contract_type FROM contract_infos WHERE deleted_at IS NULL;

-- 查看合约源代码文件
-- SELECT contract_address, file_name, file_type, file_size FROM contract_source_files WHERE deleted_at IS NULL;

-- 查看代币转账记录（最近10条）
-- SELECT tt.*, ci.token_symbol FROM token_transfers tt
-- LEFT JOIN contract_infos ci ON tt.contract_address = ci.contract_address
-- WHERE tt.deleted_at IS NULL ORDER BY tt.block_timestamp DESC LIMIT 10;

-- 查看账户余额
-- SELECT * FROM account_balances WHERE deleted_at IS NULL ORDER BY address, denom;

-- 查看所有代币市场数据
-- SELECT * FROM token_market_cache WHERE deleted_at IS NULL ORDER BY market_cap_usd DESC;

-- 查看所有合约信息
-- SELECT contract_address, token_name, token_symbol, contract_type FROM contract_infos WHERE deleted_at IS NULL;

-- 查看代币转账记录（最近10条）
-- SELECT tt.*, ci.token_symbol FROM token_transfers tt
-- LEFT JOIN contract_infos ci ON tt.contract_address = ci.contract_address
-- WHERE tt.deleted_at IS NULL ORDER BY tt.block_timestamp DESC LIMIT 10;

-- 查看账户余额
-- SELECT * FROM account_balances WHERE deleted_at IS NULL ORDER BY address, denom;
-- ================================================================
-- IP管理模块测试数据
-- 创建时间: 2026-01-29
-- 描述: 为IP管理相关功能生成测试数据，包括IP注册记录、IP黑名单、API请求日志
-- ================================================================

-- 清空现有测试数据（可选，谨慎使用）
-- TRUNCATE TABLE `ip_registration_records`;
-- TRUNCATE TABLE `ip_blacklist`;
-- TRUNCATE TABLE `api_request_logs`;

-- ================================================================
-- 1. IP注册记录测试数据 (ip_registration_records)
-- ================================================================

-- 正常注册IP（每个IP 1-2次注册）
INSERT INTO `ip_registration_records` (`ip_address`, `ip_location`, `user_id`, `email`, `created_at`) VALUES
('120.79.45.123', '中国广东深圳', 'user_001', 'zhang.san@gmail.com', DATE_SUB(NOW(), INTERVAL 1 DAY)),
('183.60.123.45', '中国上海', 'user_002', 'li.si@163.com', DATE_SUB(NOW(), INTERVAL 2 DAY)),
('58.220.78.90', '中国北京', 'user_003', 'wang.wu@qq.com', DATE_SUB(NOW(), INTERVAL 3 DAY)),
('123.125.114.144', '中国浙江杭州', 'user_004', 'zhao.liu@outlook.com', DATE_SUB(NOW(), INTERVAL 4 DAY)),
('61.135.169.125', '中国四川成都', 'user_005', 'chen.qi@yahoo.com', DATE_SUB(NOW(), INTERVAL 5 DAY));

-- 高频注册IP（同一IP多次注册，可疑行为）
INSERT INTO `ip_registration_records` (`ip_address`, `ip_location`, `user_id`, `email`, `created_at`) VALUES
-- 192.168.1.100 注册了10次
('192.168.1.100', '本地网络', 'batch_user_001', 'test001@tempmail.com', DATE_SUB(NOW(), INTERVAL 6 HOUR)),
('192.168.1.100', '本地网络', 'batch_user_002', 'test002@tempmail.com', DATE_SUB(NOW(), INTERVAL 5 HOUR)),
('192.168.1.100', '本地网络', 'batch_user_003', 'test003@tempmail.com', DATE_SUB(NOW(), INTERVAL 4 HOUR)),
('192.168.1.100', '本地网络', 'batch_user_004', 'test004@tempmail.com', DATE_SUB(NOW(), INTERVAL 3 HOUR)),
('192.168.1.100', '本地网络', 'batch_user_005', 'test005@tempmail.com', DATE_SUB(NOW(), INTERVAL 2 HOUR)),
('192.168.1.100', '本地网络', 'batch_user_006', 'test006@tempmail.com', DATE_SUB(NOW(), INTERVAL 1 HOUR)),
('192.168.1.100', '本地网络', 'batch_user_007', 'test007@tempmail.com', DATE_SUB(NOW(), INTERVAL 50 MINUTE)),
('192.168.1.100', '本地网络', 'batch_user_008', 'test008@tempmail.com', DATE_SUB(NOW(), INTERVAL 40 MINUTE)),
('192.168.1.100', '本地网络', 'batch_user_009', 'test009@tempmail.com', DATE_SUB(NOW(), INTERVAL 30 MINUTE)),
('192.168.1.100', '本地网络', 'batch_user_010', 'test010@tempmail.com', DATE_SUB(NOW(), INTERVAL 20 MINUTE)),

-- 203.0.113.50 注册了8次
('203.0.113.50', '美国加利福尼亚', 'spam_001', 'spam001@guerrillamail.com', DATE_SUB(NOW(), INTERVAL 12 HOUR)),
('203.0.113.50', '美国加利福尼亚', 'spam_002', 'spam002@guerrillamail.com', DATE_SUB(NOW(), INTERVAL 11 HOUR)),
('203.0.113.50', '美国加利福尼亚', 'spam_003', 'spam003@guerrillamail.com', DATE_SUB(NOW(), INTERVAL 10 HOUR)),
('203.0.113.50', '美国加利福尼亚', 'spam_004', 'spam004@guerrillamail.com', DATE_SUB(NOW(), INTERVAL 9 HOUR)),
('203.0.113.50', '美国加利福尼亚', 'spam_005', 'spam005@guerrillamail.com', DATE_SUB(NOW(), INTERVAL 8 HOUR)),
('203.0.113.50', '美国加利福尼亚', 'spam_006', 'spam006@guerrillamail.com', DATE_SUB(NOW(), INTERVAL 7 HOUR)),
('203.0.113.50', '美国加利福尼亚', 'spam_007', 'spam007@guerrillamail.com', DATE_SUB(NOW(), INTERVAL 6 HOUR)),
('203.0.113.50', '美国加利福尼亚', 'spam_008', 'spam008@guerrillamail.com', DATE_SUB(NOW(), INTERVAL 5 HOUR)),

-- 45.76.123.45 注册了5次
('45.76.123.45', '新加坡', 'proxy_001', 'proxy001@10minutemail.com', DATE_SUB(NOW(), INTERVAL 2 DAY)),
('45.76.123.45', '新加坡', 'proxy_002', 'proxy002@10minutemail.com', DATE_SUB(NOW(), INTERVAL 2 DAY)),
('45.76.123.45', '新加坡', 'proxy_003', 'proxy003@10minutemail.com', DATE_SUB(NOW(), INTERVAL 1 DAY)),
('45.76.123.45', '新加坡', 'proxy_004', 'proxy004@10minutemail.com', DATE_SUB(NOW(), INTERVAL 1 DAY)),
('45.76.123.45', '新加坡', 'proxy_005', 'proxy005@10minutemail.com', DATE_SUB(NOW(), INTERVAL 1 DAY)),

-- 今日新增注册
('101.89.77.66', '中国江苏南京', 'today_user_001', 'nanjing@example.com', DATE_SUB(NOW(), INTERVAL 3 HOUR)),
('112.65.22.33', '中国湖北武汉', 'today_user_002', 'wuhan@example.com', DATE_SUB(NOW(), INTERVAL 2 HOUR)),
('171.88.99.100', '中国湖南长沙', 'today_user_003', 'changsha@example.com', DATE_SUB(NOW(), INTERVAL 1 HOUR));

-- ================================================================
-- 2. IP黑名单测试数据 (ip_blacklist)
-- ================================================================

INSERT INTO `ip_blacklist` (`ip_address`, `ip_location`, `reason`, `reason_text`, `block_count`, `operator_id`, `operator_name`, `block_type`, `created_at`) VALUES
-- 批量注册类型
('192.168.1.100', '本地网络', 'batch_register', '短时间内注册10个账号，疑似批量注册行为', 45, '1', '超级管理员', 'all', DATE_SUB(NOW(), INTERVAL 1 DAY)),
('203.0.113.50', '美国加利福尼亚', 'batch_register', '使用临时邮箱批量注册账号', 32, '1', '超级管理员', 'register', DATE_SUB(NOW(), INTERVAL 2 DAY)),

-- API滥用类型
('198.51.100.23', '美国纽约', 'api_abuse', '短时间内大量API请求，超出速率限制', 156, '2', '运维管理员', 'api', DATE_SUB(NOW(), INTERVAL 3 DAY)),
('203.0.113.88', '美国德克萨斯', 'api_abuse', '恶意爬取数据，频繁触发429错误', 89, '2', '运维管理员', 'all', DATE_SUB(NOW(), INTERVAL 5 DAY)),

-- 代理IP类型
('45.76.123.45', '新加坡', 'proxy', '已知的VPN/代理服务器IP', 12, '1', '超级管理员', 'register', DATE_SUB(NOW(), INTERVAL 7 DAY)),
('104.28.10.50', '美国', 'proxy', 'Cloudflare代理节点，被滥用', 8, '3', '安全管理员', 'all', DATE_SUB(NOW(), INTERVAL 10 DAY)),

-- 垃圾信息类型
('185.220.101.10', '荷兰', 'spam', 'Tor出口节点，发送垃圾信息', 234, '3', '安全管理员', 'all', DATE_SUB(NOW(), INTERVAL 15 DAY)),
('91.195.240.120', '俄罗斯', 'spam', '发送恶意请求和垃圾内容', 67, '1', '超级管理员', 'all', DATE_SUB(NOW(), INTERVAL 20 DAY)),

-- 其他原因
('172.16.0.50', '本地网络', 'other', '测试黑名单功能', 5, '1', '超级管理员', 'all', DATE_SUB(NOW(), INTERVAL 30 DAY)),

-- 今日新增黑名单
('211.147.6.78', '中国广东广州', 'batch_register', '今日发现批量注册行为', 3, '1', '超级管理员', 'register', DATE_SUB(NOW(), INTERVAL 2 HOUR));

-- ================================================================
-- 3. API请求日志测试数据 (api_request_logs)
-- ================================================================

-- 假设有一些测试token_id和user_id
SET @test_token_id_1 = 1;
SET @test_token_id_2 = 2;
SET @test_user_id_1 = 'user_001';
SET @test_user_id_2 = 'user_002';

-- 3.1 正常请求（状态码 200）
INSERT INTO `api_request_logs` (`token_id`, `user_id`, `ip_address`, `ip_location`, `endpoint`, `method`, `status_code`, `response_time`, `error_message`, `created_at`) VALUES
-- 最近1小时的请求
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/blocks/latest', 'GET', 200, 45, NULL, DATE_SUB(NOW(), INTERVAL 10 MINUTE)),
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/transactions/0x123abc', 'GET', 200, 67, NULL, DATE_SUB(NOW(), INTERVAL 15 MINUTE)),
(@test_token_id_2, @test_user_id_2, '183.60.123.45', '中国上海', '/api/v1/addresses/0xabc123', 'GET', 200, 52, NULL, DATE_SUB(NOW(), INTERVAL 20 MINUTE)),
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/blocks/12345', 'GET', 200, 38, NULL, DATE_SUB(NOW(), INTERVAL 30 MINUTE)),
(@test_token_id_2, @test_user_id_2, '183.60.123.45', '中国上海', '/api/v1/stats/summary', 'GET', 200, 89, NULL, DATE_SUB(NOW(), INTERVAL 45 MINUTE)),

-- 最近6小时的请求
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/tokens/list', 'GET', 200, 123, NULL, DATE_SUB(NOW(), INTERVAL 2 HOUR)),
(@test_token_id_2, @test_user_id_2, '183.60.123.45', '中国上海', '/api/v1/contracts/verify', 'POST', 200, 456, NULL, DATE_SUB(NOW(), INTERVAL 3 HOUR)),
(@test_token_id_1, @test_user_id_1, '58.220.78.90', '中国北京', '/api/v1/blocks/range', 'GET', 200, 234, NULL, DATE_SUB(NOW(), INTERVAL 4 HOUR)),
(@test_token_id_2, @test_user_id_2, '123.125.114.144', '中国浙江杭州', '/api/v1/search', 'GET', 200, 178, NULL, DATE_SUB(NOW(), INTERVAL 5 HOUR)),

-- 最近24小时的请求
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/blocks/latest', 'GET', 200, 56, NULL, DATE_SUB(NOW(), INTERVAL 8 HOUR)),
(@test_token_id_2, @test_user_id_2, '183.60.123.45', '中国上海', '/api/v1/transactions/pending', 'GET', 200, 92, NULL, DATE_SUB(NOW(), INTERVAL 12 HOUR)),
(@test_token_id_1, @test_user_id_1, '61.135.169.125', '中国四川成都', '/api/v1/validators/list', 'GET', 200, 145, NULL, DATE_SUB(NOW(), INTERVAL 18 HOUR)),
(@test_token_id_2, @test_user_id_2, '120.79.45.123', '中国广东深圳', '/api/v1/stats/daily', 'GET', 200, 201, NULL, DATE_SUB(NOW(), INTERVAL 22 HOUR));

-- 3.2 错误请求（400系列 - 客户端错误）
INSERT INTO `api_request_logs` (`token_id`, `user_id`, `ip_address`, `ip_location`, `endpoint`, `method`, `status_code`, `response_time`, `error_message`, `created_at`) VALUES
-- 400 Bad Request
(NULL, NULL, '203.0.113.50', '美国加利福尼亚', '/api/v1/blocks/invalid', 'GET', 400, 12, 'Invalid block number format', DATE_SUB(NOW(), INTERVAL 1 HOUR)),
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/transactions/create', 'POST', 400, 23, 'Missing required field: to_address', DATE_SUB(NOW(), INTERVAL 3 HOUR)),
(NULL, NULL, '45.76.123.45', '新加坡', '/api/v1/addresses/0xinvalid', 'GET', 400, 15, 'Invalid address format', DATE_SUB(NOW(), INTERVAL 5 HOUR)),

-- 401 Unauthorized
(NULL, NULL, '198.51.100.23', '美国纽约', '/api/v1/admin/users', 'GET', 401, 8, 'Missing or invalid API token', DATE_SUB(NOW(), INTERVAL 2 HOUR)),
(NULL, NULL, '203.0.113.88', '美国德克萨斯', '/api/v1/tokens/create', 'POST', 401, 10, 'Authentication required', DATE_SUB(NOW(), INTERVAL 4 HOUR)),
(NULL, NULL, '192.168.1.100', '本地网络', '/api/v1/admin/plans', 'GET', 401, 7, 'Invalid token', DATE_SUB(NOW(), INTERVAL 6 HOUR)),

-- 403 Forbidden
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/admin/settings', 'PUT', 403, 18, 'Insufficient permissions', DATE_SUB(NOW(), INTERVAL 7 HOUR)),
(@test_token_id_2, @test_user_id_2, '183.60.123.45', '中国上海', '/api/v1/admin/ip/blacklist', 'POST', 403, 14, 'Admin access required', DATE_SUB(NOW(), INTERVAL 9 HOUR)),

-- 404 Not Found
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/blocks/999999999', 'GET', 404, 25, 'Block not found', DATE_SUB(NOW(), INTERVAL 30 MINUTE)),
(@test_token_id_2, @test_user_id_2, '183.60.123.45', '中国上海', '/api/v1/transactions/0xnonexistent', 'GET', 404, 31, 'Transaction not found', DATE_SUB(NOW(), INTERVAL 1 HOUR)),
(@test_token_id_1, @test_user_id_1, '58.220.78.90', '中国北京', '/api/v1/addresses/0xnotfound', 'GET', 404, 28, 'Address not found', DATE_SUB(NOW(), INTERVAL 2 HOUR)),

-- 429 Too Many Requests (速率限制)
(@test_token_id_1, @test_user_id_1, '198.51.100.23', '美国纽约', '/api/v1/blocks/latest', 'GET', 429, 5, 'Rate limit exceeded: 100 requests per minute', DATE_SUB(NOW(), INTERVAL 15 MINUTE)),
(@test_token_id_1, @test_user_id_1, '198.51.100.23', '美国纽约', '/api/v1/blocks/latest', 'GET', 429, 4, 'Rate limit exceeded: 100 requests per minute', DATE_SUB(NOW(), INTERVAL 14 MINUTE)),
(@test_token_id_1, @test_user_id_1, '198.51.100.23', '美国纽约', '/api/v1/blocks/latest', 'GET', 429, 6, 'Rate limit exceeded: 100 requests per minute', DATE_SUB(NOW(), INTERVAL 13 MINUTE)),
(@test_token_id_1, @test_user_id_1, '198.51.100.23', '美国纽约', '/api/v1/transactions/list', 'GET', 429, 5, 'Rate limit exceeded: 100 requests per minute', DATE_SUB(NOW(), INTERVAL 12 MINUTE)),
(@test_token_id_2, @test_user_id_2, '203.0.113.88', '美国德克萨斯', '/api/v1/search', 'GET', 429, 7, 'Rate limit exceeded: 100 requests per minute', DATE_SUB(NOW(), INTERVAL 10 MINUTE)),
(@test_token_id_2, @test_user_id_2, '203.0.113.88', '美国德克萨斯', '/api/v1/search', 'GET', 429, 6, 'Rate limit exceeded: 100 requests per minute', DATE_SUB(NOW(), INTERVAL 9 MINUTE));

-- 3.3 服务器错误（500系列）
INSERT INTO `api_request_logs` (`token_id`, `user_id`, `ip_address`, `ip_location`, `endpoint`, `method`, `status_code`, `response_time`, `error_message`, `created_at`) VALUES
-- 500 Internal Server Error
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/blocks/sync', 'POST', 500, 1234, 'Database connection timeout', DATE_SUB(NOW(), INTERVAL 8 HOUR)),
(@test_token_id_2, @test_user_id_2, '183.60.123.45', '中国上海', '/api/v1/contracts/compile', 'POST', 500, 2345, 'Internal server error', DATE_SUB(NOW(), INTERVAL 12 HOUR)),

-- 502 Bad Gateway
(NULL, NULL, '45.76.123.45', '新加坡', '/api/v1/blocks/latest', 'GET', 502, 30000, 'Upstream server timeout', DATE_SUB(NOW(), INTERVAL 1 DAY)),

-- 503 Service Unavailable
(NULL, NULL, '104.28.10.50', '美国', '/api/v1/stats/network', 'GET', 503, 15000, 'Service temporarily unavailable', DATE_SUB(NOW(), INTERVAL 2 DAY));

-- 3.4 高频请求IP的日志（用于监控异常检测）
INSERT INTO `api_request_logs` (`token_id`, `user_id`, `ip_address`, `ip_location`, `endpoint`, `method`, `status_code`, `response_time`, `error_message`, `created_at`) VALUES
-- 198.51.100.23 - API滥用IP，大量请求
(@test_token_id_1, @test_user_id_1, '198.51.100.23', '美国纽约', '/api/v1/blocks/latest', 'GET', 200, 45, NULL, DATE_SUB(NOW(), INTERVAL 5 MINUTE)),
(@test_token_id_1, @test_user_id_1, '198.51.100.23', '美国纽约', '/api/v1/blocks/latest', 'GET', 200, 48, NULL, DATE_SUB(NOW(), INTERVAL 4 MINUTE)),
(@test_token_id_1, @test_user_id_1, '198.51.100.23', '美国纽约', '/api/v1/blocks/latest', 'GET', 200, 52, NULL, DATE_SUB(NOW(), INTERVAL 3 MINUTE)),
(@test_token_id_1, @test_user_id_1, '198.51.100.23', '美国纽约', '/api/v1/transactions/list', 'GET', 200, 67, NULL, DATE_SUB(NOW(), INTERVAL 2 MINUTE)),
(@test_token_id_1, @test_user_id_1, '198.51.100.23', '美国纽约', '/api/v1/transactions/list', 'GET', 200, 71, NULL, DATE_SUB(NOW(), INTERVAL 1 MINUTE)),
(@test_token_id_1, @test_user_id_1, '198.51.100.23', '美国纽约', '/api/v1/search', 'GET', 429, 12, 'Rate limit exceeded', NOW());

-- 3.5 近7天的历史数据（用于趋势分析）
INSERT INTO `api_request_logs` (`token_id`, `user_id`, `ip_address`, `ip_location`, `endpoint`, `method`, `status_code`, `response_time`, `error_message`, `created_at`) VALUES
-- 7天前
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/blocks/latest', 'GET', 200, 56, NULL, DATE_SUB(NOW(), INTERVAL 7 DAY)),
(@test_token_id_2, @test_user_id_2, '183.60.123.45', '中国上海', '/api/v1/transactions/list', 'GET', 200, 78, NULL, DATE_SUB(NOW(), INTERVAL 7 DAY)),
(NULL, NULL, '203.0.113.50', '美国加利福尼亚', '/api/v1/blocks/100', 'GET', 404, 23, 'Block not found', DATE_SUB(NOW(), INTERVAL 7 DAY)),

-- 6天前
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/blocks/latest', 'GET', 200, 62, NULL, DATE_SUB(NOW(), INTERVAL 6 DAY)),
(@test_token_id_2, @test_user_id_2, '183.60.123.45', '中国上海', '/api/v1/stats/summary', 'GET', 200, 89, NULL, DATE_SUB(NOW(), INTERVAL 6 DAY)),
(@test_token_id_1, @test_user_id_1, '58.220.78.90', '中国北京', '/api/v1/search', 'GET', 400, 15, 'Invalid query', DATE_SUB(NOW(), INTERVAL 6 DAY)),

-- 5天前
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/blocks/latest', 'GET', 200, 54, NULL, DATE_SUB(NOW(), INTERVAL 5 DAY)),
(@test_token_id_2, @test_user_id_2, '183.60.123.45', '中国上海', '/api/v1/transactions/pending', 'GET', 200, 91, NULL, DATE_SUB(NOW(), INTERVAL 5 DAY)),
(NULL, NULL, '198.51.100.23', '美国纽约', '/api/v1/admin/users', 'GET', 401, 8, 'Unauthorized', DATE_SUB(NOW(), INTERVAL 5 DAY)),

-- 4天前
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/blocks/latest', 'GET', 200, 48, NULL, DATE_SUB(NOW(), INTERVAL 4 DAY)),
(@test_token_id_2, @test_user_id_2, '183.60.123.45', '中国上海', '/api/v1/validators/list', 'GET', 200, 134, NULL, DATE_SUB(NOW(), INTERVAL 4 DAY)),
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/blocks/999999', 'GET', 404, 28, 'Block not found', DATE_SUB(NOW(), INTERVAL 4 DAY)),

-- 3天前
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/blocks/latest', 'GET', 200, 51, NULL, DATE_SUB(NOW(), INTERVAL 3 DAY)),
(@test_token_id_2, @test_user_id_2, '183.60.123.45', '中国上海', '/api/v1/stats/daily', 'GET', 200, 156, NULL, DATE_SUB(NOW(), INTERVAL 3 DAY)),
(@test_token_id_1, @test_user_id_1, '58.220.78.90', '中国北京', '/api/v1/contracts/verify', 'POST', 500, 1245, 'Database error', DATE_SUB(NOW(), INTERVAL 3 DAY)),

-- 2天前
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/blocks/latest', 'GET', 200, 59, NULL, DATE_SUB(NOW(), INTERVAL 2 DAY)),
(@test_token_id_2, @test_user_id_2, '183.60.123.45', '中国上海', '/api/v1/tokens/list', 'GET', 200, 112, NULL, DATE_SUB(NOW(), INTERVAL 2 DAY)),
(NULL, NULL, '192.168.1.100', '本地网络', '/api/v1/admin/plans', 'GET', 401, 9, 'Invalid token', DATE_SUB(NOW(), INTERVAL 2 DAY)),

-- 1天前
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/blocks/latest', 'GET', 200, 47, NULL, DATE_SUB(NOW(), INTERVAL 1 DAY)),
(@test_token_id_2, @test_user_id_2, '183.60.123.45', '中国上海', '/api/v1/transactions/0xabc123', 'GET', 200, 68, NULL, DATE_SUB(NOW(), INTERVAL 1 DAY)),
(@test_token_id_1, @test_user_id_1, '120.79.45.123', '中国广东深圳', '/api/v1/search', 'GET', 429, 6, 'Rate limit exceeded', DATE_SUB(NOW(), INTERVAL 1 DAY));

-- ================================================================
-- 4. 验证数据插入结果
-- ================================================================

-- 查看IP注册记录统计
SELECT
    '总IP注册记录数' AS metric,
    COUNT(*) AS count
FROM ip_registration_records
UNION ALL
SELECT
    '今日新增注册',
    COUNT(*)
FROM ip_registration_records
WHERE DATE(created_at) = CURDATE()
UNION ALL
SELECT
    '高频注册IP数（>=5次）',
    COUNT(*)
FROM (
    SELECT ip_address, COUNT(*) as cnt
    FROM ip_registration_records
    GROUP BY ip_address
    HAVING cnt >= 5
) AS high_freq;

-- 查看IP黑名单统计
SELECT
    '总黑名单IP数' AS metric,
    COUNT(*) AS count
FROM ip_blacklist
WHERE deleted_at IS NULL
UNION ALL
SELECT
    '今日新增黑名单',
    COUNT(*)
FROM ip_blacklist
WHERE DATE(created_at) = CURDATE() AND deleted_at IS NULL
UNION ALL
SELECT
    CONCAT('原因：', reason),
    COUNT(*)
FROM ip_blacklist
WHERE deleted_at IS NULL
GROUP BY reason;

-- 查看API请求日志统计
SELECT
    '总请求日志数' AS metric,
    COUNT(*) AS count
FROM api_request_logs
UNION ALL
SELECT
    '最近1小时请求',
    COUNT(*)
FROM api_request_logs
WHERE created_at >= DATE_SUB(NOW(), INTERVAL 1 HOUR)
UNION ALL
SELECT
    '最近24小时请求',
    COUNT(*)
FROM api_request_logs
WHERE created_at >= DATE_SUB(NOW(), INTERVAL 24 HOUR)
UNION ALL
SELECT
    '最近7天请求',
    COUNT(*)
FROM api_request_logs
WHERE created_at >= DATE_SUB(NOW(), INTERVAL 7 DAY)
UNION ALL
SELECT
    '成功请求（200）',
    COUNT(*)
FROM api_request_logs
WHERE status_code = 200
UNION ALL
SELECT
    '客户端错误（4xx）',
    COUNT(*)
FROM api_request_logs
WHERE status_code >= 400 AND status_code < 500
UNION ALL
SELECT
    '服务器错误（5xx）',
    COUNT(*)
FROM api_request_logs
WHERE status_code >= 500;

-- 查看按状态码分布
SELECT
    status_code,
    COUNT(*) AS count,
    ROUND(COUNT(*) * 100.0 / (SELECT COUNT(*) FROM api_request_logs), 2) AS percentage
FROM api_request_logs
GROUP BY status_code
ORDER BY count DESC;

-- 查看Top 10 请求IP
SELECT
    ip_address,
    ip_location,
    COUNT(*) AS request_count,
    COUNT(CASE WHEN status_code = 429 THEN 1 END) AS rate_limited_count
FROM api_request_logs
WHERE created_at >= DATE_SUB(NOW(), INTERVAL 24 HOUR)
GROUP BY ip_address, ip_location
ORDER BY request_count DESC
LIMIT 10;

-- ================================================================
-- 完成
-- ================================================================
-- 测试数据插入完成！
--
-- 数据概览:
-- - IP注册记录: ~30条，包含正常注册和高频注册
-- - IP黑名单: 10条，包含各种封禁原因
-- - API请求日志: ~80条，覆盖最近7天，包含各种状态码
--
-- 接下来可以测试以下接口:
-- 1. GET /api/v1/admin/ip/records - IP注册记录列表
-- 2. GET /api/v1/admin/ip/records/stats - IP注册统计
-- 3. GET /api/v1/admin/ip/blacklist - IP黑名单列表
-- 4. GET /api/v1/admin/ip/blacklist/stats - IP黑名单统计
-- 5. GET /api/v1/admin/ip/monitor - IP请求监控
-- 6. GET /api/v1/admin/usage/errors - 错误请求列表
-- ================================================================
