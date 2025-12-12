package storage

import (
	"fmt"
	"sort"

	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/book"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/engine"
)

// SaveOrderBookWithLoader saves using lazy loader context without forcing full loads.
// If loader is nil, falls back to SaveOrderBook.
func SaveOrderBookWithLoader(store StateStore, pairID uint32, eng *engine.MatchingEngine, loader *LazyLoader) {
	if loader == nil {
		SaveOrderBook(store, pairID, eng)
		return
	}

	// Save config
	cfgData, _ := MarshalConfig(eng.GetConfig())
	store.SetStorage(PairStorageKey(pairID, KeySuffixCfg), cfgData)

	// Save meta
	metaData, _ := MarshalMeta(eng.GetNextOrderID(), eng.GetLastPrice())
	store.SetStorage(PairStorageKey(pairID, KeySuffixMeta), metaData)

	// Save sides using lazy context
	saveSideWithLoader(store, pairID, eng, core.SideBuy, loader.bidPrices, loader.loadedBid, loader.deleted)
	saveSideWithLoader(store, pairID, eng, core.SideSell, loader.askPrices, loader.loadedAsk, loader.deleted)

	// Stops: save if loaded or currently present; otherwise leave untouched
	if loader.loadedStops || eng.OrderBook.Stop.Len() > 0 {
		saveStopOrders(store, pairID, eng)
	}

	// Aggregates
	SaveAggregates(store, pairID, eng)
}

// SaveOrderBook saves a complete order book to storage.
func SaveOrderBook(store StateStore, pairID uint32, eng *engine.MatchingEngine) {
	// Save config
	cfgData, _ := MarshalConfig(eng.GetConfig())
	store.SetStorage(PairStorageKey(pairID, KeySuffixCfg), cfgData)

	// Save meta
	metaData, _ := MarshalMeta(eng.GetNextOrderID(), eng.GetLastPrice())
	store.SetStorage(PairStorageKey(pairID, KeySuffixMeta), metaData)

	// Save bids
	saveSide(store, pairID, eng, core.SideBuy)

	// Save asks
	saveSide(store, pairID, eng, core.SideSell)

	// Save stop orders
	saveStopOrders(store, pairID, eng)

	// Save aggregates for lazy loading paths (top levels + totals cache)
	SaveAggregates(store, pairID, eng)
}

// saveSide saves one side of the order book to storage.
func saveSide(store StateStore, pairID uint32, eng *engine.MatchingEngine, side core.Side) {
	var orderSide *book.OrderSide
	var sidePrefix string
	var sideSuffix string
	if side == core.SideBuy {
		orderSide = eng.OrderBook.Bids
		sidePrefix = KeySuffixLevels + "bids/"
		sideSuffix = KeySuffixBids
	} else {
		orderSide = eng.OrderBook.Asks
		sidePrefix = KeySuffixLevels + "asks/"
		sideSuffix = KeySuffixAsks
	}

	priceLevels := orderSide.GetPriceLevels()
	prices := orderSide.GetPrices()

	for _, price := range prices {
		queue := priceLevels[price]
		if queue == nil || queue.Len() == 0 {
			// Delete empty level
			levelKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", sidePrefix, price))
			store.SetStorage(levelKey, nil)
			continue
		}

		// Collect orders at this level preserving FIFO
		orders := make([]*core.Order, 0, queue.Len())
		queue.Iterate(func(o *core.Order) bool {
			orders = append(orders, o)
			return true
		})

		orderIDs := make([]uint64, len(orders))
		for i, order := range orders {
			orderIDs[i] = order.ID

			// Persist hot/cold split
			coldKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", KeySuffixOrderCold, order.ID))
			hotKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", KeySuffixOrderHot, order.ID))
			coldData, _ := MarshalOrderCold(order)
			hotData := MarshalOrderHot(order.Quantity)
			store.SetStorage(coldKey, coldData)
			store.SetStorage(hotKey, hotData)

			// Persist O(1) order locator index
			idxKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", KeySuffixOrderIndex, order.ID))
			store.SetStorage(idxKey, MarshalOrderLocator(order.Side, order.Price))
		}

		levelData, _ := MarshalLevelColumnar(orderIDs, queue.GetTotalQty())
		levelKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", sidePrefix, price))
		store.SetStorage(levelKey, levelData)
	}

	// Save price list (ordered best→worst)
	pricesData, _ := MarshalPriceList(prices)
	store.SetStorage(PairStorageKey(pairID, sideSuffix), pricesData)
}

// saveStopOrders saves stop orders to storage.
func saveStopOrders(store StateStore, pairID uint32, eng *engine.MatchingEngine) {
	stopCount := eng.OrderBook.Stop.Len()
	if stopCount == 0 {
		store.SetStorage(PairStorageKey(pairID, KeySuffixStops), nil)
		return
	}

	// Pre-allocate slice with known size
	orders := make([]*core.Order, 0, stopCount)
	eng.OrderBook.Stop.Iterate(func(o *core.Order) {
		orders = append(orders, o)
	})

	for _, order := range orders {
		coldKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", KeySuffixOrderCold, order.ID))
		hotKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", KeySuffixOrderHot, order.ID))
		coldData, _ := MarshalOrderCold(order)
		hotData := MarshalOrderHot(order.Quantity)
		store.SetStorage(coldKey, coldData)
		store.SetStorage(hotKey, hotData)

		idxKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", KeySuffixOrderIndex, order.ID))
		store.SetStorage(idxKey, MarshalOrderLocator(order.Side, order.Price))
	}

	stopsData, _ := MarshalStopOrders(orders)
	store.SetStorage(PairStorageKey(pairID, KeySuffixStops), stopsData)
}

// SaveAggregates saves the aggregate data for both sides with Top 50 level cache.
// This is called after order operations to update the FOK optimization cache.
func SaveAggregates(store StateStore, pairID uint32, eng *engine.MatchingEngine) {
	SaveSideAggregates(store, pairID, eng.OrderBook.Bids, KeySuffixAggBids)
	SaveSideAggregates(store, pairID, eng.OrderBook.Asks, KeySuffixAggAsks)
}

// SaveSideAggregates saves the aggregate data for one side with Top 50 level cache.
func SaveSideAggregates(store StateStore, pairID uint32, side *book.OrderSide, keySuffix string) {
	topPrices, topQtys, topCount := side.GetTopNLevels()

	agg := &SideAggregates{
		Best:          side.GetBest(),
		LevelCount:    uint32(side.Len()),
		TotalQty:      side.GetTotalQty(),
		TotalNotional: side.GetTotalNotional(),
		TopNCount:     topCount,
	}

	// Copy arrays
	for i := uint8(0); i < topCount; i++ {
		agg.TopNPrices[i] = topPrices[i]
		agg.TopNQty[i] = topQtys[i]
	}

	aggData, _ := MarshalAggTop50(agg)
	store.SetStorage(PairStorageKey(pairID, keySuffix), aggData)
}

// saveSideWithLoader saves only loaded/touched levels; untouched levels remain in storage.
func saveSideWithLoader(store StateStore, pairID uint32, eng *engine.MatchingEngine, side core.Side, loaderPrices []uint64, loadedPrices map[uint64]struct{}, deletedIDs map[uint64]struct{}) {
	var orderSide *book.OrderSide
	var sidePrefix string
	var sideSuffix string
	if side == core.SideBuy {
		orderSide = eng.OrderBook.Bids
		sidePrefix = KeySuffixLevels + "bids/"
		sideSuffix = KeySuffixBids
	} else {
		orderSide = eng.OrderBook.Asks
		sidePrefix = KeySuffixLevels + "asks/"
		sideSuffix = KeySuffixAsks
	}

	enginePrices := orderSide.GetPrices()
	engineSet := make(map[uint64]struct{}, len(enginePrices))
	for _, p := range enginePrices {
		engineSet[p] = struct{}{}
	}

	priceSet := make(map[uint64]struct{}, len(loaderPrices)+len(enginePrices))
	finalPrices := make([]uint64, 0, len(loaderPrices)+len(enginePrices))

	// Handle existing prices from loader
	for _, price := range loaderPrices {
		if _, loaded := loadedPrices[price]; loaded {
			if _, stillHere := engineSet[price]; !stillHere {
				// Level was loaded and removed - delete it
				levelKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", sidePrefix, price))
				store.SetStorage(levelKey, nil)
				continue
			}
		}
		finalPrices = append(finalPrices, price)
		priceSet[price] = struct{}{}
	}

	// Add new prices from engine not already present
	for _, price := range enginePrices {
		if _, exists := priceSet[price]; !exists {
			finalPrices = append(finalPrices, price)
			priceSet[price] = struct{}{}
		}
	}

	// Sort prices to maintain best→worst ordering
	if side == core.SideBuy {
		sort.Slice(finalPrices, func(i, j int) bool { return finalPrices[i] > finalPrices[j] })
	} else {
		sort.Slice(finalPrices, func(i, j int) bool { return finalPrices[i] < finalPrices[j] })
	}

	// Save/delete deleted order records
	for id := range deletedIDs {
		coldKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", KeySuffixOrderCold, id))
		hotKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", KeySuffixOrderHot, id))
		idxKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", KeySuffixOrderIndex, id))
		store.SetStorage(coldKey, nil)
		store.SetStorage(hotKey, nil)
		store.SetStorage(idxKey, nil)
	}

	// Persist levels that exist in memory or were loaded
	priceLevels := orderSide.GetPriceLevels()
	for _, price := range finalPrices {
		queue := priceLevels[price]

		if queue == nil || queue.Len() == 0 {
			// Only delete if level was known to be loaded/touched
			if _, loaded := loadedPrices[price]; loaded {
				levelKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", sidePrefix, price))
				store.SetStorage(levelKey, nil)
			}
			continue
		}

		// Collect orders at this level preserving FIFO
		orders := make([]*core.Order, 0, queue.Len())
		queue.Iterate(func(o *core.Order) bool {
			orders = append(orders, o)
			return true
		})

		orderIDs := make([]uint64, len(orders))
		for i, order := range orders {
			orderIDs[i] = order.ID

			// Persist hot/cold split
			coldKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", KeySuffixOrderCold, order.ID))
			hotKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", KeySuffixOrderHot, order.ID))
			coldData, _ := MarshalOrderCold(order)
			hotData := MarshalOrderHot(order.Quantity)
			store.SetStorage(coldKey, coldData)
			store.SetStorage(hotKey, hotData)

			// Persist locator index
			idxKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", KeySuffixOrderIndex, order.ID))
			store.SetStorage(idxKey, MarshalOrderLocator(order.Side, order.Price))
		}

		levelData, _ := MarshalLevelColumnar(orderIDs, queue.GetTotalQty())
		levelKey := PairStorageKey(pairID, fmt.Sprintf("%s%d", sidePrefix, price))
		store.SetStorage(levelKey, levelData)
	}

	// Save price list
	pricesData, _ := MarshalPriceList(finalPrices)
	store.SetStorage(PairStorageKey(pairID, sideSuffix), pricesData)
}

// SaveAggregatesBasic saves the basic aggregate data (without Top 50 cache).
