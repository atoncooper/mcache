# mcache — Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/atoncooper/mcache/sdk/go.svg)](https://pkg.go.dev/github.com/atoncooper/mcache/sdk/go)
[![Go Report Card](https://goreportcard.com/badge/github.com/atoncooper/mcache/sdk/go)](https://goreportcard.com/report/github.com/atoncooper/mcache/sdk/go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](../../LICENSE)

mcache 的原生 Go 客户端 — 支持单节点、客户端分片集群、TTL、Hash / List / Set 数据结构。

## 安装

```bash
go get github.com/atoncooper/mcache/sdk/go
```

> **导入路径**:`github.com/atoncooper/mcache/sdk/go`
> **包名**:`mcache`

## 快速上手

### 单节点

```go
package main

import (
    "fmt"
    "time"

    mcache "github.com/atoncooper/mcache/sdk/go"
)

func main() {
    c, err := mcache.NewClient("127.0.0.1:7070")
    if err != nil {
        panic(err)
    }
    defer c.Close()

    _ = c.Set("greeting", "hello", 30*time.Second)

    var v string
    _ = c.Get("greeting", &v)
    fmt.Println(v) // hello
}
```

### 集群(客户端分片)

```go
cc, _ := mcache.NewClusterClient([]string{
    "10.0.0.1:7070",
    "10.0.0.2:7070",
    "10.0.0.3:7070",
})
defer cc.Close()

// 同一 API,基于 FNV-1a 哈希自动路由
_ = cc.Set("user:42", "alice", 0)
```

## API 概览

### KV 操作

| 方法 | 说明 |
|------|------|
| `Get(key, dest) error` | 读取并反序列化 |
| `Set(key, value, ttl) error` | 写入(ttl=0 为永久) |
| `Del(key) error` | 删除 |
| `Len() (int, error)` | 总条目数 |
| `Cleanup() (int, error)` | 触发过期清理 |
| `Exists / Type / Keys / TTL / PTTL / Expire / PExpire / Persist` | 键管理 |

### Hash

`HSet / HSetNX / HGet / HDel / HExists / HGetAll / HKeys / HVals / HLen / HStrLen / HIncrBy / HIncrByFloat / HMGet / HMSet`

### List

`LPush / RPush / LPop / RPop / LLen / LRange / LIndex / LSet / LRem / LTrim / LInsert / BLPop / BRPop / LPos`

### Set

`SAdd / SRem / SIsMember / SMembers / SCard / SPop / SRandMember / SUnion / SInter / SDiff`

### 集群专用

| 方法 | 说明 |
|------|------|
| `Stats() []NodeStats` | 查询每个节点的状态(内存、连接数等) |

## 配置选项

```go
c, _ := mcache.NewClient("127.0.0.1:7070",
    mcache.WithPoolSize(16),              // 连接池大小
    mcache.WithDialTimeout(2*time.Second),
    mcache.WithReadTimeout(5*time.Second),
    mcache.WithWriteTimeout(2*time.Second),
    mcache.WithCodec(mcache.JSONCodec{}), // 切换为 JSON 序列化
)
```

## 序列化

内置两种 codec:

- `RawCodec`(默认):仅接受 `[]byte` / `string`
- `JSONCodec`:任意可 JSON 化的 Go 类型

实现自定义 codec:

```go
type Codec interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
}
```

例如使用 msgpack / protobuf,只需实现这两个方法并通过 `WithCodec` 注入。

## 错误处理

所有 sentinel 错误均可被 `errors.Is` 匹配:

```go
import "errors"

err := c.Get("missing", &v)
switch {
case errors.Is(err, mcache.ErrKeyNotFound):
    // 未命中
case errors.Is(err, mcache.ErrTimeout):
    // 超时
case errors.Is(err, mcache.ErrNotLeader):
    // 该节点非 Raft Leader,需重定向
}
```

| 错误 | 含义 |
|------|------|
| `ErrKeyNotFound` | 键不存在 |
| `ErrKeyEmpty` | 键为空字符串 |
| `ErrValueNil` | 值为 nil |
| `ErrConnClosed` | 连接已关闭 |
| `ErrTimeout` | 操作超时 |
| `ErrNoNodes` | 集群无可用节点 |
| `ErrNotLeader` | 写操作发往非 Leader 节点(Raft 模式) |

## 并发

`Client` 与 `ClusterClient` 都是并发安全的,可在多个 goroutine 中共享。底层连接池负责将请求分发到独立的 TCP 连接。

## 版本与兼容性

- Go 1.24+
- 遵循 [Semantic Versioning](https://semver.org/)
- v1.x.x 内 API 向前兼容,Breaking 变更进入 v2

详见 [CHANGELOG.md](./CHANGELOG.md)。

## 文档

- API 文档:[pkg.go.dev](https://pkg.go.dev/github.com/atoncooper/mcache/sdk/go)
- 服务端协议:[docs/protocol.md](../../docs/protocol.md)
- 集群与 Raft:[docs/raft.md](../../docs/raft.md)

## License

MIT — 详见仓库根目录 [LICENSE](../../LICENSE)。
