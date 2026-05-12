// Package main runs the same three-tier bottleneck diagnostic as tests/python/diagnose.py
// but with the Go SDK (no GIL, no Python interpreter overhead). Run this on
// the mcache server box to determine whether the throughput ceiling observed
// from Python is caused by the Python client or by the mcache server itself.
//
// Usage:
//
//	cd tests/go/diagnose
//	go run . --mcache 127.0.0.1:11211
package main

import (
	"flag"
	"fmt"
	"sync"
	"time"

	sdk "github.com/atoncooper/mcache/sdk/go"
)

func makeValue(i int, size int) []byte {
	prefix := []byte(fmt.Sprintf("%016d-", i))
	if size <= len(prefix) {
		return prefix[:size]
	}
	out := make([]byte, size)
	copy(out, prefix)
	for j := len(prefix); j < size; j++ {
		out[j] = 'x'
	}
	return out
}

// Test 1: single goroutine, single connection — pure Go SDK ceiling.
func testSingle(addr string, n int) (float64, error) {
	c, err := sdk.NewClient(addr, sdk.WithPoolSize(1), sdk.WithCodec(sdk.RawCodec{}))
	if err != nil {
		return 0, err
	}
	defer c.Close()

	for i := 0; i < 1000; i++ {
		_ = c.Set(fmt.Sprintf("__diag1__%d", i), makeValue(i, 256), 0)
	}

	t0 := time.Now()
	for i := 0; i < n; i++ {
		_ = c.Set(fmt.Sprintf("__diag1__%d", i%1000), makeValue(i, 256), 0)
	}
	elapsed := time.Since(t0).Seconds()
	return float64(n) / elapsed, nil
}

// Test 2: N goroutines, each with its own Client/connection.
func testMulti(addr string, numClients, nPerClient int) (float64, error) {
	var wg sync.WaitGroup
	var ready sync.WaitGroup
	start := make(chan struct{})
	ready.Add(numClients)

	for cid := 0; cid < numClients; cid++ {
		wg.Add(1)
		go func(cid int) {
			defer wg.Done()
			c, err := sdk.NewClient(addr, sdk.WithPoolSize(1), sdk.WithCodec(sdk.RawCodec{}))
			if err != nil {
				ready.Done()
				return
			}
			defer c.Close()
			ready.Done()
			<-start
			for i := 0; i < nPerClient; i++ {
				_ = c.Set(fmt.Sprintf("__diag2__%d_%d", cid, i%1000), makeValue(i, 256), 0)
			}
		}(cid)
	}

	ready.Wait()
	t0 := time.Now()
	close(start)
	wg.Wait()
	elapsed := time.Since(t0).Seconds()
	return float64(numClients*nPerClient) / elapsed, nil
}

// Test 3: shared Client with varying pool_size, N goroutines.
func testPool(addr string, poolSize, numClients, nPerClient int) (float64, error) {
	c, err := sdk.NewClient(addr, sdk.WithPoolSize(poolSize), sdk.WithCodec(sdk.RawCodec{}))
	if err != nil {
		return 0, err
	}
	defer c.Close()

	var wg sync.WaitGroup
	start := make(chan struct{})

	for cid := 0; cid < numClients; cid++ {
		wg.Add(1)
		go func(cid int) {
			defer wg.Done()
			<-start
			for i := 0; i < nPerClient; i++ {
				_ = c.Set(fmt.Sprintf("__diag3__%d_%d", cid, i%1000), makeValue(i, 256), 0)
			}
		}(cid)
	}

	t0 := time.Now()
	close(start)
	wg.Wait()
	elapsed := time.Since(t0).Seconds()
	return float64(numClients*nPerClient) / elapsed, nil
}

func main() {
	addr := flag.String("mcache", "127.0.0.1:11211", "mcache address")
	flag.Parse()

	bar := "════════════════════════════════════════════════════════════"
	fmt.Println(bar)
	fmt.Println("  mcache 性能瓶颈三层诊断 (Go SDK — 无 GIL)")
	fmt.Println(bar)
	fmt.Printf("  目标: %s\n\n", *addr)

	// Test 1
	fmt.Println("[1] 单 goroutine SDK 极限 (10k ops)")
	ops1, err := testSingle(*addr, 10_000)
	if err != nil {
		fmt.Println("  连接失败:", err)
		return
	}
	fmt.Printf("    → %10.0f ops/s\n", ops1)
	fmt.Printf("    → 单次 op ≈ %.0f μs\n\n", 1_000_000/ops1)

	// Test 2
	fmt.Println("[2] 多 goroutine 扩展（每 goroutine 独立 Client）")
	fmt.Printf("    %8s %14s %14s %10s\n", "goroutines", "total ops/s", "per-goroutine", "扩展比")
	for _, nc := range []int{1, 4, 16, 64, 128} {
		ops, err := testMulti(*addr, nc, 500)
		if err != nil {
			fmt.Println("    err:", err)
			continue
		}
		fmt.Printf("    %8d %14.0f %14.0f %9.1fx\n", nc, ops, ops/float64(nc), ops/ops1)
	}
	fmt.Println()

	// Test 3
	fmt.Println("[3] 共享 Client + 不同 pool_size (128 goroutines)")
	fmt.Printf("    %10s %14s\n", "pool_size", "ops/s")
	for _, ps := range []int{1, 4, 16, 64, 128} {
		ops, err := testPool(*addr, ps, 128, 200)
		if err != nil {
			fmt.Println("    err:", err)
			continue
		}
		fmt.Printf("    %10d %14.0f\n", ps, ops)
	}
	fmt.Println()

	fmt.Println(bar)
	fmt.Println("  解读：")
	fmt.Println("  • 若 Test 2 峰值 ≫ Python diagnose.py 峰值 (~9k) → Python GIL/解释器是瓶颈")
	fmt.Println("  • 若 Test 2 峰值 ≈ Python 峰值 (~9k)              → mcache 服务端有序列化点")
	fmt.Println("  • 若 Test 1 已经 > 50k                            → 单连接吞吐就很高，扩展更猛")
	fmt.Println(bar)
}
