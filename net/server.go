package net

import (
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/atoncooper/mcache"
)

// Server serves cache operations over TCP using a multiplexed frame protocol.
// It uses a fixed-size worker pool to process requests concurrently while
// keeping the number of goroutines bounded (connections + workers).
type Server struct {
	cache           *mcache.Cache
	listener        net.Listener
	closed          atomic.Bool
	wg              sync.WaitGroup
	jobCh           chan *job
	workerCount     int
	maxConns        int
	readTimeout     time.Duration
	writeTimeout    time.Duration
	gracefulTimeout time.Duration
	connMu          sync.Mutex
	conns           map[*serverConn]struct{}
	errorLog        func(format string, v ...any)
	infoLog         func(format string, v ...any)
}

// job carries a decoded request from the read loop to a worker.
type job struct {
	sc       *serverConn
	streamID uint32
	req      *Request
	setReq   *SetRequest   // non-nil for set commands
	hashReq  *HashRequest  // non-nil for hash commands
	listReq  *ListRequest  // non-nil for list commands
}

// serverConn wraps a TCP connection. Only the readLoop goroutine reads from
// it; workers acquire writeMu when sending responses back.
type serverConn struct {
	netConn net.Conn
	writeMu sync.Mutex
}

// ServerOption configures the server.
type ServerOption func(*Server)

// WithWorkers sets the number of worker goroutines (default 256).
func WithWorkers(n int) ServerOption {
	return func(s *Server) {
		s.workerCount = n
	}
}

// WithMaxConns limits the number of concurrent TCP connections.
func WithMaxConns(n int) ServerOption {
	return func(s *Server) {
		s.maxConns = n
	}
}

// WithReadTimeout sets the per-frame read timeout.
func WithReadTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.readTimeout = d
	}
}

// WithErrorLog sets a custom error logger for the server (e.g. accept errors).
// If nil, errors are silently dropped.
func WithErrorLog(fn func(format string, v ...any)) ServerOption {
	return func(s *Server) {
		s.errorLog = fn
	}
}

// WithInfoLog sets a custom info logger for the server (connections, etc.).
// If nil, informational events are silently dropped.
func WithInfoLog(fn func(format string, v ...any)) ServerOption {
	return func(s *Server) {
		s.infoLog = fn
	}
}

// WithWriteTimeout sets the per-response write timeout. Default 0 (no timeout).
func WithWriteTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.writeTimeout = d
	}
}

// WithGracefulShutdownTimeout sets the maximum time to wait for active
// connections to finish before forcefully closing. Default 0 (wait forever).
func WithGracefulShutdownTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.gracefulTimeout = d
	}
}

// NewServer creates a multiplexed TCP cache server.
func NewServer(c *mcache.Cache, opts ...ServerOption) *Server {
	s := &Server{
		cache:       c,
		workerCount: 256,
		maxConns:    100000,
		readTimeout: 30 * time.Second,
		jobCh:       make(chan *job, 65536),
		conns:       make(map[*serverConn]struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ListenAndServe starts the TCP listener and blocks until the server is closed.
func (s *Server) ListenAndServe(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = ln

	for i := 0; i < s.workerCount; i++ {
		go s.worker()
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.closed.Load() {
				return ErrServerClosed
			}
			if s.errorLog != nil {
				s.errorLog("accept error: %v", err)
			}
			continue
		}

		s.connMu.Lock()
		if len(s.conns) >= s.maxConns {
			s.connMu.Unlock()
			conn.Close()
			continue
		}
		sc := &serverConn{netConn: conn}
		s.conns[sc] = struct{}{}
		s.connMu.Unlock()

		if s.infoLog != nil {
			s.infoLog("connection opened remote=%s", conn.RemoteAddr().String())
		}

		s.wg.Add(1)
		go s.handleConn(sc)
	}
}

// Close gracefully shuts down the server. If a graceful shutdown timeout is
// configured, Close waits at most that duration for active connections to
// drain before returning.
func (s *Server) Close() error {
	s.closed.Store(true)
	if s.listener != nil {
		s.listener.Close()
	}

	s.connMu.Lock()
	for sc := range s.conns {
		sc.netConn.Close()
	}
	s.conns = make(map[*serverConn]struct{})
	s.connMu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	if s.gracefulTimeout > 0 {
		timer := time.NewTimer(s.gracefulTimeout)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
		}
	} else {
		<-done
	}

	close(s.jobCh)
	return nil
}

func (s *Server) handleConn(sc *serverConn) {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil && s.errorLog != nil {
			s.errorLog("handleConn panic: %v", r)
		}
	}()
	defer func() {
		s.connMu.Lock()
		delete(s.conns, sc)
		s.connMu.Unlock()
		if s.infoLog != nil {
			s.infoLog("connection closed remote=%s", sc.netConn.RemoteAddr().String())
		}
		sc.netConn.Close()
	}()

	for {
		if s.readTimeout > 0 {
			sc.netConn.SetReadDeadline(time.Now().Add(s.readTimeout))
		}

		frame, err := DecodeFrame(sc.netConn)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}

		if frame.Type != FrameTypeRequest {
			continue
		}

		// Determine request type from the first payload byte.
		var kvReq *Request
		var setReq *SetRequest
		var hashReq *HashRequest
		var listReq *ListRequest

		if len(frame.Payload) > 0 {
			cmd := frame.Payload[0]
			switch {
			case IsHashCmd(cmd):
				hr, err := DecodeHashRequest(frame.Payload)
				if err != nil {
					s.writeResponse(sc, frame.StreamID, &Response{Status: StatusErr, ErrMsg: "invalid hash request"})
					continue
				}
				hashReq = hr
			case IsListCmd(cmd):
				lr, err := DecodeListRequest(frame.Payload)
				if err != nil {
					s.writeResponse(sc, frame.StreamID, &Response{Status: StatusErr, ErrMsg: "invalid list request"})
					continue
				}
				listReq = lr
			case cmd >= CmdSAdd && cmd <= CmdSDiff:
				sr, err := DecodeSetRequest(frame.Payload)
				if err != nil {
					s.writeResponse(sc, frame.StreamID, &Response{Status: StatusErr, ErrMsg: "invalid set request"})
					continue
				}
				setReq = sr
			default:
				req, err := DecodeRequestPayload(frame.Payload)
				if err != nil {
					s.writeResponse(sc, frame.StreamID, &Response{Status: StatusErr, ErrMsg: "invalid request"})
					continue
				}
				kvReq = req
			}
		}

		select {
		case s.jobCh <- &job{sc: sc, streamID: frame.StreamID, req: kvReq, setReq: setReq, hashReq: hashReq, listReq: listReq}:
		default:
			// Backpressure: job queue is full.
			resp := &Response{Status: StatusErr, ErrMsg: "server overloaded"}
			s.writeResponse(sc, frame.StreamID, resp)
		}
	}
}

// worker processes jobs from the shared job channel.
func (s *Server) worker() {
	defer func() {
		if r := recover(); r != nil {
			if s.errorLog != nil {
				s.errorLog("worker fatal panic: %v", r)
			}
			// Spawn a replacement worker if the server is still running.
			if !s.closed.Load() {
				go s.worker()
			}
		}
	}()

	for job := range s.jobCh {
		s.processJob(job)
	}
}

func (s *Server) processJob(job *job) {
	defer func() {
		if r := recover(); r != nil {
			if s.errorLog != nil {
				s.errorLog("worker job panic: %v", r)
			}
			resp := &Response{Status: StatusErr, ErrMsg: "internal server error"}
			if job.hashReq != nil {
				s.writeResponse(job.sc, job.streamID, resp)
			} else if job.listReq != nil {
				s.writeResponse(job.sc, job.streamID, resp)
			} else if job.setReq != nil {
				s.writeResponse(job.sc, job.streamID, resp)
			} else {
				s.writeResponse(job.sc, job.streamID, resp)
			}
		}
	}()

	var payload []byte
	switch {
	case job.hashReq != nil:
		raw := s.processHash(job.hashReq)
		payload = make([]byte, 1+len(raw))
		payload[0] = job.hashReq.Cmd
		copy(payload[1:], raw)
	case job.listReq != nil:
		raw := s.processList(job.listReq)
		payload = make([]byte, 1+len(raw))
		payload[0] = job.listReq.Cmd
		copy(payload[1:], raw)
	case job.setReq != nil:
		raw := s.processSet(job.setReq)
		payload = make([]byte, 1+len(raw))
		payload[0] = job.setReq.Cmd
		copy(payload[1:], raw)
	default:
		payload = s.process(job.req).EncodePayload()
	}
	frame := &Frame{
		StreamID: job.streamID,
		Type:     FrameTypeResponse,
		Payload:  payload,
	}
	job.sc.writeMu.Lock()
	if s.writeTimeout > 0 {
		job.sc.netConn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}
	frame.Encode(job.sc.netConn)
	if s.writeTimeout > 0 {
		job.sc.netConn.SetWriteDeadline(time.Time{})
	}
	job.sc.writeMu.Unlock()
}

func (s *Server) writeResponse(sc *serverConn, streamID uint32, resp *Response) {
	frame := &Frame{
		StreamID: streamID,
		Type:     FrameTypeResponse,
		Payload:  resp.EncodePayload(),
	}
	sc.writeMu.Lock()
	if s.writeTimeout > 0 {
		sc.netConn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}
	frame.Encode(sc.netConn)
	if s.writeTimeout > 0 {
		sc.netConn.SetWriteDeadline(time.Time{})
	}
	sc.writeMu.Unlock()
}

func (s *Server) process(req *Request) *Response {
	switch req.Cmd {
	case CmdGet:
		val, err := s.cache.Get(req.Key)
		if err != nil {
			if err == mcache.ErrKeyNotFound {
				return &Response{Status: StatusNotFound}
			}
			return &Response{Status: StatusErr, ErrMsg: err.Error()}
		}
		return &Response{Status: StatusOK, Value: val}

	case CmdSet:
		var ttl time.Duration
		if req.TTL > 0 {
			ttl = time.Duration(req.TTL) * time.Millisecond
		}
		if err := s.cache.Set(req.Key, req.Value, ttl); err != nil {
			return &Response{Status: StatusErr, ErrMsg: err.Error()}
		}
		return &Response{Status: StatusOK}

	case CmdDel:
		if err := s.cache.Del(req.Key); err != nil {
			return &Response{Status: StatusErr, ErrMsg: err.Error()}
		}
		return &Response{Status: StatusOK}

	case CmdLen:
		n := s.cache.Len()
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, uint64(n))
		return &Response{Status: StatusOK, Value: buf}

	case CmdCleanup:
		n := s.cache.Cleanup()
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, uint64(n))
		return &Response{Status: StatusOK, Value: buf}

	// Key management commands
	case CmdExists:
		found, err := s.cache.Exists(req.Key)
		if err != nil {
			return &Response{Status: StatusErr, ErrMsg: err.Error()}
		}
		val := byte(0)
		if found {
			val = 1
		}
		return &Response{Status: StatusOK, Value: []byte{val}}

	case CmdType:
		t, err := s.cache.Type(req.Key)
		if err != nil {
			return &Response{Status: StatusErr, ErrMsg: err.Error()}
		}
		return &Response{Status: StatusOK, Value: []byte{t}}

	case CmdExpire:
		ok, err := s.cache.Expire(req.Key, req.TTL/1000)
		if err != nil {
			return &Response{Status: StatusErr, ErrMsg: err.Error()}
		}
		val := byte(0)
		if ok {
			val = 1
		}
		return &Response{Status: StatusOK, Value: []byte{val}}

	case CmdExpireAt:
		ok, err := s.cache.ExpireAt(req.Key, req.TTL/1000)
		if err != nil {
			return &Response{Status: StatusErr, ErrMsg: err.Error()}
		}
		val := byte(0)
		if ok {
			val = 1
		}
		return &Response{Status: StatusOK, Value: []byte{val}}

	case CmdPExpire:
		ok, err := s.cache.PExpire(req.Key, req.TTL)
		if err != nil {
			return &Response{Status: StatusErr, ErrMsg: err.Error()}
		}
		val := byte(0)
		if ok {
			val = 1
		}
		return &Response{Status: StatusOK, Value: []byte{val}}

	case CmdPExpireAt:
		ok, err := s.cache.PExpireAt(req.Key, req.TTL)
		if err != nil {
			return &Response{Status: StatusErr, ErrMsg: err.Error()}
		}
		val := byte(0)
		if ok {
			val = 1
		}
		return &Response{Status: StatusOK, Value: []byte{val}}

	case CmdTTL:
		ttl, err := s.cache.TTL(req.Key)
		if err != nil {
			return &Response{Status: StatusErr, ErrMsg: err.Error()}
		}
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, uint64(ttl))
		return &Response{Status: StatusOK, Value: buf}

	case CmdPTTL:
		ttl, err := s.cache.PTTL(req.Key)
		if err != nil {
			return &Response{Status: StatusErr, ErrMsg: err.Error()}
		}
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, uint64(ttl))
		return &Response{Status: StatusOK, Value: buf}

	case CmdPersist:
		ok, err := s.cache.Persist(req.Key)
		if err != nil {
			return &Response{Status: StatusErr, ErrMsg: err.Error()}
		}
		val := byte(0)
		if ok {
			val = 1
		}
		return &Response{Status: StatusOK, Value: []byte{val}}

	case CmdKeys:
		keys, err := s.cache.Keys(req.Key)
		if err != nil {
			return &Response{Status: StatusErr, ErrMsg: err.Error()}
		}
		total := 4
		for _, k := range keys {
			total += 2 + len(k)
		}
		buf := make([]byte, total)
		binary.BigEndian.PutUint32(buf[0:4], uint32(len(keys)))
		off := 4
		for _, k := range keys {
			binary.BigEndian.PutUint16(buf[off:off+2], uint16(len(k)))
			off += 2
			copy(buf[off:], k)
			off += len(k)
		}
		return &Response{Status: StatusOK, Value: buf}

	default:
		return &Response{Status: StatusErr, ErrMsg: ErrInvalidCommand.Error()}
	}
}

func (s *Server) processSet(req *SetRequest) []byte {
	switch req.Cmd {
	case CmdSAdd:
		added, err := s.cache.SAdd(req.Key, req.Elems...)
		if err != nil {
			return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusOK, Changed: uint64(added)})

	case CmdSRem:
		removed, err := s.cache.SRem(req.Key, req.Elems...)
		if err != nil {
			return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusOK, Changed: uint64(removed)})

	case CmdSIsMember:
		elem := ""
		if len(req.Elems) > 0 {
			elem = req.Elems[0]
		}
		member, err := s.cache.SIsMember(req.Key, elem)
		if err != nil {
			return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusOK, IsMember: member})

	case CmdSMembers:
		elems, err := s.cache.SMembers(req.Key)
		if err != nil {
			return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusOK, Elems: elems})

	case CmdSCard:
		n, err := s.cache.SCard(req.Key)
		if err != nil {
			return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusOK, Card: uint64(n)})

	case CmdSPop:
		elem, err := s.cache.SPop(req.Key)
		if err != nil {
			if err == mcache.ErrKeyNotFound {
				return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusNotFound})
			}
			return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusOK, Elems: []string{elem}})

	case CmdSRandMember:
		elems, err := s.cache.SRandMember(req.Key, int(req.Count))
		if err != nil {
			return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusOK, Elems: elems})

	case CmdSUnion:
		elems, err := s.cache.SUnion(req.Keys...)
		if err != nil {
			return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusOK, Elems: elems})

	case CmdSInter:
		elems, err := s.cache.SInter(req.Keys...)
		if err != nil {
			return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusOK, Elems: elems})

	case CmdSDiff:
		elems, err := s.cache.SDiff(req.Keys...)
		if err != nil {
			return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: err.Error()})
		}
		return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusOK, Elems: elems})

	default:
		return EncodeSetResponse(&SetResponse{Cmd: req.Cmd, Status: StatusErr, ErrMsg: ErrInvalidCommand.Error()})
	}
}
