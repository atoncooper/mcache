package net

import (
	"errors"

	"github.com/atoncooper/mcache"
)

var (
	ErrServerClosed   = errors.New("server closed")
	ErrInvalidCommand = errors.New("invalid command")
	ErrReadTimeout    = errors.New("read timeout")
	ErrConnClosed     = errors.New("connection closed")
	ErrBadResponse    = errors.New("bad response from server")
	ErrNotLeader      = mcache.ErrNotLeader
)
