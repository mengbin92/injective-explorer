// Package service provides object pools for reducing GC pressure.
package service

import (
	"injective-explorer/internal/models"
	"sync"
)

var (
	// blockPool is a pool for Block objects to reduce GC pressure
	blockPool = sync.Pool{
		New: func() interface{} {
			return &models.Block{}
		},
	}

	// transactionPool is a pool for Transaction objects to reduce GC pressure
	transactionPool = sync.Pool{
		New: func() interface{} {
			return &models.Transaction{}
		},
	}
)

// GetBlockFromPool gets a Block from the object pool.
// The caller should call PutBlockToPool when done with the block.
//
// Returns:
//   - *models.Block: A Block instance from the pool
func GetBlockFromPool() *models.Block {
	return blockPool.Get().(*models.Block)
}

// PutBlockToPool puts a Block back to the object pool.
// The block is reset to zero values before being put back.
//
// Parameters:
//   - block: The Block instance to return to the pool
func PutBlockToPool(block *models.Block) {
	if block == nil {
		return
	}
	// Reset the block to zero values
	*block = models.Block{}
	blockPool.Put(block)
}

// GetTransactionFromPool gets a Transaction from the object pool.
// The caller should call PutTransactionToPool when done with the transaction.
//
// Returns:
//   - *models.Transaction: A Transaction instance from the pool
func GetTransactionFromPool() *models.Transaction {
	return transactionPool.Get().(*models.Transaction)
}

// PutTransactionToPool puts a Transaction back to the object pool.
// The transaction is reset to zero values before being put back.
//
// Parameters:
//   - tx: The Transaction instance to return to the pool
func PutTransactionToPool(tx *models.Transaction) {
	if tx == nil {
		return
	}
	// Reset the transaction to zero values
	*tx = models.Transaction{}
	transactionPool.Put(tx)
}
