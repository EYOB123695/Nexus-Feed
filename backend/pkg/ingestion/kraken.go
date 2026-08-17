package ingestion

import (
	// context manages the lifecycle and cancellation of the Kraken worker
	"context"
	// json parses subscription payloads and incoming ticker events
	"encoding/json"
	// fmt formats error messages and URLs
	"fmt"
	// log outputs connection and error logs
	"log"
	// strings formats symbol pairs to standard representations
	"strings"
	// time manages timestamps and reconnection backoff delays
	"time"

	// websocket provides client connectivity to Kraken's live WebSocket feed
	"github.com/gorilla/websocket"
	// engine provides the Dispatcher to submit parsed market ticks
	"nexus-feed/backend/pkg/engine"
	// types provides the MarketTick struct and side constants
	"nexus-feed/backend/pkg/types"
)

// krakenV2SubscriptionRequest is the subscription handshake payload sent to Kraken WebSocket v2.
type krakenV2SubscriptionRequest struct {
	Method string               `json:"method"` // "subscribe"
	Params krakenV2SubParams    `json:"params"`
}

type krakenV2SubParams struct {
	Channel string   `json:"channel"` // "ticker"
	Symbol  []string `json:"symbol"`  // e.g. ["BTC/USD", "ETH/USD"]
}

// krakenV2TickerMessage represents a live ticker update from Kraken WebSocket v2.
type krakenV2TickerMessage struct {
	Channel string             `json:"channel"` // "ticker"
	Type    string             `json:"type"`    // "snapshot" or "update"
	Data    []krakenV2TickerItem `json:"data"`
}

type krakenV2TickerItem struct {
	Symbol string  `json:"symbol"`  // e.g. "BTC/USD"
	Bid    float64 `json:"bid"`     // Best bid price
	BidQty float64 `json:"bid_qty"` // Quantity at best bid
	Ask    float64 `json:"ask"`     // Best ask price
	AskQty float64 `json:"ask_qty"` // Quantity at best ask
	Last   float64 `json:"last"`    // Last trade price
}

// KrakenAdapter manages a live WebSocket connection to Kraken's public data feed.
type KrakenAdapter struct {
	// symbols is the list of normalized pairs to track (e.g. ["BTC-USDT", "ETH-USDT"])
	symbols []string

	// dispatcher receives normalized market ticks
	dispatcher *engine.Dispatcher

	// reconnectDelay is the duration to wait before reconnecting after a disconnect
	reconnectDelay time.Duration
}

// NewKrakenAdapter constructs and initializes a new KrakenAdapter instance.
func NewKrakenAdapter(symbols []string, dispatcher *engine.Dispatcher) *KrakenAdapter {
	return &KrakenAdapter{
		symbols:        symbols,
		dispatcher:     dispatcher,
		reconnectDelay: 2 * time.Second,
	}
}

// Start launches the Kraken ingestion loop with automatic reconnection.
func (k *KrakenAdapter) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("[Kraken] Ingestion stopped by context cancellation.")
			return
		default:
			log.Println("[Kraken] Connecting to live WebSocket feed...")
			if err := k.connectAndStream(ctx); err != nil {
				log.Printf("[Kraken] Connection error: %v. Reconnecting in %v...", err, k.reconnectDelay)
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(k.reconnectDelay):
			}
		}
	}
}

// connectAndStream dials the Kraken WebSocket v2 endpoint, subscribes, and processes market ticks.
func (k *KrakenAdapter) connectAndStream(ctx context.Context) error {
	url := "wss://ws.kraken.com/v2"

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("failed to dial Kraken: %w", err)
	}
	defer conn.Close()

	// Convert normalized symbols (e.g. "BTC-USDT") into Kraken v2 format (e.g. "BTC/USD")
	var krakenSymbols []string
	for _, sym := range k.symbols {
		krakenSymbols = append(krakenSymbols, k.toKrakenSymbol(sym))
	}

	// Send subscription payload for the "ticker" channel
	subReq := krakenV2SubscriptionRequest{
		Method: "subscribe",
		Params: krakenV2SubParams{
			Channel: "ticker",
			Symbol:  krakenSymbols,
		},
	}

	if err := conn.WriteJSON(subReq); err != nil {
		return fmt.Errorf("failed to send subscription message: %w", err)
	}

	log.Printf("[Kraken] Connected successfully. Subscribed to: %v", krakenSymbols)

	// Continuously process incoming ticker messages
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

			var msg krakenV2TickerMessage
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				continue
			}

			// Filter only ticker messages with valid data items
			if msg.Channel != "ticker" || len(msg.Data) == 0 {
				continue
			}

			for _, item := range msg.Data {
				normSymbol := k.normalizeSymbol(item.Symbol)

				// 1. Submit Bid Tick
				if item.Bid > 0 && item.BidQty > 0 {
					bidTick := types.MarketTick{
						Exchange:        "kraken",
						Symbol:          normSymbol,
						Side:            types.SideBid,
						Price:           item.Bid,
						Quantity:        item.BidQty,
						Timestamp:       now,
						IngestionMicros: ingestionMicros,
					}
					k.dispatcher.SubmitTick(bidTick)
				}

				// 2. Submit Ask Tick
				if item.Ask > 0 && item.AskQty > 0 {
					askTick := types.MarketTick{
						Exchange:        "kraken",
						Symbol:          normSymbol,
						Side:            types.SideAsk,
						Price:           item.Ask,
						Quantity:        item.AskQty,
						Timestamp:       now,
						IngestionMicros: ingestionMicros,
					}
					k.dispatcher.SubmitTick(askTick)
				}
			}
		}
	}
}

// toKrakenSymbol converts standard symbol format (e.g. "BTC-USDT") into Kraken format ("BTC/USD").
func (k *KrakenAdapter) toKrakenSymbol(symbol string) string {
	symbol = strings.ToUpper(symbol)
	symbol = strings.ReplaceAll(symbol, "-", "/")
	if strings.HasSuffix(symbol, "/USDT") {
		return strings.TrimSuffix(symbol, "/USDT") + "/USD"
	}
	return symbol
}

// normalizeSymbol converts Kraken format ("BTC/USD" or "XBT/USD") to engine standard ("BTC-USDT").
func (k *KrakenAdapter) normalizeSymbol(krakenSymbol string) string {
	krakenSymbol = strings.ToUpper(krakenSymbol)
	krakenSymbol = strings.ReplaceAll(krakenSymbol, "/", "-")
	if strings.HasPrefix(krakenSymbol, "XBT-") {
		krakenSymbol = "BTC-" + strings.TrimPrefix(krakenSymbol, "XBT-")
	}
	if strings.HasSuffix(krakenSymbol, "-USD") {
		krakenSymbol = strings.TrimSuffix(krakenSymbol, "-USD") + "-USDT"
	}
	return krakenSymbol
}
