package engine

import (
	"math/rand"
	"time"

	"nexus-feed/backend/pkg/types"
)

const (
	// MaxLevel defines the maximum height of forward pointers in the skip list.
	// 16 levels easily supports up to 2^16 = 65,536 active price levels with O(log N) depth.
	MaxLevel = 16

	// Probability factor for random height generation (0.5 means 50% chance of ascending to next level).
	Probability = 0.5
)

// PriceNode represents a single price level in the skip list.
type PriceNode struct {
	Price     float64
	Quantity  float64
	Exchange  string
	UpdatedAt int64
	Forward   []*PriceNode // Forward pointers for each level in the skip list
}

// SkipList holds a multi-level sorted linked list of price nodes.
type SkipList struct {
	head        *PriceNode
	level       int
	isAscending bool // true for Asks (lowest first), false for Bids (highest first)
	count       int
	rnd         *rand.Rand
}

// NewSkipList initializes a new skip list with a given sort order.
// isAscending = true for Asks (ascending)
// isAscending = false for Bids (descending)
func NewSkipList(isAscending bool) *SkipList {
	source := rand.NewSource(time.Now().UnixNano())
	return &SkipList{
		head: &PriceNode{
			Forward: make([]*PriceNode, MaxLevel),
		},
		level:       1,
		isAscending: isAscending,
		count:       0,
		rnd:         rand.New(source),
	}
}

// shouldPrecede returns true if price 'a' should come before price 'b'.
func (sl *SkipList) shouldPrecede(a, b float64) bool {
	if sl.isAscending {
		return a < b // For Asks: lower price has higher priority
	}
	return a > b // For Bids: higher price has higher priority
}

// randomLevel generates a random height for a new node using the probability factor.
func (sl *SkipList) randomLevel() int {
	lvl := 1
	for sl.rnd.Float64() < Probability && lvl < MaxLevel {
		lvl++
	}
	return lvl
}

// InsertOrUpdate inserts a new price level or updates the quantity if it already exists in O(log N).
func (sl *SkipList) InsertOrUpdate(price, quantity float64, exchange string, updatedAt int64) *PriceNode {
	update := make([]*PriceNode, MaxLevel)
	current := sl.head

	// Step 1: Traverse from highest level down to find insert position
	for i := sl.level - 1; i >= 0; i-- {
		for current.Forward[i] != nil && sl.shouldPrecede(current.Forward[i].Price, price) {
			current = current.Forward[i]
		}
		update[i] = current
	}

	current = current.Forward[0]

	// Step 2: If price level already exists, update its quantity in-place
	if current != nil && current.Price == price {
		current.Quantity = quantity
		current.UpdatedAt = updatedAt
		if exchange != "" {
			current.Exchange = exchange
		}
		return current
	}

	// Step 3: Otherwise, create a new node with a random level height
	newNodeLevel := sl.randomLevel()
	if newNodeLevel > sl.level {
		for i := sl.level; i < newNodeLevel; i++ {
			update[i] = sl.head
		}
		sl.level = newNodeLevel
	}

	newNode := &PriceNode{
		Price:     price,
		Quantity:  quantity,
		Exchange:  exchange,
		UpdatedAt: updatedAt,
		Forward:   make([]*PriceNode, newNodeLevel),
	}

	// Step 4: Splice the new node into the forward pointers
	for i := 0; i < newNodeLevel; i++ {
		newNode.Forward[i] = update[i].Forward[i]
		update[i].Forward[i] = newNode
	}

	sl.count++
	return newNode
}

// Delete removes a price level from the skip list in O(log N) time.
func (sl *SkipList) Delete(price float64) bool {
	update := make([]*PriceNode, MaxLevel)
	current := sl.head

	for i := sl.level - 1; i >= 0; i-- {
		for current.Forward[i] != nil && sl.shouldPrecede(current.Forward[i].Price, price) {
			current = current.Forward[i]
		}
		update[i] = current
	}

	current = current.Forward[0]

	// Node found, unlink it
	if current != nil && current.Price == price {
		for i := 0; i < sl.level; i++ {
			if update[i].Forward[i] != current {
				break
			}
			update[i].Forward[i] = current.Forward[i]
		}

		// Adjust the current list level if highest levels became empty
		for sl.level > 1 && sl.head.Forward[sl.level-1] == nil {
			sl.level--
		}

		sl.count--
		return true
	}

	return false
}

// PeekBest returns the top of the book (Best Bid or Best Ask) in instant O(1) time.
func (sl *SkipList) PeekBest() (float64, float64, bool) {
	first := sl.head.Forward[0]
	if first == nil {
		return 0.0, 0.0, false
	}
	return first.Price, first.Quantity, true
}

// GetTopK retrieves the top K price levels in sorted order in O(K) time without sorting.
func (sl *SkipList) GetTopK(k int) []types.PriceLevel {
	if k <= 0 {
		return nil
	}

	levels := make([]types.PriceLevel, 0, k)
	current := sl.head.Forward[0]

	for current != nil && len(levels) < k {
		levels = append(levels, types.PriceLevel{
			Price:     current.Price,
			Quantity:  current.Quantity,
			Exchange:  current.Exchange,
			UpdatedAt: current.UpdatedAt,
		})
		current = current.Forward[0]
	}

	return levels
}

// Count returns the number of active price levels.
func (sl *SkipList) Count() int {
	return sl.count
}
