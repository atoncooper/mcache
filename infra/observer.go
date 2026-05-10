package infra

import (
	"time"

	"github.com/atoncooper/mcache"
	"github.com/prometheus/client_golang/prometheus"
)

// Infra implements mcache.CacheObserver and fans out events to Logger,
// PrometheusExporter and AnalyticsCollector.
type Infra struct {
	opts      Options
	logger    Logger
	prom      *PrometheusExporter
	analytics AnalyticsCollector
}

// New creates an Infra observer. Components are enabled via Options.
func New(opts ...Option) *Infra {
	o := Options{
		LoggerEnabled:          false,
		PrometheusEnabled:      false,
		AnalyticsEnabled:       false,
		AnalyticsBufferSize:    1000,
		AnalyticsFlushInterval: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(&o)
	}

	inf := &Infra{opts: o}

	if o.LoggerEnabled {
		if o.logger != nil {
			inf.logger = o.logger
		} else {
			inf.logger = NewStdoutLogger(nil)
		}
	}
	if o.PrometheusEnabled {
		inf.prom = NewPrometheusExporter("")
	}
	if o.AnalyticsEnabled {
		inf.analytics = NewBatchCollector(
			o.AnalyticsBufferSize,
			o.AnalyticsFlushInterval,
			o.AnalyticsFlushFunc,
		)
	}
	return inf
}

// RegisterPrometheus registers the Prometheus metrics with the given registerer.
func (inf *Infra) RegisterPrometheus(reg prometheus.Registerer) error {
	if inf.prom == nil {
		return nil
	}
	return inf.prom.Register(reg)
}

// FlushAnalytics forces an immediate analytics flush.
func (inf *Infra) FlushAnalytics() error {
	if inf.analytics == nil {
		return nil
	}
	return inf.analytics.Flush()
}

// Stop halts background goroutines (analytics ticker, etc.).
func (inf *Infra) Stop() {
	if inf.analytics != nil {
		inf.analytics.Stop()
	}
}

// OnHit implements mcache.CacheObserver.
func (inf *Infra) OnHit(key string) {
	if inf.logger != nil {
		inf.logger.Info("cache hit", map[string]any{"key": key})
	}
	if inf.prom != nil {
		inf.prom.OnHit(key)
	}
	if inf.analytics != nil {
		inf.analytics.Collect(Event{Type: EventHit, Key: key, Timestamp: time.Now().UTC()})
	}
}

// OnMiss implements mcache.CacheObserver.
func (inf *Infra) OnMiss(key string) {
	if inf.logger != nil {
		inf.logger.Info("cache miss", map[string]any{"key": key})
	}
	if inf.prom != nil {
		inf.prom.OnMiss(key)
	}
	if inf.analytics != nil {
		inf.analytics.Collect(Event{Type: EventMiss, Key: key, Timestamp: time.Now().UTC()})
	}
}

// OnSet implements mcache.CacheObserver.
func (inf *Infra) OnSet(key string) {
	if inf.logger != nil {
		inf.logger.Info("cache set", map[string]any{"key": key})
	}
	if inf.prom != nil {
		inf.prom.OnSet(key)
	}
	if inf.analytics != nil {
		inf.analytics.Collect(Event{Type: EventSet, Key: key, Timestamp: time.Now().UTC()})
	}
}

// OnDel implements mcache.CacheObserver.
func (inf *Infra) OnDel(key string) {
	if inf.logger != nil {
		inf.logger.Info("cache del", map[string]any{"key": key})
	}
	if inf.prom != nil {
		inf.prom.OnDel(key)
	}
	if inf.analytics != nil {
		inf.analytics.Collect(Event{Type: EventDel, Key: key, Timestamp: time.Now().UTC()})
	}
}

// OnEvict implements mcache.CacheObserver.
func (inf *Infra) OnEvict(key string) {
	if inf.logger != nil {
		inf.logger.Warn("cache evict", map[string]any{"key": key})
	}
	if inf.prom != nil {
		inf.prom.OnEvict(key)
	}
	if inf.analytics != nil {
		inf.analytics.Collect(Event{Type: EventEvict, Key: key, Timestamp: time.Now().UTC()})
	}
}

// OnRehashStart implements mcache.CacheObserver.
func (inf *Infra) OnRehashStart(oldShards, newShards int) {
	if inf.logger != nil {
		inf.logger.Info("rehash start", map[string]any{
			"old_shards": oldShards,
			"new_shards": newShards,
		})
	}
	if inf.prom != nil {
		inf.prom.OnRehashStart(oldShards, newShards)
	}
}

// OnRehashDone implements mcache.CacheObserver.
func (inf *Infra) OnRehashDone() {
	if inf.logger != nil {
		inf.logger.Info("rehash done", nil)
	}
	if inf.prom != nil {
		inf.prom.OnRehashDone()
	}
	if inf.analytics != nil {
		inf.analytics.Collect(Event{Type: EventRehash, Timestamp: time.Now().UTC()})
	}
}

var _ mcache.CacheObserver = (*Infra)(nil)
