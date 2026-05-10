package cluster

import (
	"context"
	"sync/atomic"
	"time"

	mnet "github.com/atoncooper/mcache/net"
)

// Node represents a single cache server node in the cluster.
type Node struct {
	Addr    string
	Weight  int
	Client  *mnet.Client
	Healthy atomic.Bool
}

// NodeInfo is a read-only snapshot of node state.
type NodeInfo struct {
	Addr    string `json:"addr"`
	Weight  int    `json:"weight"`
	Healthy bool   `json:"healthy"`
}

// Info returns a snapshot of the node.
func (n *Node) Info() NodeInfo {
	return NodeInfo{
		Addr:    n.Addr,
		Weight:  n.Weight,
		Healthy: n.Healthy.Load(),
	}
}

// Close closes the node connection.
func (n *Node) Close() {
	if n.Client != nil {
		n.Client.Close()
	}
}

// probeNodeHealth checks whether a node responds within timeout without
// blocking the caller indefinitely.
func probeNodeHealth(n *Node, timeout time.Duration) bool {
	if n.Client == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		_, err := n.Client.Len()
		done <- (err == nil)
	}()

	select {
	case healthy := <-done:
		return healthy
	case <-ctx.Done():
		return false
	}
}

// dial creates a net.Client for the node.
func (n *Node) dial() error {
	c, err := mnet.NewClient(n.Addr)
	if err != nil {
		return err
	}
	n.Client = c
	n.Healthy.Store(true)
	return nil
}
