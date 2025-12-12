package engine

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
)

func TestConstraints_MinNotionalAndSteps(t *testing.T) {
	eng := NewMatchingEngine()
	cfg := core.DefaultConfig()
	cfg.MinNotional = 1_000 // requires qty*price >= 1000
	cfg.PriceTick = 10      // prices must be multiples of 10
	cfg.QtyStep = 5         // quantities must be multiples of 5
	cfg.MaxOrdersPerAccount = 1
	cfg.MinOrderQty = 5
	cfg.MaxOrderQty = 50
	cfg.MinPrice = 20
	cfg.MaxPrice = 10_000
	eng.SetConfig(cfg)

	// Seed ask liquidity at 20 (aligned to tick/step and above min price)
	_, err := eng.ProcessOrder(core.SideSell, core.TypeLimit, 10, 20, 0, core.TIFGoodTillCancel, 0, 1)
	require.NoError(t, err)

	// Market buy with too small notional should fail
	_, err = eng.ProcessOrder(core.SideBuy, core.TypeMarket, 5, 0, 0, core.TIFGoodTillCancel, 0, 2)
	require.ErrorIs(t, err, core.ErrMinNotional)

	// Limit order with misaligned price should fail (price above min, off tick)
	_, err = eng.ProcessOrder(core.SideBuy, core.TypeLimit, 10, 25, 0, core.TIFGoodTillCancel, 0, 3)
	require.ErrorIs(t, err, core.ErrInvalidPriceTick)

	// Limit order with misaligned qty should fail
	_, err = eng.ProcessOrder(core.SideBuy, core.TypeLimit, 7, 20, 0, core.TIFGoodTillCancel, 0, 4)
	require.ErrorIs(t, err, core.ErrInvalidQtyStep)

	// Limit order below min price fails (price aligned to tick but below bound)
	_, err = eng.ProcessOrder(core.SideBuy, core.TypeLimit, 10, 10, 0, core.TIFGoodTillCancel, 0, 6)
	require.ErrorIs(t, err, core.ErrMinPrice)

	// Limit order above max price fails
	_, err = eng.ProcessOrder(core.SideBuy, core.TypeLimit, 10, 20_000, 0, core.TIFGoodTillCancel, 0, 7)
	require.ErrorIs(t, err, core.ErrMaxPrice)

	// Limit order below min qty fails
	_, err = eng.ProcessOrder(core.SideBuy, core.TypeLimit, 1, 20, 0, core.TIFGoodTillCancel, 0, 8)
	require.ErrorIs(t, err, core.ErrMinOrderQty)

	// Limit order above max qty fails
	_, err = eng.ProcessOrder(core.SideBuy, core.TypeLimit, 100, 20, 0, core.TIFGoodTillCancel, 0, 9)
	require.ErrorIs(t, err, core.ErrMaxOrderQty)

	// Resting order (UID 5) on ask side with no crossing bids -> increments account
	done, err := eng.ProcessOrder(core.SideSell, core.TypeLimit, 10, 20, 0, core.TIFGoodTillCancel, 0, 5)
	require.NoError(t, err)
	require.True(t, done.Stored)

	// Second open order from same UID should hit maxOrdersPerAccount
	_, err = eng.ProcessOrder(core.SideSell, core.TypeLimit, 10, 20, 0, core.TIFGoodTillCancel, 0, 5)
	require.ErrorIs(t, err, core.ErrMaxOrdersAccount)
}
