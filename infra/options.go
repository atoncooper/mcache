package infra

import "time"

// Options holds infra configuration.
type Options struct {
	LoggerEnabled     bool
	PrometheusEnabled bool
	AnalyticsEnabled  bool

	logger Logger // pre-configured logger instance (set by WithLoggerInstance)

	AnalyticsBufferSize    int
	AnalyticsFlushInterval time.Duration
	AnalyticsFlushFunc     func([]Event) error
}

// Option configures the Infra observer.
type Option func(*Options)

// WithLogger enables the structured logger with the default StdoutLogger.
func WithLogger(enabled bool) Option {
	return func(o *Options) {
		o.LoggerEnabled = enabled
	}
}

// WithLoggerInstance sets a pre-configured Logger (overrides WithLogger).
func WithLoggerInstance(l Logger) Option {
	return func(o *Options) {
		o.LoggerEnabled = true
		o.logger = l
	}
}

// WithPrometheus enables Prometheus metrics export.
func WithPrometheus(enabled bool) Option {
	return func(o *Options) {
		o.PrometheusEnabled = enabled
	}
}

// WithAnalytics enables the analytics batch collector.
func WithAnalytics(enabled bool) Option {
	return func(o *Options) {
		o.AnalyticsEnabled = enabled
	}
}

// WithAnalyticsBuffer sets the analytics event buffer size.
func WithAnalyticsBuffer(size int) Option {
	return func(o *Options) {
		o.AnalyticsBufferSize = size
	}
}

// WithAnalyticsFlushInterval sets how often the analytics buffer flushes.
func WithAnalyticsFlushInterval(d time.Duration) Option {
	return func(o *Options) {
		o.AnalyticsFlushInterval = d
	}
}

// WithAnalyticsFlushFunc sets the custom flush handler for analytics events.
func WithAnalyticsFlushFunc(fn func([]Event) error) Option {
	return func(o *Options) {
		o.AnalyticsFlushFunc = fn
	}
}
