package monitor

import "time"

// Options configures the Monitor.
type Options struct {
	interval   time.Duration
	capacity   int
	collectors []Collector
}

// NewOptions creates default options.
func NewOptions() Options {
	return Options{
		interval: 5 * time.Second,
		capacity: 60,
	}
}

// WithInterval returns new Options with the specified collection interval.
func (o Options) WithInterval(d time.Duration) Options {
	out := o
	out.interval = d
	return out
}

// WithCapacity returns new Options with the ring buffer capacity.
func (o Options) WithCapacity(n int) Options {
	out := o
	if n < 1 {
		n = 1
	}
	out.capacity = n
	return out
}

// WithCollectors returns new Options with the given collectors.
func (o Options) WithCollectors(c ...Collector) Options {
	out := o
	out.collectors = append([]Collector(nil), c...)
	return out
}
