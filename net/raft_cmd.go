package net

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/atoncooper/mcache/raft"
)

// RaftCommand represents a cache operation serialized for Raft replication.
type RaftCommand struct {
	ReqID    uint64   `json:"req_id"`
	Op       byte     `json:"op"`
	Key      string   `json:"key,omitempty"`
	Value    []byte   `json:"value,omitempty"`
	TTL      int64    `json:"ttl,omitempty"`
	Field    string   `json:"field,omitempty"`
	Fields   []string `json:"fields,omitempty"`
	FvPairs  []string `json:"fv_pairs,omitempty"`
	DeltaI64 int64    `json:"delta_i64,omitempty"`
	DeltaF64 float64  `json:"delta_f64,omitempty"`
	Elems    []string `json:"elems,omitempty"`
	Index    int64    `json:"index,omitempty"`
	Start    int64    `json:"start,omitempty"`
	Stop     int64    `json:"stop,omitempty"`
	Count    int64    `json:"count,omitempty"`
	Pivot    string   `json:"pivot,omitempty"`
	Before   bool     `json:"before,omitempty"`
}

// raftResult carries the outcome of a Raft-applied command back to the proposer.
type raftResult struct {
	err         error
	intResult   int64
	boolResult  bool
	strResult   string
	floatResult float64
	sliceResult []string
}

// isRaftWriteOp returns true for commands that mutate state.
func isRaftWriteOp(cmd byte) bool {
	switch cmd {
	case CmdSet, CmdDel, CmdCleanup,
		CmdSAdd, CmdSRem, CmdSPop,
		CmdHSet, CmdHSetNX, CmdHDel, CmdHMSet, CmdHIncrBy, CmdHIncrByFloat,
		CmdLPush, CmdRPush, CmdLPop, CmdRPop, CmdLSet, CmdLRem, CmdLTrim, CmdLInsert:
		return true
	default:
		return false
	}
}

// raftPropose serialises a command and submits it to the Raft log.
// It blocks until the command is applied or times out.
func (s *Server) raftPropose(rc RaftCommand) (raftResult, error) {
	if s.raftNode == nil {
		return raftResult{}, fmt.Errorf("raft not initialized")
	}
	if s.raftNode.State() != raft.Leader {
		return raftResult{}, fmt.Errorf("not leader")
	}

	id := s.nextRaftReqID()
	rc.ReqID = id

	data, err := json.Marshal(rc)
	if err != nil {
		return raftResult{}, err
	}

	ch := make(chan raftResult, 1)
	s.raftPendingMu.Lock()
	s.raftPending[id] = ch
	s.raftPendingMu.Unlock()

	if !s.raftNode.Propose(data) {
		s.raftPendingMu.Lock()
		delete(s.raftPending, id)
		s.raftPendingMu.Unlock()
		return raftResult{}, fmt.Errorf("raft propose failed")
	}

	select {
	case res := <-ch:
		return res, nil
	case <-time.After(10 * time.Second):
		s.raftPendingMu.Lock()
		delete(s.raftPending, id)
		s.raftPendingMu.Unlock()
		return raftResult{}, fmt.Errorf("raft apply timeout")
	}
}

func (s *Server) nextRaftReqID() uint64 {
	s.raftReqIDMu.Lock()
	defer s.raftReqIDMu.Unlock()
	s.raftNextReqID++
	return s.raftNextReqID
}

// onRaftApply is the callback invoked by the Raft node for every committed entry.
func (s *Server) onRaftApply(msg raft.ApplyMsg) {
	var rc RaftCommand
	if err := json.Unmarshal(msg.Command, &rc); err != nil {
		s.notifyRaftPending(0, raftResult{err: err})
		return
	}

	var res raftResult
	switch rc.Op {
	case CmdSet:
		var ttl time.Duration
		if rc.TTL > 0 {
			ttl = time.Duration(rc.TTL) * time.Millisecond
		}
		res.err = s.cache.Set(rc.Key, rc.Value, ttl)

	case CmdDel:
		res.err = s.cache.Del(rc.Key)

	case CmdCleanup:
		_ = s.cache.Cleanup()

	// Hash writes
	case CmdHSet:
		n, err := s.cache.HSet(rc.Key, rc.Field, string(rc.Value))
		res.intResult = int64(n)
		res.err = err

	case CmdHSetNX:
		ok, err := s.cache.HSetNX(rc.Key, rc.Field, string(rc.Value))
		res.boolResult = ok
		res.err = err

	case CmdHDel:
		n, err := s.cache.HDel(rc.Key, rc.Fields...)
		res.intResult = int64(n)
		res.err = err

	case CmdHIncrBy:
		n, err := s.cache.HIncrBy(rc.Key, rc.Field, rc.DeltaI64)
		res.intResult = n
		res.err = err

	case CmdHIncrByFloat:
		n, err := s.cache.HIncrByFloat(rc.Key, rc.Field, rc.DeltaF64)
		res.floatResult = n
		res.err = err

	case CmdHMSet:
		res.err = s.cache.HMSet(rc.Key, rc.FvPairs...)

	// List writes
	case CmdLPush:
		n, err := s.cache.LPush(rc.Key, rc.Elems...)
		res.intResult = int64(n)
		res.err = err

	case CmdRPush:
		n, err := s.cache.RPush(rc.Key, rc.Elems...)
		res.intResult = int64(n)
		res.err = err

	case CmdLPop:
		elem, err := s.cache.LPop(rc.Key)
		res.strResult = elem
		res.err = err

	case CmdRPop:
		elem, err := s.cache.RPop(rc.Key)
		res.strResult = elem
		res.err = err

	case CmdLSet:
		res.err = s.cache.LSet(rc.Key, int(rc.Index), string(rc.Value))

	case CmdLRem:
		n, err := s.cache.LRem(rc.Key, int(rc.Count), string(rc.Value))
		res.intResult = int64(n)
		res.err = err

	case CmdLTrim:
		res.err = s.cache.LTrim(rc.Key, int(rc.Start), int(rc.Stop))

	case CmdLInsert:
		n, err := s.cache.LInsert(rc.Key, rc.Before, rc.Pivot, string(rc.Value))
		res.intResult = int64(n)
		res.err = err

	// Set writes
	case CmdSAdd:
		n, err := s.cache.SAdd(rc.Key, rc.Elems...)
		res.intResult = int64(n)
		res.err = err

	case CmdSRem:
		n, err := s.cache.SRem(rc.Key, rc.Elems...)
		res.intResult = int64(n)
		res.err = err

	case CmdSPop:
		elem, err := s.cache.SPop(rc.Key)
		res.strResult = elem
		res.err = err

	default:
		res.err = fmt.Errorf("unsupported raft op: %d", rc.Op)
	}

	if rc.ReqID > 0 {
		s.notifyRaftPending(rc.ReqID, res)
	}
}

func (s *Server) notifyRaftPending(reqID uint64, res raftResult) {
	if reqID == 0 {
		return
	}
	s.raftPendingMu.Lock()
	ch, ok := s.raftPending[reqID]
	if ok {
		delete(s.raftPending, reqID)
	}
	s.raftPendingMu.Unlock()
	if ok {
		ch <- res
		close(ch)
	}
}
