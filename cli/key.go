package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// --- Key Management CLI Commands ---

var existsCmd = &cobra.Command{
	Use:   "exists <key>",
	Short: "Check if a key exists",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		found, err := client.Exists(args[0])
		if err != nil {
			exitError("%v", err)
		}
		if found {
			fmt.Println("1")
		} else {
			fmt.Println("0")
		}
	},
}

func typeToString(t byte) string {
	switch t {
	case 1:
		return "string"
	case 2:
		return "set"
	case 3:
		return "hash"
	case 4:
		return "list"
	default:
		return "none"
	}
}

var typeCmd = &cobra.Command{
	Use:   "type <key>",
	Short: "Return the type of a key",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		t, err := client.Type(args[0])
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(t)
	},
}

var expireCmd = &cobra.Command{
	Use:   "expire <key> <seconds>",
	Short: "Set a TTL (in seconds) on a key",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		seconds, _ := strconv.ParseInt(args[1], 10, 64)
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		ok, err := client.Expire(args[0], seconds)
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

var pexpireCmd = &cobra.Command{
	Use:   "pexpire <key> <milliseconds>",
	Short: "Set a TTL (in milliseconds) on a key",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		ms, _ := strconv.ParseInt(args[1], 10, 64)
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		ok, err := client.PExpire(args[0], ms)
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

var ttlCmd = &cobra.Command{
	Use:   "ttl <key>",
	Short: "Return the remaining TTL in seconds",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		ttl, err := client.TTL(args[0])
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(ttl)
	},
}

var pttlCmd = &cobra.Command{
	Use:   "pttl <key>",
	Short: "Return the remaining TTL in milliseconds",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		ttl, err := client.PTTL(args[0])
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(ttl)
	},
}

var persistCmd = &cobra.Command{
	Use:   "persist <key>",
	Short: "Remove the TTL from a key",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		ok, err := client.Persist(args[0])
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

var keysCmd = &cobra.Command{
	Use:   "keys <pattern>",
	Short: "Find all keys matching a pattern",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()
		keys, err := client.Keys(args[0])
		if err != nil {
			exitError("%v", err)
		}
		for _, k := range keys {
			fmt.Println(k)
		}
	},
}

func init() {
	rootCmd.AddCommand(existsCmd)
	rootCmd.AddCommand(typeCmd)
	rootCmd.AddCommand(expireCmd)
	rootCmd.AddCommand(pexpireCmd)
	rootCmd.AddCommand(ttlCmd)
	rootCmd.AddCommand(pttlCmd)
	rootCmd.AddCommand(persistCmd)
	rootCmd.AddCommand(keysCmd)
}
