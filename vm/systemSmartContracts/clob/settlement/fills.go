package settlement

import (
	"fmt"

	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/account"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/pair"
)

// BalanceCacheKey is used to key the balance cache by (UID, AssetID).
type BalanceCacheKey struct {
	UID     uint64
	AssetID uint32
}

// ProcessFillsArgs holds all parameters for ProcessFills.
type ProcessFillsArgs struct {
	PairConfig       *pair.Config
	Done             *core.Done
	TakerSide        core.Side
	OrderType        core.OrderType
	TakerUID         uint64
	TakerLockAssetID uint32
	TakerLimitPrice  uint64 // taker's limit price for calculating price improvement
	ScaleFactor      uint64
}

// ProcessFills processes trade fills and updates balances.
// Returns the modified balances to be saved by the caller.
func ProcessFills(
	accountMgr *account.Manager,
	args ProcessFillsArgs,
) (map[BalanceCacheKey]*account.AssetBalance, error) {
	baseAssetID := args.PairConfig.BaseAssetID
	quoteAssetID := args.PairConfig.QuoteAssetID

	// Balance cache to handle same-account taker/maker scenarios
	// This prevents stale reads when the same account appears in multiple trades
	balanceCache := make(map[BalanceCacheKey]*account.AssetBalance)

	// Helper to get balance from cache or load from storage
	getBalance := func(uid uint64, assetID uint32) *account.AssetBalance {
		key := BalanceCacheKey{UID: uid, AssetID: assetID}
		if bal, ok := balanceCache[key]; ok {
			return bal
		}
		bal := accountMgr.LoadBalance(uid, assetID)
		balanceCache[key] = bal
		return bal
	}

	for i := 0; i < len(args.Done.Trades); i += 2 {
		if i+1 >= len(args.Done.Trades) {
			break
		}

		takerTrade := args.Done.Trades[i]
		makerTrade := args.Done.Trades[i+1]

		tradeQty := takerTrade.Quantity
		tradePrice := makerTrade.Price
		quoteAmount, calcErr := core.SafeMulDiv(tradeQty, tradePrice, args.ScaleFactor)
		if calcErr != nil {
			return nil, fmt.Errorf("quote amount calculation overflow")
		}

		makerUID := makerTrade.UID
		if makerUID == 0 {
			continue
		}

		// Calculate amounts based on side
		var takerLoseAmt, takerGainAmt uint64
		var takerGainAssetID, makerLockAssetID, makerGainAssetID uint32
		var makerLoseAmt, makerGainAmt uint64

		if args.TakerSide == core.SideBuy {
			takerLoseAmt = quoteAmount
			takerGainAssetID = baseAssetID
			takerGainAmt = tradeQty
			makerLockAssetID = baseAssetID
			makerGainAssetID = quoteAssetID
			makerLoseAmt = tradeQty
			makerGainAmt = quoteAmount
		} else {
			takerLoseAmt = tradeQty
			takerGainAssetID = quoteAssetID
			takerGainAmt = quoteAmount
			makerLockAssetID = quoteAssetID
			makerGainAssetID = baseAssetID
			makerLoseAmt = quoteAmount
			makerGainAmt = tradeQty
		}

		// Load balances from cache (handles same-account scenarios correctly)
		takerLockBal := getBalance(args.TakerUID, args.TakerLockAssetID)
		takerGainBal := getBalance(args.TakerUID, takerGainAssetID)
		makerLockBal := getBalance(makerUID, makerLockAssetID)
		makerGainBal := getBalance(makerUID, makerGainAssetID)

		// Validate before mutation
		if args.OrderType != core.TypeMarket {
			if err := takerLockBal.ValidateDeductLocked(takerLoseAmt); err != nil {
				return nil, fmt.Errorf("insufficient locked balance for taker: %w", err)
			}
		} else {
			if err := takerLockBal.ValidateDeductAvailable(takerLoseAmt); err != nil {
				return nil, fmt.Errorf("insufficient available balance for market order: have %d, need %d",
					takerLockBal.Available, takerLoseAmt)
			}
		}
		if err := makerLockBal.ValidateDeductLocked(makerLoseAmt); err != nil {
			return nil, fmt.Errorf("insufficient locked balance for maker: %w", err)
		}

		// Apply mutations - ignore: ValidateDeductLocked/ValidateDeductAvailable succeeded above
		if args.OrderType != core.TypeMarket {
			_ = takerLockBal.DeductLocked(takerLoseAmt) // validated above

			// Price improvement: unlock savings when buy limit order fills at better price
			// Taker locked at their limit price, but trade executed at maker's (potentially lower) price
			if args.TakerSide == core.SideBuy && args.TakerLimitPrice > tradePrice {
				lockedForTrade, _ := core.SafeMulDiv(tradeQty, args.TakerLimitPrice, args.ScaleFactor)
				if lockedForTrade > quoteAmount {
					savings := lockedForTrade - quoteAmount
					_ = takerLockBal.Unlock(savings) // unlock price improvement
				}
			}
		} else {
			takerLockBal.Available -= takerLoseAmt // validated above
		}
		takerGainBal.Credit(takerGainAmt)

		_ = makerLockBal.DeductLocked(makerLoseAmt) // validated above
		makerGainBal.Credit(makerGainAmt)
	}

	return balanceCache, nil
}

// SaveBalances saves all modified balances to storage.
func SaveBalances(accountMgr *account.Manager, balances map[BalanceCacheKey]*account.AssetBalance) {
	for key, bal := range balances {
		accountMgr.SaveBalance(key.UID, key.AssetID, bal)
	}
}
