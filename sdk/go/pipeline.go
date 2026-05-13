package mcache

import (
	"fmt"
	"time"

	mnet "github.com/atoncooper/mcache/net"
)

// Pipeline batches requests on a single connection and flushes them together.
// This reduces per-request overhead (lock acquisition, goroutine switching).
//
// Pipelines are NOT goroutine-safe. For concurrent use, create one pipeline per
// goroutine via Client.Pipeline().
type Pipeline struct {
	raw   *mnet.Pipeline
	codec Codec
}

// Pipeline creates a new request pipeline. Call Reset to reuse it.
func (c *Client) Pipeline() *Pipeline {
	raw := c.transport.Pipeline()
	return &Pipeline{raw: raw, codec: c.codec}
}

// AddSet marshals the value and appends a SET request to the pipeline.
// The request is NOT sent until FlushSets or Flush is called.
func (p *Pipeline) AddSet(key string, value any, ttl time.Duration) error {
	if key == "" {
		return ErrKeyEmpty
	}
	if value == nil {
		return ErrValueNil
	}
	data, err := p.codec.Marshal(value)
	if err != nil {
		return err
	}
	req := &mnet.Request{Cmd: mnet.CmdSet, Key: key, Value: data}
	if ttl > 0 {
		req.TTL = int64(ttl / time.Millisecond)
	}
	return p.raw.Add(req)
}

// AddGet appends a GET request to the pipeline.
// The request is NOT sent until FlushGets or Flush is called.
func (p *Pipeline) AddGet(key string) error {
	if key == "" {
		return ErrKeyEmpty
	}
	return p.raw.Add(&mnet.Request{Cmd: mnet.CmdGet, Key: key})
}

// FlushSets sends all buffered SET requests, waits for responses, and returns
// the first error encountered. All responses must have StatusOK.
func (p *Pipeline) FlushSets() error {
	if err := p.raw.FlushWrite(); err != nil {
		return err
	}
	responses, err := p.raw.ReadResponses()
	if err != nil {
		return err
	}
	for _, resp := range responses {
		if resp.Status != mnet.StatusOK {
			return sdkError(resp.ErrMsg)
		}
	}
	return nil
}

// FlushGets sends all buffered GET requests, waits for responses, and unmarshals
// each response value into the corresponding destination. dests must have the
// same length as the number of AddGet calls.
func (p *Pipeline) FlushGets(dests []any) error {
	if err := p.raw.FlushWrite(); err != nil {
		return err
	}
	responses, err := p.raw.ReadResponses()
	if err != nil {
		return err
	}
	for i, resp := range responses {
		if resp.Status == mnet.StatusNotFound {
			return ErrKeyNotFound
		}
		if resp.Status != mnet.StatusOK {
			return sdkError(resp.ErrMsg)
		}
		if err := p.codec.Unmarshal(resp.Value, dests[i]); err != nil {
			return err
		}
	}
	return nil
}

// Flush sends all buffered requests and collects responses. It returns raw
// responses without interpreting status codes.
func (p *Pipeline) Flush() ([]*mnet.Response, error) {
	if err := p.raw.FlushWrite(); err != nil {
		return nil, err
	}
	return p.raw.ReadResponses()
}

// Len returns the number of buffered (not yet flushed) requests.
func (p *Pipeline) Len() int {
	return p.raw.Len()
}

// Reset clears the pipeline for reuse.
func (p *Pipeline) Reset() {
	p.raw.Reset()
}

func sdkError(msg string) error {
	if msg == "not leader" {
		return mnet.ErrNotLeader
	}
	return fmt.Errorf("mcache: %s", msg)
}
