package mbr

import "sync"

// FeatureMatrix is a thread-safe ring buffer storing the most recent
// N WindowStats snapshots.
type FeatureMatrix struct {
	mu       sync.RWMutex
	buf      []WindowStats
	head     int // next write position
	size     int // number of entries written (<= capacity)
	capacity int
}

// NewFeatureMatrix creates a ring buffer with the given capacity.
func NewFeatureMatrix(capacity int) *FeatureMatrix {
	if capacity < 1 {
		capacity = 60
	}
	return &FeatureMatrix{
		buf:      make([]WindowStats, 0, capacity),
		capacity: capacity,
	}
}

// Push adds a new stats snapshot. When the buffer is full the oldest entry
// is overwritten.
func (m *FeatureMatrix) Push(stats WindowStats) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.size < m.capacity {
		m.buf = append(m.buf, stats)
		m.size++
		return
	}
	m.buf[m.head] = stats
	m.head = (m.head + 1) % m.capacity
}

// GetRecent returns the most recent n windows in chronological order (oldest first).
// If n exceeds the number of stored windows all available windows are returned.
// The returned slice is a copy; the caller may modify it freely.
func (m *FeatureMatrix) GetRecent(n int) []WindowStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.size == 0 {
		return nil
	}
	if n > m.size {
		n = m.size
	}

	out := make([]WindowStats, n)
	if m.size < m.capacity {
		// Buffer not yet full — simple slice from the end.
		copy(out, m.buf[m.size-n:])
		return out
	}

	// Buffer is full and wraps around. Read n entries ending at (head-1).
	start := (m.head - n + m.capacity) % m.capacity
	for i := 0; i < n; i++ {
		out[i] = m.buf[(start+i)%m.capacity]
	}
	return out
}

// Len returns the number of windows currently stored.
func (m *FeatureMatrix) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.size
}

// Last returns the most recent window, or false if empty.
func (m *FeatureMatrix) Last() (WindowStats, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.size == 0 {
		return WindowStats{}, false
	}
	if m.size < m.capacity {
		return m.buf[m.size-1], true
	}
	idx := (m.head - 1 + m.capacity) % m.capacity
	return m.buf[idx], true
}

// Capacity returns the maximum number of windows the matrix can hold.
func (m *FeatureMatrix) Capacity() int { return m.capacity }
