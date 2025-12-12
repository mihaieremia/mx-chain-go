package core

// ScaleFactor is the fixed-point multiplier used for price and quantity.
// With DefaultDecimals=2, a value of 100 means 100.10 is sent as 10010.
const ScaleFactor uint64 = 100

// DefaultDecimals defines how many decimal places callers should scale by.
const DefaultDecimals = 2

// OrderType of the Order
type OrderType uint8

// Different order types
const (
	TypeMarket    OrderType = 0
	TypeLimit     OrderType = 1
	TypeStopLimit OrderType = 2
)

// ParseOrderType converts a byte slice to OrderType
func ParseOrderType(s []byte) OrderType {
	switch string(s) {
	case "MARKET":
		return TypeMarket
	case "LIMIT":
		return TypeLimit
	case "STOP-LIMIT":
		return TypeStopLimit
	default:
		return TypeMarket // default fallback
	}
}

// Side of the Order
type Side byte

// Different order sides
const (
	SideBuy  Side = 'B'
	SideSell Side = 'S'
)

// TIF (Time In Force) of the Order
type TIF uint8

// Different order TIF
const (
	TIFGoodTillCancel    TIF = 0
	TIFFillOrKill        TIF = 1
	TIFImmediateOrCancel TIF = 2
	TIFPostOnly          TIF = 3
)

// ParseTIF converts a byte slice to TIF
func ParseTIF(s []byte) TIF {
	switch string(s) {
	case "GTC":
		return TIFGoodTillCancel
	case "FOK":
		return TIFFillOrKill
	case "IOC":
		return TIFImmediateOrCancel
	case "POST_ONLY", "PO":
		return TIFPostOnly
	default:
		return TIFGoodTillCancel // default fallback
	}
}

// Endpoint constants
const (
	ProcessOrderEndpoint = "processOrder"
	CancelOrderEndpoint  = "cancelOrder"
	GetOrderEndpoint     = "getOrder"
	GetDepthEndpoint     = "getDepth"
	MatchOrdersEndpoint  = "matchOrders"
)
