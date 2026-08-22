package engine

import (
	"math"
	"time"

	"nexus-feed/backend/pkg/types"
)

// ConsolidatedOrderBook aggregates order books across multiple exchanges for a single symbol.
type ConsolidatedOrderBook struct {
	Symbol      string
	books       map[string]*OrderBook
	globalBids  *SkipList
	globalAsks  *SkipList
	lastUpdated time.Time
}

// NewConsolidatedOrderBook creates a new consolidated order book for a symbol.
func NewConsolidatedOrderBook(symbol string) *ConsolidatedOrderBook {
	return &ConsolidatedOrderBook{
		Symbol:      symbol,
		books:       make(map[string]*OrderBook),
		globalBids:  NewSkipList(false), // Descending for Bids (Global Best Bid at head)
		globalAsks:  NewSkipList(true),  // Ascending for Asks (Global Best Ask at head)
		lastUpdated: time.Now(),
	}
}

// GetOrAddExchangeBook retrieves the OrderBook for an exchange or registers a new one.
func (cb *ConsolidatedOrderBook) GetOrAddExchangeBook(exchange string) *OrderBook {
	book, exists := cb.books[exchange]
	if !exists {
		book = NewOrderBook(exchange, cb.Symbol)
		cb.books[exchange] = book
	}
	return book
}

// ProcessTick applies an incoming tick to both the specific exchange book and the global book.
func (cb *ConsolidatedOrderBook) ProcessTick(tick types.MarketTick) (types.ConsolidatedBook, *types.ArbitrageOpportunity) {
	cb.lastUpdated = tick.Timestamp
	if cb.lastUpdated.IsZero() {
		cb.lastUpdated = time.Now()
	}

	updatedAtMicros := cb.lastUpdated.UnixNano() / 1000

	exBook := cb.GetOrAddExchangeBook(tick.Exchange)
	exBook.ApplyTick(tick)

	if tick.Side == types.SideBid {
		if tick.Quantity <= 0 {
			cb.globalBids.Delete(tick.Price)
		} else {
			cb.globalBids.InsertOrUpdate(tick.Price, tick.Quantity, tick.Exchange, updatedAtMicros)
		}
	} else if tick.Side == types.SideAsk {
		if tick.Quantity <= 0 {
			cb.globalAsks.Delete(tick.Price)
		} else {
			cb.globalAsks.InsertOrUpdate(tick.Price, tick.Quantity, tick.Exchange, updatedAtMicros)
		}
	}

	bestBidPrice, _, _ := cb.globalBids.PeekBest()
	bestAskPrice, _, _ := cb.globalAsks.PeekBest()

	var midPrice, spread, spreadPct float64
	if bestBidPrice > 0 && bestAskPrice > 0 {
		spread = bestAskPrice - bestBidPrice
		midPrice = (bestAskPrice + bestBidPrice) / 2.0
		if midPrice > 0 {
			spreadPct = (spread / midPrice) * 100.0
		}
	}

	snapshot := types.ConsolidatedBook{
		Symbol:      cb.Symbol,
		BestBid:     bestBidPrice,
		BestAsk:     bestAskPrice,
		MidPrice:    midPrice,
		Spread:      spread,
		SpreadPct:   spreadPct,
		Bids:        cb.globalBids.GetTopK(20),
		Asks:        cb.globalAsks.GetTopK(20),
		LastUpdated: cb.lastUpdated,
	}

	arb := cb.DetectArbitrage()
	return snapshot, arb
}

// DetectArbitrage scans top-of-book across all registered exchanges to find crossed markets.
func (cb *ConsolidatedOrderBook) DetectArbitrage() *types.ArbitrageOpportunity {
	if len(cb.books) < 2 {
		return nil
	}

	var bestBuyExchange string
	var lowestAskPrice float64 = math.MaxFloat64
	var lowestAskQty float64

	var bestSellExchange string
	var highestBidPrice float64 = 0.0
	var highestBidQty float64

	for exName, book := range cb.books {
		bidPrice, bidQty := book.GetBestBid()
		askPrice, askQty := book.GetBestAsk()

		if bidPrice > highestBidPrice {
			highestBidPrice = bidPrice
			highestBidQty = bidQty
			bestSellExchange = exName
		}

		if askPrice > 0 && askPrice < lowestAskPrice {
			lowestAskPrice = askPrice
			lowestAskQty = askQty
			bestBuyExchange = exName
		}
	}

	if highestBidPrice > 0 && lowestAskPrice < math.MaxFloat64 && bestSellExchange != bestBuyExchange {
		if highestBidPrice > lowestAskPrice {
			spreadMargin := highestBidPrice - lowestAskPrice
			profitPct := (spreadMargin / lowestAskPrice) * 100.0
			volumeAvail := math.Min(highestBidQty, lowestAskQty)

			return &types.ArbitrageOpportunity{
				Symbol:          cb.Symbol,
				BuyExchange:     bestBuyExchange,
				SellExchange:    bestSellExchange,
				BuyPrice:        lowestAskPrice,
				SellPrice:       highestBidPrice,
				SpreadMargin:    spreadMargin,
				ProfitPct:       profitPct,
				VolumeAvailable: volumeAvail,
				Timestamp:       cb.lastUpdated,
			}
		}
	}

	return nil
}

// GetSnapshot generates a ConsolidatedBook snapshot with the specified depth of bids and asks.
func (cb *ConsolidatedOrderBook) GetSnapshot(depth int) types.ConsolidatedBook {
	if depth <= 0 {
		depth = 20
	}

	bestBidPrice, _, _ := cb.globalBids.PeekBest()
	bestAskPrice, _, _ := cb.globalAsks.PeekBest()

	var midPrice, spread, spreadPct float64
	if bestBidPrice > 0 && bestAskPrice > 0 {
		spread = bestAskPrice - bestBidPrice
		midPrice = (bestAskPrice + bestBidPrice) / 2.0
		if midPrice > 0 {
			spreadPct = (spread / midPrice) * 100.0
		}
	}

	return types.ConsolidatedBook{
		Symbol:      cb.Symbol,
		BestBid:     bestBidPrice,
		BestAsk:     bestAskPrice,
		MidPrice:    midPrice,
		Spread:      spread,
		SpreadPct:   spreadPct,
		Bids:        cb.globalBids.GetTopK(depth),
		Asks:        cb.globalAsks.GetTopK(depth),
		LastUpdated: cb.lastUpdated,
	}
}

