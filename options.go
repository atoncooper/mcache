package mcache

import "time"

// Options holds cache configuration. Use NewOptions to create and With* methods to customize.
type Options struct {
	shardCount     int
	maxSize        int
	defaultTTL     time.Duration
	evictionPolicy string
	rehasher       string
	observer       CacheObserver
}

// NewOptions creates default options.
func NewOptions() Options {
	return Options{
		shardCount:     16,
		maxSize:        0,
		defaultTTL:     0,
		evictionPolicy: "lru",
		rehasher:       "incremental",
	}
}

// WithShards returns new Options with specified shard count. Must be power of two.
func (o Options) WithShards(count int) Options {
	out := o
	out.shardCount = count
	return out
}

// WithMaxSize returns new Options with size limit (0 = unlimited).
func (o Options) WithMaxSize(size int) Options {
	out := o
	out.maxSize = size
	return out
}

// WithDefaultTTL returns new Options with default TTL.
func (o Options) WithDefaultTTL(ttl time.Duration) Options {
	out := o
	out.defaultTTL = ttl
	return out
}

// WithEvictionPolicy returns new Options with the given eviction policy name.
// Available built-in: "noop", "lru", "lfu".
func (o Options) WithEvictionPolicy(name string) Options {
	out := o
	out.evictionPolicy = name
	return out
}

// WithObserver returns new Options with the given cache observer.
func (o Options) WithObserver(obs CacheObserver) Options {
	out := o
	out.observer = obs
	return out
}

// WithRehasher returns new Options with the given rehasher name.
// Available built-in: "incremental", "batch", "noop".
func (o Options) WithRehasher(name string) Options {
	out := o
	out.rehasher = name
	return out
}
