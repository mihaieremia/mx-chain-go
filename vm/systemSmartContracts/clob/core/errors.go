package core

import "errors"

var (
	ErrInvalidOrderType    = errors.New("invalid order type")
	ErrOrderNotFound       = errors.New("order not found")
	ErrNilOrderBook        = errors.New("order book is nil")
	ErrMaxOrdersReached    = errors.New("maximum number of open orders reached")
	ErrMaxPriceLevels      = errors.New("maximum number of price levels reached on this side")
	ErrMinOrderQty         = errors.New("minimum order quantity not satisfied")
	ErrMaxOrderQty         = errors.New("maximum order quantity exceeded")
	ErrMinPrice            = errors.New("minimum price not satisfied")
	ErrMaxPrice            = errors.New("maximum price exceeded")
	ErrMinNotional         = errors.New("notional below minimum")
	ErrInvalidPriceTick    = errors.New("price not aligned to tick size")
	ErrInvalidQtyStep      = errors.New("quantity not aligned to step size")
	ErrMaxOrdersAccount    = errors.New("maximum number of open orders for account reached")
	ErrInsufficientBalance = errors.New("insufficient available balance")
	ErrInsufficientLocked  = errors.New("insufficient locked balance")
	ErrFillExceedsQuantity = errors.New("fill quantity exceeds remaining order quantity")
	ErrZeroQuantity        = errors.New("order quantity must be greater than zero")
	ErrZeroPrice           = errors.New("limit order price must be greater than zero")
	ErrOverflow            = errors.New("arithmetic overflow in order calculation")
	ErrFOKNotFilled        = errors.New("fill-or-kill order could not be completely filled")
	ErrIOCPartialCancel    = errors.New("immediate-or-cancel order partially filled, remainder cancelled")
	ErrPostOnlyWouldMatch  = errors.New("post-only order would match immediately")
	ErrPriceDeviationLimit = errors.New("order price exceeds maximum deviation from last price")

	// Internal invariant errors - indicate data corruption or logic bugs
	ErrLocatorNil    = errors.New("locator is nil for order - internal invariant violation")
	ErrQueueNotFound = errors.New("queue not found for price level - internal invariant violation")
)
