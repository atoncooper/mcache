package hash

import (
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	h := New()
	if h == nil {
		t.Fatal("New returned nil")
	}
	if h.Fields() != 0 {
		t.Fatalf("expected 0 fields, got %d", h.Fields())
	}
}

func TestHSet_HGet(t *testing.T) {
	h := New()

	if added := h.HSet("f1", "v1"); added != 1 {
		t.Fatalf("expected 1 new field, got %d", added)
	}
	if added := h.HSet("f1", "v2"); added != 0 {
		t.Fatalf("expected 0 (update), got %d", added)
	}

	v, ok := h.HGet("f1")
	if !ok {
		t.Fatal("expected field to exist")
	}
	if v != "v2" {
		t.Fatalf("expected v2, got %s", v)
	}

	_, ok = h.HGet("nonexistent")
	if ok {
		t.Fatal("expected field not to exist")
	}
}

func TestHSetNX(t *testing.T) {
	h := New()

	if !h.HSetNX("f1", "v1") {
		t.Fatal("expected HSetNX to succeed")
	}
	if h.HSetNX("f1", "v2") {
		t.Fatal("expected HSetNX to fail on existing field")
	}

	v, _ := h.HGet("f1")
	if v != "v1" {
		t.Fatalf("expected v1, got %s", v)
	}
}

func TestHDel(t *testing.T) {
	h := New()
	h.HSet("f1", "v1")
	h.HSet("f2", "v2")
	h.HSet("f3", "v3")

	removed := h.HDel("f1", "f2", "nonexistent")
	if removed != 2 {
		t.Fatalf("expected 2 removed, got %d", removed)
	}
	if h.HLen() != 1 {
		t.Fatalf("expected 1 remaining, got %d", h.HLen())
	}
}

func TestHExists(t *testing.T) {
	h := New()
	h.HSet("f1", "v1")

	if !h.HExists("f1") {
		t.Fatal("expected field to exist")
	}
	if h.HExists("nonexistent") {
		t.Fatal("expected field not to exist")
	}
}

func TestHGetAll(t *testing.T) {
	h := New()
	h.HSet("a", "1")
	h.HSet("b", "2")

	all := h.HGetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}
	if all["a"] != "1" || all["b"] != "2" {
		t.Fatalf("unexpected values: %v", all)
	}

	// Verify immutability
	all["c"] = "3"
	if _, ok := h.HGet("c"); ok {
		t.Fatal("GetAll should return a copy")
	}
}

func TestHKeys(t *testing.T) {
	h := New()
	h.HSet("a", "1")
	h.HSet("b", "2")

	keys := h.HKeys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestHVals(t *testing.T) {
	h := New()
	h.HSet("a", "1")
	h.HSet("b", "2")

	vals := h.HVals()
	if len(vals) != 2 {
		t.Fatalf("expected 2 values, got %d", len(vals))
	}
}

func TestHLen(t *testing.T) {
	h := New()
	if h.HLen() != 0 {
		t.Fatal("expected 0")
	}
	h.HSet("a", "1")
	if h.HLen() != 1 {
		t.Fatal("expected 1")
	}
}

func TestHStrLen(t *testing.T) {
	h := New()
	h.HSet("name", "hello")

	if n := h.HStrLen("name"); n != 5 {
		t.Fatalf("expected 5, got %d", n)
	}
	if n := h.HStrLen("nonexistent"); n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestHIncrBy(t *testing.T) {
	h := New()

	n, err := h.HIncrBy("counter", 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}

	n, err = h.HIncrBy("counter", 5)
	if err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Fatalf("expected 6, got %d", n)
	}

	n, err = h.HIncrBy("counter", -2)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("expected 4, got %d", n)
	}

	// Error case: non-integer value
	h.HSet("bad", "not-a-number")
	_, err = h.HIncrBy("bad", 1)
	if err == nil {
		t.Fatal("expected error for non-integer value")
	}
}

func TestHIncrByFloat(t *testing.T) {
	h := New()

	n, err := h.HIncrByFloat("score", 1.5)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1.5 {
		t.Fatalf("expected 1.5, got %f", n)
	}

	n, err = h.HIncrByFloat("score", 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2.0 {
		t.Fatalf("expected 2.0, got %f", n)
	}

	h.HSet("bad", "abc")
	_, err = h.HIncrByFloat("bad", 1.0)
	if err == nil {
		t.Fatal("expected error for non-float value")
	}
}

func TestHMGet(t *testing.T) {
	h := New()
	h.HSet("a", "1")
	h.HSet("b", "2")

	result := h.HMGet("a", "b", "c")
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	if result[0] != "1" {
		t.Fatalf("expected 1, got %v", result[0])
	}
	if result[1] != "2" {
		t.Fatalf("expected 2, got %v", result[1])
	}
	if result[2] != nil {
		t.Fatalf("expected nil, got %v", result[2])
	}
}

func TestHMSet(t *testing.T) {
	h := New()
	h.HMSet("a", "1", "b", "2", "c", "3")

	if h.HLen() != 3 {
		t.Fatalf("expected 3 fields, got %d", h.HLen())
	}
	v, _ := h.HGet("a")
	if v != "1" {
		t.Fatalf("expected 1, got %s", v)
	}
}

func TestConcurrentAccess(t *testing.T) {
	h := New()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			h.HSet("f", "v")
			h.HGet("f")
			h.HExists("f")
			h.HLen()
			h.HKeys()
			h.HVals()
			h.HGetAll()
		}(i)
	}
	wg.Wait()
}
