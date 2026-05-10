//go:build !linux

package monitor

import "errors"

// ProcCollector is not available on non-Linux platforms.
type ProcCollector struct{}

// NewProc returns a stub that reports ErrNotSupported on every Collect.
func NewProc() *ProcCollector {
	return &ProcCollector{}
}

func (c *ProcCollector) Name() string { return "proc" }

func (c *ProcCollector) Collect() (*SystemSnapshot, error) {
	return nil, errors.New("ProcCollector is only supported on Linux")
}
