// Package main benchmarks mcache (with optional MBR) against Redis.
//
// Usage:
//
//	# Quick comparison (pipeline=16)
//	go run ./bench_compare --mcache 127.0.0.1:11211 --redis 127.0.0.1:6379 --mode set --profile small --ops 100000
//
//	# MBR stress: low shards + high load → watch MBR auto-scale
//	# Requires server started with mbr.enabled:true, cache.shards:4 in config.yaml
//	go run ./bench_compare --mcache 127.0.0.1:11211 --redis 127.0.0.1:6379 --mode set --profile large --ops 500000 --pipeline 32 --goroutines 128
//
//	# Compare all modes
//	for mode in set get mix; do
//	  go run ./bench_compare --mcache 127.0.0.1:11211 --redis 127.0.0.1:6379 --mode $mode --profile small --ops 100000
//	done
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	sdk "github.com/atoncooper/mcache/sdk/go"
	mnet "github.com/atoncooper/mcache/net"
	"github.com/redis/go-redis/v9"
)

var sizeProfiles = map[string]struct{ keySize, valSize int }{
	"tiny":   {16, 64},
	"small":  {64, 256},
	"medium": {256, 1024},
	"large":  {256, 4096},
	"xlarge": {512, 16384},
}

func main() {
	mcacheAddr := flag.String("mcache", "127.0.0.1:11211", "mcache address")
	redisAddr := flag.String("redis", "127.0.0.1:6379", "Redis address")
	mode := flag.String("mode", "set", "benchmark mode: set, get, mix")
	profile := flag.String("profile", "small", "size profile")
	keySize := flag.Int("key-size", 0, "key size override")
	valSize := flag.Int("val-size", 0, "value size override")
	totalOps := flag.Int("ops", 100_000, "total operations")
	goroutines := flag.Int("goroutines", 64, "concurrent goroutines")
	pipelineSize := flag.Int("pipeline", 16, "mcache pipeline batch size (0=off)")
	readRatio := flag.Float64("read-ratio", 0.5, "read ratio for mix mode")
	keySpace := flag.Int("key-space", 0, "unique key space")
	flag.Parse()

	ks, vs := *keySize, *valSize
	if ks == 0 || vs == 0 {
		p := sizeProfiles[*profile]
		if ks == 0 {
			ks = p.keySize
		}
		if vs == 0 {
			vs = p.valSize
		}
	}

	nKeys := *keySpace
	if nKeys == 0 {
		nKeys = *totalOps / 10
		if nKeys < 1000 {
			nKeys = 1000
		}
	}

	cfg := benchConfig{
		mcacheAddr:   *mcacheAddr,
		redisAddr:    *redisAddr,
		mode:         *mode,
		keySize:      ks,
		valSize:      vs,
		totalOps:     *totalOps,
		goroutines:   *goroutines,
		pipelineSize: *pipelineSize,
		readRatio:    *readRatio,
		keySpace:     nKeys,
	}

	fmt.Println(strings.Repeat("=", 90))
	fmt.Printf("  mcache vs Redis — %s | %dB key + %dB val | %d goroutines | pipeline=%d\n",
		*mode, ks, vs, *goroutines, *pipelineSize)
	fmt.Println(strings.Repeat("=", 90))

	// Show mcache server state before test
	mcacheStats := fetchMcacheStats(cfg.mcacheAddr)
	printMcacheState("  [mcache state before]", mcacheStats)

	// Run benchmarks
	mResult := benchMcache(cfg)
	rResult := benchRedis(cfg)

	// Show mcache server state after test (to see if MBR migrated shards)
	mcacheStatsAfter := fetchMcacheStats(cfg.mcacheAddr)
	printMcacheState("  [mcache state after ]", mcacheStatsAfter)

	if mcacheStatsAfter.CacheEntries != mcacheStats.CacheEntries ||
		len(mcacheStatsAfter.Value) != len(mcacheStats.Value) {
		fmt.Println("  ⚡ MBR may have triggered migration (state changed during test)")
	}

	// Print comparison table
	fmt.Println()
	fmt.Println(strings.Repeat("─", 90))
	fmt.Printf("  %-15s │ %14s │ %14s │ %10s\n", "metric", "mcache", "redis", "mcache/redis")
	fmt.Println(strings.Repeat("─", 90))

	if rResult.opsPerSec > 0 {
		ratio := mResult.opsPerSec / rResult.opsPerSec * 100
		verdict := "✓"
		if ratio < 80 {
			verdict = "✗"
		} else if ratio < 95 {
			verdict = "~"
		}
		fmt.Printf("  %-15s │ %14.0f │ %14.0f │ %7.0f%% %s\n",
			"throughput ops/s", mResult.opsPerSec, rResult.opsPerSec, ratio, verdict)
	}
	if mResult.avgLatUs > 0 {
		fmt.Printf("  %-15s │ %12.0fμs │ %12.0fμs │ %10s\n",
			"avg latency", mResult.avgLatUs, rResult.avgLatUs, "")
	}
	if mResult.p50LatUs > 0 {
		fmt.Printf("  %-15s │ %12.0fμs │ %12.0fμs │ %10s\n",
			"p50 latency", mResult.p50LatUs, rResult.p50LatUs, "")
	}
	if mResult.p99LatUs > 0 {
		fmt.Printf("  %-15s │ %12.0fμs │ %12.0fμs │ %10s\n",
			"p99 latency", mResult.p99LatUs, rResult.p99LatUs, "")
	}
	fmt.Println(strings.Repeat("─", 90))

	printVerdict(mResult, rResult)
	fmt.Println(strings.Repeat("=", 90))
}

type benchConfig struct {
	mcacheAddr, redisAddr     string
	mode                      string
	keySize, valSize          int
	totalOps, goroutines      int
	pipelineSize              int
	readRatio                 float64
	keySpace                  int
}

type benchResult struct {
	opsPerSec float64
	avgLatUs  float64
	p50LatUs  float64
	p99LatUs  float64
}

// mcacheServerStats mirrors net.ServerStats for JSON parsing.
type mcacheServerStats struct {
	UptimeMs      int64  `json:"uptime_ms"`
	Connections   int    `json:"connections"`
	TotalRequests uint64 `json:"total_requests"`
	CacheEntries  int    `json:"cache_entries"`
	CacheMemory   uint64 `json:"cache_memory"`
	MemoryLimit   uint64 `json:"memory_limit"`
	Goroutines    int    `json:"goroutines"`
	Value         []byte `json:"-"` // raw JSON
}

func fetchMcacheStats(addr string) mcacheServerStats {
	transport, err := mnet.NewClient(addr, mnet.WithPoolSize(1),
		mnet.WithDialTimeout(2*time.Second),
		mnet.WithClientReadTimeout(2*time.Second),
		mnet.WithClientWriteTimeout(2*time.Second))
	if err != nil {
		return mcacheServerStats{}
	}
	defer transport.Close()

	data, err := transport.Stats()
	if err != nil {
		return mcacheServerStats{}
	}

	var stats mcacheServerStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return mcacheServerStats{}
	}
	stats.Value = data
	return stats
}

func printMcacheState(label string, stats mcacheServerStats) {
	if stats.Value == nil {
		fmt.Printf("%s  (server unreachable)\n", label)
		return
	}
	uptime := time.Duration(stats.UptimeMs) * time.Millisecond
	fmt.Printf("%s  uptime=%v  conns=%d  entries=%d  mem=%s  goroutines=%d  reqs=%d\n",
		label, uptime.Round(time.Second), stats.Connections, stats.CacheEntries,
		formatBytes(int(stats.CacheMemory)), stats.Goroutines, stats.TotalRequests)
}

func benchMcache(cfg benchConfig) benchResult {
	c, err := sdk.NewClient(cfg.mcacheAddr, sdk.WithPoolSize(cfg.goroutines), sdk.WithCodec(sdk.RawCodec{}))
	if err != nil {
		fmt.Printf("  mcache connect error: %v\n", err)
		return benchResult{}
	}
	defer c.Close()

	opsPerG := cfg.totalOps / cfg.goroutines
	var totalOpsDone int64
	lats := make([]time.Duration, cfg.totalOps)
	var wg sync.WaitGroup
	var ready sync.WaitGroup
	start := make(chan struct{})
	ready.Add(cfg.goroutines)

	if cfg.mode == "get" || cfg.mode == "mix" {
		fmt.Printf("  populating mcache with %d keys...\n", cfg.keySpace)
		val := fillBytes(make([]byte, cfg.valSize))
		for i := 0; i < cfg.keySpace; i++ {
			_ = c.Set(genKey(nil, i, cfg.keySize), val, 0)
		}
	}

	batchSz := cfg.pipelineSize
	if batchSz <= 0 {
		batchSz = 1
	}

	for gid := 0; gid < cfg.goroutines; gid++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(gid)))
			val := fillBytes(make([]byte, cfg.valSize))
			p := c.Pipeline()
			ready.Done()
			<-start
			base := gid * opsPerG

			i := 0
			for i < opsPerG {
				end := min(i+batchSz, opsPerG)
				for j := i; j < end; j++ {
					k := genKey(nil, rng.Intn(cfg.keySpace), cfg.keySize)
					switch cfg.mode {
					case "get":
						p.AddGet(k)
					case "mix":
						if rng.Float64() < cfg.readRatio {
							p.AddGet(k)
						} else {
							p.AddSet(k, val, 0)
						}
					default:
						p.AddSet(k, val, 0)
					}
				}
				batchStart := time.Now()
				switch cfg.mode {
				case "get":
					dests := make([]any, end-i)
					for j := range dests {
						var v []byte
						dests[j] = &v
					}
					_ = p.FlushGets(dests)
				case "mix":
					_, _ = p.Flush()
				default:
					_ = p.FlushSets()
				}
				elapsed := time.Since(batchStart)
				perOp := elapsed / time.Duration(end-i)
				for j := i; j < end; j++ {
					lats[base+j] = perOp
				}
				totalOpsDone += int64(end - i)
				p.Reset()
				i = end
			}
		}(gid)
	}

	ready.Wait()
	wallStart := time.Now()
	close(start)
	wg.Wait()
	wallElapsed := time.Since(wallStart).Seconds()

	return buildResult(lats, cfg.totalOps, wallElapsed)
}

func benchRedis(cfg benchConfig) benchResult {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.redisAddr,
		PoolSize:     cfg.goroutines,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	})
	defer rdb.Close()
	ctx := context.Background()

	if cfg.mode == "get" || cfg.mode == "mix" {
		fmt.Printf("  populating redis with %d keys...\n", cfg.keySpace)
		val := fillBytes(make([]byte, cfg.valSize))
		pipe := rdb.Pipeline()
		for i := 0; i < cfg.keySpace; i++ {
			pipe.Set(ctx, genKey(nil, i, cfg.keySize), val, 0)
		}
		_, _ = pipe.Exec(ctx)
	}

	opsPerG := cfg.totalOps / cfg.goroutines
	lats := make([]time.Duration, cfg.totalOps)
	var wg sync.WaitGroup
	var ready sync.WaitGroup
	start := make(chan struct{})
	ready.Add(cfg.goroutines)

	for gid := 0; gid < cfg.goroutines; gid++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(gid)))
			val := fillBytes(make([]byte, cfg.valSize))
			ready.Done()
			<-start
			base := gid * opsPerG
			for i := 0; i < opsPerG; i++ {
				k := genKey(nil, rng.Intn(cfg.keySpace), cfg.keySize)
				t0 := time.Now()
				switch cfg.mode {
				case "get":
					_ = rdb.Get(ctx, k).Err()
				case "mix":
					if rng.Float64() < cfg.readRatio {
						_ = rdb.Get(ctx, k).Err()
					} else {
						_ = rdb.Set(ctx, k, val, 0).Err()
					}
				default:
					_ = rdb.Set(ctx, k, val, 0).Err()
				}
				lats[base+i] = time.Since(t0)
			}
		}(gid)
	}

	ready.Wait()
	wallStart := time.Now()
	close(start)
	wg.Wait()
	wallElapsed := time.Since(wallStart).Seconds()

	return buildResult(lats, cfg.totalOps, wallElapsed)
}

func buildResult(lats []time.Duration, totalOps int, elapsed float64) benchResult {
	n := len(lats)
	if n == 0 || elapsed <= 0 {
		return benchResult{}
	}

	// Sort for percentiles
	sorted := make([]time.Duration, n)
	copy(sorted, lats)
	sortDurations(sorted)

	var sum int64
	for _, d := range lats {
		sum += int64(d)
	}

	return benchResult{
		opsPerSec: float64(totalOps) / elapsed,
		avgLatUs:  float64(sum/int64(time.Microsecond)) / float64(n),
		p50LatUs:  float64(sorted[n*50/100] / time.Microsecond),
		p99LatUs:  float64(sorted[n*99/100] / time.Microsecond),
	}
}

func sortDurations(a []time.Duration) {
	for i := 0; i < len(a); i++ {
		for j := i + 1; j < len(a); j++ {
			if a[i] > a[j] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}

func printVerdict(m, r benchResult) {
	if r.opsPerSec <= 0 {
		fmt.Println("  ⚠ Redis unreachable — cannot compare")
		return
	}
	ratio := m.opsPerSec / r.opsPerSec
	switch {
	case ratio >= 0.95:
		fmt.Printf("  ✓ mcache matches Redis (%.0f%%)\n", ratio*100)
	case ratio >= 0.80:
		fmt.Printf("  ~ mcache at %.0f%% of Redis — try --pipeline 32\n", ratio*100)
	case ratio >= 0.50:
		fmt.Printf("  ✗ mcache at %.0f%% of Redis — check TCP_NODELAY and server build\n", ratio*100)
	default:
		fmt.Printf("  ✗✗ mcache only %.0f%% of Redis — server may be unoptimized\n", ratio*100)
	}
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
	if size < len(s) {
		return string(buf[:size])
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
