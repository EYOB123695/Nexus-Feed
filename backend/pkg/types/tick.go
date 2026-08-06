package types

import "time"

type Side string

const (
	SideBid Side = "BID"
	SideAsk Side = "ASK"
)

type MarketTick struct {
	Exchange        string    `json:"exchange"`
	Symbol          string    `json:"symbol"`
	Side            Side      `json:"side"`
	Price           float64   `json:"price"`
	Quantity        float64   `json:"quantity"`
	Timestamp       time.Time `json:"timestamp"`
	IngestionMicros int64     `json:"ingestion_micros"`
}

type PriceLevel struct {
	Price     float64 `json:"price"`
	Quantity  float64 `json:"quantity"`
	Exchange  string  `json:"exchange"`
	UpdatedAt int64   `json:"updated_at"`
}
type OrderBookSnapshot struct {
	Symbol    string       `json:"symbol"`
	Exchange  string       `json:"exchange"`
	Bids      []PriceLevel `json:"bids"`
	Asks      []PriceLevel `json:"asks"`
	Timestamp time.Time    `json:"timestamp"`

}

type ConsolidatedBook struct { 
	Symbol string `json:"symbol"`
	BestBid float64 `json:"best_bid"`
	BestAsk float64 `json:"best_ask"`
	MidPrice float64 `json:"mid_price"`
	Spread float64 `json:"spread"`
	SpreadPct float64 `json:"spread_pct"`
	Bids []PriceLevel `json:"bids"`
	Asks []PriceLevel `json:"asks"`
	LastUpdated time.Time `json:"last_updated"`

} 

type ArbitrageOpportunity struct {
	Symbol string `json:"symbol"`
	BuyExchange string `json:"buy_exchange"`
	SellExchange string `json:"sell_exchange"`
	BuyPrice float64 `json:"buy_price"`
	SellPrice float64 `json:"sell_price"`
	SpreadMargin float64 `json:"spread_margin"`
	ProfitPct float64 `json:"profit_pct"`
	VolumeAvailable float64 `json:"volume_available"`
	Timestamp time.Time `json:"timestamp"`
}

type SystemMetrics struct {
	TicksProcessed uint64 `json:"ticks_processed"`
	MessagesPerSec float64 `json:"messages_per_sec"`
	AverageLatencyMicros float64 `json:"average_latency_micros"`
    BufferSaturationPct float64 `json:"buffer_saturation_pct"`
    ActiveExchanges int `json:"active_exchanges"`
    LastAuditTime int64 `json:"last_audit_time"`
}


























