package mcache

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/atoncooper/mcache/ds/hash"
	"github.com/atoncooper/mcache/ds/list"
	"github.com/atoncooper/mcache/rehash"
	"github.com/atoncooper/mcache/ds/set"
)

// shard is a thread-safe partition of the cache.
type shard struct {
	mu       sync.RWMutex
	entries  map[string]cacheEntry
	sets     map[string]*set.Set
	hashes   map[string]*hash.Hash
	lists    map[string]*list.List
	policy   EvictionPolicy
	maxSize  int
	observer CacheObserver
}

func newShard(policy EvictionPolicy, maxSize int, observer CacheObserver) *shard {
	return &shard{
		entries:  make(map[string]cacheEntry),
		sets:     make(map[string]*set.Set),
		hashes:   make(map[string]*hash.Hash),
		lists:    make(map[string]*list.List),
		policy:   policy,
		maxSize:  maxSize,
		observer: observer,
	}
}

// get returns a value if present and not expired.
func (s *shard) get(key string) ([]byte, bool) {
	s.mu.RLock()
	entry, ok := s.entries[key]
	policy := s.policy
	s.mu.RUnlock()

	if !ok {
		return nil, false
	}
	if entry.isExpired() {
		s.mu.Lock()
		if e, ok2 := s.entries[key]; ok2 && e.isExpired() {
			delete(s.entries, key)
			s.policy.OnRemove(key)
		}
		s.mu.Unlock()
		return nil, false
	}
	policy.OnAccess(key)
	return append([]byte(nil), entry.value...), true
}

// Set stores a value with optional TTL and returns any evicted keys.
func (s *shard) Set(key string, value []byte, ttl time.Duration) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, existed := s.entries[key]
	s.entries[key] = newEntry(value, ttl)
	var evicted []string
	if existed {
		s.policy.OnAccess(key)
	} else {
		s.policy.OnAdd(key)
		for s.maxSize > 0 && len(s.entries) > s.maxSize {
			evictKey, ok := s.policy.Evict()
			if !ok {
				break
			}
			delete(s.entries, evictKey)
			evicted = append(evicted, evictKey)
		}
	}
	return evicted
}

// Del removes a key from all typed maps.
func (s *shard) Del(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[key]; ok {
		delete(s.entries, key)
		s.policy.OnRemove(key)
	}
	delete(s.sets, key)
	delete(s.hashes, key)
	delete(s.lists, key)
}

// Len returns the number of active entries (string keys only).
func (s *shard) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, e := range s.entries {
		if !e.isExpired() {
			count++
		}
	}
	return count
}

// cleanup removes expired entries and returns count removed.
func (s *shard) cleanup() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for k, e := range s.entries {
		if e.isExpired() {
			delete(s.entries, k)
			s.policy.OnRemove(k)
			removed++
		}
	}
	return removed
}

// swapPolicy replaces the policy and seeds it with existing keys.
func (s *shard) swapPolicy(newPolicy EvictionPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.entries {
		newPolicy.OnAdd(k)
	}
	s.policy = newPolicy
}

// ExtractN removes and returns up to n active entries.
func (s *shard) ExtractN(n int) []rehash.Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]rehash.Item, 0, n)
	now := time.Now().UnixNano()
	for k, e := range s.entries {
		if e.expiresAt > 0 && now >= e.expiresAt {
			delete(s.entries, k)
			s.policy.OnRemove(k)
			continue
		}
		var ttl time.Duration
		if e.expiresAt > 0 {
			ttl = time.Until(time.Unix(0, e.expiresAt))
			if ttl <= 0 {
				delete(s.entries, k)
				s.policy.OnRemove(k)
				continue
			}
		}
		items = append(items, rehash.Item{
			Key:   k,
			Value: append([]byte(nil), e.value...),
			TTL:   ttl,
		})
		delete(s.entries, k)
		s.policy.OnRemove(k)
		if len(items) >= n {
			break
		}
	}
	return items
}

// --- Set operations ---

func (s *shard) sAdd(key string, elems ...string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.sets[key]
	if !ok {
		ss = set.New()
		s.sets[key] = ss
	}
	return ss.Add(elems...)
}

func (s *shard) sRem(key string, elems ...string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.sets[key]
	if !ok {
		return 0
	}
	n := ss.Rem(elems...)
	if ss.Card() == 0 {
		delete(s.sets, key)
	}
	return n
}

func (s *shard) sIsMember(key, elem string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ss, ok := s.sets[key]
	if !ok {
		return false
	}
	return ss.IsMember(elem)
}

func (s *shard) sMembers(key string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ss, ok := s.sets[key]
	if !ok {
		return nil
	}
	return ss.Members()
}

func (s *shard) sCard(key string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ss, ok := s.sets[key]
	if !ok {
		return 0
	}
	return ss.Card()
}

func (s *shard) sPop(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.sets[key]
	if !ok {
		return "", false
	}
	elem, ok := ss.Pop()
	if !ok {
		return "", false
	}
	if ss.Card() == 0 {
		delete(s.sets, key)
	}
	return elem, true
}

func (s *shard) sRandMember(key string, count int) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ss, ok := s.sets[key]
	if !ok {
		return nil
	}
	return ss.RandMember(count)
}

func (s *shard) getSet(key string) *set.Set {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sets[key]
}

// --- Hash operations ---

func (s *shard) hSet(key, field, value string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.hashes[key]
	if !ok {
		h = hash.New()
		s.hashes[key] = h
	}
	return h.HSet(field, value)
}

func (s *shard) hSetNX(key, field, value string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.hashes[key]
	if !ok {
		h = hash.New()
		s.hashes[key] = h
	}
	return h.HSetNX(field, value)
}

func (s *shard) hGet(key, field string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		return "", false
	}
	return h.HGet(field)
}

func (s *shard) hDel(key string, fields ...string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.hashes[key]
	if !ok {
		return 0
	}
	n := h.HDel(fields...)
	if h.HLen() == 0 {
		delete(s.hashes, key)
	}
	return n
}

func (s *shard) hExists(key, field string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		return false
	}
	return h.HExists(field)
}

func (s *shard) hGetAll(key string) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		return nil
	}
	return h.HGetAll()
}

func (s *shard) hKeys(key string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		return nil
	}
	return h.HKeys()
}

func (s *shard) hVals(key string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		return nil
	}
	return h.HVals()
}

func (s *shard) hLen(key string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		return 0
	}
	return h.HLen()
}

func (s *shard) hStrLen(key, field string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		return 0
	}
	return h.HStrLen(field)
}

func (s *shard) hIncrBy(key, field string, delta int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.hashes[key]
	if !ok {
		h = hash.New()
		s.hashes[key] = h
	}
	return h.HIncrBy(field, delta)
}

func (s *shard) hIncrByFloat(key, field string, delta float64) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.hashes[key]
	if !ok {
		h = hash.New()
		s.hashes[key] = h
	}
	return h.HIncrByFloat(field, delta)
}

func (s *shard) hmGet(key string, fields ...string) []any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		out := make([]any, len(fields))
		return out
	}
	return h.HMGet(fields...)
}

func (s *shard) hmSet(key string, fvPairs ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.hashes[key]
	if !ok {
		h = hash.New()
		s.hashes[key] = h
	}
	h.HMSet(fvPairs...)
}

// --- List operations ---

func (s *shard) lPush(key string, elems ...string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.lists[key]
	if !ok {
		l = list.New()
		s.lists[key] = l
	}
	return l.LPush(elems...)
}

func (s *shard) rPush(key string, elems ...string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.lists[key]
	if !ok {
		l = list.New()
		s.lists[key] = l
	}
	return l.RPush(elems...)
}

func (s *shard) lPop(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.lists[key]
	if !ok {
		return "", false
	}
	elem, ok := l.LPop()
	if !ok {
		return "", false
	}
	if l.LLen() == 0 {
		delete(s.lists, key)
	}
	return elem, true
}

func (s *shard) rPop(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.lists[key]
	if !ok {
		return "", false
	}
	elem, ok := l.RPop()
	if !ok {
		return "", false
	}
	if l.LLen() == 0 {
		delete(s.lists, key)
	}
	return elem, true
}

func (s *shard) lLen(key string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.lists[key]
	if !ok {
		return 0
	}
	return l.LLen()
}

func (s *shard) lRange(key string, start, stop int) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.lists[key]
	if !ok {
		return nil
	}
	return l.LRange(start, stop)
}

func (s *shard) lIndex(key string, index int) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.lists[key]
	if !ok {
		return "", false
	}
	return l.LIndex(index)
}

func (s *shard) lSet(key string, index int, value string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.lists[key]
	if !ok {
		return false
	}
	return l.LSet(index, value)
}

func (s *shard) lRem(key string, count int, value string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.lists[key]
	if !ok {
		return 0
	}
	n := l.LRem(count, value)
	if l.LLen() == 0 {
		delete(s.lists, key)
	}
	return n
}

func (s *shard) lTrim(key string, start, stop int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.lists[key]
	if !ok {
		return
	}
	l.LTrim(start, stop)
	if l.LLen() == 0 {
		delete(s.lists, key)
	}
}

func (s *shard) lInsert(key string, before bool, pivot, value string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.lists[key]
	if !ok {
		return 0
	}
	return l.LInsert(before, pivot, value)
}

func (s *shard) lPos(key string, value string, rank, count, maxLen int) []int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.lists[key]
	if !ok {
		return nil
	}
	return l.LPos(value, rank, count, maxLen)
}

// --- Key management ---

func (s *shard) keyExists(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.entries[key]; ok {
		if e.isExpired() {
			return false
		}
		return true
	}
	if _, ok := s.sets[key]; ok {
		return true
	}
	if _, ok := s.hashes[key]; ok {
		return true
	}
	if _, ok := s.lists[key]; ok {
		return true
	}
	return false
}

func (s *shard) keyType(key string) byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.entries[key]; ok {
		if !e.isExpired() {
			return 1 // KeyTypeString
		}
		return 0 // expired -> KeyTypeNone
	}
	if _, ok := s.sets[key]; ok {
		return 2 // KeyTypeSet
	}
	if _, ok := s.hashes[key]; ok {
		return 3 // KeyTypeHash
	}
	if _, ok := s.lists[key]; ok {
		return 4 // KeyTypeList
	}
	return 0 // KeyTypeNone
}

func (s *shard) expire(key string, ttl time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[key]; ok {
		if e.isExpired() {
			return false
		}
		e.expiresAt = time.Now().Add(ttl).UnixNano()
		s.entries[key] = e
		return true
	}
	// For typed keys (set/hash/list), we don't yet track TTL on them.
	// Return false to indicate key doesn't exist or TTL not supported for typed keys.
	return false
}

func (s *shard) persist(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[key]; ok {
		if e.isExpired() {
			return false
		}
		e.expiresAt = 0
		s.entries[key] = e
		return true
	}
	return false
}

func (s *shard) ttlSeconds(key string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.entries[key]; ok {
		if e.expiresAt <= 0 {
			return -1 // no expiry
		}
		remaining := time.Until(time.Unix(0, e.expiresAt))
		if remaining <= 0 {
			return -2 // expired
		}
		return int64(remaining / time.Second)
	}
	return -2 // key doesn't exist
}

func (s *shard) ttlMillis(key string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.entries[key]; ok {
		if e.expiresAt <= 0 {
			return -1
		}
		remaining := time.Until(time.Unix(0, e.expiresAt))
		if remaining <= 0 {
			return -2
		}
		return remaining.Milliseconds()
	}
	return -2
}

func (s *shard) matchKeys(pattern string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var matched []string
	for k := range s.entries {
		if e := s.entries[k]; !e.isExpired() {
			if ok, _ := filepath.Match(pattern, k); ok {
				matched = append(matched, k)
			}
		}
	}
	for k := range s.sets {
		if ok, _ := filepath.Match(pattern, k); ok {
			matched = append(matched, k)
		}
	}
	for k := range s.hashes {
		if ok, _ := filepath.Match(pattern, k); ok {
			matched = append(matched, k)
		}
	}
	for k := range s.lists {
		if ok, _ := filepath.Match(pattern, k); ok {
			matched = append(matched, k)
		}
	}
	return matched
}
