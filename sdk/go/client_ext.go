package mcache

import (
	"time"
)

// ---------------------------------------------------------------------------
// Hash operations
// ---------------------------------------------------------------------------

// HSet sets a field in a hash. Returns 1 if the field was added, 0 if updated.
func (c *Client) HSet(key, field, value string) (int, error) {
	return c.transport.HSet(key, field, value)
}

// HSetNX sets a field only if it does not already exist. Returns true if set.
func (c *Client) HSetNX(key, field, value string) (bool, error) {
	return c.transport.HSetNX(key, field, value)
}

// HGet returns the value of a hash field.
func (c *Client) HGet(key, field string) (string, error) {
	return c.transport.HGet(key, field)
}

// HDel deletes one or more fields from a hash. Returns the number of fields removed.
func (c *Client) HDel(key string, fields ...string) (int, error) {
	return c.transport.HDel(key, fields...)
}

// HExists reports whether a field exists in the hash.
func (c *Client) HExists(key, field string) (bool, error) {
	return c.transport.HExists(key, field)
}

// HGetAll returns all field-value pairs in a hash.
func (c *Client) HGetAll(key string) (map[string]string, error) {
	return c.transport.HGetAll(key)
}

// HKeys returns all field names in a hash.
func (c *Client) HKeys(key string) ([]string, error) {
	return c.transport.HKeys(key)
}

// HVals returns all values in a hash.
func (c *Client) HVals(key string) ([]string, error) {
	return c.transport.HVals(key)
}

// HLen returns the number of fields in a hash.
func (c *Client) HLen(key string) (int, error) {
	return c.transport.HLen(key)
}

// HStrLen returns the string length of a field's value.
func (c *Client) HStrLen(key, field string) (int, error) {
	return c.transport.HStrLen(key, field)
}

// HIncrBy increments the integer value of a hash field by delta.
func (c *Client) HIncrBy(key, field string, delta int64) (int64, error) {
	return c.transport.HIncrBy(key, field, delta)
}

// HIncrByFloat increments the float value of a hash field by delta.
func (c *Client) HIncrByFloat(key, field string, delta float64) (float64, error) {
	return c.transport.HIncrByFloat(key, field, delta)
}

// HMGet returns the values of multiple hash fields. Missing fields are nil.
func (c *Client) HMGet(key string, fields ...string) ([]any, error) {
	return c.transport.HMGet(key, fields...)
}

// HMSet sets multiple field-value pairs in a hash.
func (c *Client) HMSet(key string, fvPairs ...string) error {
	return c.transport.HMSet(key, fvPairs...)
}

// ---------------------------------------------------------------------------
// List operations
// ---------------------------------------------------------------------------

// LPush inserts elements at the head of a list. Returns the new length.
func (c *Client) LPush(key string, elems ...string) (int, error) {
	return c.transport.LPush(key, elems...)
}

// RPush appends elements to the tail of a list. Returns the new length.
func (c *Client) RPush(key string, elems ...string) (int, error) {
	return c.transport.RPush(key, elems...)
}

// LPop removes and returns the first element of a list.
func (c *Client) LPop(key string) (string, error) {
	return c.transport.LPop(key)
}

// RPop removes and returns the last element of a list.
func (c *Client) RPop(key string) (string, error) {
	return c.transport.RPop(key)
}

// LLen returns the length of a list.
func (c *Client) LLen(key string) (int, error) {
	return c.transport.LLen(key)
}

// LRange returns a subset of elements from a list (inclusive indices).
func (c *Client) LRange(key string, start, stop int) ([]string, error) {
	return c.transport.LRange(key, start, stop)
}

// LIndex returns the element at the given index.
func (c *Client) LIndex(key string, index int) (string, error) {
	return c.transport.LIndex(key, index)
}

// LSet sets the element at the given index.
func (c *Client) LSet(key string, index int, value string) error {
	return c.transport.LSet(key, index, value)
}

// LRem removes up to count occurrences of value from the list.
func (c *Client) LRem(key string, count int, value string) (int, error) {
	return c.transport.LRem(key, count, value)
}

// LTrim trims a list to the specified range.
func (c *Client) LTrim(key string, start, stop int) error {
	return c.transport.LTrim(key, start, stop)
}

// LInsert inserts value before or after the pivot element. before=true means before pivot.
func (c *Client) LInsert(key string, before bool, pivot, value string) (int, error) {
	return c.transport.LInsert(key, before, pivot, value)
}

// BLPop blocks until an element can be popped from the head of a list.
func (c *Client) BLPop(key string, timeout time.Duration) (string, error) {
	return c.transport.BLPop(key, timeout)
}

// BRPop blocks until an element can be popped from the tail of a list.
func (c *Client) BRPop(key string, timeout time.Duration) (string, error) {
	return c.transport.BRPop(key, timeout)
}

// LPos returns the indices of matching elements within a list.
func (c *Client) LPos(key, value string, rank, count, maxLen int) ([]int, error) {
	return c.transport.LPos(key, value, rank, count, maxLen)
}

// ---------------------------------------------------------------------------
// Set operations
// ---------------------------------------------------------------------------

// SAdd adds elements to a set. Returns the number of elements added.
func (c *Client) SAdd(key string, elems ...string) (int, error) {
	return c.transport.SAdd(key, elems...)
}

// SRem removes elements from a set. Returns the number of elements removed.
func (c *Client) SRem(key string, elems ...string) (int, error) {
	return c.transport.SRem(key, elems...)
}

// SIsMember reports whether an element is a member of a set.
func (c *Client) SIsMember(key, elem string) (bool, error) {
	return c.transport.SIsMember(key, elem)
}

// SMembers returns all members of a set.
func (c *Client) SMembers(key string) ([]string, error) {
	return c.transport.SMembers(key)
}

// SCard returns the number of elements in a set.
func (c *Client) SCard(key string) (int, error) {
	return c.transport.SCard(key)
}

// SPop removes and returns a random element from a set.
func (c *Client) SPop(key string) (string, error) {
	return c.transport.SPop(key)
}

// SRandMember returns one or more random members from a set.
func (c *Client) SRandMember(key string, count int) ([]string, error) {
	return c.transport.SRandMember(key, count)
}

// SUnion returns the union of multiple sets.
func (c *Client) SUnion(keys ...string) ([]string, error) {
	return c.transport.SUnion(keys...)
}

// SInter returns the intersection of multiple sets.
func (c *Client) SInter(keys ...string) ([]string, error) {
	return c.transport.SInter(keys...)
}

// SDiff returns the difference between the first set and all subsequent sets.
func (c *Client) SDiff(keys ...string) ([]string, error) {
	return c.transport.SDiff(keys...)
}

// ---------------------------------------------------------------------------
// Key management
// ---------------------------------------------------------------------------

// Exists reports whether a key exists.
func (c *Client) Exists(key string) (bool, error) {
	return c.transport.Exists(key)
}

// Type returns the data type of the value stored at key
// (string, set, hash, list) or "none" if the key does not exist.
func (c *Client) Type(key string) (string, error) {
	b, err := c.transport.Type(key)
	if err != nil {
		return "", err
	}
	return typeToString(b), nil
}

// Expire sets a time-to-live in seconds on a key. Returns true if set.
func (c *Client) Expire(key string, seconds int64) (bool, error) {
	return c.transport.Expire(key, seconds)
}

// PExpire sets a time-to-live in milliseconds on a key. Returns true if set.
func (c *Client) PExpire(key string, ms int64) (bool, error) {
	return c.transport.PExpire(key, ms)
}

// TTL returns the remaining time-to-live of a key in seconds.
func (c *Client) TTL(key string) (int64, error) {
	return c.transport.TTL(key)
}

// PTTL returns the remaining time-to-live of a key in milliseconds.
func (c *Client) PTTL(key string) (int64, error) {
	return c.transport.PTTL(key)
}

// Persist removes the expiration from a key. Returns true if removed.
func (c *Client) Persist(key string) (bool, error) {
	return c.transport.Persist(key)
}

// Keys returns all keys matching a glob pattern.
func (c *Client) Keys(pattern string) ([]string, error) {
	return c.transport.Keys(pattern)
}

func typeToString(b byte) string {
	switch b {
	case 0:
		return "none"
	case 1:
		return "string"
	case 2:
		return "set"
	case 3:
		return "hash"
	case 4:
		return "list"
	default:
		return "unknown"
	}
}
