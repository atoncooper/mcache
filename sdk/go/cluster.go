package mcache

import (
	"hash/fnv"
	"time"
)

// ClusterClient distributes keys across multiple mcache server nodes using
// a hash-based placement strategy.
type ClusterClient struct {
	nodes []*Client
	hash  func(string) uint32
}

// NewClusterClient creates a cluster client from a list of server addresses.
// Each address gets its own connection pool.
func NewClusterClient(addrs []string, opts ...Option) (*ClusterClient, error) {
	if len(addrs) == 0 {
		return nil, ErrNoNodes
	}

	nodes := make([]*Client, len(addrs))
	for i, addr := range addrs {
		c, err := NewClient(addr, opts...)
		if err != nil {
			for j := range i {
				nodes[j].Close()
			}
			return nil, err
		}
		nodes[i] = c
	}

	return &ClusterClient{
		nodes: nodes,
		hash: func(key string) uint32 {
			h := fnv.New32a()
			_, _ = h.Write([]byte(key))
			return h.Sum32()
		},
	}, nil
}

func (cc *ClusterClient) pickNode(key string) *Client {
	if len(cc.nodes) == 1 {
		return cc.nodes[0]
	}
	idx := cc.hash(key) % uint32(len(cc.nodes))
	return cc.nodes[idx]
}

// Get retrieves a value from the node responsible for the key.
func (cc *ClusterClient) Get(key string, dest any) error {
	return cc.pickNode(key).Get(key, dest)
}

// Set stores a value on the node responsible for the key.
func (cc *ClusterClient) Set(key string, value any, ttl time.Duration) error {
	return cc.pickNode(key).Set(key, value, ttl)
}

// Del removes a key from the node responsible for it.
func (cc *ClusterClient) Del(key string) error {
	return cc.pickNode(key).Del(key)
}

// Len returns the total number of entries across all nodes.
func (cc *ClusterClient) Len() (int, error) {
	total := 0
	for _, node := range cc.nodes {
		n, err := node.Len()
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// Cleanup triggers expiration cleanup on all nodes and returns total removed.
func (cc *ClusterClient) Cleanup() (int, error) {
	total := 0
	for _, node := range cc.nodes {
		n, err := node.Cleanup()
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// Close closes all node connections.
func (cc *ClusterClient) Close() error {
	var firstErr error
	for _, node := range cc.nodes {
		if err := node.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
