// Package mcache provides a native Go client for the mcache in-memory cache server.
//
// # Overview
//
// mcache is a high-performance in-memory cache supporting KV, Hash, List, and Set
// data structures with optional TTL, sharded clustering, and Raft consensus for
// write replication. This SDK exposes both a single-node Client and a hash-based
// ClusterClient.
//
// # Installation
//
//	go get github.com/atoncooper/mcache/sdk/go
//
// # Quick Start (single node)
//
//	package main
//
//	import (
//		"fmt"
//		"time"
//
//		mcache "github.com/atoncooper/mcache/sdk/go"
//	)
//
//	func main() {
//		c, err := mcache.NewClient("127.0.0.1:7070")
//		if err != nil {
//			panic(err)
//		}
//		defer c.Close()
//
//		if err := c.Set("hello", "world", 30*time.Second); err != nil {
//			panic(err)
//		}
//
//		var v string
//		if err := c.Get("hello", &v); err != nil {
//			panic(err)
//		}
//		fmt.Println(v) // → world
//	}
//
// # Cluster Mode (client-side hash sharding)
//
//	cc, err := mcache.NewClusterClient([]string{
//		"10.0.0.1:7070",
//		"10.0.0.2:7070",
//		"10.0.0.3:7070",
//	})
//	if err != nil { panic(err) }
//	defer cc.Close()
//
//	// Keys are distributed by FNV-1a hash. Same API as Client.
//	_ = cc.Set("user:42", "alice", 0)
//
// # Codecs
//
// By default values are passed through as raw bytes/strings (RawCodec). To store
// arbitrary Go structures, swap in the built-in JSONCodec or implement your own:
//
//	c, _ := mcache.NewClient("127.0.0.1:7070", mcache.WithCodec(mcache.JSONCodec{}))
//
//	type User struct{ Name string; Age int }
//	_ = c.Set("u:1", User{"alice", 30}, 0)
//
//	var u User
//	_ = c.Get("u:1", &u)
//
// # Error Handling
//
// The SDK exposes typed sentinel errors that can be matched with errors.Is:
//
//	if err := c.Get("missing", &v); errors.Is(err, mcache.ErrKeyNotFound) {
//		// handle miss
//	}
//
// See ErrKeyNotFound, ErrKeyEmpty, ErrValueNil, ErrConnClosed, ErrTimeout,
// ErrNoNodes, and ErrNotLeader.
//
// # Connection Pooling
//
// Each Client maintains a TCP connection pool. Tune via WithPoolSize and the
// timeout options:
//
//	c, _ := mcache.NewClient("127.0.0.1:7070",
//		mcache.WithPoolSize(16),
//		mcache.WithDialTimeout(2*time.Second),
//		mcache.WithReadTimeout(5*time.Second),
//		mcache.WithWriteTimeout(2*time.Second),
//	)
//
// # Concurrency
//
// Client and ClusterClient are safe for concurrent use by multiple goroutines.
// The underlying connection pool serialises individual requests per connection.
package mcache
