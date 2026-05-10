package monitor

import (
	"testing"
	"time"
)

// mockCollector returns a fixed snapshot.
type mockCollector struct {
	name string
	snap *SystemSnapshot
}

func (m *mockCollector) Name() string                { return m.name }
func (m *mockCollector) Collect() (*SystemSnapshot, error) { return m.snap, nil }

func TestMonitor_StartStop(t *testing.T) {
	c := &mockCollector{
		name: "mock",
		snap: &SystemSnapshot{CPU: &CPUMetrics{UsagePercent: 42}},
	}
	opts := NewOptions().
		WithInterval(50 * time.Millisecond).
		WithCapacity(5).
		WithCollectors(c)
	m := New(opts)

	m.Start()
	time.Sleep(180 * time.Millisecond) // should collect ~3 times
	m.Stop()

	if m.Count() < 2 {
		t.Errorf("expected at least 2 collections, got %d", m.Count())
	}

	snap, ok := m.Latest()
	if !ok {
		t.Fatal("expected a latest snapshot")
	}
	if snap.CPU == nil || snap.CPU.UsagePercent != 42 {
		t.Errorf("expected CPU 42%%, got %v", snap.CPU)
	}
}

func TestMonitor_RingBuffer(t *testing.T) {
	c := &mockCollector{
		name: "mock",
		snap: &SystemSnapshot{Memory: &MemoryMetrics{Used: 100}},
	}
	opts := NewOptions().
		WithInterval(10 * time.Millisecond).
		WithCapacity(3).
		WithCollectors(c)
	m := New(opts)
	m.Start()
	time.Sleep(80 * time.Millisecond) // ~8 collections, buffer wraps
	m.Stop()

	hist := m.History()
	if len(hist) != 3 {
		t.Fatalf("expected history len=3, got %d", len(hist))
	}
	// Verify all entries have the expected value.
	for i, s := range hist {
		if s.Memory == nil || s.Memory.Used != 100 {
			t.Errorf("history[%d] unexpected memory value", i)
		}
	}
}

func TestMonitor_NoCollectors(t *testing.T) {
	m := New(NewOptions().WithInterval(10 * time.Millisecond))
	m.Start()
	time.Sleep(30 * time.Millisecond)
	m.Stop()

	if m.Count() != 0 {
		t.Errorf("expected 0 collections with no collectors, got %d", m.Count())
	}
	_, ok := m.Latest()
	if ok {
		t.Error("expected no latest snapshot")
	}
}

func TestMonitor_ConcurrentRead(t *testing.T) {
	c := &mockCollector{
		name: "mock",
		snap: &SystemSnapshot{CPU: &CPUMetrics{CoreCount: 4}},
	}
	opts := NewOptions().
		WithInterval(20 * time.Millisecond).
		WithCapacity(10).
		WithCollectors(c)
	m := New(opts)
	m.Start()
	time.Sleep(60 * time.Millisecond)

	done := make(chan struct{})
	for i := range 10 {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			for j := range 50 {
				_ = j
				m.Latest()
				m.History()
				m.Count()
			}
		}(i)
	}
	for range 10 {
		<-done
	}
	m.Stop()
}

func TestNoopCollector(t *testing.T) {
	c := NewNoop()
	if c.Name() != "noop" {
		t.Errorf("expected name noop, got %s", c.Name())
	}
	snap, err := c.Collect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.CPU != nil || snap.Memory != nil {
		t.Error("expected empty snapshot")
	}
}

func TestRuntimeCollector(t *testing.T) {
	c := NewRuntime()
	if c.Name() != "runtime" {
		t.Errorf("expected name runtime, got %s", c.Name())
	}
	snap, err := c.Collect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.CPU == nil || snap.CPU.CoreCount <= 0 {
		t.Error("expected positive core count")
	}
	if snap.Memory == nil {
		t.Error("expected memory metrics")
	}
}

func TestProcCollector_OtherPlatform(t *testing.T) {
	c := NewProc()
	if c.Name() != "proc" {
		t.Errorf("expected name proc, got %s", c.Name())
	}
	_, err := c.Collect()
	// On non-Linux this should error; on Linux it should succeed.
	// We just verify it doesn't panic.
	_ = err
}

func TestOptionsImmutable(t *testing.T) {
	base := NewOptions()
	mod := base.WithInterval(1 * time.Second).WithCapacity(100).WithCollectors(NewNoop())
	if base.interval != 5*time.Second {
		t.Error("base was mutated")
	}
	if mod.interval != 1*time.Second || mod.capacity != 100 || len(mod.collectors) != 1 {
		t.Error("mod did not reflect changes")
	}
}
