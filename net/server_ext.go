package net

import (
	"time"

	"github.com/atoncooper/mcache"
	"github.com/atoncooper/mcache/raft"
)

func (s *Server) processHash(req *HashRequest) []byte {
	if s.raftNode != nil && isRaftWriteOp(req.Cmd) {
		return s.processHashRaft(req)
	}
	switch req.Cmd {
	case CmdHSet:
		added, err := s.cache.HSet(req.Key, req.Field, req.Value)
		if err != nil {
			return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusOK, IntResult: int64(added)})

	case CmdHSetNX:
		ok, err := s.cache.HSetNX(req.Key, req.Field, req.Value)
		if err != nil {
			return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusOK, BoolResult: ok})

	case CmdHGet:
		val, err := s.cache.HGet(req.Key, req.Field)
		if err != nil {
			if err == mcache.ErrKeyNotFound {
				return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusNotFound})
			}
			return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusOK, StrResult: val})

	case CmdHDel:
		n, err := s.cache.HDel(req.Key, req.Fields...)
		if err != nil {
			return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusOK, IntResult: int64(n)})

	case CmdHExists:
		ok, err := s.cache.HExists(req.Key, req.Field)
		if err != nil {
			return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusOK, BoolResult: ok})

	case CmdHGetAll:
		all, err := s.cache.HGetAll(req.Key)
		if err != nil {
			if err == mcache.ErrKeyNotFound {
				return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusNotFound})
			}
			return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusOK, MapResult: all})

	case CmdHKeys:
		keys, err := s.cache.HKeys(req.Key)
		if err != nil {
			if err == mcache.ErrKeyNotFound {
				return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusNotFound})
			}
			return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusOK, SliceResult: keys})

	case CmdHVals:
		vals, err := s.cache.HVals(req.Key)
		if err != nil {
			if err == mcache.ErrKeyNotFound {
				return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusNotFound})
			}
			return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusOK, SliceResult: vals})

	case CmdHLen:
		n, err := s.cache.HLen(req.Key)
		if err != nil {
			return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusOK, IntResult: int64(n)})

	case CmdHStrLen:
		n, err := s.cache.HStrLen(req.Key, req.Field)
		if err != nil {
			return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusOK, IntResult: int64(n)})

	case CmdHIncrBy:
		n, err := s.cache.HIncrBy(req.Key, req.Field, req.DeltaI64)
		if err != nil {
			return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusOK, IntResult: n})

	case CmdHIncrByFloat:
		n, err := s.cache.HIncrByFloat(req.Key, req.Field, req.DeltaF64)
		if err != nil {
			return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusOK, FloatResult: n})

	case CmdHMGet:
		vals, err := s.cache.HMGet(req.Key, req.Fields...)
		if err != nil {
			return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusOK, AnySlice: vals})

	case CmdHMSet:
		if err := s.cache.HMSet(req.Key, req.FvPairs...); err != nil {
			return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusOK})

	default:
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: ErrInvalidCommand.Error()})
	}
}

func (s *Server) processList(req *ListRequest) []byte {
	if s.raftNode != nil && isRaftWriteOp(req.Cmd) {
		return s.processListRaft(req)
	}
	switch req.Cmd {
	case CmdLPush:
		n, err := s.cache.LPush(req.Key, req.Elements...)
		if err != nil {
			return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK, IntResult: int64(n)})

	case CmdRPush:
		n, err := s.cache.RPush(req.Key, req.Elements...)
		if err != nil {
			return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK, IntResult: int64(n)})

	case CmdLPop:
		elem, err := s.cache.LPop(req.Key)
		if err != nil {
			if err == mcache.ErrKeyNotFound {
				return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusNotFound})
			}
			return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK, StrResult: elem})

	case CmdRPop:
		elem, err := s.cache.RPop(req.Key)
		if err != nil {
			if err == mcache.ErrKeyNotFound {
				return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusNotFound})
			}
			return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK, StrResult: elem})

	case CmdLLen:
		n, err := s.cache.LLen(req.Key)
		if err != nil {
			return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK, IntResult: int64(n)})

	case CmdLRange:
		elems, err := s.cache.LRange(req.Key, int(req.Start), int(req.Stop))
		if err != nil {
			if err == mcache.ErrKeyNotFound {
				return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusNotFound})
			}
			return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK, SliceResult: elems})

	case CmdLIndex:
		elem, err := s.cache.LIndex(req.Key, int(req.Index))
		if err != nil {
			if err == mcache.ErrKeyNotFound {
				return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusNotFound})
			}
			return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK, StrResult: elem})

	case CmdLSet:
		err := s.cache.LSet(req.Key, int(req.Index), req.Value)
		if err != nil {
			if err == mcache.ErrKeyNotFound {
				return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusNotFound})
			}
			return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK, BoolResult: true})

	case CmdLRem:
		n, err := s.cache.LRem(req.Key, int(req.Count), req.Value)
		if err != nil {
			return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK, IntResult: int64(n)})

	case CmdLTrim:
		err := s.cache.LTrim(req.Key, int(req.Start), int(req.Stop))
		if err != nil {
			return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK})

	case CmdLInsert:
		n, err := s.cache.LInsert(req.Key, req.Before, req.Pivot, req.Value)
		if err != nil {
			return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK, IntResult: int64(n)})

	case CmdBLPop:
		if req.Timeout > 0 {
			timeout := time.Duration(req.Timeout) * time.Millisecond
			deadline := time.Now().Add(timeout)
			for {
				elem, err := s.cache.LPop(req.Key)
				if err == nil {
					return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK, StrResult: elem})
				}
				if time.Now().After(deadline) {
					return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusNotFound})
				}
				time.Sleep(time.Millisecond)
			}
		}
		elem, err := s.cache.LPop(req.Key)
		if err != nil {
			if err == mcache.ErrKeyNotFound {
				return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusNotFound})
			}
			return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK, StrResult: elem})

	case CmdBRPop:
		if req.Timeout > 0 {
			timeout := time.Duration(req.Timeout) * time.Millisecond
			deadline := time.Now().Add(timeout)
			for {
				elem, err := s.cache.RPop(req.Key)
				if err == nil {
					return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK, StrResult: elem})
				}
				if time.Now().After(deadline) {
					return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusNotFound})
				}
				time.Sleep(time.Millisecond)
			}
		}
		elem, err := s.cache.RPop(req.Key)
		if err != nil {
			if err == mcache.ErrKeyNotFound {
				return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusNotFound})
			}
			return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK, StrResult: elem})

	case CmdLPos:
		positions, err := s.cache.LPos(req.Key, req.Value, int(req.Rank), int(req.Count), int(req.MaxLen))
		if err != nil {
			return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		posResult := make([]int64, len(positions))
		for i, p := range positions {
			posResult[i] = int64(p)
		}
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusOK, PosResult: posResult})

	default:
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: ErrInvalidCommand.Error()})
	}
}

// -------------------------------------------------------------------
// Raft helpers for Hash / List / Set writes
// -------------------------------------------------------------------

func (s *Server) processHashRaft(req *HashRequest) []byte {
	if s.raftNode.State() != raft.Leader {
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: "not leader"})
	}
	rc := hashReqToRaft(req)
	res, err := s.raftPropose(rc)
	if err != nil {
		return EncodeHashResponse(&HashResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
	}
	return raftResultToHashResponseBytes(req.Cmd, res)
}

func hashReqToRaft(req *HashRequest) RaftCommand {
	rc := RaftCommand{Op: req.Cmd, Key: req.Key, Field: req.Field}
	switch req.Cmd {
	case CmdHSet, CmdHSetNX:
		rc.Value = []byte(req.Value)
	case CmdHDel:
		rc.Fields = req.Fields
	case CmdHIncrBy:
		rc.DeltaI64 = req.DeltaI64
	case CmdHIncrByFloat:
		rc.DeltaF64 = req.DeltaF64
	case CmdHMSet:
		rc.FvPairs = req.FvPairs
	}
	return rc
}

func raftResultToHashResponseBytes(cmd byte, res raftResult) []byte {
	if res.err != nil {
		return EncodeHashResponse(&HashResponse{Cmd: cmd, Status: StatusErr, ErrMsg: res.err.Error()})
	}
	switch cmd {
	case CmdHSet, CmdHDel, CmdHIncrBy:
		return EncodeHashResponse(&HashResponse{Cmd: cmd, Status: StatusOK, IntResult: res.intResult})
	case CmdHSetNX:
		return EncodeHashResponse(&HashResponse{Cmd: cmd, Status: StatusOK, BoolResult: res.boolResult})
	case CmdHIncrByFloat:
		return EncodeHashResponse(&HashResponse{Cmd: cmd, Status: StatusOK, FloatResult: res.floatResult})
	case CmdHMSet:
		return EncodeHashResponse(&HashResponse{Cmd: cmd, Status: StatusOK})
	default:
		return EncodeHashResponse(&HashResponse{Cmd: cmd, Status: StatusErr, ErrMsg: "unsupported raft hash op"})
	}
}

func (s *Server) processListRaft(req *ListRequest) []byte {
	if s.raftNode.State() != raft.Leader {
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: "not leader"})
	}
	rc := listReqToRaft(req)
	res, err := s.raftPropose(rc)
	if err != nil {
		return EncodeListResponse(&ListResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
	}
	return raftResultToListResponseBytes(req.Cmd, res)
}

func listReqToRaft(req *ListRequest) RaftCommand {
	rc := RaftCommand{Op: req.Cmd, Key: req.Key}
	switch req.Cmd {
	case CmdLPush, CmdRPush:
		rc.Elems = req.Elements
	case CmdLSet:
		rc.Index = req.Index
		rc.Value = []byte(req.Value)
	case CmdLRem:
		rc.Count = req.Count
		rc.Value = []byte(req.Value)
	case CmdLTrim:
		rc.Start = req.Start
		rc.Stop = req.Stop
	case CmdLInsert:
		rc.Before = req.Before
		rc.Pivot = req.Pivot
		rc.Value = []byte(req.Value)
	}
	return rc
}

func raftResultToListResponseBytes(cmd byte, res raftResult) []byte {
	if res.err != nil {
		if res.err == mcache.ErrKeyNotFound {
			return EncodeListResponse(&ListResponse{Cmd: cmd, Status: StatusNotFound})
		}
		return EncodeListResponse(&ListResponse{Cmd: cmd, Status: StatusErr, ErrMsg: res.err.Error()})
	}
	switch cmd {
	case CmdLPush, CmdRPush, CmdLRem, CmdLInsert:
		return EncodeListResponse(&ListResponse{Cmd: cmd, Status: StatusOK, IntResult: res.intResult})
	case CmdLPop, CmdRPop:
		return EncodeListResponse(&ListResponse{Cmd: cmd, Status: StatusOK, StrResult: res.strResult})
	case CmdLSet:
		return EncodeListResponse(&ListResponse{Cmd: cmd, Status: StatusOK, BoolResult: true})
	case CmdLTrim:
		return EncodeListResponse(&ListResponse{Cmd: cmd, Status: StatusOK})
	default:
		return EncodeListResponse(&ListResponse{Cmd: cmd, Status: StatusErr, ErrMsg: "unsupported raft list op"})
	}
}

func (s *Server) processSetRaft(req *SetRequest) []byte {
	if s.raftNode.State() != raft.Leader {
		return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: "not leader"})
	}
	rc := setReqToRaft(req)
	res, err := s.raftPropose(rc)
	if err != nil {
		return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
	}
	return raftResultToSetResponseBytes(req.Cmd, res)
}

func setReqToRaft(req *SetRequest) RaftCommand {
	rc := RaftCommand{Op: req.Cmd, Key: req.Key}
	switch req.Cmd {
	case CmdSAdd, CmdSRem:
		rc.Elems = req.Elems
	}
	return rc
}

func raftResultToSetResponseBytes(cmd byte, res raftResult) []byte {
	if res.err != nil {
		if res.err == mcache.ErrKeyNotFound && cmd == CmdSPop {
			return EncodeSetResponse(&SetResponse{Cmd: cmd, Status: StatusNotFound})
		}
		return EncodeSetResponse(&SetResponse{Cmd: cmd, Status: StatusErr, ErrMsg: res.err.Error()})
	}
	switch cmd {
	case CmdSAdd, CmdSRem:
		return EncodeSetResponse(&SetResponse{Cmd: cmd, Status: StatusOK, Changed: uint64(res.intResult)})
	case CmdSPop:
		return EncodeSetResponse(&SetResponse{Cmd: cmd, Status: StatusOK, Elems: []string{res.strResult}})
	default:
		return EncodeSetResponse(&SetResponse{Cmd: cmd, Status: StatusErr, ErrMsg: "unsupported raft set op"})
	}
}
