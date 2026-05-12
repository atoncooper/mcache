# Raft 共识

mcache 内建 Raft 共识层，用于在多节点间强一致复制写操作。该模块为可选组件，通过配置文件开启。

## 架构概览

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Client    │────▶│   Leader    │◄────│   Client    │
└─────────────┘     │  (Node 1)   │     └─────────────┘
                    └──────┬──────┘
                           │ AppendEntries
           ┌───────────────┼───────────────┐
           ▼               ▼               ▼
    ┌────────────┐  ┌────────────┐  ┌────────────┐
    │ Follower   │  │ Follower   │  │ Follower   │
    │  (Node 2)  │  │  (Node 3)  │  │  (Node 4)  │
    └────────────┘  └────────────┘  └────────────┘
```

- **Leader**：接受客户端写请求，将命令序列化为 `RaftCommand` 提交到日志，广播给 Follower
- **Follower**：被动接收并追加日志，应用已提交的命令到本地缓存状态机
- **Candidate**：Leader 故障时发起选举

Raft 层复用独立的 TCP 端口（默认 `:12001`）进行节点间通信，与客户端服务端口（`:11211`）分离。

## 支持的复制命令

启用 Raft 后，以下写操作会自动通过共识日志复制到多数节点：

### KV
| 命令 | 说明 |
|------|------|
| `SET` | 写入键值 |
| `DEL` | 删除键 |
| `CLEANUP` | 清理过期条目 |

### Hash
| 命令 | 说明 |
|------|------|
| `HSET` | 设置字段 |
| `HSETNX` | 仅不存在时设置 |
| `HDEL` | 删除字段 |
| `HINCRBY` | 整数增量 |
| `HINCRBYFLOAT` | 浮点增量 |
| `HMSET` | 批量设置 |

### List
| 命令 | 说明 |
|------|------|
| `LPUSH` | 头部插入 |
| `RPUSH` | 尾部插入 |
| `LPOP` | 头部弹出 |
| `RPOP` | 尾部弹出 |
| `LSET` | 索引设置 |
| `LREM` | 移除元素 |
| `LTRIM` | 范围修剪 |
| `LINSERT` | 插入元素 |

### Set
| 命令 | 说明 |
|------|------|
| `SADD` | 添加元素 |
| `SREM` | 移除元素 |
| `SPOP` | 随机弹出 |

**读操作**（`GET`, `HGET`, `LRANGE`, `SMEMBERS` 等）不经过 Raft，直接查询本地缓存，保证高性能。

## 配置

```yaml
raft:
  enabled: true
  node_id: 1
  bind_addr: ":12001"
  peers:
    - id: 2
      addr: "10.0.0.2:12001"
    - id: 3
      addr: "10.0.0.3:12001"
```

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 是否启用 Raft |
| `node_id` | uint64 | `1` | 当前节点 ID（从 1 开始） |
| `bind_addr` | string | `":12001"` | 节点间通信监听地址 |
| `peers` | []object | `[]` | 其他对等节点 |

每个节点需要独立的 `node_id` 和互不冲突的 `bind_addr`。

## 三节点部署示例

**节点 1（10.0.0.1）**

```yaml
server:
  address: ":11211"
raft:
  enabled: true
  node_id: 1
  bind_addr: ":12001"
  peers:
    - id: 2
      addr: "10.0.0.2:12001"
    - id: 3
      addr: "10.0.0.3:12001"
```

**节点 2（10.0.0.2）**

```yaml
server:
  address: ":11211"
raft:
  enabled: true
  node_id: 2
  bind_addr: ":12001"
  peers:
    - id: 1
      addr: "10.0.0.1:12001"
    - id: 3
      addr: "10.0.0.3:12001"
```

**节点 3（10.0.0.3）**

```yaml
server:
  address: ":11211"
raft:
  enabled: true
  node_id: 3
  bind_addr: ":12001"
  peers:
    - id: 1
      addr: "10.0.0.1:12001"
    - id: 2
      addr: "10.0.0.2:12001"
```

启动三个节点后，它们自动完成 Leader 选举。写请求发送到 Leader 即可自动复制。

## 命令流程

```
Client ──► Server.processJob()
              │
              ▼ (写操作 + Raft 启用)
         raftPropose(RaftCommand)
              │
              ▼
         Leader 追加本地 log
              │
              ▼
         广播 AppendEntries 到 Followers
              │
              ▼
         多数确认 → commitIndex 推进
              │
              ▼
         onRaftApply() 在所有节点执行
              │
              ▼
         修改本地 cache 状态机
              │
              ▼
         Leader 通过 pending channel 通知等待方
              │
              ▼
         构造响应返回给 Client
```

`RaftCommand` 结构体序列化所有写操作所需的字段：

```go
type RaftCommand struct {
    ReqID    uint64   // 请求 ID，用于 Leader 回调匹配
    Op       byte     // 操作码（CmdSet / CmdHSet / CmdLPush ...）
    Key      string   // 键
    Value    []byte   // 值（KV / Hash / List 复用）
    TTL      int64    // 过期时间（毫秒）
    Field    string   // Hash 字段名
    Fields   []string // HDel 字段列表
    FvPairs  []string // HMSet 字段-值对
    DeltaI64 int64    // HIncrBy 增量
    DeltaF64 float64  // HIncrByFloat 增量
    Elems    []string // List Push / Set Add / Set Rem 元素列表
    Index    int64    // LSet / LRem 索引
    Start    int64    // LTrim 起始
    Stop     int64    // LTrim 结束
    Count    int64    // LRem 计数
    Pivot    string   // LInsert 基准元素
    Before   bool     // LInsert 方向
}
```

## TCP Transport

节点间使用自定义 TCP 传输层，基于 length-prefixed 二进制帧：

```
[1 byte: 消息类型][4 bytes: payload 长度][N bytes: JSON payload]
```

消息类型：
- `1` — AppendEntries
- `2` — AppendEntriesResponse
- `3` — RequestVote
- `4` — RequestVoteResponse

特性：
- 节点间维护持久 TCP 连接，断线后自动重连（指数退避）
- 发送 RPC 时优先使用持久连接，不可用时回退到临时连接
- 最大 payload 限制 16 MB

## 客户端行为

### Leader 重定向

非 Leader 节点收到写请求时返回错误：

```
mcache -a 10.0.0.2:11211 set foo bar
# Error: not leader
```

客户端应捕获该错误并重试到其他节点，或先通过 `mcache stats` 查询 Leader 地址（未来版本将支持自动重定向）。

### 读操作

读操作不经过 Raft，可在任意节点执行：

```bash
mcache -a 10.0.0.1:11211 get foo      # OK
mcache -a 10.0.0.2:11211 get foo      # OK（可能略有延迟）
mcache -a 10.0.0.3:11211 get foo      # OK
```

Follower 的读操作可能读到稍旧的数据（最终一致性），若需强一致读，应直接查询 Leader。

## 状态查询

```go
srv.IsRaftLeader()   // 当前节点是否为 Leader
node.State()         // Follower / Candidate / Leader
node.Term()          // 当前任期
node.LeaderID()      // 已知 Leader ID（0 = 未知）
```

## 底层 raft 子包

`raft/` 目录包含独立的 Raft 共识库实现，不依赖 mcache 业务逻辑。其核心 API：

```go
import "github.com/atoncooper/mcache/raft"

cfg := raft.Config{
    NodeID:            1,
    Peers:             []string{"10.0.0.2:12001", "10.0.0.3:12001"},
    ElectionTimeout:   500 * time.Millisecond,
    HeartbeatInterval: 100 * time.Millisecond,
}

node := raft.NewNode(cfg, transport, applyCallback)
node.Start()
```

该子包提供纯 Raft 状态机（Leader 选举、日志复制、快照），Transport 和 Apply 回调由上层（`net` 包）注入。

## 已知限制

- **SPOP 非确定性**：由于 Go map 迭代顺序随机化，各节点对同一 `SPOP` 命令可能弹出不同元素。建议生产环境避免在 Raft 模式下使用 `SPOP`，改用 `SREM` 指定移除已知元素。
- **BLPOP / BRPOP**：阻塞列表弹出尚未通过 Raft 复制，建议在 Raft 集群中使用非阻塞的 `LPOP` / `RPOP`。
