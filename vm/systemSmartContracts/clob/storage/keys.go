package storage

import "encoding/binary"

// Storage key constants for CLOB system smart contract persistence

// PairStorageKey constructs a storage key for pair-specific data.
// Format: [pairID:4][suffix]
func PairStorageKey(pairID uint32, suffix string) []byte {
	key := make([]byte, 4+len(suffix))
	binary.BigEndian.PutUint32(key, pairID)
	copy(key[4:], suffix)
	return key
}

// Account storage keys
const (
	// KeyAccountsMeta stores the next UID counter for account registration
	KeyAccountsMeta = "accounts/meta"
)

// Pair storage keys
const (
	// KeyMetaLastPairID stores the last assigned pair ID
	KeyMetaLastPairID = "meta/last_pair_id"

	// KeyPairPrefix is the prefix for pair configuration data
	KeyPairPrefix = "pair/"

	// KeyPairIdxPrefix is the prefix for pair index lookup (asset1_asset2 -> pairID)
	KeyPairIdxPrefix = "pair_idx/"
)

// Order book storage suffixes (prefixed by pairID)
const (
	// KeySuffixCfg stores the order book configuration for a pair
	KeySuffixCfg = "/cfg"

	// KeySuffixMeta stores metadata (nextOrderID, lastPrice) for a pair
	KeySuffixMeta = "/meta"

	// KeySuffixBids stores the list of bid price levels
	KeySuffixBids = "/bids"

	// KeySuffixAsks stores the list of ask price levels
	KeySuffixAsks = "/asks"

	// KeySuffixStops stores stop orders
	KeySuffixStops = "/stops"

	// KeySuffixLevels is the prefix for individual price level data (columnar: IDs only)
	KeySuffixLevels = "/levels/"

	// KeySuffixOrderIndex is the prefix for order locator entries (O(1) cancel)
	// Full key: {pairID}/idx/{orderID} -> {side:1}{price:varint}
	KeySuffixOrderIndex = "/idx/"

	// KeySuffixAggBids stores aggregate data for bids (best, totalQty, levelCount, top50)
	KeySuffixAggBids = "/agg/bids"

	// KeySuffixAggAsks stores aggregate data for asks (best, totalQty, levelCount, top50)
	KeySuffixAggAsks = "/agg/asks"

	// Hot/Cold Order Splitting:
	// Cold records store immutable order fields (written once)
	// Hot records store mutable order fields (updated on fills)

	// KeySuffixOrderCold stores immutable order data (Side, Type, Price, Stop, TIF, OCO, UID)
	// Full key: {pairID}/cold/{orderID}
	KeySuffixOrderCold = "/cold/"

	// KeySuffixOrderHot stores mutable order data (Quantity only)
	// Full key: {pairID}/hot/{orderID}
	KeySuffixOrderHot = "/hot/"
)
