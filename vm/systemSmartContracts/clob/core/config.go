package core

// Default limits to bound book growth and gas/time usage.
// Aligned with Binance-style constraints for 200ms block times.
const (
	DefaultMaxOrders             = 20000                       // ~500KB storage cap per pair
	DefaultMaxPriceLevelsPerSide = 1000                        // 1000 levels per side (vs Binance 5000+)
	DefaultMinOrderQty           = 1                           // scaled units; with 2 decimals => 0.01
	DefaultMaxOrderQty           = 1_000_000_000 * ScaleFactor // 1e9.00
	DefaultMinPrice              = 1                           // scaled units; with 2 decimals => 0.01
	DefaultMaxPrice              = 1_000_000_000 * ScaleFactor
	DefaultMinNotional           = 0                            // Configurable per pair (set based on decimal places)
	DefaultQtyStep               = 1
	DefaultPriceTick             = 1
	DefaultMaxOrdersPerAccount   = 100                         // Binance: 200, more conservative for on-chain
	DefaultMaxPriceDeviation     = 5000                        // 50% max deviation from last price (basis points)
)

// Config holds tunable limits for the order book.
type Config struct {
	MaxOrders             int
	MaxPriceLevelsPerSide int
	MinOrderQty           uint64
	MaxOrderQty           uint64
	MinPrice              uint64
	MaxPrice              uint64
	MinNotional           uint64
	QtyStep               uint64
	PriceTick             uint64
	MaxOrdersPerAccount   int
	Decimals              int
	MaxPriceDeviation     uint16 // Max deviation from last price in basis points (e.g., 5000 = 50%)
}

// DefaultConfig returns the default limits.
func DefaultConfig() *Config {
	return &Config{
		MaxOrders:             DefaultMaxOrders,
		MaxPriceLevelsPerSide: DefaultMaxPriceLevelsPerSide,
		MinOrderQty:           DefaultMinOrderQty,
		MaxOrderQty:           DefaultMaxOrderQty,
		MinPrice:              DefaultMinPrice,
		MaxPrice:              DefaultMaxPrice,
		MinNotional:           DefaultMinNotional,
		QtyStep:               DefaultQtyStep,
		PriceTick:             DefaultPriceTick,
		MaxOrdersPerAccount:   DefaultMaxOrdersPerAccount,
		Decimals:              DefaultDecimals,
		MaxPriceDeviation:     DefaultMaxPriceDeviation,
	}
}
