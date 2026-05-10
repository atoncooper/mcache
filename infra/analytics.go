package infra

import (
	"sync"
	"time"
)

// EventType categorises cache lifecycle events for analytics.
type EventType string

const (
	EventHit      EventType = "hit"
	EventMiss     EventType = "miss"
	EventSet      EventType = "set"
	EventDel      EventType = "del"
	EventEvict    EventType = "evict"
	EventRehash   EventType = "rehash"
)

// Event is a single observation sent to analytics backends (Kafka, ClickHouse, Flink, etc.).
type Event struct {
	Type      EventType `json:"type"`
	Key       string    `json:"key,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Meta      map[string]any `json:"meta,omitempty"`
}

// AnalyticsCollector is the abstraction for big-data pipelines.
type AnalyticsCollector interface {
	Collect(e Event)
	Flush() error
	Stop()
}

// BatchCollector buffers events and flushes asynchronously.
// It never blocks the hot path: when the buffer is full, the oldest events are dropped.
type BatchCollector struct {
	mu            sync.Mutex
	buf           []Event
	cap           int
	ticker        *time.Ticker
	flushFn       func([]Event) error
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// NewBatchCollector creates a collector with the given buffer capacity, flush interval,
// and a custom flush function (e.g. send to Kafka, ClickHouse, S3, etc.).
func NewBatchCollector(capacity int, flushInterval time.Duration, flushFn func([]Event) error) *BatchCollector {
	if capacity < 1 {
		capacity = 1000
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}
	bc := &BatchCollector{
		buf:     make([]Event, 0, capacity),
		cap:     capacity,
		ticker:  time.NewTicker(flushInterval),
		flushFn: flushFn,
		stopCh:  make(chan struct{}),
	}
	bc.wg.Add(1)
	go bc.loop()
	return bc
}

// Collect appends an event to the buffer. If the buffer is full the oldest event is dropped.
func (bc *BatchCollector) Collect(e Event) {
	bc.mu.Lock()
	if len(bc.buf) >= bc.cap {
		bc.buf = bc.buf[1:]
	}
	bc.buf = append(bc.buf, e)
	bc.mu.Unlock()
}

// Flush immediately sends the current buffer to the flush function.
func (bc *BatchCollector) Flush() error {
	bc.mu.Lock()
	if len(bc.buf) == 0 {
		bc.mu.Unlock()
		return nil
	}
	batch := make([]Event, len(bc.buf))
	copy(batch, bc.buf)
	bc.buf = bc.buf[:0]
	bc.mu.Unlock()

	if bc.flushFn != nil {
		return bc.flushFn(batch)
	}
	return nil
}

// Stop halts the background flush loop.
func (bc *BatchCollector) Stop() {
	close(bc.stopCh)
	bc.wg.Wait()
	bc.ticker.Stop()
}

func (bc *BatchCollector) loop() {
	defer bc.wg.Done()
	for {
		select {
		case <-bc.ticker.C:
			_ = bc.Flush()
		case <-bc.stopCh:
			_ = bc.Flush()
			return
		}
	}
}
