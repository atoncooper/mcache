# 集群管理

`cluster` 包提供统一的 `ClusterManager` 入口，支持三种拓扑模式。

## 模式一览

| 模式 | 路由方式 | 故障处理 | 适用场景 |
|------|----------|----------|----------|
| `shard` | 加权一致性哈希 | 健康检查 + 节点剔除 | 横向扩容 |
| `sentinel` | Sentinel 发现 | 自动故障切换 | 高可用 |
| `master_slave` | 主写从读 | 主节点故障切换 | 读写分离 |

## ClusterManager API

```go
import "github.com/atoncooper/mcache/cluster"

cm, err := cluster.New(/* opts */)
if err != nil {
    panic(err)
}
defer cm.Close()

// CRUD（与单节点接口一致）
cm.Set("key", []byte("value"), time.Minute)
val, _ := cm.Get("key")
cm.Del("key")
n, _ := cm.Len()

// 获取节点健康快照
nodes := cm.Nodes()
for _, n := range nodes {
    fmt.Printf("%s: healthy=%v\n", n.Addr, n.Healthy)
}
```

## Option 参考

```go
cm, _ := cluster.New(
    cluster.WithMode("shard"),
    cluster.WithNodes([]cluster.NodeConfig{
        {Addr: "127.0.0.1:11211", Weight: 1},
        {Addr: "127.0.0.1:11212", Weight: 2},  // 权重 2，分配更多 key
    }),
    cluster.WithHealthCheckInterval(5*time.Second),
    cluster.WithHealthCheckTimeout(2*time.Second),
    cluster.WithFailoverTimeout(10*time.Second),
)
```

| Option | 说明 |
|--------|------|
| `WithMode(m)` | 拓扑模式：`shard` / `sentinel` / `master_slave` |
| `WithNodes(n)` | 节点配置列表（shard/master_slave 模式） |
| `WithSentinels(s)` | Sentinel 地址列表 |
| `WithMaster(addr)` | Master 节点地址（master_slave 模式） |
| `WithSlaves(s)` | Slave 节点地址列表 |
| `WithHealthCheckInterval(d)` | 健康检查间隔 |
| `WithHealthCheckTimeout(d)` | 单次检查超时 |
| `WithFailoverTimeout(d)` | 故障切换超时 |

## Shard 模式

加权一致性哈希分片，支持水平扩容。

```
        ┌───────────────┐
        │ ClusterManager │
        └───────┬───────┘
                │ hash(key)
                ▼
    ┌───────────┼───────────┐
    ▼           ▼           ▼
┌──────┐   ┌──────┐   ┌──────┐
│Node 1│   │Node 2│   │Node 3│
│w=1   │   │w=2   │   │w=2   │
└──────┘   └──────┘   └──────┘
```

- 每个 `NodeConfig` 可指定 `Weight`，权重越大的节点占据哈希环上更多区间
- 健康检查失败时自动从哈希环剔除，恢复后重新加入

## Sentinel 模式

通过外部 Sentinel 服务发现主节点：

```go
cm, _ := cluster.New(
    cluster.WithMode("sentinel"),
    cluster.WithSentinels([]string{
        "127.0.0.1:26379",
        "127.0.0.1:26380",
    }),
)
```

- 启动时向 Sentinel 查询当前主节点地址
- Sentinel 通知主节点变更时自动切换

## Master-Slave 模式

```go
cm, _ := cluster.New(
    cluster.WithMode("master_slave"),
    cluster.WithMaster("127.0.0.1:11211"),
    cluster.WithSlaves([]string{
        "127.0.0.1:11212",
        "127.0.0.1:11213",
    }),
)
```

- 写操作路由到 Master
- 读操作在 Slave 间负载均衡
- Master 不可用时触发故障切换

## NodeConfig

```go
type NodeConfig struct {
    Addr   string  // 节点地址 "host:port"
    Weight int     // 权重（仅 shard 模式），默认 1
}
```

## NodeInfo

```go
type NodeInfo struct {
    Addr    string
    Healthy bool
    // ...
}
```

`Nodes()` 返回当前所有节点的健康状态快照，可用于监控和告警。

## 健康检查

- 周期性对所有节点发送心跳（Ping）
- 超时或失败标记为不健康
- 恢复后自动重新加入集群
- 健康检查在独立 goroutine 中运行，不阻塞业务操作
