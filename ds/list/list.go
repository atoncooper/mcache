// Package list implements a thread-safe doubly-linked string list
// similar to Redis Lists. Supports O(1) push/pop at both ends.
package list

import (
	"container/list"
	"sync"
)

// List is a thread-safe doubly-linked list of strings.
type List struct {
	mu sync.Mutex
	l  *list.List
}

// New creates an empty List.
func New() *List {
	return &List{l: list.New()}
}

// LPush inserts elements at the head. Returns new length.
func (l *List) LPush(elems ...string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range elems {
		l.l.PushFront(e)
	}
	return l.l.Len()
}

// RPush inserts elements at the tail. Returns new length.
func (l *List) RPush(elems ...string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range elems {
		l.l.PushBack(e)
	}
	return l.l.Len()
}

// LPop removes and returns the head element. ok=false if empty.
func (l *List) LPop() (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.l.Len() == 0 {
		return "", false
	}
	return l.l.Remove(l.l.Front()).(string), true
}

// RPop removes and returns the tail element. ok=false if empty.
func (l *List) RPop() (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.l.Len() == 0 {
		return "", false
	}
	return l.l.Remove(l.l.Back()).(string), true
}

// LLen returns the number of elements.
func (l *List) LLen() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.l.Len()
}

// LRange returns elements from start to stop (inclusive).
// Negative indices count from end.
func (l *List) LRange(start, stop int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := l.l.Len()
	if n == 0 {
		return nil
	}
	start, stop = normalizeRange(start, stop, n)
	if start > stop {
		return nil
	}
	out := make([]string, 0, stop-start+1)
	e := l.l.Front()
	for i := 0; i < start; i++ {
		e = e.Next()
	}
	for i := start; i <= stop && e != nil; i++ {
		out = append(out, e.Value.(string))
		e = e.Next()
	}
	return out
}

// LIndex returns the element at index. Negative indices count from end.
func (l *List) LIndex(index int) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := l.l.Len()
	if n == 0 {
		return "", false
	}
	if index < 0 {
		index = n + index
	}
	if index < 0 || index >= n {
		return "", false
	}
	e := l.l.Front()
	for i := 0; i < index; i++ {
		e = e.Next()
	}
	return e.Value.(string), true
}

// LSet sets the element at index. Returns false if out of range.
func (l *List) LSet(index int, value string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := l.l.Len()
	if index < 0 {
		index = n + index
	}
	if index < 0 || index >= n {
		return false
	}
	e := l.l.Front()
	for i := 0; i < index; i++ {
		e = e.Next()
	}
	e.Value = value
	return true
}

// LRem removes up to count occurrences of value.
// count > 0: remove from head; count < 0: remove from tail; count = 0: remove all.
func (l *List) LRem(count int, value string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.l.Len() == 0 {
		return 0
	}
	removed := 0
	if count >= 0 {
		maxRemove := count
		if maxRemove == 0 {
			maxRemove = l.l.Len()
		}
		e := l.l.Front()
		for e != nil && removed < maxRemove {
			next := e.Next()
			if e.Value.(string) == value {
				l.l.Remove(e)
				removed++
			}
			e = next
		}
	} else {
		maxRemove := -count
		e := l.l.Back()
		for e != nil && removed < maxRemove {
			prev := e.Prev()
			if e.Value.(string) == value {
				l.l.Remove(e)
				removed++
			}
			e = prev
		}
	}
	return removed
}

// LTrim keeps only elements in [start, stop] range.
func (l *List) LTrim(start, stop int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := l.l.Len()
	if n == 0 {
		return
	}
	start, stop = normalizeRange(start, stop, n)
	if start > stop {
		l.l.Init()
		return
	}
	for i := 0; i < start; i++ {
		l.l.Remove(l.l.Front())
	}
	for i := stop + 1; i < n; i++ {
		l.l.Remove(l.l.Back())
	}
}

// LInsert inserts value before or after pivot. Returns new length or -1 if pivot not found.
func (l *List) LInsert(before bool, pivot, value string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	for e := l.l.Front(); e != nil; e = e.Next() {
		if e.Value.(string) == pivot {
			if before {
				l.l.InsertBefore(value, e)
			} else {
				l.l.InsertAfter(value, e)
			}
			return l.l.Len()
		}
	}
	return -1
}

// LPos returns the first index of value, or -1 if not found.
// rank: 1-based Nth match. count: max positions to return. maxLen: max scan distance.
func (l *List) LPos(value string, rank, count, maxLen int) []int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := l.l.Len()
	if maxLen == 0 || maxLen > n {
		maxLen = n
	}
	if count < 0 {
		count = 1
	}
	allMatches := count == 0
	var positions []int
	found := 0
	i := 0
	for e := l.l.Front(); e != nil && i < maxLen; e, i = e.Next(), i+1 {
		if !allMatches && len(positions) >= count {
			break
		}
		if e.Value.(string) == value {
			found++
			if found >= rank {
				positions = append(positions, i)
			}
		}
	}
	return positions
}

// ForEach calls fn for each element. Stops if fn returns false.
func (l *List) ForEach(fn func(elem string) bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for e := l.l.Front(); e != nil; e = e.Next() {
		if !fn(e.Value.(string)) {
			return
		}
	}
}

func normalizeRange(start, stop, n int) (int, int) {
	if start < 0 {
		start = n + start
	}
	if stop < 0 {
		stop = n + stop
	}
	if start < 0 {
		start = 0
	}
	if stop >= n {
		stop = n - 1
	}
	return start, stop
}
