# mcache

高性能 Go 内存缓存服务，支持单机部署和分布式集群。提供 String / Set / Hash / List 四种数据结构，兼容 Redis 风格命令。

[![Go Version](https://img.shields.io/badge/Go-1.24.3+-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

## 目录

- [特性](#特性)
- [快速开始](#快速开始)
- [安装](#安装)
- [运行服务](#运行服务)
- [数据结构](#数据结构)
- [作为 Go 库使用](#作为-go-库使用)
- [SDK 远程调用](#sdk-远程调用)
- [集群部署](#集群部署)
- [可观测性](#可观测性)
- [文档](#文档)
- [测试](#测试)
- [贡献](#贡献)
- [License](#license)

## 特性

- **四种数据结构**：KV / Hash / List / Set，命令集对标 Redis 核心操作
- **高性能**：16 路分片 + 固定 Worker Pool（256 goroutine）+ 多路复用帧协议
- **可嵌入**：`import "github.com/atoncooper/mcache"` 即可作为进程内缓存引擎
- **分布式集群**：支持 Shard（一致性哈希分片）、Sentinel（自动故障切换）、Master-Slave（读写分离）
- **可观测性**：结构化日志（JSON/Text）+ Prometheus 指标 /metrics 端点 + 系统资源监控
- **独立二进制**：编译产物 `bin/mcache` 同时包含服务端和 CLI 客户端，零外部依赖
- **跨平台**：Linux / macOS / Windows
- **MIT 开源**

## 快速开始

```bash
# 编译
go build -o bin/mcache ./cmd/mcache

# 启动服务（使用默认配置）
./bin/mcache server

# 另一个终端操作
./bin/mcache set hello world
./bin/mcache get hello
# → world
```

## 安装

**从源码编译（推荐）**

```bash
git clone https://github.com/atoncooper/mcache.git
cd mcache
go build -o bin/mcache ./cmd/mcache
```

**go install**

```bash
go install github.com/atoncooper/mcache/cmd/mcache@latest
```

**Make**

```bash
make build      # → bin/mcache（当前平台）
make build-all  # → 全平台交叉编译
```

> 需要 Go 1.24.3+。

**Docker（无需安装 Go）**

```bash
docker run -d --name mcache -p 11211:11211 atoncooper/mcache:latest
docker exec mcache mcache ping
# PONG
```

详见 [Docker 部署指南](docs/docker.md)。

## 运行服务

### 1. 配置文件

```bash
cp config.yaml config.local.yaml
```

编辑 `config.local.yaml`：

```yaml
cache:
  shards: 16
  max_size: 10000
  eviction_policy: lru

server:
  address: ":11211"
  workers: 256
  logging:
    level: info
    format: text
    output: stdout
```

### 2. 启动

```bash
./bin/mcache server --config config.local.yaml
```

### 3. 客户端操作

```bash
# KV
./bin/mcache set hello "world"
./bin/mcache get hello               # world
./bin/mcache del hello

# Hash
./bin/mcache hset user:1 name alice
./bin/mcache hget user:1 name        # alice
./bin/mcache hgetall user:1          # name: alice  age: 30

# List
./bin/mcache lpush tasks "write doc"
./bin/mcache rpush tasks "review"
./bin/mcache lrange tasks 0 -1       # write doc / review

# Set
./bin/mcache sadd tags go cache      # 2（批量添加）
./bin/mcache smembers tags           # go / cache
./bin/mcache sunion tags other-tags

# Key 管理
./bin/mcache keys "user:*"
./bin/mcache exists user:1           # 1
./bin/mcache type user:1             # hash
./bin/mcache expire user:1 3600

# 其他
./bin/mcache len                     # 条目数
./bin/mcache ping                    # PONG (0.12ms)
./bin/mcache repl                    # 交互式命令行
```

### 4. 连接远程

```bash
./bin/mcache -a 192.168.1.10:11211 get mykey
./bin/mcache -a 10.0.0.1:11211 -t 5s --pool 8 hset user:1 name alice
```

## 数据结构

mcache 提供四种数据结构，API 对标 Redis 核心命令。

### String

| 命令 | 说明 |
|------|------|
| `set <k> <v> [--ttl]` | 写入键值 |
| `get <k>` | 读取键值 |
| `del <k>` | 删除键 |
| `len` | 总条目数 |
| `cleanup` | 清理过期条目 |

### Hash

| 命令 | 说明 |
|------|------|
| `hset <k> <f> <v>` | 设置字段 |
| `hsetnx <k> <f> <v>` | 仅当字段不存在时设置 |
| `hget <k> <f>` | 获取字段值 |
| `hdel <k> <f> [f...]` | 删除字段 |
| `hexists <k> <f>` | 判断字段存在 |
| `hgetall <k>` | 获取所有字段和值 |
| `hkeys <k>` | 获取所有字段名 |
| `hvals <k>` | 获取所有值 |
| `hlen <k>` | 字段数量 |
| `hstrlen <k> <f>` | 字段值字符串长度 |
| `hincrby <k> <f> <d>` | 整数增量 |
| `hincrbyfloat <k> <f> <d>` | 浮点增量 |
| `hmget <k> <f> [f...]` | 批量获取 |
| `hmset <k> <f> <v> [f v...]` | 批量设置 |

### List

| 命令 | 说明 |
|------|------|
| `lpush <k> <e> [e...]` | 头部插入 |
| `rpush <k> <e> [e...]` | 尾部插入 |
| `lpop <k>` | 头部弹出 |
| `rpop <k>` | 尾部弹出 |
| `blpop <k> <timeout>` | 阻塞头部弹出 |
| `brpop <k> <timeout>` | 阻塞尾部弹出 |
| `llen <k>` | 列表长度 |
| `lrange <k> <start> <stop>` | 范围查询 |
| `lindex <k> <idx>` | 索引查询 |
| `lset <k> <idx> <v>` | 索引设置 |
| `lrem <k> <count> <v>` | 移除元素 |
| `ltrim <k> <start> <stop>` | 范围修剪 |
| `linsert <k> before\|after <p> <v>` | 插入元素 |
| `lpos <k> <v> [rank] [count] [maxlen]` | 查找位置 |

### Set

| 命令 | 说明 |
|------|------|
| `sadd <k> <e> [e...]` | 批量添加元素 |
| `srem <k> <e> [e...]` | 批量移除元素 |
| `sismember <k> <e>` | 成员判断 |
| `smembers <k>` | 列出所有元素 |
| `scard <k>` | 元素数量 |
| `spop <k>` | 随机弹出 |
| `srandmember <k> [count]` | 随机返回 |
| `sunion <k> [k...]` | 并集 |
| `sinter <k> [k...]` | 交集 |
| `sdiff <k> [k...]` | 差集 |

### Key 管理

| 命令 | 说明 |
|------|------|
| `exists <k>` | 键是否存在 |
| `type <k>` | 键类型（string/set/hash/list/none） |
| `expire <k> <sec>` | 设置过期（秒） |
| `pexpire <k> <ms>` | 设置过期（毫秒） |
| `ttl <k>` | 剩余 TTL（秒） |
| `pttl <k>` | 剩余 TTL（毫秒） |
| `persist <k>` | 移除过期时间 |
| `keys <pattern>` | Glob 模式匹配查找键 |

## 作为 Go 库使用

mcache 可以直接嵌入你的 Go 应用作为进程内缓存引擎：

```go
import "github.com/atoncooper/mcache"

c, _ := mcache.New(mcache.NewOptions().
    WithShards(16).
    WithMaxSize(10000).
    WithEvictionPolicy("lru"),
)
defer c.Close()

// KV
c.Set("key", []byte("value"))
val, _ := c.Get("key")

// Hash
c.HSet("user:1", "name", "alice")
name, _ := c.HGet("user:1", "name")

// List
c.LPush("queue", "task1", "task2")
task, _ := c.RPop("queue")

// Set
c.SAdd("tags", "go", "cache")
members, _ := c.SMembers("tags")

// Key 管理
c.Expire("user:1", 3600)
keys, _ := c.Keys("user:*")
```

## SDK 远程调用

通过 Go SDK 连接远程 mcache 服务：

```go
import sdk "github.com/atoncooper/mcache/sdk/go"

client, _ := sdk.NewClient("127.0.0.1:11211",
    sdk.WithPoolSize(8),
    sdk.WithCodec(sdk.JSONCodec{}),
)
defer client.Close()

// KV（支持 JSON codec 自动序列化）
client.Set("user:42", map[string]any{"name": "alice"}, time.Hour)
var user map[string]any
client.Get("user:42", &user)

// Hash / List / Set / Key 管理 — 全部可用
client.HSet("user:1", "name", "alice")
client.LPush("queue", "task1", "task2")
client.SAdd("tags", "go", "cache")
client.Expire("user:1", 3600)
```

**集群模式**：单次请求 `ClusterClient` 在多个节点间做 hash 路由：

```go
cc, _ := sdk.NewClusterClient([]string{
    "10.0.0.1:11211",
    "10.0.0.2:11211",
    "10.0.0.3:11211",
}, sdk.WithPoolSize(4))
defer cc.Close()

cc.Set("k1", "v1", time.Hour)        // → 自动路由到对应节点
keys, _ := cc.Keys("user:*")         // → 聚合所有节点结果
```

Python SDK 同样可用：

```bash
pip install mcache
```

```python
from mcache import Client

c = Client("127.0.0.1:11211")
c.set("k", "v")
print(c.get("k"))           # v
c.hset("h", "f", "val")
c.sadd("s", "a", "b", "c")  # 批量添加
```

## 集群部署

修改配置文件：

```yaml
cluster:
  mode: shard
  nodes:
    - "10.0.0.1:11211"
    - "10.0.0.2:11211"
    - "10.0.0.3:11211"
```

三种模式：

| 模式 | 路由策略 | 故障处理 | 适用场景 |
|------|----------|----------|----------|
| `shard` | 加权哈希分片 | 健康检查 + 节点剔除 | 横向扩容 |
| `sentinel` | 主节点直连 | 自动故障切换（带 debounce） | 高可用 |
| `master_slave` | 主写从读 | 自动故障切换 + 旧主回收 | 读写分离 |

集群客户端会透明路由请求到正确的节点。参见 [集群管理](docs/cluster.md)。

## 可观测性

### 结构化日志

```yaml
server:
  logging:
    level: info       # debug / info / warn / error
    format: json      # text / json
    output: stdout    # stdout / 文件路径（如 logs/mcache.log）
```

每个请求自动记录：

```json
{"level":"info","ts":"2026-05-10T10:00:00Z","msg":"request done","cmd":"get","key":"foo","duration_ms":2,"success":true}
```

### Prometheus 指标

```yaml
server:
  metrics:
    enabled: true
    address: ":9090"
```

启动后访问 `http://localhost:9090/metrics` 获取：

- `mcache_hits_total` / `mcache_misses_total`
- `mcache_sets_total` / `mcache_deletes_total`
- `mcache_evictions_total` / `mcache_rehashes_total`

### 系统监控

```bash
./bin/mcache monitor
```

实时输出 CPU、内存、磁盘 IO、网络流量。

## 文档

| 文档 | 内容 |
|------|------|
| [快速入门](docs/getting-started.md) | 编译安装、交叉编译、版本注入 |
| [Make 构建指南](docs/make-guide.md) | Makefile 全部目标、版本注入、CI/CD 集成 |
| [配置参考](docs/configuration.md) | 完整配置项说明 |
| [CLI 参考](docs/cli.md) | 命令行完整参考 |
| [核心库 API](docs/core-cache.md) | 嵌入应用的 API 文档 |
| [Go SDK](docs/sdk.md) | SDK 客户端与集群客户端 |
| [Python SDK](sdk/python/) | Python 客户端与集群支持 |
| [集群管理](docs/cluster.md) | Shard / Sentinel / Master-Slave 详解 |
| [可观测性](docs/observability.md) | 日志、Prometheus、系统监控 |
| [网络协议](docs/network-protocol.md) | TCP 帧协议与命令编码（73 个命令） |
| [服务端架构](docs/server.md) | Worker Pool + 多路复用设计 |
| [MBR 决策引擎](docs/mbr.md) | PID 自适应调度：迁移 vs 淘汰 |
| [Raft 共识](docs/raft.md) | Raft 共识模块 |
| [数据结构](docs/set.md) | Set / Hash / List 数据结构实现参考 |
| [Docker 部署](docs/docker.md) | 构建、运行、发布到 Docker Hub |
| [扩展接口](docs/extending.md) | 自定义淘汰策略、Rehash 策略和编解码 |

## 测试

```bash
# 运行所有测试
go test ./...

# 带竞态检测（Linux/macOS）
go test -race ./...

# 覆盖率
go test -cover ./...
```

## 贡献

欢迎提交 Issue 和 Pull Request。

## License

[MIT](LICENSE) © 2026 atoncooper
