package book

import (
	"github.com/google/btree"
)

// PriceTree provides O(log n) price level management using a B-tree.
// This replaces the sorted slice approach which had O(n) insertions/deletions.
type PriceTree struct {
	tree       *btree.BTreeG[uint64]
	descending bool // true for bids (highest first), false for asks (lowest first)
}

// NewPriceTree creates a new price tree.
// descending=true for bids (highest price first), false for asks (lowest price first)
func NewPriceTree(descending bool) *PriceTree {
	less := func(a, b uint64) bool { return a < b }
	return &PriceTree{
		tree:       btree.NewG[uint64](32, less), // degree 32 is good for cache efficiency
		descending: descending,
	}
}

// Insert adds a price to the tree. O(log n)
func (pt *PriceTree) Insert(price uint64) {
	pt.tree.ReplaceOrInsert(price)
}

// Delete removes a price from the tree. O(log n)
func (pt *PriceTree) Delete(price uint64) {
	pt.tree.Delete(price)
}

// Has checks if a price exists. O(log n)
func (pt *PriceTree) Has(price uint64) bool {
	_, ok := pt.tree.Get(price)
	return ok
}

// Len returns the number of prices. O(1)
func (pt *PriceTree) Len() int {
	return pt.tree.Len()
}

// Best returns the best price (highest for bids, lowest for asks). O(log n)
func (pt *PriceTree) Best() (uint64, bool) {
	if pt.tree.Len() == 0 {
		return 0, false
	}
	if pt.descending {
		// Bids: highest price is best
		price, ok := pt.tree.Max()
		return price, ok
	}
	// Asks: lowest price is best
	price, ok := pt.tree.Min()
	return price, ok
}

// Iterate calls fn for each price in order (best to worst). O(n)
func (pt *PriceTree) Iterate(fn func(price uint64) bool) {
	if pt.descending {
		// Bids: iterate highest to lowest
		pt.tree.Descend(func(price uint64) bool {
			return fn(price)
		})
	} else {
		// Asks: iterate lowest to highest
		pt.tree.Ascend(func(price uint64) bool {
			return fn(price)
		})
	}
}

// ToSlice returns all prices as a slice in order (best to worst). O(n)
func (pt *PriceTree) ToSlice() []uint64 {
	result := make([]uint64, 0, pt.tree.Len())
	pt.Iterate(func(price uint64) bool {
		result = append(result, price)
		return true
	})
	return result
}
