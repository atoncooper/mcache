# 扩展接口

mcache 提供多个可插拔扩展点，支持运行时注册和热切换。

## 自定义淘汰策略

实现 `EvictionPolicy` 接口并通过 `RegisterPolicy` 注册：

```go
type EvictionPolicy interface {
    Name() string
    OnAccess(key string)       // key 被读取时调用
    OnAdd(key string)          // key 被添加时调用
    OnRemove(key string)       // key 被删除时调用
    Evict() (string, bool)     // 选择要淘汰的 key
    Len() int                  // 当前追踪的 key 数量
    Clear()                    // 清空所有追踪
}
```

### 示例：FIFO 策略

```go
package main

import (
    "container/list"
    "sync"
    "github.com/atoncooper/mcache"
)

type fifoPolicy struct {
    mu    sync.Mutex
    order *list.List
    idx   map[string]*list.Element
}

func newFIFO() *fifoPolicy {
    return &fifoPolicy{
        order: list.New(),
        idx:   make(map[string]*list.Element),
    }
}

func (p *fifoPolicy) Name() string { return "fifo" }

func (p *fifoPolicy) OnAccess(_ string) {} // FIFO 不更新访问顺序

func (p *fifoPolicy) OnAdd(key string) {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.idx[key] = p.order.PushBack(key)
}

func (p *fifoPolicy) OnRemove(key string) {
    p.mu.Lock()
    defer p.mu.Unlock()
    if el, ok := p.idx[key]; ok {
        p.order.Remove(el)
        delete(p.idx, key)
    }
}

func (p *fifoPolicy) Evict() (string, bool) {
    p.mu.Lock()
    defer p.mu.Unlock()
    if p.order.Len() == 0 {
        return "", false
    }
    el := p.order.Front()
    key := el.Value.(string)
    p.order.Remove(el)
    delete(p.idx, key)
    return key, true
}

func (p *fifoPolicy) Len() int {
    p.mu.Lock()
    defer p.mu.Unlock()
    return p.order.Len()
}

func (p *fifoPolicy) Clear() {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.order.Init()
    p.idx = make(map[string]*list.Element)
}

func init() {
    // 注册到全局注册表
    mcache.RegisterPolicy("fifo", func() mcache.EvictionPolicy {
        return newFIFO()
    })
}
```

使用：

```go
opts := mcache.NewOptions().WithEvictionPolicy("fifo")
c, _ := mcache.New(opts)

// 或运行时热切换
c.SetEvictionPolicy("fifo")
```

### 线程安全要求

`EvictionPolicy` 的实现必须线程安全。shard 内部持有 `sync.RWMutex`，`OnAccess` 在 RLock 下调用，`OnAdd`/`OnRemove`/`Evict` 在 Lock 下调用，但策略内部的 `Len()` 可能在单独路径被调用。

**推荐做法**：策略内部使用独立的 `sync.Mutex`，不依赖调用方的锁。

## 自定义 Rehash 策略

实现 `rehash.Rehasher` 接口并通过 `rehash.Register` 注册：

```go
type Rehasher interface {
    Name() string
    Start(oldShards []Shard, oldMask uint32)
    Step(newShards []Shard, indexFunc func(key string) uint32) (done bool, justCompleted bool)
    IsRehashing() bool
    OldShard(key string) Shard
    OldShards() []Shard
    Stop()
}
```

### 示例：固定批量 Rehasher

```go
package main

import "github.com/atoncooper/mcache/rehash"

type fixedBatchRehasher struct {
    oldShards []rehash.Shard
    oldMask   uint32
    active    bool
    batchSize int
    cursor    int
}

func newFixedBatch(batchSize int) *fixedBatchRehasher {
    return &fixedBatchRehasher{batchSize: batchSize}
}

func (r *fixedBatchRehasher) Name() string { return "fixed-batch" }

func (r *fixedBatchRehasher) Start(oldShards []rehash.Shard, oldMask uint32) {
    r.oldShards = oldShards
    r.oldMask = oldMask
    r.active = true
    r.cursor = 0
}

func (r *fixedBatchRehasher) Step(newShards []rehash.Shard, indexFunc func(key string) uint32) (bool, bool) {
    if !r.active {
        return true, false
    }
    migrated := 0
    for r.cursor < len(r.oldShards) && migrated < r.batchSize {
        items := r.oldShards[r.cursor].ExtractN(r.batchSize - migrated)
        for _, item := range items {
            idx := indexFunc(item.Key)
            newShards[idx].Set(item.Key, item.Value, item.TTL)
            migrated++
        }
        if r.oldShards[r.cursor].Len() == 0 {
            r.cursor++
        }
    }
    if r.cursor >= len(r.oldShards) {
        r.Stop()
        return true, true
    }
    return false, false
}

func (r *fixedBatchRehasher) IsRehashing() bool     { return r.active }
func (r *fixedBatchRehasher) OldShard(key string) rehash.Shard {
    if !r.active { return nil }
    return r.oldShards[rehash.OldShardIndex(key, r.oldMask)]
}
func (r *fixedBatchRehasher) OldShards() []rehash.Shard { return r.oldShards }
func (r *fixedBatchRehasher) Stop()                     { r.active = false; r.oldShards = nil }

func init() {
    rehash.Register("fixed-batch", func() rehash.Rehasher {
        return newFixedBatch(32)
    })
}
```

使用：

```go
opts := mcache.NewOptions().WithRehasher("fixed-batch")
c, _ := mcache.New(opts)

// 运行时切换
c.SetRehasher("fixed-batch")
```

### Rehasher 契约

- `Start` 在 `Resize` 时调用（持有写锁）
- `Step` 在每次 `Get`/`Set`/`Del` 中调用（持有读锁）
  - `done=true` 表示迁移完成
  - `justCompleted=true` 仅在此次调用导致完成时为 true（触发 `OnRehashDone`）
- `OldShard` 用于在旧分片表中查找 key（rehash 期间的读写双查）
- `Stop` 用于中止 rehash（`SetRehasher` 或 `Close` 时调用）

## 自定义 Codec（SDK）

实现 `Codec` 接口注入到 SDK 客户端：

```go
type Codec interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
}
```

### 示例：MessagePack Codec

```go
import "github.com/vmihailenco/msgpack/v5"

type MessagePackCodec struct{}

func (c MessagePackCodec) Marshal(v any) ([]byte, error) {
    return msgpack.Marshal(v)
}

func (c MessagePackCodec) Unmarshal(data []byte, v any) error {
    return msgpack.Unmarshal(data, v)
}

// 使用
client, _ := sdk.NewClient(addr, sdk.WithCodec(MessagePackCodec{}))
```

## 自定义 Monitor Collector

实现 `monitor.Collector` 接口并通过 `monitor.RegisterCollector` 注册：

```go
type Collector interface {
    Name() string
    Collect() (*SystemSnapshot, error)
}
```

### 示例：GPU 采集器

```go
type GPUCollector struct{}

func (c *GPUCollector) Name() string { return "gpu" }

func (c *GPUCollector) Collect() (*monitor.SystemSnapshot, error) {
    // 调用 nvidia-smi 或 NVML 库
    return &monitor.SystemSnapshot{
        CPU: &monitor.CPUMetrics{
            UsagePercent: gpuUtilization,
        },
    }, nil
}

func init() {
    monitor.RegisterCollector("gpu", func() monitor.Collector {
        return &GPUCollector{}
    })
}
```

## 自定义 CacheObserver

实现 `mcache.CacheObserver` 接口直接注入：

```go
type MyObserver struct {
    hits   atomic.Int64
    misses atomic.Int64
}

func (o *MyObserver) OnHit(key string)                      { o.hits.Add(1) }
func (o *MyObserver) OnMiss(key string)                     { o.misses.Add(1) }
func (o *MyObserver) OnSet(key string)                      {}
func (o *MyObserver) OnDel(key string)                      {}
func (o *MyObserver) OnEvict(key string)                    {}
func (o *MyObserver) OnRehashStart(oldShards, newShards int) {}
func (o *MyObserver) OnRehashDone()                          {}

// 注入
opts := mcache.NewOptions().WithObserver(&MyObserver{})
c, _ := mcache.New(opts)
```

## 扩展点总结

| 扩展类型 | 接口 | 注册方法 | 热切换 |
|----------|------|----------|--------|
| 淘汰策略 | `EvictionPolicy` | `RegisterPolicy` | `SetEvictionPolicy` |
| Rehash 策略 | `rehash.Rehasher` | `rehash.Register` | `SetRehasher` |
| Codec | `Codec` | — (注入) | — |
| Monitor Collector | `monitor.Collector` | `monitor.RegisterCollector` | — |
| CacheObserver | `CacheObserver` | — (注入) | — |
