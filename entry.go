package mcache

import "time"

// cacheEntry stores a single cached value with optional expiration.
type cacheEntry struct {
	value     []byte
	expiresAt int64 // Unix nano, 0 = no expiration
}

// newEntry creates an immutable cache entry.
func newEntry(value []byte, ttl time.Duration) cacheEntry {
	now := time.Now().UnixNano()
	var expires int64
	if ttl > 0 {
		expires = now + int64(ttl)
	}
	return cacheEntry{
		value:    append([]byte(nil), value...), // copy to enforce immutability
		expiresAt: expires,
	}
}

// isExpired reports whether the entry has expired.
func (e cacheEntry) isExpired() bool {
	if e.expiresAt == 0 {
		return false
	}
	return time.Now().UnixNano() >= e.expiresAt
}
