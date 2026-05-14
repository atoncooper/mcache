package mbr

import (
	"testing"
	"time"
)

// --- FeatureMatrix ---

func TestFeatureMatrix_PushGetRecent(t *testing.T) {
	m := NewFeatureMatrix(10)

	// Push 5 windows
	for i := 0; i < 5; i++ {
		m.Push(WindowStats{MemUsageRatio: float64(i) / 10})
	}

	recent := m.GetRecent(3)
	if len(recent) != 3 {
		t.Fatalf("expected 3, got %d", len(recent))
	}
	// Should be windows 2,3,4
	if recent[0].MemUsageRatio != 0.2 {
		t.Errorf("idx 0: expected 0.2, got %f", recent[0].MemUsageRatio)
	}
	if recent[2].MemUsageRatio != 0.4 {
		t.Errorf("idx 2: expected 0.4, got %f", recent[2].MemUsageRatio)
	}

	if m.Len() != 5 {
		t.Errorf("expected len 5, got %d", m.Len())
	}
}

func TestFeatureMatrix_WrapAround(t *testing.T) {
	m := NewFeatureMatrix(3)

	// Push 5 windows — wrap around after 3
	for i := 0; i < 5; i++ {
		m.Push(WindowStats{MemUsageRatio: float64(i)})
	}

	if m.Len() != 3 {
		t.Errorf("expected len 3 (full), got %d", m.Len())
	}

	// Recent 3 should be windows 2,3,4
	recent := m.GetRecent(3)
	if len(recent) != 3 {
		t.Fatalf("expected 3, got %d", len(recent))
	}
	if recent[0].MemUsageRatio != 2.0 {
		t.Errorf("expected 2.0, got %f", recent[0].MemUsageRatio)
	}
	if recent[2].MemUsageRatio != 4.0 {
		t.Errorf("expected 4.0, got %f", recent[2].MemUsageRatio)
	}
}

func TestFeatureMatrix_Last(t *testing.T) {
	m := NewFeatureMatrix(5)
	_, ok := m.Last()
	if ok {
		t.Error("expected false for empty matrix")
	}

	m.Push(WindowStats{MemUsageRatio: 0.5})
	s, ok := m.Last()
	if !ok || s.MemUsageRatio != 0.5 {
		t.Error("Last() returned wrong value")
	}
}

func TestFeatureMatrix_Concurrency(t *testing.T) {
	m := NewFeatureMatrix(100)
	done := make(chan bool)
	go func() {
		for i := 0; i < 1000; i++ {
			m.Push(WindowStats{MemUsageRatio: float64(i)})
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 1000; i++ {
			m.GetRecent(10)
			m.Len()
			m.Last()
		}
		done <- true
	}()
	<-done
	<-done
}

// --- PID ---

func TestPID_Compute(t *testing.T) {
	pid := NewPIDController(PIDConfig{
		Kp: 1.0, Ki: 0.0, Kd: 0.0,
		Setpoint: 0.6, Min: -1.0, Max: 1.0,
	})

	// Below setpoint → positive output (need more)
	out := pid.Compute(0.5, 0.5)
	if out <= 0 {
		t.Errorf("expected positive output when below setpoint, got %f", out)
	}

	// Above setpoint → negative output (need less)
	out = pid.Compute(0.9, 0.5)
	if out >= 0 {
		t.Errorf("expected negative output when above setpoint, got %f", out)
	}
}

func TestPID_Clamping(t *testing.T) {
	pid := NewPIDController(PIDConfig{
		Kp: 10.0, Ki: 10.0, Kd: 0.0,
		Setpoint: 0.6, Min: -0.5, Max: 0.5,
	})

	// Extreme deviation should be clamped
	out := pid.Compute(0.0, 0.5)
	if out > 0.5 || out < -0.5 {
		t.Errorf("output %f exceeds clamp bounds", out)
	}
}

// --- Scorecard per-factor functions ---

func TestMemGrowthScore(t *testing.T) {
	if s := memGrowthScore(-0.1); s != 0 {
		t.Errorf("negative growth should be 0, got %f", s)
	}
	if s := memGrowthScore(0.0); s != 0 {
		t.Errorf("zero growth should be 0, got %f", s)
	}
	if s := memGrowthScore(0.03); s <= 0 || s >= 1 {
		t.Errorf("mid growth should be in (0,1), got %f", s)
	}
	if s := memGrowthScore(0.05); s != 1 {
		t.Errorf("high growth should be 1, got %f", s)
	}
}

func TestHitRateScore(t *testing.T) {
	if s := hitRateScore(0.95); s != 0 {
		t.Errorf("high hit rate should be 0, got %f", s)
	}
	if s := hitRateScore(0.70); s != 1 {
		t.Errorf("low hit rate should be 1, got %f", s)
	}
	if s := hitRateScore(0.60); s != 1 {
		t.Errorf("very low hit rate should be 1, got %f", s)
	}
}

func TestNewKeysScore(t *testing.T) {
	if s := newKeysScore(0.5); s != 1 {
		t.Errorf("high new keys should be 1, got %f", s)
	}
	if s := newKeysScore(0.1); s != 0 {
		t.Errorf("low new keys should be 0, got %f", s)
	}
}

func TestEvictionPressureScore(t *testing.T) {
	if s := evictionPressureScore(100); s != 1 {
		t.Errorf("high eviction should be 1, got %f", s)
	}
	if s := evictionPressureScore(10); s != 0 {
		t.Errorf("low eviction should be 0, got %f", s)
	}
}

func TestBufferPenalty_Normal(t *testing.T) {
	s := WindowStats{}
	p := bufferPenalty(s)
	if p != 0 {
		t.Errorf("normal stats should have 0 penalty, got %f", p)
	}
}

func TestBufferPenalty_FullClientOutput(t *testing.T) {
	s := WindowStats{ClientOutputBufferUsage: 1.0}
	p := bufferPenalty(s)
	if p == 0 {
		t.Error("full client output buffer should produce penalty")
	}
}

func TestBufferPenalty_LargeAOFBuffer(t *testing.T) {
	s := WindowStats{AofRewriteBufferSize: 1024 * 1024 * 1024} // 1 GB
	p := bufferPenalty(s)
	if p == 0 {
		t.Error("large AOF buffer should produce penalty")
	}
}

func TestBufferPenalty_LargeInputClients(t *testing.T) {
	s := WindowStats{LargeInputBufClients: 5}
	p := bufferPenalty(s)
	if p == 0 {
		t.Error("large input buffer clients should produce penalty")
	}
}

func TestIsFalsePressure(t *testing.T) {
	if isFalsePressure(WindowStats{}) {
		t.Error("empty stats should not be false pressure")
	}
	if !isFalsePressure(WindowStats{ClientOutputBufferUsage: 0.9}) {
		t.Error("high client output buffer should be false pressure")
	}
	if !isFalsePressure(WindowStats{AofRewriteBufferSize: 1024 * 1024 * 1024}) {
		t.Error("large AOF buffer should be false pressure")
	}
}

// --- Decide ---

func TestDecide_EvictWhenLowPressure(t *testing.T) {
	matrix := NewFeatureMatrix(10)
	stats := WindowStats{
		MemUsageRatio:  0.2,
		HitRate:        0.95,
		NewKeysRate:    0.05,
		EvictionsPerSec: 5,
	}
	d, score := Decide(stats, matrix, DefaultWeights())
	if d != DecisionEvict {
		t.Errorf("expected EVICT, got %s (score=%.3f)", d, score)
	}
}

func TestDecide_MigrateWhenHighPressure(t *testing.T) {
	matrix := NewFeatureMatrix(10)
	// Fill matrix with high-pressure windows for consecutive confirmation
	for i := 0; i < 5; i++ {
		matrix.Push(WindowStats{
			MemUsageRatio:   0.9,
			MemGrowthRate:   0.5,
			HitRate:         0.5,
			NewKeysRate:     0.6,
			EvictionsPerSec: 150,
		})
	}

	stats := WindowStats{
		MemUsageRatio:   0.9,
		MemGrowthRate:   0.5,
		HitRate:         0.5,
		NewKeysRate:     0.6,
		EvictionsPerSec: 150,
	}
	d, score := Decide(stats, matrix, DefaultWeights())
	if d != DecisionMigrate {
		t.Errorf("expected MIGRATE with high pressure, got %s (score=%.3f)", d, score)
	}
	_ = score
}

func TestDecide_RehashSuppression(t *testing.T) {
	matrix := NewFeatureMatrix(10)
	for i := 0; i < 5; i++ {
		matrix.Push(WindowStats{
			MemUsageRatio:   0.9,
			MemGrowthRate:   0.5,
			HitRate:         0.5,
			NewKeysRate:     0.6,
			EvictionsPerSec: 150,
		})
	}

	stats := WindowStats{
		MemUsageRatio:   0.9,
		MemGrowthRate:   0.5,
		HitRate:         0.5,
		NewKeysRate:     0.6,
		EvictionsPerSec: 150,
		RehashActive:    true, // should suppress
	}
	d, _ := Decide(stats, matrix, DefaultWeights())
	if d != DecisionEvict {
		t.Errorf("expected EVICT when rehash active, got %s", d)
	}
}

func TestDecide_BufferPenalty(t *testing.T) {
	matrix := NewFeatureMatrix(10)
	for i := 0; i < 5; i++ {
		matrix.Push(WindowStats{
			MemUsageRatio:   0.9,
			MemGrowthRate:   0.5,
			HitRate:         0.5,
			NewKeysRate:     0.6,
			EvictionsPerSec: 150,
		})
	}

	stats := WindowStats{
		MemUsageRatio:           0.9,
		MemGrowthRate:           0.5,
		HitRate:                 0.5,
		NewKeysRate:             0.6,
		EvictionsPerSec:        150,
		ClientOutputBufferUsage: 0.95, // false pressure
	}
	d, _ := Decide(stats, matrix, DefaultWeights())
	if d != DecisionEvict {
		t.Errorf("expected EVICT with buffer penalty, got %s", d)
	}
}

func TestDecide_ConsecutiveWindow(t *testing.T) {
	// Single strong window (>0.65) — triggers migration without full history.
	matrix := NewFeatureMatrix(10)
	matrix.Push(WindowStats{
		MemUsageRatio:   0.9,
		MemGrowthRate:   0.5,
		HitRate:         0.5,
		NewKeysRate:     0.6,
		EvictionsPerSec: 150,
	})

	stats := WindowStats{
		MemUsageRatio:   0.9,
		MemGrowthRate:   0.5,
		HitRate:         0.5,
		NewKeysRate:     0.6,
		EvictionsPerSec: 150,
	}
	d, _ := Decide(stats, matrix, DefaultWeights())
	if d != DecisionMigrate {
		t.Errorf("expected MIGRATE with single strong window, got %s", d)
	}

	// Weak window (score ≤0.65) — should stay EVICT.
	weakStats := WindowStats{
		MemUsageRatio: 0.1,
		HitRate:       1.0,
		NewKeysRate:   0.0,
	}
	matrix2 := NewFeatureMatrix(10)
	d2, _ := Decide(weakStats, matrix2, DefaultWeights())
	if d2 != DecisionEvict {
		t.Errorf("expected EVICT with weak window, got %s", d2)
	}
}

// --- Migrator ---

func TestNextPowerOfTwo(t *testing.T) {
	cases := []struct{ in, want int }{
		{1, 1}, {2, 2}, {3, 4}, {4, 4},
		{5, 8}, {8, 8}, {9, 16}, {15, 16},
		{16, 16}, {17, 32}, {100, 128}, {1000, 1024},
	}
	for _, c := range cases {
		got := nextPowerOfTwo(c.in)
		if got != c.want {
			t.Errorf("nextPowerOfTwo(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestMigrator_CalculateTargetShards(t *testing.T) {
	e := &IncrementalMigrationExecutor{
		cfg: DefaultMigratorConfig(),
	}
	// 1024 keys / 512 per shard = 2 → next power of two = 2
	// but MinShards is 4, so result is 4
	s := e.calculateTargetShards(1024)
	if s != 4 {
		t.Errorf("expected 4, got %d", s)
	}

	// 4096 keys / 512 = 8 → power of two = 8
	s = e.calculateTargetShards(4096)
	if s != 8 {
		t.Errorf("expected 8, got %d", s)
	}

	// 100000 keys → floor/512 = 195 → next power of two = 256
	s = e.calculateTargetShards(100000)
	if s != 256 {
		t.Errorf("expected 256, got %d", s)
	}
}

func TestMigrator_ShouldPause(t *testing.T) {
	e := &IncrementalMigrationExecutor{
		cfg: MigratorConfig{
			PauseOnCPUThreshold: 0.80,
			PauseOnMemThreshold: 0.85,
		},
	}
	// No monitor data → should not pause
	if e.shouldPause() {
		t.Error("should not pause when no monitor data")
	}
}

func TestDecisionEvent_Fields(t *testing.T) {
	event := DecisionEvent{
		Decision:  DecisionMigrate,
		Score:     0.85,
		Timestamp: time.Now(),
	}
	if event.Decision != DecisionMigrate {
		t.Error("wrong decision")
	}
	if event.Score != 0.85 {
		t.Error("wrong score")
	}
}
