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

// Decide runs the weighted scorecard against the current stats and recent
// history. It returns the decision and the final computed score.
func Decide(stats WindowStats, matrix *FeatureMatrix, weights ScoreWeights) (Decision, float64) {
	// --- Step 1: per-factor scores ---
	memScore := memGrowthScore(stats.MemGrowthRate)
	hitScore := hitRateScore(stats.HitRate)
	nkScore  := newKeysScore(stats.NewKeysRate)
	evScore  := evictionPressureScore(stats.EvictionsPerSec)
	bufPen   := bufferPenalty(stats)

	// --- Step 2: false-pressure suppression ---
	falsePressure := isFalsePressure(stats)

	// --- Step 3: weighted raw score ---
	raw := weights.MemGrowth*memScore +
		weights.HitRate*hitScore +
		weights.NewKeys*nkScore +
		weights.EvictionPressure*evScore +
		weights.BufferPenalty*bufPen

	// Clamp to [0, 1]
	raw = clamp01(raw)

	// --- Step 3b: false-pressure suppression ---
	if falsePressure {
		raw *= 0.4 // strong dampening: buffer pressure ≠ hot-data growth
	}

	// --- Step 4: migration suppression ---
	final := raw
	if stats.RehashActive || stats.MigrationActive {
		final = raw * 0.3
	}

	// --- Step 5: consecutive-window confirmation ---
	if final > 0.7 {
		recent := matrix.GetRecent(3)
		if len(recent) >= 3 {
			s0 := recomputeScore(recent[0], weights)
			s1 := recomputeScore(recent[1], weights)
			if s0 > 0.7 && s1 > 0.7 {
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
	nkScore  := newKeysScore(stats.NewKeysRate)
	evScore  := evictionPressureScore(stats.EvictionsPerSec)
	bufPen   := bufferPenalty(stats)
	if isFalsePressure(stats) {
		bufPen = -1.0
	}
	raw := weights.MemGrowth*memScore +
		weights.HitRate*hitScore +
		weights.NewKeys*nkScore +
		weights.EvictionPressure*evScore +
		weights.BufferPenalty*bufPen
	raw = clamp01(raw)
	if isFalsePressure(stats) {
		raw *= 0.4
	}
	if stats.RehashActive || stats.MigrationActive {
		raw *= 0.3
	}
	return raw
}

// --- Per-factor scoring functions (all return [0, 1]) ---

// memGrowthScore: high growth rate → high score (favour migration).
// MemGrowthRate is a fraction (mem usage ratio change per second).
// <= 0 → 0; >= 0.3 → 1; linear in between.
func memGrowthScore(rate float64) float64 {
	if rate <= 0 {
		return 0
	}
	if rate >= 0.3 {
		return 1
	}
	return rate / 0.3
}

// hitRateScore: low hit rate → high score (cache is inefficient, need more shards).
// >= 0.95 → 0; <= 0.7 → 1; linear in between.
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
	return math.Max(0, math.Min(1, v))
}
