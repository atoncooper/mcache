//go:build !windows

package cli

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

func killProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}

	// SIGTERM first
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM: %w", err)
	}

	// Wait for graceful exit
	for i := 0; i < 60; i++ {
		time.Sleep(100 * time.Millisecond)
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			// Process no longer exists
			return nil
		}
	}

	// Force kill after timeout
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		return fmt.Errorf("send SIGKILL: %w", err)
	}
	return nil
}
