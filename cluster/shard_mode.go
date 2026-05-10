package cluster

import (
	"errors"
	"hash/fnv"
	"sync"
	"time"
)

// ShardMode distributes keys across multiple nodes using consistent hashing.
type ShardMode struct {
	nodes    []*Node
	mu       sync.RWMutex
	hc       *HealthChecker
	closedCh chan struct{}
}

// newShardMode creates a shard topology from node configs.
func newShardMode(opts Options) (*ShardMode, error) {
	sm := &ShardMode{
		nodes:    make([]*Node, 0, len(opts.Nodes)),
		closedCh: make(chan struct{}),
	}

	for _, nc := range opts.Nodes {
		if nc.Addr == "" {
			continue
		}
		n := &Node{Addr: nc.Addr, Weight: nc.Weight}
		if n.Weight <= 0 {
			n.Weight = 1
		}
		if err := n.dial(); err != nil {
			sm.Close()
			return nil, err
		}
		sm.nodes = append(sm.nodes, n)
	}

	if len(sm.nodes) == 0 {
		return nil, errors.New("shard mode requires at least one node")
	}

	sm.hc = NewHealthChecker(sm.nodes, opts.HealthCheckInterval, opts.HealthCheckTimeout, opts.ErrorLog)
	sm.hc.Start()
	return sm, nil
}

func (sm *ShardMode) node(key string) (*Node, error) {
	n := sm.pickNode(key)
	if n == nil || !n.Healthy.Load() {
		return nil, errors.New("no healthy node available")
	}
	return n, nil
}

// Get routes the key to the responsible node.
func (sm *ShardMode) Get(key string) ([]byte, error) {
	n := sm.pickNode(key)
	if n == nil {
		return nil, errors.New("no healthy node available")
	}
	return n.Client.Get(key)
}

// Set routes the key to the responsible node.
func (sm *ShardMode) Set(key string, value []byte, ttl time.Duration) error {
	n := sm.pickNode(key)
	if n == nil {
		return errors.New("no healthy node available")
	}
	return n.Client.Set(key, value, ttl)
}

// Del routes the key to the responsible node.
func (sm *ShardMode) Del(key string) error {
	n := sm.pickNode(key)
	if n == nil {
		return errors.New("no healthy node available")
	}
	return n.Client.Del(key)
}

// Len returns the total entries across all healthy nodes.
func (sm *ShardMode) Len() (int, error) {
	sm.mu.RLock()
	nodes := make([]*Node, len(sm.nodes))
	copy(nodes, sm.nodes)
	sm.mu.RUnlock()

	total := 0
	for _, n := range nodes {
		if !n.Healthy.Load() || n.Client == nil {
			continue
		}
		c, err := n.Client.Len()
		if err != nil {
			return 0, err
		}
		total += c
	}
	return total, nil
}

// Nodes returns a snapshot of node info.
func (sm *ShardMode) Nodes() []NodeInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	infos := make([]NodeInfo, 0, len(sm.nodes))
	for _, n := range sm.nodes {
		infos = append(infos, n.Info())
	}
	return infos
}

// Keys aggregates keys matching pattern from all healthy nodes.
func (sm *ShardMode) Keys(pattern string) ([]string, error) {
	sm.mu.RLock()
	nodes := make([]*Node, len(sm.nodes))
	copy(nodes, sm.nodes)
	sm.mu.RUnlock()

	var all []string
	for _, n := range nodes {
		if !n.Healthy.Load() || n.Client == nil {
			continue
		}
		keys, err := n.Client.Keys(pattern)
		if err != nil {
			return nil, err
		}
		all = append(all, keys...)
	}
	return all, nil
}

// Close shuts down connections and health checker.
func (sm *ShardMode) Close() error {
	close(sm.closedCh)
	if sm.hc != nil {
		sm.hc.Stop()
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for _, n := range sm.nodes {
		n.Close()
	}
	return nil
}

func (sm *ShardMode) pickNode(key string) *Node {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// simple weighted hash: replicate virtual nodes by weight
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	hash := h.Sum32()

	var totalWeight int
	for _, n := range sm.nodes {
		if n.Healthy.Load() {
			totalWeight += n.Weight
		}
	}
	if totalWeight == 0 {
		return nil
	}

	idx := hash % uint32(totalWeight)
	var cursor uint32
	for _, n := range sm.nodes {
		if !n.Healthy.Load() {
			continue
		}
		cursor += uint32(n.Weight)
		if idx < cursor {
			return n
		}
	}
	return nil
}
