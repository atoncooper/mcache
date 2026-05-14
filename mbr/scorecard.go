package mbr

import "math"

// ScoreWeights controls the relative importance of each factor in the scorecard.
type ScoreWeights struct {
	MemGrowth        float64
	HitRate          float64
	NewKeys          float64
	EvictionPressure float64
	BufferPenalty    float64
}

// DefaultWeights returns the preset weight values.
func DefaultWeights() ScoreWeights {
	return ScoreWeights{
		MemGrowth:        0.35,
		HitRate:          0.25,
		NewKeys:          0.20,
		EvictionPressure: 0.15,
		BufferPenalty:    0.05,
	}
}

const (
	// migrateThreshold is the minimum score to consider a migration decision.
	migrateThreshold = 0.55
	// consecutiveWindows is the number of recent windows that must all exceed
	// the threshold before a migration is triggered (prevents flapping).
	consecutiveWindows = 2
	// falsePressureFactor dampens the score when buffer pressure is the
	// dominant cause of high memory usage rather than genuine hot-data growth.
	falsePressureFactor = 0.6
	// migrationSuppressionFactor strongly dampens the score while a migration
	// or rehash is already in progress.
	migrationSuppressionFactor = 0.3
)

// Decide runs the weighted scorecard against the current stats and recent
// history. It returns the decision and the final computed score.
func Decide(stats WindowStats, matrix *FeatureMatrix, weights ScoreWeights) (Decision, float64) {
	// --- Step 1: per-factor scores ---
	memScore := memGrowthScore(stats.MemGrowthRate)
	hitScore := hitRateScore(stats.HitRate)
	nkScore := newKeysScore(stats.NewKeysRate)
	evScore := evictionPressureScore(stats.EvictionsPerSec)
	keyScore := keysGrowthScore(stats.KeysGrowthRate)
	bufPen := bufferPenalty(stats)

	// --- Step 2: false-pressure suppression ---
	falsePressure := isFalsePressure(stats)

	// --- Step 3: weighted raw score ---
	// Blend memGrowth and keysGrowth: use the stronger signal as the primary
	// growth indicator so that write-heavy workloads (high KeysGrowthRate)
	// can trigger migration even when the system memory monitor updates slowly.
	growthScore := math.Max(memScore, keyScore*0.8)

	raw := weights.MemGrowth*growthScore +
		weights.HitRate*hitScore +
		weights.NewKeys*nkScore +
		weights.EvictionPressure*evScore +
		weights.BufferPenalty*bufPen

	// Clamp to [0, 1]
	raw = clamp01(raw)

	// --- Step 3b: false-pressure suppression ---
	if falsePressure {
		raw *= falsePressureFactor
	}

	// --- Step 4: migration suppression ---
	final := raw
	if stats.RehashActive || stats.MigrationActive {
		final = raw * migrationSuppressionFactor
	}

	// --- Step 5: consecutive-window confirmation ---
	if final > migrateThreshold {
		recent := matrix.GetRecent(consecutiveWindows + 1)
		if len(recent) >= consecutiveWindows+1 {
			ok := true
			for i := 0; i < consecutiveWindows; i++ {
				s := recomputeScore(recent[i], weights)
				if s <= migrateThreshold {
					ok = false
					break
				}
			}
			if ok {
				return DecisionMigrate, final
			}
		} else if len(recent) >= 1 {
			// Not enough history yet; allow migration if the single current
			// window is strongly above threshold.
			if final > migrateThreshold+0.10 {
				return DecisionMigrate, final
			}
		}
	}

	return DecisionEvict, final
}

// recomputeScore applies the same scoring logic to a historical snapshot.
// It does NOT recurse into matrix — that would cause infinite loops.
func recomputeScore(stats WindowStats, weights ScoreWeights) float64 {
	memScore := memGrowthScore(stats.MemGrowthRate)
	hitScore := hitRateScore(stats.HitRate)
	nkScore := newKeysScore(stats.NewKeysRate)
	evScore := evictionPressureScore(stats.EvictionsPerSec)
	keyScore := keysGrowthScore(stats.KeysGrowthRate)
	bufPen := bufferPenalty(stats)
	if isFalsePressure(stats) {
		bufPen = -1.0
	}

	growthScore := math.Max(memScore, keyScore*0.8)

	raw := weights.MemGrowth*growthScore +
		weights.HitRate*hitScore +
		weights.NewKeys*nkScore +
		weights.EvictionPressure*evScore +
		weights.BufferPenalty*bufPen
	raw = clamp01(raw)
	if isFalsePressure(stats) {
		raw *= falsePressureFactor
	}
	if stats.RehashActive || stats.MigrationActive {
		raw *= migrationSuppressionFactor
	}
	return raw
}

// --- Per-factor scoring functions (all return [0, 1]) ---

// memGrowthScore: high growth rate → high score (favour migration).
// MemGrowthRate is the memory usage ratio change per second.
// Thresholds are calibrated for rates computed over multi-second intervals.
// <= 0 → 0; >= 0.05 → 1; linear in between.
func memGrowthScore(rate float64) float64 {
	if rate <= 0 {
		return 0
	}
	if rate >= 0.05 {
		return 1
	}
	return rate / 0.05
}

// hitRateScore: low hit rate → high score (cache is inefficient, need more shards).
// >= 0.95 → 0; <= 0.70 → 1; linear in between.
func hitRateScore(rate float64) float64 {
	if rate >= 0.95 {
		return 0
	}
	if rate <= 0.70 {
		return 1
	}
	return (0.95 - rate) / 0.25
}

// newKeysScore: high new-key rate → high score (more shards needed).
// >= 0.5 → 1; <= 0.1 → 0; linear in between.
func newKeysScore(rate float64) float64 {
	if rate >= 0.5 {
		return 1
	}
	if rate <= 0.1 {
		return 0
	}
	return (rate - 0.1) / 0.4
}

// evictionPressureScore: high eviction rate → high score (LRU struggling).
// >= 100 evictions/s → 1; <= 10 → 0; linear in between.
func evictionPressureScore(eps float64) float64 {
	if eps >= 100 {
		return 1
	}
	if eps <= 10 {
		return 0
	}
	return (eps - 10) / 90
}

// keysGrowthScore: high key-count growth → high score (cache filling rapidly).
// This is an alternative growth signal that does not depend on the system
// memory monitor, so it is always up-to-date.
// >= 500 keys/s → 1; <= 0 → 0; linear in between.
func keysGrowthScore(rate float64) float64 {
	if rate <= 0 {
		return 0
	}
	if rate >= 500 {
		return 1
	}
	return rate / 500
}

// bufferPenalty returns a negative score when buffers are stressed.
// This reflects "false pressure": high memory use caused by buffers rather
// than hot data that would benefit from migration.
// Returns a value in [-1, 0].
func bufferPenalty(stats WindowStats) float64 {
	score := 0.0

	// Client output buffer
	if stats.ClientOutputBufferUsage > 0.8 {
		score -= (stats.ClientOutputBufferUsage - 0.8) / 0.2 // up to -1
	}
	// Replication backlog
	if stats.ReplBacklogUsage > 0.8 {
		score -= (stats.ReplBacklogUsage - 0.8) / 0.2
	}
	// AOF rewrite buffer (threshold: 512 MB)
	aofMB := stats.AofRewriteBufferSize / (1024 * 1024)
	if aofMB > 512 {
		over := (aofMB - 512) / 512 // normalised excess
		if over > 1 {
			over = 1
		}
		score -= over
	}
	// Large input buffer clients
	if stats.LargeInputBufClients > 0 {
		c := float64(stats.LargeInputBufClients)
		if c > 10 {
			c = 10
		}
		score -= c / 10
	}

	// Clamp to [-1, 0]
	if score < -1 {
		score = -1
	}
	return score
}

// isFalsePressure returns true when buffer pressure is the dominant cause of
// high memory usage rather than genuine hot-data growth.
func isFalsePressure(stats WindowStats) bool {
	if stats.ClientOutputBufferUsage > 0.80 {
		return true
	}
	if stats.AofRewriteBufferSize > 512*1024*1024 {
		return true
	}
	if stats.LargeInputBufClients > 0 {
		return true
	}
	return false
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
