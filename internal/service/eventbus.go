// Package service provides an in-memory event bus for decoupling data synchronization
// from streaming services. This allows the SyncService to publish events when new
// blocks or transactions are processed, and the streaming services can subscribe
// to these events to push real-time updates to clients.
package service

import (
	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"sync"
)

// Event types for different kinds of blockchain events
const (
	EventTypeBlockNew          = "block_new"
	EventTypeBlockUpdate       = "block_update"
	EventTypeTransactionNew    = "transaction_new"
	EventTypeTransactionUpdate = "transaction_update"
	EventTypeAccountUpdate     = "account_update"
)

// BlockEvent represents a block-related event
type BlockEvent struct {
	Block *models.Block
	Type  string
}

// TransactionEvent represents a transaction-related event
type TransactionEvent struct {
	Transaction *models.Transaction
	Type        string
}

// AccountEvent represents an account-related event
type AccountEvent struct {
	Address string
	Type    string
	Data    map[string]interface{}
}

// EventBus is an in-memory event bus for publishing and subscribing to blockchain events.
// It uses channels to distribute events to subscribers in a thread-safe manner.
type EventBus struct {
	// Block event channels (one per subscriber)
	blockSubscribers map[chan *BlockEvent]bool
	blockMutex       sync.RWMutex

	// Transaction event channels (one per subscriber)
	txSubscribers map[chan *TransactionEvent]bool
	txMutex       sync.RWMutex

	// Account event channels (one channel per address per subscriber)
	// Key: address, Value: map of subscriber channels
	accountSubscribers map[string]map[chan *AccountEvent]bool
	accountMutex       sync.RWMutex
}

var (
	globalEventBus *EventBus
	eventBusOnce   sync.Once
)

// GetEventBus returns the global event bus instance (singleton).
func GetEventBus() *EventBus {
	eventBusOnce.Do(func() {
		globalEventBus = &EventBus{
			blockSubscribers:   make(map[chan *BlockEvent]bool),
			txSubscribers:      make(map[chan *TransactionEvent]bool),
			accountSubscribers: make(map[string]map[chan *AccountEvent]bool),
		}
	})
	return globalEventBus
}

// SubscribeBlocks subscribes to block events and returns a channel for receiving events.
// The caller should call UnsubscribeBlocks when done to prevent memory leaks.
func (eb *EventBus) SubscribeBlocks() chan *BlockEvent {
	eb.blockMutex.Lock()
	defer eb.blockMutex.Unlock()

	ch := make(chan *BlockEvent, 100) // Buffered channel to avoid blocking
	eb.blockSubscribers[ch] = true
	return ch
}

// UnsubscribeBlocks unsubscribes from block events and closes the channel.
func (eb *EventBus) UnsubscribeBlocks(ch chan *BlockEvent) {
	eb.blockMutex.Lock()
	defer eb.blockMutex.Unlock()

	if _, exists := eb.blockSubscribers[ch]; exists {
		delete(eb.blockSubscribers, ch)
		close(ch)
	}
}

// PublishBlock publishes a block event to all subscribers.
func (eb *EventBus) PublishBlock(event *BlockEvent) {
	eb.blockMutex.RLock()
	defer eb.blockMutex.RUnlock()

	subscriberCount := len(eb.blockSubscribers)
	if subscriberCount == 0 {
		// No subscribers, nothing to do
		return
	}

	global.Logger.Debugf("PublishBlock: publishing event for block height=%d to %d subscribers",
		event.Block.Height, subscriberCount)

	skippedCount := 0
	for ch := range eb.blockSubscribers {
		select {
		case ch <- event:
			// Successfully sent
		default:
			// Channel is full, skip this subscriber to avoid blocking
			skippedCount++
			global.Logger.Warnf("PublishBlock: channel full, skipping subscriber (block height=%d)",
				event.Block.Height)
		}
	}

	if skippedCount > 0 {
		global.Logger.Warnf("PublishBlock: %d/%d subscribers skipped due to full channels",
			skippedCount, subscriberCount)
	} else {
		global.Logger.Debugf("PublishBlock: successfully published to all %d subscribers", subscriberCount)
	}
}

// SubscribeTransactions subscribes to transaction events and returns a channel for receiving events.
// The caller should call UnsubscribeTransactions when done to prevent memory leaks.
func (eb *EventBus) SubscribeTransactions() chan *TransactionEvent {
	eb.txMutex.Lock()
	defer eb.txMutex.Unlock()

	ch := make(chan *TransactionEvent, 100) // Buffered channel to avoid blocking
	eb.txSubscribers[ch] = true
	return ch
}

// UnsubscribeTransactions unsubscribes from transaction events and closes the channel.
func (eb *EventBus) UnsubscribeTransactions(ch chan *TransactionEvent) {
	eb.txMutex.Lock()
	defer eb.txMutex.Unlock()

	if _, exists := eb.txSubscribers[ch]; exists {
		delete(eb.txSubscribers, ch)
		close(ch)
	}
}

// PublishTransaction publishes a transaction event to all subscribers.
func (eb *EventBus) PublishTransaction(event *TransactionEvent) {
	eb.txMutex.RLock()
	defer eb.txMutex.RUnlock()

	for ch := range eb.txSubscribers {
		select {
		case ch <- event:
		default:
			// Channel is full, skip this subscriber to avoid blocking
		}
	}
}

// SubscribeAccount subscribes to account events for a specific address.
// The caller should call UnsubscribeAccount when done to prevent memory leaks.
func (eb *EventBus) SubscribeAccount(address string) chan *AccountEvent {
	eb.accountMutex.Lock()
	defer eb.accountMutex.Unlock()

	if eb.accountSubscribers[address] == nil {
		eb.accountSubscribers[address] = make(map[chan *AccountEvent]bool)
	}

	ch := make(chan *AccountEvent, 100) // Buffered channel to avoid blocking
	eb.accountSubscribers[address][ch] = true
	return ch
}

// UnsubscribeAccount unsubscribes from account events for a specific address.
func (eb *EventBus) UnsubscribeAccount(address string, ch chan *AccountEvent) {
	eb.accountMutex.Lock()
	defer eb.accountMutex.Unlock()

	if subscribers, exists := eb.accountSubscribers[address]; exists {
		if _, chExists := subscribers[ch]; chExists {
			delete(subscribers, ch)
			close(ch)
			if len(subscribers) == 0 {
				delete(eb.accountSubscribers, address)
			}
		}
	}
}

// PublishAccount publishes an account event for a specific address to all subscribers.
func (eb *EventBus) PublishAccount(address string, event *AccountEvent) {
	eb.accountMutex.RLock()
	defer eb.accountMutex.RUnlock()

	if subscribers, exists := eb.accountSubscribers[address]; exists {
		for ch := range subscribers {
			select {
			case ch <- event:
			default:
				// Channel is full, skip this subscriber to avoid blocking
			}
		}
	}
}
