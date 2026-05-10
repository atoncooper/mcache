package raft

// Transport abstracts the network layer for Raft RPC.
// Implementations may use TCP, gRPC, HTTP, or the project's own net/ package.
type Transport interface {
	// SendAppendEntries sends an AppendEntries RPC to the given peer.
	SendAppendEntries(peerID uint64, req *AppendEntriesRequest) (*AppendEntriesResponse, error)

	// SendRequestVote sends a RequestVote RPC to the given peer.
	SendRequestVote(peerID uint64, req *RequestVoteRequest) (*RequestVoteResponse, error)

	// LocalAddr returns the local node address.
	LocalAddr() string

	// PeerAddr returns the address of the given peer.
	PeerAddr(peerID uint64) string

	// Shutdown closes the transport.
	Shutdown() error
}
