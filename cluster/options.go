package cluster

import "time"

// Options holds cluster configuration.
type Options struct {
	Mode                string
	Nodes               []NodeConfig
	Sentinels           []string
	Master              string
	Slaves              []string
	HealthCheckInterval time.Duration
	HealthCheckTimeout  time.Duration
	FailoverTimeout     time.Duration
	ErrorLog            func(format string, v ...any) // optional; if nil, silent
}

// logf writes a formatted message to the error log, if configured.
func (o Options) logf(format string, v ...any) {
	if o.ErrorLog != nil {
		o.ErrorLog(format, v...)
	}
}

// NodeConfig describes a single cache node.
type NodeConfig struct {
	Addr     string `yaml:"addr"`
	Weight   int    `yaml:"weight"`
	IsMaster bool   `yaml:"is_master"`
}

// Option configures a ClusterManager.
type Option func(*Options)

// WithMode sets the cluster mode: "shard", "sentinel", "master_slave".
func WithMode(mode string) Option {
	return func(o *Options) {
		o.Mode = mode
	}
}

// WithNodes sets the node list (used by shard mode).
func WithNodes(nodes []NodeConfig) Option {
	return func(o *Options) {
		o.Nodes = nodes
	}
}

// WithSentinels sets sentinel addresses (used by sentinel mode).
func WithSentinels(addrs []string) Option {
	return func(o *Options) {
		o.Sentinels = addrs
	}
}

// WithMaster sets the master address (used by master-slave mode).
func WithMaster(addr string) Option {
	return func(o *Options) {
		o.Master = addr
	}
}

// WithSlaves sets the slave addresses (used by master-slave mode).
func WithSlaves(addrs []string) Option {
	return func(o *Options) {
		o.Slaves = addrs
	}
}

// WithHealthCheckInterval sets how often nodes are health-checked.
func WithHealthCheckInterval(d time.Duration) Option {
	return func(o *Options) {
		o.HealthCheckInterval = d
	}
}

// WithHealthCheckTimeout sets the timeout for a single health check probe.
func WithHealthCheckTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.HealthCheckTimeout = d
	}
}

// WithFailoverTimeout sets how long to wait before triggering failover.
func WithFailoverTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.FailoverTimeout = d
	}
}

// WithErrorLog sets the error logging function for cluster operations.
func WithErrorLog(fn func(format string, v ...any)) Option {
	return func(o *Options) {
		o.ErrorLog = fn
	}
}

func defaultOptions() Options {
	return Options{
		Mode:                "shard",
		HealthCheckInterval: 5 * time.Second,
		HealthCheckTimeout:  2 * time.Second,
		FailoverTimeout:     10 * time.Second,
	}
}
