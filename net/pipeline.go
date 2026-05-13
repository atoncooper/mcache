package net

import (
	"errors"
	"sync/atomic"
	"time"
)

var (
	ErrPipelineClosed     = errors.New("pipeline: already flushed, must reset before reuse")
	ErrPipelineNotFlushed = errors.New("pipeline: must flush before reading responses")
)

// Pipeline buffers requests on a single connection and flushes them in one batch.
// This amortizes writeMu lock acquisition and goroutine switching overhead:
// N requests cost 1 lock + 1 unlock instead of N each.
//
// Pipelines are NOT goroutine-safe; each goroutine should use its own.
type Pipeline struct {
	conn         *clientConn
	readTimeout  time.Duration
	writeTimeout time.Duration

	frames   []*Frame
	payloads [][]byte // to putBuf after write
	ids      []uint32
	chs      []chan *Response

	written  bool
	writeErr error
}

// NewPipeline creates a pipeline bound to this connection.
func (cc *clientConn) NewPipeline(readTimeout, writeTimeout time.Duration) *Pipeline {
	return &Pipeline{
		conn:         cc,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
	}
}

// Pipeline creates a pipeline on a connection selected by round-robin.
func (c *Client) Pipeline() *Pipeline {
	return c.pickConn().NewPipeline(c.readTimeout, c.writeTimeout)
}

// Add appends a request to the pipeline without sending it.
// Returns an error if the pipeline has already been flushed.
func (p *Pipeline) Add(req *Request) error {
	if p.written {
		return ErrPipelineClosed
	}
	if p.conn.closed.Load() {
		return ErrConnClosed
	}

	streamID := atomic.AddUint32(&p.conn.nextStreamID, 1)
	if streamID == 0 {
		streamID = atomic.AddUint32(&p.conn.nextStreamID, 1)
	}

	respCh := respChPool.Get().(chan *Response)
	p.conn.pendingMap.Store(streamID, respCh)

	payload := req.EncodePayload()
	frame := &Frame{
		StreamID: streamID,
		Type:     FrameTypeRequest,
		Payload:  payload,
	}

	p.frames = append(p.frames, frame)
	p.payloads = append(p.payloads, payload)
	p.ids = append(p.ids, streamID)
	p.chs = append(p.chs, respCh)

	return nil
}

// FlushWrite sends all buffered frames to the connection in a single writeMu
// acquisition. After this call, the pipeline is in "reading" state.
func (p *Pipeline) FlushWrite() error {
	if p.written {
		return p.writeErr
	}
	p.written = true

	if p.conn.closed.Load() {
		p.cleanup(ErrConnClosed)
		return ErrConnClosed
	}

	if len(p.frames) == 0 {
		return nil
	}

	if p.writeTimeout > 0 {
		p.conn.netConn.SetWriteDeadline(time.Now().Add(p.writeTimeout))
	}

	p.conn.writeMu.Lock()
	for _, frame := range p.frames {
		if err := frame.Encode(p.conn.netConn); err != nil {
			p.conn.writeMu.Unlock()
			p.conn.markBad()
			p.writeErr = err
			p.cleanup(err)
			return err
		}
	}
	p.conn.writeMu.Unlock()

	for _, payload := range p.payloads {
		putBuf(payload)
	}
	p.payloads = nil

	return nil
}

// ReadResponses waits for all pending responses and returns them in the same
// order as Add calls. Must be called after FlushWrite.
func (p *Pipeline) ReadResponses() ([]*Response, error) {
	if !p.written {
		return nil, ErrPipelineNotFlushed
	}

	n := len(p.chs)
	results := make([]*Response, n)

	if p.readTimeout > 0 {
		deadline := time.Now().Add(p.readTimeout)
		for i, ch := range p.chs {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				for j := i; j < n; j++ {
					results[j] = &Response{Status: StatusErr, ErrMsg: "pipeline read timeout"}
				}
				p.cleanup(nil)
				return results, ErrReadTimeout
			}
			timer := time.NewTimer(remaining)
			select {
			case resp := <-ch:
				results[i] = resp
				timer.Stop()
			case <-timer.C:
				for j := i; j < n; j++ {
					results[j] = &Response{Status: StatusErr, ErrMsg: "pipeline read timeout"}
				}
				p.cleanup(nil)
				return results, ErrReadTimeout
			}
		}
	} else {
		for i, ch := range p.chs {
			results[i] = <-ch
		}
	}

	p.cleanup(nil)
	return results, nil
}

// DoBatch is a convenience method that combines Add + FlushWrite + ReadResponses
// for a slice of requests. The pipeline is consumed and cannot be reused without
// calling Reset.
func (p *Pipeline) DoBatch(reqs []*Request) ([]*Response, error) {
	for _, req := range reqs {
		if err := p.Add(req); err != nil {
			return nil, err
		}
	}
	if err := p.FlushWrite(); err != nil {
		return nil, err
	}
	return p.ReadResponses()
}

// Len returns the number of buffered (not yet flushed) requests.
func (p *Pipeline) Len() int {
	return len(p.frames)
}

// Reset clears the pipeline for reuse. If the pipeline has been flushed,
// pending responses must already have been read via ReadResponses.
func (p *Pipeline) Reset() {
	if !p.written {
		p.cleanup(nil)
	}
	p.frames = p.frames[:0]
	p.payloads = p.payloads[:0]
	p.ids = p.ids[:0]
	p.chs = p.chs[:0]
	p.written = false
	p.writeErr = nil
}

func (p *Pipeline) cleanup(_ error) {
	for _, frame := range p.frames {
		if frame.Payload != nil {
			putBuf(frame.Payload)
		}
	}
	for i, id := range p.ids {
		p.conn.pendingMap.Delete(id)
		select {
		case <-p.chs[i]:
		default:
		}
		respChPool.Put(p.chs[i])
	}
}
