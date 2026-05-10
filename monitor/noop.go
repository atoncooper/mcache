package monitor

// NoopCollector is a no-op collector that always returns an empty snapshot.
// Useful as a placeholder or on platforms without native support.
type NoopCollector struct{}

// NewNoop returns a no-op collector.
func NewNoop() *NoopCollector {
	return &NoopCollector{}
}

func (c *NoopCollector) Name() string                { return "noop" }
func (c *NoopCollector) Collect() (*SystemSnapshot, error) {
	return &SystemSnapshot{}, nil
}
