package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var sAddCmd = &cobra.Command{
	Use:   "sadd <key> <element> [element...]",
	Short: "Add one or more elements to a set",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		start := time.Now()
		cliLogger.Info("request start", map[string]any{"cmd": "sadd", "addr": globalAddr, "key": key})

		client, err := newCmdClient()
		if err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "sadd", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		defer client.Close()

		added, err := client.SAdd(key, args[1:]...)
		if err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "sadd", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		cliLogger.Info("request done", map[string]any{"cmd": "sadd", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": true, "added": added})
		fmt.Printf("%d\n", added)
	},
}

var sRemCmd = &cobra.Command{
	Use:   "srem <key> <element> [element...]",
	Short: "Remove one or more elements from a set",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		start := time.Now()
		cliLogger.Info("request start", map[string]any{"cmd": "srem", "addr": globalAddr, "key": key})

		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()

		removed, err := client.SRem(key, args[1:]...)
		if err != nil {
			exitError("%v", err)
		}
		cliLogger.Info("request done", map[string]any{"cmd": "srem", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": true, "removed": removed})
		fmt.Printf("%d\n", removed)
	},
}

var sIsMemberCmd = &cobra.Command{
	Use:   "sismember <key> <element>",
	Short: "Check if an element is in a set",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		elem := strings.Join(args[1:], " ")

		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()

		member, err := client.SIsMember(key, elem)
		if err != nil {
			exitError("%v", err)
		}
		if member {
			fmt.Println("1")
		} else {
			fmt.Println("0")
		}
	},
}

var sMembersCmd = &cobra.Command{
	Use:   "smembers <key>",
	Short: "List all elements in a set",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()

		elems, err := client.SMembers(key)
		if err != nil {
			exitError("%v", err)
		}
		for _, e := range elems {
			fmt.Println(e)
		}
	},
}

var sCardCmd = &cobra.Command{
	Use:   "scard <key>",
	Short: "Return the number of elements in a set",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()

		n, err := client.SCard(key)
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(n)
	},
}

var sPopCmd = &cobra.Command{
	Use:   "spop <key>",
	Short: "Remove and return a random element from a set",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()

		elem, err := client.SPop(key)
		if err != nil {
			exitError("%v", err)
		}
		fmt.Println(elem)
	},
}

var sRandMemberCmd = &cobra.Command{
	Use:   "srandmember <key> [count]",
	Short: "Return random elements from a set",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		count := 1
		if len(args) > 1 {
			fmt.Sscanf(args[1], "%d", &count)
		}

		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()

		elems, err := client.SRandMember(key, count)
		if err != nil {
			exitError("%v", err)
		}
		for _, e := range elems {
			fmt.Println(e)
		}
	},
}

var sUnionCmd = &cobra.Command{
	Use:   "sunion <key> [key...]",
	Short: "Return the union of multiple sets",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()

		elems, err := client.SUnion(args...)
		if err != nil {
			exitError("%v", err)
		}
		for _, e := range elems {
			fmt.Println(e)
		}
	},
}

var sInterCmd = &cobra.Command{
	Use:   "sinter <key> [key...]",
	Short: "Return the intersection of multiple sets",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()

		elems, err := client.SInter(args...)
		if err != nil {
			exitError("%v", err)
		}
		for _, e := range elems {
			fmt.Println(e)
		}
	},
}

var sDiffCmd = &cobra.Command{
	Use:   "sdiff <key> [key...]",
	Short: "Return elements in first set not in subsequent sets",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()

		elems, err := client.SDiff(args...)
		if err != nil {
			exitError("%v", err)
		}
		for _, e := range elems {
			fmt.Println(e)
		}
	},
}

func init() {
	rootCmd.AddCommand(sAddCmd)
	rootCmd.AddCommand(sRemCmd)
	rootCmd.AddCommand(sIsMemberCmd)
	rootCmd.AddCommand(sMembersCmd)
	rootCmd.AddCommand(sCardCmd)
	rootCmd.AddCommand(sPopCmd)
	rootCmd.AddCommand(sRandMemberCmd)
	rootCmd.AddCommand(sUnionCmd)
	rootCmd.AddCommand(sInterCmd)
	rootCmd.AddCommand(sDiffCmd)
}
