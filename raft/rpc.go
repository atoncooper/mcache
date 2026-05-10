package raft

// AppendEntriesRequest is sent by the leader to replicate log entries.
type AppendEntriesRequest struct {
	Term         uint64     `json:"term"`
	LeaderID     uint64     `json:"leader_id"`
	PrevLogIndex uint64     `json:"prev_log_index"`
	PrevLogTerm  uint64     `json:"prev_log_term"`
	Entries      []LogEntry `json:"entries"`
	LeaderCommit uint64     `json:"leader_commit"`
}

// AppendEntriesResponse is the reply to AppendEntries.
type AppendEntriesResponse struct {
	Term      uint64 `json:"term"`
	Success   bool   `json:"success"`
	LastIndex uint64 `json:"last_index"` // follower's last log index after processing
}

// RequestVoteRequest is sent by candidates to gather votes.
type RequestVoteRequest struct {
	Term         uint64 `json:"term"`
	CandidateID  uint64 `json:"candidate_id"`
	LastLogIndex uint64 `json:"last_log_index"`
	LastLogTerm  uint64 `json:"last_log_term"`
}

// RequestVoteResponse is the reply to RequestVote.
type RequestVoteResponse struct {
	Term        uint64 `json:"term"`
	VoteGranted bool   `json:"vote_granted"`
}

// RPC is a union type for all Raft RPC messages.
// Transport implementations should set Respond so the state machine can reply.
type RPC struct {
	From    uint64
	AE      *AppendEntriesRequest
	AER     *AppendEntriesResponse
	RV      *RequestVoteRequest
	RVR     *RequestVoteResponse
	Respond func(resp any)
}
