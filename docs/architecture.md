# 架构设计

## 整体架构

```
┌──────────────────────────────────────────────────────┐
│                     mcache CLI                        │
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────────┐  │
│  │ get  │ │ set  │ │ del  │ │ repl │ │ monitor  │  │
│  └──┬───┘ └──┬───┘ └──┬───┘ └──┬───┘ └────┬─────┘  │
└─────┼────────┼────────┼────────┼───────────┼────────┘
      │        │        │        │           │
      ▼        ▼        ▼        ▼           ▼
┌──────────────────────────────────────────────────────┐
│                 net.Client (连接池)                   │
│  ┌──────────────────────────────────────────────┐    │
│  │        Multiplexed TCP Protocol               │    │
│  │   Frame[StreamID, Type, Payload]              │    │
│  └────────────────────┬─────────────────────────┘    │
└───────────────────────┼──────────────────────────────┘
                        │
                        ▼
┌──────────────────────────────────────────────────────┐
│                net.Server (worker pool)               │
│  ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐           │
│  │Worker1│ │Worker2│ │Worker3│ │ ...   │           │
│  └───┬───┘ └───┬───┘ └───┬───┘ └───┬───┘           │
└──────┼─────────┼─────────┼─────────┼─────────────────┘
       │         │         │         │
       └─────────┴────┬────┴─────────┘
                      ▼
┌──────────────────────────────────────────────────────┐
│                   mcache.Cache                        │
│  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐       │
│  │ Shard0 │ │ Shard1 │ │ Shard2 │ │ Shard3 │       │
│  │ map    │ │ map    │ │ map    │ │ map    │       │
│  │ + LRU  │ │ + LRU  │ │ + LRU  │ │ + LRU  │       │
│  └────────┘ └────────┘ └────────┘ └────────┘       │
│                                                      │
│  ┌──────────────────────────────────────────────┐    │
│  │         Rehasher (incremental/batch)          │    │
│  └──────────────────────────────────────────────┘    │
│                                                      │
│  ┌──────────────────────────────────────────────┐    │
│  │         CacheObserver (Infra)                  │    │
│  │    Logger / Prometheus / Analytics            │    │
│  └──────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────┘
```

## 分片架构

```
key → FNV-1a → hash & mask → shardIndex
```

```
┌─────────────────────────────────────────┐
│                Cache                     │
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐   │
│  │Shard0│ │Shard1│ │Shard2│ │Shard3│   │
│  │      │ │      │ │      │ │      │   │
│  │ map  │ │ map  │ │ map  │ │ map  │   │
│  │ RWMtx│ │ RWMtx│ │ RWMtx│ │ RWMtx│   │
│  │ LRU  │ │ LRU  │ │ LRU  │ │ LRU  │   │
│  └──────┘ └──────┘ └──────┘ └──────┘   │
└─────────────────────────────────────────┘
```

核心设计要点：
- **分片降低锁竞争**：每个 shard 独立 `sync.RWMutex`，高并发下锁竞争仅限于单个 shard
- **FNV-1a 哈希**：快速非加密哈希，分布均匀
- **2 的幂分片数**：`hash & mask` 代替取模运算
- **perShardMax**：全局 `maxSize` 均摊到每个 shard

## 数据不可变性

```
Set(key, value) → copy(value) → cacheEntry
Get(key) → copy(entry.value) → return
```

- 写入时拷贝 value（`append([]byte(nil), value...)`）
- 读取时返回独立拷贝（`append([]byte(nil), entry.value...)`）
- `cacheEntry.touch()` 返回新实例而非修改原值
- 消除共享可变状态引发的并发 bug

## 服务端请求处理

```
Listener.Accept()
    │
    ▼
handleConn (1 goroutine per conn)
    │
    ├── DecodeFrame (read loop)
    │
    ├── DecodeRequestPayload
    │
    ├── jobCh <- job{conn, streamID, request}
    │   └── 队列满 → "server overloaded"
    │
    └── worker (M fixed goroutines)
        │
        ├── cache.Get/Set/Del/Len/Cleanup
        │
        └── writeResponse (持有 conn.writeMu)
```

**关键设计**：连接数（N）与处理 goroutine 数（M）解耦。
- N 可以很大（100K），每个连接只需一个轻量 readLoop
- M 固定（256），避免 goroutine 数量爆炸
- `jobCh` 提供背压：队列满时立即返回错误

## Rehash 流程

```
Resize(newShards) 触发
    │
    ▼
1. rehasher.Start(oldShards, oldMask)
   - 保存旧分片表
    │
    ▼
2. 创建新分片表，更新 mask
    │
    ▼
3. 每次 Get/Set/Del 调用:
   ├── 查询新分片表
   ├── 若 rehash 中 + key 在旧表:
   │   └── 旧表读取 → 写入新表 → 删除旧表
   └── rehasher.Step() 执行一步迁移
       └── justCompleted=true → OnRehashDone()
```

### Rehash 策略对比

| 策略 | 每次迁移量 | 延迟影响 | 适用场景 |
|------|-----------|----------|----------|
| `incremental` | 16 条目 | 极低 | 生产环境 |
| `batch` | 全部条目 | 可能暂停 | 可接受短暂停顿 |
| `noop` | 0 | 无 | 测试 / 禁用 |

## 淘汰策略对比

| 策略 | 算法 | 驱逐依据 | 适用场景 |
|------|------|----------|----------|
| `lru` | 双向链表 + map | 最近最少使用 | 通用场景 |
| `lfu` | 频率计数器 | 最少频繁使用 | 热点数据保护 |
| `noop` | 无操作 | 不驱逐 | maxSize=0 或测试 |

所有操作 O(1)。

## 多路复用协议

```
Connection
  ├── Stream 1: Get("key1")  ──── wait ──── Response
  ├── Stream 2: Set("key2")  ──── wait ──── Response
  └── Stream 3: Get("key3")  ──── wait ──── Response
```

- 单 TCP 连接承载多个并发请求
- 每个请求分配唯一 StreamID
- `pending` map 匹配响应到请求
- 减少连接建立/拆除开销

## 目录结构

```
mcache/
├── cache.go              # Cache 核心逻辑
├── shard.go              # 分片实现
├── entry.go              # 缓存条目（TTL、不可变性）
├── policy.go             # 淘汰策略接口与注册表
├── lru.go                # LRU 实现
├── lfu.go                # LFU 实现
├── observer.go           # CacheObserver 接口
├── options.go            # 配置构建器
├── errors.go             # 错误定义
├── config.go             # YAML 配置加载
├── rehasher_compat.go    # Rehasher 适配
├── cmd/mcache/main.go    # 统一 CLI + Server 入口
├── cli/                  # CLI 命令包（Cobra）
├── net/                  # TCP 协议实现
├── sdk/go/               # 独立 Go SDK module
├── cluster/              # 集群管理子包
├── raft/                 # Raft 共识子包
├── infra/                # 可观测性子包
├── rehash/               # Rehash 策略子包
├── monitor/              # 系统监控子包
└── tests/                # 端到端测试与压测
```
