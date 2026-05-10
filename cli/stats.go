package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show server statistics",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		start := time.Now()
		cliLogger.Info("request start", map[string]any{"cmd": "stats", "addr": globalAddr})

		client, err := newCmdClient()
		if err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "stats", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		defer client.Close()

		n, err := client.Len()
		if err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "stats", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		cliLogger.Info("request done", map[string]any{"cmd": "stats", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": true, "entries": n})
		fmt.Printf("entries: %d\n", n)
		fmt.Printf("server:  %s\n", globalAddr)
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
}
