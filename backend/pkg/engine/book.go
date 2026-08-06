package engine

import (
	"sort"
	"sync"
	"time"

	"nexus-feed/backend/pkg/types"
)

type OrderBook struct {
	Exchange    string
	Symbol      string
	bids        map[float64]float64
	asks        map[float64]float64
	mu          sync.RWMutex
	lastUpdated time.Time
}

func NewOrderBook(exchange, symbol string) *OrderBook {
	return &OrderBook{
		Exchange:    exchange,
		Symbol:      symbol,
		bids:        make(map[float64]float64),
		asks:        make(map[float64]float64),
		lastUpdated: time.Now(),
	}
}

func (ob *OrderBook) ApplyTick(tick types.MarketTick) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	ob.lastUpdated = time.Now()
	targetMap := ob.bids
	if tick.Side == types.SideAsk {
		targetMap = ob.asks
	}

	if tick.Quantity <= 0 {
		delete(targetMap, tick.Price)
	} else {
		targetMap[tick.Price] = tick.Quantity
	}
}

func (ob *OrderBook) GetBestBid() (float64, float64) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	bestPrice := 0.0
	bestQty := 0.0
	for price, qty := range ob.bids {
		if price > bestPrice {
			bestPrice = price
			bestQty = qty
		}
	}
	return bestPrice, bestQty
}

func (ob *OrderBook) GetBestAsk() (float64, float64) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	bestPrice := 0.0
	bestQty := 0.0
	for price, qty := range ob.asks {
		if bestPrice == 0.0 || price < bestPrice {
			bestPrice = price
			bestQty = qty
		}
	}
	return bestPrice, bestQty
}

func (ob *OrderBook) GetSnapshot(depth int) types.OrderBookSnapshot {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	bidLevels := make([]types.PriceLevel, 0, len(ob.bids))
	for p, q := range ob.bids {
		bidLevels = append(bidLevels, types.PriceLevel{
			Price:     p,
			Quantity:  q,
			Exchange:  ob.Exchange,
			UpdatedAt: ob.lastUpdated.UnixNano() / 1000,
		})
	}
	sort.Slice(bidLevels, func(i, j int) bool {
		return bidLevels[i].Price > bidLevels[j].Price
	})

	askLevels := make([]types.PriceLevel, 0, len(ob.asks))
	for p, q := range ob.asks {
		askLevels = append(askLevels, types.PriceLevel{
			Price:     p,
			Quantity:  q,
			Exchange:  ob.Exchange,
			UpdatedAt: ob.lastUpdated.UnixNano() / 1000,
		})
	}
	sort.Slice(askLevels, func(i, j int) bool {
		return askLevels[i].Price < askLevels[j].Price
	})

	if len(bidLevels) > depth {
		bidLevels = bidLevels[:depth]
	}
	if len(askLevels) > depth {
		askLevels = askLevels[:depth]
	}

	return types.OrderBookSnapshot{
		Symbol:    ob.Symbol,
		Exchange:  ob.Exchange,
		Bids:      bidLevels,
		Asks:      askLevels,
		Timestamp: ob.lastUpdated,
	}
}
