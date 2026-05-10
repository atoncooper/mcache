package mbr

import "time"

// Options bundles all MBR configuration.
type Options struct {
	MatrixCapacity   int
	DecisionInterval time.Duration
	PID              PIDConfig
	Weights          ScoreWeights
	Migration        MigratorConfig
}

// NewOptions returns default MBR options.
func NewOptions() Options {
	return Options{
		MatrixCapacity:   60,
		DecisionInterval: 500 * time.Millisecond,
		PID:              DefaultPIDConfig(),
		Weights:          DefaultWeights(),
		Migration:        DefaultMigratorConfig(),
	}
}

// WithMatrixCapacity returns new Options with the given matrix capacity.
func (o Options) WithMatrixCapacity(n int) Options {
	out := o
	if n < 1 {
		n = 1
	}
	out.MatrixCapacity = n
	return out
}

// WithDecisionInterval returns new Options with the given interval.
func (o Options) WithDecisionInterval(d time.Duration) Options {
	out := o
	out.DecisionInterval = d
	return out
}

// WithPID returns new Options with the given PID config.
func (o Options) WithPID(cfg PIDConfig) Options {
	out := o
	out.PID = cfg
	return out
}

// WithWeights returns new Options with the given scorecard weights.
func (o Options) WithWeights(w ScoreWeights) Options {
	out := o
	out.Weights = w
	return out
}

// WithMigration returns new Options with the given migrator config.
func (o Options) WithMigration(cfg MigratorConfig) Options {
	out := o
	out.Migration = cfg
	return out
}
