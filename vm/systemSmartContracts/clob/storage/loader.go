package storage

import (
	"fmt"

	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/book"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/engine"
)

// StateStore abstracts storage access for the CLOB system.
// Implemented by vm.SystemEI or test mocks.
type StateStore interface {
	GetStorage(key []byte) []byte
	SetStorage(key []byte, value []byte)
}

// ============================================================================
// Lazy Loading - Load levels on-demand during matching
// ============================================================================

// LazyLoader provides on-demand loading of price levels during order processing.
// Instead of loading all levels upfront, levels are loaded progressively as needed.
type LazyLoader struct {
	store       StateStore
	pairID      uint32
	eng         *engine.MatchingEngine
	bidPrices   []uint64 // Sorted list of bid prices (not yet loaded)
	askPrices   []uint64 // Sorted list of ask prices (not yet loaded)
	bidIdx      int      // Current index in bidPrices
	askIdx      int      // Current index in askPrices
	deleted     map[uint64]struct{}
	loadedBid   map[uint64]struct{}
	loadedAsk   map[uint64]struct{}
	loadedStops bool
}

// LoadOrderBookLazy loads only config, meta, and price lists without loading any levels.
// Levels are loaded on-demand via the returned LazyLoader.
// This dramatically reduces I/O for operations that don't touch all levels.
func LoadOrderBookLazy(store StateStore, pairID uint32) (*engine.MatchingEngine, *LazyLoader, error) {
	eng := engine.NewMatchingEngine()

	// Load config
	cfgData := store.GetStorage(PairStorageKey(pairID, KeySuffixCfg))
	if len(cfgData) > 0 {
		cfg, err := UnmarshalConfig(cfgData)
		if err != nil {
			return nil, nil, fmt.Errorf("unmarshal config: %w", err)
		}
		eng.SetConfig(cfg)
	}

	// Load meta (nextOrderID, lastPrice)
	metaData := store.GetStorage(PairStorageKey(pairID, KeySuffixMeta))
	if len(metaData) > 0 {
		nextOrderID, lastPrice, err := UnmarshalMeta(metaData)
		if err != nil {
			return nil, nil, fmt.Errorf("unmarshal meta: %w", err)
		}
		eng.SetNextOrderID(nextOrderID)
		eng.SetLastPrice(lastPrice)
	}

	loader := &LazyLoader{
		store:     store,
		pairID:    pairID,
		eng:       eng,
		deleted:   make(map[uint64]struct{}),
		loadedBid: make(map[uint64]struct{}),
		loadedAsk: make(map[uint64]struct{}),
	}

	// Load price lists (small data, needed for lazy iteration)
	bidsData := store.GetStorage(PairStorageKey(pairID, KeySuffixBids))
	if len(bidsData) > 0 {
		prices, err := UnmarshalPriceList(bidsData)
		if err != nil {
			return nil, nil, fmt.Errorf("unmarshal bid prices: %w", err)
		}
		loader.bidPrices = prices
	}

	asksData := store.GetStorage(PairStorageKey(pairID, KeySuffixAsks))
	if len(asksData) > 0 {
		prices, err := UnmarshalPriceList(asksData)
		if err != nil {
			return nil, nil, fmt.Errorf("unmarshal ask prices: %w", err)
		}
		loader.askPrices = prices
	}

	// Load aggregates for FOK optimization (small data)
	if err := loader.loadAggregates(); err != nil {
		return nil, nil, fmt.Errorf("load aggregates: %w", err)
	}

	return eng, loader, nil
}

// loadAggregates loads the aggregate data for both sides.
func (ll *LazyLoader) loadAggregates() error {
	// Load bid aggregates
	bidAggData := ll.store.GetStorage(PairStorageKey(ll.pairID, KeySuffixAggBids))
	if len(bidAggData) >= 28 {
		best, levelCount, totalQty, totalNotional, err := UnmarshalAgg(bidAggData)
		if err == nil {
			ll.eng.OrderBook.Bids.SetBest(best)
			ll.eng.OrderBook.Bids.SetTotalQty(totalQty)
			ll.eng.OrderBook.Bids.SetTotalNotional(totalNotional)
			_ = levelCount // Stored for reference but priceTree tracks actual count
		}
	}

	// Load ask aggregates
	askAggData := ll.store.GetStorage(PairStorageKey(ll.pairID, KeySuffixAggAsks))
	if len(askAggData) >= 28 {
		best, levelCount, totalQty, totalNotional, err := UnmarshalAgg(askAggData)
		if err == nil {
			ll.eng.OrderBook.Asks.SetBest(best)
			ll.eng.OrderBook.Asks.SetTotalQty(totalQty)
			ll.eng.OrderBook.Asks.SetTotalNotional(totalNotional)
			_ = levelCount
		}
	}

	return nil
}

// LoadNextBidLevel loads the next best bid level that hasn't been loaded yet.
// Returns nil if all levels are loaded or if loading fails.
func (ll *LazyLoader) LoadNextBidLevel() error {
	if ll.bidIdx >= len(ll.bidPrices) {
		return nil // All levels loaded
	}

	price := ll.bidPrices[ll.bidIdx]
	ll.bidIdx++
	ll.loadedBid[price] = struct{}{}

	return ll.loadLevel(core.SideBuy, price)
}

// LoadNextAskLevel loads the next best ask level that hasn't been loaded yet.
// Returns nil if all levels are loaded or if loading fails.
func (ll *LazyLoader) LoadNextAskLevel() error {
	if ll.askIdx >= len(ll.askPrices) {
		return nil // All levels loaded
	}

	price := ll.askPrices[ll.askIdx]
	ll.askIdx++
	ll.loadedAsk[price] = struct{}{}

	return ll.loadLevel(core.SideSell, price)
}

// loadLevel loads a single price level from storage using columnar + hot/cold.
func (ll *LazyLoader) loadLevel(side core.Side, price uint64) error {
	var sidePrefix string
	var orderSide *book.OrderSide
	if side == core.SideBuy {
		sidePrefix = KeySuffixLevels + "bids/"
		orderSide = ll.eng.OrderBook.Bids
		ll.loadedBid[price] = struct{}{}
	} else {
		sidePrefix = KeySuffixLevels + "asks/"
		orderSide = ll.eng.OrderBook.Asks
		ll.loadedAsk[price] = struct{}{}
	}

	levelKey := PairStorageKey(ll.pairID, fmt.Sprintf("%s%d", sidePrefix, price))
	levelData := ll.store.GetStorage(levelKey)
	if len(levelData) == 0 {
		return nil // Level doesn't exist (possibly deleted)
	}

	orderIDs, _, err := UnmarshalLevelColumnar(levelData)
	if err != nil {
		return err
	}

	for _, orderID := range orderIDs {
		if _, skip := ll.deleted[orderID]; skip {
			continue
		}
		if _, exists := ll.eng.OrderBook.Orders[orderID]; exists {
			continue
		}

		coldKey := PairStorageKey(ll.pairID, fmt.Sprintf("%s%d", KeySuffixOrderCold, orderID))
		hotKey := PairStorageKey(ll.pairID, fmt.Sprintf("%s%d", KeySuffixOrderHot, orderID))

		coldData := ll.store.GetStorage(coldKey)
		if len(coldData) == 0 {
			continue
		}
		order, errCold := UnmarshalOrderCold(coldData, orderID)
		if errCold != nil {
			return errCold
		}

		hotData := ll.store.GetStorage(hotKey)
		qty, errHot := UnmarshalOrderHot(hotData)
		if errHot != nil {
			return errHot
		}
		order.Quantity = qty

		// Ensure consistency with requested level
		order.Side = side
		order.Price = price

		ll.eng.OrderBook.Orders[order.ID] = order
		orderSide.AppendLoaded(order) // aggregates already set from cached agg
		ll.eng.OrderBook.GetIndex()[order.ID] = book.NewLocator(order)
		ll.eng.OrderBook.IncAccountOrderCount(order.UID)
	}

	// Refresh best after loading
	orderSide.RefreshBest()

	return nil
}

// LoadOrderByID loads the price level containing the given order ID using the locator index.
func (ll *LazyLoader) LoadOrderByID(orderID uint64) error {
	if _, exists := ll.eng.OrderBook.Orders[orderID]; exists {
		return nil
	}

	idxKey := PairStorageKey(ll.pairID, fmt.Sprintf("%s%d", KeySuffixOrderIndex, orderID))
	locData := ll.store.GetStorage(idxKey)
	if len(locData) == 0 {
		// Try stop book as fallback (stop orders do not have level locators)
		if err := ll.LoadStops(); err != nil {
			return err
		}
		if _, exists := ll.eng.OrderBook.Orders[orderID]; exists {
			return nil
		}
		return fmt.Errorf("order %d not found", orderID)
	}

	side, price, err := UnmarshalOrderLocator(locData)
	if err != nil {
		return err
	}

	if err := ll.loadLevel(side, price); err != nil {
		return err
	}

	if _, exists := ll.eng.OrderBook.Orders[orderID]; !exists {
		return fmt.Errorf("order %d missing after loading level %d", orderID, price)
	}
	return nil
}

// CountOrdersForUID counts open orders for a UID without populating the engine.
func (ll *LazyLoader) CountOrdersForUID(uid uint64) (int, error) {
	count := 0

	// helper to scan one side
	scanSide := func(prices []uint64, sidePrefix string) error {
		for _, price := range prices {
			levelKey := PairStorageKey(ll.pairID, fmt.Sprintf("%s%d", sidePrefix, price))
			levelData := ll.store.GetStorage(levelKey)
			if len(levelData) == 0 {
				continue
			}
			orderIDs, _, err := UnmarshalLevelColumnar(levelData)
			if err != nil {
				return err
			}
			for _, id := range orderIDs {
				coldKey := PairStorageKey(ll.pairID, fmt.Sprintf("%s%d", KeySuffixOrderCold, id))
				coldData := ll.store.GetStorage(coldKey)
				if len(coldData) == 0 {
					continue
				}
				order, errCold := UnmarshalOrderCold(coldData, id)
				if errCold != nil {
					return errCold
				}
				if order.GetUID() == uid {
					count++
				}
			}
		}
		return nil
	}

	if err := scanSide(ll.bidPrices, KeySuffixLevels+"bids/"); err != nil {
		return 0, err
	}
	if err := scanSide(ll.askPrices, KeySuffixLevels+"asks/"); err != nil {
		return 0, err
	}

	// Stops
	stopsData := ll.store.GetStorage(PairStorageKey(ll.pairID, KeySuffixStops))
	if len(stopsData) > 0 {
		orders, _, err := UnmarshalStopOrders(stopsData)
		if err != nil {
			return 0, err
		}
		for _, order := range orders {
			if order.GetUID() == uid {
				count++
			}
		}
	}

	return count, nil
}

// LoadStops loads stop orders on-demand.
func (ll *LazyLoader) LoadStops() error {
	stopsData := ll.store.GetStorage(PairStorageKey(ll.pairID, KeySuffixStops))
	if len(stopsData) == 0 {
		return nil
	}

	orders, _, err := UnmarshalStopOrders(stopsData)
	if err != nil {
		return err
	}

	for _, order := range orders {
		if _, skip := ll.deleted[order.ID]; skip {
			continue
		}
		ll.eng.OrderBook.Stop.Append(order)
		ll.eng.OrderBook.Orders[order.ID] = order
		ll.eng.OrderBook.GetIndex()[order.ID] = book.NewLocator(order)
		ll.eng.OrderBook.IncAccountOrderCount(order.UID)
	}
	ll.loadedStops = true
	return nil
}

// MarkDeleted records an order ID that should be ignored when lazily loading.
func (ll *LazyLoader) MarkDeleted(orderID uint64) {
	ll.deleted[orderID] = struct{}{}
}

// LoadAllLevels loads all remaining levels (fallback for operations that need full book).
func (ll *LazyLoader) LoadAllLevels() error {
	// Load remaining bid levels
	for ll.bidIdx < len(ll.bidPrices) {
		if err := ll.LoadNextBidLevel(); err != nil {
			return err
		}
	}

	// Load remaining ask levels
	for ll.askIdx < len(ll.askPrices) {
		if err := ll.LoadNextAskLevel(); err != nil {
			return err
		}
	}

	return nil
}

// HasMoreBidLevels returns true if there are more bid levels to load.
func (ll *LazyLoader) HasMoreBidLevels() bool {
	return ll.bidIdx < len(ll.bidPrices)
}

// HasMoreAskLevels returns true if there are more ask levels to load.
func (ll *LazyLoader) HasMoreAskLevels() bool {
	return ll.askIdx < len(ll.askPrices)
}

// GetBidPriceCount returns the total number of bid price levels.
func (ll *LazyLoader) GetBidPriceCount() int {
	return len(ll.bidPrices)
}

// GetAskPriceCount returns the total number of ask price levels.
func (ll *LazyLoader) GetAskPriceCount() int {
	return len(ll.askPrices)
}
