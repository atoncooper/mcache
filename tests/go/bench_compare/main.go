// Package main benchmarks mcache against Redis across multiple dimensions:
//
// Single point (default):
//
//	go run ./bench_compare --mcache :11211 --redis :6379 --mode set --profile small
//
// Comprehensive sweep:
//
//	go run ./bench_compare --mcache :11211 --redis :6379 --sweep
//
// Memory overhead:
//
//	go run ./bench_compare --mcache :11211 --redis :6379 --mode set --profile small --overhead
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"sort"
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
	keySize := flag.Int("key-size", 0, "key size override (0=use profile)")
	valSize := flag.Int("val-size", 0, "value size override (0=use profile)")
	totalOps := flag.Int("ops", 100_000, "total operations")
	goroutines := flag.Int("goroutines", 64, "concurrent goroutines")
	pipelineSize := flag.Int("pipeline", 0, "pipeline batch size (0=off)")
	sweep := flag.Bool("sweep", false, "run comprehensive sweep across goroutines/profiles")
	overhead := flag.Bool("overhead", false, "measure per-key memory overhead")
	flag.Parse()

	ks, vs := *keySize, *valSize
	if ks == 0 || vs == 0 {
		sp, ok := sizeProfiles[*profile]
		if !ok {
			sp = sizeProfiles["small"]
		}
		if ks == 0 {
			ks = sp.keySize
		}
		if vs == 0 {
			vs = sp.valSize
		}
	}

	if *sweep {
		runSweep(*mcacheAddr, *redisAddr)
		return
	}

	if *overhead {
		runOverhead(*mcacheAddr, *redisAddr, ks, vs)
		return
	}

	// Single-point comparison
	cfg := benchConfig{
		mcacheAddr:   *mcacheAddr,
		redisAddr:    *redisAddr,
		mode:         *mode,
		keySize:      ks,
		valSize:      vs,
		totalOps:     *totalOps,
		goroutines:   *goroutines,
		pipelineSize: *pipelineSize,
	}

	printHeader(cfg)
	printMcacheState("  [mcache]", cfg.mcacheAddr)
	mResult := benchMcache(cfg)
	rResult := benchRedis(cfg)
	printMcacheState("  [mcache]", cfg.mcacheAddr)
	printComparison(mResult, rResult, cfg)
}

// ---------------------------------------------------------------------------
// Sweep mode — test multiple concurrency levels and profiles
// ---------------------------------------------------------------------------

func runSweep(mcacheAddr, redisAddr string) {
	profiles := []string{"small", "medium", "large"}
	modes := []string{"set", "get", "mix"}
	gCounts := []int{16, 64, 128}

	fmt.Println(strings.Repeat("=", 100))
	fmt.Println("  mcache vs Redis — Comprehensive Sweep")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Println()

	for _, mode := range modes {
		for _, prof := range profiles {
			sp := sizeProfiles[prof]
			for _, g := range gCounts {
				cfg := benchConfig{
					mcacheAddr:   mcacheAddr,
					redisAddr:    redisAddr,
					mode:         mode,
					keySize:      sp.keySize,
					valSize:      sp.valSize,
					totalOps:     max(50000, g*1000),
					goroutines:   g,
					pipelineSize: 0,
				}
				mResult := benchMcache(cfg)
				rResult := benchRedis(cfg)
				ratio := 0.0
				if rResult.opsPerSec > 0 {
					ratio = mResult.opsPerSec / rResult.opsPerSec * 100
				}
				fmt.Printf("  %-4s %-7s g=%-3d │ mcache %8.0f │ redis %8.0f │ %5.0f%%\n",
					mode, prof, g, mResult.opsPerSec, rResult.opsPerSec, ratio)
			}
		}
		fmt.Println()
	}

	// Pipeline sweep for SET only (biggest pipeline benefit)
	fmt.Println("  --- Pipeline sweep (mode=set, profile=small, g=64) ---")
	for _, ps := range []int{0, 8, 16, 32, 64} {
		cfg := benchConfig{
			mcacheAddr:   mcacheAddr,
			redisAddr:    redisAddr,
			mode:         "set",
			keySize:      64,
			valSize:      256,
			totalOps:     100_000,
			goroutines:   64,
			pipelineSize: ps,
		}
		mResult := benchMcache(cfg)
		rResult := benchRedis(cfg)
		ratio := 0.0
		if rResult.opsPerSec > 0 {
			ratio = mResult.opsPerSec / rResult.opsPerSec * 100
		}
		pLabel := "off"
		if ps > 0 {
			pLabel = fmt.Sprintf("%d", ps)
		}
		fmt.Printf("  pipeline=%-4s │ mcache %8.0f │ redis %8.0f │ %5.0f%%\n",
			pLabel, mResult.opsPerSec, rResult.opsPerSec, ratio)
	}

	fmt.Println(strings.Repeat("=", 100))
}

// ---------------------------------------------------------------------------
// Overhead mode — measure per-key memory cost
// ---------------------------------------------------------------------------

func runOverhead(mcacheAddr, redisAddr string, keySize, valSize int) {
	nKeys := 50000
	val := fillBytes(make([]byte, valSize))

	// Measure mcache memory
	fmt.Println(strings.Repeat("=", 90))
	fmt.Printf("  Memory Overhead — %dB key + %dB val, %d keys\n", keySize, valSize, nKeys)
	fmt.Println(strings.Repeat("=", 90))

	mBefore := getRSS(mcacheAddr) // best-effort via stats
	mc, _ := sdk.NewClient(mcacheAddr, sdk.WithPoolSize(4), sdk.WithCodec(sdk.RawCodec{}))
	for i := 0; i < nKeys; i++ {
		mc.Set(genKey(nil, i, keySize), val, 0)
	}
	mc.Close()
	time.Sleep(500 * time.Millisecond) // let GC settle

	mStats := fetchMcacheStats(mcacheAddr)
	mMem := parseCacheMemory(mStats)
	mEntries := parseCacheEntries(mStats)
	mPerKey := float64(mMem) / float64(max(mEntries, 1))

	// Measure Redis memory
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr, PoolSize: 4})
	ctx := context.Background()
	pipe := rdb.Pipeline()
	for i := nKeys; i < nKeys*2; i++ {
		pipe.Set(ctx, genKey(nil, i, keySize), val, 0)
	}
	pipe.Exec(ctx)
	time.Sleep(500 * time.Millisecond)

	rInfo, _ := rdb.Info(ctx, "memory").Result()
	rMem := int64(0)
	for _, line := range strings.Split(rInfo, "\r\n") {
		if strings.HasPrefix(line, "used_memory_dataset:") {
			fmt.Sscanf(line, "used_memory_dataset:%d", &rMem)
		}
	}
	rdb.Close()

	rPerKey := float64(rMem) / float64(max(nKeys, 1))

	fmt.Printf("  %-20s │ %14s │ %14s\n", "metric", "mcache", "redis")
	fmt.Println(strings.Repeat("─", 90))
	fmt.Printf("  %-20s │ %12d B │ %12d B\n", "total memory", mMem, rMem)
	fmt.Printf("  %-20s │ %14d │ %14d\n", "entries", mEntries, nKeys)
	fmt.Printf("  %-20s │ %12.0f B │ %12.0f B\n", "per-key overhead", mPerKey, rPerKey)
	fmt.Printf("  %-20s │ %12.0f B │ %12.0f B\n", "theoretical (key+val)", float64(keySize+valSize), float64(keySize+valSize))
	fmt.Printf("  %-20s │ %12.0f B │ %12.0f B\n", "extra overhead", mPerKey-float64(keySize+valSize), rPerKey-float64(keySize+valSize))
	fmt.Println(strings.Repeat("=", 90))
	_ = mBefore
}

func getRSS(addr string) int64 { return 0 } // placeholder — would use OS-specific RSS query

// ---------------------------------------------------------------------------
// Bench types and config
// ---------------------------------------------------------------------------

type benchConfig struct {
	mcacheAddr, redisAddr     string
	mode                      string
	keySize, valSize          int
	totalOps, goroutines      int
	pipelineSize              int
	readRatio                 float64
}

type benchResult struct {
	opsPerSec float64
	avgUs     float64
	p50Us     float64
	p99Us     float64
}

// ---------------------------------------------------------------------------
// mcache benchmark — same logic as bench_mcache/main.go
// ---------------------------------------------------------------------------

func benchMcache(cfg benchConfig) benchResult {
	c, err := sdk.NewClient(cfg.mcacheAddr, sdk.WithPoolSize(cfg.goroutines), sdk.WithCodec(sdk.RawCodec{}))
	if err != nil {
		return benchResult{}
	}
	defer c.Close()

	opsPerG := cfg.totalOps / cfg.goroutines
	lats := make([]time.Duration, cfg.totalOps)
	var wg sync.WaitGroup
	var ready sync.WaitGroup
	start := make(chan struct{})
	ready.Add(cfg.goroutines)

	// Pre-populate for read modes
	if cfg.mode == "get" || cfg.mode == "mix" {
		keySpace := cfg.totalOps / 10
		if keySpace < 1000 {
			keySpace = 1000
		}
		val := fillBytes(make([]byte, cfg.valSize))
		for i := 0; i < keySpace; i++ {
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
			keyBuf := make([]byte, cfg.keySize)
			pipeline := c.Pipeline()
			ready.Done()
			<-start
			base := gid * opsPerG

			if batchSz > 1 {
				for i := 0; i < opsPerG; {
					end := min(i+batchSz, opsPerG)
					keySpace := cfg.totalOps / 10
					if keySpace < 1000 {
						keySpace = 1000
					}
					for j := i; j < end; j++ {
						k := genKey(nil, rng.Intn(keySpace), cfg.keySize)
						switch cfg.mode {
						case "get":
							pipeline.AddGet(k)
						case "mix":
							if rng.Float64() < 0.5 {
								pipeline.AddGet(k)
							} else {
								pipeline.AddSet(k, val, 0)
							}
						default:
							pipeline.AddSet(k, val, 0)
						}
					}
					t0 := time.Now()
					switch cfg.mode {
					case "get":
						dests := make([]any, end-i)
						for j := range dests {
							var v []byte
							dests[j] = &v
						}
						pipeline.FlushGets(dests)
					case "mix":
						pipeline.Flush()
					default:
						pipeline.FlushSets()
					}
					elapsed := time.Since(t0)
					perOp := elapsed / time.Duration(end-i)
					for j := i; j < end; j++ {
						lats[base+j] = perOp
					}
					pipeline.Reset()
					i = end
				}
			} else {
				keySpace := cfg.totalOps / 10
				if keySpace < 1000 {
					keySpace = 1000
				}
				for i := 0; i < opsPerG; i++ {
					k := genKey(keyBuf, rng.Intn(keySpace), cfg.keySize)
					t0 := time.Now()
					switch cfg.mode {
					case "get":
						var v []byte
						_ = c.Get(k, &v)
					case "mix":
						if rng.Float64() < 0.5 {
							var v []byte
							_ = c.Get(k, &v)
						} else {
							_ = c.Set(k, val, 0)
						}
					default:
						_ = c.Set(k, val, 0)
					}
					lats[base+i] = time.Since(t0)
				}
			}
		}(gid)
	}

	ready.Wait()
	t0 := time.Now()
	close(start)
	wg.Wait()
	elapsed := time.Since(t0).Seconds()

	return buildResult(lats, cfg.totalOps, elapsed)
}

// ---------------------------------------------------------------------------
// Redis benchmark — same logic as bench_redis/main.go
// ---------------------------------------------------------------------------

func benchRedis(cfg benchConfig) benchResult {
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.redisAddr, PoolSize: cfg.goroutines,
		ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second,
	})
	defer rdb.Close()
	ctx := context.Background()

	opsPerG := cfg.totalOps / cfg.goroutines
	lats := make([]time.Duration, cfg.totalOps)
	var wg sync.WaitGroup
	var ready sync.WaitGroup
	start := make(chan struct{})
	ready.Add(cfg.goroutines)

	// Pre-populate for read modes
	if cfg.mode == "get" || cfg.mode == "mix" {
		keySpace := cfg.totalOps / 10
		if keySpace < 1000 {
			keySpace = 1000
		}
		val := fillBytes(make([]byte, cfg.valSize))
		pipe := rdb.Pipeline()
		for i := 0; i < keySpace; i++ {
			pipe.Set(ctx, genKey(nil, i, cfg.keySize), val, 0)
		}
		pipe.Exec(ctx)
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
			ready.Done()
			<-start
			base := gid * opsPerG

			if batchSz > 1 {
				keySpace := cfg.totalOps / 10
				if keySpace < 1000 {
					keySpace = 1000
				}
				for i := 0; i < opsPerG; {
					end := min(i+batchSz, opsPerG)
					pipe := rdb.Pipeline()
					for j := i; j < end; j++ {
						k := genKey(nil, rng.Intn(keySpace), cfg.keySize)
						switch cfg.mode {
						case "get":
							pipe.Get(ctx, k)
						case "mix":
							if rng.Float64() < 0.5 {
								pipe.Get(ctx, k)
							} else {
								pipe.Set(ctx, k, val, 0)
							}
						default:
							pipe.Set(ctx, k, val, 0)
						}
					}
					t0 := time.Now()
					pipe.Exec(ctx)
					elapsed := time.Since(t0)
					perOp := elapsed / time.Duration(end-i)
					for j := i; j < end; j++ {
						lats[base+j] = perOp
					}
					i = end
				}
			} else {
				keySpace := cfg.totalOps / 10
				if keySpace < 1000 {
					keySpace = 1000
				}
				for i := 0; i < opsPerG; i++ {
					k := genKey(nil, rng.Intn(keySpace), cfg.keySize)
					t0 := time.Now()
					switch cfg.mode {
					case "get":
						rdb.Get(ctx, k)
					case "mix":
						if rng.Float64() < 0.5 {
							rdb.Get(ctx, k)
						} else {
							rdb.Set(ctx, k, val, 0)
						}
					default:
						rdb.Set(ctx, k, val, 0)
					}
					lats[base+i] = time.Since(t0)
				}
			}
		}(gid)
	}

	ready.Wait()
	t0 := time.Now()
	close(start)
	wg.Wait()
	elapsed := time.Since(t0).Seconds()

	return buildResult(lats, cfg.totalOps, elapsed)
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

func printHeader(cfg benchConfig) {
	fmt.Println(strings.Repeat("=", 90))
	pipLabel := "off"
	if cfg.pipelineSize > 0 {
		pipLabel = fmt.Sprintf("%d", cfg.pipelineSize)
	}
	fmt.Printf("  mcache vs Redis — %s | %dB key + %dB val | %d goroutines | pipeline=%s\n",
		cfg.mode, cfg.keySize, cfg.valSize, cfg.goroutines, pipLabel)
	fmt.Println(strings.Repeat("=", 90))
}

func printComparison(m, r benchResult, cfg benchConfig) {
	fmt.Println()
	fmt.Println(strings.Repeat("─", 90))
	fmt.Printf("  %-18s │ %14s │ %14s │ %10s\n", "metric", "mcache", "redis", "ratio")
	fmt.Println(strings.Repeat("─", 90))

	if r.opsPerSec > 0 {
		ratio := m.opsPerSec / r.opsPerSec * 100
		mark := "✓"
		if ratio < 80 {
			mark = "✗"
		} else if ratio < 95 {
			mark = "~"
		}
		fmt.Printf("  %-18s │ %14.0f │ %14.0f │ %7.0f%% %s\n",
			"throughput (ops/s)", m.opsPerSec, r.opsPerSec, ratio, mark)
	}
	if m.avgUs > 0 && r.avgUs > 0 {
		fmt.Printf("  %-18s │ %12.0fμs │ %12.0fμs │ %10s\n",
			"avg latency", m.avgUs, r.avgUs, "")
	}
	if m.p50Us > 0 && r.p50Us > 0 {
		fmt.Printf("  %-18s │ %12.0fμs │ %12.0fμs │ %10s\n",
			"p50 latency", m.p50Us, r.p50Us, "")
	}
	if m.p99Us > 0 && r.p99Us > 0 {
		fmt.Printf("  %-18s │ %12.0fμs │ %12.0fμs │ %10s\n",
			"p99 latency", m.p99Us, r.p99Us, "")
	}
	fmt.Println(strings.Repeat("─", 90))
	if r.opsPerSec > 0 {
		ratio := m.opsPerSec / r.opsPerSec
		switch {
		case ratio >= 0.95:
			fmt.Printf("  ✓ mcache matches Redis (%.0f%%)\n", ratio*100)
		case ratio >= 0.80:
			fmt.Printf("  ~ mcache at %.0f%% of Redis\n", ratio*100)
		default:
			fmt.Printf("  ✗ mcache at %.0f%% of Redis\n", ratio*100)
		}
	}
	fmt.Println(strings.Repeat("=", 90))
	_ = cfg
}

func printMcacheState(label, addr string) {
	transport, err := mnet.NewClient(addr,
		mnet.WithPoolSize(1),
		mnet.WithDialTimeout(2*time.Second),
		mnet.WithClientReadTimeout(2*time.Second),
		mnet.WithClientWriteTimeout(2*time.Second))
	if err != nil {
		return
	}
	data, err := transport.Stats()
	transport.Close()
	if err != nil {
		return
	}
	var s struct {
		UptimeMs      int64  `json:"uptime_ms"`
		Connections   int    `json:"connections"`
		CacheEntries  int    `json:"cache_entries"`
		CacheMemory   uint64 `json:"cache_memory"`
		Goroutines    int    `json:"goroutines"`
		TotalRequests uint64 `json:"total_requests"`
	}
	json.Unmarshal(data, &s)
	uptime := time.Duration(s.UptimeMs) * time.Millisecond
	fmt.Printf("%s  uptime=%v  conns=%d  entries=%d  mem=%s  gos=%d  reqs=%d\n",
		label, uptime.Round(time.Second), s.Connections, s.CacheEntries,
		formatBytes(int(s.CacheMemory)), s.Goroutines, s.TotalRequests)
}

func buildResult(lats []time.Duration, totalOps int, elapsed float64) benchResult {
	if len(lats) == 0 || elapsed <= 0 {
		return benchResult{}
	}
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	n := len(lats)
	var sum int64
	for _, d := range lats {
		sum += int64(d)
	}
	return benchResult{
		opsPerSec: float64(totalOps) / elapsed,
		avgUs:     float64(sum/int64(time.Microsecond)) / float64(n),
		p50Us:     float64(lats[n*50/100] / time.Microsecond),
		p99Us:     float64(lats[n*99/100] / time.Microsecond),
	}
}

func fetchMcacheStats(addr string) []byte {
	transport, _ := mnet.NewClient(addr, mnet.WithPoolSize(1),
		mnet.WithDialTimeout(2*time.Second), mnet.WithClientReadTimeout(2*time.Second))
	if transport == nil {
		return nil
	}
	defer transport.Close()
	data, _ := transport.Stats()
	return data
}

func parseCacheEntries(data []byte) int {
	if data == nil {
		return -1
	}
	var s struct{ CacheEntries int `json:"cache_entries"` }
	json.Unmarshal(data, &s)
	return s.CacheEntries
}

func parseCacheMemory(data []byte) int {
	if data == nil {
		return -1
	}
	var s struct{ CacheMemory uint64 `json:"cache_memory"` }
	json.Unmarshal(data, &s)
	return int(s.CacheMemory)
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

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

