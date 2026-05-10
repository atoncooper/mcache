package infra

import (
	"maps"
	"sync"
	"time"
)

// HeartbeatRegistry tracks the liveness of registered components.
// Each component must call Beat(id) periodically; the Alerter goroutine
// checks for timeouts and emits alerts.
type HeartbeatRegistry struct {
	mu       sync.RWMutex
	beats    map[string]time.Time
	timeouts map[string]time.Duration
}

// NewHeartbeatRegistry creates an empty registry.
func NewHeartbeatRegistry() *HeartbeatRegistry {
	return &HeartbeatRegistry{
		beats:    make(map[string]time.Time),
		timeouts: make(map[string]time.Duration),
	}
}

// Register adds a component with its heartbeat timeout.
func (r *HeartbeatRegistry) Register(id string, timeout time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.timeouts[id] = timeout
	r.beats[id] = time.Now()
}

// Unregister removes a component.
func (r *HeartbeatRegistry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.timeouts, id)
	delete(r.beats, id)
}

// Beat records a heartbeat from component id.
func (r *HeartbeatRegistry) Beat(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.beats[id] = time.Now()
}

// Check returns a list of component ids whose last heartbeat has exceeded
// their configured timeout.
func (r *HeartbeatRegistry) Check() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	var expired []string
	for id, timeout := range r.timeouts {
		last, ok := r.beats[id]
		if !ok || now.Sub(last) > timeout {
			expired = append(expired, id)
		}
	}
	return expired
}

// Stats returns current heartbeat timestamps for inspection.
func (r *HeartbeatRegistry) Stats() map[string]time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]time.Time, len(r.beats))
	maps.Copy(out, r.beats)
	return out
}
