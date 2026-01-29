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
    contract_type, chain_id, verification_status, is_verified, verified_at, creator, metadata
) VALUES
-- INJ 原生代币
(NOW(), NOW(), '0x0000000000000000000000000000000000000000', 'INJ Token', 'INJ Token', 'INJ',
 'native', 'injective-1', 'verified', 1, NOW(), '0x1111111111111111111111111111111111111111',
 '{"decimals": 18, "website": "https://injective.network", "description": "Injective native token"}'),

-- USDT 合约
(NOW(), NOW(), '0xdAC17F958D2ee523a2206206994597C13D831ec7', 'TetherToken', 'Tether USD', 'USDT',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x2222222222222222222222222222222222222222',
 '{"decimals": 6, "website": "https://tether.to", "description": "USD Tether stablecoin"}'),

-- USDC 合约
(NOW(), NOW(), '0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48', 'FiatTokenV2', 'USD Coin', 'USDC',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x3333333333333333333333333333333333333333',
 '{"decimals": 6, "website": "https://circle.com", "description": "USD Coin stablecoin"}'),

-- ETH (WETH) 合约
(NOW(), NOW(), '0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2', 'Wrapped Ether', 'Wrapped Ether', 'WETH',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x4444444444444444444444444444444444444444',
 '{"decimals": 18, "website": "https://weth.io", "description": "Wrapped Ether"}'),

-- WBTC 合约
(NOW(), NOW(), '0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599', 'Wrapped BTC', 'Wrapped Bitcoin', 'WBTC',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x5555555555555555555555555555555555555555',
 '{"decimals": 8, "website": "https://wbtc.network", "description": "Wrapped Bitcoin"}'),

-- LINK 合约
(NOW(), NOW(), '0x514910771AF9Ca656af840dff83E8264EcF986CA', 'LinkToken', 'ChainLink Token', 'LINK',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x6666666666666666666666666666666666666666',
 '{"decimals": 18, "website": "https://chain.link", "description": "ChainLink Token"}'),

-- UNI 合约
(NOW(), NOW(), '0x1f9840a85d5aF5bf1D1762F925BDADdC4201F984', 'Uniswap', 'Uniswap', 'UNI',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x7777777777777777777777777777777777777777',
 '{"decimals": 18, "website": "https://uniswap.org", "description": "Uniswap Governance Token"}'),

-- AAVE 合约
(NOW(), NOW(), '0x7Fc66500c84A76Ad7e9c93437bFc5Ac33E2DDaE9', 'AaveToken', 'Aave Token', 'AAVE',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x8888888888888888888888888888888888888888',
 '{"decimals": 18, "website": "https://aave.com", "description": "Aave Governance Token"}'),

-- COMP 合约
(NOW(), NOW(), '0xc00e94Cb662C3520282E6f5717214004A7f26888', 'Compound', 'Compound', 'COMP',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0x9999999999999999999999999999999999999999',
 '{"decimals": 18, "website": "https://compound.finance", "description": "Compound Governance Token"}'),

-- MKR 合约
(NOW(), NOW(), '0x9f8F72aA9304c8B593d555F12eF6589cC3A579A2', 'Maker Token', 'Maker', 'MKR',
 'erc20', 'injective-1', 'verified', 1, NOW(), '0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA',
 '{"decimals": 18, "website": "https://makerdao.com", "description": "Maker DAO Governance Token"}');

-- --------------------------------------------------------
-- 3. 插入代币转账记录 (token_transfers)
-- 模拟最近的交易记录
-- --------------------------------------------------------
INSERT INTO token_transfers (
    created_at, updated_at, transaction_hash, evm_hash, contract_address,
    from_address, to_address, amount, method, block_number, block_timestamp, log_index, chain_id
) VALUES
-- INJ 转账记录
(NOW(), NOW(), '0xabc123...', '0xdef456...', '0x0000000000000000000000000000000000000000',
 '0x1111111111111111111111111111111111111111', '0x2222222222222222222222222222222222222222',
 '1000000000000000000000', 'Transfer', 15000001, UNIX_TIMESTAMP() - 3600, 0, 'injective-1'),

(NOW(), NOW(), '0xabc124...', '0xdef457...', '0x0000000000000000000000000000000000000000',
 '0x2222222222222222222222222222222222222222', '0x3333333333333333333333333333333333333333',
 '500000000000000000000', 'Transfer', 15000002, UNIX_TIMESTAMP() - 3500, 0, 'injective-1'),

(NOW(), NOW(), '0xabc125...', '0xdef458...', '0x0000000000000000000000000000000000000000',
 '0x3333333333333333333333333333333333333333', '0x4444444444444444444444444444444444444444',
 '2500000000000000000000', 'Transfer', 15000003, UNIX_TIMESTAMP() - 3400, 0, 'injective-1'),

-- USDT 转账记录
(NOW(), NOW(), '0xabc126...', '0xdef459...', '0xdAC17F958D2ee523a2206206994597C13D831ec7',
 '0x1111111111111111111111111111111111111111', '0x5555555555555555555555555555555555555555',
 '1000000000', 'Transfer', 15000004, UNIX_TIMESTAMP() - 3300, 1, 'injective-1'),

(NOW(), NOW(), '0xabc127...', '0xdef460...', '0xdAC17F958D2ee523a2206206994597C13D831ec7',
 '0x5555555555555555555555555555555555555555', '0x6666666666666666666666666666666666666666',
 '50000000000', 'Transfer', 15000005, UNIX_TIMESTAMP() - 3200, 0, 'injective-1'),

(NOW(), NOW(), '0xabc128...', '0xdef461...', '0xdAC17F958D2ee523a2206206994597C13D831ec7',
 '0x6666666666666666666666666666666666666666', '0x7777777777777777777777777777777777777777',
 '250000000000', 'Transfer', 15000006, UNIX_TIMESTAMP() - 3100, 2, 'injective-1'),

-- ETH (WETH) 转账记录
(NOW(), NOW(), '0xabc129...', '0xdef462...', '0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2',
 '0x1111111111111111111111111111111111111111', '0x8888888888888888888888888888888888888888',
 '5000000000000000000', 'Transfer', 15000007, UNIX_TIMESTAMP() - 3000, 0, 'injective-1'),

(NOW(), NOW(), '0xabc130...', '0xdef463...', '0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2',
 '0x8888888888888888888888888888888888888888', '0x9999999999999999999999999999999999999999',
 '10000000000000000000', 'Transfer', 15000008, UNIX_TIMESTAMP() - 2900, 1, 'injective-1'),

-- LINK 转账记录
(NOW(), NOW(), '0xabc131...', '0xdef464...', '0x514910771AF9Ca656af840dff83E8264EcF986CA',
 '0x2222222222222222222222222222222222222222', '0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA',
 '1000000000000000000000', 'Transfer', 15000009, UNIX_TIMESTAMP() - 2800, 0, 'injective-1'),

-- UNI 转账记录
(NOW(), NOW(), '0xabc132...', '0xdef465...', '0x1f9840a85d5aF5bf1D1762F925BDADdC4201F984',
 '0x3333333333333333333333333333333333333333', '0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB',
 '500000000000000000000', 'Transfer', 15000010, UNIX_TIMESTAMP() - 2700, 0, 'injective-1'),

-- AAVE 转账记录
(NOW(), NOW(), '0xabc133...', '0xdef466...', '0x7Fc66500c84A76Ad7e9c93437bFc5Ac33E2DDaE9',
 '0x4444444444444444444444444444444444444444', '0xCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC',
 '250000000000000000000', 'Transfer', 15000011, UNIX_TIMESTAMP() - 2600, 0, 'injective-1'),

-- 铸造记录 (Mint)
(NOW(), NOW(), '0xabc134...', '0xdef467...', '0xdAC17F958D2ee523a2206206994597C13D831ec7',
 '0x0000000000000000000000000000000000000000', '0xDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD',
 '1000000000000000', 'Mint', 15000012, UNIX_TIMESTAMP() - 2500, 0, 'injective-1'),

-- 销毁记录 (Burn)
(NOW(), NOW(), '0xabc135...', '0xdef468...', '0xdAC17F958D2ee523a2206206994597C13D831ec7',
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
