package core

// Depth represents the order book depth.
type Depth struct {
	Bids [][2]uint64
	Asks [][2]uint64
}

// NewDepth creates a new instance of Depth.
func NewDepth() *Depth {
	return &Depth{
		Bids: make([][2]uint64, 0),
		Asks: make([][2]uint64, 0),
	}
}
