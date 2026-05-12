package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mcache",
	Short: "mcache — in-memory cache server and CLI",
	Long: `mcache is a unified command for running the mcache server and interacting with it.

Server:
  server    Start the TCP cache server
  stop      Stop a background server

KV:
  get       Retrieve a value by key
  set       Store a key-value pair
  del       Delete a key
  len       Show entry count
  cleanup   Trigger expiration cleanup
  ping      Check server connectivity
  stats     Show server statistics

Hash:
  hset, hsetnx, hget, hdel, hexists, hgetall, hkeys, hvals,
  hlen, hstrlen, hincrby, hincrbyfloat, hmget, hmset

List:
  lpush, rpush, lpop, rpop, blpop, brpop, llen, lrange,
  lindex, lset, lrem, ltrim, linsert, lpos

Set:
  sadd, srem, sismember, smembers, scard, spop,
  srandmember, sunion, sinter, sdiff

Key Management:
  exists, type, expire, pexpire, ttl, pttl, persist, keys

Other:
  monitor   Show real-time system resource usage
  repl      Start interactive REPL
  version   Print version information

Use "mcache [command] --help" for more information about a command.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&globalAddr, "addr", "a", "127.0.0.1:11211", "mcache server address")
	rootCmd.PersistentFlags().DurationVarP(&globalTimeout, "timeout", "t", 10*time.Second, "operation timeout")
	rootCmd.PersistentFlags().IntVar(&globalPool, "pool", 4, "connection pool size")
	rootCmd.PersistentFlags().StringVar(&clusterConfigPath, "cluster-config", "", "cluster config file (enables cluster mode)")
}

func exitError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
