package mbr

import (
	"sync/atomic"
	"time"

	"github.com/atoncooper/mcache"
	"github.com/atoncooper/mcache/monitor"
)

// StatsProvider supplies WindowStats for the decision engine.
type StatsProvider interface {
	GetLatestStats() WindowStats
}

// DefaultStatsProvider collects features by integrating:
//   - mcache.Cache (Len, IsRehashing, eviction policy)
//   - monitor.Monitor (CPU, memory, IO, network)
//   - an internal CacheObserver (hit rate, eviction rate, evicted-idle tracking)
//   - a PIDController for setpoint deviation
type DefaultStatsProvider struct {
	cache   *mcache.Cache
	mon     *monitor.Monitor
	pid     *PIDController

	// Observer-driven counters (updated from cache callbacks)
	hits      atomic.Int64
	misses    atomic.Int64
	sets      atomic.Uint64
	evictions atomic.Int64
	evictIdle atomic.Int64 // cumulative idle nanos of evicted keys
	evictCnt  atomic.Uint64

	// Previous-state tracking for rate calculations
	prevKeys    int64
	prevHit     int64
	prevMiss    int64
	prevEvict   int64
	prevMem     float64
	lastCollect time.Time
}

// Ensure DefaultStatsProvider satisfies StatsProvider.
var _ StatsProvider = (*DefaultStatsProvider)(nil)

// NewDefaultStatsProvider creates a provider wired to the given cache and monitor.
func NewDefaultStatsProvider(c *mcache.Cache, mon *monitor.Monitor, pid *PIDController) *DefaultStatsProvider {
	return &DefaultStatsProvider{
		cache:       c,
		mon:         mon,
		pid:         pid,
		lastCollect: time.Now(),
	}
}

// Observer returns a cache.Observer-compatible handle for tracking
// hit / miss / eviction events. The caller should inject this into
// the Cache via WithObserver. If another observer (e.g. infra.Infra)
// is already in use, compose them with a fan-out observer.
func (p *DefaultStatsProvider) Observer() mcache.CacheObserver {
	return &providerObserver{p: p}
}

// GetLatestStats gathers all features into a WindowStats snapshot and
// updates internal rate-trackers.
func (p *DefaultStatsProvider) GetLatestStats() WindowStats {
	now := time.Now()
	dt := now.Sub(p.lastCollect).Seconds()
	if dt <= 0 {
		dt = 0.5
	}
	p.lastCollect = now

	stats := WindowStats{}

	// --- Capacity pressure ---
	currentKeys := int64(p.cache.Len())
	keyDelta := currentKeys - p.prevKeys
	p.prevKeys = currentKeys
	if keyDelta < 0 {
		keyDelta = 0
	}
	stats.KeysGrowthRate = float64(keyDelta) / dt

	// --- Cache efficiency (from observer counters) ---
	h := p.hits.Load()
	m := p.misses.Load()
	totalAccess := h + m
	hDelta := h - p.prevHit
	mDelta := m - p.prevMiss
	totalDelta := hDelta + mDelta
	p.prevHit = h
	p.prevMiss = m

	if totalAccess > 0 {
		stats.HitRate = float64(h) / float64(totalAccess)
	}

	eCurr := p.evictions.Load()
	if dt > 0 {
		stats.EvictionsPerSec = float64(eCurr-p.prevEvict) / dt
	}
	p.prevEvict = eCurr

	// Avg evicted idle time
	evCnt := p.evictCnt.Load()
	if evCnt > 0 {
		stats.AvgEvictedIdle = float64(p.evictIdle.Load()) / float64(evCnt) / 1e9
	}

	// --- Access pattern ---
	s := p.sets.Load()
	if totalDelta > 0 {
		stats.NewKeysRate = float64(s) / float64(totalDelta)
	} else if s > 0 {
		stats.NewKeysRate = 1.0
	}
	if s > 0 {
		reads := totalDelta - int64(s)
		if reads > 0 {
			stats.ReadWriteRatio = float64(reads) / float64(s)
		}
	}

	// --- Resource pressure (from monitor) ---
	snap, ok := p.mon.Latest()
	if ok {
		if snap.CPU != nil {
			stats.CPUUtil = snap.CPU.UsagePercent / 100
		}
		if snap.Memory != nil {
			stats.MemUsageRatio = snap.Memory.UsedPercent / 100
			memDelta := stats.MemUsageRatio - p.prevMem
			if memDelta > 0 {
				stats.MemGrowthRate = memDelta / dt
			}
			p.prevMem = stats.MemUsageRatio
		}
		// Disk I/O pressure: aggregate transfer rate relative to 100 MB/s
		var totalR, totalW float64
		for _, io := range snap.IO {
			totalR += io.ReadBytesRate
			totalW += io.WriteBytesRate
		}
		stats.DiskIOPressure = clamp01((totalR + totalW) / (100 * 1024 * 1024))
		// Network utilisation relative to 1 Gbps = 125 MB/s
		var totalTx, totalRx float64
		for _, net := range snap.Network {
			totalTx += net.SendRate
			totalRx += net.RecvRate
		}
		stats.NetUtil = clamp01((totalTx + totalRx) / (125 * 1024 * 1024))
	}

	// --- Migration state ---
	stats.RehashActive = p.cache.IsRehashing()

	// --- Buffer pressure (defaults to 0; fill from real metrics when available) ---

	// --- Policy state ---
	switch p.cache.EvictionPolicy() {
	case "noop":
		stats.CurrentEvictionPolicy = PolicyNoop
	case "lru":
		stats.CurrentEvictionPolicy = PolicyLRU
	case "lfu":
		stats.CurrentEvictionPolicy = PolicyLFU
	}

	// PID setpoint deviation
	if p.pid != nil {
		stats.PIDSetpointDeviation = p.pid.Compute(stats.MemUsageRatio, dt)
	}

	return stats
}

// providerObserver implements mcache.CacheObserver and feeds counters.
type providerObserver struct {
	p *DefaultStatsProvider
}

func (o *providerObserver) OnHit(key string)  { o.p.hits.Add(1) }
func (o *providerObserver) OnMiss(key string) { o.p.misses.Add(1) }
func (o *providerObserver) OnSet(key string)  { o.p.sets.Add(1) }
func (o *providerObserver) OnDel(key string)  {} // not used by scorecard
func (o *providerObserver) OnEvict(key string) {
	o.p.evictions.Add(1)
	o.p.evictCnt.Add(1)
	// We don't have idle-time tracking from the shard; default to 0.
}
func (o *providerObserver) OnRehashStart(oldShards, newShards int) {}
func (o *providerObserver) OnRehashDone()                           {}
