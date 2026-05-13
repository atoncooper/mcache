package net

import (
	"encoding/binary"
	"sync/atomic"
	"time"

	"github.com/atoncooper/mcache"
)

// --- Hash operations ---

func (c *Client) HSet(key, field, value string) (int, error) {
	return c.pickConn().hSet(key, field, value, c.readTimeout, c.writeTimeout)
}

func (c *Client) HSetNX(key, field, value string) (bool, error) {
	return c.pickConn().hSetNX(key, field, value, c.readTimeout, c.writeTimeout)
}

func (c *Client) HGet(key, field string) (string, error) {
	return c.pickConn().hGet(key, field, c.readTimeout, c.writeTimeout)
}

func (c *Client) HDel(key string, fields ...string) (int, error) {
	return c.pickConn().hDel(key, c.readTimeout, c.writeTimeout, fields...)
}

func (c *Client) HExists(key, field string) (bool, error) {
	return c.pickConn().hExists(key, field, c.readTimeout, c.writeTimeout)
}

func (c *Client) HGetAll(key string) (map[string]string, error) {
	return c.pickConn().hGetAll(key, c.readTimeout, c.writeTimeout)
}

func (c *Client) HKeys(key string) ([]string, error) {
	return c.pickConn().hKeys(key, c.readTimeout, c.writeTimeout)
}

func (c *Client) HVals(key string) ([]string, error) {
	return c.pickConn().hVals(key, c.readTimeout, c.writeTimeout)
}

func (c *Client) HLen(key string) (int, error) {
	return c.pickConn().hLen(key, c.readTimeout, c.writeTimeout)
}

func (c *Client) HStrLen(key, field string) (int, error) {
	return c.pickConn().hStrLen(key, field, c.readTimeout, c.writeTimeout)
}

func (c *Client) HIncrBy(key, field string, delta int64) (int64, error) {
	return c.pickConn().hIncrBy(key, field, delta, c.readTimeout, c.writeTimeout)
}

func (c *Client) HIncrByFloat(key, field string, delta float64) (float64, error) {
	return c.pickConn().hIncrByFloat(key, field, delta, c.readTimeout, c.writeTimeout)
}

func (c *Client) HMGet(key string, fields ...string) ([]any, error) {
	return c.pickConn().hmGet(key, c.readTimeout, c.writeTimeout, fields...)
}

func (c *Client) HMSet(key string, fvPairs ...string) error {
	return c.pickConn().hmSet(key, c.readTimeout, c.writeTimeout, fvPairs...)
}

// --- List operations ---

func (c *Client) LPush(key string, elems ...string) (int, error) {
	return c.pickConn().lPush(key, c.readTimeout, c.writeTimeout, elems...)
}

func (c *Client) RPush(key string, elems ...string) (int, error) {
	return c.pickConn().rPush(key, c.readTimeout, c.writeTimeout, elems...)
}

func (c *Client) LPop(key string) (string, error) {
	return c.pickConn().lPop(key, c.readTimeout, c.writeTimeout)
}

func (c *Client) RPop(key string) (string, error) {
	return c.pickConn().rPop(key, c.readTimeout, c.writeTimeout)
}

func (c *Client) LLen(key string) (int, error) {
	return c.pickConn().lLen(key, c.readTimeout, c.writeTimeout)
}

func (c *Client) LRange(key string, start, stop int) ([]string, error) {
	return c.pickConn().lRange(key, start, stop, c.readTimeout, c.writeTimeout)
}

func (c *Client) LIndex(key string, index int) (string, error) {
	return c.pickConn().lIndex(key, index, c.readTimeout, c.writeTimeout)
}

func (c *Client) LSet(key string, index int, value string) error {
	return c.pickConn().lSet(key, index, value, c.readTimeout, c.writeTimeout)
}

func (c *Client) LRem(key string, count int, value string) (int, error) {
	return c.pickConn().lRem(key, count, value, c.readTimeout, c.writeTimeout)
}

func (c *Client) LTrim(key string, start, stop int) error {
	return c.pickConn().lTrim(key, start, stop, c.readTimeout, c.writeTimeout)
}

func (c *Client) LInsert(key string, before bool, pivot, value string) (int, error) {
	return c.pickConn().lInsert(key, before, pivot, value, c.readTimeout, c.writeTimeout)
}

func (c *Client) BLPop(key string, timeout time.Duration) (string, error) {
	return c.pickConn().bLPop(key, timeout, c.readTimeout, c.writeTimeout)
}

func (c *Client) BRPop(key string, timeout time.Duration) (string, error) {
	return c.pickConn().bRPop(key, timeout, c.readTimeout, c.writeTimeout)
}

func (c *Client) LPos(key, value string, rank, count, maxLen int) ([]int, error) {
	return c.pickConn().lPos(key, value, rank, count, maxLen, c.readTimeout, c.writeTimeout)
}

// --- Key management operations ---

func (c *Client) Exists(key string) (bool, error) {
	return c.pickConn().existsCmd(key, c.readTimeout, c.writeTimeout)
}

func (c *Client) Type(key string) (byte, error) {
	return c.pickConn().typeCmd(key, c.readTimeout, c.writeTimeout)
}

func (c *Client) Expire(key string, seconds int64) (bool, error) {
	return c.pickConn().expireCmd(key, seconds, c.readTimeout, c.writeTimeout)
}

func (c *Client) PExpire(key string, ms int64) (bool, error) {
	return c.pickConn().pExpireCmd(key, ms, c.readTimeout, c.writeTimeout)
}

func (c *Client) TTL(key string) (int64, error) {
	return c.pickConn().ttlCmd(key, c.readTimeout, c.writeTimeout)
}

func (c *Client) PTTL(key string) (int64, error) {
	return c.pickConn().pttlCmd(key, c.readTimeout, c.writeTimeout)
}

func (c *Client) Persist(key string) (bool, error) {
	return c.pickConn().persistCmd(key, c.readTimeout, c.writeTimeout)
}

func (c *Client) Keys(pattern string) ([]string, error) {
	return c.pickConn().keysCmd(pattern, c.readTimeout, c.writeTimeout)
}

// --- clientConn Hash helpers ---

func (cc *clientConn) doHash(req *HashRequest, readTimeout, writeTimeout time.Duration) (*HashResponse, error) {
	if cc.closed.Load() {
		return nil, ErrConnClosed
	}
	streamID := atomic.AddUint32(&cc.nextStreamID, 1)
	if streamID == 0 {
		streamID = atomic.AddUint32(&cc.nextStreamID, 1)
	}

	respCh := hashRespChPool.Get().(chan *HashResponse)
	cc.pendingHashMap.Store(streamID, respCh)

	defer func() {
		cc.pendingHashMap.Delete(streamID)
		select {
		case <-respCh:
		default:
		}
		hashRespChPool.Put(respCh)
	}()

	payload := EncodeHashRequest(req)
	if payload == nil {
		return nil, ErrBadResponse
	}
	frame := &Frame{
		StreamID: streamID,
		Type:     FrameTypeRequest,
		Payload:  payload,
	}

	if writeTimeout > 0 {
		cc.netConn.SetWriteDeadline(time.Now().Add(writeTimeout))
	}
	cc.writeMu.Lock()
	err := frame.Encode(cc.netConn)
	cc.writeMu.Unlock()
	putBuf(payload)
	if err != nil {
		cc.markBad()
		return nil, err
	}

	// --- doList follows ---

	if readTimeout > 0 {
		timer := time.NewTimer(readTimeout)
		defer timer.Stop()
		select {
		case resp := <-respCh:
			return resp, nil
		case <-timer.C:
			return nil, ErrReadTimeout
		}
	}
	return <-respCh, nil
}

func (cc *clientConn) hSet(key, field, value string, readTimeout, writeTimeout time.Duration) (int, error) {
	resp, err := cc.doHash(&HashRequest{Cmd: CmdHSet, Key: key, Field: field, Value: value}, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, errorString(resp.ErrMsg)
	}
	return int(resp.IntResult), nil
}

func (cc *clientConn) hSetNX(key, field, value string, readTimeout, writeTimeout time.Duration) (bool, error) {
	resp, err := cc.doHash(&HashRequest{Cmd: CmdHSetNX, Key: key, Field: field, Value: value}, readTimeout, writeTimeout)
	if err != nil {
		return false, err
	}
	if resp.Status != StatusOK {
		return false, errorString(resp.ErrMsg)
	}
	return resp.BoolResult, nil
}

func (cc *clientConn) hGet(key, field string, readTimeout, writeTimeout time.Duration) (string, error) {
	resp, err := cc.doHash(&HashRequest{Cmd: CmdHGet, Key: key, Field: field}, readTimeout, writeTimeout)
	if err != nil {
		return "", err
	}
	if resp.Status == StatusNotFound {
		return "", mcache.ErrKeyNotFound
	}
	if resp.Status != StatusOK {
		return "", errorString(resp.ErrMsg)
	}
	return resp.StrResult, nil
}

func (cc *clientConn) hDel(key string, readTimeout, writeTimeout time.Duration, fields ...string) (int, error) {
	resp, err := cc.doHash(&HashRequest{Cmd: CmdHDel, Key: key, Fields: fields}, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, errorString(resp.ErrMsg)
	}
	return int(resp.IntResult), nil
}

func (cc *clientConn) hExists(key, field string, readTimeout, writeTimeout time.Duration) (bool, error) {
	resp, err := cc.doHash(&HashRequest{Cmd: CmdHExists, Key: key, Field: field}, readTimeout, writeTimeout)
	if err != nil {
		return false, err
	}
	if resp.Status != StatusOK {
		return false, errorString(resp.ErrMsg)
	}
	return resp.BoolResult, nil
}

func (cc *clientConn) hGetAll(key string, readTimeout, writeTimeout time.Duration) (map[string]string, error) {
	resp, err := cc.doHash(&HashRequest{Cmd: CmdHGetAll, Key: key}, readTimeout, writeTimeout)
	if err != nil {
		return nil, err
	}
	if resp.Status == StatusNotFound {
		return nil, mcache.ErrKeyNotFound
	}
	if resp.Status != StatusOK {
		return nil, errorString(resp.ErrMsg)
	}
	return resp.MapResult, nil
}

func (cc *clientConn) hKeys(key string, readTimeout, writeTimeout time.Duration) ([]string, error) {
	resp, err := cc.doHash(&HashRequest{Cmd: CmdHKeys, Key: key}, readTimeout, writeTimeout)
	if err != nil {
		return nil, err
	}
	if resp.Status != StatusOK {
		return nil, errorString(resp.ErrMsg)
	}
	return resp.SliceResult, nil
}

func (cc *clientConn) hVals(key string, readTimeout, writeTimeout time.Duration) ([]string, error) {
	resp, err := cc.doHash(&HashRequest{Cmd: CmdHVals, Key: key}, readTimeout, writeTimeout)
	if err != nil {
		return nil, err
	}
	if resp.Status != StatusOK {
		return nil, errorString(resp.ErrMsg)
	}
	return resp.SliceResult, nil
}

func (cc *clientConn) hLen(key string, readTimeout, writeTimeout time.Duration) (int, error) {
	resp, err := cc.doHash(&HashRequest{Cmd: CmdHLen, Key: key}, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, errorString(resp.ErrMsg)
	}
	return int(resp.IntResult), nil
}

func (cc *clientConn) hStrLen(key, field string, readTimeout, writeTimeout time.Duration) (int, error) {
	resp, err := cc.doHash(&HashRequest{Cmd: CmdHStrLen, Key: key, Field: field}, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, errorString(resp.ErrMsg)
	}
	return int(resp.IntResult), nil
}

func (cc *clientConn) hIncrBy(key, field string, delta int64, readTimeout, writeTimeout time.Duration) (int64, error) {
	resp, err := cc.doHash(&HashRequest{Cmd: CmdHIncrBy, Key: key, Field: field, DeltaI64: delta}, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, errorString(resp.ErrMsg)
	}
	return resp.IntResult, nil
}

func (cc *clientConn) hIncrByFloat(key, field string, delta float64, readTimeout, writeTimeout time.Duration) (float64, error) {
	resp, err := cc.doHash(&HashRequest{Cmd: CmdHIncrByFloat, Key: key, Field: field, DeltaF64: delta}, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, errorString(resp.ErrMsg)
	}
	return resp.FloatResult, nil
}

func (cc *clientConn) hmGet(key string, readTimeout, writeTimeout time.Duration, fields ...string) ([]any, error) {
	resp, err := cc.doHash(&HashRequest{Cmd: CmdHMGet, Key: key, Fields: fields}, readTimeout, writeTimeout)
	if err != nil {
		return nil, err
	}
	if resp.Status != StatusOK {
		return nil, errorString(resp.ErrMsg)
	}
	return resp.AnySlice, nil
}

func (cc *clientConn) hmSet(key string, readTimeout, writeTimeout time.Duration, fvPairs ...string) error {
	resp, err := cc.doHash(&HashRequest{Cmd: CmdHMSet, Key: key, FvPairs: fvPairs}, readTimeout, writeTimeout)
	if err != nil {
		return err
	}
	if resp.Status != StatusOK {
		return errorString(resp.ErrMsg)
	}
	return nil
}

// --- clientConn List helpers ---

func (cc *clientConn) doList(req *ListRequest, readTimeout, writeTimeout time.Duration) (*ListResponse, error) {
	if cc.closed.Load() {
		return nil, ErrConnClosed
	}
	streamID := atomic.AddUint32(&cc.nextStreamID, 1)
	if streamID == 0 {
		streamID = atomic.AddUint32(&cc.nextStreamID, 1)
	}

	respCh := listRespChPool.Get().(chan *ListResponse)
	cc.pendingListMap.Store(streamID, respCh)

	defer func() {
		cc.pendingListMap.Delete(streamID)
		select {
		case <-respCh:
		default:
		}
		listRespChPool.Put(respCh)
	}()

	payload := EncodeListRequest(req)
	if payload == nil {
		return nil, ErrBadResponse
	}
	frame := &Frame{
		StreamID: streamID,
		Type:     FrameTypeRequest,
		Payload:  payload,
	}

	if writeTimeout > 0 {
		cc.netConn.SetWriteDeadline(time.Now().Add(writeTimeout))
	}
	cc.writeMu.Lock()
	err := frame.Encode(cc.netConn)
	cc.writeMu.Unlock()
	putBuf(payload)
	if err != nil {
		cc.markBad()
		return nil, err
	}

	if readTimeout > 0 {
		timer := time.NewTimer(readTimeout)
		defer timer.Stop()
		select {
		case resp := <-respCh:
			return resp, nil
		case <-timer.C:
			return nil, ErrReadTimeout
		}
	}
	return <-respCh, nil
}

func (cc *clientConn) lPush(key string, readTimeout, writeTimeout time.Duration, elems ...string) (int, error) {
	resp, err := cc.doList(&ListRequest{Cmd: CmdLPush, Key: key, Elements: elems}, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, errorString(resp.ErrMsg)
	}
	return int(resp.IntResult), nil
}

func (cc *clientConn) rPush(key string, readTimeout, writeTimeout time.Duration, elems ...string) (int, error) {
	resp, err := cc.doList(&ListRequest{Cmd: CmdRPush, Key: key, Elements: elems}, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, errorString(resp.ErrMsg)
	}
	return int(resp.IntResult), nil
}

func (cc *clientConn) lPop(key string, readTimeout, writeTimeout time.Duration) (string, error) {
	resp, err := cc.doList(&ListRequest{Cmd: CmdLPop, Key: key}, readTimeout, writeTimeout)
	if err != nil {
		return "", err
	}
	if resp.Status == StatusNotFound {
		return "", mcache.ErrKeyNotFound
	}
	if resp.Status != StatusOK {
		return "", errorString(resp.ErrMsg)
	}
	return resp.StrResult, nil
}

func (cc *clientConn) rPop(key string, readTimeout, writeTimeout time.Duration) (string, error) {
	resp, err := cc.doList(&ListRequest{Cmd: CmdRPop, Key: key}, readTimeout, writeTimeout)
	if err != nil {
		return "", err
	}
	if resp.Status == StatusNotFound {
		return "", mcache.ErrKeyNotFound
	}
	if resp.Status != StatusOK {
		return "", errorString(resp.ErrMsg)
	}
	return resp.StrResult, nil
}

func (cc *clientConn) lLen(key string, readTimeout, writeTimeout time.Duration) (int, error) {
	resp, err := cc.doList(&ListRequest{Cmd: CmdLLen, Key: key}, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, errorString(resp.ErrMsg)
	}
	return int(resp.IntResult), nil
}

func (cc *clientConn) lRange(key string, start, stop int, readTimeout, writeTimeout time.Duration) ([]string, error) {
	resp, err := cc.doList(&ListRequest{Cmd: CmdLRange, Key: key, Start: int64(start), Stop: int64(stop)}, readTimeout, writeTimeout)
	if err != nil {
		return nil, err
	}
	if resp.Status != StatusOK {
		return nil, errorString(resp.ErrMsg)
	}
	return resp.SliceResult, nil
}

func (cc *clientConn) lIndex(key string, index int, readTimeout, writeTimeout time.Duration) (string, error) {
	resp, err := cc.doList(&ListRequest{Cmd: CmdLIndex, Key: key, Index: int64(index)}, readTimeout, writeTimeout)
	if err != nil {
		return "", err
	}
	if resp.Status == StatusNotFound {
		return "", mcache.ErrKeyNotFound
	}
	if resp.Status != StatusOK {
		return "", errorString(resp.ErrMsg)
	}
	return resp.StrResult, nil
}

func (cc *clientConn) lSet(key string, index int, value string, readTimeout, writeTimeout time.Duration) error {
	resp, err := cc.doList(&ListRequest{Cmd: CmdLSet, Key: key, Index: int64(index), Value: value}, readTimeout, writeTimeout)
	if err != nil {
		return err
	}
	if resp.Status == StatusNotFound {
		return mcache.ErrKeyNotFound
	}
	if resp.Status != StatusOK {
		return errorString(resp.ErrMsg)
	}
	return nil
}

func (cc *clientConn) lRem(key string, count int, value string, readTimeout, writeTimeout time.Duration) (int, error) {
	resp, err := cc.doList(&ListRequest{Cmd: CmdLRem, Key: key, Count: int64(count), Value: value}, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, errorString(resp.ErrMsg)
	}
	return int(resp.IntResult), nil
}

func (cc *clientConn) lTrim(key string, start, stop int, readTimeout, writeTimeout time.Duration) error {
	resp, err := cc.doList(&ListRequest{Cmd: CmdLTrim, Key: key, Start: int64(start), Stop: int64(stop)}, readTimeout, writeTimeout)
	if err != nil {
		return err
	}
	if resp.Status != StatusOK {
		return errorString(resp.ErrMsg)
	}
	return nil
}

func (cc *clientConn) lInsert(key string, before bool, pivot, value string, readTimeout, writeTimeout time.Duration) (int, error) {
	resp, err := cc.doList(&ListRequest{Cmd: CmdLInsert, Key: key, Before: before, Pivot: pivot, Value: value}, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, errorString(resp.ErrMsg)
	}
	return int(resp.IntResult), nil
}

func (cc *clientConn) bLPop(key string, timeout time.Duration, readTimeout, writeTimeout time.Duration) (string, error) {
	req := &ListRequest{Cmd: CmdBLPop, Key: key, Timeout: timeout.Milliseconds()}
	resp, err := cc.doList(req, readTimeout, writeTimeout)
	if err != nil {
		return "", err
	}
	if resp.Status == StatusNotFound {
		return "", mcache.ErrKeyNotFound
	}
	if resp.Status != StatusOK {
		return "", errorString(resp.ErrMsg)
	}
	return resp.StrResult, nil
}

func (cc *clientConn) bRPop(key string, timeout time.Duration, readTimeout, writeTimeout time.Duration) (string, error) {
	req := &ListRequest{Cmd: CmdBRPop, Key: key, Timeout: timeout.Milliseconds()}
	resp, err := cc.doList(req, readTimeout, writeTimeout)
	if err != nil {
		return "", err
	}
	if resp.Status == StatusNotFound {
		return "", mcache.ErrKeyNotFound
	}
	if resp.Status != StatusOK {
		return "", errorString(resp.ErrMsg)
	}
	return resp.StrResult, nil
}

func (cc *clientConn) lPos(key, value string, rank, count, maxLen int, readTimeout, writeTimeout time.Duration) ([]int, error) {
	resp, err := cc.doList(&ListRequest{Cmd: CmdLPos, Key: key, Value: value, Rank: int64(rank), Count: int64(count), MaxLen: int64(maxLen)}, readTimeout, writeTimeout)
	if err != nil {
		return nil, err
	}
	if resp.Status != StatusOK {
		return nil, errorString(resp.ErrMsg)
	}
	result := make([]int, len(resp.PosResult))
	for i, p := range resp.PosResult {
		result[i] = int(p)
	}
	return result, nil
}

// --- clientConn Key management helpers ---

func (cc *clientConn) existsCmd(key string, readTimeout, writeTimeout time.Duration) (bool, error) {
	req := &Request{Cmd: CmdExists, Key: key}
	resp, err := cc.do(req, readTimeout, writeTimeout)
	if err != nil {
		return false, err
	}
	if resp.Status != StatusOK {
		return false, errorString(resp.ErrMsg)
	}
	return len(resp.Value) > 0 && resp.Value[0] == 1, nil
}

func (cc *clientConn) typeCmd(key string, readTimeout, writeTimeout time.Duration) (byte, error) {
	req := &Request{Cmd: CmdType, Key: key}
	resp, err := cc.do(req, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, errorString(resp.ErrMsg)
	}
	if len(resp.Value) > 0 {
		return resp.Value[0], nil
	}
	return 0, nil
}

func (cc *clientConn) expireCmd(key string, seconds int64, readTimeout, writeTimeout time.Duration) (bool, error) {
	req := &Request{Cmd: CmdExpire, Key: key, TTL: seconds * 1000}
	resp, err := cc.do(req, readTimeout, writeTimeout)
	if err != nil {
		return false, err
	}
	if resp.Status != StatusOK {
		return false, errorString(resp.ErrMsg)
	}
	return len(resp.Value) > 0 && resp.Value[0] == 1, nil
}

func (cc *clientConn) pExpireCmd(key string, ms int64, readTimeout, writeTimeout time.Duration) (bool, error) {
	req := &Request{Cmd: CmdPExpire, Key: key, TTL: ms}
	resp, err := cc.do(req, readTimeout, writeTimeout)
	if err != nil {
		return false, err
	}
	if resp.Status != StatusOK {
		return false, errorString(resp.ErrMsg)
	}
	return len(resp.Value) > 0 && resp.Value[0] == 1, nil
}

func (cc *clientConn) ttlCmd(key string, readTimeout, writeTimeout time.Duration) (int64, error) {
	req := &Request{Cmd: CmdTTL, Key: key}
	resp, err := cc.do(req, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, errorString(resp.ErrMsg)
	}
	if len(resp.Value) >= 8 {
		return int64(binary.BigEndian.Uint64(resp.Value)), nil
	}
	return 0, nil
}

func (cc *clientConn) pttlCmd(key string, readTimeout, writeTimeout time.Duration) (int64, error) {
	req := &Request{Cmd: CmdPTTL, Key: key}
	resp, err := cc.do(req, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, errorString(resp.ErrMsg)
	}
	if len(resp.Value) >= 8 {
		return int64(binary.BigEndian.Uint64(resp.Value)), nil
	}
	return 0, nil
}

func (cc *clientConn) persistCmd(key string, readTimeout, writeTimeout time.Duration) (bool, error) {
	req := &Request{Cmd: CmdPersist, Key: key}
	resp, err := cc.do(req, readTimeout, writeTimeout)
	if err != nil {
		return false, err
	}
	if resp.Status != StatusOK {
		return false, errorString(resp.ErrMsg)
	}
	return len(resp.Value) > 0 && resp.Value[0] == 1, nil
}

func (cc *clientConn) keysCmd(pattern string, readTimeout, writeTimeout time.Duration) ([]string, error) {
	req := &Request{Cmd: CmdKeys, Key: pattern}
	resp, err := cc.do(req, readTimeout, writeTimeout)
	if err != nil {
		return nil, err
	}
	if resp.Status != StatusOK {
		return nil, errorString(resp.ErrMsg)
	}
	if len(resp.Value) < 4 {
		return nil, nil
	}
	count := int(binary.BigEndian.Uint32(resp.Value[0:4]))
	off := 4
	result := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if off+2 > len(resp.Value) {
			break
		}
		kl := int(binary.BigEndian.Uint16(resp.Value[off : off+2]))
		off += 2
		if off+kl > len(resp.Value) {
			break
		}
		result = append(result, string(resp.Value[off:off+kl]))
		off += kl
	}
	return result, nil
}
