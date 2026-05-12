// Package main benchmarks mcache with configurable KV sizes and operation modes.
//
// Usage:
//
//	go run ./bench_mcache --addr 127.0.0.1:11211 --mode set --key-size 64 --val-size 256 --ops 100000 --goroutines 64
//	go run ./bench_mcache --addr 127.0.0.1:11211 --mode get  --key-size 64 --val-size 1024 --ops 100000
//	go run ./bench_mcache --addr 127.0.0.1:11211 --mode mix --key-size 256 --val-size 4096 --ops 200000 --goroutines 128
//
// Predefined size profiles (override with --key-size / --val-size):
//
//	tiny:   16B key,   64B value
//	small:  64B key,  256B value
//	medium: 256B key,   1KB value
//	large:  256B key,   4KB value
//	xlarge: 512B key,  16KB value
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	sdk "github.com/atoncooper/mcache/sdk/go"
)

var sizeProfiles = map[string]struct{ keySize, valSize int }{
	"tiny":    {16, 64},
	"small":   {64, 256},
	"medium":  {256, 1024},
	"large":   {256, 4096},
	"xlarge":  {512, 16384},
}

func main() {
	addr := flag.String("addr", "127.0.0.1:11211", "mcache address")
	mode := flag.String("mode", "set", "benchmark mode: set, get, mix")
	profile := flag.String("profile", "small", "size profile: tiny, small, medium, large, xlarge")
	keySize := flag.Int("key-size", 0, "key size in bytes (overrides profile)")
	valSize := flag.Int("val-size", 0, "value size in bytes (overrides profile)")
	totalOps := flag.Int("ops", 100_000, "total operations")
	goroutines := flag.Int("goroutines", 64, "number of concurrent goroutines")
	poolSize := flag.Int("pool", 0, "connection pool size (default: goroutines)")
	readRatio := flag.Float64("read-ratio", 0.5, "read ratio for mix mode (0.0-1.0)")
	keySpace := flag.Int("key-space", 0, "unique key space (default: totalOps/10)")
	flag.Parse()

	ks, vs := *keySize, *valSize
	if ks == 0 || vs == 0 {
		p, ok := sizeProfiles[*profile]
		if !ok {
			fmt.Printf("unknown profile %q, using small\n", *profile)
			p = sizeProfiles["small"]
		}
		if ks == 0 {
			ks = p.keySize
		}
		if vs == 0 {
			vs = p.valSize
		}
	}

	ps := *poolSize
	if ps == 0 {
		ps = *goroutines
	}

	nKeys := *keySpace
	if nKeys == 0 {
		nKeys = *totalOps / 10
		if nKeys < 1000 {
			nKeys = 1000
		}
	}

	title := fmt.Sprintf("mcache OPS Benchmark — %s | key=%dB val=%dB (%s) | %d goroutines",
		*mode, ks, vs, formatBytes(vs), *goroutines)
	fmt.Println(strings.Repeat("═", 80))
	fmt.Println("  " + title)
	fmt.Println(strings.Repeat("═", 80))
	fmt.Printf("  target:  %s\n", *addr)
	fmt.Printf("  mode:    %s\n", *mode)
	fmt.Printf("  profile: %s (key=%dB, val=%dB, payload=%s)\n", *profile, ks, vs, formatBytes(ks+vs))
	fmt.Printf("  ops:     %d  | goroutines: %d  | pool: %d  | keyspace: %d\n\n", *totalOps, *goroutines, ps, nKeys)

	cfg := benchConfig{
		addr:       *addr,
		mode:       *mode,
		keySize:    ks,
		valSize:    vs,
		totalOps:   *totalOps,
		goroutines: *goroutines,
		poolSize:   ps,
		readRatio:  *readRatio,
		keySpace:   nKeys,
	}

	result := runBenchmark(cfg)
	result.print()
}

type benchConfig struct {
	addr                       string
	mode                       string
	keySize, valSize           int
	totalOps, goroutines       int
	poolSize                   int
	readRatio                  float64
	keySpace                   int
}

type benchResult struct {
	opsPerSec  float64
	bandwidth  float64 // MB/s
	latencies  []time.Duration
	totalOps   int
	goroutines int
}

func (r *benchResult) print() {
	if len(r.latencies) == 0 {
		fmt.Println("  No results collected.")
		return
	}

	sort.Slice(r.latencies, func(i, j int) bool {
		return r.latencies[i] < r.latencies[j]
	})

	n := len(r.latencies)
	avg := avgDur(r.latencies)
	p50 := r.latencies[n*50/100]
	p95 := r.latencies[n*95/100]
	p99 := r.latencies[n*99/100]
	p999 := r.latencies[n*999/1000]
	minL := r.latencies[0]
	maxL := r.latencies[n-1]

	fmt.Println(strings.Repeat("─", 80))
	fmt.Printf("  Throughput:   %10.0f ops/s\n", r.opsPerSec)
	fmt.Printf("  Bandwidth:    %10.2f MB/s\n", r.bandwidth)
	fmt.Println()
	fmt.Printf("  Latency (μs):  avg %8.0f  |  p50 %8.0f  |  p95 %8.0f  |  p99 %8.0f  |  p999 %8.0f\n",
		float64(avg)/float64(time.Microsecond),
		float64(p50)/float64(time.Microsecond),
		float64(p95)/float64(time.Microsecond),
		float64(p99)/float64(time.Microsecond),
		float64(p999)/float64(time.Microsecond))
	fmt.Printf("  Latency (μs):  min %8.0f  |  max %8.0f\n",
		float64(minL)/float64(time.Microsecond),
		float64(maxL)/float64(time.Microsecond))
	fmt.Println(strings.Repeat("═", 80))
}

func runBenchmark(cfg benchConfig) benchResult {
	switch cfg.mode {
	case "get":
		return benchGet(cfg)
	case "mix":
		return benchMix(cfg)
	default:
		return benchSet(cfg)
	}
}

func benchSet(cfg benchConfig) benchResult {
	c, err := sdk.NewClient(cfg.addr, sdk.WithPoolSize(cfg.poolSize), sdk.WithCodec(sdk.RawCodec{}))
	if err != nil {
		fmt.Println("  connect error:", err)
		return benchResult{}
	}
	defer c.Close()

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
			key := make([]byte, cfg.keySize)
			val := fillBytes(make([]byte, cfg.valSize))
			ready.Done()
			<-start
			base := gid * opsPerG
			for i := 0; i < opsPerG; i++ {
				k := genKey(key, rng.Intn(cfg.keySpace), cfg.keySize)
				t0 := time.Now()
				_ = c.Set(k, val, 0)
				lats[base+i] = time.Since(t0)
			}
		}(gid)
	}

	ready.Wait()
	t0 := time.Now()
	close(start)
	wg.Wait()
	elapsed := time.Since(t0).Seconds()

	return buildResult(lats, cfg.totalOps, elapsed, cfg.keySize+cfg.valSize)
}

func benchGet(cfg benchConfig) benchResult {
	c, err := sdk.NewClient(cfg.addr, sdk.WithPoolSize(cfg.poolSize), sdk.WithCodec(sdk.RawCodec{}))
	if err != nil {
		fmt.Println("  connect error:", err)
		return benchResult{}
	}
	defer c.Close()

	val := fillBytes(make([]byte, cfg.valSize))

	// Pre-populate.
	fmt.Printf("  populating %d keys...\n", cfg.keySpace)
	for i := 0; i < cfg.keySpace; i++ {
		k := genKey(nil, i, cfg.keySize)
		_ = c.Set(k, val, 0)
	}
	fmt.Println("  running GET benchmark...")

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
			ready.Done()
			<-start
			base := gid * opsPerG
			for i := 0; i < opsPerG; i++ {
				k := genKey(nil, rng.Intn(cfg.keySpace), cfg.keySize)
				t0 := time.Now()
				var v []byte
				_ = c.Get(k, &v)
				lats[base+i] = time.Since(t0)
			}
		}(gid)
	}

	ready.Wait()
	t0 := time.Now()
	close(start)
	wg.Wait()
	elapsed := time.Since(t0).Seconds()

	return buildResult(lats, cfg.totalOps, elapsed, cfg.keySize+cfg.valSize)
}

func benchMix(cfg benchConfig) benchResult {
	c, err := sdk.NewClient(cfg.addr, sdk.WithPoolSize(cfg.poolSize), sdk.WithCodec(sdk.RawCodec{}))
	if err != nil {
		fmt.Println("  connect error:", err)
		return benchResult{}
	}
	defer c.Close()

	val := fillBytes(make([]byte, cfg.valSize))

	fmt.Printf("  populating %d keys...\n", cfg.keySpace)
	for i := 0; i < cfg.keySpace; i++ {
		k := genKey(nil, i, cfg.keySize)
		_ = c.Set(k, val, 0)
	}
	fmt.Println("  running MIX benchmark...")

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
			ready.Done()
			<-start
			base := gid * opsPerG
			for i := 0; i < opsPerG; i++ {
				k := genKey(nil, rng.Intn(cfg.keySpace), cfg.keySize)
				t0 := time.Now()
				if rng.Float64() < cfg.readRatio {
					var v []byte
					_ = c.Get(k, &v)
				} else {
					_ = c.Set(k, val, 0)
				}
				lats[base+i] = time.Since(t0)
			}
		}(gid)
	}

	ready.Wait()
	t0 := time.Now()
	close(start)
	wg.Wait()
	elapsed := time.Since(t0).Seconds()

	return buildResult(lats, cfg.totalOps, elapsed, cfg.keySize+cfg.valSize)
}

func buildResult(lats []time.Duration, totalOps int, elapsed float64, payloadBytes int) benchResult {
	bandwidth := float64(totalOps*payloadBytes) / elapsed / (1 << 20)
	return benchResult{
		opsPerSec:  float64(totalOps) / elapsed,
		bandwidth:  bandwidth,
		latencies:  lats,
		totalOps:   totalOps,
	}
}

func genKey(buf []byte, i int, size int) string {
	if buf == nil {
		buf = make([]byte, size)
	}
	// Write the integer as a hex-padded prefix for uniqueness, fill rest with 'k'.
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

func avgDur(ds []time.Duration) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	var sum int64
	for _, d := range ds {
		sum += int64(d)
	}
	return time.Duration(sum / int64(len(ds)))
}

func formatBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}
