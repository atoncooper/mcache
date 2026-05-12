# Go SDK

SDK 是独立的 Go module（`github.com/atoncooper/mcache/sdk/go`），为 mcache 服务端提供类型安全的客户端。

## 单节点客户端

### 创建客户端

```go
import sdk "github.com/atoncooper/mcache/sdk/go"

client, err := sdk.NewClient("127.0.0.1:11211",
    sdk.WithPoolSize(4),
    sdk.WithDialTimeout(5*time.Second),
    sdk.WithReadTimeout(10*time.Second),
    sdk.WithWriteTimeout(5*time.Second),
    sdk.WithCodec(sdk.RawCodec{}),
)
if err != nil {
    panic(err)
}
defer client.Close()
```

### Option 参考

| Option | 默认值 | 说明 |
|--------|--------|------|
| `WithPoolSize(n)` | `4` | TCP 连接池大小 |
| `WithDialTimeout(d)` | `5s` | 连接拨号超时 |
| `WithReadTimeout(d)` | `10s` | 响应读取超时 |
| `WithWriteTimeout(d)` | `5s` | 请求写入超时 |
| `WithCodec(c)` | `RawCodec{}` | 编解码器（RawCodec / JSONCodec） |
| `WithAddrs(addrs...)` | `nil` | 集群节点地址列表（用于集群配置） |

### CRUD 操作

```go
// 写入（RawCodec 模式，直接传 []byte 或 string）
client.Set("k1", []byte("hello"), time.Hour)
client.Set("k2", "world", time.Minute)

// 读取
var val []byte
err := client.Get("k1", &val)

// 删除
client.Del("k1")

// 条目数
n, err := client.Len()

// 清理过期
removed, err := client.Cleanup()
```

## 连接池

SDK 内部维护连接池，以 round-robin 方式分配请求：

```
Client
 ├── conn[0] → TCP conn (stream 1, 5, 9, ...)
 ├── conn[1] → TCP conn (stream 2, 6, 10, ...)
 ├── conn[2] → TCP conn (stream 3, 7, 11, ...)
 └── conn[3] → TCP conn (stream 4, 8, 12, ...)
```

每条连接独立运行 `readLoop` goroutine，匹配响应的 `StreamID`。

## Codec 系统

### RawCodec（默认）

零拷贝透传，适用场景：key/value 已是 `[]byte` 或 `string`。

```go
client, _ := sdk.NewClient(addr, sdk.WithCodec(sdk.RawCodec{}))
client.Set("k", []byte("raw bytes"), 0)
var val []byte
client.Get("k", &val) // []byte
```

### JSONCodec

自动 JSON 编解码任意 Go 类型：

```go
client, _ := sdk.NewClient(addr, sdk.WithCodec(sdk.JSONCodec{}))

// 写入结构体
client.Set("user:42", map[string]any{"name": "alice"}, time.Hour)

// 读取并反序列化
var data map[string]any
client.Get("user:42", &data)
fmt.Println(data["name"]) // alice
```

### 自定义 Codec

实现 `Codec` 接口即可接入任意序列化格式：

```go
type Codec interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
}

// 例如 Protobuf Codec
type ProtoCodec struct{}

func (c ProtoCodec) Marshal(v any) ([]byte, error) {
    return proto.Marshal(v.(proto.Message))
}

func (c ProtoCodec) Unmarshal(data []byte, v any) error {
    return proto.Unmarshal(data, v.(proto.Message))
}

client, _ := sdk.NewClient(addr, sdk.WithCodec(ProtoCodec{}))
```

## 集群客户端

集群客户端基于一致性哈希（FNV-1a）自动将 key 路由到对应节点。

```go
cluster, err := sdk.NewClusterClient(
    []string{
        "127.0.0.1:11211",
        "127.0.0.1:11212",
        "127.0.0.1:11213",
    },
    sdk.WithPoolSize(4),
)
if err != nil {
    panic(err)
}
defer cluster.Close()

// 用法与单节点客户端完全一致
cluster.Set("user:42", payload, time.Minute)
val, _ := cluster.Get("user:42")
```

### 一致性哈希原理

```
key → FNV-1a → hash % len(nodes) → node
```

- 基于 FNV-1a 哈希取模的简单分片策略
- 每个 key 映射到固定的节点
- 节点增减时所有 key 重新分布（适合节点数量较稳定的场景）

### 集群节点状态查询

```go
stats := cluster.Stats()
for _, ns := range stats {
    if ns.Err != nil {
        fmt.Printf("节点 %s 异常: %v\n", ns.Addr, ns.Err)
        continue
    }
    fmt.Printf("节点 %s 状态: %s\n", ns.Addr, string(ns.Stats))
}
```

`Stats()` 向每个节点发送 `STATS` 命令，返回 `[]NodeStats`：

| 字段 | 类型 | 说明 |
|------|------|------|
| `Addr` | `string` | 节点地址 |
| `Stats` | `[]byte` | 节点返回的原始 JSON 数据 |
| `Err` | `error` | 查询该节点时的错误（如有） |

### 集群使用限制

1. **无自动故障转移**：节点故障时需自行处理重试或切换
2. **集合多键路由**：`SUnion` / `SInter` / `SDiff` 按第一个 key 路由，需确保相关 key 在同一节点
3. **Keys 可能重复**：`Keys()` 聚合所有节点结果，Raft 复制场景下可能出现重复 key
4. **无动态扩缩容**：节点列表在创建时固定，不支持运行时增删节点

## Raft 集群与 ErrNotLeader

当服务端启用 Raft 共识时，写请求必须由 Leader 处理。如果 SDK 连接到 Follower 节点，会收到 `ErrNotLeader` 错误。

```go
err := cluster.Set("key", value, time.Minute)
if errors.Is(err, sdk.ErrNotLeader) {
    // 当前连接的节点不是 Leader，建议：
    // 1. 轮询其他节点
    // 2. 从 Stats() 中识别 Leader（看节点状态）
    // 3. 在应用层实现重定向逻辑
}
```

**建议**：配合 Raft 使用时，在应用层维护 Leader 地址，或对所有节点进行重试。

## 错误处理

```go
var val []byte
err := client.Get("missing-key", &val)
if errors.Is(err, sdk.ErrKeyNotFound) {
    // key 不存在
} else if errors.Is(err, sdk.ErrNotLeader) {
    // Raft 场景：当前节点不是 Leader
} else if err != nil {
    // 网络超时、连接关闭等
}
```

| 错误常量 | 说明 |
|----------|------|
| `ErrKeyNotFound` | key 不存在 |
| `ErrKeyEmpty` | key 为空 |
| `ErrValueNil` | value 为 nil |
| `ErrConnClosed` | 连接已关闭 |
| `ErrTimeout` | 操作超时 |
| `ErrNoNodes` | 集群无可用节点 |
| `ErrNotLeader` | Raft 场景：当前节点不是 Leader（写操作需重定向到 Leader） |

## 连接健康

- `readLoop` 检测到读错误时，标记连接为 bad 并关闭
- 连接池不自动重连，需重建 Client
- 写超时同样标记连接为 bad
