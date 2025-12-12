package book

import (
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
)

// IntrusiveQueue is a doubly-linked list using Order's embedded pointers.
// This eliminates allocations for list nodes (no more list.Element overhead).
//
// All operations maintain the following invariants:
//   - head == nil iff tail == nil iff len == 0
//   - For any order in the queue: order.prev points to the previous order (or nil if head)
//   - For any order in the queue: order.next points to the next order (or nil if tail)
//   - totalQty is the sum of all orders' quantities in the queue
type IntrusiveQueue struct {
	head     *core.Order
	tail     *core.Order
	len      int
	price    uint64
	totalQty uint64
}

// NewIntrusiveQueue creates a new intrusive queue for a specific price level.
func NewIntrusiveQueue(price uint64) *IntrusiveQueue {
	return &IntrusiveQueue{
		price: price,
	}
}

// Len returns the number of orders in the queue.
func (q *IntrusiveQueue) Len() int {
	return q.len
}

// Front returns the first order in the queue without removing it.
func (q *IntrusiveQueue) Front() *core.Order {
	return q.head
}

// Back returns the last order in the queue without removing it.
func (q *IntrusiveQueue) Back() *core.Order {
	return q.tail
}

// PushBack adds an order to the back of the queue.
// The order's Prev/Next pointers MUST be nil before calling this.
func (q *IntrusiveQueue) PushBack(order *core.Order) {
	// Clear any existing links (safety check)
	order.Prev = nil
	order.Next = nil

	if q.tail == nil {
		// Empty queue
		q.head = order
		q.tail = order
	} else {
		// Append to tail
		q.tail.Next = order
		order.Prev = q.tail
		q.tail = order
	}

	q.len++
	q.totalQty += order.GetQuantity()
}

// PopFront removes and returns the first order from the queue.
func (q *IntrusiveQueue) PopFront() *core.Order {
	if q.head == nil {
		return nil
	}

	order := q.head
	q.head = order.Next

	if q.head == nil {
		// Queue is now empty
		q.tail = nil
	} else {
		q.head.Prev = nil
	}

	// Clear the removed order's pointers
	order.Prev = nil
	order.Next = nil

	q.len--
	q.totalQty -= order.GetQuantity()

	return order
}

// Remove removes a specific order from the queue in O(1) time.
// This uses the order's embedded Prev/Next pointers for direct removal.
// Returns the removed order, or nil if the order was not in the queue.
func (q *IntrusiveQueue) Remove(order *core.Order) *core.Order {
	if order == nil {
		return nil
	}

	// Update the previous node's Next pointer
	if order.Prev != nil {
		order.Prev.Next = order.Next
	} else {
		// This was the head
		q.head = order.Next
	}

	// Update the next node's Prev pointer
	if order.Next != nil {
		order.Next.Prev = order.Prev
	} else {
		// This was the tail
		q.tail = order.Prev
	}

	// Clear the removed order's pointers
	order.Prev = nil
	order.Next = nil

	q.len--
	q.totalQty -= order.GetQuantity()

	return order
}

// ApplyFill decreases cached totals when an order in this level is partially filled.
func (q *IntrusiveQueue) ApplyFill(qty uint64) {
	if qty >= q.totalQty {
		q.totalQty = 0
		return
	}
	q.totalQty -= qty
}

// GetTotalQty returns the total quantity in the queue.
func (q *IntrusiveQueue) GetTotalQty() uint64 {
	return q.totalQty
}

// GetPrice returns the price level of this queue.
func (q *IntrusiveQueue) GetPrice() uint64 {
	return q.price
}

// Iterate calls the provided function for each order in the queue.
// If the function returns false, iteration stops early.
func (q *IntrusiveQueue) Iterate(fn func(*core.Order) bool) {
	for order := q.head; order != nil; order = order.Next {
		if !fn(order) {
			return
		}
	}
}

// GetOrderIDs returns a slice of all order IDs in the queue.
func (q *IntrusiveQueue) GetOrderIDs() []uint64 {
	ids := make([]uint64, 0, q.len)
	for order := q.head; order != nil; order = order.Next {
		ids = append(ids, order.GetID())
	}
	return ids
}
