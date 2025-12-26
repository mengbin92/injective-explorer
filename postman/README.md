# Postman Collection for API Management

This Postman collection provides test cases for the API Management system.

## Setup

1. Import the collection file `API_Management.postman_collection.json` into Postman
2. Set up environment variables (or use the default values):
   - `base_url`: API server URL (default: `http://localhost:8000`)
   - `user_id`: User ID for token management (default: `test_user_123`)
   - `api_token`: API token (will be set automatically after creating a token)
   - `token_id`: Token ID (will be set automatically after creating a token)
   - `tx_hash`: Transaction hash for testing (optional)
   - `account_address`: Account address for testing (optional)

## Collection Structure

### 1. Token Management
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

### 2. API Access (with Token)
Endpoints that require Bearer token authentication.

- **Get Latest Blocks**: Get latest blocks with pagination
- **Get Transaction By Hash**: Get transaction details by hash
- **Get Account Info**: Get account information

All these endpoints require:
- Header: `Authorization: Bearer {api_token}`

Rate limits apply:
- IP: 100 requests per minute
- Token: 5 requests per second
- User: 100,000 requests per month

### 3. Rate Limit Tests
Test cases for rate limiting and authentication:

- **Test IP Rate Limit**: Send multiple requests quickly to test IP rate limit
- **Test Token Rate Limit**: Send 6+ requests per second to test token rate limit
- **Test Invalid Token**: Test with invalid token (should return 401)
- **Test Missing Token**: Test without token (should return 401)

## Usage Flow

1. **Create a Token**:
   - Run "Create Token" request
   - The token will be automatically saved to `api_token` environment variable
   - Copy the token for use in other requests

2. **Use the Token**:
   - All "API Access" requests automatically use the saved token
   - The token is included in the `Authorization: Bearer {api_token}` header

3. **Test Rate Limits**:
   - Use "Test IP Rate Limit" or "Test Token Rate Limit" to verify rate limiting
   - Run the request multiple times quickly to trigger rate limits

## Testing Tips

1. **Token Creation**: The full token is only shown once. Save it immediately or use the token preview for reference.

2. **Rate Limiting**: 
   - IP rate limit: Send 100+ requests within 1 minute
   - Token rate limit: Send 6+ requests within 1 second
   - Monthly limit: Requires 100,000+ requests (not practical for manual testing)

3. **Environment Variables**: 
   - Update `user_id` to match your test user
   - Update `base_url` to match your server URL
   - Token and token_id are set automatically after token creation

4. **Error Testing**:
   - Test with invalid tokens
   - Test without tokens
   - Test with disabled tokens (requires database manipulation)

## Example Request

### Create Token
```http
POST http://localhost:8000/api/v1/tokens
Content-Type: application/json
X-User-ID: test_user_123

{
    "name": "My API Token"
}
```

### Use Token
```http
GET http://localhost:8000/api/v1/block/latest-height
Authorization: Bearer inj_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

## Notes

- Token management endpoints (`/api/v1/tokens/*`) do NOT require Bearer token authentication
- They require `X-User-ID` header instead (for user authentication)
- All other API endpoints require Bearer token authentication
- Rate limiting is applied to all authenticated API requests

