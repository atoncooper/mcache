package cluster

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// SentinelMode monitors a master node and auto-failovers to a replica when
// the master becomes unhealthy for longer than FailoverTimeout.
type SentinelMode struct {
	master           atomic.Pointer[Node]
	replicas         []*Node
	opts             Options
	mu               sync.RWMutex
	stopCh           chan struct{}
	wg               sync.WaitGroup
	failoverMu       sync.Mutex
	firstUnhealthyAt time.Time // zero when master is currently healthy
}

// newSentinelMode creates a sentinel topology.
func newSentinelMode(opts Options) (*SentinelMode, error) {
	if opts.Master == "" {
		return nil, errors.New("sentinel mode requires a master address")
	}
	if len(opts.Sentinels) == 0 {
		return nil, errors.New("sentinel mode requires at least one sentinel/replica")
	}

	sm := &SentinelMode{
		replicas: make([]*Node, 0),
		opts:     opts,
		stopCh:   make(chan struct{}),
	}

	master := &Node{Addr: opts.Master}
	if err := master.dial(); err != nil {
		return nil, err
	}
	sm.master.Store(master)

	for _, addr := range opts.Sentinels {
		n := &Node{Addr: addr}
		if err := n.dial(); err != nil {
			sm.opts.logf("sentinel: failed to dial replica %s: %v", addr, err)
			continue
		}
		sm.replicas = append(sm.replicas, n)
	}

	if len(sm.replicas) == 0 {
		master.Close()
		return nil, errors.New("sentinel mode: no reachable replicas")
	}

	sm.wg.Add(1)
	go sm.monitorLoop()
	return sm, nil
}

// Get reads from the current master.
func (sm *SentinelMode) Get(key string) ([]byte, error) {
	m := sm.currentMaster()
	if m == nil || !m.Healthy.Load() {
		return nil, errors.New("no healthy master available")
	}
	return m.Client.Get(key)
}

// Set writes to the current master.
func (sm *SentinelMode) Set(key string, value []byte, ttl time.Duration) error {
	m := sm.currentMaster()
	if m == nil || !m.Healthy.Load() {
		return errors.New("no healthy master available")
	}
	return m.Client.Set(key, value, ttl)
}

// Del deletes from the current master.
func (sm *SentinelMode) Del(key string) error {
	m := sm.currentMaster()
	if m == nil || !m.Healthy.Load() {
		return errors.New("no healthy master available")
	}
	return m.Client.Del(key)
}

// Len returns the entry count of the current master.
func (sm *SentinelMode) Len() (int, error) {
	m := sm.currentMaster()
	if m == nil || !m.Healthy.Load() {
		return 0, errors.New("no healthy master available")
	}
	return m.Client.Len()
}

// Keys returns keys matching pattern from the master.
func (sm *SentinelMode) Keys(pattern string) ([]string, error) {
	m := sm.master.Load()
	if m == nil || !m.Healthy.Load() {
		return nil, errors.New("no healthy master available")
	}
	return m.Client.Keys(pattern)
}

// Nodes returns a snapshot of master + replicas.
func (sm *SentinelMode) Nodes() []NodeInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	infos := make([]NodeInfo, 0, 1+len(sm.replicas))
	if m := sm.master.Load(); m != nil {
		infos = append(infos, m.Info())
	}
	for _, r := range sm.replicas {
		infos = append(infos, r.Info())
	}
	return infos
}

// Close shuts down the sentinel.
func (sm *SentinelMode) Close() error {
	close(sm.stopCh)
	sm.wg.Wait()
	if m := sm.master.Load(); m != nil {
		m.Close()
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for _, r := range sm.replicas {
		r.Close()
	}
	return nil
}

func (sm *SentinelMode) node(key string) (*Node, error) {
	m := sm.master.Load()
	if m == nil || !m.Healthy.Load() {
		return nil, errors.New("no healthy master available")
	}
	return m, nil
}

func (sm *SentinelMode) currentMaster() *Node {
	return sm.master.Load()
}

func (sm *SentinelMode) monitorLoop() {
	defer sm.wg.Done()
	ticker := time.NewTicker(sm.opts.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.checkAndFailover()
		case <-sm.stopCh:
			return
		}
	}
}

func (sm *SentinelMode) checkAndFailover() {
	// Always probe replicas so their health status stays current.
	sm.probeReplicas()

	m := sm.master.Load()
	if m == nil || m.Client == nil {
		sm.tryFailover()
		return
	}

	healthy := probeNodeHealth(m, sm.opts.HealthCheckTimeout)
	m.Healthy.Store(healthy)

	if healthy {
		sm.firstUnhealthyAt = time.Time{}
		return
	}

	sm.opts.logf("sentinel: master %s unhealthy", m.Addr)

	now := time.Now()
	if sm.firstUnhealthyAt.IsZero() {
		sm.firstUnhealthyAt = now
		return
	}

	if now.Sub(sm.firstUnhealthyAt) >= sm.opts.FailoverTimeout {
		sm.tryFailover()
	}
}

// probeReplicas updates the health of every replica.
func (sm *SentinelMode) probeReplicas() {
	sm.mu.RLock()
	replicas := make([]*Node, len(sm.replicas))
	copy(replicas, sm.replicas)
	sm.mu.RUnlock()

	for _, r := range replicas {
		if r.Client == nil {
			r.Healthy.Store(false)
			continue
		}
		r.Healthy.Store(probeNodeHealth(r, sm.opts.HealthCheckTimeout))
	}
}

func (sm *SentinelMode) tryFailover() {
	sm.failoverMu.Lock()
	defer sm.failoverMu.Unlock()

	current := sm.master.Load()
	if current != nil && current.Healthy.Load() {
		return
	}

	sm.mu.RLock()
	var best *Node
	for _, r := range sm.replicas {
		if r == current {
			continue
		}
		if r.Client != nil && r.Healthy.Load() {
			best = r
			break
		}
	}
	sm.mu.RUnlock()

	if best == nil {
		sm.opts.logf("sentinel: failover failed, no healthy replica found")
		return
	}

	old := sm.master.Swap(best)
	sm.firstUnhealthyAt = time.Time{}

	// Update the replicas list: remove the promoted node, add the demoted master.
	sm.mu.Lock()
	for i, r := range sm.replicas {
		if r == best {
			sm.replicas = append(sm.replicas[:i], sm.replicas[i+1:]...)
			break
		}
	}
	if old != nil {
		old.Healthy.Store(false)
		sm.replicas = append(sm.replicas, old)
	}
	sm.mu.Unlock()

	if old != nil {
		sm.opts.logf("sentinel: promoted %s to master (old master %s demoted to replica)", best.Addr, old.Addr)
	}
}
