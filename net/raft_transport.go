package net

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/atoncooper/mcache/raft"
)

// raftMsgType identifies the kind of Raft RPC.
type raftMsgType byte

const (
	raftMsgAE  raftMsgType = 1 // AppendEntries
	raftMsgAER raftMsgType = 2 // AppendEntriesResponse
	raftMsgRV  raftMsgType = 3 // RequestVote
	raftMsgRVR raftMsgType = 4 // RequestVoteResponse
)

// raftRPCEnvelope is the wire format for Raft RPCs.
// Layout: [1:type][4:payloadLen][payload]
type raftRPCEnvelope struct {
	Type    raftMsgType `json:"-"`
	Payload []byte      `json:"-"`
}

func (e *raftRPCEnvelope) encode() []byte {
	buf := make([]byte, 5+len(e.Payload))
	buf[0] = byte(e.Type)
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(e.Payload)))
	copy(buf[5:], e.Payload)
	return buf
}

func decodeRaftRPCEnvelope(r io.Reader) (*raftRPCEnvelope, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	t := raftMsgType(header[0])
	payloadLen := binary.BigEndian.Uint32(header[1:5])
	if payloadLen > 16*1024*1024 {
		return nil, fmt.Errorf("raft payload too large: %d", payloadLen)
	}
	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}
	}
	return &raftRPCEnvelope{Type: t, Payload: payload}, nil
}

// TCPTransport implements raft.Transport over persistent TCP connections.
type TCPTransport struct {
	nodeID    uint64
	localAddr string
	peers     map[uint64]string // peerID -> "host:port"

	listener net.Listener
	conns    map[uint64]net.Conn
	connMu   sync.RWMutex

	rpcCh    chan raft.RPC
	shutdown chan struct{}
	wg       sync.WaitGroup
}

// NewTCPTransport creates a Raft TCP transport.
// peers maps peerID to "host:port".
func NewTCPTransport(nodeID uint64, bindAddr string, peers map[uint64]string) *TCPTransport {
	return &TCPTransport{
		nodeID:    nodeID,
		localAddr: bindAddr,
		peers:     peers,
		conns:     make(map[uint64]net.Conn),
		rpcCh:     make(chan raft.RPC, 256),
		shutdown:  make(chan struct{}),
	}
}

// Start listens for inbound connections and dials all peers.
func (t *TCPTransport) Start() error {
	ln, err := net.Listen("tcp", t.localAddr)
	if err != nil {
		return err
	}
	t.listener = ln

	t.wg.Add(1)
	go t.acceptLoop()

	// Dial all peers.
	for pid, addr := range t.peers {
		if pid == t.nodeID {
			continue
		}
		go t.connectToPeer(pid, addr)
	}
	return nil
}

// RPCCh delivers inbound RPCs.
func (t *TCPTransport) RPCCh() <-chan raft.RPC {
	return t.rpcCh
}

// SendAppendEntries sends an AppendEntries RPC and returns the response.
func (t *TCPTransport) SendAppendEntries(peerID uint64, req *raft.AppendEntriesRequest) (*raft.AppendEntriesResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	respPayload, err := t.sendRPC(peerID, raftMsgAE, payload)
	if err != nil {
		return nil, err
	}
	var resp raft.AppendEntriesResponse
	if err := json.Unmarshal(respPayload, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SendRequestVote sends a RequestVote RPC and returns the response.
func (t *TCPTransport) SendRequestVote(peerID uint64, req *raft.RequestVoteRequest) (*raft.RequestVoteResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	respPayload, err := t.sendRPC(peerID, raftMsgRV, payload)
	if err != nil {
		return nil, err
	}
	var resp raft.RequestVoteResponse
	if err := json.Unmarshal(respPayload, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// LocalAddr returns the local bind address.
func (t *TCPTransport) LocalAddr() string {
	return t.localAddr
}

// PeerAddr returns the address of the given peer.
func (t *TCPTransport) PeerAddr(peerID uint64) string {
	return t.peers[peerID]
}

// Shutdown closes the transport.
func (t *TCPTransport) Shutdown() error {
	close(t.shutdown)
	if t.listener != nil {
		t.listener.Close()
	}
	t.connMu.Lock()
	for _, c := range t.conns {
		c.Close()
	}
	t.conns = make(map[uint64]net.Conn)
	t.connMu.Unlock()
	t.wg.Wait()
	close(t.rpcCh)
	return nil
}

// -------------------------------------------------------------------
// Internal
// -------------------------------------------------------------------

func (t *TCPTransport) acceptLoop() {
	defer t.wg.Done()
	for {
		conn, err := t.listener.Accept()
		if err != nil {
			select {
			case <-t.shutdown:
				return
			default:
				continue
			}
		}
		t.wg.Add(1)
		go t.handleConn(conn)
	}
}

func (t *TCPTransport) handleConn(conn net.Conn) {
	defer t.wg.Done()
	defer conn.Close()
	for {
		if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
			return
		}
		env, err := decodeRaftRPCEnvelope(conn)
		if err != nil {
			return
		}
		t.dispatch(env, conn)
	}
}

func (t *TCPTransport) dispatch(env *raftRPCEnvelope, conn net.Conn) {
	switch env.Type {
	case raftMsgAE:
		var req raft.AppendEntriesRequest
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return
		}
		rpc := raft.RPC{
			AE: &req,
			Respond: func(resp any) {
				t.writeResponse(conn, raftMsgAER, resp)
			},
		}
		select {
		case t.rpcCh <- rpc:
		case <-t.shutdown:
		}

	case raftMsgRV:
		var req raft.RequestVoteRequest
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return
		}
		rpc := raft.RPC{
			RV: &req,
			Respond: func(resp any) {
				t.writeResponse(conn, raftMsgRVR, resp)
			},
		}
		select {
		case t.rpcCh <- rpc:
		case <-t.shutdown:
		}

	case raftMsgAER:
		var resp raft.AppendEntriesResponse
		if err := json.Unmarshal(env.Payload, &resp); err != nil {
			return
		}
		select {
		case t.rpcCh <- raft.RPC{From: 0, AER: &resp}:
		case <-t.shutdown:
		}

	case raftMsgRVR:
		var resp raft.RequestVoteResponse
		if err := json.Unmarshal(env.Payload, &resp); err != nil {
			return
		}
		select {
		case t.rpcCh <- raft.RPC{From: 0, RVR: &resp}:
		case <-t.shutdown:
		}
	}
}

func (t *TCPTransport) writeResponse(conn net.Conn, typ raftMsgType, resp any) {
	payload, err := json.Marshal(resp)
	if err != nil {
		return
	}
	env := &raftRPCEnvelope{Type: typ, Payload: payload}
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, _ = conn.Write(env.encode())
}

func (t *TCPTransport) connectToPeer(peerID uint64, addr string) {
	backoff := time.Second
	for {
		select {
		case <-t.shutdown:
			return
		default:
		}

		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff += time.Second
			}
			continue
		}

		t.connMu.Lock()
		t.conns[peerID] = conn
		t.connMu.Unlock()

		// Wait for connection to close, then retry.
		t.handleConn(conn)

		t.connMu.Lock()
		delete(t.conns, peerID)
		t.connMu.Unlock()

		backoff = time.Second
	}
}

func (t *TCPTransport) sendRPC(peerID uint64, typ raftMsgType, payload []byte) ([]byte, error) {
	if peerID == t.nodeID {
		return nil, fmt.Errorf("cannot send RPC to self")
	}

	// Use a temporary connection if persistent connection is not available.
	t.connMu.RLock()
	conn, ok := t.conns[peerID]
	t.connMu.RUnlock()

	if !ok {
		addr := t.peers[peerID]
		if addr == "" {
			return nil, fmt.Errorf("unknown peer %d", peerID)
		}
		var err error
		conn, err = net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
	}

	env := &raftRPCEnvelope{Type: typ, Payload: payload}
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, err
	}
	if _, err := conn.Write(env.encode()); err != nil {
		return nil, err
	}

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, err
	}
	respEnv, err := decodeRaftRPCEnvelope(conn)
	if err != nil {
		return nil, err
	}
	return respEnv.Payload, nil
}
