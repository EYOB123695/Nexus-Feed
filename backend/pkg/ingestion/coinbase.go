package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"nexus-feed/backend/pkg/engine"
	"nexus-feed/backend/pkg/types"
)

type coinbaseSubscriptionMessage struct {
	Type       string   `json:"type"`
	ProductIDs []string `json:"product_ids"`
	Channels   []string `json:"channels"`
}

type coinbaseTickerEvent struct {
	Type        string    `json:"type"`
	Sequence    int64     `json:"sequence"`
	ProductID   string    `json:"product_id"`
	Price       string    `json:"price"`
	BestBid     string    `json:"best_bid"`
	BestBidSize string    `json:"best_bid_size"`
	BestAsk     string    `json:"best_ask"`
	BestAskSize string    `json:"best_ask_size"`
	Time        time.Time `json:"time"`
}

// CoinbaseAdapter manages a live WebSocket connection to Coinbase Exchange.
type CoinbaseAdapter struct {
	symbols        []string
	dispatcher     *engine.Dispatcher
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

	var productIDs []string
	for _, sym := range c.symbols {
		productIDs = append(productIDs, c.toCoinbaseProductID(sym))
	}

	subMsg := coinbaseSubscriptionMessage{
		Type:       "subscribe",
		ProductIDs: productIDs,
		Channels:   []string{"ticker"},
	}

	if err := conn.WriteJSON(subMsg); err != nil {
		return fmt.Errorf("failed to send subscription message: %w", err)
	}

	log.Printf("[Coinbase] Connected successfully. Subscribed to: %v", productIDs)

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

			if event.Type != "ticker" {
				continue
			}

			bidPrice, errBidP := strconv.ParseFloat(event.BestBid, 64)
			bidSize, errBidS := strconv.ParseFloat(event.BestBidSize, 64)

			askPrice, errAskP := strconv.ParseFloat(event.BestAsk, 64)
			askSize, errAskS := strconv.ParseFloat(event.BestAskSize, 64)

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
