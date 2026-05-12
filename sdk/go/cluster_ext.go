package mcache

import "time"

// ---------------------------------------------------------------------------
// Hash operations
// ---------------------------------------------------------------------------

// HSet sets a field in a hash on the node responsible for the key.
func (cc *ClusterClient) HSet(key, field, value string) (int, error) {
	return cc.pickNode(key).HSet(key, field, value)
}

// HSetNX sets a field only if it does not already exist.
func (cc *ClusterClient) HSetNX(key, field, value string) (bool, error) {
	return cc.pickNode(key).HSetNX(key, field, value)
}

// HGet returns the value of a hash field.
func (cc *ClusterClient) HGet(key, field string) (string, error) {
	return cc.pickNode(key).HGet(key, field)
}

// HDel deletes fields from a hash. Returns the number of fields removed.
func (cc *ClusterClient) HDel(key string, fields ...string) (int, error) {
	return cc.pickNode(key).HDel(key, fields...)
}

// HExists reports whether a field exists in the hash.
func (cc *ClusterClient) HExists(key, field string) (bool, error) {
	return cc.pickNode(key).HExists(key, field)
}

// HGetAll returns all field-value pairs in a hash.
func (cc *ClusterClient) HGetAll(key string) (map[string]string, error) {
	return cc.pickNode(key).HGetAll(key)
}

// HKeys returns all field names in a hash.
func (cc *ClusterClient) HKeys(key string) ([]string, error) {
	return cc.pickNode(key).HKeys(key)
}

// HVals returns all values in a hash.
func (cc *ClusterClient) HVals(key string) ([]string, error) {
	return cc.pickNode(key).HVals(key)
}

// HLen returns the number of fields in a hash.
func (cc *ClusterClient) HLen(key string) (int, error) {
	return cc.pickNode(key).HLen(key)
}

// HStrLen returns the string length of a field's value.
func (cc *ClusterClient) HStrLen(key, field string) (int, error) {
	return cc.pickNode(key).HStrLen(key, field)
}

// HIncrBy increments the integer value of a hash field.
func (cc *ClusterClient) HIncrBy(key, field string, delta int64) (int64, error) {
	return cc.pickNode(key).HIncrBy(key, field, delta)
}

// HIncrByFloat increments the float value of a hash field.
func (cc *ClusterClient) HIncrByFloat(key, field string, delta float64) (float64, error) {
	return cc.pickNode(key).HIncrByFloat(key, field, delta)
}

// HMGet returns the values of multiple hash fields.
func (cc *ClusterClient) HMGet(key string, fields ...string) ([]any, error) {
	return cc.pickNode(key).HMGet(key, fields...)
}

// HMSet sets multiple field-value pairs in a hash.
func (cc *ClusterClient) HMSet(key string, fvPairs ...string) error {
	return cc.pickNode(key).HMSet(key, fvPairs...)
}

// ---------------------------------------------------------------------------
// List operations
// ---------------------------------------------------------------------------

// LPush inserts elements at the head of a list. Returns the new length.
func (cc *ClusterClient) LPush(key string, elems ...string) (int, error) {
	return cc.pickNode(key).LPush(key, elems...)
}

// RPush appends elements to the tail of a list. Returns the new length.
func (cc *ClusterClient) RPush(key string, elems ...string) (int, error) {
	return cc.pickNode(key).RPush(key, elems...)
}

// LPop removes and returns the first element of a list.
func (cc *ClusterClient) LPop(key string) (string, error) {
	return cc.pickNode(key).LPop(key)
}

// RPop removes and returns the last element of a list.
func (cc *ClusterClient) RPop(key string) (string, error) {
	return cc.pickNode(key).RPop(key)
}

// LLen returns the length of a list.
func (cc *ClusterClient) LLen(key string) (int, error) {
	return cc.pickNode(key).LLen(key)
}

// LRange returns a subset of elements from a list.
func (cc *ClusterClient) LRange(key string, start, stop int) ([]string, error) {
	return cc.pickNode(key).LRange(key, start, stop)
}

// LIndex returns the element at the given index.
func (cc *ClusterClient) LIndex(key string, index int) (string, error) {
	return cc.pickNode(key).LIndex(key, index)
}

// LSet sets the element at the given index.
func (cc *ClusterClient) LSet(key string, index int, value string) error {
	return cc.pickNode(key).LSet(key, index, value)
}

// LRem removes occurrences of value from the list.
func (cc *ClusterClient) LRem(key string, count int, value string) (int, error) {
	return cc.pickNode(key).LRem(key, count, value)
}

// LTrim trims a list to the specified range.
func (cc *ClusterClient) LTrim(key string, start, stop int) error {
	return cc.pickNode(key).LTrim(key, start, stop)
}

// LInsert inserts value before or after the pivot element.
func (cc *ClusterClient) LInsert(key string, before bool, pivot, value string) (int, error) {
	return cc.pickNode(key).LInsert(key, before, pivot, value)
}

// BLPop blocks until an element can be popped from the head of a list.
func (cc *ClusterClient) BLPop(key string, timeout time.Duration) (string, error) {
	return cc.pickNode(key).BLPop(key, timeout)
}

// BRPop blocks until an element can be popped from the tail of a list.
func (cc *ClusterClient) BRPop(key string, timeout time.Duration) (string, error) {
	return cc.pickNode(key).BRPop(key, timeout)
}

// LPos returns the indices of matching elements within a list.
func (cc *ClusterClient) LPos(key, value string, rank, count, maxLen int) ([]int, error) {
	return cc.pickNode(key).LPos(key, value, rank, count, maxLen)
}

// ---------------------------------------------------------------------------
// Set operations
// ---------------------------------------------------------------------------

// SAdd adds elements to a set. Returns the number of elements added.
func (cc *ClusterClient) SAdd(key string, elems ...string) (int, error) {
	return cc.pickNode(key).SAdd(key, elems...)
}

// SRem removes elements from a set. Returns the number of elements removed.
func (cc *ClusterClient) SRem(key string, elems ...string) (int, error) {
	return cc.pickNode(key).SRem(key, elems...)
}

// SIsMember reports whether an element is a member of a set.
func (cc *ClusterClient) SIsMember(key, elem string) (bool, error) {
	return cc.pickNode(key).SIsMember(key, elem)
}

// SMembers returns all members of a set.
func (cc *ClusterClient) SMembers(key string) ([]string, error) {
	return cc.pickNode(key).SMembers(key)
}

// SCard returns the number of elements in a set.
func (cc *ClusterClient) SCard(key string) (int, error) {
	return cc.pickNode(key).SCard(key)
}

// SPop removes and returns a random element from a set.
func (cc *ClusterClient) SPop(key string) (string, error) {
	return cc.pickNode(key).SPop(key)
}

// SRandMember returns random members from a set.
func (cc *ClusterClient) SRandMember(key string, count int) ([]string, error) {
	return cc.pickNode(key).SRandMember(key, count)
}

// SUnion returns the union of multiple sets. Routes by the first key.
func (cc *ClusterClient) SUnion(keys ...string) ([]string, error) {
	if len(keys) == 0 {
		return nil, ErrKeyEmpty
	}
	return cc.pickNode(keys[0]).SUnion(keys...)
}

// SInter returns the intersection of multiple sets. Routes by the first key.
func (cc *ClusterClient) SInter(keys ...string) ([]string, error) {
	if len(keys) == 0 {
		return nil, ErrKeyEmpty
	}
	return cc.pickNode(keys[0]).SInter(keys...)
}

// SDiff returns the difference between sets. Routes by the first key.
func (cc *ClusterClient) SDiff(keys ...string) ([]string, error) {
	if len(keys) == 0 {
		return nil, ErrKeyEmpty
	}
	return cc.pickNode(keys[0]).SDiff(keys...)
}

// ---------------------------------------------------------------------------
// Key management
// ---------------------------------------------------------------------------

// Exists reports whether a key exists.
func (cc *ClusterClient) Exists(key string) (bool, error) {
	return cc.pickNode(key).Exists(key)
}

// Type returns the data type stored at key.
func (cc *ClusterClient) Type(key string) (string, error) {
	return cc.pickNode(key).Type(key)
}

// Expire sets a time-to-live in seconds on a key.
func (cc *ClusterClient) Expire(key string, seconds int64) (bool, error) {
	return cc.pickNode(key).Expire(key, seconds)
}

// PExpire sets a time-to-live in milliseconds on a key.
func (cc *ClusterClient) PExpire(key string, ms int64) (bool, error) {
	return cc.pickNode(key).PExpire(key, ms)
}

// TTL returns the remaining time-to-live of a key in seconds.
func (cc *ClusterClient) TTL(key string) (int64, error) {
	return cc.pickNode(key).TTL(key)
}

// PTTL returns the remaining time-to-live of a key in milliseconds.
func (cc *ClusterClient) PTTL(key string) (int64, error) {
	return cc.pickNode(key).PTTL(key)
}

// Persist removes the expiration from a key.
func (cc *ClusterClient) Persist(key string) (bool, error) {
	return cc.pickNode(key).Persist(key)
}

// Keys returns all keys matching a glob pattern. Aggregates results from all nodes.
func (cc *ClusterClient) Keys(pattern string) ([]string, error) {
	var all []string
	for _, node := range cc.nodes {
		keys, err := node.client.Keys(pattern)
		if err != nil {
			return nil, err
		}
		all = append(all, keys...)
	}
	return all, nil
}
