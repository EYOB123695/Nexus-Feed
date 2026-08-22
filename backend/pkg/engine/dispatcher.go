package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"nexus-feed/backend/pkg/types"
)

// OutputHandler is a callback function signature invoked whenever a consolidated book is updated.
type OutputHandler func(snapshot types.ConsolidatedBook, arb *types.ArbitrageOpportunity)

// Dispatcher coordinates high-throughput market tick ingestion, routes ticks to the
// corresponding consolidated order books, detects arbitrage, and forwards updates downstream.
type Dispatcher struct {
	books            map[string]*ConsolidatedOrderBook
	tickChan         chan types.MarketTick
	outputHandler    OutputHandler
	ticksCount       uint64
	totalLatencyNano int64
	startTime        time.Time
	mu               sync.RWMutex
}

// NewDispatcher initializes a new engine Dispatcher with a specified buffer size for incoming ticks.
func NewDispatcher(bufferSize int, handler OutputHandler) *Dispatcher {
	if bufferSize <= 0 {
		bufferSize = 65536
	}

	return &Dispatcher{
		books:         make(map[string]*ConsolidatedOrderBook),
		tickChan:      make(chan types.MarketTick, bufferSize),
		outputHandler: handler,
		startTime:     time.Now(),
	}
}

// SubmitTick enqueues an incoming tick into the dispatcher's buffered queue.
func (d *Dispatcher) SubmitTick(tick types.MarketTick) bool {
	select {
	case d.tickChan <- tick:
		return true
	default:
		return false
	}
}

// SubmitTickBlocking pushes a tick into the queue, blocking until space is available or context cancelled.
func (d *Dispatcher) SubmitTickBlocking(ctx context.Context, tick types.MarketTick) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case d.tickChan <- tick:
		return nil
	}
}

// GetTickChannel returns the raw write-only input channel for direct pipe integration.
func (d *Dispatcher) GetTickChannel() chan<- types.MarketTick {
	return d.tickChan
}

// getOrCreateBook retrieves the ConsolidatedOrderBook for a symbol, creating one safely if not present.
func (d *Dispatcher) getOrCreateBook(symbol string) *ConsolidatedOrderBook {
	d.mu.RLock()
	book, exists := d.books[symbol]
	d.mu.RUnlock()
	if exists {
		return book
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if book, exists = d.books[symbol]; exists {
		return book
	}

	book = NewConsolidatedOrderBook(symbol)
	d.books[symbol] = book
	return book
}

// Start begins processing market ticks from the input queue in a dedicated single-threaded event loop.
func (d *Dispatcher) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case tick, ok := <-d.tickChan:
			if !ok {
				return
			}
			d.processTick(tick)
		}
	}
}

// processTick routes the tick to the appropriate symbol book, updates metrics, and dispatches downstream.
func (d *Dispatcher) processTick(tick types.MarketTick) {
	now := time.Now()
	if tick.IngestionMicros > 0 {
		latencyNano := now.UnixNano() - (tick.IngestionMicros * 1000)
		if latencyNano > 0 {
			atomic.AddInt64(&d.totalLatencyNano, latencyNano)
		}
	}

	book := d.getOrCreateBook(tick.Symbol)
	snapshot, arb := book.ProcessTick(tick)
	atomic.AddUint64(&d.ticksCount, 1)

	if d.outputHandler != nil {
		d.outputHandler(snapshot, arb)
	}
}

// GetBook returns a pointer to a symbol's consolidated book in a thread-safe manner.
func (d *Dispatcher) GetBook(symbol string) (*ConsolidatedOrderBook, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	book, exists := d.books[symbol]
	return book, exists
}

// GetActiveSymbols returns a list of all symbols currently tracked in the engine.
func (d *Dispatcher) GetActiveSymbols() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	symbols := make([]string, 0, len(d.books))
	for symbol := range d.books {
		symbols = append(symbols, symbol)
	}
	return symbols
}

// GetMetrics aggregates and computes real-time performance and telemetry metrics.
func (d *Dispatcher) GetMetrics() types.SystemMetrics {
	totalTicks := atomic.LoadUint64(&d.ticksCount)
	elapsedSec := time.Since(d.startTime).Seconds()

	var msgPerSec float64
	if elapsedSec > 0 {
		msgPerSec = float64(totalTicks) / elapsedSec
	}

	var avgLatencyMicros float64
	totalLatency := atomic.LoadInt64(&d.totalLatencyNano)
	if totalTicks > 0 && totalLatency > 0 {
		avgLatencyMicros = (float64(totalLatency) / float64(totalTicks)) / 1000.0
	}

	chanLen := len(d.tickChan)
	chanCap := cap(d.tickChan)
	var saturationPct float64
	if chanCap > 0 {
		saturationPct = (float64(chanLen) / float64(chanCap)) * 100.0
	}

	d.mu.RLock()
	exchangesMap := make(map[string]struct{})
	for _, book := range d.books {
		for exName := range book.books {
			exchangesMap[exName] = struct{}{}
		}
	}
	activeExchanges := len(exchangesMap)
	d.mu.RUnlock()

	return types.SystemMetrics{
		TicksProcessed:       totalTicks,
		MessagesPerSec:       msgPerSec,
		AverageLatencyMicros: avgLatencyMicros,
		BufferSaturationPct:  saturationPct,
		ActiveExchanges:      activeExchanges,
		LastAuditTime:        time.Now().UnixNano() / 1000,
	}
}

// Close gracefully closes the input tick channel.
func (d *Dispatcher) Close() {
	close(d.tickChan)
}
