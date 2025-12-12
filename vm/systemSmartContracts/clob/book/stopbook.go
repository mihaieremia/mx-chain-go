package book

import (
	"github.com/google/btree"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
)

// stopEntry holds orders at a specific stop price level
type stopEntry struct {
	stopPrice uint64
	orders    []*core.Order // Orders at this stop price
}

// IndexedStopBook provides O(log n) stop order operations using B-trees.
// Buy stop orders are indexed ascending (trigger when price >= stop).
// Sell stop orders are indexed descending (trigger when price <= stop).
type IndexedStopBook struct {
	// buyStops indexed by stop price ascending - for finding stops where stopPrice <= lastPrice
	buyStops *btree.BTreeG[*stopEntry]
	// sellStops indexed by stop price descending - for finding stops where stopPrice >= lastPrice
	sellStops *btree.BTreeG[*stopEntry]
	// index for O(1) order lookup by ID
	orderIndex map[uint64]*stopEntry
	// count of total orders
	count int
}

// StopBook is an alias for backwards compatibility
type StopBook = IndexedStopBook

// NewStopBook creates a new indexed stop book.
func NewStopBook() *StopBook {
	lessAsc := func(a, b *stopEntry) bool { return a.stopPrice < b.stopPrice }
	lessDesc := func(a, b *stopEntry) bool { return a.stopPrice > b.stopPrice }

	return &IndexedStopBook{
		buyStops:   btree.NewG[*stopEntry](32, lessAsc),
		sellStops:  btree.NewG[*stopEntry](32, lessDesc),
		orderIndex: make(map[uint64]*stopEntry),
	}
}

// Len returns the total number of stop orders.
func (sb *IndexedStopBook) Len() int {
	return sb.count
}

// Append adds a stop order to the book. O(log n)
func (sb *IndexedStopBook) Append(order *core.Order) {
	stopPrice := order.GetStop()
	side := order.GetSide()

	var tree *btree.BTreeG[*stopEntry]
	if side == core.SideBuy {
		tree = sb.buyStops
	} else {
		tree = sb.sellStops
	}

	// Find or create the entry for this stop price
	searchEntry := &stopEntry{stopPrice: stopPrice}
	entry, found := tree.Get(searchEntry)
	if !found {
		entry = &stopEntry{
			stopPrice: stopPrice,
			orders:    make([]*core.Order, 0, 4),
		}
		tree.ReplaceOrInsert(entry)
	}

	entry.orders = append(entry.orders, order)
	sb.orderIndex[order.GetID()] = entry
	sb.count++
}

// Remove removes a specific stop order from the book. O(log n)
func (sb *IndexedStopBook) Remove(order *core.Order) *core.Order {
	entry, exists := sb.orderIndex[order.GetID()]
	if !exists {
		return nil
	}

	// Remove from the entry's order list
	for i, o := range entry.orders {
		if o.GetID() == order.GetID() {
			entry.orders = append(entry.orders[:i], entry.orders[i+1:]...)
			delete(sb.orderIndex, order.GetID())
			sb.count--

			// If entry is now empty, remove it from the tree
			if len(entry.orders) == 0 {
				if order.GetSide() == core.SideBuy {
					sb.buyStops.Delete(entry)
				} else {
					sb.sellStops.Delete(entry)
				}
			}
			return order
		}
	}
	return nil
}

// GetTriggeredBuyStops returns buy stop orders where stopPrice <= lastPrice. O(k log n)
// Buy stops trigger when the market price rises to or above the stop price.
func (sb *IndexedStopBook) GetTriggeredBuyStops(lastPrice uint64) []*core.Order {
	var triggered []*core.Order

	// Iterate through buy stops in ascending order of stop price
	// Stop when we hit a stop price > lastPrice
	sb.buyStops.Ascend(func(entry *stopEntry) bool {
		if entry.stopPrice > lastPrice {
			return false // Stop iterating
		}
		triggered = append(triggered, entry.orders...)
		return true
	})

	return triggered
}

// GetTriggeredSellStops returns sell stop orders where stopPrice >= lastPrice. O(k log n)
// Sell stops trigger when the market price falls to or below the stop price.
func (sb *IndexedStopBook) GetTriggeredSellStops(lastPrice uint64) []*core.Order {
	var triggered []*core.Order

	// Iterate through sell stops in descending order of stop price
	// Stop when we hit a stop price < lastPrice
	sb.sellStops.Descend(func(entry *stopEntry) bool {
		if entry.stopPrice < lastPrice {
			return false // Stop iterating
		}
		triggered = append(triggered, entry.orders...)
		return true
	})

	return triggered
}

// GetTriggeredStops returns all stop orders that should be triggered at the given price.
// This combines both buy stops (stopPrice <= lastPrice) and sell stops (stopPrice >= lastPrice).
func (sb *IndexedStopBook) GetTriggeredStops(lastPrice uint64) []*core.Order {
	buyTriggered := sb.GetTriggeredBuyStops(lastPrice)
	sellTriggered := sb.GetTriggeredSellStops(lastPrice)

	result := make([]*core.Order, 0, len(buyTriggered)+len(sellTriggered))
	result = append(result, buyTriggered...)
	result = append(result, sellTriggered...)
	return result
}

// Iterate iterates through ALL stop orders (for backwards compatibility).
// Prefer GetTriggeredStops for efficient triggered order lookup.
func (sb *IndexedStopBook) Iterate(process func(*core.Order)) {
	sb.buyStops.Ascend(func(entry *stopEntry) bool {
		for _, order := range entry.orders {
			process(order)
		}
		return true
	})
	sb.sellStops.Descend(func(entry *stopEntry) bool {
		for _, order := range entry.orders {
			process(order)
		}
		return true
	})
}

// HasOrder checks if an order exists in the stop book. O(1)
func (sb *IndexedStopBook) HasOrder(orderID uint64) bool {
	_, exists := sb.orderIndex[orderID]
	return exists
}
