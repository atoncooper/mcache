package monitor

import "runtime"

// RuntimeCollector provides basic metrics available through the Go runtime.
// It works on all platforms but only exposes Go-specific memory stats and
// CPU core count (GOMAXPROCS), not true system-wide metrics.
type RuntimeCollector struct{}

// NewRuntime returns a runtime-backed collector.
func NewRuntime() *RuntimeCollector {
	return &RuntimeCollector{}
}

func (c *RuntimeCollector) Name() string { return "runtime" }

func (c *RuntimeCollector) Collect() (*SystemSnapshot, error) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	snap := &SystemSnapshot{
		CPU: &CPUMetrics{
			CoreCount: runtime.GOMAXPROCS(0),
		},
		Memory: &MemoryMetrics{
			// Alloc is bytes of allocated (in-use) heap objects.
			Used: ms.Alloc,
			// Idle memory = OS-obtained minus in-use.
			Free: ms.Sys - ms.Alloc,
		},
	}
	if ms.Sys > 0 {
		snap.Memory.UsedPercent = float64(ms.Alloc) / float64(ms.Sys) * 100
	}
	return snap, nil
}
