package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

// --- List CLI Commands ---

var lPushCmd = &cobra.Command{
	Use:   "lpush <key> <element> [element...]",
	Short: "Prepend elements to a list",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		logger := Logger()
		start := time.Now()
		logger.Info("request start", map[string]any{"cmd": "lpush", "addr": globalAddr, "key": key})
		client, err := newCmdClient()
		if err != nil {
			logger.Info("request done", map[string]any{"cmd": "lpush", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		defer client.Close()
		n, err := client.LPush(args[0], args[1:]...)
		if err != nil {
			logger.Info("request done", map[string]any{"cmd": "lpush", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		logger.Info("request done", map[string]any{"cmd": "lpush", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": true})
		fmt.Println(n)
	},
}

var rPushCmd = &cobra.Command{
	Use:   "rpush <key> <element> [element...]",
	Short: "Append elements to a list",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		n, err := client.RPush(args[0], args[1:]...)
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(n)
	},
}

var lPopCmd = &cobra.Command{
	Use:   "lpop <key>",
	Short: "Remove and return the first element of a list",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		elem, err := client.LPop(args[0])
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(elem)
	},
}

var rPopCmd = &cobra.Command{
	Use:   "rpop <key>",
	Short: "Remove and return the last element of a list",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		elem, err := client.RPop(args[0])
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(elem)
	},
}

var lLenCmd = &cobra.Command{
	Use:   "llen <key>",
	Short: "Return the length of a list",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		n, err := client.LLen(args[0])
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(n)
	},
}

var lRangeCmd = &cobra.Command{
	Use:   "lrange <key> <start> <stop>",
	Short: "Return a range of elements from a list",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		start, _ := strconv.Atoi(args[1])
		stop, _ := strconv.Atoi(args[2])
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		elems, err := client.LRange(args[0], start, stop)
		if err != nil {
			exitError("%v", err)
		}
		for _, e := range elems {
			fmt.Println(e)
		}
	},
}

var lIndexCmd = &cobra.Command{
	Use:   "lindex <key> <index>",
	Short: "Return the element at index in a list",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		index, _ := strconv.Atoi(args[1])
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		elem, err := client.LIndex(args[0], index)
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(elem)
	},
}

var lSetCmd = &cobra.Command{
	Use:   "lset <key> <index> <value>",
	Short: "Set the element at index in a list",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		index, _ := strconv.Atoi(args[1])
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		if err := client.LSet(args[0], index, args[2]); err != nil {
			exitError("%v", err)
		}
		fmt.Println("OK")
	},
}

var lRemCmd = &cobra.Command{
	Use:   "lrem <key> <count> <value>",
	Short: "Remove elements from a list",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		count, _ := strconv.Atoi(args[1])
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		n, err := client.LRem(args[0], count, args[2])
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(n)
	},
}

var lTrimCmd = &cobra.Command{
	Use:   "ltrim <key> <start> <stop>",
	Short: "Trim a list to the specified range",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		start, _ := strconv.Atoi(args[1])
		stop, _ := strconv.Atoi(args[2])
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		if err := client.LTrim(args[0], start, stop); err != nil {
			exitError("%v", err)
		}
		fmt.Println("OK")
	},
}

var lInsertCmd = &cobra.Command{
	Use:   "linsert <key> before|after <pivot> <value>",
	Short: "Insert an element before or after a pivot value",
	Args:  cobra.ExactArgs(4),
	Run: func(cmd *cobra.Command, args []string) {
		var before bool
		switch args[1] {
		case "before", "BEFORE":
			before = true
		case "after", "AFTER":
			before = false
		default:
			exitError("expected 'before' or 'after', got %q", args[1])
		}
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		n, err := client.LInsert(args[0], before, args[2], args[3])
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(n)
	},
}

var bLPopCmd = &cobra.Command{
	Use:   "blpop <key> <timeout>",
	Short: "Block and remove the first element of a list (timeout in seconds)",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		timeout, _ := strconv.Atoi(args[1])
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		elem, err := client.BLPop(args[0], time.Duration(timeout)*time.Second)
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(elem)
	},
}

var bRPopCmd = &cobra.Command{
	Use:   "brpop <key> <timeout>",
	Short: "Block and remove the last element of a list (timeout in seconds)",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		timeout, _ := strconv.Atoi(args[1])
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		elem, err := client.BRPop(args[0], time.Duration(timeout)*time.Second)
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(elem)
	},
}

var lPosCmd = &cobra.Command{
	Use:   "lpos <key> <value> [rank] [count] [maxlen]",
	Short: "Return the index of matching elements in a list",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		rank := 1
		count := 1
		maxLen := 0
		if len(args) > 2 {
			rank, _ = strconv.Atoi(args[2])
		}
		if len(args) > 3 {
			count, _ = strconv.Atoi(args[3])
		}
		if len(args) > 4 {
			maxLen, _ = strconv.Atoi(args[4])
		}
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		positions, err := client.LPos(args[0], args[1], rank, count, maxLen)
		if err != nil {
			exitError("%v", err)
		}
		for _, p := range positions {
			fmt.Println(p)
		}
	},
}

func init() {
	rootCmd.AddCommand(lPushCmd)
	rootCmd.AddCommand(rPushCmd)
	rootCmd.AddCommand(lPopCmd)
	rootCmd.AddCommand(rPopCmd)
	rootCmd.AddCommand(lLenCmd)
	rootCmd.AddCommand(lRangeCmd)
	rootCmd.AddCommand(lIndexCmd)
	rootCmd.AddCommand(lSetCmd)
	rootCmd.AddCommand(lRemCmd)
	rootCmd.AddCommand(lTrimCmd)
	rootCmd.AddCommand(lInsertCmd)
	rootCmd.AddCommand(bLPopCmd)
	rootCmd.AddCommand(bRPopCmd)
	rootCmd.AddCommand(lPosCmd)
}
