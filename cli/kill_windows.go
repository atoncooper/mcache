//go:build windows

package cli

import (
	"fmt"
	"os"
)

func killProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}
	if err := proc.Kill(); err != nil {
		return fmt.Errorf("kill process: %w", err)
	}
	return nil
}
