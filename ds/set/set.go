// Package set implements a thread-safe unordered unique string collection
// similar to Redis Sets. Supports Add, Rem, membership tests, random selection,
// and set operations (union, intersection, difference).
package set

import (
	"math/rand"
	"sync"
)

// Set is a thread-safe unordered collection of unique strings.
type Set struct {
	mu       sync.RWMutex
	elements map[string]struct{}
}

// New creates an empty Set.
func New() *Set {
	return &Set{elements: make(map[string]struct{})}
}

// NewFrom creates a Set from a slice of elements.
func NewFrom(elems []string) *Set {
	s := New()
	for _, e := range elems {
		s.elements[e] = struct{}{}
	}
	return s
}

// Add inserts element(s). Returns the number of elements actually added (excluding duplicates).
func (s *Set) Add(elems ...string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	added := 0
	for _, e := range elems {
		if _, ok := s.elements[e]; !ok {
			s.elements[e] = struct{}{}
			added++
		}
	}
	return added
}

// Rem removes element(s). Returns the number of elements actually removed.
func (s *Set) Rem(elems ...string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for _, e := range elems {
		if _, ok := s.elements[e]; ok {
			delete(s.elements, e)
			removed++
		}
	}
	return removed
}

// IsMember tests whether elem is in the set.
func (s *Set) IsMember(elem string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.elements[elem]
	return ok
}

// Members returns all elements in the set (order is non-deterministic).
func (s *Set) Members() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.elements))
	for e := range s.elements {
		out = append(out, e)
	}
	return out
}

// Card returns the number of elements (cardinality).
func (s *Set) Card() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.elements)
}

// Pop removes and returns a random element. Returns false if set is empty.
func (s *Set) Pop() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.elements) == 0 {
		return "", false
	}
	n := rand.Intn(len(s.elements))
	i := 0
	for e := range s.elements {
		if i == n {
			delete(s.elements, e)
			return e, true
		}
		i++
	}
	return "", false
}

// RandMember returns up to count random elements. If count is negative elements
// may repeat.
func (s *Set) RandMember(count int) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.elements) == 0 {
		return nil
	}

	out := make([]string, 0, abs(count))
	elems := make([]string, 0, len(s.elements))
	for e := range s.elements {
		elems = append(elems, e)
	}

	if count >= 0 {
		// Distinct random elements (no repeats)
		if count > len(elems) {
			count = len(elems)
		}
		perm := rand.Perm(len(elems))
		for i := 0; i < count; i++ {
			out = append(out, elems[perm[i]])
		}
	} else {
		// Allow repeats
		count = -count
		for i := 0; i < count; i++ {
			out = append(out, elems[rand.Intn(len(elems))])
		}
	}
	return out
}

// Clear removes all elements.
func (s *Set) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.elements = make(map[string]struct{})
}

// ForEach calls fn for each element. Iteration stops if fn returns false.
func (s *Set) ForEach(fn func(elem string) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for e := range s.elements {
		if !fn(e) {
			return
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
