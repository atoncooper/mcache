package net

import "errors"

var (
	ErrServerClosed   = errors.New("server closed")
	ErrInvalidCommand = errors.New("invalid command")
	ErrReadTimeout    = errors.New("read timeout")
	ErrConnClosed     = errors.New("connection closed")
	ErrBadResponse    = errors.New("bad response from server")
)
