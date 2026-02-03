-- Migration: Add has_token_transfer field to transactions table
-- This field is used to efficiently filter EVM transactions with token transfers
-- Date: 2026-02-02

-- Add has_token_transfer column to transactions table
ALTER TABLE transactions 
ADD COLUMN has_token_transfer BOOLEAN DEFAULT FALSE NOT NULL COMMENT 'Indicates if this transaction has ERC20/ERC721 token transfers (for efficient filtering)';

-- Add index on has_token_transfer for faster filtering
CREATE INDEX idx_transactions_has_token_transfer ON transactions(has_token_transfer);

-- Update existing transactions that have token transfers
-- This backfills the field for existing data
UPDATE transactions t
SET has_token_transfer = TRUE
WHERE EXISTS (
    SELECT 1 
    FROM token_transfers tt 
    WHERE tt.transaction_hash = t.hash
)
AND (t.tx_type = 'MsgEthereumTx' OR JSON_SEARCH(t.message_types, 'one', '%MsgEthereumTx%') IS NOT NULL);

-- For MySQL, also check message_types as JSON array
-- Note: This query may take some time on large datasets
UPDATE transactions t
SET has_token_transfer = TRUE
WHERE EXISTS (
    SELECT 1 
    FROM token_transfers tt 
    WHERE tt.transaction_hash = t.hash
)
AND (
    t.tx_type = 'MsgEthereumTx' 
    OR JSON_CONTAINS(t.message_types, '"MsgEthereumTx"')
    OR JSON_SEARCH(t.message_types, 'one', '%MsgEthereumTx%') IS NOT NULL
    OR JSON_SEARCH(t.message_types, 'one', '%ethermint.evm.v1%') IS NOT NULL
    OR JSON_SEARCH(t.message_types, 'one', '%injective.evm.v1%') IS NOT NULL
);
