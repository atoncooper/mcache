package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var stopPidFile string

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a background mcache server",
	Long: `Stop a running mcache server started with --daemon --pidfile.

The PID file (default: mcache.pid) is read to find the server process.
SIGTERM is sent first; if the process does not exit within the timeout,
SIGKILL is sent.`,
	Run: func(cmd *cobra.Command, args []string) {
		data, err := os.ReadFile(stopPidFile)
		if err != nil {
			exitError("cannot read pidfile %q: %v", stopPidFile, err)
		}

		pidStr := strings.TrimSpace(string(data))
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid <= 0 {
			exitError("invalid pid in %q: %s", stopPidFile, pidStr)
		}

		if err := killProcess(pid); err != nil {
			exitError("failed to stop server (pid %d): %v", pid, err)
		}

		os.Remove(stopPidFile)
		fmt.Printf("Server stopped (pid %d)\n", pid)
	},
}

func init() {
	stopCmd.Flags().StringVar(&stopPidFile, "pidfile", "mcache.pid", "PID file of the server to stop")
	rootCmd.AddCommand(stopCmd)
}
