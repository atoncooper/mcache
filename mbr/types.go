// Package mbr implements the Multi-dimensional matrix Based intelligent scheduling
// decision engine. It maintains a sliding window of system features and uses a
// weighted scorecard to decide whether the cache should migrate (expand shards)
// or rely on LRU eviction.
package mbr

import "time"

// Decision is the output of the decision engine.
type Decision string

const (
	DecisionEvict   Decision = "EVICT"
	DecisionMigrate Decision = "MIGRATE"
)

// MigrationState tracks the current phase of an incremental migration.
type MigrationState string

const (
	MigrationIdle      MigrationState = "IDLE"
	MigrationRunning   MigrationState = "RUNNING"
	MigrationPaused    MigrationState = "PAUSED"
	MigrationCompleted MigrationState = "COMPLETED"
)

// EvictionPolicy mirrors the registered eviction policies.
type EvictionPolicy int

const (
	PolicyUnknown EvictionPolicy = iota
	PolicyNoop
	PolicyLRU
	PolicyLFU
)

// WindowStats is a single time-window snapshot of multi-dimensional features.
// All ratio fields are in [0,1]; rate fields are per-second deltas.
type WindowStats struct {
	// --- Capacity pressure ---
	MemUsageRatio  float64 // memory used / total [0,1]
	MemGrowthRate  float64 // memory growth rate (bytes/s)
	KeysGrowthRate float64 // key count growth rate (keys/s)

	// --- Cache efficiency ---
	HitRate         float64 // cache hit ratio [0,1]
	EvictionsPerSec float64 // evictions per second
	AvgEvictedIdle  float64 // average idle time of evicted keys (seconds)

	// --- Access pattern ---
	NewKeysRate    float64 // fraction of recently-seen keys [0,1]
	ReadWriteRatio float64 // read/write ratio (>1 means read-heavy)

	// --- Resource pressure (from monitor) ---
	CPUUtil        float64 // CPU usage [0,1]
	DiskIOPressure float64 // disk I/O pressure [0,1]
	NetUtil        float64 // network utilisation [0,1]

	// --- Migration state ---
	RehashActive    bool    // IsRehashing() == true
	RehashTempMem   float64 // estimated temporary memory during rehash (bytes)
	MigrationActive bool    // migration executor is currently running

	// --- Buffer pressure (false-pressure detection) ---
	ClientOutputBufferUsage float64 // client output buffer usage ratio [0,1]
	ReplBacklogUsage        float64 // replication backlog usage ratio [0,1]
	AofRewriteBufferSize    float64 // AOF rewrite buffer size (bytes)
	LargeInputBufClients    int     // count of clients with large input buffers

	// --- Policy state ---
	CurrentEvictionPolicy EvictionPolicy
	PIDSetpointDeviation  float64 // PID setpoint deviation [-1, 1]
}

// DecisionEvent carries a decision plus metadata to the executor.
type DecisionEvent struct {
	Decision    Decision
	Score       float64
	Timestamp   time.Time
	WindowStats WindowStats
}

// MigrationProgress is a snapshot of migration progress reported by the executor.
type MigrationProgress struct {
	State         MigrationState
	OldShards     int
	NewShards     int
	MigratedKeys  int64
	RemainingKeys int64
	StartTime     time.Time
	ElapsedSecs   float64
	PauseReason   string
}
