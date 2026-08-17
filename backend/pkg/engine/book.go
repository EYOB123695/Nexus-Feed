package engine

import (
	"time"

	"nexus-feed/backend/pkg/types"
)

// OrderBook represents a high-performance L2 order book for a single exchange and symbol.
type OrderBook struct {
	Exchange    string
	Symbol      string
	bids        *SkipList
	asks        *SkipList
	lastUpdated time.Time
}

// NewOrderBook creates a new OrderBook instance for a specific exchange and symbol.
func NewOrderBook(exchange, symbol string) *OrderBook {
	return &OrderBook{
		Exchange:    exchange,
		Symbol:      symbol,
		bids:        NewSkipList(false), // false = Descending order (highest buyer first -> Best Bid at head)
		asks:        NewSkipList(true),  // true  = Ascending order (lowest seller first -> Best Ask at head)
		lastUpdated: time.Now(),
	}
}

// ApplyTick updates the order book with an incoming market tick.
// If Quantity <= 0, the price level is removed (cancellation).
// If Quantity > 0, the price level is updated or inserted in O(log N).
func (ob *OrderBook) ApplyTick(tick types.MarketTick) {
	ob.lastUpdated = tick.Timestamp
	if ob.lastUpdated.IsZero() {
		ob.lastUpdated = time.Now()
	}

	updatedAtMicros := ob.lastUpdated.UnixNano() / 1000

	// Handle BID (Buy side)
	if tick.Side == types.SideBid {
		if tick.Quantity <= 0 {
			ob.bids.Delete(tick.Price)
		} else {
			ob.bids.InsertOrUpdate(tick.Price, tick.Quantity, ob.Exchange, updatedAtMicros)
		}
		return
	}

	// Handle ASK (Sell side)
	if tick.Side == types.SideAsk {
		if tick.Quantity <= 0 {
			ob.asks.Delete(tick.Price)
		} else {
			ob.asks.InsertOrUpdate(tick.Price, tick.Quantity, ob.Exchange, updatedAtMicros)
		}
	}
}

// GetBestBid returns the highest price willing to buy and its quantity in instant O(1) time.
func (ob *OrderBook) GetBestBid() (float64, float64) {
	price, qty, exists := ob.bids.PeekBest()
	if !exists {
		return 0.0, 0.0
	}
	return price, qty
}

// GetBestAsk returns the lowest price willing to sell and its quantity in instant O(1) time.
func (ob *OrderBook) GetBestAsk() (float64, float64) {
	price, qty, exists := ob.asks.PeekBest()
	if !exists {
		return 0.0, 0.0
	}
	return price, qty
}

// GetSnapshot extracts the top K levels of bids and asks in O(K) time without sorting.
func (ob *OrderBook) GetSnapshot(depth int) types.OrderBookSnapshot {
	return types.OrderBookSnapshot{
		Symbol:    ob.Symbol,
		Exchange:  ob.Exchange,
		Bids:      ob.bids.GetTopK(depth),
		Asks:      ob.asks.GetTopK(depth),
		Timestamp: ob.lastUpdated,
	}
}

// DepthCount returns the total number of active bid and ask price levels.
func (ob *OrderBook) DepthCount() (int, int) {
	return ob.bids.Count(), ob.asks.Count()
}
