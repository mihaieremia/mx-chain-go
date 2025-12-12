package book

import (
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
)

// OrderBook represents the order book for a single market.
type OrderBook struct {
	Bids     *OrderSide
	Asks     *OrderSide
	Stop     *StopBook
	Orders   map[uint64]*core.Order
	index    map[uint64]*OrderLocator
	cfg      *core.Config
	accounts map[uint64]int // UID → order count
}

// NewOrderBook creates a new instance of the OrderBook.
func NewOrderBook() *OrderBook {
	return &OrderBook{
		Bids:     NewOrderSide(core.SideBuy),
		Asks:     NewOrderSide(core.SideSell),
		Stop:     NewStopBook(),
		Orders:   make(map[uint64]*core.Order),
		index:    make(map[uint64]*OrderLocator),
		cfg:      core.DefaultConfig(),
		accounts: make(map[uint64]int),
	}
}

// Process processes a new order.
func (ob *OrderBook) Process(order *core.Order) (*core.Done, error) {
	return ob.ProcessWithLoader(order, nil)
}

// ProcessWithLoader processes a new order using an optional lazy loader callback.
func (ob *OrderBook) ProcessWithLoader(order *core.Order, loadNextLevel func() error) (*core.Done, error) {
	if len(ob.Orders) >= ob.cfg.MaxOrders {
		return nil, core.ErrMaxOrdersReached
	}
	if order.GetUID() != 0 && ob.accounts[order.GetUID()] >= ob.cfg.MaxOrdersPerAccount {
		return nil, core.ErrMaxOrdersAccount
	}
	if order.IsStopOrder() {
		ob.Stop.Append(order)
		ob.Orders[order.GetID()] = order
		ob.index[order.GetID()] = NewLocator(order)
		ob.incAccount(order.GetUID())
		return &core.Done{Order: order, Stored: true, Processed: 0}, nil
	}

	done, err := ob.processOrder(order, loadNextLevel)
	if err != nil {
		return nil, err
	}

	if !order.IsFilled() {
		if order.GetType() != core.TypeMarket {
			if err := ob.appendLimitOrder(order); err != nil {
				return nil, err
			}
			done.Stored = true
		}
	} else {
		// Taker order was completely filled during matching
		// Note: A filled taker was never added to the book, so these deletes are no-ops.
		// We still call them for correctness if the order ID somehow existed.
		done.Stored = false

		// ATOMICITY: Cancel taker's OCO order FIRST (before any state mutation)
		// If OCO cancellation fails, we return error without having modified any state.
		// CancelOrderInternal validates before deleting (two-phase pattern).
		ocoID := order.GetOCO()
		if ocoID != 0 {
			ocoOrder, err := ob.CancelOrderInternal(ocoID)
			if err != nil {
				return nil, err
			}
			if ocoOrder != nil {
				done.Canceled = append(done.Canceled, formatOrderID(ocoID))
				done.CanceledOrders = append(done.CanceledOrders, ocoOrder)
			}
		}

		// Safe to clean up taker state (these are typically no-ops for filled takers)
		delete(ob.Orders, order.GetID())
		delete(ob.index, order.GetID())
		ob.decAccount(order.GetUID())
	}

	return done, nil
}

// CancelOrder cancels an existing order with O(1) complexity.
// This method requires that the order's locator was properly set during insertion.
// For stop orders, removal uses the StopBook's mechanism.
// For limit orders, this uses the order's embedded prev/next pointers for O(1) list removal.
// Also cancels any linked OCO order.
// Returns (nil, nil) if order not found, (order, nil) on success, or (nil, error) on invariant violation.
func (ob *OrderBook) CancelOrder(orderID uint64) (*core.Order, error) {
	return ob.CancelOrderWithOCO(orderID, nil)
}

// CancelOrderWithOCO cancels an order and optionally records OCO cancellations in done.
// Returns (nil, nil) if order not found, (order, nil) on success, or (nil, error) on invariant violation.
func (ob *OrderBook) CancelOrderWithOCO(orderID uint64, done *core.Done) (*core.Order, error) {
	order, err := ob.CancelOrderInternal(orderID)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, nil
	}

	// Cancel linked OCO order if exists
	ocoID := order.GetOCO()
	if ocoID != 0 {
		ocoOrder, err := ob.CancelOrderInternal(ocoID)
		if err != nil {
			return nil, err
		}
		if ocoOrder != nil && done != nil {
			done.Canceled = append(done.Canceled, formatOrderID(ocoID))
			done.CanceledOrders = append(done.CanceledOrders, ocoOrder)
		}
	}

	return order, nil
}

// GetOrder retrieves an order by its ID.
func (ob *OrderBook) GetOrder(orderID uint64) *core.Order {
	return ob.Orders[orderID]
}

// Depth returns the order book depth.
func (ob *OrderBook) Depth() *core.Depth {
	depth := core.NewDepth()
	bids := ob.Bids.Depth()
	asks := ob.Asks.Depth()

	for _, level := range bids {
		depth.Bids = append(depth.Bids, [2]uint64{level.Price, level.Quantity})
	}
	for _, level := range asks {
		depth.Asks = append(depth.Asks, [2]uint64{level.Price, level.Quantity})
	}

	return depth
}

func (ob *OrderBook) processOrder(order *core.Order, loadNextLevel func() error) (*core.Done, error) {
	done := &core.Done{Order: order, Processed: 0}
	var err error

	if order.GetType() == core.TypeMarket {
		done, err = ob.processMarketOrder(done, order)
	} else {
		done, err = ob.processLimitOrder(done, order, loadNextLevel)
	}

	return done, err
}

func (ob *OrderBook) processLimitOrder(done *core.Done, order *core.Order, loadNextLevel func() error) (*core.Done, error) {
	s := ob.OppositeSide(order.GetSide())
	originalQty := order.GetQuantity()

	// FOK: Check if order can be completely filled before matching
	if order.GetTIF() == core.TIFFillOrKill {
		if loadNextLevel != nil && s.Len() == 0 {
			_ = loadNextLevel()
		}
		available := ob.calculateAvailableLiquidity(order)
		if available < originalQty {
			return done, core.ErrFOKNotFilled
		}
	}

	// Post-Only: Reject if order would match immediately (must add liquidity only)
	if order.GetTIF() == core.TIFPostOnly {
		if s.Len() == 0 && loadNextLevel != nil {
			if err := loadNextLevel(); err != nil {
				return nil, err
			}
		}
		if s.Len() > 0 {
			bestPrice := s.Best().GetPrice()
			wouldMatch := (order.GetSide() == core.SideBuy && order.GetPrice() >= bestPrice) ||
				(order.GetSide() == core.SideSell && order.GetPrice() <= bestPrice)
			if wouldMatch {
				return done, core.ErrPostOnlyWouldMatch
			}
		}
	}

	quantity := order.GetQuantity()
	for quantity > 0 {
		if s.Len() == 0 && loadNextLevel != nil {
			if err := loadNextLevel(); err != nil {
				return nil, err
			}
		}
		if s.Len() == 0 {
			break
		}

		best := s.Best()
		if best == nil && loadNextLevel != nil {
			if err := loadNextLevel(); err != nil {
				return nil, err
			}
			best = s.Best()
		}
		if best == nil {
			break
		}

		bestPrice := best.GetPrice()
		if order.GetSide() == core.SideBuy && order.GetPrice() < bestPrice {
			break
		}
		if order.GetSide() == core.SideSell && order.GetPrice() > bestPrice {
			break
		}

		if err := ob.match(order, best, done); err != nil {
			return nil, err
		}
		quantity = order.GetQuantity()

		if s.Len() == 0 && quantity > 0 && loadNextLevel != nil {
			if err := loadNextLevel(); err != nil {
				return nil, err
			}
		}
	}

	// IOC: Cancel any unfilled portion (set qty to 0 so it won't be stored)
	if order.GetTIF() == core.TIFImmediateOrCancel && !order.IsFilled() {
		done.Left = order.GetQuantity()
		order.Quantity = 0 // Mark as fully processed so it won't be stored
	}

	// FOK: enforce full fill after matching
	if order.GetTIF() == core.TIFFillOrKill && !order.IsFilled() {
		done.Left = order.GetQuantity()
		order.Quantity = 0
		return done, core.ErrFOKNotFilled
	}

	return done, nil
}

func (ob *OrderBook) processMarketOrder(done *core.Done, order *core.Order) (*core.Done, error) {
	s := ob.OppositeSide(order.GetSide())
	originalQty := order.GetQuantity()

	// FOK: Check if order can be completely filled before matching
	if order.GetTIF() == core.TIFFillOrKill {
		// For market orders, check total available liquidity on opposite side
		if s.GetTotalQty() < originalQty {
			return done, core.ErrFOKNotFilled
		}
	}

	quantity := order.GetQuantity()
	for s.Len() > 0 && quantity > 0 {
		best := s.Best()
		if err := ob.match(order, best, done); err != nil {
			return nil, err
		}
		quantity = order.GetQuantity()
	}

	// IOC: Cancel any unfilled portion (market order unfilled means no more liquidity)
	if order.GetTIF() == core.TIFImmediateOrCancel && !order.IsFilled() {
		done.Left = order.GetQuantity()
		order.Quantity = 0
	}

	return done, nil
}

// ProcessMarketOrderProgressive processes market order with progressive level loading.
// Instead of loading all levels upfront, it loads levels on-demand during matching.
// The loadNextLevel callback should load the next best price level and return nil,
// or return an error if loading fails.
func (ob *OrderBook) ProcessMarketOrderProgressive(done *core.Done, order *core.Order, loadNextLevel func() error) (*core.Done, error) {
	s := ob.OppositeSide(order.GetSide())

	// FOK: Check using preloaded TotalQty aggregate (no level loading needed)
	if order.GetTIF() == core.TIFFillOrKill {
		if s.GetTotalQty() < order.GetQuantity() {
			return done, core.ErrFOKNotFilled
		}
	}

	for order.GetQuantity() > 0 {
		best := s.Best()

		// If no best order available, try loading the next level
		if best == nil {
			if err := loadNextLevel(); err != nil {
				return nil, err
			}
			best = s.Best()
			if best == nil {
				break // No more liquidity even after loading
			}
		}

		if err := ob.match(order, best, done); err != nil {
			return nil, err
		}

		// After matching, if current level is exhausted, load next
		if s.Best() == nil && order.GetQuantity() > 0 {
			if err := loadNextLevel(); err != nil {
				return nil, err
			}
		}
	}

	// IOC: Cancel any unfilled portion
	if order.GetTIF() == core.TIFImmediateOrCancel && !order.IsFilled() {
		done.Left = order.GetQuantity()
		order.Quantity = 0
	}

	return done, nil
}

// match executes a trade between taker and maker orders.
//
// ATOMICITY: Order removal (if maker is filled) is attempted BEFORE updating side totals.
// This ensures totals remain consistent if removal fails due to invariant violations.
// Note: With the fixes to removeOrder and CancelOrderInternal, failures are only possible
// if there's data corruption (which would be detected by the validation checks).
func (ob *OrderBook) match(taker, maker *core.Order, done *core.Done) error {
	tradeQuantity := taker.GetQuantity()
	if maker.GetQuantity() < tradeQuantity {
		tradeQuantity = maker.GetQuantity()
	}

	// Save state for potential rollback
	makerPrice := maker.GetPrice()
	side := ob.sideByOrder(maker)

	// Fill both orders - errors should not happen here since tradeQuantity
	// is already capped to the minimum of both quantities
	_ = taker.Fill(tradeQuantity)
	_ = maker.Fill(tradeQuantity)

	// If maker is completely filled, remove it from the book BEFORE updating totals
	// This ensures side totals are not corrupted if removal fails
	if maker.IsFilled() {
		if err := ob.removeOrderWithOCO(maker, done); err != nil {
			// Rollback the fills
			taker.Quantity += tradeQuantity
			maker.Quantity += tradeQuantity
			return err
		}
	}

	// Record trades with full info for balance updates.
	// Trades are recorded in pairs: [taker, maker]
	// Include Side and UID so we can properly update account balances.
	done.Trades = append(done.Trades, &core.Order{
		ID:       taker.GetID(),
		Side:     taker.GetSide(),
		Price:    makerPrice, // Trade price is maker's price
		Quantity: tradeQuantity,
		UID:      taker.GetUID(),
	}, &core.Order{
		ID:       maker.GetID(),
		Side:     maker.GetSide(),
		Price:    makerPrice,
		Quantity: tradeQuantity,
		UID:      maker.GetUID(), // Include maker UID for balance updates
	})

	done.Processed += tradeQuantity

	// Update cached totals for the maker side AFTER successful removal
	// This ensures totals are accurate
	side.OnFill(makerPrice, tradeQuantity)

	return nil
}

func (ob *OrderBook) sideByOrder(order *core.Order) *OrderSide {
	if order.GetSide() == core.SideBuy {
		return ob.Bids
	}
	return ob.Asks
}

func (ob *OrderBook) appendLimitOrder(order *core.Order) error {
	side := ob.sideByOrder(order)
	if _, exists := side.GetPriceLevels()[order.GetPrice()]; !exists && side.Len() >= ob.cfg.MaxPriceLevelsPerSide {
		return core.ErrMaxPriceLevels
	}
	ob.Orders[order.GetID()] = order
	side.Append(order)
	ob.index[order.GetID()] = NewLocator(order)
	ob.incAccount(order.GetUID())
	return nil
}

// removeOrder removes a filled order with O(1) complexity.
// This is called internally when an order is completely filled during matching.
// The locator must exist for all orders in the book.
// Returns an error if invariants are violated (indicates data corruption).
//
// ATOMICITY: This method validates ALL invariants BEFORE any state mutation.
// This ensures no partial state changes occur on validation failure.
func (ob *OrderBook) removeOrder(order *core.Order) error {
	loc := ob.index[order.GetID()]

	// PHASE 1: Validate ALL invariants BEFORE any deletion
	// The locator must exist - guaranteed by appendLimitOrder which always stores it.
	if loc == nil {
		return core.ErrLocatorNil
	}

	side := ob.sideByOrder(order)
	queue := side.GetPriceLevels()[loc.Price]
	if queue == nil {
		return core.ErrQueueNotFound
	}

	// PHASE 2: All validations passed - safe to delete
	delete(ob.Orders, order.GetID())
	delete(ob.index, order.GetID())

	removed := side.RemoveByOrder(loc.Price, queue, order)
	if removed != nil {
		ob.decAccount(order.GetUID())
	}
	return nil
}

// removeOrderWithOCO removes a filled order and its linked OCO order if any.
// Records cancelled OCO orders in done.Canceled for balance unlocking.
// Returns error if internal invariants are violated.
func (ob *OrderBook) removeOrderWithOCO(order *core.Order, done *core.Done) error {
	if err := ob.removeOrder(order); err != nil {
		return err
	}

	// Cancel linked OCO order if exists
	ocoID := order.GetOCO()
	if ocoID != 0 {
		ocoOrder, err := ob.CancelOrderInternal(ocoID)
		if err != nil {
			return err
		}
		if ocoOrder != nil {
			// Record the cancelled OCO order ID for balance unlocking
			done.Canceled = append(done.Canceled, formatOrderID(ocoID))
			done.CanceledOrders = append(done.CanceledOrders, ocoOrder)
		}
	}
	return nil
}

// CancelOrderInternal cancels an order without cascading OCO cancellation.
// Used internally to avoid infinite loops when both OCO orders try to cancel each other.
// Returns (nil, nil) if order not found, (order, nil) on success, or (nil, error) on invariant violation.
//
// ATOMICITY: This method validates ALL invariants BEFORE any state mutation.
// This ensures no partial state changes occur on validation failure.
func (ob *OrderBook) CancelOrderInternal(orderID uint64) (*core.Order, error) {
	order, ok := ob.Orders[orderID]
	if !ok {
		return nil, nil
	}

	loc := ob.index[orderID]

	// PHASE 1: Validate ALL invariants BEFORE any deletion
	// This prevents state corruption if validation fails
	var side *OrderSide
	var queue *OrderQueue
	if !order.IsStopOrder() {
		if loc == nil {
			return nil, core.ErrLocatorNil
		}
		side = ob.sideByOrder(order)
		queue = side.GetPriceLevels()[loc.Price]
		if queue == nil {
			return nil, core.ErrQueueNotFound
		}
	}

	// PHASE 2: All validations passed - safe to delete
	// These operations cannot fail after validation
	delete(ob.Orders, orderID)
	delete(ob.index, orderID)

	if order.IsStopOrder() {
		ob.decAccount(order.GetUID())
		return ob.Stop.Remove(order), nil
	}

	// For limit orders, use pre-validated side and queue
	removed := side.RemoveByOrder(loc.Price, queue, order)
	if removed != nil {
		ob.decAccount(order.GetUID())
	}
	return removed, nil
}

// formatOrderID converts order ID to string for the Canceled slice.
func formatOrderID(id uint64) string {
	// Simple integer to string conversion
	if id == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for id > 0 {
		i--
		buf[i] = byte('0' + id%10)
		id /= 10
	}
	return string(buf[i:])
}

// IncAccountOrderCount increments the order count for an account.
func (ob *OrderBook) IncAccountOrderCount(uid uint64) {
	if uid == 0 {
		return
	}
	ob.accounts[uid]++
}

func (ob *OrderBook) incAccount(uid uint64) {
	ob.IncAccountOrderCount(uid)
}

func (ob *OrderBook) decAccount(uid uint64) {
	if uid == 0 {
		return
	}
	if ob.accounts[uid] > 0 {
		ob.accounts[uid]--
		if ob.accounts[uid] == 0 {
			delete(ob.accounts, uid)
		}
	}
}

// OppositeSide returns the opposite side of the order book.
func (ob *OrderBook) OppositeSide(side core.Side) *OrderSide {
	if side == core.SideBuy {
		return ob.Asks
	}
	return ob.Bids
}

// calculateAvailableLiquidity calculates total quantity available at or better than order's price.
// For buy orders: sum of ask quantities at prices <= order price
// For sell orders: sum of bid quantities at prices >= order price
func (ob *OrderBook) calculateAvailableLiquidity(order *core.Order) uint64 {
	s := ob.OppositeSide(order.GetSide())
	orderPrice := order.GetPrice()
	var totalQty uint64

	s.Iterate(func(o *core.Order) bool {
		if order.GetSide() == core.SideBuy {
			// Buy order can match asks with price <= order price
			if o.GetPrice() > orderPrice {
				return false // No more matching levels
			}
		} else {
			// Sell order can match bids with price >= order price
			if o.GetPrice() < orderPrice {
				return false // No more matching levels
			}
		}
		totalQty += o.GetQuantity()
		return true
	})

	return totalQty
}

// GetConfig returns the configuration for this order book.
func (ob *OrderBook) GetConfig() *core.Config {
	return ob.cfg
}

// SetConfig sets the configuration for this order book.
func (ob *OrderBook) SetConfig(cfg *core.Config) {
	if cfg != nil {
		ob.cfg = cfg
	}
}

// GetIndex returns the order locator index.
func (ob *OrderBook) GetIndex() map[uint64]*OrderLocator {
	return ob.index
}

// GetAccounts returns the account order count map.
func (ob *OrderBook) GetAccounts() map[uint64]int {
	return ob.accounts
}

// SimulatedFill represents a potential fill from market order simulation.
// This is used to pre-calculate costs without mutating state.
type SimulatedFill struct {
	Price    uint64
	Quantity uint64
	MakerUID uint64
}

// SimulateMarketOrder calculates what fills would occur for a market order
// WITHOUT mutating any state. Used to pre-validate taker balance before execution.
// Returns fills, total filled quantity, and error if FOK cannot be satisfied.
func (ob *OrderBook) SimulateMarketOrder(side core.Side, quantity uint64, tif core.TIF) ([]SimulatedFill, uint64, error) {
	s := ob.OppositeSide(side)

	// FOK: Check if order can be completely filled
	if tif == core.TIFFillOrKill {
		if s.GetTotalQty() < quantity {
			return nil, 0, core.ErrFOKNotFilled
		}
	}

	var fills []SimulatedFill
	var totalFilled uint64
	remaining := quantity

	// Walk the opposite side without modifying anything
	s.Iterate(func(o *core.Order) bool {
		if remaining == 0 {
			return false // Done
		}

		fillQty := o.GetQuantity()
		if fillQty > remaining {
			fillQty = remaining
		}

		fills = append(fills, SimulatedFill{
			Price:    o.GetPrice(),
			Quantity: fillQty,
			MakerUID: o.GetUID(),
		})

		totalFilled += fillQty
		remaining -= fillQty
		return true // Continue
	})

	return fills, totalFilled, nil
}
