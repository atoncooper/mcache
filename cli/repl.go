package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var replCmd = &cobra.Command{
	Use:   "repl",
	Short: "Start an interactive REPL session",
	Long: `Start an interactive read-eval-print loop connected to an mcache server.

Supported commands:
  get <key>                Retrieve a value
  set <key> <value> [ttl]  Store a value (ttl examples: 5m, 1h)
  del <key>                Delete a key
  len                      Show entry count
  cleanup                  Trigger expiration cleanup
  ping                     Check server connectivity
  stats                    Show server statistics
  exists <key>             Check if a key exists
  type <key>               Return the type of a key
  expire <key> <sec>       Set a TTL in seconds
  pexpire <key> <ms>       Set a TTL in milliseconds
  ttl <key>                Get remaining TTL in seconds
  pttl <key>               Get remaining TTL in milliseconds
  persist <key>            Remove the TTL from a key
  keys <pattern>           Find keys matching pattern
  help                     Show this help
  exit / quit              Exit REPL
Set commands: sadd, srem, sismember, smembers, scard, spop, srandmember, sunion, sinter, sdiff
Hash commands: hset, hget, hdel, hexists, hgetall, hkeys, hvals, hlen, hstrlen, hincrby, hincrbyfloat, hmget, hmset, hsetnx
List commands: lpush, rpush, lpop, rpop, blpop, brpop, llen, lrange, lindex, lset, lrem, ltrim, linsert, lpos
`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newCmdClient()
		if err != nil {
			exitError("%v", err)
		}
		defer client.Close()

		fmt.Printf("mcache %s connected to %s\n", Version, globalAddr)
		fmt.Println("Type 'help' for available commands, 'exit' to quit.")

		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Print("> ")
			line, err := reader.ReadString('\n')
			if err != nil {
				fmt.Println()
				return
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			parts := strings.Fields(line)
			if len(parts) == 0 {
				continue
			}

			switch parts[0] {
			case "exit", "quit":
				fmt.Println("Bye.")
				return
			case "help":
				printReplHelp()
			case "get":
				if len(parts) < 2 {
					fmt.Println("usage: get <key>")
					continue
				}
				start := time.Now()
				cliLogger.Info("repl request start", map[string]any{"cmd": "get", "addr": globalAddr, "key": parts[1]})
				val, err := client.Get(parts[1])
				if err != nil {
					cliLogger.Info("repl request done", map[string]any{"cmd": "get", "addr": globalAddr, "key": parts[1], "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
					fmt.Println("Error:", err)
				} else {
					cliLogger.Info("repl request done", map[string]any{"cmd": "get", "addr": globalAddr, "key": parts[1], "duration_ms": time.Since(start).Milliseconds(), "success": true})
					fmt.Println(string(val))
				}
			case "set":
				if len(parts) < 3 {
					fmt.Println("usage: set <key> <value> [ttl]")
					continue
				}
				key := parts[1]
				value := parts[2]
				var ttl time.Duration
				if len(parts) > 3 {
					if d, err := time.ParseDuration(parts[len(parts)-1]); err == nil {
						ttl = d
						value = strings.Join(parts[2:len(parts)-1], " ")
					} else {
						value = strings.Join(parts[2:], " ")
					}
				}
				start := time.Now()
				cliLogger.Info("repl request start", map[string]any{"cmd": "set", "addr": globalAddr, "key": key, "ttl": ttl.String()})
				if err := client.Set(key, []byte(value), ttl); err != nil {
					cliLogger.Info("repl request done", map[string]any{"cmd": "set", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
					fmt.Println("Error:", err)
				} else {
					cliLogger.Info("repl request done", map[string]any{"cmd": "set", "addr": globalAddr, "key": key, "duration_ms": time.Since(start).Milliseconds(), "success": true})
					fmt.Println("OK")
				}
			case "del":
				if len(parts) < 2 {
					fmt.Println("usage: del <key>")
					continue
				}
				start := time.Now()
				cliLogger.Info("repl request start", map[string]any{"cmd": "del", "addr": globalAddr, "key": parts[1]})
				if err := client.Del(parts[1]); err != nil {
					cliLogger.Info("repl request done", map[string]any{"cmd": "del", "addr": globalAddr, "key": parts[1], "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
					fmt.Println("Error:", err)
				} else {
					cliLogger.Info("repl request done", map[string]any{"cmd": "del", "addr": globalAddr, "key": parts[1], "duration_ms": time.Since(start).Milliseconds(), "success": true})
					fmt.Println("OK")
				}
			case "len":
				start := time.Now()
				cliLogger.Info("repl request start", map[string]any{"cmd": "len", "addr": globalAddr})
				n, err := client.Len()
				if err != nil {
					cliLogger.Info("repl request done", map[string]any{"cmd": "len", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
					fmt.Println("Error:", err)
				} else {
					cliLogger.Info("repl request done", map[string]any{"cmd": "len", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": true, "result": n})
					fmt.Println(n)
				}
			case "cleanup":
				start := time.Now()
				cliLogger.Info("repl request start", map[string]any{"cmd": "cleanup", "addr": globalAddr})
				n, err := client.Cleanup()
				if err != nil {
					cliLogger.Info("repl request done", map[string]any{"cmd": "cleanup", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
					fmt.Println("Error:", err)
				} else {
					cliLogger.Info("repl request done", map[string]any{"cmd": "cleanup", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": true, "removed": n})
					fmt.Printf("removed %d expired entries\n", n)
				}
			case "ping":
				start := time.Now()
				cliLogger.Info("repl request start", map[string]any{"cmd": "ping", "addr": globalAddr})
				_, err := client.Len()
				if err != nil {
					cliLogger.Info("repl request done", map[string]any{"cmd": "ping", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
					fmt.Println("Error:", err)
				} else {
					cliLogger.Info("repl request done", map[string]any{"cmd": "ping", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": true})
					fmt.Printf("PONG (%.3fms)\n", float64(time.Since(start).Microseconds())/1000.0)
				}
			case "stats":
				start := time.Now()
				cliLogger.Info("repl request start", map[string]any{"cmd": "stats", "addr": globalAddr})
				n, err := client.Len()
				if err != nil {
					cliLogger.Info("repl request done", map[string]any{"cmd": "stats", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
					fmt.Println("Error:", err)
				} else {
					cliLogger.Info("repl request done", map[string]any{"cmd": "stats", "addr": globalAddr, "duration_ms": time.Since(start).Milliseconds(), "success": true, "entries": n})
					fmt.Printf("entries: %d\n", n)
					fmt.Printf("server:  %s\n", globalAddr)
				}
			case "sadd":
				if len(parts) < 3 {
					fmt.Println("usage: sadd <key> <element>")
					continue
				}
				added, err := client.SAdd(parts[1], strings.Join(parts[2:], " "))
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", added)
				}
			case "srem":
				if len(parts) < 3 {
					fmt.Println("usage: srem <key> <element>")
					continue
				}
				removed, err := client.SRem(parts[1], strings.Join(parts[2:], " "))
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", removed)
				}
			case "sismember":
				if len(parts) < 3 {
					fmt.Println("usage: sismember <key> <element>")
					continue
				}
				member, err := client.SIsMember(parts[1], strings.Join(parts[2:], " "))
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", boolToInt(member))
				}
			case "smembers":
				if len(parts) < 2 {
					fmt.Println("usage: smembers <key>")
					continue
				}
				elems, err := client.SMembers(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					for i, e := range elems {
						fmt.Printf("%d) %s\n", i+1, e)
					}
				}
			case "scard":
				if len(parts) < 2 {
					fmt.Println("usage: scard <key>")
					continue
				}
				n, err := client.SCard(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", n)
				}
			case "spop":
				if len(parts) < 2 {
					fmt.Println("usage: spop <key>")
					continue
				}
				elem, err := client.SPop(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("%q\n", elem)
				}
			case "srandmember":
				if len(parts) < 2 {
					fmt.Println("usage: srandmember <key> [count]")
					continue
				}
				count := 1
				if len(parts) > 2 {
					fmt.Sscanf(parts[2], "%d", &count)
				}
				elems, err := client.SRandMember(parts[1], count)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					for _, e := range elems {
						fmt.Printf("%q\n", e)
					}
				}
			case "sunion":
				if len(parts) < 2 {
					fmt.Println("usage: sunion <key> [key...]")
					continue
				}
				elems, err := client.SUnion(parts[1:]...)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					for i, e := range elems {
						fmt.Printf("%d) %s\n", i+1, e)
					}
				}
			case "sinter":
				if len(parts) < 2 {
					fmt.Println("usage: sinter <key> [key...]")
					continue
				}
				elems, err := client.SInter(parts[1:]...)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					for i, e := range elems {
						fmt.Printf("%d) %s\n", i+1, e)
					}
				}
			case "sdiff":
				if len(parts) < 2 {
					fmt.Println("usage: sdiff <key> [key...]")
					continue
				}
				elems, err := client.SDiff(parts[1:]...)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					for i, e := range elems {
						fmt.Printf("%d) %s\n", i+1, e)
					}
				}
			// Hash commands
			case "hset":
				if len(parts) < 4 {
					fmt.Println("usage: hset <key> <field> <value>")
					continue
				}
				added, err := client.HSet(parts[1], parts[2], parts[3])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", added)
				}
			case "hget":
				if len(parts) < 3 {
					fmt.Println("usage: hget <key> <field>")
					continue
				}
				v, err := client.HGet(parts[1], parts[2])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("%q\n", v)
				}
			case "hdel":
				if len(parts) < 3 {
					fmt.Println("usage: hdel <key> <field> [field...]")
					continue
				}
				n, err := client.HDel(parts[1], parts[2:]...)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", n)
				}
			case "hexists":
				if len(parts) < 3 {
					fmt.Println("usage: hexists <key> <field>")
					continue
				}
				ok, err := client.HExists(parts[1], parts[2])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", boolToInt(ok))
				}
			case "hgetall":
				if len(parts) < 2 {
					fmt.Println("usage: hgetall <key>")
					continue
				}
				all, err := client.HGetAll(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					for k, v := range all {
						fmt.Printf("  %s: %s\n", k, v)
					}
				}
			case "hkeys":
				if len(parts) < 2 {
					fmt.Println("usage: hkeys <key>")
					continue
				}
				keys, err := client.HKeys(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					for i, k := range keys {
						fmt.Printf("%d) %s\n", i+1, k)
					}
				}
			case "hvals":
				if len(parts) < 2 {
					fmt.Println("usage: hvals <key>")
					continue
				}
				vals, err := client.HVals(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					for i, v := range vals {
						fmt.Printf("%d) %s\n", i+1, v)
					}
				}
			case "hlen":
				if len(parts) < 2 {
					fmt.Println("usage: hlen <key>")
					continue
				}
				n, err := client.HLen(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", n)
				}
			case "hstrlen":
				if len(parts) < 3 {
					fmt.Println("usage: hstrlen <key> <field>")
					continue
				}
				n, err := client.HStrLen(parts[1], parts[2])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", n)
				}
			case "hincrby":
				if len(parts) < 4 {
					fmt.Println("usage: hincrby <key> <field> <delta>")
					continue
				}
				delta, _ := strconv.ParseInt(parts[3], 10, 64)
				n, err := client.HIncrBy(parts[1], parts[2], delta)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", n)
				}
			case "hincrbyfloat":
				if len(parts) < 4 {
					fmt.Println("usage: hincrbyfloat <key> <field> <delta>")
					continue
				}
				delta, _ := strconv.ParseFloat(parts[3], 64)
				n, err := client.HIncrByFloat(parts[1], parts[2], delta)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("%g\n", n)
				}
			case "hmget":
				if len(parts) < 3 {
					fmt.Println("usage: hmget <key> <field> [field...]")
					continue
				}
				vals, err := client.HMGet(parts[1], parts[2:]...)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					for i, v := range vals {
						if v == nil {
							fmt.Printf("%d) (nil)\n", i+1)
						} else {
							fmt.Printf("%d) %v\n", i+1, v)
						}
					}
				}
			case "hmset":
				if len(parts) < 4 {
					fmt.Println("usage: hmset <key> <field> <value> [field value...]")
					continue
				}
				if err := client.HMSet(parts[1], parts[2:]...); err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Println("OK")
				}
			case "hsetnx":
				if len(parts) < 4 {
					fmt.Println("usage: hsetnx <key> <field> <value>")
					continue
				}
				ok, err := client.HSetNX(parts[1], parts[2], parts[3])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", boolToInt(ok))
				}
			// List commands
			case "lpush":
				if len(parts) < 3 {
					fmt.Println("usage: lpush <key> <element> [element...]")
					continue
				}
				n, err := client.LPush(parts[1], parts[2:]...)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", n)
				}
			case "rpush":
				if len(parts) < 3 {
					fmt.Println("usage: rpush <key> <element> [element...]")
					continue
				}
				n, err := client.RPush(parts[1], parts[2:]...)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", n)
				}
			case "lpop":
				if len(parts) < 2 {
					fmt.Println("usage: lpop <key>")
					continue
				}
				elem, err := client.LPop(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("%q\n", elem)
				}
			case "rpop":
				if len(parts) < 2 {
					fmt.Println("usage: rpop <key>")
					continue
				}
				elem, err := client.RPop(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("%q\n", elem)
				}
			case "llen":
				if len(parts) < 2 {
					fmt.Println("usage: llen <key>")
					continue
				}
				n, err := client.LLen(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", n)
				}
			case "lrange":
				if len(parts) < 4 {
					fmt.Println("usage: lrange <key> <start> <stop>")
					continue
				}
				start, _ := strconv.Atoi(parts[2])
				stop, _ := strconv.Atoi(parts[3])
				elems, err := client.LRange(parts[1], start, stop)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					for i, e := range elems {
						fmt.Printf("%d) %s\n", i+1, e)
					}
				}
			case "lindex":
				if len(parts) < 3 {
					fmt.Println("usage: lindex <key> <index>")
					continue
				}
				index, _ := strconv.Atoi(parts[2])
				elem, err := client.LIndex(parts[1], index)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("%q\n", elem)
				}
			case "lset":
				if len(parts) < 4 {
					fmt.Println("usage: lset <key> <index> <value>")
					continue
				}
				index, _ := strconv.Atoi(parts[2])
				if err := client.LSet(parts[1], index, parts[3]); err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Println("OK")
				}
			case "lrem":
				if len(parts) < 4 {
					fmt.Println("usage: lrem <key> <count> <value>")
					continue
				}
				count, _ := strconv.Atoi(parts[2])
				n, err := client.LRem(parts[1], count, parts[3])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", n)
				}
			case "ltrim":
				if len(parts) < 4 {
					fmt.Println("usage: ltrim <key> <start> <stop>")
					continue
				}
				start, _ := strconv.Atoi(parts[2])
				stop, _ := strconv.Atoi(parts[3])
				if err := client.LTrim(parts[1], start, stop); err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Println("OK")
				}
			case "linsert":
				if len(parts) < 5 {
					fmt.Println("usage: linsert <key> before|after <pivot> <value>")
					continue
				}
				before := parts[2] == "before" || parts[2] == "BEFORE"
				n, err := client.LInsert(parts[1], before, parts[3], parts[4])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", n)
				}
			case "blpop":
				if len(parts) < 3 {
					fmt.Println("usage: blpop <key> <timeout_seconds>")
					continue
				}
				timeout, _ := strconv.Atoi(parts[2])
				elem, err := client.BLPop(parts[1], time.Duration(timeout)*time.Second)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("%q\n", elem)
				}
			case "brpop":
				if len(parts) < 3 {
					fmt.Println("usage: brpop <key> <timeout_seconds>")
					continue
				}
				timeout, _ := strconv.Atoi(parts[2])
				elem, err := client.BRPop(parts[1], time.Duration(timeout)*time.Second)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("%q\n", elem)
				}
			case "lpos":
				if len(parts) < 3 {
					fmt.Println("usage: lpos <key> <value> [rank] [count] [maxlen]")
					continue
				}
				rank, count, maxLen := 1, 1, 0
				if len(parts) > 3 {
					rank, _ = strconv.Atoi(parts[3])
				}
				if len(parts) > 4 {
					count, _ = strconv.Atoi(parts[4])
				}
				if len(parts) > 5 {
					maxLen, _ = strconv.Atoi(parts[5])
				}
				positions, err := client.LPos(parts[1], parts[2], rank, count, maxLen)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					for _, p := range positions {
						fmt.Printf("(integer) %d\n", p)
					}
				}
			// Key management
			case "exists":
				if len(parts) < 2 {
					fmt.Println("usage: exists <key>")
					continue
				}
				found, err := client.Exists(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", boolToInt(found))
				}
			case "type":
				if len(parts) < 2 {
					fmt.Println("usage: type <key>")
					continue
				}
				t, err := client.Type(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Println(t)
				}
			case "expire":
				if len(parts) < 3 {
					fmt.Println("usage: expire <key> <seconds>")
					continue
				}
				sec, _ := strconv.ParseInt(parts[2], 10, 64)
				ok, err := client.Expire(parts[1], sec)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", boolToInt(ok))
				}
			case "pexpire":
				if len(parts) < 3 {
					fmt.Println("usage: pexpire <key> <milliseconds>")
					continue
				}
				ms, _ := strconv.ParseInt(parts[2], 10, 64)
				ok, err := client.PExpire(parts[1], ms)
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", boolToInt(ok))
				}
			case "ttl":
				if len(parts) < 2 {
					fmt.Println("usage: ttl <key>")
					continue
				}
				ttl, err := client.TTL(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", ttl)
				}
			case "pttl":
				if len(parts) < 2 {
					fmt.Println("usage: pttl <key>")
					continue
				}
				ttl, err := client.PTTL(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", ttl)
				}
			case "persist":
				if len(parts) < 2 {
					fmt.Println("usage: persist <key>")
					continue
				}
				ok, err := client.Persist(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					fmt.Printf("(integer) %d\n", boolToInt(ok))
				}
			case "keys":
				if len(parts) < 2 {
					fmt.Println("usage: keys <pattern>")
					continue
				}
				keys, err := client.Keys(parts[1])
				if err != nil {
					fmt.Println("Error:", err)
				} else {
					for i, k := range keys {
						fmt.Printf("%d) %s\n", i+1, k)
					}
				}
			default:
				fmt.Printf("Unknown command: %q. Type 'help' for available commands.\n", parts[0])
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(replCmd)
}

func printReplHelp() {
	fmt.Println(`Available REPL commands:
  KV:   get, set, del, len, cleanup, ping, stats
  Set:  sadd, srem, sismember, smembers, scard, spop, srandmember, sunion, sinter, sdiff
  Hash: hset, hget, hdel, hexists, hgetall, hkeys, hvals, hlen, hstrlen, hincrby, hincrbyfloat, hmget, hmset, hsetnx
  List: lpush, rpush, lpop, rpop, blpop, brpop, llen, lrange, lindex, lset, lrem, ltrim, linsert, lpos
  Key:  exists, type, expire, pexpire, ttl, pttl, persist, keys
  help, exit / quit`)
}

func boolToInt(b bool) int {
	if b { return 1 }
	return 0
}
