package mcache

import (
	"errors"

	"github.com/atoncooper/mcache"
)

var (
	ErrKeyNotFound = mcache.ErrKeyNotFound
	ErrKeyEmpty    = errors.New("key cannot be empty")
	ErrValueNil    = errors.New("value cannot be nil")
	ErrConnClosed  = errors.New("connection closed")
	ErrTimeout     = errors.New("operation timeout")
	ErrNoNodes     = errors.New("no available nodes")
	ErrNotLeader   = mcache.ErrNotLeader
)
