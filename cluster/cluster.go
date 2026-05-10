package cluster

import (
	"fmt"
	"time"
)

// ClusterManager is the unified entry point for cluster operations.
// It delegates to a concrete Mode implementation based on the topology.
type ClusterManager struct {
	mode Mode
}

// New creates a ClusterManager with the given topology mode.
// Supported modes: "shard", "sentinel", "master_slave".
func New(opts ...Option) (*ClusterManager, error) {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}

	var m Mode
	var err error
	switch o.Mode {
	case "shard":
		m, err = newShardMode(o)
	case "sentinel":
		m, err = newSentinelMode(o)
	case "master_slave":
		m, err = newMasterSlaveMode(o)
	default:
		return nil, fmt.Errorf("unknown cluster mode: %s", o.Mode)
	}
	if err != nil {
		return nil, err
	}
	return &ClusterManager{mode: m}, nil
}

// Get retrieves a value by key from the appropriate node.
func (cm *ClusterManager) Get(key string) ([]byte, error) {
	return cm.mode.Get(key)
}

// Set stores a value with optional TTL.
func (cm *ClusterManager) Set(key string, value []byte, ttl time.Duration) error {
	return cm.mode.Set(key, value, ttl)
}

// Del removes a key.
func (cm *ClusterManager) Del(key string) error {
	return cm.mode.Del(key)
}

// Len returns the total number of entries (mode-dependent semantics).
func (cm *ClusterManager) Len() (int, error) {
	return cm.mode.Len()
}

// Nodes returns a snapshot of all known nodes and their health.
func (cm *ClusterManager) Nodes() []NodeInfo {
	return cm.mode.Nodes()
}

// Close shuts down the cluster manager and all underlying connections.
func (cm *ClusterManager) Close() error {
	return cm.mode.Close()
}
