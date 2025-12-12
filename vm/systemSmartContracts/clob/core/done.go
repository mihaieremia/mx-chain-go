package core

// Done contains information about processed order
type Done struct {
	Order          *Order
	Trades         []*Order
	Canceled       []string  // IDs of cancelled orders (as strings)
	CanceledOrders []*Order  // Full order objects for cancelled OCO orders (for balance unlocking)
	Activated      []string
	Left           uint64
	Processed      uint64
	Stored         bool
}

// NewDone creates a Done with pre-allocated slices for reduced allocations.
// tradesCapacity: expected number of trade pairs (typically 2-8 for partial fills)
// ocoCapacity: max OCO cancellations (typically 2)
func NewDone(order *Order, tradesCapacity, ocoCapacity int) *Done {
	return &Done{
		Order:          order,
		Trades:         make([]*Order, 0, tradesCapacity*2), // trades come in pairs
		Canceled:       make([]string, 0, ocoCapacity),
		CanceledOrders: make([]*Order, 0, ocoCapacity),
	}
}
