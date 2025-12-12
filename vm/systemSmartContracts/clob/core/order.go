package core

// Order represents a single order in the order book.
// Priority is determined by Order ID (lower ID = earlier = higher priority).
type Order struct {
	ID        uint64
	Side      Side
	OrderType OrderType
	Quantity  uint64
	Price     uint64
	Stop      uint64 // 0 means no stop
	TIF       TIF    // Time in force
	OCO       uint64 // 0 means no OCO link
	UID       uint64 // Virtual account ID (1, 2, 3...)

	// Intrusive list pointers for O(1) queue operations
	// These are not serialized - they're runtime state only
	Prev *Order
	Next *Order
}

// NewMarketOrder creates a new market order.
func NewMarketOrder(orderID uint64, side Side, quantity uint64) *Order {
	return &Order{
		ID:        orderID,
		Side:      side,
		OrderType: TypeMarket,
		Quantity:  quantity,
	}
}

// NewLimitOrder creates a new limit order.
func NewLimitOrder(orderID uint64, side Side, quantity, price uint64, tif TIF, oco uint64) *Order {
	return &Order{
		ID:        orderID,
		Side:      side,
		OrderType: TypeLimit,
		Quantity:  quantity,
		Price:     price,
		TIF:       tif,
		OCO:       oco,
	}
}

// NewStopLimitOrder creates a new stop-limit order.
func NewStopLimitOrder(orderID uint64, side Side, quantity, price, stop uint64, oco uint64) *Order {
	return &Order{
		ID:        orderID,
		Side:      side,
		OrderType: TypeStopLimit,
		Quantity:  quantity,
		Price:     price,
		Stop:      stop,
		OCO:       oco,
	}
}

// GetID returns the order ID.
func (o *Order) GetID() uint64 {
	return o.ID
}

// GetSide returns the order side.
func (o *Order) GetSide() Side {
	return o.Side
}

// GetType returns the order type.
func (o *Order) GetType() OrderType {
	return o.OrderType
}

// GetQuantity returns the order quantity.
func (o *Order) GetQuantity() uint64 {
	return o.Quantity
}

// GetPrice returns the order price.
func (o *Order) GetPrice() uint64 {
	return o.Price
}

// GetStop returns the order stop price.
func (o *Order) GetStop() uint64 {
	return o.Stop
}

// GetTIF returns the time in force.
func (o *Order) GetTIF() TIF {
	return o.TIF
}

// GetOCO returns the OCO order ID (0 means no OCO link).
func (o *Order) GetOCO() uint64 {
	return o.OCO
}

// GetUID returns the virtual account ID.
func (o *Order) GetUID() uint64 {
	return o.UID
}

// SetUID sets the virtual account ID.
func (o *Order) SetUID(uid uint64) {
	o.UID = uid
}

// IsStopOrder returns true if the order is a stop order.
func (o *Order) IsStopOrder() bool {
	return o.OrderType == TypeStopLimit
}

// IsFilled returns true if the order is completely filled.
func (o *Order) IsFilled() bool {
	return o.Quantity == 0
}

// Fill fills the order with the given quantity.
// Returns error if fill quantity exceeds remaining order quantity.
func (o *Order) Fill(quantity uint64) error {
	if quantity > o.Quantity {
		return ErrFillExceedsQuantity
	}
	o.Quantity -= quantity
	return nil
}

// DeepCopy creates a deep copy of the order.
// Note: Prev/Next pointers are NOT copied - they remain nil in the copy.
func (o *Order) DeepCopy() *Order {
	return &Order{
		ID:        o.ID,
		Side:      o.Side,
		OrderType: o.OrderType,
		Quantity:  o.Quantity,
		Price:     o.Price,
		Stop:      o.Stop,
		TIF:       o.TIF,
		OCO:       o.OCO,
		UID:       o.UID,
		// Prev and Next are intentionally nil - runtime-only pointers
	}
}
