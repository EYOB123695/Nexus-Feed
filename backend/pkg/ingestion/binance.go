package ingestion
import (
	// context manages cancellation and lifecycle of the ingestion worker
	"context"
	// json parses incoming raw JSON payloads from Binance
	"encoding/json"
	// fmt handles string formatting and URL construction
	"fmt"
	// log outputs status and connection error messages
	"log"
	// strconv converts price and quantity strings from Binance into float64
	"strconv"
	// strings formats symbol pairs to uppercase and lowercase
	"strings"
	// time manages timestamps and reconnection backoff delays
	"time"
	// websocket provides the client to connect to Binance WebSocket streams
	"github.com/gorilla/websocket"
	// engine provides the Dispatcher struct to submit parsed ticks
	"nexus-feed/backend/pkg/engine"
	// types provides the MarketTick struct and SideBid/SideAsk constants
	"nexus-feed/backend/pkg/types"
)


// binanceCombinedEvent is the JSON wrapper returned by Binance's combined stream endpoint.

type binanceCombinedEvent struct {
	// Stream name (e.g. "btcusdt@bookTicker")
	Stream string `json:"stream"`
	// Data holds the actual bookTicker event payload
	Data binanceBookTicker `json:"data"`
}

// binanceBookTicker represents the real-time best bid and ask state from Binance.

type binanceBookTicker struct {
	// UpdateID is the monotonically increasing update identifier
	UpdateID int64 `json:"u"`
	// Symbol is the trading pair identifier (e.g. "BTCUSDT")
	Symbol string `json:"s"`
	// BestBidPrice is the best bid price as a string
	BestBidPrice string `json:"b"`
	// BestBidQty is the quantity available at the best bid price
	BestBidQty string `json:"B"`
	// BestAskPrice is the best ask price as a string
	BestAskPrice string `json:"a"`
	// BestAskQty is the quantity available at the best ask price
	BestAskQty string `json:"A"`
}

// BinanceAdapter manages a live WebSocket connection to Binance's public data feed.

type BinanceAdapter struct {
	// symbols is the list of normalized pairs to track (e.g. ["BTC-USDT", "ETH-USDT"])
	symbols []string
	// dispatcher receives normalized market ticks
	dispatcher *engine.Dispatcher
	// reconnectDelay is the duration to wait before reconnecting after a disconnect
	reconnectDelay time.Duration
}
// NewBinanceAdapter constructs and initializes a new BinanceAdapter instance.
func NewBinanceAdapter(symbols []string, dispatcher *engine.Dispatcher) *BinanceAdapter {
	return &BinanceAdapter{
		symbols:        symbols,
		dispatcher:     dispatcher,
		reconnectDelay: 2 * time.Second,
	}
}


// Start runs the ingestion loop in a continuous auto-reconnecting cycle until context is canceled.
func (b *BinanceAdapter) Start (ctx context.Context) { 
     // Loop continuously to handle auto-reconnection

	 for { 
		select { 
			// If context is canceled, exit the loop cleanly
			case <-ctx.Done(): 
			   log.Println("[Binance] Ingestion stopped by context cancellation.")
			   return
			// Otherwise attempt to connect and stream market data
		    default:
				log.Println("[Binance] Connecting to live WebSocket feed...")
				if err := b.connectAndStream(ctx); err != nil {
				log.Printf("[Binance] Connection error: %v. Reconnecting in %v...", err, b.reconnectDelay)
			   }
			   // Sleep for the backoff duration before attempting to reconnect
		       time.Sleep(b.reconnectDelay)
			}
			// Sleep for the backoff duration before attempting to reconnect
			select {
			case <-ctx.Done():
				return
			case <-time.After(b.reconnectDelay):
			}


			        

		}
	 }
// connectAndStream dials the Binance WebSocket endpoint and processes incoming tick messages.
func (b *BinanceAdapter) connectAndStream(ctx context.Context) error {
	// Build the combined streams URL query (e.g. "btcusdt@bookTicker/ethusdt@bookTicker")
	var streamNames []string
	for _, sym := range b.symbols {
		clean := strings.ToLower(strings.ReplaceAll(sym, "-", ""))
		streamNames = append(streamNames, clean+"@bookTicker")
	}
	url := fmt.Sprintf("wss://stream.binance.com:9443/stream?streams=%s", strings.Join(streamNames, "/"))

	// Dial the Binance WebSocket server
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("failed to dial Binance: %w", err)
	}
	// Ensure connection is closed when function returns
	defer conn.Close()

	log.Printf("[Binance] Connected successfully. Streaming symbols: %v", b.symbols)

	// Continuously read messages from the WebSocket connection
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			// Read the next raw message from the WebSocket
			_, rawMsg, err := conn.ReadMessage()
			if err != nil {
				return fmt.Errorf("read error: %w", err)
			}

			// Record microsecond ingestion timestamp immediately upon receipt
			now := time.Now()
			ingestionMicros := now.UnixNano() / 1000

			// Parse the outer combined stream envelope
			var event binanceCombinedEvent
			if err := json.Unmarshal(rawMsg, &event); err != nil {
				continue
			}

			// Parse Bid price and quantity into float64
			bidPrice, errBidP := strconv.ParseFloat(event.Data.BestBidPrice, 64)
			bidQty, errBidQ := strconv.ParseFloat(event.Data.BestBidQty, 64)

			// Parse Ask price and quantity into float64
			askPrice, errAskP := strconv.ParseFloat(event.Data.BestAskPrice, 64)
			askQty, errAskQ := strconv.ParseFloat(event.Data.BestAskQty, 64)

			// If parsing succeeded, construct MarketTicks
			normSymbol := b.normalizeSymbol(event.Data.Symbol)

			// 1. Submit Bid Tick into engine dispatcher
			if errBidP == nil && errBidQ == nil && bidPrice > 0 {
				bidTick := types.MarketTick{
					Exchange:        "binance",
					Symbol:          normSymbol,
					Side:            types.SideBid,
					Price:           bidPrice,
					Quantity:        bidQty,
					Timestamp:       now,
					IngestionMicros: ingestionMicros,
				}
				b.dispatcher.SubmitTick(bidTick)
			}

			// 2. Submit Ask Tick into engine dispatcher
			if errAskP == nil && errAskQ == nil && askPrice > 0 {
				askTick := types.MarketTick{
					Exchange:        "binance",
					Symbol:          normSymbol,
					Side:            types.SideAsk,
					Price:           askPrice,
					Quantity:        askQty,
					Timestamp:       now,
					IngestionMicros: ingestionMicros,
				}
				b.dispatcher.SubmitTick(askTick)
			}
		}
	}
}

// normalizeSymbol formats raw Binance symbols (e.g. "BTCUSDT") into standard format ("BTC-USDT").
func (b *BinanceAdapter) normalizeSymbol(raw string) string {
	raw = strings.ToUpper(raw)
	// Check common suffixes and insert hyphen
	if strings.HasSuffix(raw, "USDT") {
		base := strings.TrimSuffix(raw, "USDT")
		return base + "-USDT"
	}
	if strings.HasSuffix(raw, "USD") {
		base := strings.TrimSuffix(raw, "USD")
		return base + "-USD"
	}
	return raw
}










