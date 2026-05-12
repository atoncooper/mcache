package mcache

import "time"

// --- Hash operations ---

// HSet sets field to value in the hash at key. Returns 1 if field is new, 0 if updated.
func (c *Cache) HSet(key, field, value string) (int, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).hSet(key, field, value), nil
}

// HSetNX sets field to value only if field does not exist.
func (c *Cache) HSetNX(key, field, value string) (bool, error) {
	if err := c.validateOpen(); err != nil {
		return false, err
	}
	if key == "" {
		return false, ErrKeyEmpty
	}
	return c.getShard(key).hSetNX(key, field, value), nil
}

// HGet returns the value of field in the hash at key.
func (c *Cache) HGet(key, field string) (string, error) {
	if err := c.validateOpen(); err != nil {
		return "", err
	}
	if key == "" {
		return "", ErrKeyEmpty
	}
	v, ok := c.getShard(key).hGet(key, field)
	if !ok {
		return "", ErrKeyNotFound
	}
	return v, nil
}

// HDel removes fields from the hash at key. Returns the number of fields removed.
func (c *Cache) HDel(key string, fields ...string) (int, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).hDel(key, fields...), nil
}

// HExists reports whether field exists in the hash at key.
func (c *Cache) HExists(key, field string) (bool, error) {
	if err := c.validateOpen(); err != nil {
		return false, err
	}
	if key == "" {
		return false, ErrKeyEmpty
	}
	return c.getShard(key).hExists(key, field), nil
}

// HGetAll returns all field-value pairs in the hash at key.
func (c *Cache) HGetAll(key string) (map[string]string, error) {
	if err := c.validateOpen(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrKeyEmpty
	}
	result := c.getShard(key).hGetAll(key)
	if result == nil {
		return nil, ErrKeyNotFound
	}
	return result, nil
}

// HKeys returns all field names in the hash at key.
func (c *Cache) HKeys(key string) ([]string, error) {
	if err := c.validateOpen(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrKeyEmpty
	}
	result := c.getShard(key).hKeys(key)
	if result == nil {
		return nil, ErrKeyNotFound
	}
	return result, nil
}

// HVals returns all values in the hash at key.
func (c *Cache) HVals(key string) ([]string, error) {
	if err := c.validateOpen(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrKeyEmpty
	}
	result := c.getShard(key).hVals(key)
	if result == nil {
		return nil, ErrKeyNotFound
	}
	return result, nil
}

// HLen returns the number of fields in the hash at key.
func (c *Cache) HLen(key string) (int, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).hLen(key), nil
}

// HStrLen returns the string length of the value at field in the hash at key.
func (c *Cache) HStrLen(key, field string) (int, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).hStrLen(key, field), nil
}

// HIncrBy increments the integer value of field by delta.
func (c *Cache) HIncrBy(key, field string, delta int64) (int64, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).hIncrBy(key, field, delta)
}

// HIncrByFloat increments the float value of field by delta.
func (c *Cache) HIncrByFloat(key, field string, delta float64) (float64, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).hIncrByFloat(key, field, delta)
}

// HMGet returns values for the given fields. Missing fields return nil.
func (c *Cache) HMGet(key string, fields ...string) ([]any, error) {
	if err := c.validateOpen(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrKeyEmpty
	}
	return c.getShard(key).hmGet(key, fields...), nil
}

// HMSet sets multiple field-value pairs in the hash at key.
func (c *Cache) HMSet(key string, fvPairs ...string) error {
	if err := c.validateOpen(); err != nil {
		return err
	}
	if key == "" {
		return ErrKeyEmpty
	}
	c.getShard(key).hmSet(key, fvPairs...)
	return nil
}

// --- List operations ---

// LPush inserts elements at the head of the list at key. Returns new length.
func (c *Cache) LPush(key string, elems ...string) (int, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).lPush(key, elems...), nil
}

// RPush inserts elements at the tail of the list at key. Returns new length.
func (c *Cache) RPush(key string, elems ...string) (int, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).rPush(key, elems...), nil
}

// LPop removes and returns the head element of the list at key.
func (c *Cache) LPop(key string) (string, error) {
	if err := c.validateOpen(); err != nil {
		return "", err
	}
	if key == "" {
		return "", ErrKeyEmpty
	}
	elem, ok := c.getShard(key).lPop(key)
	if !ok {
		return "", ErrKeyNotFound
	}
	return elem, nil
}

// RPop removes and returns the tail element of the list at key.
func (c *Cache) RPop(key string) (string, error) {
	if err := c.validateOpen(); err != nil {
		return "", err
	}
	if key == "" {
		return "", ErrKeyEmpty
	}
	elem, ok := c.getShard(key).rPop(key)
	if !ok {
		return "", ErrKeyNotFound
	}
	return elem, nil
}

// LLen returns the number of elements in the list at key.
func (c *Cache) LLen(key string) (int, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).lLen(key), nil
}

// LRange returns elements from start to stop in the list at key.
func (c *Cache) LRange(key string, start, stop int) ([]string, error) {
	if err := c.validateOpen(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrKeyEmpty
	}
	result := c.getShard(key).lRange(key, start, stop)
	if result == nil {
		return nil, ErrKeyNotFound
	}
	return result, nil
}

// LIndex returns the element at index in the list at key.
func (c *Cache) LIndex(key string, index int) (string, error) {
	if err := c.validateOpen(); err != nil {
		return "", err
	}
	if key == "" {
		return "", ErrKeyEmpty
	}
	elem, ok := c.getShard(key).lIndex(key, index)
	if !ok {
		return "", ErrKeyNotFound
	}
	return elem, nil
}

// LSet sets the element at index in the list at key. Returns false if out of range.
func (c *Cache) LSet(key string, index int, value string) error {
	if err := c.validateOpen(); err != nil {
		return err
	}
	if key == "" {
		return ErrKeyEmpty
	}
	if !c.getShard(key).lSet(key, index, value) {
		return ErrKeyNotFound
	}
	return nil
}

// LRem removes count occurrences of value from the list at key.
func (c *Cache) LRem(key string, count int, value string) (int, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).lRem(key, count, value), nil
}

// LTrim keeps only elements in [start, stop] range in the list at key.
func (c *Cache) LTrim(key string, start, stop int) error {
	if err := c.validateOpen(); err != nil {
		return err
	}
	if key == "" {
		return ErrKeyEmpty
	}
	c.getShard(key).lTrim(key, start, stop)
	return nil
}

// LInsert inserts value before or after pivot in the list at key.
func (c *Cache) LInsert(key string, before bool, pivot, value string) (int, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).lInsert(key, before, pivot, value), nil
}

// LPos returns the index of matching elements in the list at key.
func (c *Cache) LPos(key string, value string, rank, count, maxLen int) ([]int, error) {
	if err := c.validateOpen(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrKeyEmpty
	}
	return c.getShard(key).lPos(key, value, rank, count, maxLen), nil
}

// --- Key management ---

// Exists reports whether key exists in the cache (any type).
func (c *Cache) Exists(key string) (bool, error) {
	if err := c.validateOpen(); err != nil {
		return false, err
	}
	if key == "" {
		return false, ErrKeyEmpty
	}
	return c.getShard(key).keyExists(key), nil
}

// Type returns the type of key (string/set/hash/list/none).
func (c *Cache) Type(key string) (byte, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).keyType(key), nil
}

// Expire sets a TTL (in seconds) on key.
func (c *Cache) Expire(key string, seconds int64) (bool, error) {
	if err := c.validateOpen(); err != nil {
		return false, err
	}
	if key == "" {
		return false, ErrKeyEmpty
	}
	return c.getShard(key).expire(key, time.Duration(seconds)*time.Second), nil
}

// ExpireAt sets an absolute Unix timestamp expiration on key.
func (c *Cache) ExpireAt(key string, timestamp int64) (bool, error) {
	if err := c.validateOpen(); err != nil {
		return false, err
	}
	if key == "" {
		return false, ErrKeyEmpty
	}
	ttl := time.Until(time.Unix(timestamp, 0))
	if ttl <= 0 {
		return false, nil
	}
	return c.getShard(key).expire(key, ttl), nil
}

// PExpire sets a TTL (in milliseconds) on key.
func (c *Cache) PExpire(key string, ms int64) (bool, error) {
	if err := c.validateOpen(); err != nil {
		return false, err
	}
	if key == "" {
		return false, ErrKeyEmpty
	}
	return c.getShard(key).expire(key, time.Duration(ms)*time.Millisecond), nil
}

// PExpireAt sets an absolute Unix timestamp expiration (in ms) on key.
func (c *Cache) PExpireAt(key string, msTimestamp int64) (bool, error) {
	if err := c.validateOpen(); err != nil {
		return false, err
	}
	if key == "" {
		return false, ErrKeyEmpty
	}
	t := time.Unix(0, msTimestamp*int64(time.Millisecond))
	ttl := time.Until(t)
	if ttl <= 0 {
		return false, nil
	}
	return c.getShard(key).expire(key, ttl), nil
}

// TTL returns the remaining time to live in seconds.
// Returns -1 if key has no expiry, -2 if key doesn't exist.
func (c *Cache) TTL(key string) (int64, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).ttlSeconds(key), nil
}

// PTTL returns the remaining time to live in milliseconds.
func (c *Cache) PTTL(key string) (int64, error) {
	if err := c.validateOpen(); err != nil {
		return 0, err
	}
	if key == "" {
		return 0, ErrKeyEmpty
	}
	return c.getShard(key).ttlMillis(key), nil
}

// Persist removes the TTL from key.
func (c *Cache) Persist(key string) (bool, error) {
	if err := c.validateOpen(); err != nil {
		return false, err
	}
	if key == "" {
		return false, ErrKeyEmpty
	}
	return c.getShard(key).persist(key), nil
}

// Keys returns all keys matching the glob-style pattern.
func (c *Cache) Keys(pattern string) ([]string, error) {
	if err := c.validateOpen(); err != nil {
		return nil, err
	}
	var matched []string
	for _, s := range c.loadShardTable().shards {
		matched = append(matched, s.(*shard).matchKeys(pattern)...)
	}
	return matched, nil
}
