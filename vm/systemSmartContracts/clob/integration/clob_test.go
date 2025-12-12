package integration

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
)

// ============================================================================
// Basic Order Tests
// ============================================================================

func TestLimitOrder_PlaceOnEmptyBook(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2) // Base=BTC(1), Quote=USDC(2)

	// Register user and deposit
	env.RegisterUser("alice")
	env.Deposit("alice", 2, 10000) // 10000 USDC

	// Place limit buy order: 100 qty @ price 100 (needs 100*100/100=100 USDC locked)
	result, err := env.PlaceOrder("alice", pairID, core.SideBuy, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, uint64(0), result.FilledQty, "should not be filled on empty book")
	require.Equal(t, uint64(100), result.RemainingQty)
	require.Equal(t, byte(0), result.Status, "should be placed on book")

	// Verify balance: 100 USDC locked
	env.AssertBalance("alice", 2, 9900, 100)

	// Verify depth
	bids, asks := env.GetDepth(pairID)
	require.Len(t, bids, 1)
	require.Equal(t, [2]uint64{100, 100}, bids[0])
	require.Len(t, asks, 0)
}

func TestLimitOrder_FullMatch(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	// Setup: Alice places sell order, Bob places matching buy order
	// With ScaleFactor=100 (2 decimals), "100" represents 1.00 units
	env.RegisterUser("alice")
	env.RegisterUser("bob")
	env.Deposit("alice", 1, 1000) // 10.00 BTC (in scaled units)
	env.Deposit("bob", 2, 10000)  // 100.00 USDC (in scaled units)

	// Alice places sell: 100 (1.00 BTC) @ price 100 (1.00 USDC per BTC)
	// Lock amount for sell = quantity = 100
	_, err := env.PlaceOrder("alice", pairID, core.SideSell, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	env.AssertBalance("alice", 1, 900, 100) // 100 BTC locked

	// Bob places buy: 100 BTC @ 100 - should fully match
	// Lock amount for buy = qty * price / scaleFactor = 100 * 100 / 100 = 100 USDC
	result, err := env.PlaceOrder("bob", pairID, core.SideBuy, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(100), result.FilledQty)
	require.Equal(t, uint64(0), result.RemainingQty)
	require.Equal(t, byte(1), result.Status, "should be fully filled")

	// Verify balances after trade
	// quoteAmount = qty * price / scaleFactor = 100 * 100 / 100 = 100 USDC
	// Alice: sold 100 BTC, received 100 USDC
	env.AssertBalance("alice", 1, 900, 0) // No more locked
	env.AssertBalance("alice", 2, 100, 0) // Received USDC
	// Bob: bought 100 BTC, paid 100 USDC
	env.AssertBalance("bob", 1, 100, 0)  // Received BTC
	env.AssertBalance("bob", 2, 9900, 0) // Paid 100 USDC (10000 - 100)

	// Verify book is empty
	bids, asks := env.GetDepth(pairID)
	require.Len(t, bids, 0)
	require.Len(t, asks, 0)
}

func TestLimitOrder_PartialMatch(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.RegisterUser("bob")
	env.Deposit("alice", 1, 1000)
	env.Deposit("bob", 2, 20000)

	// Alice places sell: 100 BTC @ 100
	_, err := env.PlaceOrder("alice", pairID, core.SideSell, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Bob places buy: 50 BTC @ 100 - partial match
	result, err := env.PlaceOrder("bob", pairID, core.SideBuy, core.TypeLimit, 50, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(50), result.FilledQty)
	require.Equal(t, uint64(0), result.RemainingQty)
	require.Equal(t, byte(1), result.Status)

	// Alice's order should still have 50 remaining
	bids, asks := env.GetDepth(pairID)
	require.Len(t, bids, 0)
	require.Len(t, asks, 1)
	require.Equal(t, [2]uint64{100, 50}, asks[0]) // 50 remaining
}

// ============================================================================
// Market Order Tests
// ============================================================================

func TestMarketOrder_FullFill(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.RegisterUser("bob")
	env.Deposit("alice", 1, 1000)
	env.Deposit("bob", 2, 10000)

	// Alice places limit sell: 100 @ 100
	_, err := env.PlaceOrder("alice", pairID, core.SideSell, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Bob places market buy for 100
	// quoteAmount = 100 * 100 / 100 = 100 USDC
	result, err := env.PlaceOrder("bob", pairID, core.SideBuy, core.TypeMarket, 100, 0, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(100), result.FilledQty)
	require.Equal(t, byte(1), result.Status)

	// Verify balances (bob paid 100 USDC for 100 BTC)
	env.AssertBalance("bob", 1, 100, 0)
	env.AssertBalance("bob", 2, 9900, 0) // 10000 - 100
}

func TestMarketOrder_PartialFill_NoLiquidity(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.RegisterUser("bob")
	env.Deposit("alice", 1, 50) // Only 50 BTC
	env.Deposit("bob", 2, 20000)

	// Alice places limit sell: only 50 BTC
	_, err := env.PlaceOrder("alice", pairID, core.SideSell, core.TypeLimit, 50, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Bob places market buy for 100 - only 50 available
	result, err := env.PlaceOrder("bob", pairID, core.SideBuy, core.TypeMarket, 100, 0, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(50), result.FilledQty)
	require.Equal(t, uint64(50), result.RemainingQty)
}

// ============================================================================
// TIF (Time-In-Force) Tests
// ============================================================================

func TestTIF_FOK_Success(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.RegisterUser("bob")
	env.Deposit("alice", 1, 100)
	env.Deposit("bob", 2, 10000)

	// Alice places limit sell: 100 BTC @ 100
	_, err := env.PlaceOrder("alice", pairID, core.SideSell, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Bob places FOK buy for exactly 100 - should succeed
	result, err := env.PlaceOrder("bob", pairID, core.SideBuy, core.TypeLimit, 100, 100, 0, core.TIFFillOrKill, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(100), result.FilledQty)
	require.Equal(t, byte(1), result.Status)
}

func TestTIF_FOK_Fail_InsufficientLiquidity(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.RegisterUser("bob")
	env.Deposit("alice", 1, 50) // Only 50 BTC
	env.Deposit("bob", 2, 20000)

	// Alice places limit sell: only 50 BTC
	_, err := env.PlaceOrder("alice", pairID, core.SideSell, core.TypeLimit, 50, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Bob places FOK buy for 100 - should fail (only 50 available)
	_, err = env.PlaceOrder("bob", pairID, core.SideBuy, core.TypeLimit, 100, 100, 0, core.TIFFillOrKill, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fill-or-kill")

	// Bob's balance should be unchanged
	env.AssertBalance("bob", 2, 20000, 0)
}

func TestTIF_IOC_PartialFillAndCancel(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.RegisterUser("bob")
	env.Deposit("alice", 1, 50)
	env.Deposit("bob", 2, 20000)

	// Alice places limit sell: 50 BTC @ 100
	_, err := env.PlaceOrder("alice", pairID, core.SideSell, core.TypeLimit, 50, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Bob places IOC buy for 100 @ 100 - should partially fill, remainder cancelled
	// Lock for buy = 100 * 100 / 100 = 100 USDC
	result, err := env.PlaceOrder("bob", pairID, core.SideBuy, core.TypeLimit, 100, 100, 0, core.TIFImmediateOrCancel, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(50), result.FilledQty)
	require.Equal(t, uint64(50), result.RemainingQty)
	require.Equal(t, byte(2), result.Status, "should be partial fill + cancelled")

	// Bob should only have paid for 50 BTC
	// quoteAmount = 50 * 100 / 100 = 50 USDC
	env.AssertBalance("bob", 1, 50, 0)
	env.AssertBalance("bob", 2, 19950, 0) // 20000 - 50 (paid for 50 BTC)
}

// ============================================================================
// Cancel Order Tests
// ============================================================================

func TestCancelOrder_UnlocksFunds(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.Deposit("alice", 2, 10000)

	// Place limit buy
	result, err := env.PlaceOrder("alice", pairID, core.SideBuy, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	orderID := result.OrderID

	// Verify funds locked
	env.AssertBalance("alice", 2, 9900, 100)

	// Cancel order
	cancelResult, err := env.CancelOrder("alice", pairID, orderID)
	require.NoError(t, err)
	require.Equal(t, orderID, cancelResult.OrderID)
	require.Equal(t, uint64(100), cancelResult.UnlockedQty)

	// Verify funds unlocked
	env.AssertBalance("alice", 2, 10000, 0)
}

func TestCancelOrder_PartiallyFilled(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.RegisterUser("bob")
	env.Deposit("alice", 1, 100)
	env.Deposit("bob", 2, 10000)

	// Alice places limit sell: 100 BTC @ 100
	result, err := env.PlaceOrder("alice", pairID, core.SideSell, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	aliceOrderID := result.OrderID

	// Bob partially fills: buys 30 BTC @ 100
	// quoteAmount = 30 * 100 / 100 = 30 USDC
	_, err = env.PlaceOrder("bob", pairID, core.SideBuy, core.TypeLimit, 30, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Alice cancels remaining 70
	cancelResult, err := env.CancelOrder("alice", pairID, aliceOrderID)
	require.NoError(t, err)
	require.Equal(t, uint64(70), cancelResult.UnlockedQty) // 100 - 30 filled

	// Alice: sold 30 BTC, 70 unlocked
	// Received 30 USDC (30 * 100 / 100)
	env.AssertBalance("alice", 1, 70, 0)
	env.AssertBalance("alice", 2, 30, 0) // Received from 30 BTC sale
}

// ============================================================================
// Balance Validation Tests
// ============================================================================

func TestPlaceOrder_InsufficientBalance(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.Deposit("alice", 2, 50) // Only 50 USDC

	// Try to place order requiring 100 USDC
	_, err := env.PlaceOrder("alice", pairID, core.SideBuy, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient")

	// Balance unchanged
	env.AssertBalance("alice", 2, 50, 0)
}

func TestWithdraw_InsufficientBalance(t *testing.T) {
	env := NewTestEnv(t)
	env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.Deposit("alice", 2, 1000)

	// Try to withdraw more than available
	err := env.Withdraw("alice", 2, 2000)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient")

	// Balance unchanged
	env.AssertBalance("alice", 2, 1000, 0)
}

func TestWithdraw_LockedFunds(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.Deposit("alice", 2, 1000)

	// Place order to lock some funds
	_, err := env.PlaceOrder("alice", pairID, core.SideBuy, core.TypeLimit, 50, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	env.AssertBalance("alice", 2, 950, 50)

	// Try to withdraw all (including locked) - should fail
	err = env.Withdraw("alice", 2, 1000)
	require.Error(t, err)

	// Can withdraw available
	err = env.Withdraw("alice", 2, 950)
	require.NoError(t, err)
	env.AssertBalance("alice", 2, 0, 50)
}

// ============================================================================
// Zero/Invalid Input Tests (Bug 9 regression)
// ============================================================================

func TestPlaceOrder_ZeroQuantity(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.Deposit("alice", 2, 10000)

	_, err := env.PlaceOrder("alice", pairID, core.SideBuy, core.TypeLimit, 0, 100, 0, core.TIFGoodTillCancel, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "quantity must be greater than zero")
}

func TestPlaceOrder_ZeroPrice_LimitOrder(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.Deposit("alice", 2, 10000)

	_, err := env.PlaceOrder("alice", pairID, core.SideBuy, core.TypeLimit, 100, 0, 0, core.TIFGoodTillCancel, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "price must be greater than zero")
}

// ============================================================================
// Multiple Price Levels Tests
// ============================================================================

func TestMultiplePriceLevels_PriceTimePriority(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.RegisterUser("bob")
	env.RegisterUser("charlie")
	env.Deposit("alice", 1, 1000)
	env.Deposit("bob", 1, 1000)
	env.Deposit("charlie", 2, 100000)

	// Alice places sell @ 100
	_, err := env.PlaceOrder("alice", pairID, core.SideSell, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Bob places sell @ 99 (better price)
	_, err = env.PlaceOrder("bob", pairID, core.SideSell, core.TypeLimit, 100, 99, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Verify book has both levels
	_, asks := env.GetDepth(pairID)
	require.Len(t, asks, 2)
	require.Equal(t, uint64(99), asks[0][0], "best ask should be 99")
	require.Equal(t, uint64(100), asks[1][0])

	// Charlie buys 100 - should fill at 99 first (Bob's order)
	result, err := env.PlaceOrder("charlie", pairID, core.SideBuy, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(100), result.FilledQty)

	// Bob should have sold, Alice should not
	env.AssertBalance("bob", 1, 900, 0)     // Sold 100
	env.AssertBalance("alice", 1, 900, 100) // Still has 100 locked
}

func TestGapInPriceLevels_MarketOrderSweep(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.RegisterUser("bob")
	env.RegisterUser("charlie")
	env.Deposit("alice", 1, 100)
	env.Deposit("bob", 1, 100)
	env.Deposit("charlie", 2, 100000)

	// Create asks at various prices with gaps
	// Alice sells 50 @ 100
	_, err := env.PlaceOrder("alice", pairID, core.SideSell, core.TypeLimit, 50, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	// Bob sells 50 @ 200
	_, err = env.PlaceOrder("bob", pairID, core.SideSell, core.TypeLimit, 50, 200, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Market order should sweep both levels
	result, err := env.PlaceOrder("charlie", pairID, core.SideBuy, core.TypeMarket, 100, 0, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(100), result.FilledQty)

	// Both sellers should have sold
	env.AssertBalance("alice", 1, 50, 0)
	env.AssertBalance("bob", 1, 50, 0)

	// Charlie paid: 50*100/100 + 50*200/100 = 50 + 100 = 150 USDC
	env.AssertBalance("charlie", 2, 99850, 0) // 100000 - 150
}

// ============================================================================
// Per-Asset Decimal Tests
// ============================================================================

func TestDecimals_HigherPrecision(t *testing.T) {
	env := NewTestEnv(t)
	// Use 4 decimal places for price (scaleFactor = 10000)
	pairID := env.CreatePair(1, 2, 4, 4, 4)

	env.RegisterUser("alice")
	env.RegisterUser("bob")
	env.Deposit("alice", 1, 10000)    // 1.0000 BTC (4 decimals)
	env.Deposit("bob", 2, 1000000000) // 100000.0000 USDC (4 decimals)

	// Alice sells 1.0000 BTC @ price 50000.0000 USDC
	// With 4 decimal places: qty=10000, price=500000000
	// Lock for sell = 10000 BTC
	_, err := env.PlaceOrder("alice", pairID, core.SideSell, core.TypeLimit,
		10000,     // 1.0000 BTC
		500000000, // 50000.0000 USDC price
		0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Bob buys 1.0000 BTC @ 50000.0000 USDC
	// Lock for buy = qty * price / scaleFactor = 10000 * 500000000 / 10000 = 500000000 USDC
	result, err := env.PlaceOrder("bob", pairID, core.SideBuy, core.TypeLimit,
		10000,
		500000000,
		0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(10000), result.FilledQty)

	// Verify balances
	// quoteAmount = 10000 * 500000000 / 10000 = 500000000 (50000.0000 USDC)
	env.AssertBalance("alice", 1, 0, 0)
	env.AssertBalance("alice", 2, 500000000, 0)
	env.AssertBalance("bob", 1, 10000, 0)
	env.AssertBalance("bob", 2, 500000000, 0) // 1000000000 - 500000000
}

// ============================================================================
// Level Cleanup Tests
// ============================================================================

func TestLevelCleanup_AfterFullConsumption(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.RegisterUser("bob")
	env.Deposit("alice", 1, 100)
	env.Deposit("bob", 2, 10000)

	// Alice places sell
	_, err := env.PlaceOrder("alice", pairID, core.SideSell, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Verify ask exists
	_, asks := env.GetDepth(pairID)
	require.Len(t, asks, 1)

	// Bob fully consumes the level
	_, err = env.PlaceOrder("bob", pairID, core.SideBuy, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Level should be cleaned up
	_, asks = env.GetDepth(pairID)
	require.Len(t, asks, 0, "level should be removed after full consumption")
}

// ============================================================================
// Additional Complex Scenarios
// ============================================================================

func TestMarketSell_SweepsMultipleBidLevels(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("bidder1")
	env.RegisterUser("bidder2")
	env.RegisterUser("seller")
	env.Deposit("bidder1", 2, 200) // enough to lock 50 @ 100
	env.Deposit("bidder2", 2, 200) // enough to lock 100 @ 90
	env.Deposit("seller", 1, 200)  // sells 120

	// Two bid levels
	_, err := env.PlaceOrder("bidder1", pairID, core.SideBuy, core.TypeLimit, 50, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	_, err = env.PlaceOrder("bidder2", pairID, core.SideBuy, core.TypeLimit, 100, 90, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Seller sweeps both levels (50 @ 100, 70 @ 90)
	result, err := env.PlaceOrder("seller", pairID, core.SideSell, core.TypeMarket, 120, 0, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(120), result.FilledQty)
	require.Equal(t, uint64(0), result.RemainingQty)

	// Seller receives 50*100/100 + 70*90/100 = 113 quote; loses 120 base
	env.AssertBalance("seller", 1, 80, 0)
	env.AssertBalance("seller", 2, 113, 0)

	// Maker 1 fully filled, maker 2 partially filled with 30 remaining
	env.AssertBalance("bidder1", 1, 50, 0)
	env.AssertBalance("bidder1", 2, 150, 0)

	// bidder2 locked 90 initially, spent 63 for 70 qty, 30 qty remain locked (27)
	env.AssertBalance("bidder2", 1, 70, 0)
	env.AssertBalance("bidder2", 2, 110, 27)

	// Depth keeps remaining bid
	env.AssertDepth(pairID, [][2]uint64{{90, 30}}, nil)
}

func TestLimitCross_PartialFillRestsOnBook(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("seller")
	env.RegisterUser("buyer")
	env.Deposit("seller", 1, 100)
	env.Deposit("buyer", 2, 1000)

	// One ask at 100
	_, err := env.PlaceOrder("seller", pairID, core.SideSell, core.TypeLimit, 50, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Buyer crosses for more size; remainder should rest on the book
	result, err := env.PlaceOrder("buyer", pairID, core.SideBuy, core.TypeLimit, 120, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(50), result.FilledQty)
	require.Equal(t, uint64(70), result.RemainingQty)
	require.Equal(t, byte(0), result.Status, "remainder should stay posted")

	// Buyer: spent 50 quote, 70 qty still locked
	env.AssertBalance("buyer", 1, 50, 0)
	env.AssertBalance("buyer", 2, 880, 70)

	// Seller paid out and unlocked
	env.AssertBalance("seller", 1, 50, 0)
	env.AssertBalance("seller", 2, 50, 0)

	// Depth shows resting bid
	env.AssertDepth(pairID, [][2]uint64{{100, 70}}, nil)
}

func TestCancelOrder_UnauthorizedFailsAndOwnerSucceeds(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("alice")
	env.RegisterUser("bob")
	env.Deposit("alice", 2, 100)

	res, err := env.PlaceOrder("alice", pairID, core.SideBuy, core.TypeLimit, 50, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Bob cannot cancel Alice's order
	_, err = env.CancelOrder("bob", pairID, res.OrderID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unauthorized")

	// Order still on the book
	env.AssertDepth(pairID, [][2]uint64{{100, 50}}, nil)

	// Alice cancels successfully
	_, err = env.CancelOrder("alice", pairID, res.OrderID)
	require.NoError(t, err)
	env.AssertBalance("alice", 2, 100, 0)
	env.AssertDepth(pairID, nil, nil)
}

func TestPostOnly_RejectedWithoutBalanceChange(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("maker")
	env.RegisterUser("postonly")
	env.Deposit("maker", 1, 100)
	env.Deposit("postonly", 2, 1000)

	// Existing ask at 100
	_, err := env.PlaceOrder("maker", pairID, core.SideSell, core.TypeLimit, 20, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Post-only buy that would match should be rejected and not lock funds
	_, err = env.PlaceOrder("postonly", pairID, core.SideBuy, core.TypeLimit, 10, 100, 0, core.TIFPostOnly, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "post-only")
	env.AssertBalance("postonly", 2, 1000, 0)

	// Depth unchanged
	env.AssertDepth(pairID, nil, [][2]uint64{{100, 20}})
}

func TestStopLimit_TriggersOnPriceRiseViaMatchOrders(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	// Establish initial price at 95
	env.RegisterUser("sellerInit")
	env.RegisterUser("buyerInit")
	env.Deposit("sellerInit", 1, 100)
	env.Deposit("buyerInit", 2, 1000)
	_, err := env.PlaceOrder("sellerInit", pairID, core.SideSell, core.TypeLimit, 10, 95, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	_, err = env.PlaceOrder("buyerInit", pairID, core.SideBuy, core.TypeMarket, 10, 0, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Liquidity at 101
	env.RegisterUser("asker")
	env.Deposit("asker", 1, 100)
	_, err = env.PlaceOrder("asker", pairID, core.SideSell, core.TypeLimit, 30, 101, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Stop buy that should trigger once price >= 100
	env.RegisterUser("stopper")
	env.Deposit("stopper", 2, 5000)
	_, err = env.PlaceOrder("stopper", pairID, core.SideBuy, core.TypeStopLimit, 15, 101, 100, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Trade at 101 to move lastPrice above stop
	env.RegisterUser("pump")
	env.Deposit("pump", 2, 2000)
	_, err = env.PlaceOrder("pump", pairID, core.SideBuy, core.TypeMarket, 5, 0, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Trigger stop via matchOrders
	matches := env.MatchOrders(pairID)
	require.GreaterOrEqual(t, matches, 1, "stop order should activate and match")

	// Note: matchOrders does not perform balance transfers; funds remain locked for the stop order
	// even though the book state reflects the match.
	env.AssertBalance("stopper", 1, 0, 0)
	env.AssertBalance("stopper", 2, 4985, 15)

	// Remaining asks reduced (30 - 5 - 15 = 10)
	env.AssertDepth(pairID, nil, [][2]uint64{{101, 10}})
}

// ============================================================================
// View Endpoints (getBalance, getOrder)
// ============================================================================

func TestViewEndpoints_GetOrderAndBalance(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("maker")
	env.RegisterUser("taker")
	env.Deposit("maker", 2, 10000)
	env.Deposit("taker", 1, 100)

	// Maker posts buy 100 @ 100 (locks 100 quote)
	order, err := env.PlaceOrder("maker", pairID, core.SideBuy, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	env.AssertBalance("maker", 2, 9900, 100)

	// Taker sells 40, partially filling maker
	_, err = env.PlaceOrder("taker", pairID, core.SideSell, core.TypeLimit, 40, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Check balances via view endpoint
	avail, locked := env.GetBalance("maker", 2)
	require.Equal(t, uint64(9900), avail)
	require.Equal(t, uint64(60), locked) // 40 filled, 60 remaining locked

	// getOrder returns remaining quantity and metadata
	view := env.GetOrder(pairID, order.OrderID)
	require.Equal(t, order.OrderID, view.ID)
	require.Equal(t, env.UIDs["maker"], view.UID)
	require.Equal(t, core.SideBuy, view.Side)
	require.Equal(t, core.TypeLimit, view.Type)
	require.Equal(t, core.TIFGoodTillCancel, view.TIF)
	require.Equal(t, uint64(60), view.Quantity)
	require.Equal(t, uint64(100), view.Price)
	require.Equal(t, uint64(0), view.Stop)
}

func TestAdminSetConfig_EnforcesNotionalTickAndStep(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	// Admin sets constraints: minNotional=1000, qtyStep=5, priceTick=10, minQty=5, maxQty=50, minPrice=20, maxPrice=10000
	args := [][]byte{
		big.NewInt(int64(pairID)).Bytes(),
		nil,                       // maxOrders unchanged
		nil,                       // maxPriceLevelsPerSide unchanged
		big.NewInt(5).Bytes(),     // minOrderQty
		big.NewInt(50).Bytes(),    // maxOrderQty
		big.NewInt(20).Bytes(),    // minPrice
		big.NewInt(10000).Bytes(), // maxPrice
		big.NewInt(1000).Bytes(),  // minNotional
		big.NewInt(5).Bytes(),     // qtyStep
		big.NewInt(10).Bytes(),    // priceTick
		nil,                       // maxOrdersPerAccount unchanged
	}
	err := env.SetConfig(pairID, args)
	require.NoError(t, err)

	env.RegisterUser("maker")
	env.RegisterUser("taker")
	env.Deposit("maker", 1, 100) // base
	env.Deposit("taker", 2, 5000)

	// Maker posts aligned ask: qty=20 (step 5), price=100 (tick 10)
	_, err = env.PlaceOrder("maker", pairID, core.SideSell, core.TypeLimit, 20, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Misaligned price rejected
	_, err = env.PlaceOrder("taker", pairID, core.SideBuy, core.TypeLimit, 10, 101, 0, core.TIFGoodTillCancel, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "tick")

	// Misaligned qty rejected
	_, err = env.PlaceOrder("taker", pairID, core.SideBuy, core.TypeLimit, 7, 100, 0, core.TIFGoodTillCancel, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "step")

	// Below min price rejected (price aligned but under bound)
	_, err = env.PlaceOrder("taker", pairID, core.SideBuy, core.TypeLimit, 10, 10, 0, core.TIFGoodTillCancel, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "minimum price")

	// Above max price rejected
	_, err = env.PlaceOrder("taker", pairID, core.SideBuy, core.TypeLimit, 10, 20000, 0, core.TIFGoodTillCancel, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "maximum price")

	// Below min qty rejected
	_, err = env.PlaceOrder("taker", pairID, core.SideBuy, core.TypeLimit, 1, 100, 0, core.TIFGoodTillCancel, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "minimum order quantity")

	// Above max qty rejected
	_, err = env.PlaceOrder("taker", pairID, core.SideBuy, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "maximum order quantity")

	// Market buy below minNotional rejected: 5 * 100 = 500 < 1000
	_, err = env.PlaceOrder("taker", pairID, core.SideBuy, core.TypeMarket, 5, 0, 0, core.TIFGoodTillCancel, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "notional")

	// Market buy aligned and above minNotional succeeds: 15 * 100 = 1500
	_, err = env.PlaceOrder("taker", pairID, core.SideBuy, core.TypeMarket, 15, 0, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
}

func TestAdminSetConfig_MaxOrdersPerAccount(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	// Set MaxOrdersPerAccount = 1
	args := [][]byte{
		big.NewInt(int64(pairID)).Bytes(),
		nil, nil, nil, nil, nil, nil, nil, nil, nil,
		big.NewInt(1).Bytes(), // maxOrdersPerAccount
	}
	err := env.SetConfig(pairID, args)
	require.NoError(t, err)

	env.RegisterUser("alice")
	env.Deposit("alice", 2, 1000)

	// First order ok
	_, err = env.PlaceOrder("alice", pairID, core.SideBuy, core.TypeLimit, 10, 10, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)

	// Second order from same user should fail due to maxOrdersPerAccount
	_, err = env.PlaceOrder("alice", pairID, core.SideBuy, core.TypeLimit, 5, 10, 0, core.TIFGoodTillCancel, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "maximum number of open orders")
}

// ============================================================================
// Balance Consistency Tests (Same-Account Taker/Maker)
// ============================================================================

// TestSameAccountTakerMaker_BuyMatchesOwnSell tests the scenario where
// a user's new buy order matches against their own existing sell order.
// This verifies that balances are correctly updated without stale reads.
func TestSameAccountTakerMaker_BuyMatchesOwnSell(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2) // BTC/USDC

	env.RegisterUser("alice")
	env.Deposit("alice", 1, 1000) // 10.00 BTC
	env.Deposit("alice", 2, 1000) // 10.00 USDC

	// Alice places sell order: 100 BTC @ price 100
	// This locks 100 base (BTC)
	sellResult, err := env.PlaceOrder("alice", pairID, core.SideSell, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.NotNil(t, sellResult)
	env.AssertBalance("alice", 1, 900, 100) // 900 available, 100 locked
	env.AssertBalance("alice", 2, 1000, 0)

	// Alice places buy order: 50 BTC @ price 100
	// This should match against her own sell order
	// Lock amount for buy = 50 * 100 / 100 = 50 USDC
	buyResult, err := env.PlaceOrder("alice", pairID, core.SideBuy, core.TypeLimit, 50, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.NotNil(t, buyResult)
	require.Equal(t, uint64(50), buyResult.FilledQty, "buy order should be fully filled")

	// After the match (same-account partial fill):
	// - Alice's sell order partially filled (50 out of 100), 50 BTC remains locked
	// - Alice's buy order fully filled (taker)
	//
	// Current behavior with same-account matching:
	// The system processes both sides of the trade:
	// - Sell side (maker): 50 BTC unlocked from original 100, receives 50 USDC
	// - Buy side (taker): pays 50 USDC, receives 50 BTC
	// - Net: +50 USDC -50 USDC = 0, +50 BTC -50 BTC = 0
	// However, the implementation appears to credit the filled amount:
	// BTC: 900 base + 50 (from buy) = 950 available, 50 locked
	// USDC: 1000 (net zero after self-trade) available, 0 locked
	env.AssertBalance("alice", 1, 950, 50) // 950 available (900 + 50 from self-trade), 50 still locked
	env.AssertBalance("alice", 2, 1000, 0) // No net USDC change (paid 50, received 50)

	// Verify order book state
	_, asks := env.GetDepth(pairID)
	require.Len(t, asks, 1)
	require.Equal(t, [2]uint64{100, 50}, asks[0]) // 50 BTC remaining at price 100
}

// TestSameAccountTakerMaker_SellMatchesOwnBuy tests the scenario where
// a user's new sell order matches against their own existing buy order.
func TestSameAccountTakerMaker_SellMatchesOwnBuy(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("bob")
	env.Deposit("bob", 1, 1000) // 10.00 BTC
	env.Deposit("bob", 2, 1000) // 10.00 USDC

	// Bob places buy order: 100 BTC @ price 100
	// Lock amount = 100 * 100 / 100 = 100 USDC
	buyResult, err := env.PlaceOrder("bob", pairID, core.SideBuy, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.NotNil(t, buyResult)
	env.AssertBalance("bob", 1, 1000, 0)
	env.AssertBalance("bob", 2, 900, 100) // 900 available, 100 locked

	// Bob places sell order: 50 BTC @ price 100
	// This should match against his own buy order
	sellResult, err := env.PlaceOrder("bob", pairID, core.SideSell, core.TypeLimit, 50, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.NotNil(t, sellResult)
	require.Equal(t, uint64(50), sellResult.FilledQty)

	// After the match (same-account partial fill):
	// - Bob's buy order (maker) partially filled (50 out of 100), 50 BTC remaining at 50 USDC cost
	// - Bob's sell order (taker) fully filled
	//
	// Current behavior with same-account matching:
	// - Buy side (maker): 50 USDC unlocked from original 100, pays 50 USDC for 50 BTC
	// - Sell side (taker): sells 50 BTC, receives 50 USDC
	// - Net: +50 USDC -50 USDC = 0, +50 BTC -50 BTC = 0
	// BTC: 1000 available (no net change), 0 locked
	// USDC: 900 base + 50 (received from sell) = 950 available, 50 locked for remaining buy
	env.AssertBalance("bob", 1, 1000, 0)  // No net BTC change (sold 50, bought 50)
	env.AssertBalance("bob", 2, 950, 50) // 950 available (900 + 50 from self-trade), 50 locked

	// Verify order book state
	bids, _ := env.GetDepth(pairID)
	require.Len(t, bids, 1)
	require.Equal(t, [2]uint64{100, 50}, bids[0]) // 50 BTC bid remaining at price 100
}

// TestSameAccountTakerMaker_FullMatch tests complete fill of own order
func TestSameAccountTakerMaker_FullMatch(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("charlie")
	env.Deposit("charlie", 1, 1000)
	env.Deposit("charlie", 2, 1000)

	// Charlie places sell: 100 @ 100
	_, err := env.PlaceOrder("charlie", pairID, core.SideSell, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	env.AssertBalance("charlie", 1, 900, 100)

	// Charlie places buy: 100 @ 100 (fully matches own sell)
	result, err := env.PlaceOrder("charlie", pairID, core.SideBuy, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(100), result.FilledQty)

	// After full self-match, all should be unlocked and net zero change
	// BTC: started with 1000, locked 100 for sell, unlocked after match = 1000 available
	// USDC: started with 1000, no lock needed for fully filled buy = 1000 available
	env.AssertBalance("charlie", 1, 1000, 0)
	env.AssertBalance("charlie", 2, 1000, 0)

	// Book should be empty
	bids, asks := env.GetDepth(pairID)
	require.Len(t, bids, 0)
	require.Len(t, asks, 0)
}

// TestSameAccountTakerMaker_MarketOrder tests market order matching own limit order
func TestSameAccountTakerMaker_MarketOrder(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("dave")
	env.Deposit("dave", 1, 1000)
	env.Deposit("dave", 2, 1000)

	// Dave places limit sell: 100 @ 100
	_, err := env.PlaceOrder("dave", pairID, core.SideSell, core.TypeLimit, 100, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	env.AssertBalance("dave", 1, 900, 100)

	// Dave places market buy for 50 (matches own sell)
	result, err := env.PlaceOrder("dave", pairID, core.SideBuy, core.TypeMarket, 50, 0, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(50), result.FilledQty)

	// After partial match via market order (same-account):
	// - Sell order (maker): 50 filled out of 100, 50 locked remaining
	// - Market buy (taker): filled 50
	// Current behavior: Credits the filled amount
	// BTC: 900 base + 50 (from buy) = 950 available, 50 locked
	// USDC: 1000 available (net zero: paid 50, received 50)
	env.AssertBalance("dave", 1, 950, 50)
	env.AssertBalance("dave", 2, 1000, 0)

	// Verify remaining sell order
	_, asks := env.GetDepth(pairID)
	require.Len(t, asks, 1)
	require.Equal(t, [2]uint64{100, 50}, asks[0])
}

// TestSameAccountTakerMaker_MultipleLevels tests self-matching across price levels
func TestSameAccountTakerMaker_MultipleLevels(t *testing.T) {
	env := NewTestEnv(t)
	pairID := env.CreatePair(1, 2)

	env.RegisterUser("eve")
	env.Deposit("eve", 1, 1000)
	env.Deposit("eve", 2, 2000)

	// Eve places multiple sell orders at different prices
	_, err := env.PlaceOrder("eve", pairID, core.SideSell, core.TypeLimit, 50, 100, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	_, err = env.PlaceOrder("eve", pairID, core.SideSell, core.TypeLimit, 50, 110, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	env.AssertBalance("eve", 1, 900, 100) // 100 BTC locked across both orders

	// Eve places buy that sweeps both levels: 100 @ 110
	result, err := env.PlaceOrder("eve", pairID, core.SideBuy, core.TypeLimit, 100, 110, 0, core.TIFGoodTillCancel, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(100), result.FilledQty)

	// After full match across both price levels (same-account):
	// - Sell orders fully consumed (50@100 + 50@110 = 100 BTC)
	// - Buy order fully filled (100 BTC @ 110 limit, executed at 100 and 110)
	// Cost: 50*100/100 + 50*110/100 = 50 + 55 = 105 USDC paid
	// Revenue: Same 105 USDC received (self-match: buyer pays seller, both are Eve)
	// Net USDC effect: 0 (she pays herself)
	// Price improvement: Locked 110 USDC (100@110 limit), paid 105, savings of 5 unlocked
	// Final: BTC 1000 (100 locked returned), USDC 2000 (net zero from self-match)
	env.AssertBalance("eve", 1, 1000, 0)
	env.AssertBalance("eve", 2, 2000, 0) // net zero for self-match

	// Book should be empty
	bids, asks := env.GetDepth(pairID)
	require.Len(t, bids, 0)
	require.Len(t, asks, 0)
}
