/**
 * NEXUS-FEED FRONTEND DATA CONTRACTS
 */
 
// 1. Supported exchange identifiers matching Go ingestion adapters (Binance, Coinbase, Kraken)
export type ExchangeName = 'binance' | 'coinbase' | 'kraken';
// 2. Supported trading market pairs
export type MarketSymbol = 'BTC-USDT' | 'ETH-USDT' | 'SOL-USDT' | 'ALL';
// 3. Order Book Side
export type OrderSide = 'BID' | 'ASK';

// 4. WebSocket connection states
export type ConnectionStatus = 'CONNECTING' | 'CONNECTED' | 'DISCONNECTED' | 'ERROR';

/**
 * Single price level in the consolidated order book ladder.
 * Corresponds to types.PriceLevel in Go.
 */
export interface PriceLevel {
  price: number;              // Price point (e.g. 96450.50)
  quantity: number;           // Volume available at this exact price level
  exchange: ExchangeName | string; // Originating exchange ('binance' | 'coinbase' | 'kraken')
  updated_at: number;         // Microsecond Unix timestamp
  
  // Client-side computed fields for visual depth bar rendering
  total?: number;             // Cumulative volume up to this price level
  depthPct?: number;          // Relative percentage for depth background bar (0 - 100%)
}

/**
 * Consolidated Cross-Exchange Order Book.
 * Corresponds to types.ConsolidatedBook in Go (emitted by SkipList aggregator).
 */
export interface ConsolidatedBook {
  symbol: string;             // e.g. "BTC-USDT"
  best_bid: number;           // Top bid price across all 3 exchanges (NBBO)
  best_ask: number;           // Top ask price across all 3 exchanges (NBBO)
  mid_price: number;          // Midpoint: (best_bid + best_ask) / 2
  spread: number;             // Absolute spread: best_ask - best_bid
  spread_pct: number;         // Percentage spread: (spread / mid_price) * 100
  bids: PriceLevel[];         // Sorted descending (highest bid first)
  asks: PriceLevel[];         // Sorted ascending (lowest ask first)
  last_updated: string;       // Timestamp string from Go time.Time
}


/**
 * Cross-Exchange Arbitrage Opportunity.
 * Corresponds to types.ArbitrageOpportunity in Go.
 * Emitted when BestBid(Exchange A) > BestAsk(Exchange B).
 */
export interface ArbitrageOpportunity {
  symbol: string;             // Market pair (e.g. "BTC-USDT")
  buy_exchange: string;       // Exchange with lower ask (where we buy cheap)
  sell_exchange: string;      // Exchange with higher bid (where we sell high)
  buy_price: number;          // Ask price on buy exchange
  sell_price: number;         // Bid price on sell exchange
  spread_margin: number;      // Net price difference (sell_price - buy_price)
  profit_pct: number;         // Net profit percentage before fees
  volume_available: number;   // Max volume executable at top-of-book
  timestamp: string;          // Detection timestamp (ISO string)
}



/**
 * Dispatcher Engine Performance Metrics.
 * Corresponds to types.SystemMetrics in Go.
 */
export interface EngineMetrics {
  ticks_processed: number;         // Total lifetime ticks processed by SkipList
  messages_per_sec: number;        // Current ingestion throughput
  average_latency_micros: number;  // Processing time per tick in microseconds (µs)
  buffer_saturation_pct: number;   // 64K lock-free queue saturation percentage (0 - 100%)
  active_exchanges: number;        // Number of currently connected exchange feeds
  last_audit_time: number;         // Unix timestamp of last audit window
}


/**
 * Conflation Engine Metrics (50ms / 20 FPS flush loop).
 */
export interface ConflationMetrics {
  raw_inputs_received: number;     // Raw incoming ticks from all exchanges
  conflated_outputs: number;       // Broadcast batches dispatched to WebSocket clients
  arbitrage_events: number;        // Total arbitrage triggers processed
  compression_ratio: number;       // Tick reduction ratio (e.g. 94.5%)
}

/**
 * Full Telemetry payload returned by GET /api/metrics.
 */
export interface SystemTelemetry {
  engine: EngineMetrics;
  conflation: ConflationMetrics;
  stream: {
    active_ws_clients: number;     // Active browser WebSocket connections
  };
  active_symbols: string[];        // ["BTC-USDT", "ETH-USDT", "SOL-USDT"]
  timestamp: number;               // Server microsecond timestamp
}


/**
 * Standard WebSocket JSON Envelope from ws://localhost:8080/ws.
 * Corresponds to stream.WSMessage in Go.
 */
export interface WSMessage<T = unknown> {
  type: 'book_update' | 'arbitrage' | 'system_metrics';
  data: T;
  timestamp: number;
}
/**
 * Client command sent TO the WebSocket server to change subscriptions.
 * Corresponds to stream.ClientRequest in Go.
 */
export interface ClientSubscriptionRequest {
  action: 'subscribe' | 'unsubscribe';
  symbol: MarketSymbol | string;
}

/**
 * Health check response from GET /api/health.
 */
export interface HealthStatus {
  status: string;
  system: string;
}












