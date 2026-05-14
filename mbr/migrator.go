package mbr

import (
	"context"
	"math/bits"
	"sync"
	"sync/atomic"
	"time"

	"github.com/atoncooper/mcache"
	"github.com/atoncooper/mcache/monitor"
)

// MigratorConfig controls migration executor behaviour.
type MigratorConfig struct {
	CheckInterval       time.Duration // progress check interval (default 100ms)
	MaxMigrationTime    time.Duration // maximum time before force-complete (default 5min)
	PauseOnCPUThreshold float64       // CPU ratio above which to pause (default 0.80)
	PauseOnMemThreshold float64       // memory ratio above which to pause (default 0.85)
	TargetLoadPerShard  int           // target keys per shard (default 512)
	MinShards           int           // minimum shard count (default 4)
	MaxShards           int           // maximum shard count (default 1024)
}

// DefaultMigratorConfig returns sensible defaults.
func DefaultMigratorConfig() MigratorConfig {
	return MigratorConfig{
		CheckInterval:       100 * time.Millisecond,
		MaxMigrationTime:    5 * time.Minute,
		PauseOnCPUThreshold: 0.80,
		PauseOnMemThreshold: 0.85,
		TargetLoadPerShard:  512,
		MinShards:           4,
		MaxShards:           1024,
	}
}

// IncrementalMigrationExecutor drives an incremental rehash and adapts to
// system pressure, pausing when the system is overloaded.
type IncrementalMigrationExecutor struct {
	cache   *mcache.Cache
	mon     *monitor.Monitor
	cfg     MigratorConfig
	mu      sync.Mutex
	paused  atomic.Bool
	progress atomic.Value // *MigrationProgress
}

// NewMigrationExecutor creates an executor wired to the given cache and monitor.
func NewMigrationExecutor(c *mcache.Cache, mon *monitor.Monitor, cfg MigratorConfig) *IncrementalMigrationExecutor {
	e := &IncrementalMigrationExecutor{
		cache: c,
		mon:   mon,
		cfg:   cfg,
	}
	e.progress.Store(&MigrationProgress{State: MigrationIdle})
	return e
}

// Progress returns the current migration progress snapshot.
func (e *IncrementalMigrationExecutor) Progress() MigrationProgress {
	p := e.progress.Load()
	if p == nil {
		return MigrationProgress{State: MigrationIdle}
	}
	return *p.(*MigrationProgress)
}

// IsActive reports whether a migration is currently running or paused.
func (e *IncrementalMigrationExecutor) IsActive() bool {
	s := e.Progress().State
	return s == MigrationRunning || s == MigrationPaused
}

// updateProgress atomically sets the migration state.
func (e *IncrementalMigrationExecutor) updateProgress(state MigrationState, pauseReason string) {
	p := e.Progress()
	p.State = state
	p.PauseReason = pauseReason
	if p.StartTime.IsZero() {
		p.StartTime = time.Now()
	}
	p.ElapsedSecs = time.Since(p.StartTime).Seconds()
	e.progress.Store(&p)
}

// Execute triggers an incremental migration to the calculated target shard count
// and runs until completion or context cancellation.
func (e *IncrementalMigrationExecutor) Execute(ctx context.Context) error {
	e.mu.Lock()

	// Already migrating?
	if e.cache.IsRehashing() {
		e.mu.Unlock()
		return nil
	}

	// Calculate target shards
	currentKeys := e.cache.Len()
	targetShards := e.calculateTargetShards(currentKeys)
	currentShards := e.currentShardCount()

	if targetShards <= currentShards {
		e.mu.Unlock()
		return nil // no expansion needed
	}

	// Trigger resize (starts incremental rehash)
	if err := e.cache.Resize(targetShards); err != nil {
		e.mu.Unlock()
		return err
	}

	e.updateProgress(MigrationRunning, "")
	e.mu.Unlock()

	// Monitor rehash progress with pressure-aware pausing.
	ticker := time.NewTicker(e.cfg.CheckInterval)
	defer ticker.Stop()

	start := time.Now()

	for e.cache.IsRehashing() {
		select {
		case <-ticker.C:
			// Check system pressure
			if e.shouldPause() {
				e.paused.Store(true)
				e.updateProgress(MigrationPaused, "system_overload")
				e.waitResume(ctx)
				e.paused.Store(false)
				if ctx.Err() != nil {
					return ctx.Err()
				}
				e.updateProgress(MigrationRunning, "")
			}

			// Force-complete if exceeding max time
			if time.Since(start) > e.cfg.MaxMigrationTime {
				// Let the ongoing rehash finish naturally; force by draining.
				e.forceComplete()
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Mark complete
	e.progress.Store(&MigrationProgress{
		State:     MigrationCompleted,
		OldShards: currentShards,
		NewShards: targetShards,
		StartTime: start,
	})
	return nil
}

func (e *IncrementalMigrationExecutor) shouldPause() bool {
	if e.mon == nil {
		return false
	}
	snap, ok := e.mon.Latest()
	if !ok {
		return false
	}
	if snap.CPU != nil && snap.CPU.UsagePercent > e.cfg.PauseOnCPUThreshold*100 {
		return true
	}
	if snap.Memory != nil && snap.Memory.UsedPercent > e.cfg.PauseOnMemThreshold*100 {
		return true
	}
	return false
}

func (e *IncrementalMigrationExecutor) waitResume(ctx context.Context) {
	ticker := time.NewTicker(e.cfg.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if !e.shouldPause() {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// forceComplete accelerates rehash completion by calling Get on a dummy key
// repeatedly to trigger incremental Step calls.
func (e *IncrementalMigrationExecutor) forceComplete() {
	for e.cache.IsRehashing() {
		// Each call triggers rehash.Step() internally.
		_, _ = e.cache.Get("__mbr_force_complete__")
	}
	e.progress.Store(&MigrationProgress{State: MigrationCompleted})
}

// calculateTargetShards computes the optimal shard count based on key count
// adjusted by memory pressure. When memory exceeds the PID setpoint, the
// effective load per shard is scaled down so more shards are requested.
// Result is always a power of two.
func (e *IncrementalMigrationExecutor) calculateTargetShards(currentKeys int) int {
	effectiveLoad := e.cfg.TargetLoadPerShard

	// Reduce effective target load when memory is under pressure so that
	// large values don't get stuck: 512KB × 1248 keys can saturate memory
	// while the count-based formula thinks only 4 shards are needed.
	if e.mon != nil {
		if snap, ok := e.mon.Latest(); ok && snap.Memory != nil {
			memRatio := snap.Memory.UsedPercent / 100
			const setpoint = 0.60
			if memRatio > setpoint {
				// Scale factor from 0 (at setpoint) to 1 (at 100%).
				pressure := (memRatio - setpoint) / (1.0 - setpoint)
				adjusted := int(float64(effectiveLoad) * (1.0 - pressure*0.75))
				if adjusted < 64 {
					adjusted = 64
				}
				effectiveLoad = adjusted
			}
		}
	}

	target := currentKeys / effectiveLoad
	if target < e.cfg.MinShards {
		target = e.cfg.MinShards
	}
	if target > e.cfg.MaxShards {
		target = e.cfg.MaxShards
	}
	return nextPowerOfTwo(target)
}

func (e *IncrementalMigrationExecutor) currentShardCount() int {
	return e.cache.ShardCount()
}

func nextPowerOfTwo(n int) int {
	if n <= 1 {
		return 1
	}
	// Round up to next power of two.
	return 1 << (bits.Len(uint(n - 1)))
}
