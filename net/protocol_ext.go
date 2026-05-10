package net

import (
	"encoding/binary"
	"fmt"
	"math"
)

// --- Key type constants ---

const (
	KeyTypeNone   byte = 0
	KeyTypeString byte = 1
	KeyTypeSet    byte = 2
	KeyTypeHash   byte = 3
	KeyTypeList   byte = 4
)

// --- Hash / List / Key management command codes ---

const (
	// Hash commands (32-45)
	CmdHSet         byte = 32
	CmdHGet         byte = 33
	CmdHDel         byte = 34
	CmdHExists      byte = 35
	CmdHGetAll      byte = 36
	CmdHKeys        byte = 37
	CmdHVals        byte = 38
	CmdHLen         byte = 39
	CmdHStrLen      byte = 40
	CmdHIncrBy      byte = 41
	CmdHIncrByFloat byte = 42
	CmdHMGet        byte = 43
	CmdHMSet        byte = 44
	CmdHSetNX       byte = 45

	// List commands (48-61)
	CmdLPush   byte = 48
	CmdRPush   byte = 49
	CmdLPop    byte = 50
	CmdRPop    byte = 51
	CmdLLen    byte = 52
	CmdLRange  byte = 53
	CmdLIndex  byte = 54
	CmdLSet    byte = 55
	CmdLRem    byte = 56
	CmdLTrim   byte = 57
	CmdLInsert byte = 58
	CmdBLPop   byte = 59
	CmdBRPop   byte = 60
	CmdLPos    byte = 61

	// Key management commands (64-73)
	CmdExists    byte = 64
	CmdType      byte = 65
	CmdExpire    byte = 66
	CmdExpireAt  byte = 67
	CmdPExpire   byte = 68
	CmdPExpireAt byte = 69
	CmdTTL       byte = 70
	CmdPTTL      byte = 71
	CmdPersist   byte = 72
	CmdKeys      byte = 73
)

// IsHashCmd reports whether cmd is a hash command (32-45).
func IsHashCmd(cmd byte) bool { return cmd >= CmdHSet && cmd <= CmdHSetNX }

// IsListCmd reports whether cmd is a list command (48-61).
func IsListCmd(cmd byte) bool { return cmd >= CmdLPush && cmd <= CmdLPos }

// IsKeyCmd reports whether cmd is a key management command (64-73).
func IsKeyCmd(cmd byte) bool { return cmd >= CmdExists && cmd <= CmdKeys }

// --- Hash Request / Response ---

// HashRequest carries a hash operation.
type HashRequest struct {
	Cmd      byte
	Key      string
	Field    string
	Value    string
	Fields   []string
	FvPairs  []string
	DeltaI64 int64
	DeltaF64 float64
}

// HashResponse carries a hash operation result.
type HashResponse struct {
	Cmd         byte
	Status      byte
	IntResult   int64
	BoolResult  bool
	StrResult   string
	MapResult   map[string]string
	SliceResult []string
	AnySlice    []any
	FloatResult float64
	ErrMsg      string
}

// EncodeHashRequest encodes a hash request into a wire payload.
func EncodeHashRequest(req *HashRequest) []byte {
	switch req.Cmd {
	case CmdHGetAll, CmdHKeys, CmdHVals, CmdHLen:
		kl := len(req.Key)
		p := make([]byte, 3+kl)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		copy(p[3:], req.Key)
		return p

	case CmdHGet, CmdHExists, CmdHStrLen:
		kl, fl := len(req.Key), len(req.Field)
		p := make([]byte, 5+kl+fl)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		binary.BigEndian.PutUint16(p[3:5], uint16(fl))
		copy(p[5:], req.Key)
		copy(p[5+kl:], req.Field)
		return p

	case CmdHSet, CmdHSetNX:
		kl, fl, vl := len(req.Key), len(req.Field), len(req.Value)
		p := make([]byte, 9+kl+fl+vl)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		binary.BigEndian.PutUint16(p[3:5], uint16(fl))
		binary.BigEndian.PutUint32(p[5:9], uint32(vl))
		copy(p[9:], req.Key)
		copy(p[9+kl:], req.Field)
		copy(p[9+kl+fl:], req.Value)
		return p

	case CmdHDel, CmdHMGet:
		kl := len(req.Key)
		total := 5 + kl
		for _, f := range req.Fields {
			total += 2 + len(f)
		}
		p := make([]byte, total)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		binary.BigEndian.PutUint16(p[3:5], uint16(len(req.Fields)))
		off := 5
		copy(p[off:], req.Key)
		off += kl
		for _, f := range req.Fields {
			binary.BigEndian.PutUint16(p[off:off+2], uint16(len(f)))
			off += 2
			copy(p[off:], f)
			off += len(f)
		}
		return p

	case CmdHIncrBy:
		kl, fl := len(req.Key), len(req.Field)
		p := make([]byte, 13+kl+fl)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		binary.BigEndian.PutUint16(p[3:5], uint16(fl))
		binary.BigEndian.PutUint64(p[5:13], uint64(req.DeltaI64))
		copy(p[13:], req.Key)
		copy(p[13+kl:], req.Field)
		return p

	case CmdHIncrByFloat:
		kl, fl := len(req.Key), len(req.Field)
		p := make([]byte, 13+kl+fl)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		binary.BigEndian.PutUint16(p[3:5], uint16(fl))
		binary.BigEndian.PutUint64(p[5:13], math.Float64bits(req.DeltaF64))
		copy(p[13:], req.Key)
		copy(p[13+kl:], req.Field)
		return p

	case CmdHMSet:
		kl := len(req.Key)
		total := 5 + kl
		for i := 0; i+1 < len(req.FvPairs); i += 2 {
			total += 2 + len(req.FvPairs[i]) + 2 + len(req.FvPairs[i+1])
		}
		p := make([]byte, total)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		binary.BigEndian.PutUint16(p[3:5], uint16(len(req.FvPairs)/2))
		off := 5
		copy(p[off:], req.Key)
		off += kl
		for i := 0; i+1 < len(req.FvPairs); i += 2 {
			f, v := req.FvPairs[i], req.FvPairs[i+1]
			binary.BigEndian.PutUint16(p[off:off+2], uint16(len(f)))
			off += 2
			copy(p[off:], f)
			off += len(f)
			binary.BigEndian.PutUint16(p[off:off+2], uint16(len(v)))
			off += 2
			copy(p[off:], v)
			off += len(v)
		}
		return p
	}
	return nil
}

// DecodeHashRequest decodes a hash request from a wire payload.
func DecodeHashRequest(p []byte) (*HashRequest, error) {
	if len(p) < 3 {
		return nil, fmt.Errorf("hash request payload too short")
	}
	req := &HashRequest{Cmd: p[0]}
	keyLen := int(binary.BigEndian.Uint16(p[1:3]))

	switch req.Cmd {
	case CmdHGetAll, CmdHKeys, CmdHVals, CmdHLen:
		if len(p) < 3+keyLen {
			return nil, fmt.Errorf("hash request payload truncated")
		}
		req.Key = string(p[3 : 3+keyLen])

	case CmdHGet, CmdHExists, CmdHStrLen:
		if len(p) < 5 {
			return nil, fmt.Errorf("hash request payload too short")
		}
		fieldLen := int(binary.BigEndian.Uint16(p[3:5]))
		if len(p) < 5+keyLen+fieldLen {
			return nil, fmt.Errorf("hash request payload truncated")
		}
		req.Key = string(p[5 : 5+keyLen])
		req.Field = string(p[5+keyLen : 5+keyLen+fieldLen])

	case CmdHSet, CmdHSetNX:
		if len(p) < 9 {
			return nil, fmt.Errorf("hash request payload too short")
		}
		fieldLen := int(binary.BigEndian.Uint16(p[3:5]))
		valueLen := int(binary.BigEndian.Uint32(p[5:9]))
		if len(p) < 9+keyLen+fieldLen+valueLen {
			return nil, fmt.Errorf("hash request payload truncated")
		}
		req.Key = string(p[9 : 9+keyLen])
		req.Field = string(p[9+keyLen : 9+keyLen+fieldLen])
		req.Value = string(p[9+keyLen+fieldLen : 9+keyLen+fieldLen+valueLen])

	case CmdHDel, CmdHMGet:
		if len(p) < 5 {
			return nil, fmt.Errorf("hash request payload too short")
		}
		numFields := int(binary.BigEndian.Uint16(p[3:5]))
		off := 5
		if off+keyLen > len(p) {
			return nil, fmt.Errorf("hash request payload truncated")
		}
		req.Key = string(p[off : off+keyLen])
		off += keyLen
		req.Fields = make([]string, 0, numFields)
		for i := 0; i < numFields; i++ {
			if off+2 > len(p) {
				return nil, fmt.Errorf("hash request payload truncated")
			}
			fl := int(binary.BigEndian.Uint16(p[off : off+2]))
			off += 2
			if off+fl > len(p) {
				return nil, fmt.Errorf("hash request payload truncated")
			}
			req.Fields = append(req.Fields, string(p[off:off+fl]))
			off += fl
		}

	case CmdHIncrBy:
		if len(p) < 13 {
			return nil, fmt.Errorf("hash request payload too short")
		}
		fieldLen := int(binary.BigEndian.Uint16(p[3:5]))
		req.DeltaI64 = int64(binary.BigEndian.Uint64(p[5:13]))
		if len(p) < 13+keyLen+fieldLen {
			return nil, fmt.Errorf("hash request payload truncated")
		}
		req.Key = string(p[13 : 13+keyLen])
		req.Field = string(p[13+keyLen : 13+keyLen+fieldLen])

	case CmdHIncrByFloat:
		if len(p) < 13 {
			return nil, fmt.Errorf("hash request payload too short")
		}
		fieldLen := int(binary.BigEndian.Uint16(p[3:5]))
		req.DeltaF64 = math.Float64frombits(binary.BigEndian.Uint64(p[5:13]))
		if len(p) < 13+keyLen+fieldLen {
			return nil, fmt.Errorf("hash request payload truncated")
		}
		req.Key = string(p[13 : 13+keyLen])
		req.Field = string(p[13+keyLen : 13+keyLen+fieldLen])

	case CmdHMSet:
		if len(p) < 5 {
			return nil, fmt.Errorf("hash request payload too short")
		}
		numPairs := int(binary.BigEndian.Uint16(p[3:5]))
		off := 5
		if off+keyLen > len(p) {
			return nil, fmt.Errorf("hash request payload truncated")
		}
		req.Key = string(p[off : off+keyLen])
		off += keyLen
		req.FvPairs = make([]string, 0, numPairs*2)
		for i := 0; i < numPairs; i++ {
			if off+2 > len(p) {
				return nil, fmt.Errorf("hash request payload truncated")
			}
			fl := int(binary.BigEndian.Uint16(p[off : off+2]))
			off += 2
			if off+fl > len(p) {
				return nil, fmt.Errorf("hash request payload truncated")
			}
			f := string(p[off : off+fl])
			off += fl
			if off+2 > len(p) {
				return nil, fmt.Errorf("hash request payload truncated")
			}
			vl := int(binary.BigEndian.Uint16(p[off : off+2]))
			off += 2
			if off+vl > len(p) {
				return nil, fmt.Errorf("hash request payload truncated")
			}
			v := string(p[off : off+vl])
			off += vl
			req.FvPairs = append(req.FvPairs, f, v)
		}

	default:
		return nil, ErrInvalidCommand
	}
	return req, nil
}

// EncodeHashResponse encodes a hash response into a wire payload.
func EncodeHashResponse(resp *HashResponse) []byte {
	if resp.Status == StatusErr {
		errLen := len(resp.ErrMsg)
		p := make([]byte, 3+errLen)
		p[0] = StatusErr
		binary.BigEndian.PutUint16(p[1:3], uint16(errLen))
		copy(p[3:], resp.ErrMsg)
		return p
	}
	switch resp.Cmd {
	case CmdHSet, CmdHDel, CmdHLen, CmdHStrLen, CmdHIncrBy:
		p := make([]byte, 9)
		p[0] = resp.Status
		binary.BigEndian.PutUint64(p[1:9], uint64(resp.IntResult))
		return p

	case CmdHExists, CmdHSetNX:
		p := make([]byte, 2)
		p[0] = resp.Status
		if resp.BoolResult {
			p[1] = 1
		}
		return p

	case CmdHGet:
		vl := len(resp.StrResult)
		p := make([]byte, 5+vl)
		p[0] = resp.Status
		binary.BigEndian.PutUint32(p[1:5], uint32(vl))
		copy(p[5:], resp.StrResult)
		return p

	case CmdHGetAll:
		total := 5
		for k, v := range resp.MapResult {
			total += 2 + len(k) + 4 + len(v)
		}
		p := make([]byte, total)
		p[0] = resp.Status
		binary.BigEndian.PutUint32(p[1:5], uint32(len(resp.MapResult)))
		off := 5
		for k, v := range resp.MapResult {
			binary.BigEndian.PutUint16(p[off:off+2], uint16(len(k)))
			off += 2
			copy(p[off:], k)
			off += len(k)
			binary.BigEndian.PutUint32(p[off:off+4], uint32(len(v)))
			off += 4
			copy(p[off:], v)
			off += len(v)
		}
		return p

	case CmdHKeys, CmdHVals:
		total := 5
		for _, e := range resp.SliceResult {
			total += 2 + len(e)
		}
		p := make([]byte, total)
		p[0] = resp.Status
		binary.BigEndian.PutUint32(p[1:5], uint32(len(resp.SliceResult)))
		off := 5
		for _, e := range resp.SliceResult {
			binary.BigEndian.PutUint16(p[off:off+2], uint16(len(e)))
			off += 2
			copy(p[off:], e)
			off += len(e)
		}
		return p

	case CmdHIncrByFloat:
		p := make([]byte, 9)
		p[0] = resp.Status
		binary.BigEndian.PutUint64(p[1:9], math.Float64bits(resp.FloatResult))
		return p

	case CmdHMGet:
		total := 5
		for _, v := range resp.AnySlice {
			total++
			if s, ok := v.(string); ok {
				total += 4 + len(s)
			}
		}
		p := make([]byte, total)
		p[0] = resp.Status
		binary.BigEndian.PutUint32(p[1:5], uint32(len(resp.AnySlice)))
		off := 5
		for _, v := range resp.AnySlice {
			if s, ok := v.(string); ok {
				p[off] = 1
				off++
				binary.BigEndian.PutUint32(p[off:off+4], uint32(len(s)))
				off += 4
				copy(p[off:], s)
				off += len(s)
			} else {
				p[off] = 0
				off++
			}
		}
		return p

	case CmdHMSet:
		return []byte{resp.Status}
	}
	return nil
}

// DecodeHashResponse decodes a hash response from a wire payload.
func DecodeHashResponse(p []byte, cmd byte) (*HashResponse, error) {
	if len(p) < 1 {
		return nil, fmt.Errorf("hash response payload too short")
	}
	resp := &HashResponse{Cmd: cmd, Status: p[0]}

	if resp.Status == StatusErr {
		if len(p) >= 3 {
			errLen := int(binary.BigEndian.Uint16(p[1:3]))
			if len(p) >= 3+errLen {
				resp.ErrMsg = string(p[3 : 3+errLen])
			}
		}
		return resp, nil
	}

	switch cmd {
	case CmdHSet, CmdHDel, CmdHLen, CmdHStrLen, CmdHIncrBy:
		if len(p) < 9 {
			return nil, fmt.Errorf("hash response payload too short")
		}
		resp.IntResult = int64(binary.BigEndian.Uint64(p[1:9]))

	case CmdHExists, CmdHSetNX:
		if len(p) < 2 {
			return nil, fmt.Errorf("hash response payload too short")
		}
		resp.BoolResult = p[1] != 0

	case CmdHGet:
		if len(p) < 5 {
			return nil, fmt.Errorf("hash response payload too short")
		}
		vl := int(binary.BigEndian.Uint32(p[1:5]))
		if vl > 0 && len(p) >= 5+vl {
			resp.StrResult = string(p[5 : 5+vl])
		}

	case CmdHGetAll:
		if len(p) < 5 {
			return nil, fmt.Errorf("hash response payload too short")
		}
		count := int(binary.BigEndian.Uint32(p[1:5]))
		resp.MapResult = make(map[string]string, count)
		off := 5
		for i := 0; i < count; i++ {
			if off+2 > len(p) {
				break
			}
			kl := int(binary.BigEndian.Uint16(p[off : off+2]))
			off += 2
			if off+kl > len(p) {
				break
			}
			k := string(p[off : off+kl])
			off += kl
			if off+4 > len(p) {
				break
			}
			vl := int(binary.BigEndian.Uint32(p[off : off+4]))
			off += 4
			if off+vl > len(p) {
				break
			}
			v := string(p[off : off+vl])
			off += vl
			resp.MapResult[k] = v
		}

	case CmdHKeys, CmdHVals:
		if len(p) < 5 {
			return nil, fmt.Errorf("hash response payload too short")
		}
		count := int(binary.BigEndian.Uint32(p[1:5]))
		off := 5
		resp.SliceResult = make([]string, 0, count)
		for i := 0; i < count; i++ {
			if off+2 > len(p) {
				break
			}
			el := int(binary.BigEndian.Uint16(p[off : off+2]))
			off += 2
			if off+el > len(p) {
				break
			}
			resp.SliceResult = append(resp.SliceResult, string(p[off:off+el]))
			off += el
		}

	case CmdHIncrByFloat:
		if len(p) < 9 {
			return nil, fmt.Errorf("hash response payload too short")
		}
		resp.FloatResult = math.Float64frombits(binary.BigEndian.Uint64(p[1:9]))

	case CmdHMGet:
		if len(p) < 5 {
			return nil, fmt.Errorf("hash response payload too short")
		}
		count := int(binary.BigEndian.Uint32(p[1:5]))
		off := 5
		resp.AnySlice = make([]any, count)
		for i := 0; i < count; i++ {
			if off >= len(p) {
				break
			}
			hasValue := p[off]
			off++
			if hasValue != 0 {
				if off+4 > len(p) {
					break
				}
				vl := int(binary.BigEndian.Uint32(p[off : off+4]))
				off += 4
				if off+vl > len(p) {
					break
				}
				resp.AnySlice[i] = string(p[off : off+vl])
				off += vl
			}
		}
	}
	return resp, nil
}

// --- List Request / Response ---

// ListRequest carries a list operation.
type ListRequest struct {
	Cmd      byte
	Key      string
	Elements []string
	Value    string
	Index    int64
	Start    int64
	Stop     int64
	Count    int64
	Pivot    string
	Before   bool
	Timeout  int64
	Rank     int64
	MaxLen   int64
}

// ListResponse carries a list operation result.
type ListResponse struct {
	Cmd         byte
	Status      byte
	IntResult   int64
	BoolResult  bool
	StrResult   string
	SliceResult []string
	PosResult   []int64
	ErrMsg      string
}

// EncodeListRequest encodes a list request into a wire payload.
func EncodeListRequest(req *ListRequest) []byte {
	switch req.Cmd {
	case CmdLPop, CmdRPop, CmdLLen:
		kl := len(req.Key)
		p := make([]byte, 3+kl)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		copy(p[3:], req.Key)
		return p

	case CmdLPush, CmdRPush:
		kl := len(req.Key)
		total := 7 + kl
		for _, e := range req.Elements {
			total += 2 + len(e)
		}
		p := make([]byte, total)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		binary.BigEndian.PutUint32(p[3:7], uint32(len(req.Elements)))
		off := 7
		copy(p[off:], req.Key)
		off += kl
		for _, e := range req.Elements {
			binary.BigEndian.PutUint16(p[off:off+2], uint16(len(e)))
			off += 2
			copy(p[off:], e)
			off += len(e)
		}
		return p

	case CmdLRange, CmdLTrim:
		kl := len(req.Key)
		p := make([]byte, 19+kl)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		binary.BigEndian.PutUint64(p[3:11], uint64(req.Start))
		binary.BigEndian.PutUint64(p[11:19], uint64(req.Stop))
		copy(p[19:], req.Key)
		return p

	case CmdLIndex:
		kl := len(req.Key)
		p := make([]byte, 11+kl)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		binary.BigEndian.PutUint64(p[3:11], uint64(req.Index))
		copy(p[11:], req.Key)
		return p

	case CmdLSet:
		kl, vl := len(req.Key), len(req.Value)
		p := make([]byte, 15+kl+vl)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		binary.BigEndian.PutUint64(p[3:11], uint64(req.Index))
		binary.BigEndian.PutUint32(p[11:15], uint32(vl))
		copy(p[15:], req.Key)
		copy(p[15+kl:], req.Value)
		return p

	case CmdLRem:
		kl, vl := len(req.Key), len(req.Value)
		p := make([]byte, 15+kl+vl)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		binary.BigEndian.PutUint64(p[3:11], uint64(req.Count))
		binary.BigEndian.PutUint32(p[11:15], uint32(vl))
		copy(p[15:], req.Key)
		copy(p[15+kl:], req.Value)
		return p

	case CmdLInsert:
		kl, pl, vl := len(req.Key), len(req.Pivot), len(req.Value)
		p := make([]byte, 10+kl+pl+vl)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		if req.Before {
			p[3] = 1
		}
		binary.BigEndian.PutUint16(p[4:6], uint16(pl))
		binary.BigEndian.PutUint32(p[6:10], uint32(vl))
		off := 10
		copy(p[off:], req.Key)
		off += kl
		copy(p[off:], req.Pivot)
		off += pl
		copy(p[off:], req.Value)
		return p

	case CmdBLPop, CmdBRPop:
		kl := len(req.Key)
		p := make([]byte, 11+kl)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		binary.BigEndian.PutUint64(p[3:11], uint64(req.Timeout))
		copy(p[11:], req.Key)
		return p

	case CmdLPos:
		kl, vl := len(req.Key), len(req.Value)
		p := make([]byte, 31+kl+vl)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(kl))
		binary.BigEndian.PutUint64(p[3:11], uint64(req.Rank))
		binary.BigEndian.PutUint64(p[11:19], uint64(req.Count))
		binary.BigEndian.PutUint64(p[19:27], uint64(req.MaxLen))
		binary.BigEndian.PutUint32(p[27:31], uint32(vl))
		copy(p[31:], req.Key)
		copy(p[31+kl:], req.Value)
		return p
	}
	return nil
}

// DecodeListRequest decodes a list request from a wire payload.
func DecodeListRequest(p []byte) (*ListRequest, error) {
	if len(p) < 3 {
		return nil, fmt.Errorf("list request payload too short")
	}
	req := &ListRequest{Cmd: p[0]}
	keyLen := int(binary.BigEndian.Uint16(p[1:3]))

	switch req.Cmd {
	case CmdLPop, CmdRPop, CmdLLen:
		if len(p) < 3+keyLen {
			return nil, fmt.Errorf("list request payload truncated")
		}
		req.Key = string(p[3 : 3+keyLen])

	case CmdLPush, CmdRPush:
		if len(p) < 7 {
			return nil, fmt.Errorf("list request payload too short")
		}
		numElems := int(binary.BigEndian.Uint32(p[3:7]))
		off := 7
		if off+keyLen > len(p) {
			return nil, fmt.Errorf("list request payload truncated")
		}
		req.Key = string(p[off : off+keyLen])
		off += keyLen
		req.Elements = make([]string, 0, numElems)
		for i := 0; i < numElems; i++ {
			if off+2 > len(p) {
				return nil, fmt.Errorf("list request payload truncated")
			}
			el := int(binary.BigEndian.Uint16(p[off : off+2]))
			off += 2
			if off+el > len(p) {
				return nil, fmt.Errorf("list request payload truncated")
			}
			req.Elements = append(req.Elements, string(p[off:off+el]))
			off += el
		}

	case CmdLRange, CmdLTrim:
		if len(p) < 19 {
			return nil, fmt.Errorf("list request payload too short")
		}
		req.Start = int64(binary.BigEndian.Uint64(p[3:11]))
		req.Stop = int64(binary.BigEndian.Uint64(p[11:19]))
		if len(p) < 19+keyLen {
			return nil, fmt.Errorf("list request payload truncated")
		}
		req.Key = string(p[19 : 19+keyLen])

	case CmdLIndex:
		if len(p) < 11 {
			return nil, fmt.Errorf("list request payload too short")
		}
		req.Index = int64(binary.BigEndian.Uint64(p[3:11]))
		if len(p) < 11+keyLen {
			return nil, fmt.Errorf("list request payload truncated")
		}
		req.Key = string(p[11 : 11+keyLen])

	case CmdLSet:
		if len(p) < 15 {
			return nil, fmt.Errorf("list request payload too short")
		}
		req.Index = int64(binary.BigEndian.Uint64(p[3:11]))
		valueLen := int(binary.BigEndian.Uint32(p[11:15]))
		if len(p) < 15+keyLen+valueLen {
			return nil, fmt.Errorf("list request payload truncated")
		}
		req.Key = string(p[15 : 15+keyLen])
		req.Value = string(p[15+keyLen : 15+keyLen+valueLen])

	case CmdLRem:
		if len(p) < 15 {
			return nil, fmt.Errorf("list request payload too short")
		}
		req.Count = int64(binary.BigEndian.Uint64(p[3:11]))
		valueLen := int(binary.BigEndian.Uint32(p[11:15]))
		if len(p) < 15+keyLen+valueLen {
			return nil, fmt.Errorf("list request payload truncated")
		}
		req.Key = string(p[15 : 15+keyLen])
		req.Value = string(p[15+keyLen : 15+keyLen+valueLen])

	case CmdLInsert:
		if len(p) < 10 {
			return nil, fmt.Errorf("list request payload too short")
		}
		req.Before = p[3] != 0
		pivotLen := int(binary.BigEndian.Uint16(p[4:6]))
		valueLen := int(binary.BigEndian.Uint32(p[6:10]))
		off := 10
		if off+keyLen+pivotLen+valueLen > len(p) {
			return nil, fmt.Errorf("list request payload truncated")
		}
		req.Key = string(p[off : off+keyLen])
		off += keyLen
		req.Pivot = string(p[off : off+pivotLen])
		off += pivotLen
		req.Value = string(p[off : off+valueLen])

	case CmdBLPop, CmdBRPop:
		if len(p) < 11 {
			return nil, fmt.Errorf("list request payload too short")
		}
		req.Timeout = int64(binary.BigEndian.Uint64(p[3:11]))
		if len(p) < 11+keyLen {
			return nil, fmt.Errorf("list request payload truncated")
		}
		req.Key = string(p[11 : 11+keyLen])

	case CmdLPos:
		if len(p) < 31 {
			return nil, fmt.Errorf("list request payload too short")
		}
		req.Rank = int64(binary.BigEndian.Uint64(p[3:11]))
		req.Count = int64(binary.BigEndian.Uint64(p[11:19]))
		req.MaxLen = int64(binary.BigEndian.Uint64(p[19:27]))
		valueLen := int(binary.BigEndian.Uint32(p[27:31]))
		if len(p) < 31+keyLen+valueLen {
			return nil, fmt.Errorf("list request payload truncated")
		}
		req.Key = string(p[31 : 31+keyLen])
		req.Value = string(p[31+keyLen : 31+keyLen+valueLen])

	default:
		return nil, ErrInvalidCommand
	}
	return req, nil
}

// EncodeListResponse encodes a list response into a wire payload.
func EncodeListResponse(resp *ListResponse) []byte {
	if resp.Status == StatusErr {
		errLen := len(resp.ErrMsg)
		p := make([]byte, 3+errLen)
		p[0] = StatusErr
		binary.BigEndian.PutUint16(p[1:3], uint16(errLen))
		copy(p[3:], resp.ErrMsg)
		return p
	}
	switch resp.Cmd {
	case CmdLPush, CmdRPush, CmdLLen, CmdLRem, CmdLInsert:
		p := make([]byte, 9)
		p[0] = resp.Status
		binary.BigEndian.PutUint64(p[1:9], uint64(resp.IntResult))
		return p

	case CmdLPop, CmdRPop, CmdBLPop, CmdBRPop, CmdLIndex:
		vl := len(resp.StrResult)
		p := make([]byte, 5+vl)
		p[0] = resp.Status
		binary.BigEndian.PutUint32(p[1:5], uint32(vl))
		copy(p[5:], resp.StrResult)
		return p

	case CmdLRange:
		total := 5
		for _, e := range resp.SliceResult {
			total += 2 + len(e)
		}
		p := make([]byte, total)
		p[0] = resp.Status
		binary.BigEndian.PutUint32(p[1:5], uint32(len(resp.SliceResult)))
		off := 5
		for _, e := range resp.SliceResult {
			binary.BigEndian.PutUint16(p[off:off+2], uint16(len(e)))
			off += 2
			copy(p[off:], e)
			off += len(e)
		}
		return p

	case CmdLSet:
		p := make([]byte, 2)
		p[0] = resp.Status
		if resp.BoolResult {
			p[1] = 1
		}
		return p

	case CmdLTrim:
		return []byte{resp.Status}

	case CmdLPos:
		total := 5 + len(resp.PosResult)*8
		p := make([]byte, total)
		p[0] = resp.Status
		binary.BigEndian.PutUint32(p[1:5], uint32(len(resp.PosResult)))
		off := 5
		for _, pos := range resp.PosResult {
			binary.BigEndian.PutUint64(p[off:off+8], uint64(pos))
			off += 8
		}
		return p
	}
	return nil
}

// DecodeListResponse decodes a list response from a wire payload.
func DecodeListResponse(p []byte, cmd byte) (*ListResponse, error) {
	if len(p) < 1 {
		return nil, fmt.Errorf("list response payload too short")
	}
	resp := &ListResponse{Status: p[0], Cmd: cmd}

	if resp.Status == StatusErr {
		if len(p) >= 3 {
			errLen := int(binary.BigEndian.Uint16(p[1:3]))
			if len(p) >= 3+errLen {
				resp.ErrMsg = string(p[3 : 3+errLen])
			}
		}
		return resp, nil
	}

	switch cmd {
	case CmdLPush, CmdRPush, CmdLLen, CmdLRem, CmdLInsert:
		if len(p) < 9 {
			return nil, fmt.Errorf("list response payload too short")
		}
		resp.IntResult = int64(binary.BigEndian.Uint64(p[1:9]))

	case CmdLPop, CmdRPop, CmdBLPop, CmdBRPop, CmdLIndex:
		if len(p) < 5 {
			return nil, fmt.Errorf("list response payload too short")
		}
		vl := int(binary.BigEndian.Uint32(p[1:5]))
		if vl > 0 && len(p) >= 5+vl {
			resp.StrResult = string(p[5 : 5+vl])
		}

	case CmdLRange:
		if len(p) < 5 {
			return nil, fmt.Errorf("list response payload too short")
		}
		count := int(binary.BigEndian.Uint32(p[1:5]))
		off := 5
		resp.SliceResult = make([]string, 0, count)
		for i := 0; i < count; i++ {
			if off+2 > len(p) {
				break
			}
			el := int(binary.BigEndian.Uint16(p[off : off+2]))
			off += 2
			if off+el > len(p) {
				break
			}
			resp.SliceResult = append(resp.SliceResult, string(p[off:off+el]))
			off += el
		}

	case CmdLSet:
		if len(p) < 2 {
			return nil, fmt.Errorf("list response payload too short")
		}
		resp.BoolResult = p[1] != 0

	case CmdLPos:
		if len(p) < 5 {
			return nil, fmt.Errorf("list response payload too short")
		}
		count := int(binary.BigEndian.Uint32(p[1:5]))
		off := 5
		resp.PosResult = make([]int64, 0, count)
		for i := 0; i < count; i++ {
			if off+8 > len(p) {
				break
			}
			resp.PosResult = append(resp.PosResult, int64(binary.BigEndian.Uint64(p[off:off+8])))
			off += 8
		}
	}
	return resp, nil
}
