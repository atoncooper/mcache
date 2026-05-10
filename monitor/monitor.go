package monitor

import (
	"sync"
	"time"
)

// Monitor orchestrates periodic metric collection from a set of Collectors
// and stores results in a fixed-capacity ring buffer.
type Monitor struct {
	opts     Options
	mu       sync.RWMutex
	ring     []SystemSnapshot
	pos      int
	count    int
	full     bool
	ticker   *time.Ticker
	stop     chan struct{}
	wg       sync.WaitGroup
}

// New creates a Monitor with the given options.
func New(opts Options) *Monitor {
	return &Monitor{
		opts: opts,
		ring: make([]SystemSnapshot, 0, opts.capacity),
		stop: make(chan struct{}),
	}
}

// Start begins periodic collection in a background goroutine.
// It is safe to call Start multiple times; subsequent calls are no-ops.
// After Stop, Start can be called again to resume.
func (m *Monitor) Start() {
	m.mu.Lock()
	if m.ticker != nil {
		m.mu.Unlock()
		return
	}
	m.ticker = time.NewTicker(m.opts.interval)
	// Recreate stop channel in case of restart after Stop.
	m.stop = make(chan struct{})
	m.mu.Unlock()

	tick := m.ticker.C
	stop := m.stop

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for {
			select {
			case <-tick:
				m.collect()
			case <-stop:
				return
			}
		}
	}()
}

// Stop halts periodic collection. It blocks until the background goroutine exits.
func (m *Monitor) Stop() {
	m.mu.Lock()
	if m.ticker == nil {
		m.mu.Unlock()
		return
	}
	t := m.ticker
	m.ticker = nil
	m.mu.Unlock()

	t.Stop()
	close(m.stop)
	m.wg.Wait()
}

// Latest returns the most recent snapshot, if any.
func (m *Monitor) Latest() (SystemSnapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.ring) == 0 {
		return SystemSnapshot{}, false
	}
	idx := m.pos
	if !m.full {
		idx = len(m.ring) - 1
	} else {
		idx = (m.pos - 1 + m.opts.capacity) % m.opts.capacity
	}
	return m.ring[idx], true
}

// History returns all stored snapshots in chronological order (oldest first).
func (m *Monitor) History() []SystemSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.ring) == 0 {
		return nil
	}
	out := make([]SystemSnapshot, len(m.ring))
	if !m.full {
		copy(out, m.ring)
		return out
	}
	for i := range len(m.ring) {
		out[i] = m.ring[(m.pos+i)%m.opts.capacity]
	}
	return out
}

// Count returns the total number of snapshots collected (including overwritten).
func (m *Monitor) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.count
}

func (m *Monitor) collect() {
	if len(m.opts.collectors) == 0 {
		return
	}
	snap := SystemSnapshot{Timestamp: time.Now()}
	for _, c := range m.opts.collectors {
		partial, err := c.Collect()
		if err != nil {
			continue
		}
		if partial.CPU != nil {
			snap.CPU = partial.CPU
		}
		if partial.Memory != nil {
			snap.Memory = partial.Memory
		}
		snap.IO = append(snap.IO, partial.IO...)
		snap.Network = append(snap.Network, partial.Network...)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.ring) < m.opts.capacity {
		m.ring = append(m.ring, snap)
		if len(m.ring) == m.opts.capacity {
			m.full = true
		}
	} else {
		m.ring[m.pos] = snap
		m.pos = (m.pos + 1) % m.opts.capacity
	}
	m.count++
}
