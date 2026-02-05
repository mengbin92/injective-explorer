# Postman Collections for Injective Explorer

This directory contains Postman collections for testing the Injective Explorer API.

## Collections

### 1. Complete API Collection (`Injective_Explorer_Complete_API.postman_collection.json`)
**Recommended for comprehensive testing**

This collection includes test cases for **ALL** HTTP endpoints in the Injective Explorer API:

- **Health Check**: Health check endpoint
- **Block APIs**: Latest blocks, block by height, latest height, gas utilization
- **Transaction APIs**: Latest transactions, transaction by hash, stats, failed transactions
- **Cross-Chain Transaction APIs**: IBC transfers, Peggy deposits/withdrawals
- **Account APIs**: Account info, transactions, balances
- **EVM APIs**: EVM transactions, EVM account transactions
- **Token Price APIs**: Token price and market cap
- **Token List APIs**: Token list with market data and filters
- **Token Detail APIs**: Token details, transfers, holders, holder stats, contract code
- **Contract Info APIs**: Create, read, update, delete contract info, tags management
- **Address Label APIs**: Address label CRUD, batch import/export
- **Address Remark APIs**: Address remark management
- **Tag Definition APIs**: Tag definitions management
- **API Token Management**: Create, list, delete tokens, usage statistics
- **API Token Stats APIs**: Active token count, error code analysis
- **Batch Query APIs**: Batch query transactions and blocks
- **Validator Stats APIs**: Validator block statistics
- **Global Tag APIs**: Global tag management
- **Streaming APIs**: Stream blocks, transactions, account updates (note: may not work in Postman)
- **Rate Limit & Auth Tests**: Rate limiting and authentication test cases

### 2. API Management Collection (`API_Management.postman_collection.json`)
**Focused on API token management**

This collection focuses on API token management and basic API access testing.

## Setup

1. Import the collection file into Postman:
   - For comprehensive testing: `Injective_Explorer_Complete_API.postman_collection.json`
   - For token management only: `API_Management.postman_collection.json`

2. Set up environment variables (or use the default values):
   - `base_url`: API server URL (default: `http://localhost:8000`)
   - `user_id`: User ID for token management (default: `test_user_123`)
   - `api_token`: API token (will be set automatically after creating a token)
   - `token_id`: Token ID (will be set automatically after creating a token)
   - `tx_hash`: Transaction hash for testing (optional)
   - `evm_tx_hash`: EVM transaction hash for testing (optional)
   - `account_address`: Account address for testing (e.g., `inj1...` or `0x...`)
   - `block_height`: Block height for testing (optional)
   - `contract_address`: Contract address for testing (optional)
   - `chain_id`: Chain ID (default: `injective-1`)
   - `token_symbol`: Token symbol (default: `inj`)
   - `validator_address`: Validator operator address for testing (optional)
   - `webhook_id`: Webhook ID (will be set automatically after creating a webhook)
   - `plan_id`: Plan ID for admin testing (default: `1`)

## Complete API Collection Structure

### 1. Health Check
- **Check Health**: Health check endpoint for gRPC-Web
  - Method: `GET /api/v1/health`
  - Optional query: `service` (service name to check)

### 2. Block APIs
- **Get Latest Blocks**: Get latest blocks with pagination
  - Method: `GET /api/v1/block/latest`
  - Query params: `page`, `page_size`, `cursor`
- **Get Block By Height**: Get block information by block height
  - Method: `GET /api/v1/block/by-height`
  - Query params: `height`
- **Get Latest Block Height**: Get the latest block height
  - Method: `GET /api/v1/block/latest-height`
- **Get Block Gas Utilization**: Get gas utilization for latest 10 blocks
  - Method: `GET /api/v1/block/gas-utilization`

### 3. Transaction APIs
- **Get Latest Transactions**: Get latest transactions with pagination
  - Method: `GET /api/v1/transaction/latest`
  - Query params: `page`, `page_size`, `cursor`
- **Get Transaction By Hash**: Get transaction details by hash
  - Method: `GET /api/v1/transaction/by-hash`
  - Query params: `hash`
- **Get Transaction Stats**: Get transaction volume statistics
  - Method: `GET /api/v1/transaction/stats`
- **Get Failed Transactions (24h)**: Get failed transactions in last 24 hours
  - Method: `GET /api/v1/transaction/failed-24h`
  - Query params: `pagination.page`, `pagination.page_size`, `pagination.cursor`
- **Get Failed Transactions (Latest 1000)**: Get failure statistics for latest 1000 transactions
  - Method: `GET /api/v1/transaction/failed-1000`

### 4. Cross-Chain Transaction APIs
- **Get IBC Transfer Transactions**: Get IBC cross-chain transfer transactions
  - Method: `GET /api/v1/ibc/transfers`
  - Query params: `sender`, `receiver`, `src_channel`, `src_port`, `dest_channel`, `dest_port`, `pagination.*`
  - Description: Query IBC transfer transactions with optional filters for sender, receiver, source/destination channels and ports
- **Get Peggy Deposit Transactions**: Get Peggy deposit transactions (Ethereum -> Injective)
  - Method: `GET /api/v1/peggy/deposits`
  - Query params: `sender`, `receiver`, `pagination.*`
  - Description: Query Peggy deposit transactions from Ethereum to Injective
- **Get Peggy Withdrawal Transactions**: Get Peggy withdrawal transactions (Injective -> Ethereum)
  - Method: `GET /api/v1/peggy/withdrawals`
  - Query params: `sender`, `receiver`, `pagination.*`
  - Description: Query Peggy withdrawal transactions from Injective to Ethereum

### 5. Account APIs
- **Get Account Info**: Get account information
  - Method: `GET /api/v1/account/info`
  - Query params: `address`
- **Get Account Transactions**: Get account transactions with optional type filtering
  - Method: `GET /api/v1/account/transactions`
  - Query params: `address`, `pagination.page`, `pagination.page_size`, `tx_types`
- **Get Account Balances**: Get account balances
  - Method: `GET /api/v1/account/balances`
  - Query params: `address`

### 6. EVM APIs
- **Get EVM Transaction**: Get EVM transaction by hash
  - Method: `GET /api/v1/evm/transaction`
  - Query params: `hash`
- **Get EVM Transactions**: Get EVM transactions list
  - Method: `GET /api/v1/evm/transactions`
  - Query params: `page`, `pageSize`, `cursor`
- **Get EVM Account Transactions**: Get EVM account transactions
  - Method: `GET /api/v1/evm/account/transactions`
  - Query params: `address`, `page`, `pageSize`, `cursor`
  - Supports both `0x` and `inj1` address formats

### 7. Token Price APIs
- **Get Token Price and Market Cap**: Get token price and market cap
  - Method: `GET /api/v1/token/price-marketcap`
  - Query params: `token_symbol` (optional, uses default if not specified)

### 8. Token List APIs
- **List Tokens**: Get paginated list of tokens with market data
  - Method: `GET /api/v1/tokens/list`
  - Query params: `chain_id`, `contract_type`, `search`, `sort_by`, `sort_order`, `tag_type`, `verification_status`, `pagination.*`
  - Supports sorting by market_cap, volume_24h, price, holders, deployment_time

### 9. Token Detail APIs
- **Get Token Detail**: Get detailed information for a specific token
  - Method: `GET /api/v1/tokens/detail`
  - Query params: `contract_address` (required), `chain_id` (optional)
  - Returns: basic info, overview (supply, market cap), market data, additional info
- **Get Token Transfers**: Get transfer transactions for a token
  - Method: `GET /api/v1/tokens/transfers`
  - Query params: `contract_address`, `chain_id`, `address`, `method`, `pagination.*`
- **Get Token Holders**: Get holder rankings for a token
  - Method: `GET /api/v1/tokens/holders`
  - Query params: `contract_address`, `chain_id`, `sort_by`, `sort_order`, `pagination.*`
  - Supports sorting by balance or percentage
- **Get Token Holders Stats**: Get holder statistics
  - Method: `GET /api/v1/tokens/holders-stats`
  - Query params: `contract_address`, `chain_id`
  - Returns: total_holders, top10_percentage, top100_percentage
- **Get Token Contract Code**: Get contract bytecode and source code
  - Method: `GET /api/v1/tokens/contract-code`
  - Query params: `contract_address`, `chain_id`
  - Returns: bytecode, source_code (if verified), compiler_version, etc.

### 10. Contract Info APIs
- **Create Contract Info**: Create a new contract information
  - Method: `POST /api/v1/contract/info`
  - Body: Contract info JSON
- **Get Contract Info By Address**: Retrieve contract information by address
  - Method: `GET /api/v1/contract/info/by-address`
  - Query params: `contractAddress`, `chainId`
- **Get Contracts By Symbol**: Retrieve contracts by symbol (one-to-many)
  - Method: `GET /api/v1/contract/info/by-symbol`
  - Query params: `symbol`, `chainId`, `contractType`, `pagination.*`
- **List Contract Infos**: List contract information with filters
  - Method: `GET /api/v1/contract/info/list`
  - Query params: `chainId`, `contractType`, `verificationStatus`, `name`, `symbol`, `pagination.*`
- **Update Contract Info**: Update contract information
  - Method: `PUT /api/v1/contract/info`
  - Body: Updated contract info JSON
- **Delete Contract Info**: Delete contract information (soft delete)
  - Method: `DELETE /api/v1/contract/info`
  - Query params: `contractAddress`, `chainId`
- **Add Contract Tag**: Add a tag to a contract
  - Method: `POST /api/v1/contract/info/tag`
  - Body: Tag info JSON
- **Remove Contract Tag**: Remove a tag from a contract
  - Method: `DELETE /api/v1/contract/info/tag`
  - Query params: `contractAddress`, `chainId`, `tagType`
- **Get Contract Tags**: Retrieve all tags for a contract
  - Method: `GET /api/v1/contract/info/tags`
  - Query params: `contractAddress`, `chainId`
- **Export Contract Infos**: Export contract information in CSV or JSON format
  - Method: `GET /api/v1/contract/info/export`
  - Query params: `format`, `chain_id`, `contract_type`, `verification_status`, etc.
- **Import Contract Infos**: Import contract information from CSV or JSON file
  - Method: `POST /api/v1/contract/info/import`
  - Body: Import data with format and file_content_base64
- **Upload Contract Files**: Upload contract source code files (.sol) and ABI files (.json)
  - Method: `POST /api/v1/contract/files/upload`
  - Body: Contract address, chain_id, source_code, abi, etc.

### 11. Address Label APIs
- **Create Address Label**: Create a new address label
  - Method: `POST /api/v1/address/label`
  - Body: Address label JSON (address, chain_id, label_type, etc.)
- **Get Address Labels**: Get all labels for an address
  - Method: `GET /api/v1/address/labels`
  - Query params: `address` (required), `chain_id` (required)
- **List Address Labels**: List address labels with filters
  - Method: `GET /api/v1/address/label/list`
  - Query params: `chain_id`, `label_type`, `address`, `label_value`, `pagination.*`
- **Update Address Label**: Update an existing address label
  - Method: `PUT /api/v1/address/label`
  - Body: Updated label JSON
- **Delete Address Label**: Delete an address label
  - Method: `DELETE /api/v1/address/label`
  - Query params: `address`, `chain_id`, `label_type`
- **Get Address Label Stats**: Get statistics for address labels
  - Method: `GET /api/v1/address/labels/stats`
  - Query params: `chain_id`
- **Batch Import Address Labels**: Import address labels from CSV/JSON file
  - Method: `POST /api/v1/address/labels/batch-import`
  - Body: chain_id, file_content_base64, format, import_mode
- **Batch Export Address Labels**: Export address labels to CSV/JSON file
  - Method: `POST /api/v1/address/labels/batch-export`
  - Body: chain_id, format, addresses (optional)

### 12. Address Remark APIs
- **Set Address Remark**: Set or update an address remark
  - Method: `POST /api/v1/address/remark`
  - Body: address, chain_id, remark, created_by
- **Get Address Remark**: Get an address remark
  - Method: `GET /api/v1/address/remark`
  - Query params: `address`, `chain_id`
- **Delete Address Remark**: Delete an address remark
  - Method: `DELETE /api/v1/address/remark`
  - Query params: `address`, `chain_id`

### 13. Tag Definition APIs
- **List Tag Definitions**: List tag definitions with filters
  - Method: `GET /api/v1/tag/definitions`
  - Query params: `tag_type`, `tag_category`, `is_system`, `pagination.*`
- **Create Tag Definition**: Create a new tag definition
  - Method: `POST /api/v1/tag/definition`
  - Body: tag_type, tag_name, tag_category, tag_color, etc.
- **Update Tag Definition**: Update an existing tag definition
  - Method: `PUT /api/v1/tag/definition`
  - Body: id, tag_name, tag_category, tag_color, etc.
- **Delete Tag Definition**: Delete a tag definition
  - Method: `DELETE /api/v1/tag/definition`
  - Query params: `id`
- **Get Tag Usage Stats**: Get usage statistics for tags
  - Method: `GET /api/v1/tag/usage-stats`
  - Query params: `tag_type`
- **Get Tag Definitions For Selection**: Get tag definitions for frontend selection
  - Method: `GET /api/v1/tag/definitions/selection`
  - Query params: `tag_type`, `tag_category`

### 13. API Token Management
Endpoints for managing API tokens. These require `X-User-ID` header.

- **Create Token**: Create a new API token
  - Method: `POST /api/v1/tokens`
  - Headers: `X-User-ID`
  - Body: `{ "name": "My API Token" }`
  - Response: Returns full token (shown only once), token preview, and token ID
  - Auto-saves token to environment variable

- **List Tokens**: List all tokens for a user
  - Method: `GET /api/v1/tokens`
  - Headers: `X-User-ID`

- **Get Token Usage**: Get usage statistics for a token
  - Method: `GET /api/v1/tokens/usage`
  - Query params: `token_id`
  - Headers: `X-User-ID`

- **Delete Token**: Delete a token
  - Method: `DELETE /api/v1/tokens`
  - Query params: `token_id`
  - Headers: `X-User-ID`

### 15. API Token Stats APIs
- **Get Active Token Count**: Get the count of active tokens for a user
  - Method: `GET /api/v1/tokens/active-count`
  - Query params: `days` (default: 30)
  - Returns: total_tokens, active_tokens
- **Get Error Code Analysis**: Get error code analysis for a user or token
  - Method: `GET /api/v1/stats/error-analysis`
  - Query params: `token_id` (optional), `days` (default: 30)
  - Returns: error_stats grouped by status code

### 16. Batch Query APIs
High-performance batch query interfaces for better efficiency.

- **Get Transactions Batch**: Batch query transactions by hashes
  - Method: `POST /api/v1/transactions/batch`
  - Body: `{ "hashes": ["tx_hash1", "tx_hash2", ...] }`
- **Get Blocks Batch**: Batch query blocks by heights
  - Method: `POST /api/v1/blocks/batch`
  - Body: `{ "heights": [1000000, 1000001, ...] }`

### 17. Validator Stats APIs
- **Get Validator Block Stats**: Get validator block statistics
  - Method: `GET /api/v1/validator/block-stats`
  - Query params: `validator_address` (required)
  - Returns: proposed, signed, missed blocks count, uptime_percentage

### 18. Global Tag APIs
Global tag management interfaces (not bound to specific contracts).

- **Get All Contract Tags**: Get all contract tags with filters
  - Method: `GET /api/v1/contract/tags/all`
  - Query params: `chain_id`, `tag_type`, `contract_address`, `pagination.*`
- **Add Global Tag**: Add a global tag
  - Method: `POST /api/v1/global/tag`
  - Body: chain_id, tag_type, tag_value, tag_color, etc.
- **Remove Global Tag**: Remove a global tag
  - Method: `DELETE /api/v1/global/tag`
  - Query params: `chain_id`, `tag_type`

### 19. Streaming APIs
**Note**: These endpoints use server-sent events (SSE) or similar streaming protocols and may not work properly in Postman. Use a proper streaming client or browser for testing.

- **Stream Latest Blocks**: Stream latest blocks as they are added
  - Method: `GET /api/v1/stream/blocks`
  - Query params: `fromHeight` (optional)
- **Stream Latest Transactions**: Stream latest transactions as they are added
  - Method: `GET /api/v1/stream/transactions`
  - Query params: `types` (optional, filter by transaction types)
- **Stream Account Updates**: Stream account updates (balance changes, new transactions)
  - Method: `GET /api/v1/stream/account`
  - Query params: `address` (required), `fromTimestamp` (optional)

### 20. Admin APIs
Admin management endpoints for plans, IP management, and email blacklist.

- **Plan Management**: Create, update, publish/unpublish plans
  - Methods: `GET /api/v1/admin/plan`, `GET /api/v1/admin/plans`, `POST /api/v1/admin/plans`, `PUT /api/v1/admin/plans`, `PUT /api/v1/admin/plans/publish`, `PUT /api/v1/admin/plans/unpublish`
- **IP Management**: IP registration records, blacklist, monitoring
  - Methods: `GET /api/v1/admin/ip/records`, `GET /api/v1/admin/ip/detail`, `GET /api/v1/admin/ip/blacklist`, `POST /api/v1/admin/ip/blacklist`, `DELETE /api/v1/admin/ip/blacklist`, etc.
- **Email Blacklist**: Email domain blacklist management
  - Methods: `GET /api/v1/admin/email/blacklist`, `POST /api/v1/admin/email/blacklist`, `PUT /api/v1/admin/email/blacklist`, `DELETE /api/v1/admin/email/blacklist`, etc.
- **Usage Statistics**: Error logs, usage stats, trends
  - Methods: `GET /api/v1/admin/usage/errors`, `GET /api/v1/admin/usage/stats`, etc.
- **User Token Management**: List user tokens, get token stats
  - Methods: `GET /api/v1/admin/user/tokens`, `GET /api/v1/admin/user/token-stats`

#### Admin - Token Market Cache Management

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

### 20. Rate Limit & Auth Tests
Test cases for rate limiting and authentication:

- **Test IP Rate Limit**: Send multiple requests quickly to test IP rate limit (100/min)
- **Test Token Rate Limit**: Send 6+ requests per second to test token rate limit (5/sec)
- **Test Invalid Token**: Test with invalid token (should return 401)
- **Test Missing Token**: Test without token (should return 401)

## Authentication

Most API endpoints require Bearer token authentication:
- Header: `Authorization: Bearer {api_token}`

Token management endpoints require:
- Header: `X-User-ID: {user_id}`

Rate limits apply:
- IP: 100 requests per minute
- Token: 5 requests per second
- User: 100,000 requests per month

## Usage Flow

### Quick Start

1. **Create a Token**:
   - Run "Create Token" request from "API Token Management" folder
   - The token will be automatically saved to `api_token` environment variable
   - Copy the token for use in other requests

2. **Use the Token**:
   - All API requests (except token management) automatically use the saved token
   - The token is included in the `Authorization: Bearer {api_token}` header

3. **Test APIs**:
   - Start with "Health Check" to verify the server is running
   - Test "Block APIs" to get latest blocks and block height
   - Test "Transaction APIs" to query transactions
   - Test "Account APIs" with a valid account address
   - Test "Contract Info APIs" to manage contract information

4. **Test Rate Limits**:
   - Use "Test IP Rate Limit" or "Test Token Rate Limit" to verify rate limiting
   - Run the request multiple times quickly to trigger rate limits

## Testing Tips

1. **Token Creation**: 
   - The full token is only shown once. Save it immediately or use the token preview for reference.
   - Token is automatically saved to environment variable after creation

2. **Environment Variables**:
   - Update `user_id` to match your test user
   - Update `base_url` to match your server URL (default: `http://localhost:8000`)
   - Set `account_address` to a valid address (e.g., `inj1...` or `0x...`)
   - Set `contract_address` for contract-related tests
   - Set `chain_id` (default: `injective-1`)
   - Set `token_symbol`: Token symbol for testing (default: `inj`)
     - Used in token detail, list, and market cache admin endpoints
     - Examples: `inj`, `usdt`, `usdc`
   - Set `validator_address` for validator stats testing
   - Token and token_id are set automatically after token creation
   - Webhook_id is set automatically after creating a webhook

3. **Rate Limiting**: 
   - IP rate limit: Send 100+ requests within 1 minute
   - Token rate limit: Send 6+ requests within 1 second
   - Monthly limit: Requires 100,000+ requests (not practical for manual testing)

4. **Pagination**:
   - Most list endpoints support pagination with `page` and `page_size`
   - Cursor-based pagination is also supported (use `cursor` parameter)
   - Cursor pagination takes priority over page-based pagination

5. **Error Testing**:
   - Test with invalid tokens
   - Test without tokens
   - Test with disabled tokens (requires database manipulation)
   - Test with invalid addresses or parameters

6. **Streaming APIs**:
   - Streaming endpoints may not work properly in Postman
   - Use a proper streaming client (e.g., curl with `--no-buffer`, or a browser) for testing
   - These endpoints use server-sent events (SSE) or similar protocols

7. **Contract Info APIs**:
   - Create a contract info first before testing other contract operations
   - Contract addresses should be valid addresses on the chain
   - Tags can be added/removed after contract creation

8. **Token List & Detail APIs**:
   - Set `contract_address` to a valid token contract address for testing
   - Token list supports multiple filters: chain_id, contract_type, search, tag_type, verification_status
   - Token detail provides comprehensive information including market data, holders, and contract code

9. **Batch Query APIs**:
   - Use for high-performance batch data retrieval
   - Batch transactions: Provide array of transaction hashes
   - Batch blocks: Provide array of block heights
   - Maximum batch size may be limited by server configuration

10. **Validator Stats APIs**:
    - Set `validator_address` to a valid validator operator address
    - Returns statistics about blocks proposed, signed, and missed

11. **Address Label APIs**:
    - Labels can be created with different types and colors
    - Supports batch import/export via CSV/JSON files
    - Labels can be marked as visible or hidden in frontend

12. **Tag Definition APIs**:
    - System preset tags cannot be deleted
    - Tags can be categorized as normal or abnormal
    - Usage stats show how often each tag is used

## Example Requests

### Create Token
```http
POST http://localhost:8000/api/v1/tokens
Content-Type: application/json
X-User-ID: test_user_123

{
    "name": "My API Token"
}
```

### Use Token - Get Latest Blocks
```http
GET http://localhost:8000/api/v1/block/latest?page=1&page_size=20
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Get Account Info
```http
GET http://localhost:8000/api/v1/account/info?address=inj1...
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Create Contract Info
```http
POST http://localhost:8000/api/v1/contract/info
Content-Type: application/json
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

{
    "contractAddress": "inj1...",
    "name": "Test Contract",
    "tokenName": "Test Token",
    "tokenSymbol": "TEST",
    "contractType": "cosmwasm",
    "chainId": "injective-1",
    "verificationStatus": "unverified"
}
```

### Get EVM Transaction
```http
GET http://localhost:8000/api/v1/evm/transaction?hash=0x1234...
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Get Token List
```http
GET http://localhost:8000/api/v1/tokens/list?chain_id=injective-1&sort_by=market_cap&sort_order=desc&pagination.page=1&pagination.page_size=20
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Get Token Detail
```http
GET http://localhost:8000/api/v1/tokens/detail?contract_address=inj1...&chain_id=injective-1
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Get Token Transfers
```http
GET http://localhost:8000/api/v1/tokens/transfers?contract_address=inj1...&pagination.page=1&pagination.page_size=20
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Get Token Holders
```http
GET http://localhost:8000/api/v1/tokens/holders?contract_address=inj1...&sort_by=balance&sort_order=desc&pagination.page=1&pagination.page_size=20
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Batch Query Transactions
```http
POST http://localhost:8000/api/v1/transactions/batch
Content-Type: application/json
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

{
    "hashes": [
        "0x1234567890abcdef...",
        "0xabcdef1234567890..."
    ]
}
```

### Batch Query Blocks
```http
POST http://localhost:8000/api/v1/blocks/batch
Content-Type: application/json
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

{
    "heights": [1000000, 1000001, 1000002]
}
```

### Get Validator Block Stats
```http
GET http://localhost:8000/api/v1/validator/block-stats?validator_address=injvaloper1...
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Create Address Label
```http
POST http://localhost:8000/api/v1/address/label
Content-Type: application/json
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

{
    "address": "inj1...",
    "chain_id": "injective-1",
    "label_type": "exchange",
    "label_value": "Binance",
    "label_color": "#F0B90B",
    "description": "Binance Hot Wallet",
    "visible": true
}
```

### Create Tag Definition
```http
POST http://localhost:8000/api/v1/tag/definition
Content-Type: application/json
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

{
    "tag_type": "contract",
    "tag_name": "DeFi",
    "tag_category": "normal",
    "tag_color": "#FF6B6B",
    "description": "DeFi protocol contract"
}
```

### Get Active Token Count
```http
GET http://localhost:8000/api/v1/tokens/active-count?days=30
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Get Error Code Analysis
```http
GET http://localhost:8000/api/v1/stats/error-analysis?days=30
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Get IBC Transfer Transactions
```http
GET http://localhost:8000/api/v1/ibc/transfers?sender=inj1...&pagination.page=1&pagination.page_size=20
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Get Peggy Deposit Transactions
```http
GET http://localhost:8000/api/v1/peggy/deposits?receiver=inj1...&pagination.page=1&pagination.page_size=20
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Get Peggy Withdrawal Transactions
```http
GET http://localhost:8000/api/v1/peggy/withdrawals?sender=inj1...&pagination.page=1&pagination.page_size=20
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

## Notes

- **Token Management**: Endpoints under `/api/v1/tokens/*` do NOT require Bearer token authentication. They require `X-User-ID` header instead (for user authentication).

- **All Other APIs**: All other API endpoints require Bearer token authentication via `Authorization: Bearer {api_token}` header.

- **Rate Limiting**: Rate limiting is applied to all authenticated API requests:
  - IP-based: 100 requests per minute
  - Token-based: 5 requests per second
  - User-based: 100,000 requests per month

- **Health Check**: The `/api/v1/health` endpoint does not require authentication.

- **Streaming APIs**: Streaming endpoints (`/api/v1/stream/*`) may not work properly in Postman. Use a proper streaming client for testing.

- **Response Format**: Most endpoints return responses in the format:
  ```json
  {
    "code": 0,
    "message": "success",
    "data": { ... }
  }
  ```
  Where `code` 0 indicates success, non-zero indicates error.

## Collection Statistics

The Complete API Collection includes:
- **63+ API endpoints** covering all HTTP interfaces
- **21 categories** of APIs (Health, Blocks, Transactions, Cross-Chain Transactions, Accounts, EVM, Token Price, Token List, Token Detail, Contract Info, Address Label, Address Remark, Tag Definition, API Token Management, API Token Stats, Batch Query, Validator Stats, Global Tag, Streaming, Admin, Rate Limit Tests)
- **Automated test scripts** for response validation
- **Environment variable management** for easy configuration
- **Comprehensive documentation** in request descriptions


## Recent Updates

### Messages Field Format Update (2026-02-05)

**Transaction API 测试更新**

"根据交易哈希获取交易详情" (Get Transaction By Hash) 接口测试已更新，添加了对 `messages` 字段的验证：

**新增测试项：**

1. **Messages 字段类型验证**
   - 验证 `messages` 是数组而不是字符串
   - 确保返回结构化的 JSON 对象数组

2. **Messages 内容验证**
   - 验证每个消息对象包含 `@type` 字段
   - 验证消息已解码（不包含 base64 编码的 `value` 字段）
   - 对于 `MsgSend` 类型，验证包含 `from_address`、`to_address`、`amount` 字段

**API 响应格式变更：**

Before:
```json
{
  "messages": "{\"messages\":[{\"@type\":\"...\",\"field\":\"value\"}]}"
}
```

After:
```json
{
  "messages": [
    {
      "@type": "/cosmos.bank.v1beta1.MsgSend",
      "from_address": "0x...",
      "to_address": "0x...",
      "amount": [{"denom": "inj", "amount": "1000000"}]
    }
  ]
}
```

**优势：**
- ✅ 更好的类型安全
- ✅ 无需手动解析 JSON 字符串
- ✅ 直接访问消息字段
- ✅ 与 protobuf 定义一致

