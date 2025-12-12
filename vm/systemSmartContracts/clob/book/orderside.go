package book

import (
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
)

// OrderSide represents one side of the order book (bids or asks).
// Uses a B-tree for O(log n) price level management instead of sorted slice.
type OrderSide struct {
	side          core.Side
	priceTree     *PriceTree             // O(log n) price operations
	priceLevels   map[uint64]*OrderQueue // O(1) queue lookup by price
	numOrders     int
	totalNotional uint64
	totalQty      uint64
	best          uint64 // cached best price, 0 means empty
}

// NewOrderSide creates a new instance of the OrderSide.
func NewOrderSide(side core.Side) *OrderSide {
	return &OrderSide{
		side:          side,
		priceTree:     NewPriceTree(side == core.SideBuy), // descending for bids
		priceLevels:   make(map[uint64]*OrderQueue),
		totalNotional: 0,
		totalQty:      0,
		best:          0,
	}
}

// Len returns the number of price levels.
func (os *OrderSide) Len() int {
	return os.priceTree.Len()
}

// Best returns the best order (highest bid or lowest ask).
func (os *OrderSide) Best() *core.Order {
	if os.best == 0 {
		return nil
	}
	level := os.priceLevels[os.best]
	if level == nil || level.Len() == 0 {
		// Cache is stale - refresh and try again
		os.refreshBest()
		if os.best == 0 {
			return nil
		}
		level = os.priceLevels[os.best]
		if level == nil || level.Len() == 0 {
			return nil
		}
	}
	return level.Front()
}

// Append appends a new order to the side. O(log n) for new price levels, O(1) for existing.
func (os *OrderSide) Append(order *core.Order) {
	price := order.GetPrice()
	level, ok := os.priceLevels[price]
	if !ok {
		level = NewOrderQueue(price)
		os.priceLevels[price] = level
		os.priceTree.Insert(price) // O(log n)
	}
	level.PushBack(order)
	os.numOrders++
	// Use safe arithmetic with saturation for aggregates
	notional, err := core.SafeMul(order.GetQuantity(), order.GetPrice())
	if err == nil {
		newNotional, err := core.SafeAdd(os.totalNotional, notional)
		if err == nil {
			os.totalNotional = newNotional
		}
		// On overflow, keep existing totalNotional (saturate)
	}
	newQty, err := core.SafeAdd(os.totalQty, order.GetQuantity())
	if err == nil {
		os.totalQty = newQty
	}
	// On overflow, keep existing totalQty (saturate)

	// Update best price if needed
	os.updateBestOnInsert(price)
}

// AppendLoaded appends an order loaded from storage without updating aggregates.
// Use this when loading from storage where aggregates are already set from persisted values.
// O(log n) for new price levels, O(1) for existing.
func (os *OrderSide) AppendLoaded(order *core.Order) {
	price := order.GetPrice()
	level, ok := os.priceLevels[price]
	if !ok {
		level = NewOrderQueue(price)
		os.priceLevels[price] = level
		os.priceTree.Insert(price) // O(log n)
	}
	level.PushBack(order)
	os.numOrders++
	// Note: Do NOT update totalNotional/totalQty - they're already set from storage
}

// updateBestOnInsert updates the cached best price after insertion.
func (os *OrderSide) updateBestOnInsert(price uint64) {
	if os.best == 0 {
		os.best = price
		return
	}
	if os.side == core.SideBuy && price > os.best {
		os.best = price
	} else if os.side == core.SideSell && price < os.best {
		os.best = price
	}
}

// RemoveByOrder removes an order from the side using the order's intrusive pointers.
// This guarantees O(1) removal complexity for the order itself.
// Price level cleanup is O(log n) if the level becomes empty.
func (os *OrderSide) RemoveByOrder(price uint64, level *OrderQueue, order *core.Order) *core.Order {
	if level == nil || order == nil {
		return nil
	}

	removed := level.RemoveOrder(order)
	if removed != nil {
		os.numOrders--
		// Safe subtraction with floor at 0
		notional, err := core.SafeMul(removed.GetQuantity(), removed.GetPrice())
		if err == nil {
			if notional >= os.totalNotional {
				os.totalNotional = 0
			} else {
				os.totalNotional -= notional
			}
		}
		if removed.GetQuantity() >= os.totalQty {
			os.totalQty = 0
		} else {
			os.totalQty -= removed.GetQuantity()
		}
		os.cleanupLevelIfEmpty(price, level)
	}
	return removed
}

// OnFill updates cached aggregates for a partial/complete fill.
func (os *OrderSide) OnFill(price uint64, qty uint64) {
	level, ok := os.priceLevels[price]
	if !ok {
		return
	}
	level.ApplyFill(qty)
	// Safe subtraction with floor at 0
	if qty >= os.totalQty {
		os.totalQty = 0
	} else {
		os.totalQty -= qty
	}
	// Use safe multiplication for notional
	notional, err := core.SafeMul(qty, price)
	if err == nil {
		if notional >= os.totalNotional {
			os.totalNotional = 0
		} else {
			os.totalNotional -= notional
		}
	} else {
		// Multiplication overflowed, just zero out (conservative)
		os.totalNotional = 0
	}
}

// cleanupLevelIfEmpty removes an empty price level. O(log n)
func (os *OrderSide) cleanupLevelIfEmpty(price uint64, level *OrderQueue) {
	if level.Len() == 0 {
		delete(os.priceLevels, price)
		os.priceTree.Delete(price) // O(log n)
		os.refreshBest()
	}
}

// CleanupLevelIfEmpty is the public version for external callers.
func (os *OrderSide) CleanupLevelIfEmpty(price uint64, level *OrderQueue) {
	os.cleanupLevelIfEmpty(price, level)
}

// refreshBest recalculates the best price from the tree. O(log n)
func (os *OrderSide) refreshBest() {
	best, ok := os.priceTree.Best()
	if !ok {
		os.best = 0
		return
	}
	os.best = best
}

// RefreshBest recalculates the best price from the tree. O(log n)
// Public version for use after lazy loading levels.
func (os *OrderSide) RefreshBest() {
	os.refreshBest()
}

// Depth returns the depth of the side.
func (os *OrderSide) Depth() []struct {
	Price    uint64
	Quantity uint64
} {
	levels := make([]struct {
		Price    uint64
		Quantity uint64
	}, 0, os.priceTree.Len())

	os.priceTree.Iterate(func(price uint64) bool {
		level, ok := os.priceLevels[price]
		if !ok {
			return true
		}
		levels = append(levels, struct {
			Price    uint64
			Quantity uint64
		}{Price: price, Quantity: level.totalQty})
		return true
	})
	return levels
}

// GetPriceLevels returns the price levels map.
func (os *OrderSide) GetPriceLevels() map[uint64]*OrderQueue {
	return os.priceLevels
}

// GetNumOrders returns the number of orders in the side.
func (os *OrderSide) GetNumOrders() int {
	return os.numOrders
}

// GetTotalNotional returns the total notional value.
func (os *OrderSide) GetTotalNotional() uint64 {
	return os.totalNotional
}

// SetTotalNotional sets the total notional value.
func (os *OrderSide) SetTotalNotional(totalNotional uint64) {
	os.totalNotional = totalNotional
}

// GetTotalQty returns the total quantity.
func (os *OrderSide) GetTotalQty() uint64 {
	return os.totalQty
}

// SetTotalQty sets the total quantity.
func (os *OrderSide) SetTotalQty(totalQty uint64) {
	os.totalQty = totalQty
}

// GetPrices returns all price levels in order (best to worst).
func (os *OrderSide) GetPrices() []uint64 {
	return os.priceTree.ToSlice()
}

// GetBest returns the best price (highest for bids, lowest for asks).
func (os *OrderSide) GetBest() uint64 {
	return os.best
}

// SetBest sets the best price.
func (os *OrderSide) SetBest(best uint64) {
	os.best = best
}

// Iterate calls the provided function for each order in price-time priority.
func (os *OrderSide) Iterate(fn func(*core.Order) bool) {
	os.priceTree.Iterate(func(price uint64) bool {
		level, ok := os.priceLevels[price]
		if !ok {
			return true
		}
		shouldContinue := true
		level.Iterate(func(order *core.Order) bool {
			shouldContinue = fn(order)
			return shouldContinue
		})
		return shouldContinue
	})
}

// IterateLevels calls the provided function for each price level in priority order.
// This is more efficient than Iterate when you only need aggregate data per level.
// The callback receives the price and total quantity at that level.
// Return false to stop iteration early.
func (os *OrderSide) IterateLevels(fn func(price uint64, totalQty uint64) bool) {
	os.priceTree.Iterate(func(price uint64) bool {
		level, ok := os.priceLevels[price]
		if !ok {
			return true
		}
		return fn(price, level.GetTotalQty())
	})
}

// TopNLevels is the number of top levels to cache for FOK optimization
const TopNLevels = 50

// GetTopNLevels returns the top N price levels (best first) with their quantities.
// Used to build the aggregate cache for FOK optimization.
// Returns: prices slice, quantities slice, count of valid entries.
func (os *OrderSide) GetTopNLevels() (prices [TopNLevels]uint64, quantities [TopNLevels]uint64, count uint8) {
	os.priceTree.Iterate(func(price uint64) bool {
		level, ok := os.priceLevels[price]
		if !ok {
			return true
		}
		if count < TopNLevels {
			prices[count] = price
			quantities[count] = level.GetTotalQty()
			count++
		}
		return count < TopNLevels // Stop after TopNLevels
	})
	return
}

// CalculateTopNSum returns the sum of quantities in the top N levels.
// Used for quick FOK validation without loading individual order data.
func (os *OrderSide) CalculateTopNSum(n int) uint64 {
	if n > TopNLevels {
		n = TopNLevels
	}
	var sum uint64
	count := 0
	os.priceTree.Iterate(func(price uint64) bool {
		level, ok := os.priceLevels[price]
		if !ok {
			return true
		}
		sum += level.GetTotalQty()
		count++
		return count < n
	})
	return sum
}
