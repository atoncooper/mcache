# 核心库 API

`mcache` 核心库提供线程安全的分片内存缓存，支持可插拔淘汰策略和动态扩缩容。

## 创建缓存

```go
import "github.com/atoncooper/mcache"

opts := mcache.NewOptions().
    WithShards(16).              // shard 数量，必须是 2 的幂
    WithMaxSize(1000).           // 全局最大条目数（0 = 无限制）
    WithDefaultTTL(time.Hour).   // 默认 TTL（0 = 不过期）
    WithEvictionPolicy("lru").   // 淘汰策略：noop / lru / lfu
    WithRehasher("incremental"). // rehash 策略：incremental / batch / noop
    WithObserver(myObserver)     // 自定义观察者

c, err := mcache.New(opts)
if err != nil {
    panic(err)
}
defer c.Close()
```

## Options 参考

`Options` 采用不可变构建器模式，每个 `With*` 方法返回新实例。

```go
type Options struct {
    shardCount     int           // 分片数（2 的幂）
    maxSize        int           // 最大条目数
    defaultTTL     time.Duration // 默认 TTL
    evictionPolicy string        // 淘汰策略名称
    rehasher       string        // rehash 策略名称
    observer       CacheObserver // 事件观察者
}
```

### 构建器方法

| 方法 | 默认值 | 说明 |
|------|--------|------|
| `NewOptions()` | — | 创建默认配置（16 shards, lru, incremental） |
| `WithShards(n)` | `16` | 分片数，必须为 2 的幂 |
| `WithMaxSize(n)` | `0` | 最大条目数，0 = 无限 |
| `WithDefaultTTL(d)` | `0` | 默认 TTL，0 = 永不过期 |
| `WithEvictionPolicy(s)` | `lru` | 淘汰策略：`noop` / `lru` / `lfu` |
| `WithRehasher(s)` | `incremental` | Rehash 策略：`incremental` / `batch` / `noop` |
| `WithObserver(o)` | `noopObserver` | 自定义 CacheObserver |

## 基本操作

```go
// 写入（可选 TTL，0 使用默认值，<0 永不过期）
c.Set("key", []byte("value"))
c.Set("key", []byte("value"), 5*time.Minute)

// 读取
val, err := c.Get("key")
// val 是新分配的副本，调用方可安全修改

// 删除
c.Del("key")

// 条目数
n := c.Len()

// 清理过期条目
removed := c.Cleanup()

// 关闭（之后所有操作返回 ErrCacheClosed）
c.Close()
```

## 数据结构操作

### Hash

```go
c.HSet("user:1", "name", "alice")        // → 1（新增）
c.HSet("user:1", "name", "bob")          // → 0（更新）
name, _ := c.HGet("user:1", "name")      // → "bob"
c.HExists("user:1", "name")              // → true
c.HLen("user:1")                          // → 1
c.HKeys("user:1")                        // → ["name"]
c.HVals("user:1")                        // → ["bob"]
all, _ := c.HGetAll("user:1")            // → {"name":"bob"}

c.HIncrBy("user:1", "age", 1)            // → 1（不存在从 0 开始）
c.HIncrByFloat("user:1", "score", 1.5)   // → 1.5

c.HMSet("user:2", "a", "1", "b", "2")    // 批量设置
vals, _ := c.HMGet("user:2", "a", "b")   // → ["1", "2"]

c.HDel("user:1", "name", "age")          // 删除字段
c.HSetNX("user:1", "name", "charlie")    // true（不存在时设置）
c.HStrLen("user:1", "name")              // 值长度
```

### List

```go
c.LPush("queue", "task1", "task2")       // → 2（头部插入）
c.RPush("queue", "task3")                // → 3（尾部插入）
task, _ := c.LPop("queue")               // → "task2"
task, _ = c.RPop("queue")                // → "task3"
n, _ := c.LLen("queue")                  // → 1

items, _ := c.LRange("queue", 0, -1)     // → ["task1"]
c.LSet("queue", 0, "urgent")             // 按索引设置
c.LIndex("queue", 0)                     // → "urgent"

c.LRem("queue", 1, "urgent")             // → 1（移除元素）
c.LTrim("queue", 0, 4)                    // 保留前 5 个

// 在 pivot 前后插入
c.LInsert("queue", true, "task3", "task2") // before
c.LInsert("queue", false, "task3", "task4") // after

// 查找位置
pos, _ := c.LPos("queue", "task3", 1, 0, 0) // → [2]
```

### Set

```go
c.SAdd("tags", "go", "cache", "distributed")  // → 3
c.SMembers("tags")                              // → ["go","cache","distributed"]
c.SCard("tags")                                 // → 3
c.SIsMember("tags", "go")                       // → true
c.SPop("tags")                                  // 随机弹出
c.SRandMember("tags", 2)                        // 随机返回 2 个

// 集合运算
c.SUnion("s1", "s2")     // 并集
c.SInter("s1", "s2")     // 交集
c.SDiff("s1", "s2")      // 差集
```

### Key 管理

```go
c.Exists("user:1")        // → true
c.Type("user:1")          // → 3（KeyTypeHash）
c.Expire("user:1", 3600)  // 1 小时后过期
c.TTL("user:1")           // → 3598
c.Persist("user:1")       // 移除过期
c.Keys("user:*")          // → ["user:1","user:2"]
```

## 错误类型

| 错误 | 触发条件 |
|------|----------|
| `ErrKeyNotFound` | Get/HGet/LPop 等不存在的 key |
| `ErrKeyEmpty` | key 为空字符串 |
| `ErrValueNil` | Set value 为 nil |
| `ErrFieldNotFound` | HGet/SAdd 等不存在的 field |
| `ErrIndexOutOfRange` | LIndex/LSet 越界 |
| `ErrInvalidIncr` | HIncrBy 对非数值操作 |
| `ErrNegativeTTL` | TTL 为负数 |
| `ErrCacheClosed` | 缓存已关闭后的操作 |
| `ErrInvalidShards` | shard 数不是 2 的幂 |
| `ErrUnknownPolicy` | 未知淘汰策略名称 |
| `ErrUnknownRehasher` | 未知 rehash 策略名称 |

## TTL 语义

```go
// 使用默认 TTL
c.Set("k1", []byte("v1"))

// 显式 TTL
c.Set("k2", []byte("v2"), time.Hour)

// 永不过期（覆盖默认 TTL）
c.Set("k3", []byte("v3"), -1)
```

- `Set` 不传 TTL → 使用 `Options.defaultTTL`
- `Set` 传 0 → 使用默认 TTL
- `Set` 传正数 → 使用指定 TTL
- `Set` 传负数 → 永不过期

过期条目在 `Get` 时惰性删除，或通过 `Cleanup()` 显式清理。

## 运行时热切换淘汰策略

```go
c.SetEvictionPolicy("lfu")  // 从 LRU 切换到 LFU
c.SetEvictionPolicy("lru")  // 切回 LRU
```

`SetEvictionPolicy` 会替换所有 shard 及旧 shard 表（rehash 期间）的策略，并重新种子现有 key。

## 动态扩缩容（Resize）

```go
// 从 16 shard 扩展到 64 shard
c.Resize(64)

// 在 rehash 期间，每次 Get/Set/Del 执行一个迁移 step
for c.IsRehashing() {
    c.Get("some-key") // 触发 incremental rehash 的一步
}
```

`Resize` 触发 rehash 流程：
1. 保存旧分片表到 `rehasher.Start()`
2. 创建新分片表，更新 mask
3. 每次 `Get`/`Set`/`Del` 调用 `rehasher.Step()` 执行一步迁移
4. `Step` 返回 `justCompleted=true` 时触发 `OnRehashDone()`

## 切换 Rehash 策略

```go
c.SetRehasher("batch")   // 一次性迁移
c.SetRehasher("noop")    // 禁用迁移（测试用）
c.Rehasher()              // 查询当前策略名
```

## 分片架构

```
key → FNV-1a hash → index = hash & mask → Shard[index]
```

- 每个 shard 持有独立的 `map[string]cacheEntry` 和 `EvictionPolicy` 实例
- 全局 `maxSize` 按 shard 数均摊为 `perShardMax`
- shard 内部使用 `sync.RWMutex`，读操作可并发

## 数据不可变性

- `cacheEntry.value` 在存储时执行 `append([]byte(nil), value...)` 拷贝
- `Get` 返回的数据也是独立拷贝
- `Set` 的 value 不会被调用方后续修改影响

## CacheObserver 钩子

详见 [可观测性](observability.md)。

```go
type CacheObserver interface {
    OnHit(key string)
    OnMiss(key string)
    OnSet(key string)
    OnDel(key string)
    OnEvict(key string)
    OnRehashStart(oldShards, newShards int)
    OnRehashDone()
}
```

所有回调在热路径中调用时**不持有全局锁**，实现者需保证自身线程安全。

## 注册自定义淘汰策略

详见 [扩展接口](extending.md#自定义淘汰策略)。

```go
mcache.RegisterPolicy("my-policy", func() mcache.EvictionPolicy {
    return &myPolicy{}
})
```
