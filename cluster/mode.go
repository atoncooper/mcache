package cluster

import "time"

// Mode is the abstraction for cluster topologies (shard, sentinel, master_slave).
type Mode interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte, ttl time.Duration) error
	Del(key string) error
	Len() (int, error)
	Nodes() []NodeInfo
	Close() error

	// node returns the Node responsible for key, or an error if none is available.
	node(key string) (*Node, error)

	// Keys returns all keys matching pattern across the cluster.
	Keys(pattern string) ([]string, error)
}
