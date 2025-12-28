# Postman Collections for Injective Explorer

This directory contains Postman collections for testing the Injective Explorer API.

## Collections

### 1. Complete API Collection (`Injective_Explorer_Complete_API.postman_collection.json`)
**Recommended for comprehensive testing**

This collection includes test cases for **ALL** HTTP endpoints in the Injective Explorer API:

- **Health Check**: Health check endpoint
- **Block APIs**: Latest blocks, block by height, latest height, gas utilization
- **Transaction APIs**: Latest transactions, transaction by hash, stats, failed transactions
- **Account APIs**: Account info, transactions, balances
- **EVM APIs**: EVM transactions, EVM account transactions
- **Token Price APIs**: Token price and market cap
- **Contract Info APIs**: Create, read, update, delete contract info, tags management
- **API Token Management**: Create, list, delete tokens, usage statistics
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

### 4. Account APIs
- **Get Account Info**: Get account information
  - Method: `GET /api/v1/account/info`
  - Query params: `address`
- **Get Account Transactions**: Get account transactions with optional type filtering
  - Method: `GET /api/v1/account/transactions`
  - Query params: `address`, `pagination.page`, `pagination.page_size`, `tx_types`
- **Get Account Balances**: Get account balances
  - Method: `GET /api/v1/account/balances`
  - Query params: `address`

### 5. EVM APIs
- **Get EVM Transaction**: Get EVM transaction by hash
  - Method: `GET /api/v1/evm/transaction/{hash}`
- **Get EVM Transactions**: Get EVM transactions list
  - Method: `GET /api/v1/evm/transactions`
  - Query params: `page`, `pageSize`, `cursor`
- **Get EVM Account Transactions**: Get EVM account transactions
  - Method: `GET /api/v1/evm/account/{address}/transactions`
  - Query params: `page`, `pageSize`, `cursor`
  - Supports both `0x` and `inj1` address formats

### 6. Token Price APIs
- **Get Token Price and Market Cap**: Get token price and market cap
  - Method: `GET /api/v1/token/price-marketcap`
  - Query params: `token_symbol` (optional, uses default if not specified)

### 7. Contract Info APIs
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

### 8. API Token Management
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
  - Method: `GET /api/v1/tokens/{token_id}/usage`
  - Headers: `X-User-ID`

- **Delete Token**: Delete a token
  - Method: `DELETE /api/v1/tokens/{token_id}`
  - Headers: `X-User-ID`

### 9. Streaming APIs
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

### 10. Rate Limit & Auth Tests
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
   - Token and token_id are set automatically after token creation

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
GET http://localhost:8000/api/v1/evm/transaction/0x1234...
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
- **40+ API endpoints** covering all HTTP interfaces
- **10 categories** of APIs (Health, Blocks, Transactions, Accounts, EVM, Token Price, Contract Info, Token Management, Streaming, Rate Limit Tests)
- **Automated test scripts** for response validation
- **Environment variable management** for easy configuration
- **Comprehensive documentation** in request descriptions

