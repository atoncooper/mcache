package monitor

import "time"

// SystemSnapshot aggregates metrics from all collectors at a single point in time.
type SystemSnapshot struct {
	Timestamp time.Time
	CPU       *CPUMetrics
	Memory    *MemoryMetrics
	IO        []*IOMetrics
	Network   []*NetMetrics
}

// CPUMetrics holds CPU utilization and load information.
type CPUMetrics struct {
	UsagePercent float64
	CoreCount    int
	LoadAvg1     float64
	LoadAvg5     float64
	LoadAvg15    float64
}

// MemoryMetrics holds system memory information.
type MemoryMetrics struct {
	Total       uint64
	Used        uint64
	Free        uint64
	UsedPercent float64
}

// IOMetrics holds per-device disk I/O statistics.
type IOMetrics struct {
	Device         string
	ReadBytes      uint64
	WriteBytes     uint64
	ReadOps        uint64
	WriteOps       uint64
	ReadBytesRate  float64 // bytes per second
	WriteBytesRate float64 // bytes per second
}

// NetMetrics holds per-interface network statistics.
type NetMetrics struct {
	Interface   string
	BytesSent   uint64
	BytesRecv   uint64
	PacketsSent uint64
	PacketsRecv uint64
	SendRate    float64 // bytes per second
	RecvRate    float64 // bytes per second
}
