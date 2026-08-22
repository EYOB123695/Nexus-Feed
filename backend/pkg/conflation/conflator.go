package conflation

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"nexus-feed/backend/pkg/types"
)

// ConflationHandler is the callback signature triggered when a conflated batch is flushed.
type ConflationHandler func(snapshots []types.ConsolidatedBook, arbs []*types.ArbitrageOpportunity)

// Conflator throttles and coalesces high-frequency market updates into periodic batches.
type Conflator struct {
	interval time.Duration
	handler  ConflationHandler
	mu       sync.Mutex

	pendingBooks map[string]types.ConsolidatedBook
	pendingArbs  []*types.ArbitrageOpportunity

	inputCount  uint64
	outputCount uint64
	arbsEmitted uint64
	lastFlush   time.Time
}

// NewConflator constructs and initializes a new Conflator instance.
func NewConflator(interval time.Duration, handler ConflationHandler) *Conflator {
	if interval <= 0 {
		interval = 50 * time.Millisecond
	}
	return &Conflator{
		interval:     interval,
		handler:      handler,
		pendingBooks: make(map[string]types.ConsolidatedBook),
		pendingArbs:  make([]*types.ArbitrageOpportunity, 0, 64),
		lastFlush:    time.Now(),
	}
}

// Push ingests an incoming consolidated book snapshot and an optional arbitrage opportunity.
func (c *Conflator) Push(snapshot types.ConsolidatedBook, arb *types.ArbitrageOpportunity) {
	atomic.AddUint64(&c.inputCount, 1)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.pendingBooks[snapshot.Symbol] = snapshot
	if arb != nil {
		c.pendingArbs = append(c.pendingArbs, arb)
	}
}

// PushBook ingests only an order book snapshot without arbitrage data.
func (c *Conflator) PushBook(snapshot types.ConsolidatedBook) {
	atomic.AddUint64(&c.inputCount, 1)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.pendingBooks[snapshot.Symbol] = snapshot
}

// PushArbitrage ingests only an arbitrage opportunity.
func (c *Conflator) PushArbitrage(arb *types.ArbitrageOpportunity) {
	if arb == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.pendingArbs = append(c.pendingArbs, arb)
}

// Start runs the periodic conflation event loop until the provided context is canceled.
func (c *Conflator) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.Flush()
			return
		case <-ticker.C:
			c.Flush()
		}
	}
}

// Flush extracts pending updates, clears internal buffers, and invokes the downstream handler.
func (c *Conflator) Flush() {
	c.mu.Lock()
	if len(c.pendingBooks) == 0 && len(c.pendingArbs) == 0 {
		c.mu.Unlock()
		return
	}

	snapshots := make([]types.ConsolidatedBook, 0, len(c.pendingBooks))
	for _, book := range c.pendingBooks {
		snapshots = append(snapshots, book)
	}

	arbs := c.pendingArbs
	c.pendingBooks = make(map[string]types.ConsolidatedBook)
	c.pendingArbs = make([]*types.ArbitrageOpportunity, 0, 64)
	c.lastFlush = time.Now()
	c.mu.Unlock()

	atomic.AddUint64(&c.outputCount, uint64(len(snapshots)))
	atomic.AddUint64(&c.arbsEmitted, uint64(len(arbs)))

	if c.handler != nil {
		c.handler(snapshots, arbs)
	}
}

// GetMetrics returns real-time conflation telemetry and data compression ratio.
func (c *Conflator) GetMetrics() (inputs uint64, outputs uint64, arbs uint64, compressionRatio float64) {
	inputs = atomic.LoadUint64(&c.inputCount)
	outputs = atomic.LoadUint64(&c.outputCount)
	arbs = atomic.LoadUint64(&c.arbsEmitted)

	if outputs > 0 {
		compressionRatio = float64(inputs) / float64(outputs)
	}
	return inputs, outputs, arbs, compressionRatio
}

// GetPendingCount returns the number of symbols and arbitrage events currently pending.
func (c *Conflator) GetPendingCount() (bookCount int, arbCount int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.pendingBooks), len(c.pendingArbs)
}



    




	




