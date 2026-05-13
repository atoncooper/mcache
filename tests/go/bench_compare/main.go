// Package main benchmarks mcache against Redis side-by-side.
// Uses the exact same code paths as bench_mcache (verified working).
//
// Usage:
//
//	go run ./bench_compare --mcache 127.0.0.1:11211 --redis 127.0.0.1:6379 --mode set --profile small
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
	keySize := flag.Int("key-size", 0, "key size override")
	valSize := flag.Int("val-size", 0, "value size override")
	totalOps := flag.Int("ops", 100_000, "total operations")
	goroutines := flag.Int("goroutines", 64, "concurrent goroutines")
	pipelineSize := flag.Int("pipeline", 0, "mcache pipeline batch size (0=same as bench_mcache default)")
	readRatio := flag.Float64("read-ratio", 0.5, "read ratio for mix mode")
	keySpace := flag.Int("key-space", 0, "unique key space")
	flag.Parse()

	ks, vs := *keySize, *valSize
	if ks == 0 || vs == 0 {
		p, ok := sizeProfiles[*profile]
		if !ok {
			p = sizeProfiles["small"]
		}
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

	ps := *pipelineSize
	usePipeline := ps > 0

	fmt.Println(strings.Repeat("=", 90))
	pipLabel := "off"
	if usePipeline {
		pipLabel = fmt.Sprintf("%d", ps)
	}
	fmt.Printf("  mcache vs Redis — %s | %dB key + %dB val | %d goroutines | pipeline=%s\n",
		*mode, ks, vs, *goroutines, pipLabel)
	fmt.Println(strings.Repeat("=", 90))

	// Fetch and display mcache server state before test
	printMcacheState("  [mcache state before]", *mcacheAddr)

	// --- Run benchmarks (same logic as bench_mcache + bench_redis) ---
	mResult := runMcache(*mcacheAddr, *mode, ks, vs, *totalOps, *goroutines, ps, *readRatio, nKeys)
	rResult := runRedis(*redisAddr, *mode, ks, vs, *totalOps, *goroutines, *readRatio, nKeys)

	// Fetch and display mcache server state after test
	printMcacheState("  [mcache state after ]", *mcacheAddr)

	// Print comparison
	fmt.Println()
	fmt.Println(strings.Repeat("─", 90))
	fmt.Printf("  %-18s │ %14s │ %14s │ %10s\n", "metric", "mcache", "redis", "ratio")
	fmt.Println(strings.Repeat("─", 90))

	if rResult.opsPerSec > 0 {
		ratio := mResult.opsPerSec / rResult.opsPerSec * 100
		mark := verdict(ratio)
		fmt.Printf("  %-18s │ %14.0f │ %14.0f │ %7.0f%% %s\n",
			"throughput (ops/s)", mResult.opsPerSec, rResult.opsPerSec, ratio, mark)
	}
	if mResult.avgUs > 0 && rResult.avgUs > 0 {
		fmt.Printf("  %-18s │ %12.0fμs │ %12.0fμs │ %10s\n",
			"avg latency", mResult.avgUs, rResult.avgUs, "")
	}
	if mResult.p50Us > 0 && rResult.p50Us > 0 {
		fmt.Printf("  %-18s │ %12.0fμs │ %12.0fμs │ %10s\n",
			"p50 latency", mResult.p50Us, rResult.p50Us, "")
	}
	if mResult.p99Us > 0 && rResult.p99Us > 0 {
		fmt.Printf("  %-18s │ %12.0fμs │ %12.0fμs │ %10s\n",
			"p99 latency", mResult.p99Us, rResult.p99Us, "")
	}
	fmt.Println(strings.Repeat("─", 90))
	if rResult.opsPerSec > 0 {
		ratio := mResult.opsPerSec / rResult.opsPerSec
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
}

type result struct {
	opsPerSec float64
	avgUs     float64
	p50Us     float64
	p99Us     float64
}

// ---------------------------------------------------------------------------
// mcache benchmark — EXACT same logic as bench_mcache/main.go benchSet/benchGet/benchMix
// ---------------------------------------------------------------------------

func runMcache(addr, mode string, ks, vs, totalOps, goroutines, pipelineSize int, readRatio float64, keySpace int) result {
	c, err := sdk.NewClient(addr, sdk.WithPoolSize(goroutines), sdk.WithCodec(sdk.RawCodec{}))
	if err != nil {
		fmt.Printf("  mcache connect error: %v\n", err)
		return result{}
	}
	defer c.Close()

	if mode == "get" || mode == "mix" {
		fmt.Printf("  populating mcache with %d keys...\n", keySpace)
		val := fillBytes(make([]byte, vs))
		for i := 0; i < keySpace; i++ {
			_ = c.Set(genKey(nil, i, ks), val, 0)
		}
		if mode == "get" {
			fmt.Println("  running GET benchmark...")
		} else {
			fmt.Println("  running MIX benchmark...")
		}
	}

	opsPerG := totalOps / goroutines
	lats := make([]time.Duration, totalOps)
	var wg sync.WaitGroup
	var ready sync.WaitGroup
	start := make(chan struct{})
	ready.Add(goroutines)

	if pipelineSize > 0 {
		batchSz := pipelineSize
		for gid := 0; gid < goroutines; gid++ {
			wg.Add(1)
			go func(gid int) {
				defer wg.Done()
				rng := rand.New(rand.NewSource(int64(gid)))
				val := fillBytes(make([]byte, vs))
				pipeline := c.Pipeline()
				ready.Done()
				<-start
				base := gid * opsPerG
				for i := 0; i < opsPerG; {
					end := min(i+batchSz, opsPerG)
					for j := i; j < end; j++ {
						k := genKey(nil, rng.Intn(keySpace), ks)
						switch mode {
						case "get":
							pipeline.AddGet(k)
						case "mix":
							if rng.Float64() < readRatio {
								pipeline.AddGet(k)
							} else {
								pipeline.AddSet(k, val, 0)
							}
						default:
							pipeline.AddSet(k, val, 0)
						}
					}
					t0 := time.Now()
					switch mode {
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
			}(gid)
		}
	} else {
		for gid := 0; gid < goroutines; gid++ {
			wg.Add(1)
			go func(gid int) {
				defer wg.Done()
				rng := rand.New(rand.NewSource(int64(gid)))
				key := make([]byte, ks)
				val := fillBytes(make([]byte, vs))
				ready.Done()
				<-start
				base := gid * opsPerG
				for i := 0; i < opsPerG; i++ {
					k := genKey(key, rng.Intn(keySpace), ks)
					t0 := time.Now()
					switch mode {
					case "get":
						var v []byte
						_ = c.Get(k, &v)
					case "mix":
						if rng.Float64() < readRatio {
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
			}(gid)
		}
	}

	ready.Wait()
	t0 := time.Now()
	close(start)
	wg.Wait()
	elapsed := time.Since(t0).Seconds()

	return buildResult(lats, totalOps, elapsed)
}

// ---------------------------------------------------------------------------
// Redis benchmark — same logic as bench_redis/main.go
// ---------------------------------------------------------------------------

func runRedis(addr, mode string, ks, vs, totalOps, goroutines int, readRatio float64, keySpace int) result {
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		PoolSize:     goroutines,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	})
	defer rdb.Close()
	ctx := context.Background()

	if mode == "get" || mode == "mix" {
		fmt.Printf("  populating redis with %d keys...\n", keySpace)
		val := fillBytes(make([]byte, vs))
		pipe := rdb.Pipeline()
		for i := 0; i < keySpace; i++ {
			pipe.Set(ctx, genKey(nil, i, ks), val, 0)
		}
		_, _ = pipe.Exec(ctx)
	}

	opsPerG := totalOps / goroutines
	lats := make([]time.Duration, totalOps)
	var wg sync.WaitGroup
	var ready sync.WaitGroup
	start := make(chan struct{})
	ready.Add(goroutines)

	for gid := 0; gid < goroutines; gid++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(gid)))
			val := fillBytes(make([]byte, vs))
			ready.Done()
			<-start
			base := gid * opsPerG
			for i := 0; i < opsPerG; i++ {
				k := genKey(nil, rng.Intn(keySpace), ks)
				t0 := time.Now()
				switch mode {
				case "get":
					_ = rdb.Get(ctx, k).Err()
				case "mix":
					if rng.Float64() < readRatio {
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
	t0 := time.Now()
	close(start)
	wg.Wait()
	elapsed := time.Since(t0).Seconds()

	return buildResult(lats, totalOps, elapsed)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func buildResult(lats []time.Duration, totalOps int, elapsed float64) result {
	if len(lats) == 0 || elapsed <= 0 {
		return result{}
	}
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	n := len(lats)
	var sum int64
	for _, d := range lats {
		sum += int64(d)
	}
	return result{
		opsPerSec: float64(totalOps) / elapsed,
		avgUs:     float64(sum/int64(time.Microsecond)) / float64(n),
		p50Us:     float64(lats[n*50/100] / time.Microsecond),
		p99Us:     float64(lats[n*99/100] / time.Microsecond),
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

func verdict(ratio float64) string {
	if ratio >= 0.95 {
		return "✓"
	} else if ratio >= 0.80 {
		return "~"
	}
	return "✗"
}

// --- Server stats query (uses direct mnet.Client, independent of benchmark) ---

func printMcacheState(label, addr string) {
	fmt.Printf("%s ", label)
	transport, err := mnet.NewClient(addr,
		mnet.WithPoolSize(1),
		mnet.WithDialTimeout(2*time.Second),
		mnet.WithClientReadTimeout(2*time.Second),
		mnet.WithClientWriteTimeout(2*time.Second))
	if err != nil {
		fmt.Println("(unreachable)")
		return
	}
	data, err := transport.Stats()
	transport.Close()
	if err != nil {
		fmt.Println("(stats failed)")
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
	fmt.Printf("uptime=%v  conns=%d  entries=%d  mem=%s  goroutines=%d  reqs=%d\n",
		uptime.Round(time.Second), s.Connections, s.CacheEntries,
		formatBytes(int(s.CacheMemory)), s.Goroutines, s.TotalRequests)
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
