package raft

import (
	"fmt"
	"sync"
)

// MemoryTransport is an in-memory Transport for testing Raft clusters.
// All nodes must be registered before Start() is called.
type MemoryTransport struct {
	mu     sync.Mutex
	nodeID uint64
	peers  map[uint64]*MemoryTransport
	recvCh chan<- RPC
	closed bool
}

// NewMemoryTransport creates a transport for the given node.
func NewMemoryTransport(nodeID uint64) *MemoryTransport {
	return &MemoryTransport{
		nodeID: nodeID,
		peers:  make(map[uint64]*MemoryTransport),
	}
}

// RegisterPeer links another in-memory transport so RPCs can be delivered.
func (t *MemoryTransport) RegisterPeer(id uint64, peer *MemoryTransport) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.peers[id] = peer
}

// SetReceiveCh sets the channel where inbound RPCs are delivered.
func (t *MemoryTransport) SetReceiveCh(ch chan<- RPC) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.recvCh = ch
}

// SendAppendEntries delivers the request to the peer's RPCCh.
func (t *MemoryTransport) SendAppendEntries(peerID uint64, req *AppendEntriesRequest) (*AppendEntriesResponse, error) {
	respCh := make(chan any, 1)
	rpc := RPC{
		From:    t.nodeID,
		AE:      req,
		Respond: func(resp any) { respCh <- resp },
	}
	if err := t.deliver(peerID, rpc); err != nil {
		return nil, err
	}
	r := <-respCh
	return r.(*AppendEntriesResponse), nil
}

// SendRequestVote delivers the request to the peer's RPCCh.
func (t *MemoryTransport) SendRequestVote(peerID uint64, req *RequestVoteRequest) (*RequestVoteResponse, error) {
	respCh := make(chan any, 1)
	rpc := RPC{
		From:    t.nodeID,
		RV:      req,
		Respond: func(resp any) { respCh <- resp },
	}
	if err := t.deliver(peerID, rpc); err != nil {
		return nil, err
	}
	r := <-respCh
	return r.(*RequestVoteResponse), nil
}

// LocalAddr returns a pseudo-address for this node.
func (t *MemoryTransport) LocalAddr() string {
	return fmt.Sprintf("mem://%d", t.nodeID)
}

// PeerAddr returns a pseudo-address for the peer.
func (t *MemoryTransport) PeerAddr(peerID uint64) string {
	return fmt.Sprintf("mem://%d", peerID)
}

// Shutdown closes the transport.
func (t *MemoryTransport) Shutdown() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

func (t *MemoryTransport) deliver(peerID uint64, rpc RPC) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return fmt.Errorf("transport closed")
	}
	peer, ok := t.peers[peerID]
	if !ok {
		return fmt.Errorf("unknown peer %d", peerID)
	}
	if peer.recvCh == nil {
		return fmt.Errorf("peer %d recvCh not set", peerID)
	}
	select {
	case peer.recvCh <- rpc:
		return nil
	default:
		return fmt.Errorf("peer %d recvCh full", peerID)
	}
}
