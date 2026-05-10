package raft

import (
	"testing"
	"time"
)

func TestRaftLeaderElection(t *testing.T) {
	cluster := setupCluster(t, 3)
	defer cluster.shutdown()

	leader := cluster.waitForLeader(t, 2*time.Second)
	if leader == 0 {
		t.Fatal("no leader elected")
	}

	// verify exactly one leader
	leaders := 0
	for _, n := range cluster.nodes {
		if n.State() == Leader {
			leaders++
		}
	}
	if leaders != 1 {
		t.Fatalf("expected 1 leader, got %d", leaders)
	}
}

func TestRaftLogReplication(t *testing.T) {
	cluster := setupCluster(t, 3)
	defer cluster.shutdown()

	leader := cluster.waitForLeader(t, 2*time.Second)
	if leader == 0 {
		t.Fatal("no leader elected")
	}

	// propose a command
	cmd := []byte("hello")
	if !cluster.nodes[leader-1].Propose(cmd) {
		t.Fatal("leader refused proposal")
	}

	// wait for all nodes to apply
	cluster.waitForApplied(t, 1, 2*time.Second)

	// verify log length on all nodes
	for i, n := range cluster.nodes {
		idx, term := n.raft.log.Last()
		if idx != 1 {
			t.Fatalf("node %d: expected last index 1, got %d", i+1, idx)
		}
		if term != n.Term() {
			t.Fatalf("node %d: expected term %d, got %d", i+1, n.Term(), term)
		}
	}
}

func TestRaftFollowerDropPropose(t *testing.T) {
	cluster := setupCluster(t, 3)
	defer cluster.shutdown()

	// don't wait for leader; try to propose from a follower
	follower := cluster.nodes[0]
	if follower.Propose([]byte("x")) {
		t.Fatal("follower should reject proposal")
	}
}

// ------------------------------------------------------------------
// test harness
// ------------------------------------------------------------------

type testCluster struct {
	nodes []*Node
}

func setupCluster(t *testing.T, size int) *testCluster {
	t.Helper()
	transports := make([]*MemoryTransport, size)
	for i := range size {
		transports[i] = NewMemoryTransport(uint64(i + 1))
	}
	// fully connect the mesh
	for i := range size {
		for j := range size {
			if i != j {
				transports[i].RegisterPeer(uint64(j+1), transports[j])
			}
		}
	}

	tc := &testCluster{nodes: make([]*Node, size)}
	for i := range size {
		cfg := Config{
			NodeID:            uint64(i + 1),
			Peers:             make([]string, size),
			HeartbeatInterval: 50 * time.Millisecond,
			ElectionTimeout:   200 * time.Millisecond,
		}
		tc.nodes[i] = NewNode(cfg, transports[i], nil)
		transports[i].SetReceiveCh(tc.nodes[i].raft.RPCCh())
		tc.nodes[i].Start()
	}
	return tc
}

func (tc *testCluster) shutdown() {
	for _, n := range tc.nodes {
		n.Shutdown()
	}
}

func (tc *testCluster) waitForLeader(t *testing.T, timeout time.Duration) uint64 {
	t.Helper()
	deadline := time.After(timeout)
	for {
		for _, n := range tc.nodes {
			if n.State() == Leader {
				return n.raft.config.NodeID
			}
		}
		select {
		case <-deadline:
			return 0
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (tc *testCluster) waitForApplied(t *testing.T, count int, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		done := true
		for i, n := range tc.nodes {
			if int(n.raft.lastApplied) < count {
				done = false
				t.Logf("node %d lastApplied=%d", i+1, n.raft.lastApplied)
				break
			}
		}
		if done {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d applied entries", count)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
