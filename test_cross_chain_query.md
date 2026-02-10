# Cross-Chain Transaction Query Fix

## Problem

When querying transactions for an address (e.g., `0x8fb68d46f8856b00632b4141f139499df864c9ff`), cross-chain bridge transactions (such as `MsgDepositClaim`) were not being returned in the results.

### Example Missing Transaction

```json
{
    "block_number": 395740,
    "block_unix_timestamp": 1770627999,
    "gas_used": 111507,
    "hash": "0xd6c96022bc095df80302a3135ce93b88e8c69b0b0ba63c0f47b91cde84d49a40",
    "messages": [
        {
            "@type": "/injective.peggy.v1.MsgDepositClaim",
            "amount": "7000000000000000000",
            "cosmos_receiver": "0x8fb68d46f8856b00632b4141f139499df864c9ff",
            "ethereum_sender": "0x8fb68d46f8856b00632b4141f139499df864c9ff",
            "orchestrator": "0x7bbdc9ae25ad167a40a401d1bda05130db5ee235"
        }
    ],
    "signer": "0x7bbdc9ae25ad167a40a401d1bda05130db5ee235"
}
```

**Issue**: The `signer` field is the orchestrator address, not the user address (`ethereum_sender`/`cosmos_receiver`).

## Root Cause

The `GetAccountTransactions` API only queried the `transactions` table using:

```sql
WHERE (signer = ? OR `to` = ?)
```

However, cross-chain transactions are stored in separate tables:
- `peggy_deposit_txs` - For deposits from EVM chains to Injective chain
- `peggy_withdrawal_txs` - For withdrawals from Injective chain to EVM chains
- `ibc_transfer_txs` - For IBC transfers

These tables use `sender` and `receiver` fields instead of `signer` and `to`.

## Solution

Modified the `GetAccountTransactions` method in `internal/service/explorer.go` to:

1. Query regular transactions from the `transactions` table (existing logic)
2. Query cross-chain transactions from:
   - `peggy_deposit_txs` (where `sender = ? OR receiver = ?`)
   - `peggy_withdrawal_txs` (where `sender = ? OR receiver = ?`)
   - `ibc_transfer_txs` (where `sender = ? OR receiver = ?`)
3. Convert cross-chain transactions to regular transaction format
4. Merge all transactions and sort by timestamp
5. Apply pagination limit after merging

### New Helper Methods

1. **`queryCrossChainTransactionsForAddress`**
   - Queries all three cross-chain transaction tables
   - Returns unified `CrossChainTransaction` structs

2. **`convertCrossChainToTransactions`**
   - Converts cross-chain transactions to `models.Transaction` format
   - Checks for duplicates (transactions may exist in both tables)
   - Creates simplified transaction records for display

## Changes Made

### File: `internal/service/explorer.go`

#### Modified Method: `GetAccountTransactions`

Added logic to:
- Query cross-chain transactions after regular transactions
- Convert and merge results
- Sort combined results by timestamp
- Apply pagination to merged results

#### New Struct: `CrossChainTransaction`

```go
type CrossChainTransaction struct {
    TxHash         string
    BlockHeight    uint64
    BlockTimestamp uint64
    Sender         string
    Receiver       string
    Amount         string
    Denom          string
    TxType         string
    State          string
}
```

#### New Method: `queryCrossChainTransactionsForAddress`

Queries Peggy and IBC transaction tables for a given address.

#### New Method: `convertCrossChainToTransactions`

Converts cross-chain transactions to regular transaction format, avoiding duplicates.

## Testing

To test the fix:

1. Query an address that has cross-chain transactions:
   ```bash
   curl -X POST http://localhost:8080/api/explorer/v1/get_account_transactions \
     -H "Content-Type: application/json" \
     -d '{
       "address": "0x8fb68d46f8856b00632b4141f139499df864c9ff",
       "pagination": {"page": 1, "page_size": 20}
     }'
   ```

2. Verify that the response includes:
   - Regular transactions (where the address is `signer` or `to`)
   - Peggy deposit transactions (where the address is `sender` or `receiver`)
   - Peggy withdrawal transactions (where the address is `sender` or `receiver`)
   - IBC transfer transactions (where the address is `sender` or `receiver`)

3. Check that transactions are sorted by timestamp in descending order

## Benefits

- Users can now see their complete transaction history, including cross-chain bridge transactions
- No changes needed to the database schema
- Backward compatible with existing code
- Proper pagination support for merged results
- Efficient querying with deduplication logic

## Notes

- Cross-chain transactions that don't exist in the main `transactions` table will be returned with limited information
- The merge logic ensures no duplicate transactions in the response
- Sorting by timestamp ensures chronological order
- Pagination is applied after merging to ensure correct page counts
