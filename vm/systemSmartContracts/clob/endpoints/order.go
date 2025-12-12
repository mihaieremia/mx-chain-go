package endpoints

import (
	"fmt"
	"math/big"

	vmcommon "github.com/multiversx/mx-chain-vm-common-go"

	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/engine"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/pair"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/settlement"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/storage"
)

// OrderParams holds parsed order parameters.
type OrderParams struct {
	PairID    uint32
	Side      core.Side
	OrderType core.OrderType
	Quantity  uint64
	Price     uint64
	Stop      uint64
	TIF       core.TIF
	OCO       uint64
}

// ParseOrderParams parses order parameters from endpoint arguments.
func ParseOrderParams(args [][]byte) (*OrderParams, error) {
	if len(args) < 8 {
		return nil, fmt.Errorf("processOrder requires pairID, side, type, qty, price, stop, tif, oco")
	}

	params := &OrderParams{
		PairID:    uint32(big.NewInt(0).SetBytes(args[0]).Uint64()),
		Side:      core.Side(args[1][0]),
		OrderType: core.ParseOrderType(args[2]),
		Quantity:  big.NewInt(0).SetBytes(args[3]).Uint64(),
		Price:     big.NewInt(0).SetBytes(args[4]).Uint64(),
		Stop:      big.NewInt(0).SetBytes(args[5]).Uint64(),
		TIF:       core.ParseTIF(args[6]),
		OCO:       big.NewInt(0).SetBytes(args[7]).Uint64(),
	}

	return params, nil
}

// ValidateOrderParams validates basic order parameters.
func ValidateOrderParams(params *OrderParams) error {
	if params.Quantity == 0 {
		return fmt.Errorf("quantity must be greater than zero")
	}
	if params.OrderType != core.TypeMarket && params.Price == 0 {
		return fmt.Errorf("limit order price must be greater than zero")
	}
	return nil
}

// CalculateLockAmount calculates the amount to lock for an order.
func CalculateLockAmount(pairConfig *pair.Config, side core.Side, quantity, price, scaleFactor uint64) (assetID uint32, amount uint64, err error) {
	if side == core.SideBuy {
		assetID = pairConfig.QuoteAssetID
		amount, err = core.SafeMulDiv(quantity, price, scaleFactor)
		if err != nil {
			return 0, 0, fmt.Errorf("order value overflow")
		}
	} else {
		assetID = pairConfig.BaseAssetID
		amount = quantity
	}
	return assetID, amount, nil
}

// SimulateMarketOrder simulates a market order to get exact cost using lazy level loading.
func SimulateMarketOrder(eng *engine.MatchingEngine, loader *storage.LazyLoader, side core.Side, quantity uint64, tif core.TIF, scaleFactor uint64) (uint64, error) {
	var loadNext func() error
	var hasMore func() bool
	if loader != nil {
		if side == core.SideBuy {
			loadNext = loader.LoadNextAskLevel
			hasMore = loader.HasMoreAskLevels
		} else {
			loadNext = loader.LoadNextBidLevel
			hasMore = loader.HasMoreBidLevels
		}
	}

	// Prime first level if available
	if hasMore != nil && hasMore() {
		if err := loadNext(); err != nil {
			return 0, err
		}
	}

	fills, filled, simErr := eng.OrderBook.SimulateMarketOrder(side, quantity, tif)
	for simErr == nil && filled < quantity && hasMore != nil && hasMore() {
		if err := loadNext(); err != nil {
			return 0, err
		}
		fills, filled, simErr = eng.OrderBook.SimulateMarketOrder(side, quantity, tif)
	}
	if simErr != nil {
		return 0, simErr
	}

	var exactCost uint64
	if side == core.SideBuy {
		for _, fill := range fills {
			fillCost, calcErr := core.SafeMulDiv(fill.Quantity, fill.Price, scaleFactor)
			if calcErr != nil {
				return 0, fmt.Errorf("fill cost overflow")
			}
			exactCost, calcErr = core.SafeAdd(exactCost, fillCost)
			if calcErr != nil {
				return 0, fmt.Errorf("total cost overflow")
			}
		}
	} else {
		for _, fill := range fills {
			var calcErr error
			exactCost, calcErr = core.SafeAdd(exactCost, fill.Quantity)
			if calcErr != nil {
				return 0, fmt.Errorf("total quantity overflow")
			}
		}
	}

	return exactCost, nil
}

// DetermineOrderStatus determines the order status from Done result.
func DetermineOrderStatus(done *core.Done, quantity, filledQty uint64) byte {
	if done.Order == nil || done.Order.Quantity == 0 {
		if filledQty == quantity {
			return 1 // Fully filled
		}
		return 2 // Partial fill, remainder cancelled (IOC/FOK)
	}
	return 0 // Placed on book
}

// CalculateUnlockAmount calculates amount to unlock for cancelled remainder.
func CalculateUnlockAmount(side core.Side, remainingQty, price, scaleFactor uint64) uint64 {
	if side == core.SideBuy {
		unlockAmount, _ := core.SafeMulDiv(remainingQty, price, scaleFactor)
		return unlockAmount
	}
	return remainingQty
}

// GetOrder handles getOrder endpoint.
func GetOrder(ctx *Context, args [][]byte) vmcommon.ReturnCode {
	if len(args) < 2 {
		return ctx.Error("getOrder requires pairID, orderID")
	}

	pairID := uint32(big.NewInt(0).SetBytes(args[0]).Uint64())
	orderID := big.NewInt(0).SetBytes(args[1]).Uint64()

	eng, loader, err := storage.LoadOrderBookLazy(ctx.Eei, pairID)
	if err != nil {
		return ctx.Error(err.Error())
	}

	if err := loader.LoadOrderByID(orderID); err != nil {
		return ctx.Error(err.Error())
	}

	order, err := eng.GetOrder(orderID)
	if err != nil {
		return ctx.Error(err.Error())
	}

	ctx.Eei.Finish(big.NewInt(int64(order.ID)).Bytes())
	ctx.Eei.Finish(big.NewInt(int64(order.UID)).Bytes())
	ctx.Eei.Finish([]byte{byte(order.Side)})
	ctx.Eei.Finish([]byte{byte(order.OrderType)})
	ctx.Eei.Finish(big.NewInt(int64(order.Quantity)).Bytes())
	ctx.Eei.Finish(big.NewInt(int64(order.Price)).Bytes())
	ctx.Eei.Finish(big.NewInt(int64(order.Stop)).Bytes())
	ctx.Eei.Finish([]byte{byte(order.TIF)})

	return vmcommon.Ok
}

// GetDepth handles getDepth endpoint.
func GetDepth(ctx *Context, args [][]byte) vmcommon.ReturnCode {
	if len(args) < 1 {
		return ctx.Error("getDepth requires pairID")
	}

	pairID := uint32(big.NewInt(0).SetBytes(args[0]).Uint64())

	eng, loader, err := storage.LoadOrderBookLazy(ctx.Eei, pairID)
	if err != nil {
		return ctx.Error(err.Error())
	}

	// Load all levels to return full depth
	if err := loader.LoadAllLevels(); err != nil {
		return ctx.Error(err.Error())
	}
	depth := eng.GetDepth()
	if depth == nil {
		return vmcommon.Ok
	}

	// Return: [numBids, bid1Price, bid1Qty, ..., numAsks, ask1Price, ask1Qty, ...]
	ctx.Eei.Finish(big.NewInt(int64(len(depth.Bids))).Bytes())
	for _, level := range depth.Bids {
		ctx.Eei.Finish(big.NewInt(int64(level[0])).Bytes())
		ctx.Eei.Finish(big.NewInt(int64(level[1])).Bytes())
	}

	ctx.Eei.Finish(big.NewInt(int64(len(depth.Asks))).Bytes())
	for _, level := range depth.Asks {
		ctx.Eei.Finish(big.NewInt(int64(level[0])).Bytes())
		ctx.Eei.Finish(big.NewInt(int64(level[1])).Bytes())
	}

	return vmcommon.Ok
}

// MatchOrders handles matchOrders endpoint.
func MatchOrders(ctx *Context, args [][]byte) vmcommon.ReturnCode {
	if len(args) < 1 {
		return ctx.Error("matchOrders requires pairID")
	}

	pairID := uint32(big.NewInt(0).SetBytes(args[0]).Uint64())

	eng, loader, err := storage.LoadOrderBookLazy(ctx.Eei, pairID)
	if err != nil {
		return ctx.Error(err.Error())
	}

	// Ensure lastPrice is set; fallback to best prices if missing
	if eng.GetLastPrice() == nil {
		if bestAsk := eng.OrderBook.Asks.Best(); bestAsk != nil {
			price := bestAsk.GetPrice()
			eng.SetLastPrice(&price)
		} else if bestBid := eng.OrderBook.Bids.Best(); bestBid != nil {
			price := bestBid.GetPrice()
			eng.SetLastPrice(&price)
		}
	}

	if err := loader.LoadStops(); err != nil {
		return ctx.Error(err.Error())
	}

	// Load full book for deterministic matching in this endpoint
	if err := loader.LoadAllLevels(); err != nil {
		return ctx.Error(err.Error())
	}

	// Seed lastPrice from current top-of-book to ensure stop activation uses latest book state
	if bestBid := eng.OrderBook.Bids.Best(); bestBid != nil || eng.OrderBook.Asks.Best() != nil {
		var candidate uint64
		if bestBid != nil {
			candidate = bestBid.GetPrice()
		}
		if bestAsk := eng.OrderBook.Asks.Best(); bestAsk != nil && bestAsk.GetPrice() > candidate {
			candidate = bestAsk.GetPrice()
		}
		if candidate > 0 {
			eng.SetLastPrice(&candidate)
		}
	}

	dones, err := eng.MatchOrders(nil, nil)
	if err != nil {
		return ctx.Error(err.Error())
	}

	storage.SaveOrderBook(ctx.Eei, pairID, eng)

	ctx.Eei.Finish(big.NewInt(int64(len(dones))).Bytes())
	return vmcommon.Ok
}

// ProcessOrderResult encapsulates the result of order processing.
type ProcessOrderResult struct {
	OrderID      uint64
	FilledQty    uint64
	RemainingQty uint64
	Status       byte
	Done         *core.Done
	Engine       *engine.MatchingEngine
}

// ProcessOrder handles the processOrder endpoint with split helper functions.
// This is the main orchestration function that coordinates the order flow.
func ProcessOrder(ctx *Context, caller []byte, args [][]byte) vmcommon.ReturnCode {
	// Parse parameters
	params, err := ParseOrderParams(args)
	if err != nil {
		return ctx.Error(err.Error())
	}

	// Get pair config
	pairConfig, err := ctx.PairMgr.GetConfig(params.PairID)
	if err != nil {
		return ctx.Error(err.Error())
	}

	// Get user UID
	uid, err := ctx.AccountMgr.GetUIDByAddress(caller)
	if err != nil {
		return ctx.Error("account not registered - call register first")
	}

	// Validate parameters
	if err := ValidateOrderParams(params); err != nil {
		return ctx.Error(err.Error())
	}

	// Calculate scale factor and lock amount
	scaleFactor := pair.ScaleFactor(pairConfig)
	lockAssetID, lockAmount, err := CalculateLockAmount(pairConfig, params.Side, params.Quantity, params.Price, scaleFactor)
	if err != nil {
		return ctx.Error(err.Error())
	}

	// Load order book lazily
	eng, loader, err := storage.LoadOrderBookLazy(ctx.Eei, params.PairID)
	if err != nil {
		return ctx.Error(err.Error())
	}

	loadNextLevel := func() error { return nil }
	if loader != nil {
		if params.Side == core.SideBuy {
			loadNextLevel = loader.LoadNextAskLevel
		} else {
			loadNextLevel = loader.LoadNextBidLevel
		}
	}

	// Load balance
	bal := ctx.AccountMgr.LoadBalance(uid, lockAssetID)

	// Handle market vs limit orders
	if params.OrderType == core.TypeMarket {
		exactCost, simErr := SimulateMarketOrder(eng, loader, params.Side, params.Quantity, params.TIF, scaleFactor)
		if simErr != nil {
			return ctx.Error(simErr.Error())
		}
		if bal.Available < exactCost {
			return ctx.Error(fmt.Sprintf("insufficient balance for market order: have %d, need %d", bal.Available, exactCost))
		}
	} else {
		// Limit/Stop orders: lock the funds
		if err := bal.Lock(lockAmount); err != nil {
			return ctx.Error(fmt.Sprintf("insufficient balance: %v", err))
		}
		ctx.AccountMgr.SaveBalance(uid, lockAssetID, bal)
	}

	// Enforce MaxOrdersPerAccount using lazy counts
	if cfg := eng.GetConfig(); cfg != nil && cfg.MaxOrdersPerAccount > 0 && loader != nil {
		current, err := loader.CountOrdersForUID(uid)
		if err != nil {
			return ctx.Error(err.Error())
		}
		if current >= cfg.MaxOrdersPerAccount {
			return ctx.Error(core.ErrMaxOrdersAccount.Error())
		}
	}

	// Process order
	done, err := eng.ProcessOrderWithLoader(params.Side, params.OrderType, params.Quantity, params.Price, params.Stop, params.TIF, params.OCO, uid, loadNextLevel)
	if err != nil {
		// Revert lock for limit orders
		if params.OrderType != core.TypeMarket {
			_ = bal.Unlock(lockAmount)
			ctx.AccountMgr.SaveBalance(uid, lockAssetID, bal)
		}
		return ctx.Error(err.Error())
	}

	// Process fills
	filledQty := done.Processed
	remainingQty := params.Quantity - filledQty

	if filledQty > 0 {
		fillArgs := settlement.ProcessFillsArgs{
			PairConfig:       pairConfig,
			Done:             done,
			TakerSide:        params.Side,
			OrderType:        params.OrderType,
			TakerUID:         uid,
			TakerLockAssetID: lockAssetID,
			TakerLimitPrice:  params.Price,
			ScaleFactor:      scaleFactor,
		}
		balances, fillErr := settlement.ProcessFills(ctx.AccountMgr, fillArgs)
		if fillErr != nil {
			return ctx.Error(fillErr.Error())
		}
		settlement.SaveBalances(ctx.AccountMgr, balances)
	}

	// Determine status
	status := DetermineOrderStatus(done, params.Quantity, filledQty)

	// Unlock cancelled remainder for IOC/FOK
	if status == 2 && remainingQty > 0 && params.OrderType != core.TypeMarket {
		unlockAmount := CalculateUnlockAmount(params.Side, remainingQty, params.Price, scaleFactor)
		bal = ctx.AccountMgr.LoadBalance(uid, lockAssetID)
		_ = bal.Unlock(unlockAmount)
		ctx.AccountMgr.SaveBalance(uid, lockAssetID, bal)
	}

	// Save order book state
	if loader != nil {
		for _, canceled := range done.CanceledOrders {
			loader.MarkDeleted(canceled.ID)
		}
		storage.SaveOrderBookWithLoader(ctx.Eei, params.PairID, eng, loader)
	} else {
		storage.SaveOrderBook(ctx.Eei, params.PairID, eng)
	}

	// Return result
	ctx.Eei.Finish(big.NewInt(int64(done.Order.ID)).Bytes())
	ctx.Eei.Finish(big.NewInt(int64(filledQty)).Bytes())
	ctx.Eei.Finish(big.NewInt(int64(remainingQty)).Bytes())
	ctx.Eei.Finish([]byte{status})

	return vmcommon.Ok
}

// CancelOrder handles the cancelOrder endpoint.
func CancelOrder(ctx *Context, caller []byte, args [][]byte) vmcommon.ReturnCode {
	if len(args) < 2 {
		return ctx.Error("cancelOrder requires pairID, orderID")
	}

	pairID := uint32(big.NewInt(0).SetBytes(args[0]).Uint64())
	pairConfig, err := ctx.PairMgr.GetConfig(pairID)
	if err != nil {
		return ctx.Error(err.Error())
	}

	orderID := big.NewInt(0).SetBytes(args[1]).Uint64()

	// Load order book
	eng, loader, err := storage.LoadOrderBookLazy(ctx.Eei, pairID)
	if err != nil {
		return ctx.Error(err.Error())
	}

	// Get order to verify ownership
	if err := loader.LoadOrderByID(orderID); err != nil {
		return ctx.Error(err.Error())
	}
	order, err := eng.GetOrder(orderID)
	if err != nil {
		return ctx.Error(err.Error())
	}
	if order.GetOCO() != 0 {
		_ = loader.LoadOrderByID(order.GetOCO()) // best effort to ensure locator exists
	}

	// Verify caller owns this order
	uid, err := ctx.AccountMgr.GetUIDByAddress(caller)
	if err != nil || uid != order.UID {
		return ctx.Error("unauthorized: you don't own this order")
	}

	// Calculate unlock amount
	scaleFactor := pair.ScaleFactor(pairConfig)
	var unlockAssetID uint32
	var unlockAmount uint64
	if order.Side == core.SideBuy {
		unlockAssetID = pairConfig.QuoteAssetID
		unlockAmount, err = core.SafeMulDiv(order.Quantity, order.Price, scaleFactor)
		if err != nil {
			return ctx.Error("unlock calculation overflow")
		}
	} else {
		unlockAssetID = pairConfig.BaseAssetID
		unlockAmount = order.Quantity
	}

	// Validate unlock will succeed
	bal := ctx.AccountMgr.LoadBalance(uid, unlockAssetID)
	if err := bal.ValidateUnlock(unlockAmount); err != nil {
		return ctx.Error(fmt.Sprintf("cannot cancel order - unlock would fail: %v", err))
	}

	// Cancel order
	cancelled, err := eng.CancelOrder(orderID)
	if err != nil {
		return ctx.Error(err.Error())
	}

	// Apply unlock
	_ = bal.Unlock(unlockAmount)
	ctx.AccountMgr.SaveBalance(uid, unlockAssetID, bal)

	// Save order book
	if loader != nil {
		loader.MarkDeleted(orderID)
		if order.GetOCO() != 0 {
			loader.MarkDeleted(order.GetOCO())
		}
		storage.SaveOrderBookWithLoader(ctx.Eei, pairID, eng, loader)
	} else {
		storage.SaveOrderBook(ctx.Eei, pairID, eng)
	}

	// Return result
	ctx.Eei.Finish(big.NewInt(int64(cancelled.ID)).Bytes())
	ctx.Eei.Finish(big.NewInt(int64(order.UID)).Bytes())
	ctx.Eei.Finish([]byte{byte(order.Side)})
	ctx.Eei.Finish(big.NewInt(int64(order.Quantity)).Bytes())
	ctx.Eei.Finish(big.NewInt(int64(order.Price)).Bytes())

	return vmcommon.Ok
}
