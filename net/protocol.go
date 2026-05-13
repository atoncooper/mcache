package net

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"
)

// bufPool reuses byte buffers for frame encode/decode on the hot path.
// Only buffers up to 4 KB are pooled; larger allocations go to the GC.
var bufPool = sync.Pool{New: func() any { return make([]byte, 0, 256) }}

// framePool reuses Frame structs to reduce GC pressure.
var framePool = sync.Pool{New: func() any { return &Frame{} }}

func getBuf(size int) []byte {
	if size > 4096 {
		return make([]byte, size)
	}
	buf := bufPool.Get().([]byte)
	if cap(buf) >= size {
		return buf[:size]
	}
	return make([]byte, size)
}

func putBuf(buf []byte) {
	if cap(buf) <= 4096 {
		bufPool.Put(buf[:0])
	}
}

func getFrame() *Frame {
	return framePool.Get().(*Frame)
}

func putFrame(f *Frame) {
	f.Payload = nil
	f.StreamID = 0
	f.Type = 0
	f.Flags = 0
	framePool.Put(f)
}

const (
	FrameTypeRequest  byte = 0
	FrameTypeResponse byte = 1
)

// MaxPayloadSize limits the size of a single frame payload.
const MaxPayloadSize = 16 * 1024 * 1024 // 16 MB

// Frame is the basic unit of the multiplexing protocol.
// A single TCP connection can carry interleaved frames from multiple streams.
//
// Binary layout:
//   4 bytes: Payload Length
//   4 bytes: Stream ID
//   1 byte:  Type (0=request, 1=response)
//   1 byte:  Flags
//   N bytes: Payload
type Frame struct {
	StreamID uint32
	Type     byte
	Flags    byte
	Payload  []byte
}

// Encode writes the frame to w.
func (f *Frame) Encode(w io.Writer) error {
	header := make([]byte, 10)
	binary.BigEndian.PutUint32(header[0:4], uint32(len(f.Payload)))
	binary.BigEndian.PutUint32(header[4:8], f.StreamID)
	header[8] = f.Type
	header[9] = f.Flags
	if _, err := w.Write(header); err != nil {
		return err
	}
	if len(f.Payload) > 0 {
		_, err := w.Write(f.Payload)
		return err
	}
	return nil
}

// DecodeFrame reads a Frame from r. The caller should call PutFrame when done
// to return the Frame and its payload buffer to the pool.
func DecodeFrame(r io.Reader) (*Frame, error) {
	var header [10]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	payloadLen := binary.BigEndian.Uint32(header[0:4])
	if payloadLen > MaxPayloadSize {
		return nil, fmt.Errorf("payload too large: %d", payloadLen)
	}
	f := getFrame()
	f.StreamID = binary.BigEndian.Uint32(header[4:8])
	f.Type = header[8]
	f.Flags = header[9]
	if payloadLen > 0 {
		f.Payload = getBuf(int(payloadLen))
		if _, err := io.ReadFull(r, f.Payload); err != nil {
			return nil, err
		}
	}
	return f, nil
}

// PutFrame returns a Frame and its payload to their respective pools.
func PutFrame(f *Frame) {
	if f == nil {
		return
	}
	if f.Payload != nil {
		putBuf(f.Payload)
	}
	putFrame(f)
}

// Request commands.
const (
	CmdGet byte = iota + 1
	CmdSet
	CmdDel
	CmdLen
	CmdCleanup
	CmdStats // return process-level server statistics (JSON)
)

// Set commands (extension to the KV protocol).
const (
	CmdSAdd byte = iota + 10
	CmdSRem
	CmdSIsMember
	CmdSMembers
	CmdSCard
	CmdSPop
	CmdSRandMember
	CmdSUnion
	CmdSInter
	CmdSDiff
)

// Response statuses.
const (
	StatusOK byte = iota
	StatusErr
	StatusNotFound
)

// Request represents a cache operation sent by the client.
type Request struct {
	Cmd   byte
	Key   string
	Value []byte
	TTL   int64 // milliseconds, 0 = default
}

// EncodePayload encodes the request into a binary payload for the frame.
// Layout: [1:Cmd][2:KeyLen][4:ValueLen][8:TTL][Key][Value]
func (req *Request) EncodePayload() []byte {
	keyLen := len(req.Key)
	valLen := len(req.Value)
	size := 1 + 2 + 4 + 8 + keyLen + valLen
	payload := getBuf(size)
	payload[0] = req.Cmd
	binary.BigEndian.PutUint16(payload[1:3], uint16(keyLen))
	binary.BigEndian.PutUint32(payload[3:7], uint32(valLen))
	binary.BigEndian.PutUint64(payload[7:15], uint64(req.TTL))
	copy(payload[15:], req.Key)
	copy(payload[15+keyLen:], req.Value)
	return payload
}

// DecodeRequestPayload decodes a Request from a frame payload.
func DecodeRequestPayload(p []byte) (*Request, error) {
	if len(p) < 15 {
		return nil, fmt.Errorf("request payload too short")
	}
	req := &Request{Cmd: p[0]}
	keyLen := binary.BigEndian.Uint16(p[1:3])
	valLen := binary.BigEndian.Uint32(p[3:7])
	req.TTL = int64(binary.BigEndian.Uint64(p[7:15]))
	offset := 15
	if int(keyLen) > 0 {
		if len(p) < offset+int(keyLen)+int(valLen) {
			return nil, fmt.Errorf("request payload truncated")
		}
		req.Key = string(p[offset : offset+int(keyLen)])
		offset += int(keyLen)
	}
	if int(valLen) > 0 {
		req.Value = make([]byte, valLen)
		copy(req.Value, p[offset:offset+int(valLen)])
	}
	return req, nil
}

// Response represents a cache operation result sent by the server.
type Response struct {
	Status byte
	Value  []byte
	ErrMsg string
}

// EncodePayload encodes the response into a binary payload for the frame.
// Layout: [1:Status][4:ValueLen][2:ErrLen][Value][ErrMsg]
func (resp *Response) EncodePayload() []byte {
	valLen := len(resp.Value)
	errLen := len(resp.ErrMsg)
	size := 1 + 4 + 2 + valLen + errLen
	payload := getBuf(size)
	payload[0] = resp.Status
	binary.BigEndian.PutUint32(payload[1:5], uint32(valLen))
	binary.BigEndian.PutUint16(payload[5:7], uint16(errLen))
	copy(payload[7:], resp.Value)
	copy(payload[7+valLen:], resp.ErrMsg)
	return payload
}

// DecodeResponsePayload decodes a Response from a frame payload.
func DecodeResponsePayload(p []byte) (*Response, error) {
	if len(p) < 7 {
		return nil, fmt.Errorf("response payload too short")
	}
	resp := &Response{Status: p[0]}
	valLen := binary.BigEndian.Uint32(p[1:5])
	errLen := binary.BigEndian.Uint16(p[5:7])
	offset := 7
	if int(valLen) > 0 {
		if len(p) < offset+int(valLen)+int(errLen) {
			return nil, fmt.Errorf("response payload truncated")
		}
		resp.Value = make([]byte, valLen)
		copy(resp.Value, p[offset:offset+int(valLen)])
		offset += int(valLen)
	}
	if int(errLen) > 0 {
		resp.ErrMsg = string(p[offset : offset+int(errLen)])
	}
	return resp, nil
}

// --- Set Request / Response ---

// SetRequest carries a set operation (SAdd/SRem/SMembers etc.).
// Layout (variable, depends on cmd):
//
//	Single-key without elem (SMembers, SCard, SPop):
//	  [1:Cmd][2:KeyLen][Key]
//	Multi-elem (SAdd, SRem):
//	  [1:Cmd][2:KeyLen][4:ElemCount][Key][for each: 4:ElemLen+ElemBytes]
//	Single-elem (SIsMember):
//	  [1:Cmd][2:KeyLen][4:ElemLen][Key][Elem]
//	SRandMember:
//	  [1:Cmd][2:KeyLen][4:Count][Key]
//	Multi-key (SUnion, SInter, SDiff):
//	  [1:Cmd][2:NumKeys][2:Key1Len][Key1][2:Key2Len][Key2]...
type SetRequest struct {
	Cmd   byte
	Key   string
	Elems []string // for SAdd/SRem; SIsMember uses Elems[0]
	Count int32    // for SRandMember
	Keys  []string // for SUnion/SInter/SDiff
}

// SetResponse carries a set operation result.
// Layout depends on operation:
//
//	SAdd/SRem → [1:Status][8:Changed(BE uint64)]
//	SIsMember  → [1:Status][1:IsMember]
//	SCard      → [1:Status][8:Card(BE uint64)]
//	SPop/Rand  → [1:Status][2:ElemLen][Elem]
//	SMembers/Union/Inter/Diff → [1:Status][4:Count][2:Len][Elem]...
type SetResponse struct {
	Cmd      byte
	Status   byte
	Changed  uint64   // SAdd/SRem
	IsMember bool     // SIsMember
	Card     uint64   // SCard
	Elems    []string // SMembers/SPop/SRandMember/SUnion/SInter/SDiff
	ErrMsg   string
}

// EncodeSetRequest encodes a set request into a wire payload.
func EncodeSetRequest(req *SetRequest) []byte {
	switch req.Cmd {
	case CmdSMembers, CmdSCard, CmdSPop:
		keyLen := len(req.Key)
		p := make([]byte, 3+keyLen)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(keyLen))
		copy(p[3:], req.Key)
		return p

	case CmdSAdd, CmdSRem:
		keyLen := len(req.Key)
		elemCount := len(req.Elems)
		total := 7 + keyLen
		for _, e := range req.Elems {
			total += 4 + len(e)
		}
		p := make([]byte, total)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(keyLen))
		binary.BigEndian.PutUint32(p[3:7], uint32(elemCount))
		copy(p[7:], req.Key)
		off := 7 + keyLen
		for _, e := range req.Elems {
			binary.BigEndian.PutUint32(p[off:off+4], uint32(len(e)))
			off += 4
			copy(p[off:], e)
			off += len(e)
		}
		return p

	case CmdSIsMember:
		keyLen := len(req.Key)
		elem := ""
		if len(req.Elems) > 0 {
			elem = req.Elems[0]
		}
		elemLen := len(elem)
		p := make([]byte, 7+keyLen+elemLen)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(keyLen))
		binary.BigEndian.PutUint32(p[3:7], uint32(elemLen))
		copy(p[7:], req.Key)
		copy(p[7+keyLen:], elem)
		return p

	case CmdSRandMember:
		keyLen := len(req.Key)
		p := make([]byte, 7+keyLen)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(keyLen))
		binary.BigEndian.PutUint32(p[3:7], uint32(req.Count))
		copy(p[7:], req.Key)
		return p

	case CmdSUnion, CmdSInter, CmdSDiff:
		total := 3 // 1 cmd + 2 numKeys
		for _, k := range req.Keys {
			total += 2 + len(k)
		}
		p := make([]byte, total)
		p[0] = req.Cmd
		binary.BigEndian.PutUint16(p[1:3], uint16(len(req.Keys)))
		offset := 3
		for _, k := range req.Keys {
			binary.BigEndian.PutUint16(p[offset:offset+2], uint16(len(k)))
			offset += 2
			copy(p[offset:], k)
			offset += len(k)
		}
		return p

	default:
		return nil
	}
}

// DecodeSetRequest decodes a set request from a wire payload.
func DecodeSetRequest(p []byte) (*SetRequest, error) {
	if len(p) < 3 {
		return nil, fmt.Errorf("set request payload too short")
	}
	req := &SetRequest{Cmd: p[0]}
	keyLen := binary.BigEndian.Uint16(p[1:3])

	switch req.Cmd {
	case CmdSMembers, CmdSCard, CmdSPop:
		if len(p) < 3+int(keyLen) {
			return nil, fmt.Errorf("set request payload truncated")
		}
		req.Key = string(p[3 : 3+keyLen])

	case CmdSAdd, CmdSRem:
		if len(p) < 7 {
			return nil, fmt.Errorf("set request payload too short")
		}
		elemCount := binary.BigEndian.Uint32(p[3:7])
		req.Key = string(p[7 : 7+keyLen])
		off := 7 + int(keyLen)
		req.Elems = make([]string, 0, elemCount)
		for i := uint32(0); i < elemCount; i++ {
			if off+4 > len(p) {
				return nil, fmt.Errorf("set request payload truncated")
			}
			el := binary.BigEndian.Uint32(p[off : off+4])
			off += 4
			if off+int(el) > len(p) {
				return nil, fmt.Errorf("set request payload truncated")
			}
			req.Elems = append(req.Elems, string(p[off:off+int(el)]))
			off += int(el)
		}

	case CmdSIsMember:
		if len(p) < 7 {
			return nil, fmt.Errorf("set request payload too short")
		}
		elemLen := binary.BigEndian.Uint32(p[3:7])
		if len(p) < 7+int(keyLen)+int(elemLen) {
			return nil, fmt.Errorf("set request payload truncated")
		}
		req.Key = string(p[7 : 7+keyLen])
		req.Elems = []string{string(p[7+int(keyLen) : 7+int(keyLen)+int(elemLen)])}

	case CmdSRandMember:
		if len(p) < 7 {
			return nil, fmt.Errorf("set request payload too short")
		}
		req.Count = int32(binary.BigEndian.Uint32(p[3:7]))
		if len(p) < 7+int(keyLen) {
			return nil, fmt.Errorf("set request payload truncated")
		}
		req.Key = string(p[7 : 7+keyLen])

	case CmdSUnion, CmdSInter, CmdSDiff:
		numKeys := binary.BigEndian.Uint16(p[1:3])
		offset := 3
		req.Keys = make([]string, 0, numKeys)
		for i := uint16(0); i < numKeys; i++ {
			if offset+2 > len(p) {
				return nil, fmt.Errorf("set request payload truncated")
			}
			kl := binary.BigEndian.Uint16(p[offset : offset+2])
			offset += 2
			if offset+int(kl) > len(p) {
				return nil, fmt.Errorf("set request payload truncated")
			}
			req.Keys = append(req.Keys, string(p[offset:offset+int(kl)]))
			offset += int(kl)
		}

	default:
		return nil, ErrInvalidCommand
	}
	return req, nil
}

// EncodeSetResponse encodes a set response into a wire payload.
func EncodeSetResponse(resp *SetResponse) []byte {
	switch {
	case resp.Cmd == CmdSAdd || resp.Cmd == CmdSRem:
		p := make([]byte, 9)
		p[0] = resp.Status
		binary.BigEndian.PutUint64(p[1:9], resp.Changed)
		return p

	case resp.Cmd == CmdSIsMember:
		p := make([]byte, 2)
		p[0] = resp.Status
		if resp.IsMember {
			p[1] = 1
		}
		return p

	case resp.Cmd == CmdSCard:
		p := make([]byte, 9)
		p[0] = resp.Status
		binary.BigEndian.PutUint64(p[1:9], resp.Card)
		return p

	case resp.Cmd == CmdSPop:
		elemLen := 0
		if len(resp.Elems) > 0 {
			elemLen = len(resp.Elems[0])
		}
		p := make([]byte, 3+elemLen)
		p[0] = resp.Status
		binary.BigEndian.PutUint16(p[1:3], uint16(elemLen))
		if elemLen > 0 {
			copy(p[3:], resp.Elems[0])
		}
		return p

	case resp.Cmd == CmdSMembers || resp.Cmd == CmdSRandMember || resp.Cmd == CmdSUnion || resp.Cmd == CmdSInter || resp.Cmd == CmdSDiff:
		total := 5 // 1 status + 4 count
		for _, e := range resp.Elems {
			total += 2 + len(e)
		}
		p := make([]byte, total)
		p[0] = resp.Status
		binary.BigEndian.PutUint32(p[1:5], uint32(len(resp.Elems)))
		offset := 5
		for _, e := range resp.Elems {
			binary.BigEndian.PutUint16(p[offset:offset+2], uint16(len(e)))
			offset += 2
			copy(p[offset:], e)
			offset += len(e)
		}
		return p

	default:
		return nil
	}
}

// EncodeSetResponse uses cmd to determine output format.
func (resp *SetResponse) EncodePayload() []byte {
	return EncodeSetResponse(resp)
}

// DecodeSetResponse decodes a set response from a wire payload.
// The caller must set resp.Cmd before calling to get correct format.
func DecodeSetResponse(p []byte, cmd byte) (*SetResponse, error) {
	if len(p) < 1 {
		return nil, fmt.Errorf("set response payload too short")
	}
	resp := &SetResponse{Cmd: cmd, Status: p[0]}

	if resp.Status == StatusErr && len(p) > 1 {
		resp.ErrMsg = string(p[1:])
		return resp, nil
	}

	switch cmd {
	case CmdSAdd, CmdSRem:
		if len(p) < 9 {
			return nil, fmt.Errorf("set response payload too short")
		}
		resp.Changed = binary.BigEndian.Uint64(p[1:9])

	case CmdSIsMember:
		if len(p) < 2 {
			return nil, fmt.Errorf("set response payload too short")
		}
		resp.IsMember = p[1] != 0

	case CmdSCard:
		if len(p) < 9 {
			return nil, fmt.Errorf("set response payload too short")
		}
		resp.Card = binary.BigEndian.Uint64(p[1:9])

	case CmdSPop:
		if len(p) < 3 {
			return nil, fmt.Errorf("set response payload too short")
		}
		elemLen := binary.BigEndian.Uint16(p[1:3])
		if elemLen > 0 && len(p) >= 3+int(elemLen) {
			resp.Elems = []string{string(p[3 : 3+elemLen])}
		}

	case CmdSRandMember, CmdSMembers, CmdSUnion, CmdSInter, CmdSDiff:
		if len(p) < 5 {
			return nil, fmt.Errorf("set response payload too short")
		}
		count := binary.BigEndian.Uint32(p[1:5])
		offset := 5
		resp.Elems = make([]string, 0, count)
		for i := uint32(0); i < count; i++ {
			if offset+2 > len(p) {
				break
			}
			el := binary.BigEndian.Uint16(p[offset : offset+2])
			offset += 2
			if offset+int(el) > len(p) {
				break
			}
			resp.Elems = append(resp.Elems, string(p[offset:offset+int(el)]))
			offset += int(el)
		}
	}
	return resp, nil
}
