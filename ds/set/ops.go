package set

// Union returns a new Set containing all elements that appear in any of the given sets.
// Caller must ensure at least one set is provided.
func Union(sets ...*Set) *Set {
	result := New()
	for _, s := range sets {
		s.mu.RLock()
		for e := range s.elements {
			result.elements[e] = struct{}{}
		}
		s.mu.RUnlock()
	}
	return result
}

// Inter returns a new Set containing elements that appear in ALL given sets.
// If no sets are provided, returns an empty set.
func Inter(sets ...*Set) *Set {
	if len(sets) == 0 {
		return New()
	}

	// Start with the smallest set for efficiency.
	smallest := sets[0]
	for _, s := range sets[1:] {
		if s.Card() < smallest.Card() {
			smallest = s
		}
	}

	result := New()
	smallest.ForEach(func(elem string) bool {
		ok := true
		for _, s := range sets {
			if s == smallest {
				continue
			}
			if !s.IsMember(elem) {
				ok = false
				break
			}
		}
		if ok {
			result.elements[elem] = struct{}{}
		}
		return true
	})
	return result
}

// Diff returns a new Set containing elements in the first set that are NOT
// in any of the subsequent sets.
func Diff(sets ...*Set) *Set {
	if len(sets) == 0 {
		return New()
	}
	if len(sets) == 1 {
		return Union(sets[0]) // clone
	}

	result := New()
	first := sets[0]
	others := sets[1:]

	first.ForEach(func(elem string) bool {
		ok := true
		for _, s := range others {
			if s.IsMember(elem) {
				ok = false
				break
			}
		}
		if ok {
			result.elements[elem] = struct{}{}
		}
		return true
	})
	return result
}
