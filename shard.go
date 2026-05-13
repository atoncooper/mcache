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

const subShardsPerShard = 8

// subShard is a lock-protected partition within a shard.
// Each subShard has its own RWMutex, entry map, and independent LRU/LFU policy.
type subShard struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	policy  EvictionPolicy
}

// shard is a thread-safe partition of the cache.
// String key entries are split across 8 sub-shards for lower lock contention.
// Set/Hash/List operations use shared maps with dedicated locks (low-frequency ops).
type shard struct {
	subs       [subShardsPerShard]*subShard
	numSubs    int // actual number of active sub-shards (≤8)
	maxSubSize int // per-subShard max entry count

	// Set/Hash/List maps use dedicated locks (not sub-sharded — low frequency).
	setMu  sync.RWMutex
	sets   map[string]*set.Set
	hashMu sync.RWMutex
	hashes map[string]*hash.Hash
	listMu sync.RWMutex
	lists  map[string]*list.List

	observer CacheObserver
}

// effectiveSubShards returns the number of sub-shards to use for the given maxSize.
// For small maxSize, fewer sub-shards preserve the exact capacity limit.
func effectiveSubShards(maxSize int) int {
	if maxSize <= 0 {
		return subShardsPerShard
	}
	n := maxSize / 2 // each sub-shard should hold at least 2 entries
	if n < 1 {
		n = 1
	}
	if n > subShardsPerShard {
		n = subShardsPerShard
	}
	return n
}

// newShard creates a shard with sub-shards, each having its own policy instance.
// When maxSize is small (<16), fewer sub-shards are used to preserve capacity semantics.
func newShard(policyFactory func() EvictionPolicy, maxSize int, observer CacheObserver) *shard {
	s := &shard{
		sets:     make(map[string]*set.Set),
		hashes:   make(map[string]*hash.Hash),
		lists:    make(map[string]*list.List),
		observer: observer,
	}
	nSub := effectiveSubShards(maxSize)
	subMax := maxSize / nSub
	if subMax < 1 && maxSize > 0 {
		subMax = 1
	}
	s.numSubs = nSub
	s.maxSubSize = subMax
	for i := 0; i < nSub; i++ {
		s.subs[i] = &subShard{
			entries: make(map[string]cacheEntry),
			policy:  policyFactory(),
		}
	}
	// Remaining sub-shard slots stay nil; subIndex maps to valid sub-shards only.
	return s
}

// subIndex maps a key to a sub-shard index using FNV-1a hash for uniform distribution.
func (s *shard) subIndex(key string) int {
	h := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return int(h % uint32(s.numSubs))
}

// --- KV operations (hot path — routed to sub-shard) ---

func (s *shard) get(key string) ([]byte, bool) {
	idx := s.subIndex(key)
	sub := s.subs[idx]
	sub.mu.RLock()
	entry, ok := sub.entries[key]
	sub.mu.RUnlock()

	if !ok {
		return nil, false
	}
	if entry.isExpired() {
		sub.mu.Lock()
		if e, ok2 := sub.entries[key]; ok2 && e.isExpired() {
			delete(sub.entries, key)
			sub.policy.OnRemove(key)
		}
		sub.mu.Unlock()
		return nil, false
	}
	return append([]byte(nil), entry.value...), true
}

func (s *shard) Set(key string, value []byte, ttl time.Duration) []string {
	idx := s.subIndex(key)
	sub := s.subs[idx]
	sub.mu.Lock()
	defer sub.mu.Unlock()
	_, existed := sub.entries[key]
	sub.entries[key] = newEntry(value, ttl)
	var evicted []string
	if existed {
		sub.policy.OnAccess(key)
	} else {
		sub.policy.OnAdd(key)
		for s.maxSubSize > 0 && len(sub.entries) > s.maxSubSize {
			evictKey, ok := sub.policy.Evict()
			if !ok {
				break
			}
			delete(sub.entries, evictKey)
			evicted = append(evicted, evictKey)
		}
	}
	return evicted
}

func (s *shard) Del(key string) {
	idx := s.subIndex(key)
	sub := s.subs[idx]
	sub.mu.Lock()
	if _, ok := sub.entries[key]; ok {
		delete(sub.entries, key)
		sub.policy.OnRemove(key)
	}
	sub.mu.Unlock()

	// Typed maps use dedicated locks; lock ordering: always acquire typed locks after sub lock.
	s.setMu.Lock()
	delete(s.sets, key)
	s.setMu.Unlock()
	s.hashMu.Lock()
	delete(s.hashes, key)
	s.hashMu.Unlock()
	s.listMu.Lock()
	delete(s.lists, key)
	s.listMu.Unlock()
}

// --- Iteration methods (acquire all sub-shard locks in ascending order) ---

func (s *shard) Len() int {
	count := 0
	for i := 0; i < s.numSubs; i++ {
		sub := s.subs[i]
		sub.mu.RLock()
		for _, e := range sub.entries {
			if !e.isExpired() {
				count++
			}
		}
		sub.mu.RUnlock()
	}
	return count
}

func (s *shard) cleanup() int {
	removed := 0
	for i := 0; i < s.numSubs; i++ {
		sub := s.subs[i]
		sub.mu.Lock()
		for k, e := range sub.entries {
			if e.isExpired() {
				delete(sub.entries, k)
				sub.policy.OnRemove(k)
				removed++
			}
		}
		sub.mu.Unlock()
	}
	return removed
}

// swapPolicy replaces all sub-shard policies. Locks sub-shards in ascending order
// to prevent deadlock with concurrent Set/Get/Del which lock a single sub-shard.
func (s *shard) swapPolicy(newPolicyFunc func() EvictionPolicy) {
	for i := 0; i < s.numSubs; i++ {
		sub := s.subs[i]
		sub.mu.Lock()
		newPolicy := newPolicyFunc()
		for k := range sub.entries {
			newPolicy.OnAdd(k)
		}
		sub.policy = newPolicy
		sub.mu.Unlock()
	}
}

func (s *shard) ExtractN(n int) []rehash.Item {
	items := make([]rehash.Item, 0, n)
	now := time.Now().UnixNano()
	for i := 0; i < subShardsPerShard && len(items) < n; i++ {
		sub := s.subs[i]
		sub.mu.Lock()
		for k, e := range sub.entries {
			if e.expiresAt > 0 && now >= e.expiresAt {
				delete(sub.entries, k)
				sub.policy.OnRemove(k)
				continue
			}
			var ttl time.Duration
			if e.expiresAt > 0 {
				ttl = time.Until(time.Unix(0, e.expiresAt))
				if ttl <= 0 {
					delete(sub.entries, k)
					sub.policy.OnRemove(k)
					continue
				}
			}
			items = append(items, rehash.Item{
				Key:   k,
				Value: append([]byte(nil), e.value...),
				TTL:   ttl,
			})
			delete(sub.entries, k)
			sub.policy.OnRemove(k)
			if len(items) >= n {
				sub.mu.Unlock()
				return items
			}
		}
		sub.mu.Unlock()
	}
	return items
}

// --- Key management (iterate all sub-shards) ---

func (s *shard) keyExists(key string) bool {
	idx := s.subIndex(key)
	sub := s.subs[idx]
	sub.mu.RLock()
	e, ok := sub.entries[key]
	sub.mu.RUnlock()
	if ok && !e.isExpired() {
		return true
	}
	// Check typed maps
	s.setMu.RLock()
	_, ok = s.sets[key]
	s.setMu.RUnlock()
	if ok {
		return true
	}
	s.hashMu.RLock()
	_, ok = s.hashes[key]
	s.hashMu.RUnlock()
	if ok {
		return true
	}
	s.listMu.RLock()
	_, ok = s.lists[key]
	s.listMu.RUnlock()
	return ok
}

func (s *shard) keyType(key string) byte {
	idx := s.subIndex(key)
	sub := s.subs[idx]
	sub.mu.RLock()
	e, ok := sub.entries[key]
	sub.mu.RUnlock()
	if ok && !e.isExpired() {
		return 1 // KeyTypeString
	}

	s.setMu.RLock()
	_, ok = s.sets[key]
	s.setMu.RUnlock()
	if ok {
		return 2
	}
	s.hashMu.RLock()
	_, ok = s.hashes[key]
	s.hashMu.RUnlock()
	if ok {
		return 3
	}
	s.listMu.RLock()
	_, ok = s.lists[key]
	s.listMu.RUnlock()
	if ok {
		return 4
	}
	return 0
}

func (s *shard) expire(key string, ttl time.Duration) bool {
	idx := s.subIndex(key)
	sub := s.subs[idx]
	sub.mu.Lock()
	defer sub.mu.Unlock()
	e, ok := sub.entries[key]
	if !ok || e.isExpired() {
		return false
	}
	e.expiresAt = time.Now().Add(ttl).UnixNano()
	sub.entries[key] = e
	return true
}

func (s *shard) persist(key string) bool {
	idx := s.subIndex(key)
	sub := s.subs[idx]
	sub.mu.Lock()
	defer sub.mu.Unlock()
	e, ok := sub.entries[key]
	if !ok || e.isExpired() {
		return false
	}
	e.expiresAt = 0
	sub.entries[key] = e
	return true
}

func (s *shard) ttlSeconds(key string) int64 {
	idx := s.subIndex(key)
	sub := s.subs[idx]
	sub.mu.RLock()
	defer sub.mu.RUnlock()
	e, ok := sub.entries[key]
	if !ok {
		return -2
	}
	if e.expiresAt <= 0 {
		return -1
	}
	remaining := time.Until(time.Unix(0, e.expiresAt))
	if remaining <= 0 {
		return -2
	}
	return int64(remaining / time.Second)
}

func (s *shard) ttlMillis(key string) int64 {
	idx := s.subIndex(key)
	sub := s.subs[idx]
	sub.mu.RLock()
	defer sub.mu.RUnlock()
	e, ok := sub.entries[key]
	if !ok {
		return -2
	}
	if e.expiresAt <= 0 {
		return -1
	}
	remaining := time.Until(time.Unix(0, e.expiresAt))
	if remaining <= 0 {
		return -2
	}
	return remaining.Milliseconds()
}

func (s *shard) matchKeys(pattern string) []string {
	var matched []string
	for i := 0; i < s.numSubs; i++ {
		sub := s.subs[i]
		sub.mu.RLock()
		for k, e := range sub.entries {
			if !e.isExpired() {
				if ok, _ := filepath.Match(pattern, k); ok {
					matched = append(matched, k)
				}
			}
		}
		sub.mu.RUnlock()
	}
	// Also check typed maps
	s.setMu.RLock()
	for k := range s.sets {
		if ok, _ := filepath.Match(pattern, k); ok {
			matched = append(matched, k)
		}
	}
	s.setMu.RUnlock()
	s.hashMu.RLock()
	for k := range s.hashes {
		if ok, _ := filepath.Match(pattern, k); ok {
			matched = append(matched, k)
		}
	}
	s.hashMu.RUnlock()
	s.listMu.RLock()
	for k := range s.lists {
		if ok, _ := filepath.Match(pattern, k); ok {
			matched = append(matched, k)
		}
	}
	s.listMu.RUnlock()
	return matched
}

// --- Set operations (shared locks, not sub-sharded — low frequency) ---

func (s *shard) sAdd(key string, elems ...string) int {
	s.setMu.Lock()
	defer s.setMu.Unlock()
	ss, ok := s.sets[key]
	if !ok {
		ss = set.New()
		s.sets[key] = ss
	}
	return ss.Add(elems...)
}

func (s *shard) sRem(key string, elems ...string) int {
	s.setMu.Lock()
	defer s.setMu.Unlock()
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
	s.setMu.RLock()
	defer s.setMu.RUnlock()
	ss, ok := s.sets[key]
	if !ok {
		return false
	}
	return ss.IsMember(elem)
}

func (s *shard) sMembers(key string) []string {
	s.setMu.RLock()
	defer s.setMu.RUnlock()
	ss, ok := s.sets[key]
	if !ok {
		return nil
	}
	return ss.Members()
}

func (s *shard) sCard(key string) int {
	s.setMu.RLock()
	defer s.setMu.RUnlock()
	ss, ok := s.sets[key]
	if !ok {
		return 0
	}
	return ss.Card()
}

func (s *shard) sPop(key string) (string, bool) {
	s.setMu.Lock()
	defer s.setMu.Unlock()
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
	s.setMu.RLock()
	defer s.setMu.RUnlock()
	ss, ok := s.sets[key]
	if !ok {
		return nil
	}
	return ss.RandMember(count)
}

func (s *shard) getSet(key string) *set.Set {
	s.setMu.RLock()
	defer s.setMu.RUnlock()
	return s.sets[key]
}

// --- Hash operations (shared locks, not sub-sharded) ---

func (s *shard) hSet(key, field, value string) int {
	s.hashMu.Lock()
	defer s.hashMu.Unlock()
	h, ok := s.hashes[key]
	if !ok {
		h = hash.New()
		s.hashes[key] = h
	}
	return h.HSet(field, value)
}

func (s *shard) hSetNX(key, field, value string) bool {
	s.hashMu.Lock()
	defer s.hashMu.Unlock()
	h, ok := s.hashes[key]
	if !ok {
		h = hash.New()
		s.hashes[key] = h
	}
	return h.HSetNX(field, value)
}

func (s *shard) hGet(key, field string) (string, bool) {
	s.hashMu.RLock()
	defer s.hashMu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		return "", false
	}
	return h.HGet(field)
}

func (s *shard) hDel(key string, fields ...string) int {
	s.hashMu.Lock()
	defer s.hashMu.Unlock()
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
	s.hashMu.RLock()
	defer s.hashMu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		return false
	}
	return h.HExists(field)
}

func (s *shard) hGetAll(key string) map[string]string {
	s.hashMu.RLock()
	defer s.hashMu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		return nil
	}
	return h.HGetAll()
}

func (s *shard) hKeys(key string) []string {
	s.hashMu.RLock()
	defer s.hashMu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		return nil
	}
	return h.HKeys()
}

func (s *shard) hVals(key string) []string {
	s.hashMu.RLock()
	defer s.hashMu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		return nil
	}
	return h.HVals()
}

func (s *shard) hLen(key string) int {
	s.hashMu.RLock()
	defer s.hashMu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		return 0
	}
	return h.HLen()
}

func (s *shard) hStrLen(key, field string) int {
	s.hashMu.RLock()
	defer s.hashMu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		return 0
	}
	return h.HStrLen(field)
}

func (s *shard) hIncrBy(key, field string, delta int64) (int64, error) {
	s.hashMu.Lock()
	defer s.hashMu.Unlock()
	h, ok := s.hashes[key]
	if !ok {
		h = hash.New()
		s.hashes[key] = h
	}
	return h.HIncrBy(field, delta)
}

func (s *shard) hIncrByFloat(key, field string, delta float64) (float64, error) {
	s.hashMu.Lock()
	defer s.hashMu.Unlock()
	h, ok := s.hashes[key]
	if !ok {
		h = hash.New()
		s.hashes[key] = h
	}
	return h.HIncrByFloat(field, delta)
}

func (s *shard) hmGet(key string, fields ...string) []any {
	s.hashMu.RLock()
	defer s.hashMu.RUnlock()
	h, ok := s.hashes[key]
	if !ok {
		out := make([]any, len(fields))
		return out
	}
	return h.HMGet(fields...)
}

func (s *shard) hmSet(key string, fvPairs ...string) {
	s.hashMu.Lock()
	defer s.hashMu.Unlock()
	h, ok := s.hashes[key]
	if !ok {
		h = hash.New()
		s.hashes[key] = h
	}
	h.HMSet(fvPairs...)
}

// --- List operations (shared locks, not sub-sharded) ---

func (s *shard) lPush(key string, elems ...string) int {
	s.listMu.Lock()
	defer s.listMu.Unlock()
	l, ok := s.lists[key]
	if !ok {
		l = list.New()
		s.lists[key] = l
	}
	return l.LPush(elems...)
}

func (s *shard) rPush(key string, elems ...string) int {
	s.listMu.Lock()
	defer s.listMu.Unlock()
	l, ok := s.lists[key]
	if !ok {
		l = list.New()
		s.lists[key] = l
	}
	return l.RPush(elems...)
}

func (s *shard) lPop(key string) (string, bool) {
	s.listMu.Lock()
	defer s.listMu.Unlock()
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
	s.listMu.Lock()
	defer s.listMu.Unlock()
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
	s.listMu.RLock()
	defer s.listMu.RUnlock()
	l, ok := s.lists[key]
	if !ok {
		return 0
	}
	return l.LLen()
}

func (s *shard) lRange(key string, start, stop int) []string {
	s.listMu.RLock()
	defer s.listMu.RUnlock()
	l, ok := s.lists[key]
	if !ok {
		return nil
	}
	return l.LRange(start, stop)
}

func (s *shard) lIndex(key string, index int) (string, bool) {
	s.listMu.RLock()
	defer s.listMu.RUnlock()
	l, ok := s.lists[key]
	if !ok {
		return "", false
	}
	return l.LIndex(index)
}

func (s *shard) lSet(key string, index int, value string) bool {
	s.listMu.Lock()
	defer s.listMu.Unlock()
	l, ok := s.lists[key]
	if !ok {
		return false
	}
	return l.LSet(index, value)
}

func (s *shard) lRem(key string, count int, value string) int {
	s.listMu.Lock()
	defer s.listMu.Unlock()
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
	s.listMu.Lock()
	defer s.listMu.Unlock()
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
	s.listMu.Lock()
	defer s.listMu.Unlock()
	l, ok := s.lists[key]
	if !ok {
		return 0
	}
	return l.LInsert(before, pivot, value)
}

func (s *shard) lPos(key string, value string, rank, count, maxLen int) []int {
	s.listMu.RLock()
	defer s.listMu.RUnlock()
	l, ok := s.lists[key]
	if !ok {
		return nil
	}
	return l.LPos(value, rank, count, maxLen)
}
