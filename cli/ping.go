package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Check connectivity to the server",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		start := time.Now()
		cliLogger.Info("request start", map[string]any{"cmd": "ping", "addr": globalAddr})

		client, err := newCmdClient()
		if err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "ping", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("connect: %v", err)
		}
		defer client.Close()

		_, err = client.Len()
		if err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "ping", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("ping: %v", err)
		}
		cliLogger.Info("request done", map[string]any{"cmd": "ping", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": true})
		fmt.Printf("PONG (%.3fms)\n", float64(time.Since(start).Microseconds())/1000.0)
	},
}

func init() {
	rootCmd.AddCommand(pingCmd)
}
