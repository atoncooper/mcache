package list

import (
	"sync"
	"testing"
)

func TestNewList(t *testing.T) {
	l := New()
	if l == nil {
		t.Fatal("New returned nil")
	}
	if l.LLen() != 0 {
		t.Fatalf("expected 0, got %d", l.LLen())
	}
}

func TestLPush_RPush(t *testing.T) {
	l := New()

	n := l.LPush("a")
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}
	n = l.LPush("b")
	if n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
	// List is now [b, a]

	n = l.RPush("c")
	if n != 3 {
		t.Fatalf("expected 3, got %d", n)
	}
	// List is now [b, a, c]

	if l.LLen() != 3 {
		t.Fatalf("expected 3, got %d", l.LLen())
	}
}

func TestLPop_RPop(t *testing.T) {
	l := New()
	l.RPush("a", "b", "c") // [a, b, c]

	v, ok := l.LPop()
	if !ok || v != "a" {
		t.Fatalf("expected a, got %s", v)
	}

	v, ok = l.RPop()
	if !ok || v != "c" {
		t.Fatalf("expected c, got %s", v)
	}

	v, ok = l.LPop()
	if !ok || v != "b" {
		t.Fatalf("expected b, got %s", v)
	}

	_, ok = l.LPop()
	if ok {
		t.Fatal("expected false on empty list")
	}
	_, ok = l.RPop()
	if ok {
		t.Fatal("expected false on empty list")
	}
}

func TestLRange(t *testing.T) {
	l := New()
	l.RPush("a", "b", "c", "d", "e") // [a, b, c, d, e]

	// Full range
	r := l.LRange(0, -1)
	if len(r) != 5 {
		t.Fatalf("expected 5, got %d", len(r))
	}

	// Sub range
	r = l.LRange(1, 3)
	if len(r) != 3 || r[0] != "b" || r[2] != "d" {
		t.Fatalf("unexpected: %v", r)
	}

	// Negative indices
	r = l.LRange(-2, -1)
	if len(r) != 2 || r[0] != "d" || r[1] != "e" {
		t.Fatalf("unexpected: %v", r)
	}

	// Empty range
	r = l.LRange(10, 20)
	if len(r) != 0 {
		t.Fatal("expected empty")
	}

	// Empty list
	l2 := New()
	if l2.LRange(0, -1) != nil {
		t.Fatal("expected nil")
	}
}

func TestLIndex(t *testing.T) {
	l := New()
	l.RPush("a", "b", "c")

	v, ok := l.LIndex(0)
	if !ok || v != "a" {
		t.Fatalf("expected a, got %s", v)
	}

	v, ok = l.LIndex(-1)
	if !ok || v != "c" {
		t.Fatalf("expected c, got %s", v)
	}

	_, ok = l.LIndex(10)
	if ok {
		t.Fatal("expected false")
	}
}

func TestLSet(t *testing.T) {
	l := New()
	l.RPush("a", "b", "c")

	if !l.LSet(1, "x") {
		t.Fatal("expected true")
	}
	v, _ := l.LIndex(1)
	if v != "x" {
		t.Fatalf("expected x, got %s", v)
	}

	if l.LSet(10, "y") {
		t.Fatal("expected false")
	}
}

func TestLRem(t *testing.T) {
	l := New()
	l.RPush("a", "b", "a", "c", "a") // [a, b, a, c, a]

	// count=0: remove all
	removed := l.LRem(0, "a")
	if removed != 3 {
		t.Fatalf("expected 3 removed, got %d", removed)
	}
	if l.LLen() != 2 {
		t.Fatalf("expected 2 remaining, got %d", l.LLen())
	}

	// count>0: remove from head
	l2 := New()
	l2.RPush("x", "y", "x", "z")
	removed = l2.LRem(1, "x")
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}
	v, _ := l2.LIndex(0)
	if v != "y" {
		t.Fatalf("expected y at head, got %s", v)
	}

	// count<0: remove from tail
	l3 := New()
	l3.RPush("x", "y", "x", "z")
	removed = l3.LRem(-1, "x")
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}
	v, _ = l3.LIndex(2)
	if v != "z" {
		t.Fatalf("expected z at tail, got %s", v)
	}
}

func TestLTrim(t *testing.T) {
	l := New()
	l.RPush("a", "b", "c", "d", "e") // [a, b, c, d, e]

	l.LTrim(1, 3) // keep [b, c, d]
	if l.LLen() != 3 {
		t.Fatalf("expected 3, got %d", l.LLen())
	}
	v, _ := l.LIndex(0)
	if v != "b" {
		t.Fatalf("expected b, got %s", v)
	}

	// Trim to empty
	l2 := New()
	l2.RPush("a", "b")
	l2.LTrim(10, 20)
	if l2.LLen() != 0 {
		t.Fatalf("expected 0, got %d", l2.LLen())
	}
}

func TestLInsert(t *testing.T) {
	l := New()
	l.RPush("a", "b", "c")

	n := l.LInsert(true, "b", "x") // before b: [a, x, b, c]
	if n != 4 {
		t.Fatalf("expected 4, got %d", n)
	}
	v, _ := l.LIndex(1)
	if v != "x" {
		t.Fatalf("expected x, got %s", v)
	}

	n = l.LInsert(false, "c", "y") // after c: [a, x, b, c, y]
	if n != 5 {
		t.Fatalf("expected 5, got %d", n)
	}

	n = l.LInsert(true, "nonexistent", "z")
	if n != -1 {
		t.Fatalf("expected -1, got %d", n)
	}
}

func TestLPos(t *testing.T) {
	l := New()
	l.RPush("a", "b", "a", "c", "a")

	pos := l.LPos("a", 1, 1, 0)
	if len(pos) != 1 || pos[0] != 0 {
		t.Fatalf("expected [0], got %v", pos)
	}

	pos = l.LPos("a", 2, 1, 0)
	if len(pos) != 1 || pos[0] != 2 {
		t.Fatalf("expected [2], got %v", pos)
	}

	pos = l.LPos("a", 1, 0, 0) // all matches, count=0 means find all
	if len(pos) != 3 {
		t.Fatalf("expected 3, got %d: %v", len(pos), pos)
	}

	pos = l.LPos("nonexistent", 1, 1, 0)
	if len(pos) != 0 {
		t.Fatalf("expected empty, got %v", pos)
	}

	pos = l.LPos("a", 1, 1, 3) // maxLen=3, only scan first 3
	if len(pos) == 0 || pos[0] != 0 {
		t.Fatalf("expected [0], got %v", pos)
	}
}

func TestForEach(t *testing.T) {
	l := New()
	l.RPush("a", "b", "c")

	var result []string
	l.ForEach(func(e string) bool {
		result = append(result, e)
		return true
	})
	if len(result) != 3 {
		t.Fatalf("expected 3, got %d", len(result))
	}

	// Early termination
	result = nil
	l.ForEach(func(e string) bool {
		result = append(result, e)
		return false // stop after first
	})
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
}

func TestListConcurrent(t *testing.T) {
	l := New()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			l.LPush("item")
			l.RPush("item")
			l.LPop()
			l.RPop()
			l.LLen()
			l.LRange(0, -1)
		}(i)
	}
	wg.Wait()
}
