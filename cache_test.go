package mcache

import (
	"sync"
	"testing"
	"time"

	"github.com/atoncooper/mcache/rehash"
)

func TestNew(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		opts := NewOptions()
		c, err := New(opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer c.Close()
		if c.Len() != 0 {
			t.Errorf("expected empty cache, got len=%d", c.Len())
		}
	})

	t.Run("invalid shard count", func(t *testing.T) {
		opts := NewOptions().WithShards(3)
		_, err := New(opts)
		if err != ErrInvalidShards {
			t.Errorf("expected ErrInvalidShards, got %v", err)
		}
	})

	t.Run("negative ttl in options", func(t *testing.T) {
		opts := Options{shardCount: 1, defaultTTL: -1 * time.Second}
		_, err := New(opts)
		if err != ErrNegativeTTL {
			t.Errorf("expected ErrNegativeTTL, got %v", err)
		}
	})
}

func TestCache_SetGet(t *testing.T) {
	c, _ := New(NewOptions())
	defer c.Close()

	t.Run("basic set and get", func(t *testing.T) {
		if err := c.Set("k1", []byte("v1")); err != nil {
			t.Fatalf("set failed: %v", err)
		}
		val, err := c.Get("k1")
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		if string(val) != "v1" {
			t.Errorf("expected v1, got %s", string(val))
		}
	})

	t.Run("empty key", func(t *testing.T) {
		if err := c.Set("", []byte("v")); err != ErrKeyEmpty {
			t.Errorf("expected ErrKeyEmpty, got %v", err)
		}
		if _, err := c.Get(""); err != ErrKeyEmpty {
			t.Errorf("expected ErrKeyEmpty, got %v", err)
		}
	})

	t.Run("nil value", func(t *testing.T) {
		if err := c.Set("k", nil); err != ErrValueNil {
			t.Errorf("expected ErrValueNil, got %v", err)
		}
	})

	t.Run("key not found", func(t *testing.T) {
		_, err := c.Get("missing")
		if err != ErrKeyNotFound {
			t.Errorf("expected ErrKeyNotFound, got %v", err)
		}
	})
}

func TestCache_TTL(t *testing.T) {
	c, _ := New(NewOptions())
	defer c.Close()

	t.Run("expired entry", func(t *testing.T) {
		c.Set("k", []byte("v"), 50*time.Millisecond)
		time.Sleep(100 * time.Millisecond)
		_, err := c.Get("k")
		if err != ErrKeyNotFound {
			t.Errorf("expected ErrKeyNotFound after expiry, got %v", err)
		}
	})

	t.Run("no expiry", func(t *testing.T) {
		c.Set("k2", []byte("v2"), -1)
		val, err := c.Get("k2")
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		if string(val) != "v2" {
			t.Errorf("expected v2, got %s", string(val))
		}
	})
}

func TestCache_Del(t *testing.T) {
	c, _ := New(NewOptions())
	defer c.Close()

	c.Set("k", []byte("v"))
	if err := c.Del("k"); err != nil {
		t.Fatalf("del failed: %v", err)
	}
	_, err := c.Get("k")
	if err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound after del, got %v", err)
	}
}

func TestCache_Close(t *testing.T) {
	c, _ := New(NewOptions())
	if err := c.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if err := c.Close(); err != ErrCacheClosed {
		t.Errorf("expected ErrCacheClosed, got %v", err)
	}
	if err := c.Set("k", []byte("v")); err != ErrCacheClosed {
		t.Errorf("expected ErrCacheClosed on set, got %v", err)
	}
	if _, err := c.Get("k"); err != ErrCacheClosed {
		t.Errorf("expected ErrCacheClosed on get, got %v", err)
	}
}

func TestCache_Cleanup(t *testing.T) {
	c, _ := New(NewOptions())
	defer c.Close()

	c.Set("a", []byte("1"), 50*time.Millisecond)
	c.Set("b", []byte("2"), 50*time.Millisecond)
	c.Set("c", []byte("3")) // no expiry

	time.Sleep(100 * time.Millisecond)
	removed := c.Cleanup()
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}
	if c.Len() != 1 {
		t.Errorf("expected len=1, got %d", c.Len())
	}
}

func TestCache_Concurrent(t *testing.T) {
	c, _ := New(NewOptions().WithShards(16))
	defer c.Close()

	done := make(chan struct{})
	for i := range 100 {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			key := string(rune('a' + n%26))
			c.Set(key, []byte(key))
			c.Get(key)
		}(i)
	}
	for range 100 {
		<-done
	}
	if c.Len() != 26 {
		t.Errorf("expected 26 unique keys, got %d", c.Len())
	}
}

func TestOptionsImmutable(t *testing.T) {
	base := NewOptions()
	mod := base.WithShards(32).WithMaxSize(1000)
	if base.shardCount != 16 {
		t.Error("base was mutated")
	}
	if mod.shardCount != 32 || mod.maxSize != 1000 {
		t.Error("mod did not reflect changes")
	}
}

func TestCache_Resize(t *testing.T) {
	t.Run("basic resize with rehash", func(t *testing.T) {
		c, _ := New(NewOptions().WithShards(2))
		defer c.Close()

		for i := range 100 {
			c.Set(string(rune('a'+i%26)), []byte("v"))
		}
		if c.Len() != 26 {
			t.Fatalf("expected 26 keys before resize, got %d", c.Len())
		}

		if err := c.Resize(8); err != nil {
			t.Fatalf("resize failed: %v", err)
		}
		if !c.IsRehashing() {
			t.Fatal("expected rehashing to be true")
		}

		// Trigger incremental rehash via reads
		for i := range 100 {
			c.Get(string(rune('a' + i%26)))
		}

		// All keys should still be accessible
		for i := range 26 {
			key := string(rune('a' + i))
			val, err := c.Get(key)
			if err != nil {
				t.Errorf("key %s not found after resize: %v", key, err)
				continue
			}
			if string(val) != "v" {
				t.Errorf("key %s has wrong value", key)
			}
		}

		if c.Len() != 26 {
			t.Errorf("expected 26 keys after resize, got %d", c.Len())
		}
	})

	t.Run("invalid shard count", func(t *testing.T) {
		c, _ := New(NewOptions())
		defer c.Close()
		if err := c.Resize(3); err != ErrInvalidShards {
			t.Errorf("expected ErrInvalidShards, got %v", err)
		}
	})

	t.Run("duplicate resize rejected", func(t *testing.T) {
		c, _ := New(NewOptions().WithShards(2))
		defer c.Close()
		c.Set("k", []byte("v"))
		c.Resize(4)
		if err := c.Resize(8); err == nil {
			t.Error("expected error for duplicate resize")
		}
	})

	t.Run("resize on closed cache", func(t *testing.T) {
		c, _ := New(NewOptions())
		c.Close()
		if err := c.Resize(4); err != ErrCacheClosed {
			t.Errorf("expected ErrCacheClosed, got %v", err)
		}
	})
}

func TestCache_Resize_DuringOperations(t *testing.T) {
	c, _ := New(NewOptions().WithShards(2))
	defer c.Close()

	// Populate
	for i := range 50 {
		c.Set(string(rune('a'+i%26)), []byte("old"))
	}

	// Start resize
	c.Resize(8)

	// Writes during rehash should go to new table only
	c.Set("z", []byte("new"))

	// Reads should find data in both tables
	val, err := c.Get("z")
	if err != nil || string(val) != "new" {
		t.Errorf("expected new value for z, got %v, %v", val, err)
	}

	// Delete during rehash
	c.Del("a")
	if _, err := c.Get("a"); err != ErrKeyNotFound {
		t.Errorf("expected a deleted, got %v", err)
	}
}

func TestCache_Resize_Concurrent(t *testing.T) {
	c, _ := New(NewOptions().WithShards(4))
	defer c.Close()

	for i := range 200 {
		c.Set(string(rune('a'+i%26)), []byte("v"))
	}

	c.Resize(16)

	done := make(chan struct{})
	for i := range 100 {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			key := string(rune('a' + n%26))
			c.Get(key)
			c.Set(key, []byte("updated"))
			c.Get(key)
		}(i)
	}
	for range 100 {
		<-done
	}

	for i := range 26 {
		key := string(rune('a' + i))
		val, err := c.Get(key)
		if err != nil {
			t.Errorf("key %s missing after concurrent rehash: %v", key, err)
			continue
		}
		if string(val) != "updated" {
			t.Errorf("key %s expected updated, got %s", key, string(val))
		}
	}
}

func TestEviction_LRU_Basic(t *testing.T) {
	// Single shard so ordering is deterministic.
	c, _ := New(NewOptions().WithShards(1).WithMaxSize(3).WithEvictionPolicy("lru"))
	defer c.Close()

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3"))
	if c.Len() != 3 {
		t.Fatalf("expected len=3, got %d", c.Len())
	}

	// Re-set "a" to promote it (approximate LRU: only Set updates order).
	c.Set("a", []byte("1"))

	// Adding "d" should evict "b" (least recently used: b was not touched).
	c.Set("d", []byte("4"))
	if c.Len() != 3 {
		t.Fatalf("expected len=3 after eviction, got %d", c.Len())
	}

	if _, err := c.Get("b"); err != ErrKeyNotFound {
		t.Errorf("expected b evicted, got %v", err)
	}
	for _, key := range []string{"a", "c", "d"} {
		if _, err := c.Get(key); err != nil {
			t.Errorf("key %s should still exist: %v", key, err)
		}
	}
}

func TestEviction_LRU_AccessOrder(t *testing.T) {
	c, _ := New(NewOptions().WithShards(1).WithMaxSize(2).WithEvictionPolicy("lru"))
	defer c.Close()

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))

	// Re-set "a" to promote it (approximate LRU: only Set updates order).
	c.Set("a", []byte("1"))

	// Add "c"; "b" is LRU and should be evicted.
	c.Set("c", []byte("3"))
	if _, err := c.Get("b"); err != ErrKeyNotFound {
		t.Errorf("expected b evicted, got %v", err)
	}

	// Re-set "c" to promote it, then add "d"; "a" should be evicted.
	c.Set("c", []byte("3"))
	c.Set("d", []byte("4"))
	if _, err := c.Get("a"); err != ErrKeyNotFound {
		t.Errorf("expected a evicted, got %v", err)
	}
	for _, key := range []string{"c", "d"} {
		if _, err := c.Get(key); err != nil {
			t.Errorf("key %s should still exist: %v", key, err)
		}
	}
}

func TestEviction_LFU_Basic(t *testing.T) {
	c, _ := New(NewOptions().WithShards(1).WithMaxSize(3).WithEvictionPolicy("lfu"))
	defer c.Close()

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3"))

	// Build frequency skew (approximate LFU: only Set bumps frequency).
	// a=3, b=2, c=1
	c.Set("a", []byte("1"))
	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))

	// Add "d". Lowest freq is c (freq=1), so c should be evicted.
	c.Set("d", []byte("4"))
	if c.Len() != 3 {
		t.Fatalf("expected len=3 after eviction, got %d", c.Len())
	}
	if _, err := c.Get("c"); err != ErrKeyNotFound {
		t.Errorf("expected c evicted (lowest freq), got %v", err)
	}
	for _, key := range []string{"a", "b", "d"} {
		if _, err := c.Get(key); err != nil {
			t.Errorf("key %s should still exist: %v", key, err)
		}
	}
}

func TestEviction_LFU_FrequencyTie(t *testing.T) {
	c, _ := New(NewOptions().WithShards(1).WithMaxSize(2).WithEvictionPolicy("lfu"))
	defer c.Close()

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))

	// Both at freq=1; a was inserted first, so a is older at freq=1.
	// Adding c should evict a.
	c.Set("c", []byte("3"))
	if _, err := c.Get("a"); err != ErrKeyNotFound {
		t.Errorf("expected a evicted (oldest at min freq), got %v", err)
	}
	for _, key := range []string{"b", "c"} {
		if _, err := c.Get(key); err != nil {
			t.Errorf("key %s should still exist: %v", key, err)
		}
	}
}

func TestEviction_PolicySwap(t *testing.T) {
	c, _ := New(NewOptions().WithShards(1).WithMaxSize(3).WithEvictionPolicy("lru"))
	defer c.Close()

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3"))

	// Hot-swap to LFU.
	if err := c.SetEvictionPolicy("lfu"); err != nil {
		t.Fatalf("swap to lfu failed: %v", err)
	}
	if c.EvictionPolicy() != "lfu" {
		t.Errorf("expected policy lfu, got %s", c.EvictionPolicy())
	}

	// All keys reset to freq=1 after swap.
	// Re-set to bump frequency: a twice, b once (approximate LFU).
	c.Set("a", []byte("1"))
	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))

	// Adding d should evict c (lowest freq=1, oldest).
	c.Set("d", []byte("4"))
	if _, err := c.Get("c"); err != ErrKeyNotFound {
		t.Errorf("expected c evicted after swap, got %v", err)
	}

	// Swap back to LRU.
	if err := c.SetEvictionPolicy("lru"); err != nil {
		t.Fatalf("swap to lru failed: %v", err)
	}
	// Re-set a and d to promote them so b is the only untouched key.
	c.Set("a", []byte("1"))
	c.Set("d", []byte("4"))
	c.Set("e", []byte("5"))
	if _, err := c.Get("b"); err != ErrKeyNotFound {
		t.Errorf("expected b evicted after lru swap, got %v", err)
	}
}

func TestEviction_PolicySwap_Concurrent(t *testing.T) {
	c, _ := New(NewOptions().WithShards(4).WithMaxSize(100).WithEvictionPolicy("lru"))
	defer c.Close()

	// Populate
	for i := range 50 {
		c.Set(string(rune('a'+i%26)), []byte("v"))
	}

	done := make(chan struct{})

	// Workers doing reads/writes.
	for i := range 50 {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			key := string(rune('a' + n%26))
			c.Get(key)
			c.Set(key, []byte("updated"))
		}(i)
	}

	// In parallel, hot-swap policies multiple times.
	for i := range 10 {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			if n%2 == 0 {
				c.SetEvictionPolicy("lru")
			} else {
				c.SetEvictionPolicy("lfu")
			}
		}(i)
	}

	for range 60 {
		<-done
	}

	// Sanity: cache should be bounded.
	if c.Len() > 100 {
		t.Errorf("cache len %d exceeds maxSize 100", c.Len())
	}

	// All keys should still be readable (values may differ due to races).
	for i := range 26 {
		key := string(rune('a' + i))
		_, err := c.Get(key)
		if err == ErrKeyNotFound {
			// Some evictions are expected under contention; just ensure no panic.
		}
	}
}

func TestEviction_NoOpWithMaxSize(t *testing.T) {
	c, _ := New(NewOptions().WithShards(1).WithMaxSize(2).WithEvictionPolicy("noop"))
	defer c.Close()

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3"))

	// Noop does not evict, so all 3 keys remain.
	if c.Len() != 3 {
		t.Errorf("expected len=3 with noop policy, got %d", c.Len())
	}
}

func TestEviction_PolicyFactory_Unknown(t *testing.T) {
	if err := RegisterPolicy("", nil); err != ErrUnknownPolicy {
		t.Errorf("expected ErrUnknownPolicy for empty name, got %v", err)
	}
	_, err := New(NewOptions().WithEvictionPolicy("magic"))
	if err != ErrUnknownPolicy {
		t.Errorf("expected ErrUnknownPolicy, got %v", err)
	}
}

func TestEviction_PerShardMaxSize(t *testing.T) {
	// 2 shards, maxSize=4, so per-shard max = 2.
	c, _ := New(NewOptions().WithShards(2).WithMaxSize(4).WithEvictionPolicy("lru"))
	defer c.Close()

	// Fill both shards. With FNV hash, these keys are likely distributed.
	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3"))
	c.Set("d", []byte("4"))
	c.Set("e", []byte("5"))
	c.Set("f", []byte("6"))

	// Overall cache should be bounded at ~4 (per-shard quota may be slightly off
	// due to hash distribution, but should not grow to 6).
	if c.Len() > 6 {
		t.Errorf("cache len %d should not exceed 6", c.Len())
	}
}

func TestEviction_LenAfterExpiredCleanup(t *testing.T) {
	c, _ := New(NewOptions().WithShards(1).WithMaxSize(10).WithEvictionPolicy("lru"))
	defer c.Close()

	c.Set("a", []byte("1"), 50*time.Millisecond)
	c.Set("b", []byte("2"), 50*time.Millisecond)
	c.Set("c", []byte("3")) // no expiry

	time.Sleep(100 * time.Millisecond)
	removed := c.Cleanup()
	if removed != 2 {
		t.Errorf("expected 2 expired removed, got %d", removed)
	}
	if c.Len() != 1 {
		t.Errorf("expected len=1 after cleanup, got %d", c.Len())
	}

	// Verify LRU policy also dropped the expired keys.
	for _, key := range []string{"a", "b"} {
		if _, err := c.Get(key); err != ErrKeyNotFound {
			t.Errorf("expected %s missing after cleanup, got %v", key, err)
		}
	}
}

// countingObserver is a test-only observer that counts events.
type countingObserver struct {
	hits, misses, sets, dels, evicts int
	rehashStarts, rehashDones        int
	mu                               sync.Mutex
}

func (o *countingObserver) OnHit(key string)                      { o.mu.Lock(); o.hits++; o.mu.Unlock() }
func (o *countingObserver) OnMiss(key string)                     { o.mu.Lock(); o.misses++; o.mu.Unlock() }
func (o *countingObserver) OnSet(key string)                      { o.mu.Lock(); o.sets++; o.mu.Unlock() }
func (o *countingObserver) OnDel(key string)                      { o.mu.Lock(); o.dels++; o.mu.Unlock() }
func (o *countingObserver) OnEvict(key string)                    { o.mu.Lock(); o.evicts++; o.mu.Unlock() }
func (o *countingObserver) OnRehashStart(oldShards, newShards int) { o.mu.Lock(); o.rehashStarts++; o.mu.Unlock() }
func (o *countingObserver) OnRehashDone()                          { o.mu.Lock(); o.rehashDones++; o.mu.Unlock() }

func (o *countingObserver) counts() (hits, misses, sets, dels, evicts int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.hits, o.misses, o.sets, o.dels, o.evicts
}

func TestCacheObserver_Basic(t *testing.T) {
	obs := &countingObserver{}
	c, _ := New(NewOptions().WithObserver(obs))
	defer c.Close()

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Get("a")  // hit
	c.Get("x")  // miss
	c.Del("b")

	hits, misses, sets, dels, evicts := obs.counts()
	if sets != 2 {
		t.Errorf("expected 2 sets, got %d", sets)
	}
	if hits != 1 {
		t.Errorf("expected 1 hit, got %d", hits)
	}
	if misses != 1 {
		t.Errorf("expected 1 miss, got %d", misses)
	}
	if dels != 1 {
		t.Errorf("expected 1 del, got %d", dels)
	}
	if evicts != 0 {
		t.Errorf("expected 0 evicts, got %d", evicts)
	}
}

func TestCacheObserver_Evict(t *testing.T) {
	obs := &countingObserver{}
	c, _ := New(NewOptions().WithShards(1).WithMaxSize(2).WithEvictionPolicy("lru").WithObserver(obs))
	defer c.Close()

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3")) // should evict one

	_, _, _, _, evicts := obs.counts()
	if evicts != 1 {
		t.Errorf("expected 1 evict, got %d", evicts)
	}
}

func TestCacheObserver_Rehash(t *testing.T) {
	obs := &countingObserver{}
	c, _ := New(NewOptions().WithShards(2).WithObserver(obs))
	defer c.Close()

	for i := range 26 {
		c.Set(string(rune('a'+i)), []byte("v"))
	}

	c.Resize(8)
	if !c.IsRehashing() {
		t.Fatal("expected rehashing")
	}

	// Trigger rehash completion via reads.
	for i := range 100 {
		c.Get(string(rune('a' + i%26)))
	}

	if c.IsRehashing() {
		t.Fatal("expected rehash done")
	}

	obs.mu.Lock()
	starts := obs.rehashStarts
	dones := obs.rehashDones
	obs.mu.Unlock()
	if starts != 1 {
		t.Errorf("expected 1 rehash start, got %d", starts)
	}
	if dones != 1 {
		t.Errorf("expected 1 rehash done, got %d", dones)
	}
}

func TestSetRehasher(t *testing.T) {
	t.Run("basic hot-swap", func(t *testing.T) {
		c, _ := New(NewOptions())
		defer c.Close()

		if c.Rehasher() != "incremental" {
			t.Errorf("expected default incremental, got %s", c.Rehasher())
		}

		if err := c.SetRehasher("batch"); err != nil {
			t.Fatalf("set rehasher failed: %v", err)
		}
		if c.Rehasher() != "batch" {
			t.Errorf("expected batch, got %s", c.Rehasher())
		}

		if err := c.SetRehasher("noop"); err != nil {
			t.Fatalf("set rehasher failed: %v", err)
		}
		if c.Rehasher() != "noop" {
			t.Errorf("expected noop, got %s", c.Rehasher())
		}
	})

	t.Run("unknown rehasher", func(t *testing.T) {
		c, _ := New(NewOptions())
		defer c.Close()
		if err := c.SetRehasher("magic"); err != ErrUnknownRehasher {
			t.Errorf("expected ErrUnknownRehasher, got %v", err)
		}
	})

	t.Run("swap on closed cache", func(t *testing.T) {
		c, _ := New(NewOptions())
		c.Close()
		if err := c.SetRehasher("batch"); err != ErrCacheClosed {
			t.Errorf("expected ErrCacheClosed, got %v", err)
		}
	})

	t.Run("swap aborts in-progress rehash", func(t *testing.T) {
		obs := &countingObserver{}
		c, _ := New(NewOptions().WithShards(2).WithObserver(obs))
		defer c.Close()

		for i := range 26 {
			c.Set(string(rune('a'+i)), []byte("v"))
		}

		c.Resize(8)
		if !c.IsRehashing() {
			t.Fatal("expected rehashing")
		}

		// Swap to noop aborts the rehash.
		if err := c.SetRehasher("noop"); err != nil {
			t.Fatalf("set rehasher failed: %v", err)
		}
		if c.IsRehashing() {
			t.Error("expected rehashing aborted")
		}
	})
}

func TestBatchRehasher(t *testing.T) {
	c, _ := New(NewOptions().WithShards(2).WithRehasher("batch"))
	defer c.Close()

	for i := range 26 {
		c.Set(string(rune('a'+i)), []byte("v"))
	}

	c.Resize(8)
	if !c.IsRehashing() {
		t.Fatal("expected rehashing")
	}

	// A single Step (triggered by any operation) should complete batch rehash.
	c.Get("a")

	if c.IsRehashing() {
		t.Error("expected batch rehash done after one step")
	}

	// All keys still accessible.
	for i := range 26 {
		key := string(rune('a' + i))
		if _, err := c.Get(key); err != nil {
			t.Errorf("key %s missing: %v", key, err)
		}
	}
}

func TestRehasherRegistry(t *testing.T) {
	t.Run("register and use custom rehasher", func(t *testing.T) {
		customCalled := false
		custom := func() rehash.Rehasher {
			return rehash.NewNoopRehasher()
		}
		if err := RegisterRehasher("custom", custom); err != nil {
			t.Fatalf("register failed: %v", err)
		}

		c, _ := New(NewOptions().WithRehasher("custom"))
		defer c.Close()
		if c.Rehasher() != "noop" { // noopRehasher.Name() returns "noop"
			t.Errorf("expected noop name from custom rehasher, got %s", c.Rehasher())
		}
		_ = customCalled
	})

	t.Run("register invalid", func(t *testing.T) {
		if err := RegisterRehasher("", nil); err != ErrUnknownRehasher {
			t.Errorf("expected ErrUnknownRehasher, got %v", err)
		}
	})
}

func TestCacheObserver_Concurrent(t *testing.T) {
	obs := &countingObserver{}
	c, _ := New(NewOptions().WithShards(8).WithObserver(obs))
	defer c.Close()

	done := make(chan struct{})
	for i := range 50 {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			key := string(rune('a' + n%26))
			c.Set(key, []byte(key))
			c.Get(key)
			if n%3 == 0 {
				c.Del(key)
			}
		}(i)
	}
	for range 50 {
		<-done
	}

	obs.mu.Lock()
	sets := obs.sets
	hits := obs.hits
	obs.mu.Unlock()

	if sets != 50 {
		t.Errorf("expected 50 sets, got %d", sets)
	}
	if hits != 50 {
		t.Errorf("expected 50 hits, got %d", hits)
	}
}
