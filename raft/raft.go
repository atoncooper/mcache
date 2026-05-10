package raft

import (
	"math/rand"
	"sync"
	"time"
)

// State represents the role of a Raft node.
type State int

const (
	Follower State = iota
	Candidate
	Leader
)

func (s State) String() string {
	switch s {
	case Follower:
		return "Follower"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	default:
		return "Unknown"
	}
}

// ApplyMsg is delivered to the application when a log entry is committed.
type ApplyMsg struct {
	Index   uint64
	Command []byte
}

// Raft is the core consensus state machine.
// It is NOT safe for concurrent use except via Propose() and the RPC channels.
type Raft struct {
	config    Config
	transport Transport

	// persistent state (must survive crashes — simplified here as in-memory)
	currentTerm uint64
	votedFor    uint64
	log         *Log

	// volatile state
	commitIndex uint64
	lastApplied uint64
	state       State
	leaderID    uint64

	// volatile state on leaders
	nextIndex  map[uint64]uint64
	matchIndex map[uint64]uint64

	// channels (all events serialised through run() goroutine)
	rpcCh      chan RPC
	proposeCh  chan []byte
	applyCh    chan ApplyMsg
	shutdownCh chan struct{}

	// timers
	electionTimer  *time.Timer
	heartbeatTimer *time.Timer

	// votes received during an election
	votesReceived int
	votesNeeded   int

	mu sync.Mutex // protects external-visible state snapshots only
}

// NewRaft creates a Raft node. Call Start() to begin the event loop.
func NewRaft(cfg Config, trans Transport) *Raft {
	r := &Raft{
		config:         cfg,
		transport:      trans,
		log:            NewLog(),
		state:          Follower,
		rpcCh:          make(chan RPC, 128),
		proposeCh:      make(chan []byte, 64),
		applyCh:        make(chan ApplyMsg, 64),
		shutdownCh:     make(chan struct{}),
		electionTimer:  time.NewTimer(randomTimeout(cfg.ElectionTimeout)),
		heartbeatTimer: time.NewTimer(cfg.HeartbeatInterval),
	}
	// stop heartbeat timer until we become leader
	if !r.heartbeatTimer.Stop() {
		<-r.heartbeatTimer.C
	}
	return r
}

// Start launches the event-loop goroutine.
func (r *Raft) Start() {
	go r.run()
}

// Shutdown stops the event loop.
func (r *Raft) Shutdown() {
	close(r.shutdownCh)
}

// Propose submits a command to the replicated log (non-blocking).
// If this node is not the leader the command is silently dropped (caller should retry).
func (r *Raft) Propose(cmd []byte) bool {
	r.mu.Lock()
	st := r.state
	r.mu.Unlock()
	if st != Leader {
		return false
	}
	select {
	case r.proposeCh <- cmd:
		return true
	default:
		return false
	}
}

// ApplyCh returns the channel where committed entries are delivered.
func (r *Raft) ApplyCh() <-chan ApplyMsg {
	return r.applyCh
}

// State returns the current role.
func (r *Raft) State() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

// Term returns the current term.
func (r *Raft) Term() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.currentTerm
}

// LeaderID returns the known leader (0 if unknown).
func (r *Raft) LeaderID() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.leaderID
}

// RPCCh is used by the transport layer to deliver inbound RPCs.
func (r *Raft) RPCCh() chan<- RPC {
	return r.rpcCh
}

// -------------------------------------------------------------------
// Event loop
// -------------------------------------------------------------------

func (r *Raft) run() {
	for {
		select {
		case rpc := <-r.rpcCh:
			r.handleRPC(rpc)

		case cmd := <-r.proposeCh:
			r.handlePropose(cmd)

		case <-r.electionTimer.C:
			r.handleElectionTimeout()

		case <-r.heartbeatTimer.C:
			r.handleHeartbeatTimeout()

		case <-r.shutdownCh:
			return
		}
	}
}

// -------------------------------------------------------------------
// State transitions
// -------------------------------------------------------------------

func (r *Raft) becomeFollower(term uint64) {
	if term > r.currentTerm {
		r.currentTerm = term
		r.votedFor = 0
	}
	r.state = Follower
	r.leaderID = 0
	r.resetElectionTimer()
	// stop heartbeat timer
	if !r.heartbeatTimer.Stop() {
		select {
		case <-r.heartbeatTimer.C:
		default:
		}
	}
}

func (r *Raft) becomeCandidate() {
	r.state = Candidate
	r.currentTerm++
	r.votedFor = r.config.NodeID
	r.votesReceived = 1
	r.votesNeeded = len(r.config.Peers)/2 + 1
	r.resetElectionTimer()
}

func (r *Raft) becomeLeader() {
	r.state = Leader
	r.leaderID = r.config.NodeID

	lastIdx, _ := r.log.Last()
	r.nextIndex = make(map[uint64]uint64, len(r.config.Peers))
	r.matchIndex = make(map[uint64]uint64, len(r.config.Peers))
	for i := range r.config.Peers {
		pid := uint64(i) + 1
		if pid == r.config.NodeID {
			continue
		}
		r.nextIndex[pid] = lastIdx + 1
		r.matchIndex[pid] = 0
	}

	// stop election timer, start heartbeat
	if !r.electionTimer.Stop() {
		select {
		case <-r.electionTimer.C:
		default:
		}
	}
	r.heartbeatTimer.Reset(0) // immediate heartbeat
}

// -------------------------------------------------------------------
// RPC handlers
// -------------------------------------------------------------------

func (r *Raft) handleRPC(rpc RPC) {
	switch {
	case rpc.AE != nil:
		r.handleAppendEntries(rpc.AE, rpc.Respond)
	case rpc.AER != nil:
		r.handleAppendEntriesResponse(rpc.From, rpc.AER)
	case rpc.RV != nil:
		r.handleRequestVote(rpc.RV, rpc.Respond)
	case rpc.RVR != nil:
		r.handleRequestVoteResponse(rpc.RVR)
	}
}

func (r *Raft) handleAppendEntries(req *AppendEntriesRequest, respond func(any)) {
	success := false
	defer func() {
		if respond != nil {
			resp := &AppendEntriesResponse{Term: r.currentTerm, Success: success}
			if success {
				resp.LastIndex = req.PrevLogIndex + uint64(len(req.Entries))
			}
			respond(resp)
		}
	}()

	if req.Term < r.currentTerm {
		return
	}

	if req.Term > r.currentTerm {
		r.becomeFollower(req.Term)
	}

	// reset election timer on valid communication from leader
	r.resetElectionTimer()
	r.leaderID = req.LeaderID
	r.state = Follower

	// log consistency check
	if req.PrevLogIndex > 0 {
		entry, ok := r.log.Get(req.PrevLogIndex)
		if !ok || entry.Term != req.PrevLogTerm {
			return
		}
	}

	// append new entries
	if len(req.Entries) > 0 {
		// truncate any conflicting entries after prevLogIndex
		r.log.Truncate(req.PrevLogIndex + 1)
		r.log.Append(req.Entries...)
	}

	// update commit index
	if req.LeaderCommit > r.commitIndex {
		lastIdx, _ := r.log.Last()
		r.commitIndex = min(req.LeaderCommit, lastIdx)
		r.applyCommitted()
	}
	success = true
}

func (r *Raft) handleAppendEntriesResponse(from uint64, resp *AppendEntriesResponse) {
	if resp.Term > r.currentTerm {
		r.becomeFollower(resp.Term)
		return
	}
	if resp.Term < r.currentTerm || r.state != Leader {
		return
	}

	if resp.Success {
		if resp.LastIndex > r.matchIndex[from] {
			r.matchIndex[from] = resp.LastIndex
			r.nextIndex[from] = r.matchIndex[from] + 1
		}
		r.advanceCommitIndex()
	} else {
		if r.nextIndex[from] > 1 {
			r.nextIndex[from]--
		}
		r.sendAppendEntriesTo(from)
	}
}

func (r *Raft) handleRequestVote(req *RequestVoteRequest, respond func(any)) {
	voteGranted := false
	defer func() {
		if respond != nil {
			respond(&RequestVoteResponse{Term: r.currentTerm, VoteGranted: voteGranted})
		}
	}()

	if req.Term < r.currentTerm {
		return
	}

	if req.Term > r.currentTerm {
		r.becomeFollower(req.Term)
	}

	lastIdx, lastTerm := r.log.Last()
	logOK := req.LastLogTerm > lastTerm ||
		(req.LastLogTerm == lastTerm && req.LastLogIndex >= lastIdx)

	if (r.votedFor == 0 || r.votedFor == req.CandidateID) && logOK {
		r.votedFor = req.CandidateID
		r.resetElectionTimer()
		voteGranted = true
	}
}

func (r *Raft) handleRequestVoteResponse(resp *RequestVoteResponse) {
	if resp.Term > r.currentTerm {
		r.becomeFollower(resp.Term)
		return
	}
	if resp.Term < r.currentTerm || r.state != Candidate {
		return
	}
	if resp.VoteGranted {
		r.votesReceived++
		if r.votesReceived >= r.votesNeeded {
			r.becomeLeader()
		}
	}
}

// -------------------------------------------------------------------
// Election
// -------------------------------------------------------------------

func (r *Raft) handleElectionTimeout() {
	if r.state == Leader {
		return
	}
	r.becomeCandidate()
	r.broadcastRequestVote()
}

func (r *Raft) broadcastRequestVote() {
	lastIdx, lastTerm := r.log.Last()
	req := &RequestVoteRequest{
		Term:         r.currentTerm,
		CandidateID:  r.config.NodeID,
		LastLogIndex: lastIdx,
		LastLogTerm:  lastTerm,
	}
	for i := range r.config.Peers {
		pid := uint64(i) + 1
		if pid == r.config.NodeID {
			continue
		}
		go func(peer uint64) {
			resp, err := r.transport.SendRequestVote(peer, req)
			if err != nil {
				return
			}
			r.rpcCh <- RPC{From: peer, RVR: resp}
		}(pid)
	}
}

// -------------------------------------------------------------------
// Log replication (leader)
// -------------------------------------------------------------------

func (r *Raft) handleHeartbeatTimeout() {
	if r.state != Leader {
		return
	}
	r.broadcastAppendEntries()
	r.heartbeatTimer.Reset(r.config.HeartbeatInterval)
}

func (r *Raft) handlePropose(cmd []byte) {
	if r.state != Leader {
		return
	}
	lastIdx, _ := r.log.Last()
	entry := LogEntry{
		Index:   lastIdx + 1,
		Term:    r.currentTerm,
		Command: cmd,
	}
	r.log.Append(entry)
	r.broadcastAppendEntries()
	r.advanceCommitIndex()
}

func (r *Raft) broadcastAppendEntries() {
	for i := range r.config.Peers {
		pid := uint64(i) + 1
		if pid == r.config.NodeID {
			continue
		}
		r.sendAppendEntriesTo(pid)
	}
}

func (r *Raft) sendAppendEntriesTo(peer uint64) {
	next, ok := r.nextIndex[peer]
	if !ok {
		return
	}
	prevIdx := next - 1
	prevTerm := uint64(0)
	if prevIdx > 0 {
		if ent, ok := r.log.Get(prevIdx); ok {
			prevTerm = ent.Term
		}
	}
	entries := r.log.Slice(next, next+100) // batch up to 100 entries

	req := &AppendEntriesRequest{
		Term:         r.currentTerm,
		LeaderID:     r.config.NodeID,
		PrevLogIndex: prevIdx,
		PrevLogTerm:  prevTerm,
		Entries:      entries,
		LeaderCommit: r.commitIndex,
	}
	go func(p uint64) {
		resp, err := r.transport.SendAppendEntries(p, req)
		if err != nil {
			return
		}
		r.rpcCh <- RPC{From: p, AER: resp}
	}(peer)
}

// -------------------------------------------------------------------
// Commit / apply
// -------------------------------------------------------------------

func (r *Raft) advanceCommitIndex() {
	if r.state != Leader {
		return
	}
	for n := r.commitIndex + 1; ; n++ {
		if _, ok := r.log.Get(n); !ok {
			break
		}
		ent, _ := r.log.Get(n)
		if ent.Term != r.currentTerm {
			continue
		}
		count := 1 // leader itself
		for _, idx := range r.matchIndex {
			if idx >= n {
				count++
			}
		}
		if count > len(r.config.Peers)/2 {
			r.commitIndex = n
		} else {
			break
		}
	}
	r.applyCommitted()
}

func (r *Raft) applyCommitted() {
	for r.lastApplied < r.commitIndex {
		r.lastApplied++
		if ent, ok := r.log.Get(r.lastApplied); ok {
			r.applyCh <- ApplyMsg{Index: ent.Index, Command: ent.Command}
		}
	}
}

// -------------------------------------------------------------------
// Utilities
// -------------------------------------------------------------------

func (r *Raft) resetElectionTimer() {
	if !r.electionTimer.Stop() {
		select {
		case <-r.electionTimer.C:
		default:
		}
	}
	r.electionTimer.Reset(randomTimeout(r.config.ElectionTimeout))
}

func randomTimeout(base time.Duration) time.Duration {
	// random between base and 2*base to avoid split votes
	n := rand.Int63n(int64(base))
	return base + time.Duration(n)
}
