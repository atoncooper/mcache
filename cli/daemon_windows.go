//go:build windows

package cli

import (
	"fmt"
	"os"
)

func daemonize() {
	fmt.Fprintln(os.Stderr, "Daemon mode is not supported on Windows.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Use one of the following instead:")
	fmt.Fprintln(os.Stderr, "  1. Docker:       docker run -d --name mcache -p 11211:11211 atoncooper/mcache")
	fmt.Fprintln(os.Stderr, "  2. PowerShell:    Start-Process -NoNewWindow .\\mcache.exe server")
	fmt.Fprintln(os.Stderr, "  3. Task Scheduler: Create a scheduled task to run at startup")
	os.Exit(1)
}
