# Raft 共识

`raft` 子包提供 Raft 共识算法实现，用于多副本强一致性复制。此模块为可选组件。

## 状态机

```
        ┌──────────┐
        │ Follower │◄─────────────────────┐
        └────┬─────┘                      │
             │ election timeout           │ higher term received
             ▼                            │
        ┌──────────┐                      │
        │Candidate │──────────────────────┘
        └────┬─────┘
             │ majority votes received
             ▼
        ┌──────────┐
        │  Leader  │──────────────────────┐
        └──────────┘                      │
             │ higher term received       │
             └────────────────────────────┘
```

三种角色：
- **Follower**：被动接收 Leader 的日志复制和心跳
- **Candidate**：发起选举，请求投票
- **Leader**：处理客户端 Propose，向 Follower 复制日志

## 创建节点

```go
import "github.com/atoncooper/mcache/raft"

cfg := raft.Config{
    NodeID:           1,
    Peers:            []uint64{1, 2, 3},
    ElectionTimeout:  300 * time.Millisecond,
    HeartbeatInterval: 50 * time.Millisecond,
}

transport := raft.NewMemoryTransport(cfg.NodeID, cfg.Peers)
node := raft.NewRaft(cfg, transport)
node.Start()
defer node.Shutdown()
```

## Config

```go
type Config struct {
    NodeID            uint64        // 当前节点 ID（从 1 开始）
    Peers             []uint64      // 集群所有节点 ID
    ElectionTimeout   time.Duration // 选举超时（默认 300ms）
    HeartbeatInterval time.Duration // 心跳间隔（默认 50ms）
}
```

## Transport 接口

```go
type Transport interface {
    SendAppendEntries(peer uint64, req *AppendEntriesRequest) (*AppendEntriesResponse, error)
    SendRequestVote(peer uint64, req *RequestVoteRequest) (*RequestVoteResponse, error)
}
```

内置实现：
- `MemoryTransport`：用于测试的内存传输
- 可扩展为 gRPC / HTTP Transport

## Propose 提交命令

```go
// 仅 Leader 接受 Propose
if node.Propose([]byte("set:key:value")) {
    // 提交成功（已复制到本地日志）
} else {
    // 非 Leader 或队列满，调用方应重试
}
```

`Propose` 是非阻塞的：
- Leader 将命令追加到本地日志
- 立即广播 AppendEntries 到所有 Follower
- 当多数节点确认后，命令被 commit
- 已提交的条目通过 `ApplyCh()` 投递

## 接收已提交日志

```go
for msg := range node.ApplyCh() {
    fmt.Printf("committed: index=%d cmd=%s\n", msg.Index, string(msg.Command))
    // 应用到状态机
}
```

## RPC 消息

### AppendEntries

```go
type AppendEntriesRequest struct {
    Term         uint64
    LeaderID     uint64
    PrevLogIndex uint64
    PrevLogTerm  uint64
    Entries      []LogEntry
    LeaderCommit uint64
}

type AppendEntriesResponse struct {
    Term      uint64
    Success   bool
    LastIndex uint64
}
```

### RequestVote

```go
type RequestVoteRequest struct {
    Term         uint64
    CandidateID  uint64
    LastLogIndex uint64
    LastLogTerm  uint64
}

type RequestVoteResponse struct {
    Term        uint64
    VoteGranted bool
}
```

## 事件循环

所有 Raft 逻辑在单个 `run()` goroutine 中串行执行，通过 channel 投递事件：

```
rpcCh       ← Transport 层投递入站 RPC
proposeCh   ← Propose() 投递命令
electionTimer   ← 选举超时
heartbeatTimer  ← 心跳超时
shutdownCh  ← Shutdown()
applyCh     → 投递已提交消息给应用层
```

## 日志复制流程

```
Client → Leader.Propose(cmd)
  1. Leader 追加到本地 log
  2. Leader 广播 AppendEntries 到所有 Follower
  3. Follower 检查一致性 → 追加 → 返回 Success
  4. Leader 统计 matchIndex → 多数确认 → commitIndex 推进
  5. Leader 在下一个 AppendEntries 中告知 Follower 新的 commitIndex
  6. 各节点 applyCommitted() 投递到 applyCh
```

## 选举流程

```
Election timeout →
  1. 自增 currentTerm
  2. 投票给自己
  3. 广播 RequestVote 到所有 Peer
  4. 收集投票
     - 多数同意 → becomeLeader
     - 收到更高 term → becomeFollower
     - 超时 → 重新选举
```

选举超时使用随机化（`[base, 2*base]`）减少分裂投票。

## 状态查询

```go
node.State()    // Follower / Candidate / Leader
node.Term()     // 当前任期
node.LeaderID() // 已知 Leader ID（0 = 未知）
```

## 使用场景

Raft 模块适用于需要强一致性的场景：
- 缓存节点间复制关键配置
- 分布式锁协调
- 集群成员变更
- 作为独立共识层嵌入应用
