package mbr

import (
	"context"
	"time"

	"github.com/atoncooper/mcache"
	"github.com/atoncooper/mcache/monitor"
)

// LoopOption configures the decision loop.
type LoopOption func(*loopConfig)

type loopConfig struct {
	interval       time.Duration
	matrixCapacity int
	pid            PIDConfig
	weights        ScoreWeights
	migratorCfg    MigratorConfig
}

func defaultLoopConfig() loopConfig {
	return loopConfig{
		interval:       500 * time.Millisecond,
		matrixCapacity: 60,
		pid:            DefaultPIDConfig(),
		weights:        DefaultWeights(),
		migratorCfg:    DefaultMigratorConfig(),
	}
}

// WithInterval sets the decision-loop tick interval.
func WithInterval(d time.Duration) LoopOption {
	return func(c *loopConfig) { c.interval = d }
}

// WithMatrixCapacity sets the feature matrix capacity.
func WithMatrixCapacity(n int) LoopOption {
	return func(c *loopConfig) { c.matrixCapacity = n }
}

// WithPID sets the PID controller parameters.
func WithPID(cfg PIDConfig) LoopOption {
	return func(c *loopConfig) { c.pid = cfg }
}

// WithWeights sets the scorecard weights.
func WithWeights(w ScoreWeights) LoopOption {
	return func(c *loopConfig) { c.weights = w }
}

// WithMigratorConfig sets the migration executor parameters.
func WithMigratorConfig(cfg MigratorConfig) LoopOption {
	return func(c *loopConfig) { c.migratorCfg = cfg }
}

// RunDecisionLoop is the main decision goroutine. It periodically collects
// features, pushes them into the matrix, runs the scorecard, and emits a
// DecisionEvent on decisionCh whenever the decision changes.
func RunDecisionLoop(
	ctx context.Context,
	provider StatsProvider,
	matrix *FeatureMatrix,
	decisionCh chan<- DecisionEvent,
	opts ...LoopOption,
) {
	cfg := defaultLoopConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()

	var prevDecision Decision

	for {
		select {
		case <-ticker.C:
			stats := provider.GetLatestStats()
			matrix.Push(stats)

			decision, score := Decide(stats, matrix, cfg.weights)

			if decision != prevDecision {
				event := DecisionEvent{
					Decision:    decision,
					Score:       score,
					Timestamp:   time.Now(),
					WindowStats: stats,
				}
				select {
				case decisionCh <- event:
					prevDecision = decision
				default:
					// Channel full; drop this event to avoid blocking the loop.
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

// RunMigrationExecutor consumes DecisionEvents and executes incremental
// migrations. It feeds migration state back into the provider so the
// decision engine can apply suppression.
func RunMigrationExecutor(
	ctx context.Context,
	decisionCh <-chan DecisionEvent,
	cache *mcache.Cache,
	mon *monitor.Monitor,
	provider *DefaultStatsProvider,
	cfg MigratorConfig,
) {
	executor := NewMigrationExecutor(cache, mon, cfg)

	for {
		select {
		case event := <-decisionCh:
			if event.Decision != DecisionMigrate {
				continue
			}
			// Skip if already migrating (suppression factor handles this).
			if cache.IsRehashing() || executor.IsActive() {
				continue
			}

			// Run migration (blocks until complete or ctx cancelled).
			_ = executor.Execute(ctx)

		case <-ctx.Done():
			return
		}
	}
}
