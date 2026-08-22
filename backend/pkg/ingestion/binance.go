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

type binanceCombinedEvent struct {
	Stream string            `json:"stream"`
	Data   binanceBookTicker `json:"data"`
}

type binanceBookTicker struct {
	UpdateID     int64  `json:"u"`
	Symbol       string `json:"s"`
	BestBidPrice string `json:"b"`
	BestBidQty   string `json:"B"`
	BestAskPrice string `json:"a"`
	BestAskQty   string `json:"A"`
}

// BinanceAdapter manages a live WebSocket connection to Binance's public data feed.
type BinanceAdapter struct {
	symbols        []string
	dispatcher     *engine.Dispatcher
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
func (b *BinanceAdapter) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("[Binance] Ingestion stopped by context cancellation.")
			return
		default:
			log.Println("[Binance] Connecting to live WebSocket feed...")
			if err := b.connectAndStream(ctx); err != nil {
				log.Printf("[Binance] Connection error: %v. Reconnecting in %v...", err, b.reconnectDelay)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(b.reconnectDelay):
		}
	}
}

// connectAndStream dials the Binance WebSocket endpoint and processes incoming tick messages.
func (b *BinanceAdapter) connectAndStream(ctx context.Context) error {
	var streamNames []string
	for _, sym := range b.symbols {
		clean := strings.ToLower(strings.ReplaceAll(sym, "-", ""))
		streamNames = append(streamNames, clean+"@bookTicker")
	}
	url := fmt.Sprintf("wss://data-stream.binance.vision/stream?streams=%s", strings.Join(streamNames, "/"))

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("failed to dial Binance: %w", err)
	}
	defer conn.Close()

	log.Printf("[Binance] Connected successfully. Streaming symbols: %v", b.symbols)

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

			var event binanceCombinedEvent
			if err := json.Unmarshal(rawMsg, &event); err != nil {
				continue
			}

			bidPrice, errBidP := strconv.ParseFloat(event.Data.BestBidPrice, 64)
			bidQty, errBidQ := strconv.ParseFloat(event.Data.BestBidQty, 64)

			askPrice, errAskP := strconv.ParseFloat(event.Data.BestAskPrice, 64)
			askQty, errAskQ := strconv.ParseFloat(event.Data.BestAskQty, 64)

			normSymbol := b.normalizeSymbol(event.Data.Symbol)

			// 1. Submit Bid Tick
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

			// 2. Submit Ask Tick
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










