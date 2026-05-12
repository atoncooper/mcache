package mcache

import (
	"errors"

	"github.com/atoncooper/mcache/rehash"
)

var (
	ErrKeyNotFound     = errors.New("key not found")
	ErrKeyEmpty        = errors.New("key cannot be empty")
	ErrValueNil        = errors.New("value cannot be nil")
	ErrNegativeTTL     = errors.New("ttl cannot be negative")
	ErrCacheClosed     = errors.New("cache is closed")
	ErrInvalidShards   = errors.New("shard count must be power of two and >= 1")
	ErrUnknownPolicy   = errors.New("unknown eviction policy")
	ErrUnknownRehasher = rehash.ErrUnknownRehasher
	ErrFieldNotFound   = errors.New("field not found")
	ErrIndexOutOfRange = errors.New("index out of range")
	ErrInvalidIncr     = errors.New("value is not an integer or float")
	ErrNotLeader       = errors.New("not leader")
)
