package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var setTTL time.Duration

var getCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a value by key",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		start := time.Now()
		cliLogger.Info("request start", map[string]any{"cmd": "get", "addr": globalAddr, "key": key})

		client, err := newCmdClient()
		if err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "get", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		defer client.Close()

		val, err := client.Get(key)
		if err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "get", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		cliLogger.Info("request done", map[string]any{"cmd": "get", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": true})
		fmt.Println(string(val))
	},
}

var setCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a key to a value",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		value := args[1]
		if len(args) > 2 {
			value = strings.Join(args[1:], " ")
		}

		start := time.Now()
		cliLogger.Info("request start", map[string]any{"cmd": "set", "addr": globalAddr, "key": key, "ttl": setTTL.String()})

		client, err := newCmdClient()
		if err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "set", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		defer client.Close()

		if err := client.Set(key, []byte(value), setTTL); err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "set", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		cliLogger.Info("request done", map[string]any{"cmd": "set", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": true})
		fmt.Println("OK")
	},
}

var delCmd = &cobra.Command{
	Use:   "del <key>",
	Short: "Delete a key",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		start := time.Now()
		cliLogger.Info("request start", map[string]any{"cmd": "del", "addr": globalAddr, "key": key})

		client, err := newCmdClient()
		if err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "del", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		defer client.Close()

		if err := client.Del(key); err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "del", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		cliLogger.Info("request done", map[string]any{"cmd": "del", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": true})
		fmt.Println("OK")
	},
}

var lenCmd = &cobra.Command{
	Use:   "len",
	Short: "Return the number of entries in the cache",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		start := time.Now()
		cliLogger.Info("request start", map[string]any{"cmd": "len", "addr": globalAddr})

		client, err := newCmdClient()
		if err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "len", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		defer client.Close()

		n, err := client.Len()
		if err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "len", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		cliLogger.Info("request done", map[string]any{"cmd": "len", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": true, "result": n})
		fmt.Println(n)
	},
}

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Trigger expiration cleanup and return count removed",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		start := time.Now()
		cliLogger.Info("request start", map[string]any{"cmd": "cleanup", "addr": globalAddr})

		client, err := newCmdClient()
		if err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "cleanup", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		defer client.Close()

		n, err := client.Cleanup()
		if err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "cleanup", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		cliLogger.Info("request done", map[string]any{"cmd": "cleanup", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": true, "removed": n})
		fmt.Printf("removed %d expired entries\n", n)
	},
}

func init() {
	setCmd.Flags().DurationVar(&setTTL, "ttl", 0, "TTL duration (e.g. 30s, 5m, 1h, 1h30m)")

	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(setCmd)
	rootCmd.AddCommand(delCmd)
	rootCmd.AddCommand(lenCmd)
	rootCmd.AddCommand(cleanupCmd)
}
