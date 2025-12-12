package engine

import (
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/book"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
)

// MatchingEngine represents the Central Limit Order Book matching engine.
// Order priority is determined by Order ID (lower ID = earlier = higher priority).
type MatchingEngine struct {
	OrderBook   *book.OrderBook
	lastPrice   *uint64
	nextOrderID uint64 // Auto-incrementing order ID counter (also determines priority)
}

// NewMatchingEngine creates a new instance of the matching engine.
func NewMatchingEngine() *MatchingEngine {
	return &MatchingEngine{
		OrderBook:   book.NewOrderBook(),
		lastPrice:   nil, // Initialize as nil to indicate no trades have occurred yet
		nextOrderID: 1,   // Start order IDs at 1
	}
}

// ProcessOrder processes a new order. Order ID is auto-generated.
// Order priority is determined by Order ID (lower ID = earlier = higher priority).
func (e *MatchingEngine) ProcessOrder(
	side core.Side,
	orderType core.OrderType,
	quantity, price, stop uint64,
	tif core.TIF,
	oco uint64,
	uid uint64,
) (*core.Done, error) {
	return e.ProcessOrderWithLoader(side, orderType, quantity, price, stop, tif, oco, uid, nil)
}

// ProcessOrderWithLoader processes a new order with optional lazy level loading.
func (e *MatchingEngine) ProcessOrderWithLoader(
	side core.Side,
	orderType core.OrderType,
	quantity, price, stop uint64,
	tif core.TIF,
	oco uint64,
	uid uint64,
	loadNextLevel func() error,
) (*core.Done, error) {
	cfg := e.GetConfig()
	if cfg != nil {
		if quantity < cfg.MinOrderQty {
			return nil, core.ErrMinOrderQty
		}
		if quantity > cfg.MaxOrderQty {
			return nil, core.ErrMaxOrderQty
		}
		if orderType != core.TypeMarket {
			if price < cfg.MinPrice {
				return nil, core.ErrMinPrice
			}
			if price > cfg.MaxPrice {
				return nil, core.ErrMaxPrice
			}
			if cfg.PriceTick > 1 && price%cfg.PriceTick != 0 {
				return nil, core.ErrInvalidPriceTick
			}
		}
		if cfg.QtyStep > 1 && quantity%cfg.QtyStep != 0 {
			return nil, core.ErrInvalidQtyStep
		}
	}

	// Auto-generate order ID (also determines priority - lower ID = earlier)
	orderID := e.nextOrderID
	e.nextOrderID++

	var order *core.Order
	switch orderType {
	case core.TypeMarket:
		order = core.NewMarketOrder(orderID, side, quantity)
	case core.TypeLimit:
		order = core.NewLimitOrder(orderID, side, quantity, price, tif, oco)
	case core.TypeStopLimit:
		order = core.NewStopLimitOrder(orderID, side, quantity, price, stop, oco)
	default:
		return nil, core.ErrInvalidOrderType
	}
	order.SetUID(uid)

	if cfg != nil && orderType == core.TypeMarket && e.OrderBook != nil && cfg.MinNotional > 0 {
		sideBook := e.OrderBook.OppositeSide(order.GetSide())
		best := sideBook.Best()
		if best == nil {
			if loadNextLevel != nil {
				if err := loadNextLevel(); err != nil {
					return nil, err
				}
				best = sideBook.Best()
			}
			if best == nil {
				return nil, core.ErrMinNotional
			}
		}
		if best.GetPrice()*order.GetQuantity() < cfg.MinNotional {
			return nil, core.ErrMinNotional
		}
	}

	// Use progressive matching when loader provided for market orders
	if orderType == core.TypeMarket && loadNextLevel != nil {
		return e.processMarketWithLoader(order, loadNextLevel)
	}

	done, err := e.OrderBook.ProcessWithLoader(order, loadNextLevel)
	if err != nil {
		return nil, err
	}

	if len(done.Trades) > 0 {
		price := done.Trades[0].GetPrice()
		e.lastPrice = &price
	}

	return done, nil
}

func (e *MatchingEngine) processMarketWithLoader(order *core.Order, loadNextLevel func() error) (*core.Done, error) {
	done := core.NewDone(order, 4, 2)
	done, err := e.OrderBook.ProcessMarketOrderProgressive(done, order, loadNextLevel)
	if err != nil {
		return nil, err
	}

	// Update last price if trades occurred
	if len(done.Trades) > 0 {
		price := done.Trades[0].GetPrice()
		e.lastPrice = &price
	}

	// Market orders are never stored - check if unfilled
	if !order.IsFilled() {
		done.Stored = false
	}

	return done, nil
}

// ProcessOrderProgressive processes a market order with progressive level loading.
// Instead of requiring all levels to be loaded upfront, it loads them on-demand
// via the loadNextLevel callback during matching. This reduces I/O for market
// orders that only consume a few levels.
// For non-market orders, use ProcessOrder instead.
func (e *MatchingEngine) ProcessOrderProgressive(
	side core.Side,
	quantity uint64,
	tif core.TIF,
	uid uint64,
	loadNextLevel func() error,
) (*core.Done, error) {
	cfg := e.GetConfig()
	if cfg != nil {
		if quantity < cfg.MinOrderQty {
			return nil, core.ErrMinOrderQty
		}
		if quantity > cfg.MaxOrderQty {
			return nil, core.ErrMaxOrderQty
		}
		if cfg.QtyStep > 1 && quantity%cfg.QtyStep != 0 {
			return nil, core.ErrInvalidQtyStep
		}
	}

	// Auto-generate order ID
	orderID := e.nextOrderID
	e.nextOrderID++

	order := core.NewMarketOrder(orderID, side, quantity)
	order.SetUID(uid)
	order.TIF = tif

	// Min notional check using best price (must have at least first level loaded)
	if cfg := e.GetConfig(); cfg != nil && e.OrderBook != nil && cfg.MinNotional > 0 {
		sideBook := e.OrderBook.OppositeSide(order.GetSide())
		best := sideBook.Best()
		// If no best, try loading first level
		if best == nil {
			if err := loadNextLevel(); err != nil {
				return nil, err
			}
			best = sideBook.Best()
		}
		if best == nil {
			return nil, core.ErrMinNotional
		}
		if best.GetPrice()*order.GetQuantity() < cfg.MinNotional {
			return nil, core.ErrMinNotional
		}
	}

	// Use progressive matching with pre-allocated slices
	done := core.NewDone(order, 4, 2)
	done, err := e.OrderBook.ProcessMarketOrderProgressive(done, order, loadNextLevel)
	if err != nil {
		return nil, err
	}

	// Update last price if trades occurred
	if len(done.Trades) > 0 {
		price := done.Trades[0].GetPrice()
		e.lastPrice = &price
	}

	// Market orders are never stored - check if unfilled
	if !order.IsFilled() {
		done.Stored = false
	}

	return done, nil
}

// CancelOrder cancels an existing order.
func (e *MatchingEngine) CancelOrder(orderID uint64) (*core.Order, error) {
	order, err := e.OrderBook.CancelOrder(orderID)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, core.ErrOrderNotFound
	}
	return order, nil
}

// GetOrder retrieves an order by its ID.
func (e *MatchingEngine) GetOrder(orderID uint64) (*core.Order, error) {
	order := e.OrderBook.GetOrder(orderID)
	if order == nil {
		return nil, core.ErrOrderNotFound
	}
	return order, nil
}

// GetDepth retrieves the order book depth.
func (e *MatchingEngine) GetDepth() *core.Depth {
	return e.OrderBook.Depth()
}

// MatchOrders activates stop orders and matches any crossed limit orders.
// loadNextBidLevel/loadNextAskLevel are optional lazy loaders; pass nil if all levels are already loaded.
func (e *MatchingEngine) MatchOrders(loadNextBidLevel func() error, loadNextAskLevel func() error) ([]*core.Done, error) {
	var (
		dones []*core.Done
		err   error
	)

	if e.OrderBook.Stop.Len() > 0 {
		dones, err = e.activateStopOrders(dones)
		if err != nil {
			return nil, err
		}
	}

	if e.OrderBook.Bids.Len() > 0 && e.OrderBook.Asks.Len() > 0 {
		dones, err = e.matchLimitOrders(dones, loadNextBidLevel, loadNextAskLevel)
		if err != nil {
			return nil, err
		}
	}

	return dones, nil
}

// activateStopOrders activates triggered stop orders by converting them to limit orders.
//
// ATOMICITY: This function ensures stop orders are not lost if limit order processing fails.
// The stop order is only removed from the stop book after the new limit order is successfully
// added to the order book. If Process() fails, the stop order remains in its original state.
func (e *MatchingEngine) activateStopOrders(dones []*core.Done) ([]*core.Done, error) {
	if e.lastPrice == nil {
		return dones, nil // No trades yet, so no stop orders can be activated
	}

	// Use indexed lookup to get only triggered stops in O(k log n) instead of O(s)
	// where k is number of triggered stops and s is total stops
	activated := e.OrderBook.Stop.GetTriggeredStops(*e.lastPrice)

	for _, stopOrder := range activated {
		// Create a new limit order from the stop order.
		// The order keeps its original ID, which determines priority (lower ID = earlier).
		newOrder := core.NewLimitOrder(
			stopOrder.GetID(),
			stopOrder.GetSide(),
			stopOrder.GetQuantity(),
			stopOrder.GetPrice(),
			stopOrder.GetTIF(),
			stopOrder.GetOCO(),
		)
		newOrder.SetUID(stopOrder.GetUID())

		// PHASE 1: Remove stop order from tracking structures ONLY
		// We remove from Orders and index here, but keep in Stop book temporarily
		// This allows Process() to add the order back to Orders/index as a limit order
		delete(e.OrderBook.Orders, stopOrder.GetID())
		delete(e.OrderBook.GetIndex(), stopOrder.GetID())

		// PHASE 2: Process the new limit order
		// Process() will add the order to Orders/index and the appropriate side
		done, err := e.OrderBook.Process(newOrder)
		if err != nil {
			// ROLLBACK: Restore the stop order to tracking structures
			e.OrderBook.Orders[stopOrder.GetID()] = stopOrder
			e.OrderBook.GetIndex()[stopOrder.GetID()] = book.NewLocator(stopOrder)
			// Return error - stop order is still in Stop book and restored to Orders/index
			return nil, err
		}

		// PHASE 3: Process succeeded - now safe to remove from stop book
		e.OrderBook.Stop.Remove(stopOrder)
		dones = append(dones, done)

		if len(done.Trades) > 0 {
			price := done.Trades[len(done.Trades)-1].GetPrice()
			e.lastPrice = &price
		}
	}

	return dones, nil
}

// matchLimitOrders matches crossed limit orders on the book.
//
// ATOMICITY: CancelOrder validates before deleting, so if it fails no state changes.
// Process() failures after CancelOrder succeeds are extremely unlikely since the order
// was already on the book and passed all validations when originally placed.
func (e *MatchingEngine) matchLimitOrders(dones []*core.Done, loadNextBidLevel func() error, loadNextAskLevel func() error) ([]*core.Done, error) {
	loadIfNeeded := func(loader func() error, side *book.OrderSide) error {
		if side.Len() == 0 && loader != nil {
			return loader()
		}
		return nil
	}

	for {
		if err := loadIfNeeded(loadNextBidLevel, e.OrderBook.Bids); err != nil {
			return nil, err
		}
		if err := loadIfNeeded(loadNextAskLevel, e.OrderBook.Asks); err != nil {
			return nil, err
		}

		if e.OrderBook.Bids.Len() == 0 || e.OrderBook.Asks.Len() == 0 {
			break
		}

		if e.OrderBook.Bids.Best().GetPrice() < e.OrderBook.Asks.Best().GetPrice() {
			break
		}

		var taker *core.Order
		bestBid := e.OrderBook.Bids.Best()
		bestAsk := e.OrderBook.Asks.Best()

		// Determine which order is the taker (the one that arrived later = higher order ID)
		if bestBid.GetID() > bestAsk.GetID() {
			taker = bestBid
		} else {
			taker = bestAsk
		}

		// Remove the taker order from the book to process it against the other side
		// CancelOrder validates before deleting, so if it fails, no state was changed
		_, err := e.OrderBook.CancelOrder(taker.GetID())
		if err != nil {
			return nil, err
		}

		// Re-process the taker order to match it
		// Note: Process() failing here is extremely unlikely since:
		// - The order already passed all validations when originally placed
		// - We just removed an order, so max orders check will pass
		// - The order's price level existed, so max price levels check will pass
		done, err := e.OrderBook.Process(taker)
		if err != nil {
			return nil, err
		}
		dones = append(dones, done)

		if len(done.Trades) > 0 {
			price := done.Trades[len(done.Trades)-1].GetPrice()
			e.lastPrice = &price
		}
	}

	return dones, nil
}

// SetConfig sets the configuration for the order book.
func (e *MatchingEngine) SetConfig(cfg *core.Config) {
	if e.OrderBook != nil {
		e.OrderBook.SetConfig(cfg)
	}
}

// GetConfig returns the current configuration.
func (e *MatchingEngine) GetConfig() *core.Config {
	if e.OrderBook != nil {
		return e.OrderBook.GetConfig()
	}
	return core.DefaultConfig()
}

// SetNextOrderID sets the next order ID counter (used during state loading).
func (e *MatchingEngine) SetNextOrderID(id uint64) {
	e.nextOrderID = id
}

// GetNextOrderID returns the next order ID that will be assigned.
func (e *MatchingEngine) GetNextOrderID() uint64 {
	return e.nextOrderID
}

// SetLastPrice sets the last traded price (used during state loading).
func (e *MatchingEngine) SetLastPrice(price *uint64) {
	e.lastPrice = price
}

// GetLastPrice returns the last traded price.
func (e *MatchingEngine) GetLastPrice() *uint64 {
	return e.lastPrice
}
