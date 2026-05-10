# 数据结构 (Set / Hash / List)

mcache 支持三种去重/聚合数据结构，与 String KV 共存于同一 shard 中。所有数据结构均线程安全。

## 数据类型概览

```
key → byte[] value           ← String KV
key → {a, b, c, d}           ← Set 去重集合
key → {field1: v1, f2: v2}   ← Hash 哈希表
key → [a, b, c, d]           ← List 双向链表
```

同一 key 只能是一种类型。

---

## Set（去重集合）

### CLI 命令

| 命令 | 说明 |
|------|------|
| `sadd <k> <e>` | 添加元素，返回新增数 |
| `srem <k> <e>` | 删除元素，返回删除数 |
| `sismember <k> <e>` | 判断成员（1/0） |
| `smembers <k>` | 列出所有元素 |
| `scard <k>` | 集合大小 |
| `spop <k>` | 随机弹出 |
| `srandmember <k> [cnt]` | 随机返回 |
| `sunion <k> [k...]` | 并集 |
| `sinter <k> [k...]` | 交集 |
| `sdiff <k> [k...]` | 差集 |

### 使用示例

```bash
mcache sadd users alice bob charlie  # 3
mcache sismember users alice         # 1
mcache smembers users                # alice / bob / charlie
mcache scard users                   # 3
mcache spop users                    # "bob"（随机）

# 集合运算
mcache sadd s1 a b c
mcache sadd s2 b c d
mcache sunion s1 s2                  # a b c d
mcache sinter s1 s2                  # b c
mcache sdiff  s1 s2                  # a
```

### Go 代码

```go
import "github.com/atoncooper/mcache"

c, _ := mcache.New(mcache.NewOptions().WithShards(16))
defer c.Close()

c.SAdd("myset", "a", "b", "c")
c.SMembers("myset")           // ["a", "b", "c"]
c.SCard("myset")              // 3
c.SIsMember("myset", "a")     // true
c.SPop("myset")               // random element

// 集合运算
c.SAdd("s1", "a", "b", "c")
c.SAdd("s2", "b", "c", "d")
c.SUnion("s1", "s2")          // [a b c d]
c.SInter("s1", "s2")          // [b c]
c.SDiff("s1", "s2")           // [a]
```

### 协议命令码

| 编码 | 命令 |
|------|------|
| 10 | SAdd |
| 11 | SRem |
| 12 | SIsMember |
| 13 | SMembers |
| 14 | SCard |
| 15 | SPop |
| 16 | SRandMember |
| 17 | SUnion |
| 18 | SInter |
| 19 | SDiff |

---

## Hash（哈希表）

### CLI 命令

| 命令 | 说明 |
|------|------|
| `hset <k> <f> <v>` | 设置字段，返回 1 新增 / 0 更新 |
| `hsetnx <k> <f> <v>` | 仅当字段不存在时设置 |
| `hget <k> <f>` | 获取字段值 |
| `hdel <k> <f> [f...]` | 删除字段 |
| `hexists <k> <f>` | 判断字段是否存在 |
| `hgetall <k>` | 获取所有字段-值对 |
| `hkeys <k>` | 获取所有字段名 |
| `hvals <k>` | 获取所有值 |
| `hlen <k>` | 字段数量 |
| `hstrlen <k> <f>` | 字段值长度 |
| `hincrby <k> <f> <d>` | 整数增量 |
| `hincrbyfloat <k> <f> <d>` | 浮点增量 |
| `hmget <k> <f> [f...]` | 批量获取 |
| `hmset <k> <f> <v> [f v...]` | 批量设置 |

### 使用示例

```bash
mcache hset user:1 name "alice"      # 1
mcache hset user:1 age "30"          # 1
mcache hget user:1 name              # alice
mcache hgetall user:1                # name: alice  age: 30
mcache hkeys user:1                  # name / age
mcache hlen user:1                   # 2

mcache hincrby user:1 age 1          # 31
mcache hincrbyfloat user:1 score 1.5 # 1.5

mcache hmget user:1 name age         # alice / 30
mcache hmset user:2 a 1 b 2 c 3       # OK

mcache hexists user:1 name           # 1
mcache hstrlen user:1 name           # 5
mcache hdel user:1 age               # 1
mcache hsetnx user:1 name "bob"      # 0（已存在）
```

### Go 代码

```go
c.HSet("user:1", "name", "alice")
c.HSet("user:1", "age", "30")
name, _ := c.HGet("user:1", "name")   // "alice"
all, _ := c.HGetAll("user:1")         // {"name":"alice","age":"30"}
n, _ := c.HLen("user:1")              // 2

// 批量操作
c.HMSet("user:2", "a", "1", "b", "2")
vals, _ := c.HMGet("user:2", "a", "b") // ["1","2"]

// 自增
c.HIncrBy("user:1", "age", 1)         // 31
c.HIncrByFloat("user:1", "score", 1.5) // 1.5
```

### 协议命令码

| 编码 | 命令 |
|------|------|
| 32 | HSet |
| 33 | HGet |
| 34 | HDel |
| 35 | HExists |
| 36 | HGetAll |
| 37 | HKeys |
| 38 | HVals |
| 39 | HLen |
| 40 | HStrLen |
| 41 | HIncrBy |
| 42 | HIncrByFloat |
| 43 | HMGet |
| 44 | HMSet |
| 45 | HSetNX |

---

## List（双向链表）

### CLI 命令

| 命令 | 说明 |
|------|------|
| `lpush <k> <e> [e...]` | 头部插入 |
| `rpush <k> <e> [e...]` | 尾部插入 |
| `lpop <k>` | 头部弹出 |
| `rpop <k>` | 尾部弹出 |
| `blpop <k> <timeout>` | 阻塞头部弹出（秒） |
| `brpop <k> <timeout>` | 阻塞尾部弹出（秒） |
| `llen <k>` | 列表长度 |
| `lrange <k> <start> <stop>` | 范围查询（负索引从尾部计数） |
| `lindex <k> <idx>` | 按索引取值 |
| `lset <k> <idx> <v>` | 按索引设置 |
| `lrem <k> <cnt> <v>` | 移除元素（cnt>0 从头，cnt<0 从尾，cnt=0 全部） |
| `ltrim <k> <start> <stop>` | 范围修剪 |
| `linsert <k> before\|after <p> <v>` | 在 pivot 前后插入 |
| `lpos <k> <v> [rank] [cnt] [maxlen]` | 查找元素位置 |

### 使用示例

```bash
mcache lpush tasks "review"          # 1
mcache rpush tasks "deploy" "test"   # 3
mcache lrange tasks 0 -1             # review / deploy / test
mcache lpop tasks                     # "review"
mcache rpop tasks                     # "test"
mcache llen tasks                     # 1

mcache lset tasks 0 "code-review"     # OK
mcache lindex tasks 0                 # "code-review"

mcache lrem tasks 1 "code-review"     # 1
mcache linsert tasks before deploy verify  # 在 deploy 前插入 verify

# 阻塞弹出（超时 5 秒）
mcache blpop queue 5
```

### Go 代码

```go
c.LPush("queue", "task1", "task2")
c.RPush("queue", "task3")
task, _ := c.LPop("queue")            // "task2"
task, _ = c.RPop("queue")             // "task3"
n, _ := c.LLen("queue")               // 1

items, _ := c.LRange("queue", 0, -1)  // ["task1"]
c.LSet("queue", 0, "urgent-task")
c.LRem("queue", 1, "urgent-task")
c.LTrim("queue", 0, 4)

pos, _ := c.LPos("queue", "task1", 1, 0, 0) // 查找所有 task1 的位置
```

### 协议命令码

| 编码 | 命令 |
|------|------|
| 48 | LPush |
| 49 | RPush |
| 50 | LPop |
| 51 | RPop |
| 52 | LLen |
| 53 | LRange |
| 54 | LIndex |
| 55 | LSet |
| 56 | LRem |
| 57 | LTrim |
| 58 | LInsert |
| 59 | BLPop |
| 60 | BRPop |
| 61 | LPos |

---

## Key 管理

| 命令 | 说明 |
|------|------|
| `exists <k>` | 键是否存在 |
| `type <k>` | 键类型 |
| `expire <k> <sec>` | 设置过期（秒） |
| `pexpire <k> <ms>` | 设置过期（毫秒） |
| `ttl <k>` | 剩余秒数 |
| `pttl <k>` | 剩余毫秒数 |
| `persist <k>` | 移除过期 |
| `keys <pattern>` | 模式匹配（glob） |

```bash
mcache exists user:1         # 1
mcache type user:1           # hash
mcache expire user:1 3600   # 设置 1 小时后过期
mcache ttl user:1            # 3598
mcache persist user:1        # 移除过期
mcache keys "user:*"        # user:1 / user:2 / ...
```

## 线程安全

每种数据结构内部使用 `sync.RWMutex`（或 `sync.Mutex`），读写并发安全。Shard 级别的锁保护 typed maps（sets/hashes/lists）。

## 实现架构

```
Cache
 ├── Shard 0
 │   ├── entries: map[string]cacheEntry  ← KV
 │   ├── sets:    map[string]*set.Set    ← Set
 │   ├── hashes:  map[string]*hash.Hash  ← Hash
 │   └── lists:   map[string]*list.List  ← List
 ├── Shard 1
 │   └── ...
 └── ...
```

源码目录：

```
ds/
├── set/set.go + set_test.go + ops.go
├── hash/hash.go + hash_test.go
└── list/list.go + list_test.go
```

所有 key 通过 FNV-1a 哈希确定 shard，四种类型共享同一分片策略。
