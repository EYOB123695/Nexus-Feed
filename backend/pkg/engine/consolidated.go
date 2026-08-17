package engine

import (
	"math"
	"time"

	"nexus-feed/backend/pkg/types"
)

// ConsolidatedOrderBook aggregates order books across multiple exchanges for a single symbol.
type ConsolidatedOrderBook struct {
	Symbol      string
	books       map[string]*OrderBook // keyed by exchange name (e.g. "binance", "coinbase")
	globalBids  *SkipList             // Aggregated global bids across all exchanges
	globalAsks  *SkipList             // Aggregated global asks across all exchanges
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
// Returns (types.ConsolidatedBook, *types.ArbitrageOpportunity): the snapshot and any detected arbitrage.
func (cb *ConsolidatedOrderBook) ProcessTick(tick types.MarketTick) (types.ConsolidatedBook, *types.ArbitrageOpportunity) {
	// Set the consolidated book's lastUpdated timestamp to the incoming tick's timestamp.
	cb.lastUpdated = tick.Timestamp

	// If the incoming tick timestamp was empty/zero, fallback to current system clock time.
	if cb.lastUpdated.IsZero() {
		cb.lastUpdated = time.Now()
	}

	// Convert the timestamp into microseconds (divide Unix nanoseconds by 1000) for microsecond precision.
	updatedAtMicros := cb.lastUpdated.UnixNano() / 1000

	// Step 1: Retrieve the isolated OrderBook for this specific exchange (e.g. "binance"), creating it if new.
	exBook := cb.GetOrAddExchangeBook(tick.Exchange)

	// Apply the tick to that exchange's isolated OrderBook so its local state is updated.
	exBook.ApplyTick(tick)

	// Step 2: Update the global consolidated SkipList
	if tick.Side == types.SideBid {
		// If quantity is zero or negative, trader cancelled the order -> delete from global bids SkipList.
		if tick.Quantity <= 0 {
			cb.globalBids.Delete(tick.Price)
		} else {
			cb.globalBids.InsertOrUpdate(tick.Price, tick.Quantity, tick.Exchange, updatedAtMicros)
		}
	} else if tick.Side == types.SideAsk {
		// If quantity is zero or negative, trader cancelled the order -> delete from global asks SkipList.
		if tick.Quantity <= 0 {
			cb.globalAsks.Delete(tick.Price)
		} else {
			cb.globalAsks.InsertOrUpdate(tick.Price, tick.Quantity, tick.Exchange, updatedAtMicros)
		}
	}

	// Step 3: Peek the highest global bid price (ignoring quantity and exists flag with _).
	bestBidPrice, _, _ := cb.globalBids.PeekBest()

	// Peek the lowest global ask price (ignoring quantity and exists flag with _).
	bestAskPrice, _, _ := cb.globalAsks.PeekBest()

	// Declare float64 variables for midPrice, spread, and spreadPct initialized to 0.0.
	var midPrice, spread, spreadPct float64

	// Only compute spread metrics if both a valid bid and ask exist in the book.
	if bestBidPrice > 0 && bestAskPrice > 0 {
		// Calculate the raw dollar spread (Lowest Seller Ask minus Highest Buyer Bid).
		spread = bestAskPrice - bestBidPrice

		// Calculate the mid price (exact average between Best Ask and Best Bid).
		midPrice = (bestAskPrice + bestBidPrice) / 2.0

		// Ensure midPrice is greater than 0 to avoid division by zero.
		if midPrice > 0 {
			// Calculate the percentage spread relative to the mid price.
			spreadPct = (spread / midPrice) * 100.0
		}
	}

	// Construct the consolidated book snapshot object to send to the UI.
	snapshot := types.ConsolidatedBook{
		Symbol:      cb.Symbol,
		BestBid:     bestBidPrice,
		BestAsk:     bestAskPrice,
		MidPrice:    midPrice,
		Spread:      spread,
		SpreadPct:   spreadPct,
		Bids:        cb.globalBids.GetTopK(20), // Top 20 bids pre-sorted in descending order
		Asks:        cb.globalAsks.GetTopK(20), // Top 20 asks pre-sorted in ascending order
		LastUpdated: cb.lastUpdated,
	}

	// Step 4: Run the arbitrage detection engine across all registered exchanges.
	arb := cb.DetectArbitrage()

	// Return both the consolidated snapshot and the arbitrage opportunity (nil if none).
	return snapshot, arb
}

// DetectArbitrage scans top-of-book across all registered exchanges to find crossed markets.
func (cb *ConsolidatedOrderBook) DetectArbitrage() *types.ArbitrageOpportunity {
	// If fewer than 2 exchanges exist, cross-exchange arbitrage is impossible -> return nil.
	if len(cb.books) < 2 {
		return nil
	}

	var bestBuyExchange string
	var lowestAskPrice float64 = math.MaxFloat64 // Start with infinity
	var lowestAskQty float64

	var bestSellExchange string
	var highestBidPrice float64 = 0.0
	var highestBidQty float64

	// Loop over all registered exchange books
	for exName, book := range cb.books {
		bidPrice, bidQty := book.GetBestBid()
		askPrice, askQty := book.GetBestAsk()

		// Track the exchange with the highest buyer (Best place to sell)
		if bidPrice > highestBidPrice {
			highestBidPrice = bidPrice
			highestBidQty = bidQty
			bestSellExchange = exName
		}

		// Track the exchange with the cheapest seller (Best place to buy)
		if askPrice > 0 && askPrice < lowestAskPrice {
			lowestAskPrice = askPrice
			lowestAskQty = askQty
			bestBuyExchange = exName
		}
	}

	// Check if a crossed market exists: valid bid, valid ask, and from DIFFERENT exchanges.
	if highestBidPrice > 0 && lowestAskPrice < math.MaxFloat64 && bestSellExchange != bestBuyExchange {
		// Condition for profit: Highest Buyer is willing to pay MORE than Lowest Seller asks.
		if highestBidPrice > lowestAskPrice {
			// Calculate the dollar profit margin per unit traded (Sell Price minus Buy Price).
			spreadMargin := highestBidPrice - lowestAskPrice

			// Calculate percentage return on capital (Profit divided by Cost).
			profitPct := (spreadMargin / lowestAskPrice) * 100.0

			// Calculate the maximum volume that can be matched (the smaller of the two quantities).
			volumeAvail := math.Min(highestBidQty, lowestAskQty)

			// Allocate and return the ArbitrageOpportunity struct pointer with all details.
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

	// No crossed market / arbitrage exists -> return nil.
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

