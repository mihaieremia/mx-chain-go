package book

import (
	"testing"

	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCancelOrder_O1_Complexity verifies that CancelOrder uses O(1) removal via locator
func TestCancelOrder_O1_Complexity(t *testing.T) {
	t.Parallel()

	ob := NewOrderBook()

	// Add multiple orders at the same price level
	order1 := &core.Order{
		ID:        1,
		Side:      core.SideBuy,
		OrderType: core.TypeLimit,
		Quantity:  100,
		Price:     10000,
	}
	order2 := &core.Order{
		ID:        2,
		Side:      core.SideBuy,
		OrderType: core.TypeLimit,
		Quantity:  200,
		Price:     10000,
	}
	order3 := &core.Order{
		ID:        3,
		Side:      core.SideBuy,
		OrderType: core.TypeLimit,
		Quantity:  300,
		Price:     10000,
	}

	// Process orders
	_, err := ob.Process(order1)
	require.NoError(t, err)
	_, err = ob.Process(order2)
	require.NoError(t, err)
	_, err = ob.Process(order3)
	require.NoError(t, err)

	// Verify all orders are in the book
	assert.Equal(t, 3, len(ob.Orders))
	assert.Equal(t, 3, len(ob.index))

	// Verify locators exist (intrusive list uses embedded Prev/Next pointers in Order)
	loc1 := ob.index[1]
	require.NotNil(t, loc1)
	require.NotNil(t, loc1.Order, "Order 1 locator should have valid Order pointer")

	loc2 := ob.index[2]
	require.NotNil(t, loc2)
	require.NotNil(t, loc2.Order, "Order 2 locator should have valid Order pointer")

	loc3 := ob.index[3]
	require.NotNil(t, loc3)
	require.NotNil(t, loc3.Order, "Order 3 locator should have valid Order pointer")

	// Cancel the middle order (this tests O(1) removal)
	cancelled, err := ob.CancelOrder(2)
	require.NoError(t, err)
	require.NotNil(t, cancelled)
	assert.Equal(t, uint64(2), cancelled.GetID())

	// Verify order was removed from book
	assert.Equal(t, 2, len(ob.Orders))
	assert.Equal(t, 2, len(ob.index))
	assert.Nil(t, ob.Orders[2])

	// Verify remaining orders are still accessible
	assert.NotNil(t, ob.Orders[1])
	assert.NotNil(t, ob.Orders[3])

	// Cancel first order
	cancelled, err = ob.CancelOrder(1)
	require.NoError(t, err)
	require.NotNil(t, cancelled)
	assert.Equal(t, uint64(1), cancelled.GetID())

	// Verify order was removed
	assert.Equal(t, 1, len(ob.Orders))
	assert.Equal(t, 1, len(ob.index))

	// Cancel last order
	cancelled, err = ob.CancelOrder(3)
	require.NoError(t, err)
	require.NotNil(t, cancelled)
	assert.Equal(t, uint64(3), cancelled.GetID())

	// Verify all orders removed
	assert.Equal(t, 0, len(ob.Orders))
	assert.Equal(t, 0, len(ob.index))
	assert.Equal(t, 0, ob.Bids.GetNumOrders())
}

// TestCancelOrder_DifferentPriceLevels tests O(1) cancellation across different price levels
func TestCancelOrder_DifferentPriceLevels(t *testing.T) {
	t.Parallel()

	ob := NewOrderBook()

	// Add orders at different price levels
	order1 := &core.Order{
		ID:        1,
		Side:      core.SideBuy,
		OrderType: core.TypeLimit,
		Quantity:  100,
		Price:     10000,
	}
	order2 := &core.Order{
		ID:        2,
		Side:      core.SideBuy,
		OrderType: core.TypeLimit,
		Quantity:  200,
		Price:     9900,
	}
	order3 := &core.Order{
		ID:        3,
		Side:      core.SideSell,
		OrderType: core.TypeLimit,
		Quantity:  150,
		Price:     10100,
	}

	_, err := ob.Process(order1)
	require.NoError(t, err)
	_, err = ob.Process(order2)
	require.NoError(t, err)
	_, err = ob.Process(order3)
	require.NoError(t, err)

	// Verify all locators exist (intrusive list uses embedded Prev/Next pointers in Order)
	for id := uint64(1); id <= 3; id++ {
		loc := ob.index[id]
		require.NotNil(t, loc, "Locator for order %d should exist", id)
		require.NotNil(t, loc.Order, "Order %d locator should have valid Order pointer", id)
	}

	// Cancel orders in arbitrary order
	cancelled, err := ob.CancelOrder(2)
	require.NoError(t, err)
	require.NotNil(t, cancelled)
	assert.Equal(t, uint64(2), cancelled.GetID())

	cancelled, err = ob.CancelOrder(1)
	require.NoError(t, err)
	require.NotNil(t, cancelled)
	assert.Equal(t, uint64(1), cancelled.GetID())

	cancelled, err = ob.CancelOrder(3)
	require.NoError(t, err)
	require.NotNil(t, cancelled)
	assert.Equal(t, uint64(3), cancelled.GetID())

	// Verify all orders removed and price levels cleaned up
	assert.Equal(t, 0, len(ob.Orders))
	assert.Equal(t, 0, len(ob.index))
	assert.Equal(t, 0, ob.Bids.Len())
	assert.Equal(t, 0, ob.Asks.Len())
}

// TestCancelOrder_NotFound tests cancelling non-existent order
func TestCancelOrder_NotFound(t *testing.T) {
	t.Parallel()

	ob := NewOrderBook()

	// Try to cancel non-existent order
	cancelled, err := ob.CancelOrder(999)
	require.NoError(t, err)
	assert.Nil(t, cancelled)
}

// TestCancelOrder_StopOrder tests cancelling stop orders
func TestCancelOrder_StopOrder(t *testing.T) {
	t.Parallel()

	ob := NewOrderBook()

	// Add a stop order
	stopOrder := &core.Order{
		ID:        1,
		Side:      core.SideBuy,
		OrderType: core.TypeStopLimit,
		Quantity:  100,
		Price:     10000,
		Stop:      9900,
	}

	done, err := ob.Process(stopOrder)
	require.NoError(t, err)
	assert.True(t, done.Stored)

	// Verify order is in the book
	assert.Equal(t, 1, len(ob.Orders))
	loc := ob.index[1]
	require.NotNil(t, loc)
	// Stop orders use StopBook (no queue embedding needed)
	require.NotNil(t, loc.Order)

	// Cancel the stop order
	cancelled, err := ob.CancelOrder(1)
	require.NoError(t, err)
	require.NotNil(t, cancelled)
	assert.Equal(t, uint64(1), cancelled.GetID())

	// Verify order was removed
	assert.Equal(t, 0, len(ob.Orders))
	assert.Equal(t, 0, len(ob.index))
}

// TestRemoveOrder_FilledOrder tests internal removeOrder with O(1) complexity
func TestRemoveOrder_FilledOrder(t *testing.T) {
	t.Parallel()

	ob := NewOrderBook()

	// Add a maker order
	makerOrder := &core.Order{
		ID:        1,
		Side:      core.SideSell,
		OrderType: core.TypeLimit,
		Quantity:  100,
		Price:     10000,
	}

	_, err := ob.Process(makerOrder)
	require.NoError(t, err)

	// Verify locator exists (intrusive list uses embedded Prev/Next pointers in Order)
	loc := ob.index[1]
	require.NotNil(t, loc)
	require.NotNil(t, loc.Order)

	// Add a taker order that will match
	takerOrder := &core.Order{
		ID:        2,
		Side:      core.SideBuy,
		OrderType: core.TypeLimit,
		Quantity:  100,
		Price:     10000,
	}

	done, err := ob.Process(takerOrder)
	require.NoError(t, err)
	assert.Equal(t, uint64(100), done.Processed)

	// Both orders should be removed (filled)
	assert.Equal(t, 0, len(ob.Orders))
	assert.Equal(t, 0, len(ob.index))
}

// TestOrderQueue_RemoveOrder_O1 verifies OrderQueue.RemoveOrder is O(1) via intrusive pointers
func TestOrderQueue_RemoveOrder_O1(t *testing.T) {
	t.Parallel()

	queue := NewOrderQueue(10000)

	// Add multiple orders
	order1 := &core.Order{ID: 1, Quantity: 100}
	order2 := &core.Order{ID: 2, Quantity: 200}
	order3 := &core.Order{ID: 3, Quantity: 300}

	queue.PushBack(order1)
	queue.PushBack(order2)
	queue.PushBack(order3)

	// Verify total quantity
	assert.Equal(t, uint64(600), queue.GetTotalQty())

	// Remove middle order using O(1) removal via intrusive pointers
	removed := queue.RemoveOrder(order2)
	require.NotNil(t, removed)
	assert.Equal(t, uint64(2), removed.GetID())
	assert.Equal(t, uint64(400), queue.GetTotalQty())

	// Remove first order
	removed = queue.RemoveOrder(order1)
	require.NotNil(t, removed)
	assert.Equal(t, uint64(1), removed.GetID())
	assert.Equal(t, uint64(300), queue.GetTotalQty())

	// Remove last order
	removed = queue.RemoveOrder(order3)
	require.NotNil(t, removed)
	assert.Equal(t, uint64(3), removed.GetID())
	assert.Equal(t, uint64(0), queue.GetTotalQty())
	assert.Equal(t, 0, queue.Len())
}

// TestPostOnly_WouldMatch tests that post-only orders are rejected when they would match
func TestPostOnly_WouldMatch(t *testing.T) {
	t.Parallel()

	ob := NewOrderBook()

	// Add a sell order (ask) at price 100
	askOrder := &core.Order{
		ID:        1,
		Side:      core.SideSell,
		OrderType: core.TypeLimit,
		Quantity:  100,
		Price:     100,
		TIF:       core.TIFGoodTillCancel,
	}
	_, err := ob.Process(askOrder)
	require.NoError(t, err)

	// Post-only buy at 100 should be REJECTED (would match the ask)
	postOnlyBuy := &core.Order{
		ID:        2,
		Side:      core.SideBuy,
		OrderType: core.TypeLimit,
		Quantity:  50,
		Price:     100,
		TIF:       core.TIFPostOnly,
	}
	_, err = ob.Process(postOnlyBuy)
	assert.ErrorIs(t, err, core.ErrPostOnlyWouldMatch)

	// Post-only buy at 101 should also be REJECTED (crosses spread)
	postOnlyBuyCross := &core.Order{
		ID:        3,
		Side:      core.SideBuy,
		OrderType: core.TypeLimit,
		Quantity:  50,
		Price:     101,
		TIF:       core.TIFPostOnly,
	}
	_, err = ob.Process(postOnlyBuyCross)
	assert.ErrorIs(t, err, core.ErrPostOnlyWouldMatch)

	// Post-only buy at 99 should be ACCEPTED (doesn't cross spread)
	postOnlyBuyOK := &core.Order{
		ID:        4,
		Side:      core.SideBuy,
		OrderType: core.TypeLimit,
		Quantity:  50,
		Price:     99,
		TIF:       core.TIFPostOnly,
	}
	done, err := ob.Process(postOnlyBuyOK)
	require.NoError(t, err)
	assert.True(t, done.Stored)
	assert.Equal(t, uint64(0), done.Processed) // No matching occurred
}

// TestPostOnly_SellSide tests post-only sell orders
func TestPostOnly_SellSide(t *testing.T) {
	t.Parallel()

	ob := NewOrderBook()

	// Add a buy order (bid) at price 100
	bidOrder := &core.Order{
		ID:        1,
		Side:      core.SideBuy,
		OrderType: core.TypeLimit,
		Quantity:  100,
		Price:     100,
		TIF:       core.TIFGoodTillCancel,
	}
	_, err := ob.Process(bidOrder)
	require.NoError(t, err)

	// Post-only sell at 100 should be REJECTED (would match the bid)
	postOnlySell := &core.Order{
		ID:        2,
		Side:      core.SideSell,
		OrderType: core.TypeLimit,
		Quantity:  50,
		Price:     100,
		TIF:       core.TIFPostOnly,
	}
	_, err = ob.Process(postOnlySell)
	assert.ErrorIs(t, err, core.ErrPostOnlyWouldMatch)

	// Post-only sell at 99 should be REJECTED (crosses spread)
	postOnlySellCross := &core.Order{
		ID:        3,
		Side:      core.SideSell,
		OrderType: core.TypeLimit,
		Quantity:  50,
		Price:     99,
		TIF:       core.TIFPostOnly,
	}
	_, err = ob.Process(postOnlySellCross)
	assert.ErrorIs(t, err, core.ErrPostOnlyWouldMatch)

	// Post-only sell at 101 should be ACCEPTED (doesn't cross spread)
	postOnlySellOK := &core.Order{
		ID:        4,
		Side:      core.SideSell,
		OrderType: core.TypeLimit,
		Quantity:  50,
		Price:     101,
		TIF:       core.TIFPostOnly,
	}
	done, err := ob.Process(postOnlySellOK)
	require.NoError(t, err)
	assert.True(t, done.Stored)
	assert.Equal(t, uint64(0), done.Processed)
}

// TestPostOnly_EmptyBook tests post-only orders on empty book (always accepted)
func TestPostOnly_EmptyBook(t *testing.T) {
	t.Parallel()

	ob := NewOrderBook()

	// Post-only on empty book should always succeed
	postOnlyBuy := &core.Order{
		ID:        1,
		Side:      core.SideBuy,
		OrderType: core.TypeLimit,
		Quantity:  100,
		Price:     100,
		TIF:       core.TIFPostOnly,
	}
	done, err := ob.Process(postOnlyBuy)
	require.NoError(t, err)
	assert.True(t, done.Stored)

	postOnlySell := &core.Order{
		ID:        2,
		Side:      core.SideSell,
		OrderType: core.TypeLimit,
		Quantity:  100,
		Price:     110,
		TIF:       core.TIFPostOnly,
	}
	done, err = ob.Process(postOnlySell)
	require.NoError(t, err)
	assert.True(t, done.Stored)
}

// TestOCO_CancelLinkedOnFill ensures filling one leg cancels the linked OCO order.
func TestOCO_CancelLinkedOnFill(t *testing.T) {
	t.Parallel()

	ob := NewOrderBook()

	oco1 := &core.Order{
		ID:        1,
		Side:      core.SideSell,
		OrderType: core.TypeLimit,
		Quantity:  100,
		Price:     100,
		TIF:       core.TIFGoodTillCancel,
		OCO:       2, // linked to order 2
	}
	oco2 := &core.Order{
		ID:        2,
		Side:      core.SideSell,
		OrderType: core.TypeLimit,
		Quantity:  100,
		Price:     110,
		TIF:       core.TIFGoodTillCancel,
		OCO:       1, // linked back to order 1
	}

	_, err := ob.Process(oco1)
	require.NoError(t, err)
	_, err = ob.Process(oco2)
	require.NoError(t, err)

	// Taker buys at 110; fills order1 at 100, should cancel linked order2
	taker := &core.Order{
		ID:        3,
		Side:      core.SideBuy,
		OrderType: core.TypeLimit,
		Quantity:  100,
		Price:     110,
		TIF:       core.TIFGoodTillCancel,
	}

	done, err := ob.Process(taker)
	require.NoError(t, err)
	require.Equal(t, uint64(100), done.Processed)
	require.Contains(t, done.Canceled, "2")

	// Linked order removed from book
	assert.Nil(t, ob.Orders[2])
	assert.Nil(t, ob.index[2])
	assert.Equal(t, 0, ob.Asks.GetNumOrders())
}
