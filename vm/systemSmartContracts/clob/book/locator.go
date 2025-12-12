package book

import (
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
)

// OrderLocator keeps fast references to an order's position in the order book.
//
// This structure enables O(1) order cancellation and removal using the order's
// embedded intrusive list pointers (prev/next).
//
// Complexity Guarantees:
//   - Order Insertion: O(1) - PushBack to price level queue + store locator
//   - Order Cancellation: O(1) - Direct removal via order's intrusive pointers
//   - Order Matching/Fill: O(1) - Direct removal via order's intrusive pointers
//
// The Order field contains the intrusive list pointers needed for O(1) removal.
// Stop orders use a different storage mechanism.
type OrderLocator struct {
	Order *core.Order     // The order being tracked (has embedded prev/next pointers)
	Side  core.Side       // Buy or Sell side
	Price uint64          // Price level where the order is stored
}

// NewLocator creates an OrderLocator for tracking an order's position in the order book.
// This constructor ensures consistent initialization of the locator fields.
func NewLocator(order *core.Order) *OrderLocator {
	return &OrderLocator{
		Order: order,
		Side:  order.GetSide(),
		Price: order.GetPrice(),
	}
}
