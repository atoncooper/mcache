package cluster

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// MasterSlaveMode sends writes to the master and load-balances reads across slaves.
// When the master stays unhealthy beyond FailoverTimeout, a healthy slave is promoted.
type MasterSlaveMode struct {
	master           atomic.Pointer[Node]
	slaves           []*Node
	opts             Options
	mu               sync.RWMutex
	nextSlave        uint32 // atomic round-robin
	stopCh           chan struct{}
	wg               sync.WaitGroup
	failoverMu       sync.Mutex
	firstUnhealthyAt time.Time
}

// newMasterSlaveMode creates a master-slave topology.
func newMasterSlaveMode(opts Options) (*MasterSlaveMode, error) {
	if opts.Master == "" {
		return nil, errors.New("master-slave mode requires a master address")
	}
	if len(opts.Slaves) == 0 {
		return nil, errors.New("master-slave mode requires at least one slave")
	}

	ms := &MasterSlaveMode{
		slaves: make([]*Node, 0, len(opts.Slaves)),
		opts:   opts,
		stopCh: make(chan struct{}),
	}

	master := &Node{Addr: opts.Master, Weight: 1}
	if err := master.dial(); err != nil {
		return nil, err
	}
	ms.master.Store(master)

	for _, addr := range opts.Slaves {
		n := &Node{Addr: addr, Weight: 1}
		if err := n.dial(); err != nil {
			ms.Close()
			return nil, err
		}
		ms.slaves = append(ms.slaves, n)
	}

	ms.wg.Add(1)
	go ms.healthLoop()
	return ms, nil
}

// Get load-balances across healthy slaves (falls back to master if no slave is healthy).
func (ms *MasterSlaveMode) Get(key string) ([]byte, error) {
	slave := ms.pickSlave()
	if slave != nil && slave.Healthy.Load() {
		return slave.Client.Get(key)
	}
	m := ms.master.Load()
	if m != nil && m.Healthy.Load() {
		return m.Client.Get(key)
	}
	return nil, errors.New("no healthy node available")
}

// Set always writes to the master.
func (ms *MasterSlaveMode) Set(key string, value []byte, ttl time.Duration) error {
	m := ms.master.Load()
	if m == nil || !m.Healthy.Load() {
		return errors.New("master is not healthy")
	}
	return m.Client.Set(key, value, ttl)
}

// Del always deletes from the master.
func (ms *MasterSlaveMode) Del(key string) error {
	m := ms.master.Load()
	if m == nil || !m.Healthy.Load() {
		return errors.New("master is not healthy")
	}
	return m.Client.Del(key)
}

// Len returns the entry count of the master.
func (ms *MasterSlaveMode) Len() (int, error) {
	m := ms.master.Load()
	if m == nil || !m.Healthy.Load() {
		return 0, errors.New("master is not healthy")
	}
	return m.Client.Len()
}

// Keys returns keys matching pattern from the master.
func (ms *MasterSlaveMode) Keys(pattern string) ([]string, error) {
	m := ms.master.Load()
	if m == nil || !m.Healthy.Load() {
		return nil, errors.New("no healthy master available")
	}
	return m.Client.Keys(pattern)
}

// Nodes returns a snapshot of master + slaves.
func (ms *MasterSlaveMode) Nodes() []NodeInfo {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	infos := make([]NodeInfo, 0, 1+len(ms.slaves))
	if m := ms.master.Load(); m != nil {
		infos = append(infos, m.Info())
	}
	for _, s := range ms.slaves {
		infos = append(infos, s.Info())
	}
	return infos
}

// Close shuts down connections.
func (ms *MasterSlaveMode) Close() error {
	close(ms.stopCh)
	ms.wg.Wait()
	if m := ms.master.Load(); m != nil {
		m.Close()
	}
	ms.mu.Lock()
	defer ms.mu.Unlock()
	for _, s := range ms.slaves {
		s.Close()
	}
	return nil
}

func (ms *MasterSlaveMode) node(key string) (*Node, error) {
	m := ms.master.Load()
	if m == nil || !m.Healthy.Load() {
		return nil, errors.New("no healthy master available")
	}
	return m, nil
}

func (ms *MasterSlaveMode) pickSlave() *Node {
	ms.mu.RLock()
	slaves := make([]*Node, len(ms.slaves))
	copy(slaves, ms.slaves)
	ms.mu.RUnlock()

	if len(slaves) == 0 {
		return nil
	}
	for range len(slaves) {
		idx := atomic.AddUint32(&ms.nextSlave, 1) % uint32(len(slaves))
		s := slaves[idx]
		if s.Healthy.Load() {
			return s
		}
	}
	return nil
}

func (ms *MasterSlaveMode) healthLoop() {
	defer ms.wg.Done()
	ticker := time.NewTicker(ms.opts.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ms.checkAndFailover()
		case <-ms.stopCh:
			return
		}
	}
}

func (ms *MasterSlaveMode) checkAndFailover() {
	ms.probeSlaves()

	m := ms.master.Load()
	if m == nil || m.Client == nil {
		ms.tryFailover()
		return
	}

	healthy := probeNodeHealth(m, ms.opts.HealthCheckTimeout)
	m.Healthy.Store(healthy)

	if healthy {
		ms.firstUnhealthyAt = time.Time{}
		return
	}

	ms.opts.logf("master-slave: master %s unhealthy", m.Addr)

	now := time.Now()
	if ms.firstUnhealthyAt.IsZero() {
		ms.firstUnhealthyAt = now
		return
	}

	if now.Sub(ms.firstUnhealthyAt) >= ms.opts.FailoverTimeout {
		ms.tryFailover()
	}
}

func (ms *MasterSlaveMode) probeSlaves() {
	ms.mu.RLock()
	slaves := make([]*Node, len(ms.slaves))
	copy(slaves, ms.slaves)
	ms.mu.RUnlock()

	for _, s := range slaves {
		if s.Client == nil {
			s.Healthy.Store(false)
			continue
		}
		s.Healthy.Store(probeNodeHealth(s, ms.opts.HealthCheckTimeout))
	}
}

func (ms *MasterSlaveMode) tryFailover() {
	ms.failoverMu.Lock()
	defer ms.failoverMu.Unlock()

	current := ms.master.Load()
	if current != nil && current.Healthy.Load() {
		return
	}

	ms.mu.RLock()
	var best *Node
	for _, s := range ms.slaves {
		if s == current {
			continue
		}
		if s.Client != nil && s.Healthy.Load() {
			best = s
			break
		}
	}
	ms.mu.RUnlock()

	if best == nil {
		ms.opts.logf("master-slave: failover failed, no healthy slave found")
		return
	}

	old := ms.master.Swap(best)
	ms.firstUnhealthyAt = time.Time{}

	// Remove promoted slave from slaves list, add demoted master.
	ms.mu.Lock()
	for i, s := range ms.slaves {
		if s == best {
			ms.slaves = append(ms.slaves[:i], ms.slaves[i+1:]...)
			break
		}
	}
	if old != nil {
		old.Healthy.Store(false)
		ms.slaves = append(ms.slaves, old)
	}
	ms.mu.Unlock()

	if old != nil {
		ms.opts.logf("master-slave: promoted %s to master (old master %s demoted to slave)", best.Addr, old.Addr)
	}
}
