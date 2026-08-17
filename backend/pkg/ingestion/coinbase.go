package ingestion

import (
	// context manages lifecycle and cancellation of the Coinbase worker
	"context"
	// json serializes subscription requests and parses incoming ticker payloads
	"encoding/json"
	// fmt formats error messages and URLs
	"fmt"
	// log writes connection and error logs
	"log"
	// strconv converts string prices and sizes from Coinbase into float64
	"strconv"
	// strings handles symbol transformations and case matching
	"strings"
	// time provides timestamps and reconnection backoff delays
	"time"

	// websocket provides client dialer for Coinbase's live WebSocket feed
	"github.com/gorilla/websocket"
	// engine provides the Dispatcher to push parsed ticks into the core engine
	"nexus-feed/backend/pkg/engine"
	// types provides the MarketTick data model and side constants
	"nexus-feed/backend/pkg/types"
)

// coinbaseSubscriptionMessage is the subscription handshake JSON sent to Coinbase on connection.
type coinbaseSubscriptionMessage struct {
	Type       string   `json:"type"`        // "subscribe"
	ProductIDs []string `json:"product_ids"` // e.g. ["BTC-USD", "ETH-USD"]
	Channels   []string `json:"channels"`    // e.g. ["ticker"]
}

// coinbaseTickerEvent represents the real-time best bid, best ask, and trade state from Coinbase.
type coinbaseTickerEvent struct {
	Type        string    `json:"type"`          // "ticker"
	Sequence    int64     `json:"sequence"`      // Sequence number
	ProductID   string    `json:"product_id"`    // e.g. "BTC-USD"
	Price       string    `json:"price"`         // Last trade price
	BestBid     string    `json:"best_bid"`      // Current best bid price
	BestBidSize string    `json:"best_bid_size"` // Current best bid quantity
	BestAsk     string    `json:"best_ask"`      // Current best ask price
	BestAskSize string    `json:"best_ask_size"` // Current best ask quantity
	Time        time.Time `json:"time"`          // Event timestamp from Coinbase
}

// CoinbaseAdapter manages a live WebSocket connection to Coinbase Exchange.
type CoinbaseAdapter struct {
	// symbols is the list of normalized pairs to track (e.g. ["BTC-USDT", "ETH-USDT"])
	symbols []string

	// dispatcher receives normalized market ticks
	dispatcher *engine.Dispatcher

	// reconnectDelay is the duration to wait before reconnecting after a disconnect
	reconnectDelay time.Duration
}

// NewCoinbaseAdapter constructs and initializes a new CoinbaseAdapter.
func NewCoinbaseAdapter(symbols []string, dispatcher *engine.Dispatcher) *CoinbaseAdapter {
	return &CoinbaseAdapter{
		symbols:        symbols,
		dispatcher:     dispatcher,
		reconnectDelay: 2 * time.Second,
	}
}

// Start launches the Coinbase ingestion loop with automatic reconnection.
func (c *CoinbaseAdapter) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("[Coinbase] Ingestion stopped by context cancellation.")
			return
		default:
			log.Println("[Coinbase] Connecting to live WebSocket feed...")
			if err := c.connectAndStream(ctx); err != nil {
				log.Printf("[Coinbase] Connection error: %v. Reconnecting in %v...", err, c.reconnectDelay)
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(c.reconnectDelay):
			}
		}
	}
}

// connectAndStream dials Coinbase WebSocket feed, subscribes, and processes market ticks.
func (c *CoinbaseAdapter) connectAndStream(ctx context.Context) error {
	url := "wss://ws-feed.exchange.coinbase.com"

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("failed to dial Coinbase: %w", err)
	}
	defer conn.Close()

	// Convert normalized symbols (e.g. "BTC-USDT") into Coinbase product IDs (e.g. "BTC-USD")
	var productIDs []string
	for _, sym := range c.symbols {
		productIDs = append(productIDs, c.toCoinbaseProductID(sym))
	}

	// Send subscription payload for the "ticker" channel
	subMsg := coinbaseSubscriptionMessage{
		Type:       "subscribe",
		ProductIDs: productIDs,
		Channels:   []string{"ticker"},
	}

	if err := conn.WriteJSON(subMsg); err != nil {
		return fmt.Errorf("failed to send subscription message: %w", err)
	}

	log.Printf("[Coinbase] Connected successfully. Subscribed to: %v", productIDs)

	// Process incoming ticker updates
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			_, rawMsg, err := conn.ReadMessage()
			if err != nil {
				return fmt.Errorf("read error: %w", err)
			}

			now := time.Now()
			ingestionMicros := now.UnixNano() / 1000

			var event coinbaseTickerEvent
			if err := json.Unmarshal(rawMsg, &event); err != nil {
				continue
			}

			// Only process "ticker" messages (ignore "subscriptions" handshake response)
			if event.Type != "ticker" {
				continue
			}

			// Parse Bid price & size
			bidPrice, errBidP := strconv.ParseFloat(event.BestBid, 64)
			bidSize, errBidS := strconv.ParseFloat(event.BestBidSize, 64)

			// Parse Ask price & size
			askPrice, errAskP := strconv.ParseFloat(event.BestAsk, 64)
			askSize, errAskS := strconv.ParseFloat(event.BestAskSize, 64)

			// Normalize product ID back to standard system symbol (e.g. "BTC-USD" -> "BTC-USDT")
			normSymbol := c.normalizeSymbol(event.ProductID)

			// 1. Submit Bid Tick
			if errBidP == nil && errBidS == nil && bidPrice > 0 {
				bidTick := types.MarketTick{
					Exchange:        "coinbase",
					Symbol:          normSymbol,
					Side:            types.SideBid,
					Price:           bidPrice,
					Quantity:        bidSize,
					Timestamp:       event.Time,
					IngestionMicros: ingestionMicros,
				}
				if bidTick.Timestamp.IsZero() {
					bidTick.Timestamp = now
				}
				c.dispatcher.SubmitTick(bidTick)
			}

			// 2. Submit Ask Tick
			if errAskP == nil && errAskS == nil && askPrice > 0 {
				askTick := types.MarketTick{
					Exchange:        "coinbase",
					Symbol:          normSymbol,
					Side:            types.SideAsk,
					Price:           askPrice,
					Quantity:        askSize,
					Timestamp:       event.Time,
					IngestionMicros: ingestionMicros,
				}
				if askTick.Timestamp.IsZero() {
					askTick.Timestamp = now
				}
				c.dispatcher.SubmitTick(askTick)
			}
		}
	}
}

// toCoinbaseProductID converts standard symbol format (e.g. "BTC-USDT") to Coinbase format (e.g. "BTC-USD").
func (c *CoinbaseAdapter) toCoinbaseProductID(symbol string) string {
	symbol = strings.ToUpper(symbol)
	if strings.HasSuffix(symbol, "-USDT") {
		return strings.TrimSuffix(symbol, "-USDT") + "-USD"
	}
	return symbol
}

// normalizeSymbol normalizes Coinbase product ID (e.g. "BTC-USD") to common engine symbol (e.g. "BTC-USDT").
func (c *CoinbaseAdapter) normalizeSymbol(productID string) string {
	productID = strings.ToUpper(productID)
	if strings.HasSuffix(productID, "-USD") {
		return strings.TrimSuffix(productID, "-USD") + "-USDT"
	}
	return productID
}
