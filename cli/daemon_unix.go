//go:build !windows

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func daemonize() {
	// Re-exec the current binary with the same args minus --daemon.
	args := make([]string, 0, len(os.Args))
	for _, a := range os.Args[1:] {
		if a == "--daemon" {
			continue
		}
		args = append(args, a)
	}

	cmd := exec.Command(os.Args[0], args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "daemon: failed to start: %v\n", err)
		os.Exit(1)
	}

	// Write PID file if requested.
	if serverPidFile != "" {
		pid := fmt.Sprintf("%d\n", cmd.Process.Pid)
		os.WriteFile(serverPidFile, []byte(pid), 0644)
	}

	fmt.Printf("Server started (pid: %d)\n", cmd.Process.Pid)
	os.Exit(0)
}
