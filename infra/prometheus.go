package infra

import (
	"github.com/prometheus/client_golang/prometheus"
)

// PrometheusExporter exposes mcache metrics in Prometheus format.
// Register it with a prometheus.Registerer (e.g. prometheus.DefaultRegisterer).
type PrometheusExporter struct {
	hits      prometheus.Counter
	misses    prometheus.Counter
	sets      prometheus.Counter
	dels      prometheus.Counter
	evictions prometheus.Counter
	rehashes  prometheus.Counter
}

// NewPrometheusExporter creates a new exporter. Call Register() before use.
func NewPrometheusExporter(namespace string) *PrometheusExporter {
	if namespace == "" {
		namespace = "mcache"
	}
	return &PrometheusExporter{
		hits: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "hits_total",
			Help:      "Total number of cache hits.",
		}),
		misses: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "misses_total",
			Help:      "Total number of cache misses.",
		}),
		sets: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sets_total",
			Help:      "Total number of cache set operations.",
		}),
		dels: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "deletes_total",
			Help:      "Total number of cache delete operations.",
		}),
		evictions: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "evictions_total",
			Help:      "Total number of cache evictions.",
		}),
		rehashes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rehashes_total",
			Help:      "Total number of rehash operations completed.",
		}),
	}
}

// Register adds the metrics to the provided registerer.
func (p *PrometheusExporter) Register(reg prometheus.Registerer) error {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	for _, c := range []prometheus.Counter{p.hits, p.misses, p.sets, p.dels, p.evictions, p.rehashes} {
		if err := reg.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// OnHit implements the observer contract.
func (p *PrometheusExporter) OnHit(_ string) { p.hits.Inc() }

// OnMiss implements the observer contract.
func (p *PrometheusExporter) OnMiss(_ string) { p.misses.Inc() }

// OnSet implements the observer contract.
func (p *PrometheusExporter) OnSet(_ string) { p.sets.Inc() }

// OnDel implements the observer contract.
func (p *PrometheusExporter) OnDel(_ string) { p.dels.Inc() }

// OnEvict implements the observer contract.
func (p *PrometheusExporter) OnEvict(_ string) { p.evictions.Inc() }

// OnRehashStart is a no-op for counters.
func (p *PrometheusExporter) OnRehashStart(_, _ int) {}

// OnRehashDone implements the observer contract.
func (p *PrometheusExporter) OnRehashDone() { p.rehashes.Inc() }
