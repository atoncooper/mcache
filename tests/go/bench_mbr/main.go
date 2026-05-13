// Package main tests MBR (Multi-dimensional matrix Based intelligent scheduling)
// against Redis on dimensions Redis cannot compete: adaptive shard scaling,
// oscillation resistance, and recovery under pressure.
//
// Tests:
//   --test=scale    Adaptive shard scaling under increasing load
//   --test=oscillate Oscillation resistance under fluctuating load
//   --test=recovery  Recovery time after migration
//
// Usage:
//   go run ./bench_mbr --mcache :11211 --test=scale
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sdk "github.com/atoncooper/mcache/sdk/go"
	mnet "github.com/atoncooper/mcache/net"
	"github.com/redis/go-redis/v9"
)

func main() {
	mcacheAddr := flag.String("mcache", "127.0.0.1:11211", "mcache address")
	redisAddr := flag.String("redis", "127.0.0.1:6379", "Redis address")
	testName := flag.String("test", "scale", "test: scale, oscillate, recovery")
	keySize := flag.Int("key-size", 64, "key size in bytes")
	valSize := flag.Int("val-size", 1024, "value size in bytes (use 524288 for 512KB)")
	flag.Parse()

	switch *testName {
	case "oscillate":
		runOscillate(*mcacheAddr, *redisAddr, *keySize, *valSize)
	case "recovery":
		runRecovery(*mcacheAddr, *keySize, *valSize)
	default:
		runScale(*mcacheAddr, *redisAddr, *keySize, *valSize)
	}
}

// ---------------------------------------------------------------------------
// Test 1: Adaptive Shard Scaling
// Starts with low capacity (4 shards), ramps load to trigger MBR migration.
// MBR should detect pressure → expand shards → capacity increases → entries grow.
// Redis has fixed capacity — once full, entries plateau.
// ---------------------------------------------------------------------------

func runScale(mcacheAddr, redisAddr string, keySize, valSize int) {

	fmt.Println(strings.Repeat("=", 100))
	fmt.Println("  MBR Adaptive Shard Scaling Test")
	fmt.Println("  Starts with cache.shards=4, MBR expands under load (if enabled)")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("  key=%dB val=%dB  |  sampling stats every 500ms\n\n", keySize, valSize)

	// Config check: warn if MBR appears disabled
	initialStats := fetchStats(mcacheAddr)
	initialEntries := parseEntryCount(initialStats)
	if initialEntries < 0 {
		fmt.Println("  ERROR: mcache unreachable. Start server first.")
		return
	}

	fmt.Printf("  %-8s │ %10s │ %10s │ %10s │ %10s │ %10s\n",
		"time", "entries", "mem", "ops/s", "reqs", "note")
	fmt.Println(strings.Repeat("─", 100))

	// Start continuous load goroutines
	stopCh := make(chan struct{})
	var totalOps atomic.Int64

	mc, _ := sdk.NewClient(mcacheAddr, sdk.WithPoolSize(32), sdk.WithCodec(sdk.RawCodec{}))
	if mc == nil {
		fmt.Println("  ERROR: mcache connect failed")
		return
	}
	defer mc.Close()

	// Ramp: start with moderate load, increase over time
	for phase := 0; phase < 8; phase++ {
		goroutines := 16 * (phase + 1) // 16 → 128 goroutines
		phaseDur := 15 * time.Second
		var wg sync.WaitGroup

		for g := 0; g < goroutines; g++ {
			wg.Add(1)
			go func(gid int) {
				defer wg.Done()
				rng := rand.New(rand.NewSource(int64(gid) + time.Now().UnixNano()))
				val := fillBytes(make([]byte, valSize))
				keySpace := 100000 // large key space to create pressure
				for {
					select {
					case <-stopCh:
						return
					default:
					}
					k := genKey(nil, rng.Intn(keySpace), keySize)
					_ = mc.Set(k, val, 0)
					totalOps.Add(1)
				}
			}(g)
		}

		// Sample stats during this phase
		prevReqs := parseTotalReqs(fetchStats(mcacheAddr))
		sampleEnd := time.Now().Add(phaseDur)
		for time.Now().Before(sampleEnd) {
			time.Sleep(500 * time.Millisecond)
			stats := fetchStats(mcacheAddr)
			entries := parseEntryCount(stats)
			mem := parseCacheMemory(stats)
			reqs := parseTotalReqs(stats)
			ops := float64(reqs-prevReqs) / 0.5
			prevReqs = reqs

			note := ""
			if entries > initialEntries*2 && initialEntries > 0 {
				note = "⚡ MBR may have expanded shards!"
			}

			fmt.Printf("  %8s │ %10d │ %8s │ %10.0f │ %10d │ %s\n",
				time.Now().Format("15:04:05"), entries,
				formatBytes(int(mem)), ops, reqs, note)
		}

		// Stop goroutines for this phase
		close(stopCh)
		wg.Wait()
		stopCh = make(chan struct{})

		// Print phase summary
		stats := fetchStats(mcacheAddr)
		entries := parseEntryCount(stats)
		fmt.Printf("  --- phase %d done (g=%d): entries=%d ---\n\n", phase+1, goroutines, entries)
	}

	fmt.Println(strings.Repeat("=", 100))
	fmt.Println("  Interpretation:")
	fmt.Println("  - If entries grew across phases → MBR expanded shards ✓")
	fmt.Println("  - If entries plateaued → fixed capacity, MBR either disabled or not triggered")
	fmt.Println("  - Redis cannot do this test (fixed capacity, no auto-scale)")
	fmt.Println(strings.Repeat("=", 100))
}

// ---------------------------------------------------------------------------
// Test 2: Oscillation Resistance
// Alternates between high and low memory pressure to check if MBR
// oscillates (migrate ↔ evict repeatedly) or stabilizes.
// ---------------------------------------------------------------------------

func runOscillate(mcacheAddr, redisAddr string, keySize, valSize int) {
	fmt.Println(strings.Repeat("=", 100))
	fmt.Println("  MBR Oscillation Resistance Test")
	fmt.Println("  Alternates high/low pressure to test PID stability")
	fmt.Println(strings.Repeat("=", 100))

	mc, _ := sdk.NewClient(mcacheAddr, sdk.WithPoolSize(32), sdk.WithCodec(sdk.RawCodec{}))
	if mc == nil {
		fmt.Println("  ERROR: mcache connect failed")
		return
	}
	defer mc.Close()

	fmt.Printf("  %-8s │ %10s │ %8s │ %10s │ %14s\n",
		"time", "entries", "mem", "phase", "decision-hint")
	fmt.Println(strings.Repeat("─", 100))

	prevEntries := parseEntryCount(fetchStats(mcacheAddr))
	decisionCount := 0

	for cycle := 0; cycle < 6; cycle++ {
		// HIGH pressure: fill with many unique keys
		isHigh := cycle%2 == 0
		phase := "LOW"
		keySpace := 1000
		if isHigh {
			phase = "HIGH"
			keySpace = 50000
		}

		var wg sync.WaitGroup
		stopCh := make(chan struct{})

		for g := 0; g < 32; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rng := rand.New(rand.NewSource(time.Now().UnixNano()))
				val := fillBytes(make([]byte, valSize))
				for {
					select {
					case <-stopCh:
						return
					default:
					}
					k := genKey(nil, rng.Intn(keySpace), keySize)
					_ = mc.Set(k, val, 0)
					time.Sleep(time.Duration(50+rand.Intn(100)) * time.Microsecond)
				}
			}()
		}

		// Sample for this cycle
		for t := 0; t < 20; t++ {
			time.Sleep(500 * time.Millisecond)
			stats := fetchStats(mcacheAddr)
			entries := parseEntryCount(stats)
			mem := parseCacheMemory(stats)

			hint := ""
			if entries != prevEntries && prevEntries > 0 {
				delta := entries - prevEntries
				if delta > 500 {
					hint = fmt.Sprintf("MIGRATE? +%d entries", delta)
					decisionCount++
				} else if delta < -500 {
					hint = fmt.Sprintf("EVICT? %d entries", delta)
				}
			}
			prevEntries = entries

			fmt.Printf("  %8s │ %10d │ %8s │ %10s │ %14s\n",
				time.Now().Format("15:04:05"), entries,
				formatBytes(int(mem)), phase, hint)
		}

		close(stopCh)
		wg.Wait()
		fmt.Printf("  --- cycle %d (%s) done ---\n\n", cycle+1, phase)
	}

	fmt.Println(strings.Repeat("=", 100))
	if decisionCount <= 2 {
		fmt.Printf("  ✓ PID stable: only %d migration decisions across 6 cycles\n", decisionCount)
	} else {
		fmt.Printf("  ✗ PID oscillating: %d decisions — may need tuning (Kp/Ki/Kd)\n", decisionCount)
	}
	fmt.Println("  Redis comparison: N/A (no auto-scale, no PID controller)")
	fmt.Println(strings.Repeat("=", 100))
}

// ---------------------------------------------------------------------------
// Test 3: Recovery Time
// Triggers a migration (by filling cache beyond initial capacity), then
// measures how long until throughput recovers to pre-migration levels.
// ---------------------------------------------------------------------------

func runRecovery(mcacheAddr string, keySize, valSize int) {
	fmt.Println(strings.Repeat("=", 90))
	fmt.Println("  MBR Recovery Time Test")
	fmt.Println("  Measures: how long after migration until throughput stabilizes")
	fmt.Println(strings.Repeat("=", 90))

	mc, _ := sdk.NewClient(mcacheAddr, sdk.WithPoolSize(16), sdk.WithCodec(sdk.RawCodec{}))
	if mc == nil {
		fmt.Println("  ERROR: mcache connect failed")
		return
	}
	defer mc.Close()

	// Phase 1: Baseline — measure throughput with low load
	fmt.Println("  Phase 1: Baseline throughput...")
	baseOps := benchThroughput(mc, keySize, valSize, 16, 10000, 5*time.Second)
	fmt.Printf("  Baseline: %.0f ops/s\n\n", baseOps)

	// Phase 2: Trigger pressure — fill cache far beyond capacity
	fmt.Println("  Phase 2: Creating memory pressure...")
	val := fillBytes(make([]byte, valSize))
	pressureStart := time.Now()
	for i := 0; i < 200000; i++ {
		_ = mc.Set(genKey(nil, i, keySize), val, 0)
		if i%50000 == 0 && i > 0 {
			stats := fetchStats(mcacheAddr)
			fmt.Printf("  filled %d keys, cache entries=%d, mem=%s\n",
				i, parseEntryCount(stats), formatBytes(int(parseCacheMemory(stats))))
		}
	}
	pressureDur := time.Since(pressureStart)

	// Phase 3: Measure recovery
	fmt.Println("\n  Phase 3: Measuring recovery...")
	fmt.Printf("  %-8s │ %10s\n", "time", "ops/s")
	fmt.Println(strings.Repeat("─", 50))

	recovered := false
	recoveryStart := time.Now()
	for t := 0; t < 30; t++ {
		ops := benchThroughput(mc, keySize, valSize, 8, 5000, 2*time.Second)
		elapsed := time.Since(recoveryStart).Seconds()

		status := ""
		if ops >= baseOps*0.9 && !recovered {
			status = fmt.Sprintf(" ← recovered at +%.1fs", elapsed)
			recovered = true
		}

		fmt.Printf("  %7.1fs │ %10.0f%s\n", elapsed, ops, status)

		if recovered {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Println(strings.Repeat("=", 90))
	fmt.Printf("  Pressure phase: %v (%d keys)\n", pressureDur.Round(time.Second), 200000)
	if recovered {
		fmt.Printf("  ✓ Recovery time: %.1fs\n", time.Since(recoveryStart).Seconds())
	} else {
		fmt.Println("  ✗ Did not recover within 30s — MBR may not have triggered migration")
	}
	fmt.Println("  Redis comparison: N/A (no dynamic shard scaling)")
	fmt.Println(strings.Repeat("=", 90))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type mcacheStats struct {
	Entries       int
	Memory        uint64
	TotalRequests uint64
}

func fetchStats(addr string) mcacheStats {
	c, err := mnet.NewClient(addr,
		mnet.WithPoolSize(1),
		mnet.WithDialTimeout(2*time.Second),
		mnet.WithClientReadTimeout(2*time.Second),
		mnet.WithClientWriteTimeout(2*time.Second))
	if err != nil {
		return mcacheStats{Entries: -1}
	}
	defer c.Close()
	data, err := c.Stats()
	if err != nil {
		return mcacheStats{Entries: -1}
	}
	var s struct {
		CacheEntries  int    `json:"cache_entries"`
		CacheMemory   uint64 `json:"cache_memory"`
		TotalRequests uint64 `json:"total_requests"`
	}
	json.Unmarshal(data, &s)
	return mcacheStats{Entries: s.CacheEntries, Memory: s.CacheMemory, TotalRequests: s.TotalRequests}
}

func parseEntryCount(s mcacheStats) int  { return s.Entries }
func parseCacheMemory(s mcacheStats) int { return int(s.Memory) }
func parseTotalReqs(s mcacheStats) int   { return int(s.TotalRequests) }

func benchThroughput(mc *sdk.Client, keySize, valSize int, goroutines, totalOps int, maxDur time.Duration) float64 {
	opsPerG := totalOps / goroutines
	var wg sync.WaitGroup
	start := make(chan struct{})
	done := make(chan struct{})
	var completed atomic.Int64

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano()))
			val := fillBytes(make([]byte, valSize))
			<-start
			for i := 0; i < opsPerG; i++ {
				select {
				case <-done:
					return
				default:
				}
				k := genKey(nil, rng.Intn(10000), keySize)
				_ = mc.Set(k, val, 0)
				completed.Add(1)
			}
		}()
	}

	close(start)
	t0 := time.Now()

	// Wait for completion or timeout
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(maxDur):
	}

	elapsed := time.Since(t0).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(completed.Load()) / elapsed
}

func genKey(buf []byte, i int, size int) string {
	if buf == nil {
		buf = make([]byte, size)
	}
	s := fmt.Sprintf("%08x", i)
	copy(buf, s)
	for j := len(s); j < size; j++ {
		buf[j] = 'k'
	}
	return string(buf[:size])
}

func fillBytes(b []byte) []byte {
	for i := range b {
		b[i] = 'x'
	}
	return b
}

func formatBytes(n int) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// Suppress unused warnings for Redis imports in tests that don't use Redis
var _ = context.Background
var _ = redis.NewClient
var _ = strings.Contains
var _ = sync.Map{}
