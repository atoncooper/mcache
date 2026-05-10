package mcache

import "errors"

var (
	ErrKeyNotFound = errors.New("key not found")
	ErrKeyEmpty    = errors.New("key cannot be empty")
	ErrValueNil    = errors.New("value cannot be nil")
	ErrConnClosed  = errors.New("connection closed")
	ErrTimeout     = errors.New("operation timeout")
	ErrNoNodes     = errors.New("no available nodes")
)
