package book

import (
	"testing"

	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
)

// BenchmarkCalculateAvailableLiquidity benchmarks the FOK optimization
// that uses level-based iteration (O(k) where k = price levels) instead
// of order-based iteration (O(n) where n = total orders).
func BenchmarkCalculateAvailableLiquidity(b *testing.B) {
	ob := NewOrderBook()

	// Setup: Add 1000 orders at various price levels
	// Create 50 price levels with 20 orders each
	for i := 0; i < 1000; i++ {
		order := &core.Order{
			ID:        uint64(i + 1),
			Side:      core.SideSell,
			Price:     uint64(100 + (i % 50)), // 50 different price levels (100-149)
			Quantity:  100,
			OrderType: core.TypeLimit,
			TIF:       core.TIFGoodTillCancel,
			UID:       uint64(i + 1),
		}
		ob.Asks.Append(order)
		ob.Orders[order.ID] = order
	}

	// Test order that would need to check liquidity across multiple levels
	testOrder := &core.Order{
		Side:      core.SideBuy,
		Price:     125, // Can match against prices 100-125 (26 levels)
		OrderType: core.TypeLimit,
		TIF:       core.TIFFillOrKill,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ob.calculateAvailableLiquidity(testOrder)
	}
}

// BenchmarkCalculateAvailableLiquidity_DeepBook benchmarks with a deeper book
// to measure scaling with large number of price levels.
func BenchmarkCalculateAvailableLiquidity_DeepBook(b *testing.B) {
	ob := NewOrderBook()

	// Setup: 10,000 orders across 100 price levels
	for i := 0; i < 10000; i++ {
		order := &core.Order{
			ID:        uint64(i + 1),
			Side:      core.SideSell,
			Price:     uint64(100 + (i % 100)), // 100 different price levels
			Quantity:  50,
			OrderType: core.TypeLimit,
			TIF:       core.TIFGoodTillCancel,
			UID:       uint64(i + 1),
		}
		ob.Asks.Append(order)
		ob.Orders[order.ID] = order
	}

	testOrder := &core.Order{
		Side:      core.SideBuy,
		Price:     150, // Can match against all 100 levels
		OrderType: core.TypeLimit,
		TIF:       core.TIFFillOrKill,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ob.calculateAvailableLiquidity(testOrder)
	}
}

// BenchmarkCalculateAvailableLiquidity_SingleLevel benchmarks the best case
// where all liquidity is at a single price level.
func BenchmarkCalculateAvailableLiquidity_SingleLevel(b *testing.B) {
	ob := NewOrderBook()

	// Setup: 1000 orders at the same price level
	for i := 0; i < 1000; i++ {
		order := &core.Order{
			ID:        uint64(i + 1),
			Side:      core.SideSell,
			Price:     100,
			Quantity:  100,
			OrderType: core.TypeLimit,
			TIF:       core.TIFGoodTillCancel,
			UID:       uint64(i + 1),
		}
		ob.Asks.Append(order)
		ob.Orders[order.ID] = order
	}

	testOrder := &core.Order{
		Side:      core.SideBuy,
		Price:     100,
		OrderType: core.TypeLimit,
		TIF:       core.TIFFillOrKill,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ob.calculateAvailableLiquidity(testOrder)
	}
}

// BenchmarkProcessLimitOrder_FOK_Success benchmarks a FOK order that succeeds
// with the liquidity check optimization.
func BenchmarkProcessLimitOrder_FOK_Success(b *testing.B) {
	// Setup: Create order book with sufficient liquidity
	setupFOKBook := func() *OrderBook {
		ob := NewOrderBook()
		for i := 0; i < 100; i++ {
			order := &core.Order{
				ID:        uint64(i + 1),
				Side:      core.SideSell,
				Price:     100,
				Quantity:  10,
				OrderType: core.TypeLimit,
				TIF:       core.TIFGoodTillCancel,
				UID:       uint64(i + 1),
			}
			ob.Asks.Append(order)
			ob.Orders[order.ID] = order
		}
		return ob
	}

	b.Run("with_FOK_check", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			ob := setupFOKBook()
			testOrder := &core.Order{
				ID:        99999,
				Side:      core.SideBuy,
				Price:     100,
				Quantity:  500, // Total available is 1000
				OrderType: core.TypeLimit,
				TIF:       core.TIFFillOrKill,
				UID:       99999,
			}
			b.StartTimer()
			_, _ = ob.processLimitOrder(core.NewDone(testOrder, 4, 2), testOrder, nil)
		}
	})
}

// BenchmarkProcessLimitOrder_FOK_Fail benchmarks a FOK order that fails
// the liquidity check (should be fast - no matching attempted).
func BenchmarkProcessLimitOrder_FOK_Fail(b *testing.B) {
	// Setup: Create order book with insufficient liquidity
	setupFOKBook := func() *OrderBook {
		ob := NewOrderBook()
		for i := 0; i < 50; i++ {
			order := &core.Order{
				ID:        uint64(i + 1),
				Side:      core.SideSell,
				Price:     100,
				Quantity:  10,
				OrderType: core.TypeLimit,
				TIF:       core.TIFGoodTillCancel,
				UID:       uint64(i + 1),
			}
			ob.Asks.Append(order)
			ob.Orders[order.ID] = order
		}
		return ob
	}

	b.Run("with_FOK_check", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			ob := setupFOKBook()
			testOrder := &core.Order{
				ID:        99999,
				Side:      core.SideBuy,
				Price:     100,
				Quantity:  1000, // Total available is only 500 - should fail fast
				OrderType: core.TypeLimit,
				TIF:       core.TIFFillOrKill,
				UID:       99999,
			}
			b.StartTimer()
			_, _ = ob.processLimitOrder(core.NewDone(testOrder, 4, 2), testOrder, nil)
		}
	})
}

// BenchmarkProcessLimitOrder_GTC benchmarks a regular GTC order
// (no FOK check overhead).
func BenchmarkProcessLimitOrder_GTC(b *testing.B) {
	setupBook := func() *OrderBook {
		ob := NewOrderBook()
		for i := 0; i < 100; i++ {
			order := &core.Order{
				ID:        uint64(i + 1),
				Side:      core.SideSell,
				Price:     100,
				Quantity:  10,
				OrderType: core.TypeLimit,
				TIF:       core.TIFGoodTillCancel,
				UID:       uint64(i + 1),
			}
			ob.Asks.Append(order)
			ob.Orders[order.ID] = order
		}
		return ob
	}

	b.Run("GTC_no_check", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			ob := setupBook()
			testOrder := &core.Order{
				ID:        99999,
				Side:      core.SideBuy,
				Price:     100,
				Quantity:  500,
				OrderType: core.TypeLimit,
				TIF:       core.TIFGoodTillCancel,
				UID:       99999,
			}
			b.StartTimer()
			_, _ = ob.processLimitOrder(core.NewDone(testOrder, 4, 2), testOrder, nil)
		}
	})
}
