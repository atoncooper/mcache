package raft

import "time"

// Config holds Raft consensus parameters.
type Config struct {
	NodeID            uint64
	Peers             []string // address list of all peers (including self)
	HeartbeatInterval time.Duration
	ElectionTimeout   time.Duration
	CommitTimeout     time.Duration
	MaxLogEntries     int
}

// DefaultConfig returns sensible defaults for a 3-node cluster.
func DefaultConfig() Config {
	return Config{
		HeartbeatInterval: 100 * time.Millisecond,
		ElectionTimeout:   500 * time.Millisecond,
		CommitTimeout:     50 * time.Millisecond,
		MaxLogEntries:     10000,
	}
}
