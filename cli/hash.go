package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

// --- Hash CLI Commands ---

var hSetCmd = &cobra.Command{
	Use:   "hset <key> <field> <value>",
	Short: "Set a field in a hash",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		key, field, value := args[0], args[1], args[2]
		start := time.Now()
		cliLogger.Info("request start", map[string]any{"cmd": "hset", "addr": globalAddr, "key": key})
		client, err := newCmdClient()
		if err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "hset", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		defer client.Close()
		added, err := client.HSet(key, field, value)
		if err != nil {
			exitError("%v", err)
		}
		cliLogger.Info("request done", map[string]any{"cmd": "hset", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": true, "added": added})
		fmt.Println(added)
	},
}

var hGetCmd = &cobra.Command{
	Use:   "hget <key> <field>",
	Short: "Get a field value from a hash",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		v, err := client.HGet(args[0], args[1])
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(v)
	},
}

var hDelCmd = &cobra.Command{
	Use:   "hdel <key> <field> [field...]",
	Short: "Delete fields from a hash",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		n, err := client.HDel(args[0], args[1:]...)
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(n)
	},
}

var hExistsCmd = &cobra.Command{
	Use:   "hexists <key> <field>",
	Short: "Check if a field exists in a hash",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		ok, err := client.HExists(args[0], args[1])
		if err != nil {
			exitError("%v", err)
		}
		if ok {
			fmt.Println("1")
		} else {
			fmt.Println("0")
		}
	},
}

var hGetAllCmd = &cobra.Command{
	Use:   "hgetall <key>",
	Short: "Get all field-value pairs from a hash",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		all, err := client.HGetAll(args[0])
		if err != nil {
			exitError("%v", err)
		}
		for k, v := range all {
			fmt.Printf("%s: %s\n", k, v)
		}
	},
}

var hKeysCmd = &cobra.Command{
	Use:   "hkeys <key>",
	Short: "Get all field names from a hash",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		keys, err := client.HKeys(args[0])
		if err != nil {
			exitError("%v", err)
		}
		for _, k := range keys {
			fmt.Println(k)
		}
	},
}

var hValsCmd = &cobra.Command{
	Use:   "hvals <key>",
	Short: "Get all values from a hash",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		vals, err := client.HVals(args[0])
		if err != nil {
			exitError("%v", err)
		}
		for _, v := range vals {
			fmt.Println(v)
		}
	},
}

var hLenCmd = &cobra.Command{
	Use:   "hlen <key>",
	Short: "Return the number of fields in a hash",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		n, err := client.HLen(args[0])
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(n)
	},
}

var hStrLenCmd = &cobra.Command{
	Use:   "hstrlen <key> <field>",
	Short: "Return the string length of a field's value",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		n, err := client.HStrLen(args[0], args[1])
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(n)
	},
}

var hIncrByCmd = &cobra.Command{
	Use:   "hincrby <key> <field> <delta>",
	Short: "Increment the integer value of a hash field",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		delta, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil {
			exitError("invalid delta: %v", err)
		}
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		n, err := client.HIncrBy(args[0], args[1], delta)
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(n)
	},
}

var hIncrByFloatCmd = &cobra.Command{
	Use:   "hincrbyfloat <key> <field> <delta>",
	Short: "Increment the float value of a hash field",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		delta, err := strconv.ParseFloat(args[2], 64)
		if err != nil {
			exitError("invalid delta: %v", err)
		}
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		n, err := client.HIncrByFloat(args[0], args[1], delta)
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(n)
	},
}

var hmGetCmd = &cobra.Command{
	Use:   "hmget <key> <field> [field...]",
	Short: "Get multiple field values from a hash",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		vals, err := client.HMGet(args[0], args[1:]...)
		if err != nil {
			exitError("%v", err)
		}
		for _, v := range vals {
			if v == nil {
				fmt.Println("(nil)")
			} else {
				fmt.Println(v)
			}
		}
	},
}

var hmSetCmd = &cobra.Command{
	Use:   "hmset <key> <field> <value> [field value...]",
	Short: "Set multiple field-value pairs in a hash",
	Args:  cobra.MinimumNArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		if err := client.HMSet(args[0], args[1:]...); err != nil {
			exitError("%v", err)
		}
		fmt.Println("OK")
	},
}

var hSetNXCmd = &cobra.Command{
	Use:   "hsetnx <key> <field> <value>",
	Short: "Set a field in a hash only if it doesn't exist",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		ok, err := client.HSetNX(args[0], args[1], args[2])
		if err != nil {
			exitError("%v", err)
		}
		if ok {
			fmt.Println("1")
		} else {
			fmt.Println("0")
		}
	},
}

func init() {
	rootCmd.AddCommand(hSetCmd)
	rootCmd.AddCommand(hGetCmd)
	rootCmd.AddCommand(hDelCmd)
	rootCmd.AddCommand(hExistsCmd)
	rootCmd.AddCommand(hGetAllCmd)
	rootCmd.AddCommand(hKeysCmd)
	rootCmd.AddCommand(hValsCmd)
	rootCmd.AddCommand(hLenCmd)
	rootCmd.AddCommand(hStrLenCmd)
	rootCmd.AddCommand(hIncrByCmd)
	rootCmd.AddCommand(hIncrByFloatCmd)
	rootCmd.AddCommand(hmGetCmd)
	rootCmd.AddCommand(hmSetCmd)
	rootCmd.AddCommand(hSetNXCmd)
}
