package set

import (
	"testing"
)

func TestSet_Add(t *testing.T) {
	s := New()
	n := s.Add("a", "b", "c")
	if n != 3 {
		t.Fatalf("expected 3 added, got %d", n)
	}
	if s.Card() != 3 {
		t.Errorf("expected 3 cards, got %d", s.Card())
	}

	// Duplicate
	n = s.Add("a", "d")
	if n != 1 {
		t.Errorf("expected 1 new add, got %d", n)
	}
	if s.Card() != 4 {
		t.Errorf("expected 4 cards, got %d", s.Card())
	}
}

func TestSet_Rem(t *testing.T) {
	s := NewFrom([]string{"a", "b", "c"})
	n := s.Rem("b", "d")
	if n != 1 {
		t.Errorf("expected 1 removed, got %d", n)
	}
	if s.Card() != 2 {
		t.Errorf("expected 2 cards, got %d", s.Card())
	}
	if s.IsMember("b") {
		t.Error("b should have been removed")
	}
}

func TestSet_IsMember(t *testing.T) {
	s := NewFrom([]string{"x", "y"})
	if !s.IsMember("x") {
		t.Error("x should be member")
	}
	if s.IsMember("z") {
		t.Error("z should not be member")
	}
}

func TestSet_Members(t *testing.T) {
	s := NewFrom([]string{"a", "b", "c"})
	m := s.Members()
	if len(m) != 3 {
		t.Errorf("expected 3 members, got %d", len(m))
	}
	// Verify all expected elements present
	seen := make(map[string]bool)
	for _, e := range m {
		seen[e] = true
	}
	if !seen["a"] || !seen["b"] || !seen["c"] {
		t.Error("missing expected elements")
	}
}

func TestSet_Card(t *testing.T) {
	s := New()
	if s.Card() != 0 {
		t.Error("empty set should have card 0")
	}
	s.Add("a", "b")
	if s.Card() != 2 {
		t.Error("wrong card")
	}
}

func TestSet_Pop(t *testing.T) {
	s := New()
	if _, ok := s.Pop(); ok {
		t.Error("pop on empty should return false")
	}

	s.Add("a", "b")
	elem, ok := s.Pop()
	if !ok {
		t.Fatal("pop should return true")
	}
	if elem != "a" && elem != "b" {
		t.Errorf("unexpected elem: %s", elem)
	}
	if s.Card() != 1 {
		t.Errorf("expected 1 remaining, got %d", s.Card())
	}
}

func TestSet_RandMember(t *testing.T) {
	s := NewFrom([]string{"a", "b", "c", "d", "e"})

	// Positive count: distinct
	r := s.RandMember(3)
	if len(r) != 3 {
		t.Errorf("expected 3, got %d", len(r))
	}

	// Negative count: allow repeats
	r = s.RandMember(-5)
	if len(r) != 5 {
		t.Errorf("expected 5, got %d", len(r))
	}
}

func TestSet_Clear(t *testing.T) {
	s := NewFrom([]string{"a", "b", "c"})
	s.Clear()
	if s.Card() != 0 {
		t.Error("clear failed")
	}
}

func TestSet_ForEach(t *testing.T) {
	s := NewFrom([]string{"a", "b", "c"})
	count := 0
	s.ForEach(func(elem string) bool {
		count++
		return true
	})
	if count != 3 {
		t.Errorf("expected 3 iterations, got %d", count)
	}

	// Early stop
	count = 0
	s.ForEach(func(elem string) bool {
		count++
		return false // stop after first
	})
	if count != 1 {
		t.Errorf("expected 1 iteration, got %d", count)
	}
}

func TestNewFrom(t *testing.T) {
	s := NewFrom([]string{"x", "y", "z"})
	if s.Card() != 3 {
		t.Errorf("expected 3, got %d", s.Card())
	}
}

// --- Set operations ---

func TestUnion(t *testing.T) {
	a := NewFrom([]string{"1", "2", "3"})
	b := NewFrom([]string{"3", "4", "5"})

	u := Union(a, b)
	if u.Card() != 5 {
		t.Errorf("union expected 5, got %d", u.Card())
	}
	for _, e := range []string{"1", "2", "3", "4", "5"} {
		if !u.IsMember(e) {
			t.Errorf("union missing %s", e)
		}
	}
}

func TestInter(t *testing.T) {
	a := NewFrom([]string{"1", "2", "3"})
	b := NewFrom([]string{"2", "3", "4"})
	c := NewFrom([]string{"3", "5"})

	i := Inter(a, b, c)
	if i.Card() != 1 {
		t.Errorf("intersection expected 1, got %d", i.Card())
	}
	if !i.IsMember("3") {
		t.Error("intersection missing common 3")
	}
}

func TestInter_Empty(t *testing.T) {
	i := Inter()
	if i.Card() != 0 {
		t.Error("empty intersection should be empty")
	}
}

func TestDiff(t *testing.T) {
	a := NewFrom([]string{"1", "2", "3", "4"})
	b := NewFrom([]string{"2", "4"})

	d := Diff(a, b)
	if d.Card() != 2 {
		t.Errorf("diff expected 2, got %d", d.Card())
	}
	if !d.IsMember("1") || !d.IsMember("3") {
		t.Error("diff wrong elements")
	}
}

func TestDiff_Empty(t *testing.T) {
	d := Diff()
	if d.Card() != 0 {
		t.Error("empty diff should be empty")
	}
}

func TestDiff_Single(t *testing.T) {
	a := NewFrom([]string{"a", "b"})
	d := Diff(a) // single set = clone
	if d.Card() != 2 {
		t.Errorf("expected clone, got %d", d.Card())
	}
}
