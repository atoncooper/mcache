package net

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/atoncooper/mcache"
)

// startServer creates a cache server on a random port and returns it with the address.
func startServer(t *testing.T) (*Server, string) {
	t.Helper()
	c, err := mcache.New(mcache.NewOptions().WithShards(4).WithMaxSize(10000))
	if err != nil {
		t.Fatalf("create cache: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	// Listen on random port first to get the address
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close() // release for ListenAndServe

	srv := NewServer(c, WithWorkers(4))
	go func() {
		_ = srv.ListenAndServe(addr)
	}()
	t.Cleanup(func() { srv.Close() })

	// Wait briefly for server to start
	time.Sleep(50 * time.Millisecond)
	return srv, addr
}

func newTestClient(t *testing.T, addr string) *Client {
	t.Helper()
	c, err := NewClient(addr, WithPoolSize(1))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestPipeline_SetBatch(t *testing.T) {
	_, addr := startServer(t)
	cl := newTestClient(t, addr)
	p := cl.Pipeline()

	n := 100
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("key-%d", i)
		val := []byte(fmt.Sprintf("val-%d", i))
		if err := p.Add(&Request{Cmd: CmdSet, Key: key, Value: val}); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}

	if p.Len() != n {
		t.Fatalf("expected len %d, got %d", n, p.Len())
	}

	if err := p.FlushWrite(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	resps, err := p.ReadResponses()
	if err != nil {
		t.Fatalf("read responses: %v", err)
	}

	if len(resps) != n {
		t.Fatalf("expected %d responses, got %d", n, len(resps))
	}

	for i, resp := range resps {
		if resp.Status != StatusOK {
			t.Errorf("response %d: expected status OK, got %d (%s)", i, resp.Status, resp.ErrMsg)
		}
	}

	// Verify data was stored via normal GET
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("key-%d", i)
		val, err := cl.Get(key)
		if err != nil {
			t.Errorf("get %s: %v", key, err)
			continue
		}
		if string(val) != fmt.Sprintf("val-%d", i) {
			t.Errorf("get %s: expected val-%d, got %s", key, i, string(val))
		}
	}
}

func TestPipeline_GetBatch(t *testing.T) {
	_, addr := startServer(t)
	cl := newTestClient(t, addr)

	// Pre-populate
	for i := 0; i < 50; i++ {
		if err := cl.Set(fmt.Sprintf("k-%d", i), []byte(fmt.Sprintf("v-%d", i)), 0); err != nil {
			t.Fatalf("set k-%d: %v", i, err)
		}
	}

	p := cl.Pipeline()
	for i := 0; i < 50; i++ {
		if err := p.Add(&Request{Cmd: CmdGet, Key: fmt.Sprintf("k-%d", i)}); err != nil {
			t.Fatalf("add get %d: %v", i, err)
		}
	}

	if err := p.FlushWrite(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	resps, err := p.ReadResponses()
	if err != nil {
		t.Fatalf("read responses: %v", err)
	}

	for i, resp := range resps {
		if resp.Status != StatusOK {
			t.Errorf("response %d: expected status OK, got %d", i, resp.Status)
			continue
		}
		if string(resp.Value) != fmt.Sprintf("v-%d", i) {
			t.Errorf("response %d: expected v-%d, got %s", i, i, string(resp.Value))
		}
	}
}

func TestPipeline_DoBatch(t *testing.T) {
	_, addr := startServer(t)
	cl := newTestClient(t, addr)
	p := cl.Pipeline()

	reqs := make([]*Request, 20)
	for i := 0; i < 20; i++ {
		reqs[i] = &Request{Cmd: CmdSet, Key: fmt.Sprintf("b-%d", i), Value: []byte(fmt.Sprintf("bv-%d", i))}
	}

	resps, err := p.DoBatch(reqs)
	if err != nil {
		t.Fatalf("do batch: %v", err)
	}

	if len(resps) != 20 {
		t.Fatalf("expected 20 responses, got %d", len(resps))
	}

	for _, resp := range resps {
		if resp.Status != StatusOK {
			t.Errorf("expected status OK, got %d", resp.Status)
		}
	}
}

func TestPipeline_EmptyFlush(t *testing.T) {
	_, addr := startServer(t)
	cl := newTestClient(t, addr)
	p := cl.Pipeline()

	if err := p.FlushWrite(); err != nil {
		t.Fatalf("empty flush should succeed: %v", err)
	}

	resps, err := p.ReadResponses()
	if err != nil {
		t.Fatalf("read responses: %v", err)
	}
	if len(resps) != 0 {
		t.Errorf("expected 0 responses, got %d", len(resps))
	}
}

func TestPipeline_Reset(t *testing.T) {
	_, addr := startServer(t)
	cl := newTestClient(t, addr)
	p := cl.Pipeline()

	// Add without flushing, then reset
	for i := 0; i < 5; i++ {
		p.Add(&Request{Cmd: CmdSet, Key: fmt.Sprintf("r-%d", i), Value: []byte("v")})
	}
	p.Reset()

	if p.Len() != 0 {
		t.Errorf("expected len 0 after reset, got %d", p.Len())
	}

	// Pipeline should be reusable after reset
	for i := 0; i < 3; i++ {
		p.Add(&Request{Cmd: CmdSet, Key: fmt.Sprintf("r2-%d", i), Value: []byte("v2")})
	}
	if err := p.FlushWrite(); err != nil {
		t.Fatalf("flush after reset: %v", err)
	}
	resps, err := p.ReadResponses()
	if err != nil {
		t.Fatalf("read responses after reset: %v", err)
	}
	if len(resps) != 3 {
		t.Errorf("expected 3 responses, got %d", len(resps))
	}
}

func TestPipeline_ReuseAfterFlush(t *testing.T) {
	_, addr := startServer(t)
	cl := newTestClient(t, addr)
	p := cl.Pipeline()

	// First batch
	p.Add(&Request{Cmd: CmdSet, Key: "a", Value: []byte("1")})
	p.Add(&Request{Cmd: CmdSet, Key: "b", Value: []byte("2")})
	if _, err := p.DoBatch(nil); err != nil {
		t.Fatalf("first batch: %v", err)
	}
	p.Reset()

	// Second batch
	p.Add(&Request{Cmd: CmdGet, Key: "a"})
	p.Add(&Request{Cmd: CmdGet, Key: "b"})
	if err := p.FlushWrite(); err != nil {
		t.Fatalf("second flush: %v", err)
	}
	resps, err := p.ReadResponses()
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if string(resps[0].Value) != "1" {
		t.Errorf("expected 1, got %s", string(resps[0].Value))
	}
	if string(resps[1].Value) != "2" {
		t.Errorf("expected 2, got %s", string(resps[1].Value))
	}
}

func TestPipeline_ReadBeforeFlush(t *testing.T) {
	_, addr := startServer(t)
	cl := newTestClient(t, addr)
	p := cl.Pipeline()

	p.Add(&Request{Cmd: CmdSet, Key: "x", Value: []byte("y")})

	_, err := p.ReadResponses()
	if err != ErrPipelineNotFlushed {
		t.Errorf("expected ErrPipelineNotFlushed, got %v", err)
	}
}

func TestPipeline_DoubleFlush(t *testing.T) {
	_, addr := startServer(t)
	cl := newTestClient(t, addr)
	p := cl.Pipeline()

	p.Add(&Request{Cmd: CmdSet, Key: "x", Value: []byte("y")})

	if err := p.FlushWrite(); err != nil {
		t.Fatalf("first flush: %v", err)
	}
	// Second flush should return the same result (no-op)
	if err := p.FlushWrite(); err != nil {
		t.Fatalf("second flush: %v", err)
	}
}
