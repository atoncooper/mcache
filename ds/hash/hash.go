// Package hash implements a thread-safe string-to-string hash map
// similar to Redis Hashes. Each key maps to a map of field-value pairs.
package hash

import (
	"maps"
	"strconv"
	"sync"
)

// Hash is a thread-safe map of field-value pairs.
type Hash struct {
	mu     sync.RWMutex
	fields map[string]string
}

// New creates an empty Hash.
func New() *Hash {
	return &Hash{fields: make(map[string]string)}
}

// HSet sets field to value. Returns 1 if field is new, 0 if updated.
func (h *Hash) HSet(field, value string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.fields[field]; ok {
		h.fields[field] = value
		return 0
	}
	h.fields[field] = value
	return 1
}

// HSetNX sets field to value only if field does not exist.
func (h *Hash) HSetNX(field, value string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.fields[field]; ok {
		return false
	}
	h.fields[field] = value
	return true
}

// HGet returns the value of field.
func (h *Hash) HGet(field string) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	v, ok := h.fields[field]
	return v, ok
}

// HDel removes fields. Returns the number of fields actually removed.
func (h *Hash) HDel(fields ...string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	removed := 0
	for _, f := range fields {
		if _, ok := h.fields[f]; ok {
			delete(h.fields, f)
			removed++
		}
	}
	return removed
}

// HExists reports whether field exists.
func (h *Hash) HExists(field string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.fields[field]
	return ok
}

// HGetAll returns all field-value pairs.
func (h *Hash) HGetAll() map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[string]string, len(h.fields))
	maps.Copy(out, h.fields)
	return out
}

// HKeys returns all field names.
func (h *Hash) HKeys() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]string, 0, len(h.fields))
	for k := range h.fields {
		out = append(out, k)
	}
	return out
}

// HVals returns all values.
func (h *Hash) HVals() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]string, 0, len(h.fields))
	for _, v := range h.fields {
		out = append(out, v)
	}
	return out
}

// HLen returns the number of fields.
func (h *Hash) HLen() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.fields)
}

// HStrLen returns the string length of the value at field, or 0 if not found.
func (h *Hash) HStrLen(field string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	v, ok := h.fields[field]
	if !ok {
		return 0
	}
	return len(v)
}

// HIncrBy increments the integer value of field by delta.
func (h *Hash) HIncrBy(field string, delta int64) (int64, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	v, ok := h.fields[field]
	if !ok {
		h.fields[field] = strconv.FormatInt(delta, 10)
		return delta, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, err
	}
	n += delta
	h.fields[field] = strconv.FormatInt(n, 10)
	return n, nil
}

// HIncrByFloat increments the float value of field by delta.
func (h *Hash) HIncrByFloat(field string, delta float64) (float64, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	v, ok := h.fields[field]
	if !ok {
		h.fields[field] = strconv.FormatFloat(delta, 'f', -1, 64)
		return delta, nil
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	n += delta
	h.fields[field] = strconv.FormatFloat(n, 'f', -1, 64)
	return n, nil
}

// HMGet returns values for the given fields. Missing fields are nil in the result.
func (h *Hash) HMGet(fields ...string) []any {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]any, len(fields))
	for i, f := range fields {
		if v, ok := h.fields[f]; ok {
			out[i] = v
		}
	}
	return out
}

// HMSet sets multiple field-value pairs. Odd number of arguments truncates the last.
func (h *Hash) HMSet(fvPairs ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := 0; i+1 < len(fvPairs); i += 2 {
		h.fields[fvPairs[i]] = fvPairs[i+1]
	}
}

// ForEach calls fn for each field-value pair. Stops if fn returns false.
func (h *Hash) ForEach(fn func(field, value string) bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for k, v := range h.fields {
		if !fn(k, v) {
			return
		}
	}
}

// Fields returns the number of fields.
func (h *Hash) Fields() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.fields)
}
