package book

import (
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
)

// OrderQueue represents a queue of orders for a specific price level.
// It uses an intrusive doubly-linked list for O(1) removal without separate node allocations.
//
// Design Decision: This wrapper provides semantic clarity by naming the queue
// in domain terms (OrderQueue) rather than implementation terms (IntrusiveQueue).
// The wrapper is kept intentionally thin - it only provides domain-specific
// method names (e.g., RemoveOrder) while delegating to the underlying IntrusiveQueue.
type OrderQueue struct {
	*IntrusiveQueue
}

// NewOrderQueue creates a new instance of the OrderQueue.
func NewOrderQueue(price uint64) *OrderQueue {
	return &OrderQueue{
		IntrusiveQueue: NewIntrusiveQueue(price),
	}
}

// RemoveOrder removes a specific order from the queue.
// This is the main removal method and guarantees O(1) complexity via intrusive pointers.
func (oq *OrderQueue) RemoveOrder(order *core.Order) *core.Order {
	return oq.IntrusiveQueue.Remove(order)
}
