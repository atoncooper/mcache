package mcache

import "time"

// Options holds SDK client configuration.
type Options struct {
	Addrs        []string
	PoolSize     int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	Codec        Codec
}

// Option configures a Client.
type Option func(*Options)

func defaultOptions() Options {
	return Options{
		PoolSize:     4,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 5 * time.Second,
		Codec:        RawCodec{},
	}
}

// WithPoolSize sets the number of TCP connections per node.
func WithPoolSize(n int) Option {
	return func(o *Options) {
		o.PoolSize = n
	}
}

// WithDialTimeout sets the TCP dial timeout.
func WithDialTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.DialTimeout = d
	}
}

// WithReadTimeout sets the response read timeout.
func WithReadTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.ReadTimeout = d
	}
}

// WithWriteTimeout sets the request write timeout.
func WithWriteTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.WriteTimeout = d
	}
}

// WithCodec sets a custom serialization codec.
func WithCodec(c Codec) Option {
	return func(o *Options) {
		o.Codec = c
	}
}

// WithAddrs sets multiple server addresses for cluster mode.
func WithAddrs(addrs ...string) Option {
	return func(o *Options) {
		o.Addrs = addrs
	}
}
