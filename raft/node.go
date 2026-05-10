package raft

import "sync"

// Node is a higher-level wrapper around Raft that manages lifecycle
// and drains the apply channel so the state machine never blocks.
type Node struct {
	raft    *Raft
	onApply func(ApplyMsg)

	shutdownCh chan struct{}
	wg         sync.WaitGroup
}

// NewNode creates a Node. The optional onApply callback is invoked
// for every committed log entry on a dedicated goroutine.
func NewNode(cfg Config, trans Transport, onApply func(ApplyMsg)) *Node {
	return &Node{
		raft:       NewRaft(cfg, trans),
		onApply:    onApply,
		shutdownCh: make(chan struct{}),
	}
}

// Start launches the Raft event loop and the apply drainer.
func (n *Node) Start() {
	n.raft.Start()
	n.wg.Add(1)
	go n.runApply()
}

// Shutdown stops the event loop and closes the transport.
func (n *Node) Shutdown() {
	close(n.shutdownCh)
	n.raft.Shutdown()
	n.wg.Wait()
	n.raft.transport.Shutdown()
}

// Propose submits a command to the replicated log.
// Returns false if this node is not the leader.
func (n *Node) Propose(cmd []byte) bool {
	return n.raft.Propose(cmd)
}

// State returns the current Raft role.
func (n *Node) State() State {
	return n.raft.State()
}

// Term returns the current term.
func (n *Node) Term() uint64 {
	return n.raft.Term()
}

// LeaderID returns the known leader (0 if unknown).
func (n *Node) LeaderID() uint64 {
	return n.raft.LeaderID()
}

func (n *Node) runApply() {
	defer n.wg.Done()
	for {
		select {
		case msg := <-n.raft.ApplyCh():
			if n.onApply != nil {
				n.onApply(msg)
			}
		case <-n.shutdownCh:
			return
		}
	}
}
