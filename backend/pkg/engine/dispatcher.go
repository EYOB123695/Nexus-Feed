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
	books            map[string]*ConsolidatedOrderBook // Keyed by symbol (e.g. "BTC-USDT")
	tickChan         chan types.MarketTick             // Lock-free buffered input queue
	outputHandler    OutputHandler                     // Downstream stream handler (to ring buffer/conflation)
	ticksCount       uint64                            // Telemetry: total ticks processed (atomic)
	totalLatencyNano int64                             // Telemetry: cumulative latency in nanoseconds (atomic)
	startTime        time.Time                         // Telemetry: start timestamp
	mu               sync.RWMutex                      // Read-only mutex for external status queries
}

// NewDispatcher initializes a new engine Dispatcher with a specified buffer size for incoming ticks.
func NewDispatcher(bufferSize int, handler OutputHandler) *Dispatcher {
	if bufferSize <= 0 {
		bufferSize = 65536 // Default 64K tick buffer for ultra-low latency lock-free queueing
	}

	return &Dispatcher{
		books:         make(map[string]*ConsolidatedOrderBook),
		tickChan:      make(chan types.MarketTick, bufferSize),
		outputHandler: handler,
		startTime:     time.Now(),
	}
}

// SubmitTick enqueues an incoming tick into the dispatcher's buffered queue.
// It returns true if enqueued successfully, or false if the queue is full (backpressure/drop).
func (d *Dispatcher) SubmitTick(tick types.MarketTick) bool {
	select {
	case d.tickChan <- tick:
		return true
	default:
		// Queue full: drop or handle backpressure to prevent blocking ingestion threads
		return false
	}
}

// SubmitTickBlocking pushes a tick into the queue, blocking until space is available or context cancelled.
func (d *Dispatcher) SubmitTickBlocking(ctx context.Context, tick types.MarketTick) error {
	// Multiplex between context cancellation and writing to the buffered tick channel
	select {
	// Case 1: The cancellation context was triggered before space was available in the channel
	case <-ctx.Done():
		// Return the context cancellation error (e.g. context.Canceled)
		return ctx.Err()
	// Case 2: Space became available in the buffered tick channel; enqueue the tick
	case d.tickChan <- tick:
		// Return nil indicating the tick was successfully enqueued
		return nil
	}
}

// GetTickChannel returns the raw write-only input channel for direct pipe integration.
func (d *Dispatcher) GetTickChannel() chan<- types.MarketTick {
	// Return the internal tick channel as a send-only channel
	return d.tickChan
}

// getOrCreateBook retrieves the ConsolidatedOrderBook for a symbol, creating one safely if not present.
func (d *Dispatcher) getOrCreateBook(symbol string) *ConsolidatedOrderBook {
	// Acquire a read lock to check if the book already exists without blocking other readers
	d.mu.RLock()
	// Check if the order book for this symbol is already initialized
	book, exists := d.books[symbol]
	// Release the read lock immediately after lookup
	d.mu.RUnlock()
	// If the book already exists, return it immediately (fast path)
	if exists {
		return book
	}

	// Acquire an exclusive write lock to register the new symbol book
	d.mu.Lock()
	// Defer releasing the write lock when this function exits
	defer d.mu.Unlock()

	// Double-check map in case another goroutine created it while acquiring the write lock
	if book, exists = d.books[symbol]; exists {
		return book
	}

	// Initialize a new ConsolidatedOrderBook for the symbol (with Bid and Ask SkipLists)
	book = NewConsolidatedOrderBook(symbol)
	// Store the new consolidated book pointer in the books map keyed by symbol
	d.books[symbol] = book

	// Return the pointer to the newly created consolidated book
	return book
}

// Start begins processing market ticks from the input queue in a dedicated single-threaded event loop.
func (d *Dispatcher) Start(ctx context.Context) {
	// Run the event loop continuously
	for {
		// Wait for either context cancellation or incoming ticks on the channel
		select {
		// If context cancellation is signaled, exit the event loop cleanly
		case <-ctx.Done():
			return
		// Receive the next market tick from the buffered queue
		case tick, ok := <-d.tickChan:
			// If the channel was closed, terminate the processing loop
			if !ok {
				return
			}
			// Process the received market tick
			d.processTick(tick)
		}
	}
}

// processTick routes the tick to the appropriate symbol book, updates metrics, and dispatches downstream.
func (d *Dispatcher) processTick(tick types.MarketTick) {
	// Capture current time to measure end-to-end processing latency
	now := time.Now()
	// Check if the tick has an ingestion timestamp from the exchange adapter
	if tick.IngestionMicros > 0 {
		// Calculate latency in nanoseconds (current time minus ingestion timestamp converted to nanos)
		latencyNano := now.UnixNano() - (tick.IngestionMicros * 1000)
		// Ensure latency is positive before recording
		if latencyNano > 0 {
			// Atomically add latency nanoseconds to cumulative latency counter for telemetry
			atomic.AddInt64(&d.totalLatencyNano, latencyNano)
		}
	}
	// Fetch or initialize the ConsolidatedOrderBook for this specific tick's symbol
	book := d.getOrCreateBook(tick.Symbol)
	// Update the exchange book and global SkipLists, and detect cross-exchange arbitrage
	snapshot, arb := book.ProcessTick(tick)
	// Atomically increment total processed ticks counter
	atomic.AddUint64(&d.ticksCount, 1)
	// If a downstream output handler is registered, emit the new snapshot and arbitrage opportunity
	if d.outputHandler != nil {
		// Invoke the output handler callback
		d.outputHandler(snapshot, arb)
	}
}

// GetBook returns a pointer to a symbol's consolidated book in a thread-safe manner.
func (d *Dispatcher) GetBook(symbol string) (*ConsolidatedOrderBook, bool) {
	// Acquire read lock for thread-safe map access
	d.mu.RLock()
	// Defer releasing the read lock
	defer d.mu.RUnlock()
	// Look up the requested symbol book in the map
	book, exists := d.books[symbol]
	// Return the book pointer and existence flag
	return book, exists
}

// GetActiveSymbols returns a list of all symbols currently tracked in the engine.
func (d *Dispatcher) GetActiveSymbols() []string {
	// Acquire read lock for thread-safe map access
	d.mu.RLock()
	// Defer releasing the read lock
	defer d.mu.RUnlock()
	// Pre-allocate a slice with capacity equal to the number of tracked books
	symbols := make([]string, 0, len(d.books))
	// Iterate through map keys and append each symbol string
	for symbol := range d.books {
		symbols = append(symbols, symbol)
	}
	// Return the slice of symbol strings
	return symbols
}

// GetMetrics aggregates and computes real-time performance and telemetry metrics.
func (d *Dispatcher) GetMetrics() types.SystemMetrics {
	// Atomically load the total count of ticks processed
	totalTicks := atomic.LoadUint64(&d.ticksCount)
	// Calculate the total elapsed time in seconds since the dispatcher started
	elapsedSec := time.Since(d.startTime).Seconds()
	// Initialize messages per second variable
	var msgPerSec float64
	// Calculate messages per second if elapsed time is greater than 0
	if elapsedSec > 0 {
		msgPerSec = float64(totalTicks) / elapsedSec
	}
	// Initialize average latency variable
	var avgLatencyMicros float64
	// Atomically load total cumulative latency in nanoseconds
	totalLatency := atomic.LoadInt64(&d.totalLatencyNano)
	// Compute average latency in microseconds if ticks were processed
	if totalTicks > 0 && totalLatency > 0 {
		avgLatencyMicros = (float64(totalLatency) / float64(totalTicks)) / 1000.0
	}
	// Get the current number of queued elements in the tick channel
	chanLen := len(d.tickChan)
	// Get the total capacity of the tick channel buffer
	chanCap := cap(d.tickChan)
	// Initialize saturation percentage variable
	var saturationPct float64
	// Compute percentage of buffer utilized
	if chanCap > 0 {
		saturationPct = (float64(chanLen) / float64(chanCap)) * 100.0
	}
	// Acquire read lock to safely count unique active exchanges across all books
	d.mu.RLock()
	// Create a map to collect unique exchange names
	exchangesMap := make(map[string]struct{})
	// Loop over all registered consolidated books
	for _, book := range d.books {
		// Loop over all exchange books registered within this consolidated book
		for exName := range book.books {
			// Record the unique exchange name
			exchangesMap[exName] = struct{}{}
		}
	}
	// Count total unique active exchanges
	activeExchanges := len(exchangesMap)
	// Release read lock
	d.mu.RUnlock()
	// Return the populated SystemMetrics struct
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
	// Close the tick channel to notify consumers of shutdown
	close(d.tickChan)
}
