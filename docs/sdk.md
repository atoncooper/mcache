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

## 错误处理

```go
var val []byte
err := client.Get("missing-key", &val)
if err == sdk.ErrKeyNotFound {
    // key 不存在
} else if err != nil {
    // 网络超时、连接关闭等
}
```

SDK 将服务端返回的 `StatusErr` 转换为 Go error，`StatusNotFound` 转换为 `ErrKeyNotFound`。

## 连接健康

- `readLoop` 检测到读错误时，标记连接为 bad 并关闭
- 连接池不自动重连，需重建 Client
- 写超时同样标记连接为 bad
