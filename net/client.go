package net

import (
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/atoncooper/mcache"
)

var (
	respChPool     = sync.Pool{New: func() any { return make(chan *Response, 1) }}
	setRespChPool  = sync.Pool{New: func() any { return make(chan *SetResponse, 1) }}
	hashRespChPool = sync.Pool{New: func() any { return make(chan *HashResponse, 1) }}
	listRespChPool = sync.Pool{New: func() any { return make(chan *ListResponse, 1) }}
)

// Client is a high-concurrency multiplexed TCP client.
// It maintains a pool of connections; each connection supports concurrent
// in-flight requests via stream IDs.
type Client struct {
	conns        []*clientConn
	connCount    int
	nextConn     uint32 // atomic, round-robin
	readTimeout  time.Duration
	writeTimeout time.Duration
	dialTimeout  time.Duration
}

// clientConn is a single TCP connection that multiplexes multiple requests.
type clientConn struct {
	netConn        net.Conn
	nextStreamID   uint32 // atomic
	pendingMap     sync.Map // uint32 -> chan *Response
	pendingSetMap  sync.Map // uint32 -> chan *SetResponse
	pendingHashMap sync.Map // uint32 -> chan *HashResponse
	pendingListMap sync.Map // uint32 -> chan *ListResponse
	writeMu        sync.Mutex
	closed         atomic.Bool
	readErr        atomic.Value
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithPoolSize sets the number of TCP connections in the pool.
func WithPoolSize(n int) ClientOption {
	return func(c *Client) {
		c.connCount = n
	}
}

// WithClientReadTimeout sets the response read timeout.
func WithClientReadTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.readTimeout = d
	}
}

// WithClientWriteTimeout sets the request write timeout.
func WithClientWriteTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.writeTimeout = d
	}
}

// WithDialTimeout sets the TCP dial timeout.
func WithDialTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.dialTimeout = d
	}
}

// NewClient creates a multiplexed cache client with a connection pool.
func NewClient(addr string, opts ...ClientOption) (*Client, error) {
	c := &Client{
		connCount:    4,
		readTimeout:  10 * time.Second,
		writeTimeout: 5 * time.Second,
		dialTimeout:  5 * time.Second,
	}
	for _, opt := range opts {
		opt(c)
	}

	c.conns = make([]*clientConn, c.connCount)
	for i := 0; i < c.connCount; i++ {
		cc, err := c.dial(addr)
		if err != nil {
			for j := 0; j < i; j++ {
				c.conns[j].netConn.Close()
			}
			return nil, err
		}
		c.conns[i] = cc
		go cc.readLoop()
	}
	return c, nil
}

func (c *Client) dial(addr string) (*clientConn, error) {
	conn, err := net.DialTimeout("tcp", addr, c.dialTimeout)
	if err != nil {
		return nil, err
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		tcp.SetNoDelay(true)
	}
	return &clientConn{
		netConn: conn,
	}, nil
}

func (c *Client) pickConn() *clientConn {
	idx := atomic.AddUint32(&c.nextConn, 1) % uint32(c.connCount)
	return c.conns[idx]
}

// Get retrieves a value by key.
func (c *Client) Get(key string) ([]byte, error) {
	return c.pickConn().get(key, c.readTimeout, c.writeTimeout)
}

// Set stores a value with optional TTL.
func (c *Client) Set(key string, value []byte, ttl time.Duration) error {
	return c.pickConn().set(key, value, ttl, c.readTimeout, c.writeTimeout)
}

// Del removes a key.
func (c *Client) Del(key string) error {
	return c.pickConn().del(key, c.readTimeout, c.writeTimeout)
}

// Len returns the number of entries in the cache.
func (c *Client) Len() (int, error) {
	return c.pickConn().lenCmd(c.readTimeout, c.writeTimeout)
}

// Cleanup triggers expiration cleanup and returns count removed.
func (c *Client) Cleanup() (int, error) {
	return c.pickConn().cleanupCmd(c.readTimeout, c.writeTimeout)
}

// Stats retrieves process-level server statistics as JSON bytes.
func (c *Client) Stats() ([]byte, error) {
	return c.pickConn().statsCmd(c.readTimeout, c.writeTimeout)
}

// Close closes the client and all pooled connections.
func (c *Client) Close() error {
	for _, cc := range c.conns {
		cc.close()
	}
	return nil
}

func (cc *clientConn) get(key string, readTimeout, writeTimeout time.Duration) ([]byte, error) {
	req := &Request{Cmd: CmdGet, Key: key}
	resp, err := cc.do(req, readTimeout, writeTimeout)
	if err != nil {
		return nil, err
	}
	if resp.Status == StatusNotFound {
		return nil, mcache.ErrKeyNotFound
	}
	if resp.Status != StatusOK {
		return nil, serverError(resp.ErrMsg)
	}
	return resp.Value, nil
}

func (cc *clientConn) set(key string, value []byte, ttl time.Duration, readTimeout, writeTimeout time.Duration) error {
	req := &Request{Cmd: CmdSet, Key: key, Value: value}
	if ttl > 0 {
		req.TTL = int64(ttl / time.Millisecond)
	}
	resp, err := cc.do(req, readTimeout, writeTimeout)
	if err != nil {
		return err
	}
	if resp.Status != StatusOK {
		return serverError(resp.ErrMsg)
	}
	return nil
}

func (cc *clientConn) del(key string, readTimeout, writeTimeout time.Duration) error {
	req := &Request{Cmd: CmdDel, Key: key}
	resp, err := cc.do(req, readTimeout, writeTimeout)
	if err != nil {
		return err
	}
	if resp.Status != StatusOK {
		return serverError(resp.ErrMsg)
	}
	return nil
}

func (cc *clientConn) lenCmd(readTimeout, writeTimeout time.Duration) (int, error) {
	req := &Request{Cmd: CmdLen}
	resp, err := cc.do(req, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, serverError(resp.ErrMsg)
	}
	if len(resp.Value) < 8 {
		return 0, ErrBadResponse
	}
	return int(binary.BigEndian.Uint64(resp.Value)), nil
}

func (cc *clientConn) cleanupCmd(readTimeout, writeTimeout time.Duration) (int, error) {
	req := &Request{Cmd: CmdCleanup}
	resp, err := cc.do(req, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, serverError(resp.ErrMsg)
	}
	if len(resp.Value) < 8 {
		return 0, ErrBadResponse
	}
	return int(binary.BigEndian.Uint64(resp.Value)), nil
}

func (cc *clientConn) statsCmd(readTimeout, writeTimeout time.Duration) ([]byte, error) {
	req := &Request{Cmd: CmdStats}
	resp, err := cc.do(req, readTimeout, writeTimeout)
	if err != nil {
		return nil, err
	}
	if resp.Status != StatusOK {
		return nil, serverError(resp.ErrMsg)
	}
	return resp.Value, nil
}

func (cc *clientConn) do(req *Request, readTimeout, writeTimeout time.Duration) (*Response, error) {
	if cc.closed.Load() {
		return nil, ErrConnClosed
	}

	streamID := atomic.AddUint32(&cc.nextStreamID, 1)
	if streamID == 0 {
		streamID = atomic.AddUint32(&cc.nextStreamID, 1)
	}

	respCh := respChPool.Get().(chan *Response)
	cc.pendingMap.Store(streamID, respCh)

	defer func() {
		cc.pendingMap.Delete(streamID)
		select {
		case <-respCh:
		default:
		}
		respChPool.Put(respCh)
	}()

	payload := req.EncodePayload()
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
	resp := <-respCh
	return resp, nil
}

func (cc *clientConn) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			cc.closeAllPending(nil)
		}
	}()
	for {
		frame, err := DecodeFrame(cc.netConn)
		if err != nil {
			cc.readErr.Store(err)
			cc.closeAllPending(err)
			return
		}

		if frame.Type != FrameTypeResponse {
			continue
		}

		// Determine response type from first payload byte
		if len(frame.Payload) > 0 {
			cmd := frame.Payload[0]
			switch {
			case IsHashCmd(cmd):
				hResp, err := DecodeHashResponse(frame.Payload[1:], cmd)
				if err != nil {
					hResp = &HashResponse{Status: StatusErr, ErrMsg: "malformed hash response"}
				}
				v, ok := cc.pendingHashMap.Load(frame.StreamID)
				if !ok {
					continue
				}
				select {
				case v.(chan *HashResponse) <- hResp:
				default:
				}
			case IsListCmd(cmd):
				lResp, err := DecodeListResponse(frame.Payload[1:], cmd)
				if err != nil {
					lResp = &ListResponse{Status: StatusErr, ErrMsg: "malformed list response"}
				}
				v, ok := cc.pendingListMap.Load(frame.StreamID)
				if !ok {
					continue
				}
				select {
				case v.(chan *ListResponse) <- lResp:
				default:
				}
			case cmd >= CmdSAdd && cmd <= CmdSDiff:
				sResp, err := DecodeSetResponse(frame.Payload[1:], cmd)
				if err != nil {
					sResp = &SetResponse{Status: StatusErr, ErrMsg: "malformed set response"}
				}
				v, ok := cc.pendingSetMap.Load(frame.StreamID)
				if !ok {
					continue
				}
				select {
				case v.(chan *SetResponse) <- sResp:
				default:
				}
			default:
				resp, err := DecodeResponsePayload(frame.Payload)
				if err != nil {
					resp = &Response{Status: StatusErr, ErrMsg: "malformed response"}
				}
				v, ok := cc.pendingMap.Load(frame.StreamID)
				if !ok {
					continue
				}
				select {
				case v.(chan *Response) <- resp:
				default:
				}
			}
		}
	}
}

func (cc *clientConn) closeAllPending(err error) {
	cc.pendingMap.Range(func(key, value any) bool {
		select {
		case value.(chan *Response) <- &Response{Status: StatusErr, ErrMsg: err.Error()}:
		default:
		}
		cc.pendingMap.Delete(key)
		return true
	})
	cc.pendingSetMap.Range(func(key, value any) bool {
		select {
		case value.(chan *SetResponse) <- &SetResponse{Cmd: 0, Status: StatusErr, ErrMsg: err.Error()}:
		default:
		}
		cc.pendingSetMap.Delete(key)
		return true
	})
	cc.pendingHashMap.Range(func(key, value any) bool {
		select {
		case value.(chan *HashResponse) <- &HashResponse{Status: StatusErr, ErrMsg: err.Error()}:
		default:
		}
		cc.pendingHashMap.Delete(key)
		return true
	})
	cc.pendingListMap.Range(func(key, value any) bool {
		select {
		case value.(chan *ListResponse) <- &ListResponse{Status: StatusErr, ErrMsg: err.Error()}:
		default:
		}
		cc.pendingListMap.Delete(key)
		return true
	})
}

func (cc *clientConn) markBad() {
	cc.closed.Store(true)
}

// --- Set operations ---

func (c *Client) SAdd(key string, elems ...string) (int, error) {
	return c.pickConn().sAdd(key, c.readTimeout, c.writeTimeout, elems...)
}

func (c *Client) SRem(key string, elems ...string) (int, error) {
	return c.pickConn().sRem(key, c.readTimeout, c.writeTimeout, elems...)
}

func (c *Client) SIsMember(key, elem string) (bool, error) {
	return c.pickConn().sIsMember(key, elem, c.readTimeout, c.writeTimeout)
}

func (c *Client) SMembers(key string) ([]string, error) {
	return c.pickConn().sMembers(key, c.readTimeout, c.writeTimeout)
}

func (c *Client) SCard(key string) (int, error) {
	return c.pickConn().sCard(key, c.readTimeout, c.writeTimeout)
}

func (c *Client) SPop(key string) (string, error) {
	return c.pickConn().sPop(key, c.readTimeout, c.writeTimeout)
}

func (c *Client) SRandMember(key string, count int) ([]string, error) {
	return c.pickConn().sRandMember(key, count, c.readTimeout, c.writeTimeout)
}

func (c *Client) SUnion(keys ...string) ([]string, error) {
	return c.pickConn().sUnion(c.readTimeout, c.writeTimeout, keys...)
}

func (c *Client) SInter(keys ...string) ([]string, error) {
	return c.pickConn().sInter(c.readTimeout, c.writeTimeout, keys...)
}

func (c *Client) SDiff(keys ...string) ([]string, error) {
	return c.pickConn().sDiff(c.readTimeout, c.writeTimeout, keys...)
}

// --- clientConn Set helpers ---

func (cc *clientConn) doSet(req *SetRequest, readTimeout, writeTimeout time.Duration) (*SetResponse, error) {
	if cc.closed.Load() {
		return nil, ErrConnClosed
	}
	streamID := atomic.AddUint32(&cc.nextStreamID, 1)
	if streamID == 0 {
		streamID = atomic.AddUint32(&cc.nextStreamID, 1)
	}

	respCh := setRespChPool.Get().(chan *SetResponse)
	cc.pendingSetMap.Store(streamID, respCh)

	defer func() {
		cc.pendingSetMap.Delete(streamID)
		select {
		case <-respCh:
		default:
		}
		setRespChPool.Put(respCh)
	}()

	payload := EncodeSetRequest(req)
	if payload == nil {
		return nil, ErrBadResponse
	}
	frame := &Frame{StreamID: streamID, Type: FrameTypeRequest, Payload: payload}

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

func (cc *clientConn) sAdd(key string, readTimeout, writeTimeout time.Duration, elems ...string) (int, error) {
	resp, err := cc.doSet(&SetRequest{Cmd: CmdSAdd, Key: key, Elems: elems}, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, serverError(resp.ErrMsg)
	}
	return int(resp.Changed), nil
}

func (cc *clientConn) sRem(key string, readTimeout, writeTimeout time.Duration, elems ...string) (int, error) {
	resp, err := cc.doSet(&SetRequest{Cmd: CmdSRem, Key: key, Elems: elems}, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, serverError(resp.ErrMsg)
	}
	return int(resp.Changed), nil
}

func (cc *clientConn) sIsMember(key, elem string, readTimeout, writeTimeout time.Duration) (bool, error) {
	resp, err := cc.doSet(&SetRequest{Cmd: CmdSIsMember, Key: key, Elems: []string{elem}}, readTimeout, writeTimeout)
	if err != nil {
		return false, err
	}
	if resp.Status != StatusOK {
		return false, serverError(resp.ErrMsg)
	}
	return resp.IsMember, nil
}

func (cc *clientConn) sMembers(key string, readTimeout, writeTimeout time.Duration) ([]string, error) {
	resp, err := cc.doSet(&SetRequest{Cmd: CmdSMembers, Key: key}, readTimeout, writeTimeout)
	if err != nil {
		return nil, err
	}
	if resp.Status != StatusOK {
		return nil, serverError(resp.ErrMsg)
	}
	return resp.Elems, nil
}

func (cc *clientConn) sCard(key string, readTimeout, writeTimeout time.Duration) (int, error) {
	resp, err := cc.doSet(&SetRequest{Cmd: CmdSCard, Key: key}, readTimeout, writeTimeout)
	if err != nil {
		return 0, err
	}
	if resp.Status != StatusOK {
		return 0, serverError(resp.ErrMsg)
	}
	return int(resp.Card), nil
}

func (cc *clientConn) sPop(key string, readTimeout, writeTimeout time.Duration) (string, error) {
	resp, err := cc.doSet(&SetRequest{Cmd: CmdSPop, Key: key}, readTimeout, writeTimeout)
	if err != nil {
		return "", err
	}
	if resp.Status == StatusNotFound {
		return "", mcache.ErrKeyNotFound
	}
	if resp.Status != StatusOK {
		return "", serverError(resp.ErrMsg)
	}
	if len(resp.Elems) > 0 {
		return resp.Elems[0], nil
	}
	return "", nil
}

func (cc *clientConn) sRandMember(key string, count int, readTimeout, writeTimeout time.Duration) ([]string, error) {
	resp, err := cc.doSet(&SetRequest{Cmd: CmdSRandMember, Key: key, Count: int32(count)}, readTimeout, writeTimeout)
	if err != nil {
		return nil, err
	}
	if resp.Status != StatusOK {
		return nil, serverError(resp.ErrMsg)
	}
	return resp.Elems, nil
}

func (cc *clientConn) sUnion(readTimeout, writeTimeout time.Duration, keys ...string) ([]string, error) {
	resp, err := cc.doSet(&SetRequest{Cmd: CmdSUnion, Keys: keys}, readTimeout, writeTimeout)
	if err != nil {
		return nil, err
	}
	if resp.Status != StatusOK {
		return nil, serverError(resp.ErrMsg)
	}
	return resp.Elems, nil
}

func (cc *clientConn) sInter(readTimeout, writeTimeout time.Duration, keys ...string) ([]string, error) {
	resp, err := cc.doSet(&SetRequest{Cmd: CmdSInter, Keys: keys}, readTimeout, writeTimeout)
	if err != nil {
		return nil, err
	}
	if resp.Status != StatusOK {
		return nil, serverError(resp.ErrMsg)
	}
	return resp.Elems, nil
}

func (cc *clientConn) sDiff(readTimeout, writeTimeout time.Duration, keys ...string) ([]string, error) {
	resp, err := cc.doSet(&SetRequest{Cmd: CmdSDiff, Keys: keys}, readTimeout, writeTimeout)
	if err != nil {
		return nil, err
	}
	if resp.Status != StatusOK {
		return nil, serverError(resp.ErrMsg)
	}
	return resp.Elems, nil
}

func (cc *clientConn) close() {
	cc.closed.Store(true)
	cc.netConn.Close()
	cc.closeAllPending(ErrConnClosed)
}

func serverError(msg string) error {
	if msg == "not leader" {
		return ErrNotLeader
	}
	return errorString(msg)
}

type errorString string

func (e errorString) Error() string { return string(e) }
