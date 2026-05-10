package monitor

// Collector is the interface for resource metric collection.
// Each implementation targets a specific resource type or data source.
type Collector interface {
	Name() string
	Collect() (*SystemSnapshot, error)
}
