package storage

import (
	"encoding/binary"
	"fmt"

	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
)

// Binary serialization for CLOB data structures.
// Uses fixed-size encoding for maximum performance, avoiding reflection-heavy JSON.
// Compatible with protobuf wire format for future migration.

// MarshalOrder serializes an Order to bytes using varint encoding for IDs.
// Format: [ID:varint][Side:1][Type:1][Qty:varint][Price:varint][Stop:varint][TIF:1][OCO:varint][UID:varint]
// Priority is determined by Order ID (lower ID = earlier = higher priority).
func MarshalOrder(o *core.Order) ([]byte, error) {
	if o == nil {
		return nil, nil
	}

	// Pre-allocate worst-case buffer (all varints at max 10 bytes)
	// Worst case: ID(10) + Side(1) + Type(1) + Qty(10) + Price(10) + Stop(10) + TIF(1) + OCO(10) + UID(10) = 63 bytes
	buf := make([]byte, 63)
	offset := 0

	// ID (varint)
	n := binary.PutUvarint(buf[offset:], o.ID)
	offset += n

	// Side (1 byte)
	buf[offset] = byte(o.Side)
	offset++

	// OrderType (1 byte - uint8 value)
	buf[offset] = byte(o.OrderType)
	offset++

	// Quantity (varint)
	n = binary.PutUvarint(buf[offset:], o.Quantity)
	offset += n

	// Price (varint)
	n = binary.PutUvarint(buf[offset:], o.Price)
	offset += n

	// Stop (varint)
	n = binary.PutUvarint(buf[offset:], o.Stop)
	offset += n

	// TIF (1 byte - uint8 value)
	buf[offset] = byte(o.TIF)
	offset++

	// OCO (varint)
	n = binary.PutUvarint(buf[offset:], o.OCO)
	offset += n

	// UID (varint)
	n = binary.PutUvarint(buf[offset:], o.UID)
	offset += n

	// Return only the used portion of the buffer
	return buf[:offset], nil
}

// UnmarshalOrder deserializes an Order from bytes using varint encoding.
func UnmarshalOrder(data []byte) (*core.Order, error) {
	if len(data) == 0 {
		return nil, nil
	}

	if len(data) < 3 {
		return nil, fmt.Errorf("invalid order data: too short")
	}

	o := &core.Order{}
	offset := 0

	// ID (varint)
	id, n := binary.Uvarint(data[offset:])
	if n <= 0 {
		return nil, fmt.Errorf("invalid order data: bad ID varint")
	}
	o.ID = id
	offset += n

	// Side (1 byte)
	if offset >= len(data) {
		return nil, fmt.Errorf("invalid order data: missing side")
	}
	o.Side = core.Side(data[offset])
	offset++

	// OrderType (1 byte - uint8 value)
	if offset >= len(data) {
		return nil, fmt.Errorf("invalid order data: missing type")
	}
	o.OrderType = core.OrderType(data[offset])
	offset++

	// Quantity (varint)
	if offset >= len(data) {
		return nil, fmt.Errorf("invalid order data: missing quantity")
	}
	qty, n := binary.Uvarint(data[offset:])
	if n <= 0 {
		return nil, fmt.Errorf("invalid order data: bad quantity varint")
	}
	o.Quantity = qty
	offset += n

	// Price (varint)
	if offset >= len(data) {
		return nil, fmt.Errorf("invalid order data: missing price")
	}
	price, n := binary.Uvarint(data[offset:])
	if n <= 0 {
		return nil, fmt.Errorf("invalid order data: bad price varint")
	}
	o.Price = price
	offset += n

	// Stop (varint)
	if offset >= len(data) {
		return nil, fmt.Errorf("invalid order data: missing stop")
	}
	stop, n := binary.Uvarint(data[offset:])
	if n <= 0 {
		return nil, fmt.Errorf("invalid order data: bad stop varint")
	}
	o.Stop = stop
	offset += n

	// TIF (1 byte - uint8 value)
	if offset >= len(data) {
		return nil, fmt.Errorf("invalid order data: missing tif")
	}
	o.TIF = core.TIF(data[offset])
	offset++

	// OCO (varint)
	if offset >= len(data) {
		return nil, fmt.Errorf("invalid order data: missing oco")
	}
	oco, n := binary.Uvarint(data[offset:])
	if n <= 0 {
		return nil, fmt.Errorf("invalid order data: bad oco varint")
	}
	o.OCO = oco
	offset += n

	// UID (varint)
	if offset >= len(data) {
		return nil, fmt.Errorf("invalid order data: missing uid")
	}
	uid, n := binary.Uvarint(data[offset:])
	if n <= 0 {
		return nil, fmt.Errorf("invalid order data: bad uid varint")
	}
	o.UID = uid

	return o, nil
}

// MarshalMeta serializes metadata to bytes.
// Format: [NextOrderID:8][HasLastPrice:1][LastPrice:8]
func MarshalMeta(nextOrderID uint64, lastPrice *uint64) ([]byte, error) {
	buf := make([]byte, 8+1+8)

	binary.LittleEndian.PutUint64(buf[0:], nextOrderID)

	if lastPrice != nil {
		buf[8] = 1
		binary.LittleEndian.PutUint64(buf[9:], *lastPrice)
	} else {
		buf[8] = 0
		binary.LittleEndian.PutUint64(buf[9:], 0)
	}

	return buf, nil
}

// UnmarshalMeta deserializes metadata from bytes.
func UnmarshalMeta(data []byte) (nextOrderID uint64, lastPrice *uint64, err error) {
	if len(data) < 17 {
		return 0, nil, fmt.Errorf("invalid meta data: too short")
	}

	nextOrderID = binary.LittleEndian.Uint64(data[0:])

	if data[8] == 1 {
		price := binary.LittleEndian.Uint64(data[9:])
		lastPrice = &price
	}

	return nextOrderID, lastPrice, nil
}

// MarshalConfig serializes config to bytes.
// Format: [MaxOrders:4][MaxPriceLevelsPerSide:4][MinOrderQty:8][MaxOrderQty:8][MinPrice:8]
//
//	[MaxPrice:8][MinNotional:8][QtyStep:8][PriceTick:8][MaxOrdersPerAccount:4][Decimals:4][MaxPriceDeviation:2]
func MarshalConfig(cfg *core.Config) ([]byte, error) {
	if cfg == nil {
		return nil, nil
	}

	buf := make([]byte, 4+4+8+8+8+8+8+8+8+4+4+2)
	offset := 0

	binary.LittleEndian.PutUint32(buf[offset:], uint32(cfg.MaxOrders))
	offset += 4
	binary.LittleEndian.PutUint32(buf[offset:], uint32(cfg.MaxPriceLevelsPerSide))
	offset += 4
	binary.LittleEndian.PutUint64(buf[offset:], cfg.MinOrderQty)
	offset += 8
	binary.LittleEndian.PutUint64(buf[offset:], cfg.MaxOrderQty)
	offset += 8
	binary.LittleEndian.PutUint64(buf[offset:], cfg.MinPrice)
	offset += 8
	binary.LittleEndian.PutUint64(buf[offset:], cfg.MaxPrice)
	offset += 8
	binary.LittleEndian.PutUint64(buf[offset:], cfg.MinNotional)
	offset += 8
	binary.LittleEndian.PutUint64(buf[offset:], cfg.QtyStep)
	offset += 8
	binary.LittleEndian.PutUint64(buf[offset:], cfg.PriceTick)
	offset += 8
	binary.LittleEndian.PutUint32(buf[offset:], uint32(cfg.MaxOrdersPerAccount))
	offset += 4
	binary.LittleEndian.PutUint32(buf[offset:], uint32(cfg.Decimals))
	offset += 4
	binary.LittleEndian.PutUint16(buf[offset:], cfg.MaxPriceDeviation)

	return buf, nil
}

// UnmarshalConfig deserializes config from bytes.
func UnmarshalConfig(data []byte) (*core.Config, error) {
	minSize := 4 + 4 + 8 + 8 + 8 + 8 + 8 + 8 + 8 + 4 + 4 // 72 bytes (old format)
	if len(data) < minSize {
		return nil, fmt.Errorf("invalid config data: too short")
	}

	cfg := &core.Config{}
	offset := 0

	cfg.MaxOrders = int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	cfg.MaxPriceLevelsPerSide = int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	cfg.MinOrderQty = binary.LittleEndian.Uint64(data[offset:])
	offset += 8
	cfg.MaxOrderQty = binary.LittleEndian.Uint64(data[offset:])
	offset += 8
	cfg.MinPrice = binary.LittleEndian.Uint64(data[offset:])
	offset += 8
	cfg.MaxPrice = binary.LittleEndian.Uint64(data[offset:])
	offset += 8
	cfg.MinNotional = binary.LittleEndian.Uint64(data[offset:])
	offset += 8
	cfg.QtyStep = binary.LittleEndian.Uint64(data[offset:])
	offset += 8
	cfg.PriceTick = binary.LittleEndian.Uint64(data[offset:])
	offset += 8
	cfg.MaxOrdersPerAccount = int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	cfg.Decimals = int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	// MaxPriceDeviation (new field, optional for backward compat)
	if offset+2 <= len(data) {
		cfg.MaxPriceDeviation = binary.LittleEndian.Uint16(data[offset:])
	} else {
		cfg.MaxPriceDeviation = core.DefaultMaxPriceDeviation
	}

	return cfg, nil
}

// MarshalStopOrders serializes stop orders to bytes.
// Format: [Count:4][[OrderLen:4][OrderData]...]
func MarshalStopOrders(orders []*core.Order) ([]byte, error) {
	if len(orders) == 0 {
		buf := make([]byte, 4)
		return buf, nil
	}

	// Serialize all orders first to calculate total size
	orderBytes := make([][]byte, len(orders))
	totalSize := 4 // header: count(4)

	for i, order := range orders {
		ob, err := MarshalOrder(order)
		if err != nil {
			return nil, fmt.Errorf("marshal stop order %d: %w", i, err)
		}
		orderBytes[i] = ob
		totalSize += 4 + len(ob) // length(4) + order data
	}

	// Allocate final buffer
	buf := make([]byte, totalSize)
	offset := 0

	// Write header
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(orders)))
	offset += 4

	// Write orders with length prefixes
	for _, ob := range orderBytes {
		binary.LittleEndian.PutUint32(buf[offset:], uint32(len(ob)))
		offset += 4
		copy(buf[offset:], ob)
		offset += len(ob)
	}

	return buf, nil
}

// UnmarshalStopOrders deserializes stop orders from bytes.
// Returns the orders and count.
func UnmarshalStopOrders(data []byte) (orders []*core.Order, count int, err error) {
	if len(data) < 4 {
		return nil, 0, fmt.Errorf("invalid stop orders data: too short")
	}

	offset := 0

	// Read header
	count = int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	if count == 0 {
		return nil, 0, nil
	}

	// Read orders
	orders = make([]*core.Order, count)
	for i := 0; i < count; i++ {
		if offset+4 > len(data) {
			return nil, 0, fmt.Errorf("invalid stop orders data: missing order length at index %d", i)
		}
		orderLen := int(binary.LittleEndian.Uint32(data[offset:]))
		offset += 4

		if offset+orderLen > len(data) {
			return nil, 0, fmt.Errorf("invalid stop orders data: order overflow at index %d", i)
		}

		order, err := UnmarshalOrder(data[offset : offset+orderLen])
		if err != nil {
			return nil, 0, fmt.Errorf("invalid stop orders data: unmarshal order %d: %w", i, err)
		}
		orders[i] = order
		offset += orderLen
	}

	return orders, count, nil
}

// MarshalOrderIndex serializes an order ID index to bytes.
// Format: [Count:4][OrderID1:8][OrderID2:8]...
func MarshalOrderIndex(orderIDs []uint64) ([]byte, error) {
	buf := make([]byte, 4+8*len(orderIDs))

	binary.LittleEndian.PutUint32(buf[0:], uint32(len(orderIDs)))

	offset := 4
	for _, id := range orderIDs {
		binary.LittleEndian.PutUint64(buf[offset:], id)
		offset += 8
	}

	return buf, nil
}

// UnmarshalOrderIndex deserializes an order ID index from bytes.
func UnmarshalOrderIndex(data []byte) ([]uint64, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("invalid order index: too short")
	}

	count := int(binary.LittleEndian.Uint32(data[0:]))
	if len(data) < 4+8*count {
		return nil, fmt.Errorf("invalid order index: insufficient data")
	}

	orderIDs := make([]uint64, count)
	offset := 4
	for i := 0; i < count; i++ {
		orderIDs[i] = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
	}

	return orderIDs, nil
}

// MarshalOrderLocator serializes an order locator (side + price varint).
// Format: [Side:1][Price:varint]
func MarshalOrderLocator(side core.Side, price uint64) []byte {
	buf := make([]byte, 1+binary.MaxVarintLen64)
	buf[0] = byte(side)
	n := binary.PutUvarint(buf[1:], price)
	return buf[:1+n]
}

// UnmarshalOrderLocator deserializes an order locator (side + price).
func UnmarshalOrderLocator(data []byte) (core.Side, uint64, error) {
	if len(data) < 2 {
		return 0, 0, fmt.Errorf("invalid order locator: too short")
	}
	side := core.Side(data[0])
	price, n := binary.Uvarint(data[1:])
	if n <= 0 {
		return 0, 0, fmt.Errorf("invalid order locator: bad price varint")
	}
	return side, price, nil
}

// MarshalPriceList serializes a price list to bytes.
// Format: [Count:4][Price1:8][Price2:8]...
func MarshalPriceList(prices []uint64) ([]byte, error) {
	buf := make([]byte, 4+8*len(prices))

	binary.LittleEndian.PutUint32(buf[0:], uint32(len(prices)))

	offset := 4
	for _, p := range prices {
		binary.LittleEndian.PutUint64(buf[offset:], p)
		offset += 8
	}

	return buf, nil
}

// UnmarshalPriceList deserializes a price list from bytes.
func UnmarshalPriceList(data []byte) ([]uint64, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("invalid price list: too short")
	}

	count := int(binary.LittleEndian.Uint32(data[0:]))
	if len(data) < 4+8*count {
		return nil, fmt.Errorf("invalid price list: insufficient data")
	}

	prices := make([]uint64, count)
	offset := 4
	for i := 0; i < count; i++ {
		prices[i] = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
	}

	return prices, nil
}

// MarshalPriceListDelta serializes a sorted price list using delta encoding.
// Format: [Count:varint][Base:varint][Delta1:varint][Delta2:varint]...
// Much more compact for dense price levels (~2-3 bytes per price vs 8).
func MarshalPriceListDelta(prices []uint64) ([]byte, error) {
	if len(prices) == 0 {
		return nil, nil
	}

	// Estimate capacity: count(1-2) + base(1-10) + deltas(avg 1-3 each)
	buf := make([]byte, 0, 12+len(prices)*3)

	// Write count as varint
	buf = binary.AppendUvarint(buf, uint64(len(prices)))

	// Write base price
	buf = binary.AppendUvarint(buf, prices[0])

	// Write deltas
	for i := 1; i < len(prices); i++ {
		delta := prices[i] - prices[i-1]
		buf = binary.AppendUvarint(buf, delta)
	}

	return buf, nil
}

// UnmarshalPriceListDelta deserializes a delta-encoded price list.
func UnmarshalPriceListDelta(data []byte) ([]uint64, error) {
	if len(data) == 0 {
		return nil, nil
	}

	// Read count
	count, n := binary.Uvarint(data)
	if n <= 0 {
		return nil, fmt.Errorf("invalid delta price list: bad count")
	}
	data = data[n:]

	if count == 0 {
		return nil, nil
	}

	prices := make([]uint64, count)

	// Read base price
	base, n := binary.Uvarint(data)
	if n <= 0 {
		return nil, fmt.Errorf("invalid delta price list: bad base")
	}
	data = data[n:]
	prices[0] = base

	// Read deltas and reconstruct
	current := base
	for i := uint64(1); i < count; i++ {
		delta, n := binary.Uvarint(data)
		if n <= 0 {
			return nil, fmt.Errorf("invalid delta price list: bad delta at %d", i)
		}
		data = data[n:]
		current += delta
		prices[i] = current
	}

	return prices, nil
}

// MarshalAgg serializes aggregate data to bytes.
// Format: [Best:8][LevelCount:4][TotalQty:8][TotalNotional:8]
func MarshalAgg(best uint64, levelCount int, totalQty, totalNotional uint64) ([]byte, error) {
	buf := make([]byte, 8+4+8+8)

	binary.LittleEndian.PutUint64(buf[0:], best)
	binary.LittleEndian.PutUint32(buf[8:], uint32(levelCount))
	binary.LittleEndian.PutUint64(buf[12:], totalQty)
	binary.LittleEndian.PutUint64(buf[20:], totalNotional)

	return buf, nil
}

// UnmarshalAgg deserializes aggregate data from bytes.
func UnmarshalAgg(data []byte) (best uint64, levelCount int, totalQty, totalNotional uint64, err error) {
	if len(data) < 28 {
		return 0, 0, 0, 0, fmt.Errorf("invalid agg data: too short")
	}

	best = binary.LittleEndian.Uint64(data[0:])
	levelCount = int(binary.LittleEndian.Uint32(data[8:]))
	totalQty = binary.LittleEndian.Uint64(data[12:])
	totalNotional = binary.LittleEndian.Uint64(data[20:])

	return best, levelCount, totalQty, totalNotional, nil
}

// MarshalDone serializes a Done result to bytes.
// Format: [OrderBytes:N][TradeCount:4][Trade1Bytes:N]...[Left:8][Processed:8][Stored:1]
func MarshalDone(d *core.Done) ([]byte, error) {
	if d == nil {
		return nil, nil
	}

	// Marshal the order
	orderBytes, err := MarshalOrder(d.Order)
	if err != nil {
		return nil, err
	}

	// Calculate size
	size := 4 + len(orderBytes) + 4 + 8 + 8 + 1 // orderLen + order + tradeCount + left + processed + stored

	// Add trade sizes
	tradeBytes := make([][]byte, len(d.Trades))
	for i, trade := range d.Trades {
		tb, err := MarshalOrder(trade)
		if err != nil {
			return nil, err
		}
		tradeBytes[i] = tb
		size += 4 + len(tb) // tradeLen + trade
	}

	buf := make([]byte, size)
	offset := 0

	// Order length + bytes
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(orderBytes)))
	offset += 4
	copy(buf[offset:], orderBytes)
	offset += len(orderBytes)

	// Trade count
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(d.Trades)))
	offset += 4

	// Trades
	for _, tb := range tradeBytes {
		binary.LittleEndian.PutUint32(buf[offset:], uint32(len(tb)))
		offset += 4
		copy(buf[offset:], tb)
		offset += len(tb)
	}

	// Left
	binary.LittleEndian.PutUint64(buf[offset:], d.Left)
	offset += 8

	// Processed
	binary.LittleEndian.PutUint64(buf[offset:], d.Processed)
	offset += 8

	// Stored
	if d.Stored {
		buf[offset] = 1
	} else {
		buf[offset] = 0
	}

	return buf, nil
}

// UnmarshalDone deserializes a Done result from bytes.
func UnmarshalDone(data []byte) (*core.Done, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if len(data) < 4 {
		return nil, fmt.Errorf("invalid done data: too short")
	}

	d := &core.Done{}
	offset := 0

	// Order
	orderLen := int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	if offset+orderLen > len(data) {
		return nil, fmt.Errorf("invalid done data: order overflow")
	}
	order, err := UnmarshalOrder(data[offset : offset+orderLen])
	if err != nil {
		return nil, err
	}
	d.Order = order
	offset += orderLen

	// Trade count
	if offset+4 > len(data) {
		return nil, fmt.Errorf("invalid done data: missing trade count")
	}
	tradeCount := int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	// Trades
	d.Trades = make([]*core.Order, tradeCount)
	for i := 0; i < tradeCount; i++ {
		if offset+4 > len(data) {
			return nil, fmt.Errorf("invalid done data: missing trade length")
		}
		tradeLen := int(binary.LittleEndian.Uint32(data[offset:]))
		offset += 4
		if offset+tradeLen > len(data) {
			return nil, fmt.Errorf("invalid done data: trade overflow")
		}
		trade, err := UnmarshalOrder(data[offset : offset+tradeLen])
		if err != nil {
			return nil, err
		}
		d.Trades[i] = trade
		offset += tradeLen
	}

	// Left
	if offset+8 > len(data) {
		return nil, fmt.Errorf("invalid done data: missing left")
	}
	d.Left = binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	// Processed
	if offset+8 > len(data) {
		return nil, fmt.Errorf("invalid done data: missing processed")
	}
	d.Processed = binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	// Stored
	if offset >= len(data) {
		return nil, fmt.Errorf("invalid done data: missing stored")
	}
	d.Stored = data[offset] == 1

	return d, nil
}

// MarshalDepth serializes a Depth to bytes.
// Format: [BidCount:4][[Price:8][Qty:8]]...[AskCount:4][[Price:8][Qty:8]]...
func MarshalDepth(d *core.Depth) ([]byte, error) {
	if d == nil {
		return nil, nil
	}

	size := 4 + len(d.Bids)*16 + 4 + len(d.Asks)*16
	buf := make([]byte, size)
	offset := 0

	// Bids
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(d.Bids)))
	offset += 4
	for _, bid := range d.Bids {
		binary.LittleEndian.PutUint64(buf[offset:], bid[0]) // price
		offset += 8
		binary.LittleEndian.PutUint64(buf[offset:], bid[1]) // qty
		offset += 8
	}

	// Asks
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(d.Asks)))
	offset += 4
	for _, ask := range d.Asks {
		binary.LittleEndian.PutUint64(buf[offset:], ask[0]) // price
		offset += 8
		binary.LittleEndian.PutUint64(buf[offset:], ask[1]) // qty
		offset += 8
	}

	return buf, nil
}

// UnmarshalDepth deserializes a Depth from bytes.
func UnmarshalDepth(data []byte) (*core.Depth, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if len(data) < 8 {
		return nil, fmt.Errorf("invalid depth data: too short")
	}

	d := &core.Depth{}
	offset := 0

	// Bids
	bidCount := int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	if offset+bidCount*16 > len(data) {
		return nil, fmt.Errorf("invalid depth data: bids overflow")
	}
	d.Bids = make([][2]uint64, bidCount)
	for i := 0; i < bidCount; i++ {
		d.Bids[i][0] = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
		d.Bids[i][1] = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
	}

	// Asks
	if offset+4 > len(data) {
		return nil, fmt.Errorf("invalid depth data: missing ask count")
	}
	askCount := int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	if offset+askCount*16 > len(data) {
		return nil, fmt.Errorf("invalid depth data: asks overflow")
	}
	d.Asks = make([][2]uint64, askCount)
	for i := 0; i < askCount; i++ {
		d.Asks[i][0] = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
		d.Asks[i][1] = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
	}

	return d, nil
}

// Uint64ToBytes converts uint64 to big-endian bytes (for return values)
func Uint64ToBytes(v uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, v)
	return buf
}

// MarshalEngineState serializes the matching engine state to bytes.
// Format: [NextOrderID:8][HasLastPrice:1][LastPrice:8][OrderCount:4][Order1Len:4][Order1:N]...
func MarshalEngineState(nextOrderID uint64, lastPrice *uint64, orders []*core.Order) ([]byte, error) {
	// Calculate total size
	size := 8 + 1 + 8 + 4 // header

	orderBytes := make([][]byte, len(orders))
	for i, order := range orders {
		ob, err := MarshalOrder(order)
		if err != nil {
			return nil, err
		}
		orderBytes[i] = ob
		size += 4 + len(ob) // length prefix + order bytes
	}

	buf := make([]byte, size)
	offset := 0

	// NextOrderID
	binary.LittleEndian.PutUint64(buf[offset:], nextOrderID)
	offset += 8

	// LastPrice
	if lastPrice != nil {
		buf[offset] = 1
		offset++
		binary.LittleEndian.PutUint64(buf[offset:], *lastPrice)
	} else {
		buf[offset] = 0
		offset++
		binary.LittleEndian.PutUint64(buf[offset:], 0)
	}
	offset += 8

	// Order count
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(orders)))
	offset += 4

	// Orders
	for _, ob := range orderBytes {
		binary.LittleEndian.PutUint32(buf[offset:], uint32(len(ob)))
		offset += 4
		copy(buf[offset:], ob)
		offset += len(ob)
	}

	return buf, nil
}

// UnmarshalEngineState deserializes the matching engine state from bytes.
func UnmarshalEngineState(data []byte) (nextOrderID uint64, lastPrice *uint64, orders []*core.Order, err error) {
	if len(data) == 0 {
		return 1, nil, nil, nil
	}

	// Check minimum size
	if len(data) < 8+1+8+4 {
		return 0, nil, nil, fmt.Errorf("invalid engine state: too short")
	}

	offset := 0

	// NextOrderID
	nextOrderID = binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	// LastPrice
	if data[offset] == 1 {
		offset++
		price := binary.LittleEndian.Uint64(data[offset:])
		lastPrice = &price
	} else {
		offset++
	}
	offset += 8

	// Order count
	orderCount := int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	// Orders
	orders = make([]*core.Order, orderCount)
	for i := 0; i < orderCount; i++ {
		if offset+4 > len(data) {
			return 0, nil, nil, fmt.Errorf("invalid engine state: missing order length at index %d", i)
		}
		orderLen := int(binary.LittleEndian.Uint32(data[offset:]))
		offset += 4
		if offset+orderLen > len(data) {
			return 0, nil, nil, fmt.Errorf("invalid engine state: order overflow at index %d", i)
		}

		order, err := UnmarshalOrder(data[offset : offset+orderLen])
		if err != nil {
			return 0, nil, nil, fmt.Errorf("invalid engine state: order unmarshal error at index %d: %w", i, err)
		}
		orders[i] = order
		offset += orderLen
	}

	return nextOrderID, lastPrice, orders, nil
}

// ============================================================================
// Hot/Cold Order Splitting - Optimized for partial fills
// ============================================================================

// TopNLevels is the number of top levels to cache in aggregates for FOK optimization
const TopNLevels = 50

// MarshalOrderCold serializes the immutable fields of an order.
// Format: [Side:1][Type:1][Price:varint][Stop:varint][TIF:1][OCO:varint][UID:varint]
// Written once at order creation, never updated.
// Size: 7-44 bytes (typical: 12-18 bytes)
func MarshalOrderCold(o *core.Order) ([]byte, error) {
	if o == nil {
		return nil, nil
	}

	// Pre-allocate worst-case buffer
	// Side(1) + Type(1) + Price(10) + Stop(10) + TIF(1) + OCO(10) + UID(10) = 43 bytes
	buf := make([]byte, 43)
	offset := 0

	// Side (1 byte)
	buf[offset] = byte(o.Side)
	offset++

	// OrderType (1 byte)
	buf[offset] = byte(o.OrderType)
	offset++

	// Price (varint)
	n := binary.PutUvarint(buf[offset:], o.Price)
	offset += n

	// Stop (varint)
	n = binary.PutUvarint(buf[offset:], o.Stop)
	offset += n

	// TIF (1 byte)
	buf[offset] = byte(o.TIF)
	offset++

	// OCO (varint)
	n = binary.PutUvarint(buf[offset:], o.OCO)
	offset += n

	// UID (varint)
	n = binary.PutUvarint(buf[offset:], o.UID)
	offset += n

	return buf[:offset], nil
}

// UnmarshalOrderCold deserializes the immutable fields of an order.
// Requires orderID to be passed separately (from storage key).
func UnmarshalOrderCold(data []byte, orderID uint64) (*core.Order, error) {
	if len(data) == 0 {
		return nil, nil
	}

	if len(data) < 3 {
		return nil, fmt.Errorf("invalid cold order data: too short")
	}

	o := &core.Order{ID: orderID}
	offset := 0

	// Side (1 byte)
	o.Side = core.Side(data[offset])
	offset++

	// OrderType (1 byte)
	o.OrderType = core.OrderType(data[offset])
	offset++

	// Price (varint)
	price, n := binary.Uvarint(data[offset:])
	if n <= 0 {
		return nil, fmt.Errorf("invalid cold order data: bad price varint")
	}
	o.Price = price
	offset += n

	// Stop (varint)
	if offset >= len(data) {
		return nil, fmt.Errorf("invalid cold order data: missing stop")
	}
	stop, n := binary.Uvarint(data[offset:])
	if n <= 0 {
		return nil, fmt.Errorf("invalid cold order data: bad stop varint")
	}
	o.Stop = stop
	offset += n

	// TIF (1 byte)
	if offset >= len(data) {
		return nil, fmt.Errorf("invalid cold order data: missing tif")
	}
	o.TIF = core.TIF(data[offset])
	offset++

	// OCO (varint)
	if offset >= len(data) {
		return nil, fmt.Errorf("invalid cold order data: missing oco")
	}
	oco, n := binary.Uvarint(data[offset:])
	if n <= 0 {
		return nil, fmt.Errorf("invalid cold order data: bad oco varint")
	}
	o.OCO = oco
	offset += n

	// UID (varint)
	if offset >= len(data) {
		return nil, fmt.Errorf("invalid cold order data: missing uid")
	}
	uid, n := binary.Uvarint(data[offset:])
	if n <= 0 {
		return nil, fmt.Errorf("invalid cold order data: bad uid varint")
	}
	o.UID = uid

	return o, nil
}

// MarshalOrderHot serializes the mutable fields of an order.
// Format: [Quantity:varint]
// Updated on every partial fill.
// Size: 1-10 bytes (typical: 2-4 bytes)
func MarshalOrderHot(quantity uint64) []byte {
	buf := make([]byte, 10) // max varint size
	n := binary.PutUvarint(buf, quantity)
	return buf[:n]
}

// UnmarshalOrderHot deserializes the mutable fields of an order.
func UnmarshalOrderHot(data []byte) (uint64, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("invalid hot order data: empty")
	}

	qty, n := binary.Uvarint(data)
	if n <= 0 {
		return 0, fmt.Errorf("invalid hot order data: bad quantity varint")
	}

	return qty, nil
}

// ============================================================================
// Columnar Level Storage - IDs only, orders stored separately
// ============================================================================

// MarshalLevelColumnar serializes a price level with only order IDs (columnar format).
// Format: [Count:4][TotalQty:8][ID1:varint][ID2:varint]...
// Orders themselves are stored in hot/cold split format.
// Size: 12 bytes header + ~3 bytes per order ID (typical)
func MarshalLevelColumnar(orderIDs []uint64, totalQty uint64) ([]byte, error) {
	if len(orderIDs) == 0 {
		buf := make([]byte, 12)
		binary.LittleEndian.PutUint32(buf[0:], 0)
		binary.LittleEndian.PutUint64(buf[4:], totalQty)
		return buf, nil
	}

	// Pre-allocate: header(12) + per-ID(max 10 bytes varint)
	buf := make([]byte, 12+len(orderIDs)*10)
	offset := 0

	// Write header
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(orderIDs)))
	offset += 4
	binary.LittleEndian.PutUint64(buf[offset:], totalQty)
	offset += 8

	// Write order IDs as varints
	for _, id := range orderIDs {
		n := binary.PutUvarint(buf[offset:], id)
		offset += n
	}

	return buf[:offset], nil
}

// UnmarshalLevelColumnar deserializes a columnar price level.
// Returns order IDs and total quantity. Orders must be loaded separately.
func UnmarshalLevelColumnar(data []byte) (orderIDs []uint64, totalQty uint64, err error) {
	if len(data) < 12 {
		return nil, 0, fmt.Errorf("invalid columnar level data: too short")
	}

	offset := 0

	// Read header
	count := int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	totalQty = binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	if count == 0 {
		return nil, totalQty, nil
	}

	// Read order IDs
	orderIDs = make([]uint64, count)
	for i := 0; i < count; i++ {
		if offset >= len(data) {
			return nil, 0, fmt.Errorf("invalid columnar level data: missing order ID at index %d", i)
		}
		id, n := binary.Uvarint(data[offset:])
		if n <= 0 {
			return nil, 0, fmt.Errorf("invalid columnar level data: bad order ID varint at index %d", i)
		}
		orderIDs[i] = id
		offset += n
	}

	return orderIDs, totalQty, nil
}

// ============================================================================
// Enhanced Aggregate Cache with Top 50 Levels
// ============================================================================

// SideAggregates holds pre-computed aggregate data for one side of the book.
// Used for FOK (Fill-or-Kill) optimization without loading all levels.
type SideAggregates struct {
	Best          uint64     // Best price (0 if empty)
	LevelCount    uint32     // Number of price levels
	TotalQty      uint64     // Sum of all quantities
	TotalNotional uint64     // Sum of qty * price
	TopNCount     uint8      // Number of valid entries in TopN arrays (0-50)
	TopNPrices    [50]uint64 // Prices of top N levels (best first)
	TopNQty       [50]uint64 // Quantities at top N levels
}

// MarshalAggTop50 serializes aggregate data with top 50 levels to bytes.
// Format: [Best:8][LevelCount:4][TotalQty:8][TotalNotional:8][TopNCount:1][TopNPrices:N*varint][TopNQty:N*varint]
// Size: 29 bytes header + ~6 bytes per cached level (typical)
func MarshalAggTop50(agg *SideAggregates) ([]byte, error) {
	if agg == nil {
		return nil, nil
	}

	// Pre-allocate: header(29) + prices(50*10) + qtys(50*10) = 1029 max
	buf := make([]byte, 1029)
	offset := 0

	// Fixed header
	binary.LittleEndian.PutUint64(buf[offset:], agg.Best)
	offset += 8
	binary.LittleEndian.PutUint32(buf[offset:], agg.LevelCount)
	offset += 4
	binary.LittleEndian.PutUint64(buf[offset:], agg.TotalQty)
	offset += 8
	binary.LittleEndian.PutUint64(buf[offset:], agg.TotalNotional)
	offset += 8
	buf[offset] = agg.TopNCount
	offset++

	// Top N prices (varint encoded)
	for i := uint8(0); i < agg.TopNCount; i++ {
		n := binary.PutUvarint(buf[offset:], agg.TopNPrices[i])
		offset += n
	}

	// Top N quantities (varint encoded)
	for i := uint8(0); i < agg.TopNCount; i++ {
		n := binary.PutUvarint(buf[offset:], agg.TopNQty[i])
		offset += n
	}

	return buf[:offset], nil
}

// UnmarshalAggTop50 deserializes aggregate data with top 50 levels from bytes.
func UnmarshalAggTop50(data []byte) (*SideAggregates, error) {
	if len(data) < 29 {
		return nil, fmt.Errorf("invalid agg data: too short")
	}

	agg := &SideAggregates{}
	offset := 0

	// Fixed header
	agg.Best = binary.LittleEndian.Uint64(data[offset:])
	offset += 8
	agg.LevelCount = binary.LittleEndian.Uint32(data[offset:])
	offset += 4
	agg.TotalQty = binary.LittleEndian.Uint64(data[offset:])
	offset += 8
	agg.TotalNotional = binary.LittleEndian.Uint64(data[offset:])
	offset += 8
	agg.TopNCount = data[offset]
	offset++

	if agg.TopNCount > TopNLevels {
		agg.TopNCount = TopNLevels
	}

	// Top N prices
	for i := uint8(0); i < agg.TopNCount; i++ {
		if offset >= len(data) {
			return nil, fmt.Errorf("invalid agg data: missing price at index %d", i)
		}
		price, n := binary.Uvarint(data[offset:])
		if n <= 0 {
			return nil, fmt.Errorf("invalid agg data: bad price varint at index %d", i)
		}
		agg.TopNPrices[i] = price
		offset += n
	}

	// Top N quantities
	for i := uint8(0); i < agg.TopNCount; i++ {
		if offset >= len(data) {
			return nil, fmt.Errorf("invalid agg data: missing qty at index %d", i)
		}
		qty, n := binary.Uvarint(data[offset:])
		if n <= 0 {
			return nil, fmt.Errorf("invalid agg data: bad qty varint at index %d", i)
		}
		agg.TopNQty[i] = qty
		offset += n
	}

	return agg, nil
}

// TopNSum returns the sum of quantities in the top N levels.
// Used for quick FOK validation without loading individual levels.
func (agg *SideAggregates) TopNSum() uint64 {
	var sum uint64
	for i := uint8(0); i < agg.TopNCount; i++ {
		sum += agg.TopNQty[i]
	}
	return sum
}
