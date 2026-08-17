package conflation 

import (

	// context allows graceful shutdown and lifecycle control of the background goroutine
	"context"
    // sync provides mutex locks to prevent race conditions during buffer updates
	"sync"
	// atomic enables high-performance, lock-free telemetry counter increments
	"sync/atomic"
	// time provides high-resolution tickers, duration configuration, and timestamps
	"time"
	// types imports shared engine data structures like ConsolidatedBook and ArbitrageOpportunity
	"nexus-feed/backend/pkg/types"

)
// ConflationHandler is the callback signature triggered when a conflated batch is flushed.
// It receives the slice of latest order book snapshots and all detected arbitrage opportunities.
type ConflationHandler func(snapshots []types.ConsolidatedBook , arbs[] *types.ArbitrageOpportunity)
 // Conflator throttles and coalesces high-frequency market updates into periodic batches.

 type Conflator struct  { 
	// interval defines how frequently accumulated updates are flushed (e.g. 50ms = 20 FPS)
	interval time.Duration
	// handler is the downstream consumer callback executed on every ticker pulse
	handler ConflationHandler
	// mu is a mutual exclusion lock protecting concurrent access to pendingBooks and pendingArbs
	mu sync.Mutex
	// pendingBooks stores only the latest ConsolidatedBook snapshot per symbol (keyed by symbol)
	pendingBooks map[string]types.ConsolidatedBook
    
	// pendingArbs accumulates all arbitrage opportunities detected during the current interval
	pendingArbs []*types.ArbitrageOpportunity

   // inputCount is an atomic counter tracking the total raw updates received
	inputCount uint64

	// outputCount is an atomic counter tracking the total conflated snapshots emitted
	outputCount uint64
	// arbsEmitted is an atomic counter tracking the total arbitrage events emitted
	arbsEmitted uint64
	// lastFlush stores the timestamp when the last flush took place
	lastFlush time.Time
 }

 // NewConflator constructs and initializes a new Conflator instance.

 func NewConflator(interval time.Duration, handler ConflationHandler)  *Conflator {
            // If a zero or negative duration is passed, default to 50ms (20 updates/second)


			if interval <= 0 {
				interval = 50 * time.Millisecond

			}
			return &Conflator{
		// Set the flush interval duration
		interval: interval,
		// Set the downstream callback handler
		handler: handler,
		// Initialize the symbol-to-snapshot map
		pendingBooks: make(map[string]types.ConsolidatedBook),
		// Pre-allocate a slice for pending arbitrage opportunities with capacity of 64
		pendingArbs: make([]*types.ArbitrageOpportunity, 0, 64),
		// Set initial lastFlush timestamp to current time
		lastFlush: time.Now(),
	} }


// Push ingests an incoming consolidated book snapshot and an optional arbitrage opportunity.



func (c* Conflator) Push (snapshot types.ConsolidatedBook, arb *types.ArbitrageOpportunity) {
    // Atomically increment the total raw input updates counter
	atomic.AddUint64(&c.inputCount, 1)
	// Acquire exclusive mutex lock to ensure safe concurrent write to buffers
	c.mu.Lock()
	// Defer releasing the mutex lock when Push completes
	defer c.mu.Unlock()
	// Overwrite or insert the latest snapshot for this symbol (coalescing intermediate ticks)
	c.pendingBooks[snapshot.Symbol] = snapshot
	// If an arbitrage opportunity was detected in this tick, append it to the pending queue
	if arb != nil {
		c.pendingArbs = append(c.pendingArbs, arb)
	}




}
// PushBook ingests only an order book snapshot without arbitrage data.
func (c *Conflator) PushBook(snapshot types.ConsolidatedBook) {
	// Atomically increment the total raw input updates counter
	atomic.AddUint64(&c.inputCount, 1)
	// Acquire exclusive mutex lock
	c.mu.Lock()
	// Defer releasing the mutex lock
	defer c.mu.Unlock()
	// Store or update the snapshot for this symbol
	c.pendingBooks[snapshot.Symbol] = snapshot
}


// PushArbitrage ingests only an arbitrage opportunity.
func (c *Conflator) PushArbitrage(arb *types.ArbitrageOpportunity) {
	// If the passed pointer is nil, exit immediately
	if arb == nil {
		return
	}
	// Acquire exclusive mutex lock
	c.mu.Lock()
	// Defer releasing the mutex lock
	defer c.mu.Unlock()
	// Append the arbitrage opportunity to the pending slice
	c.pendingArbs = append(c.pendingArbs, arb)
}

// Start runs the periodic conflation event loop until the provided context is canceled.
func (c *Conflator) Start(ctx context.Context) {
	// Initialize a high-resolution ticker that ticks every configured interval (e.g. 50ms)
	ticker := time.NewTicker(c.interval)
	// Ensure the ticker is stopped and its resources freed when Start exits
	defer ticker.Stop()
	// Loop indefinitely waiting for timer ticks or shutdown signals
	for {
		select {
		// Case 1: Context cancellation or shutdown signal received
		case <-ctx.Done():
			// Perform one final flush of remaining pending data before exiting
			c.Flush()
			// Terminate the event loop
			return
		// Case 2: Ticker interval elapsed
		case <-ticker.C:
			// Flush all accumulated snapshots and arbitrage opportunities downstream
			c.Flush()
		}
	}
}


// Flush extracts pending updates, clears internal buffers, and invokes the downstream handler.
func (c *Conflator) Flush() {
	// Acquire mutex lock to safely extract and swap buffers
	c.mu.Lock()
	// If there are no pending snapshots and no pending arbitrage events, skip flush
	if len(c.pendingBooks) == 0 && len(c.pendingArbs) == 0 {
		// Release the mutex lock
		c.mu.Unlock()
		// Exit early
		return
	}
	// Pre-allocate a slice with exact capacity to hold the unique symbol snapshots
	snapshots := make([]types.ConsolidatedBook, 0, len(c.pendingBooks))
	// Iterate through the map and copy the latest snapshot for each symbol
	for _, book := range c.pendingBooks {
		snapshots = append(snapshots, book)
	}
	// Grab reference to the pending arbitrage slice
	arbs := c.pendingArbs
	// Re-allocate a clean map for the next conflation window
	c.pendingBooks = make(map[string]types.ConsolidatedBook)
	// Reset the pending arbitrage slice with fresh pre-allocated capacity
	c.pendingArbs = make([]*types.ArbitrageOpportunity, 0, 64)
	// Update the timestamp of the last flush
	c.lastFlush = time.Now()
	// Release mutex lock BEFORE calling handler to prevent deadlocks and blocking ingestion
	c.mu.Unlock()
	// Atomically add the number of emitted snapshots to output telemetry
	atomic.AddUint64(&c.outputCount, uint64(len(snapshots)))
	// Atomically add the number of emitted arbitrage events to output telemetry
	atomic.AddUint64(&c.arbsEmitted, uint64(len(arbs)))
	// If a downstream handler is registered, invoke it with the conflated batch
	if c.handler != nil {
		c.handler(snapshots, arbs)
	}
}
// GetMetrics returns real-time conflation telemetry and data compression ratio.
func (c *Conflator) GetMetrics() (inputs uint64, outputs uint64, arbs uint64, compressionRatio float64) {
	// Atomically load total input updates received
	inputs = atomic.LoadUint64(&c.inputCount)
	// Atomically load total conflated snapshot outputs emitted
	outputs = atomic.LoadUint64(&c.outputCount)
	// Atomically load total arbitrage opportunities emitted
	arbs = atomic.LoadUint64(&c.arbsEmitted)
	// If outputs were generated, compute the compression ratio (inputs / outputs)
	if outputs > 0 {
		compressionRatio = float64(inputs) / float64(outputs)
	}
	// Return telemetry metrics to the caller
	return inputs, outputs, arbs, compressionRatio
}
// GetPendingCount returns the number of symbols and arbitrage events currently pending.
func (c *Conflator) GetPendingCount() (bookCount int, arbCount int) {
	// Acquire mutex lock to safely read buffer sizes
	c.mu.Lock()
	// Defer releasing mutex lock
	defer c.mu.Unlock()
	// Return counts of pending book keys and arbitrage slice length
	return len(c.pendingBooks), len(c.pendingArbs)
}



    




	




