package cluster

import (
	"context"
	"sync"
	"time"
)

// HealthChecker probes nodes periodically and updates their Healthy flag.
type HealthChecker struct {
	nodes    []*Node
	interval time.Duration
	timeout  time.Duration
	errorLog func(format string, v ...any)
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewHealthChecker creates a checker for the given nodes.
func NewHealthChecker(nodes []*Node, interval, timeout time.Duration, errorLog func(format string, v ...any)) *HealthChecker {
	return &HealthChecker{
		nodes:    nodes,
		interval: interval,
		timeout:  timeout,
		errorLog: errorLog,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the background health-check loop.
func (hc *HealthChecker) Start() {
	hc.wg.Add(1)
	go hc.loop()
}

// Stop halts the health-check loop.
func (hc *HealthChecker) Stop() {
	close(hc.stopCh)
	hc.wg.Wait()
}

func (hc *HealthChecker) loop() {
	defer hc.wg.Done()
	defer func() {
		if r := recover(); r != nil && hc.errorLog != nil {
			hc.errorLog("health check loop panic: %v", r)
		}
	}()
	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()

	hc.probeAll()

	for {
		select {
		case <-ticker.C:
			hc.probeAll()
		case <-hc.stopCh:
			return
		}
	}
}

func (hc *HealthChecker) probeAll() {
	var wg sync.WaitGroup
	for _, n := range hc.nodes {
		if n.Client == nil {
			continue
		}
		wg.Add(1)
		go func(node *Node) {
			defer wg.Done()
			hc.probe(node)
		}(n)
	}
	wg.Wait()
}

func (hc *HealthChecker) probe(n *Node) {
	ctx, cancel := context.WithTimeout(context.Background(), hc.timeout)
	defer cancel()

	done := make(chan struct{})
	var err error
	go func() {
		_, err = n.Client.Len()
		close(done)
	}()

	select {
	case <-done:
		n.Healthy.Store(err == nil)
		if err != nil && hc.errorLog != nil {
			hc.errorLog("health check failed for %s: %v", n.Addr, err)
		}
	case <-ctx.Done():
		n.Healthy.Store(false)
		if hc.errorLog != nil {
			hc.errorLog("health check timeout for %s", n.Addr)
		}
	}
}
