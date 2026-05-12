# 可观测性

mcache 提供两层可观测性：`infra` 子包（缓存事件 fan-out）和 `monitor` 子包（系统资源采集）。

## Infra — 缓存事件观察者

`infra.Infra` 实现 `mcache.CacheObserver` 接口，将缓存事件 fan-out 到三个通道：

```
Cache Events (Get/Set/Del/Evict/Rehash)
         │
         ▼
     ┌───────┐
     │ Infra │ (CacheObserver)
     └───┬───┘
         │
    ┌────┼────────┐
    ▼    ▼        ▼
 Logger  Prometheus  Analytics
(stdout) (metrics)   (batch)
```

### 创建 Infra

```go
import (
    "github.com/atoncooper/mcache"
    "github.com/atoncooper/mcache/infra"
    "github.com/prometheus/client_golang/prometheus"
)

inf := infra.New(
    infra.WithLogger(true),
    infra.WithPrometheus(true),
    infra.WithAnalytics(true),
    infra.WithAnalyticsBuffer(1000),
    infra.WithAnalyticsFlushInterval(5*time.Second),
    infra.WithAnalyticsFlushFunc(func(events []infra.Event) error {
        // 批量写入外部系统（Kafka / ClickHouse / S3）
        for _, e := range events {
            fmt.Printf("[analytics] %s %s\n", e.Type, e.Key)
        }
        return nil
    }),
)
defer inf.Stop()

// 注册 Prometheus 指标
inf.RegisterPrometheus(prometheus.DefaultRegisterer)

// 注入到 mcache
opts := mcache.NewOptions().WithObserver(inf)
c, _ := mcache.New(opts)
```

### Option 参考

| Option | 默认值 | 说明 |
|--------|--------|------|
| `WithLogger(bool)` | `false` | 启用结构化日志输出 |
| `WithPrometheus(bool)` | `false` | 启用 Prometheus 指标 |
| `WithAnalytics(bool)` | `false` | 启用批量事件收集 |
| `WithAnalyticsBuffer(n)` | `1000` | Analytics 缓冲区大小 |
| `WithAnalyticsFlushInterval(d)` | `5s` | 自动刷新间隔 |
| `WithAnalyticsFlushFunc(fn)` | `nil` | 自定义刷新处理函数 |

### Logger 通道

- 默认输出到 stdout
- 结构化键值对日志
- `OnHit`/`OnMiss`/`OnSet`/`OnDel` 记录为 info 级别
- `OnEvict` 记录为 warn 级别
- `OnRehashStart`/`OnRehashDone` 记录为 info 级别

```json
{"level":"info","msg":"cache hit","key":"user:42"}
{"level":"warn","msg":"cache evict","key":"old-key"}
{"level":"info","msg":"rehash start","old_shards":16,"new_shards":64}
```

### Prometheus 通道

暴露以下指标（前缀 `mcache_`）：

| 指标 | 类型 | 说明 |
|------|------|------|
| `mcache_hits_total` | Counter | 命中次数 |
| `mcache_misses_total` | Counter | 未命中次数 |
| `mcache_sets_total` | Counter | 写入次数 |
| `mcache_dels_total` | Counter | 删除次数 |
| `mcache_evicts_total` | Counter | 淘汰次数 |
| `mcache_rehash_in_progress` | Gauge | rehash 进行中（0/1） |

### Analytics 通道

- 批量缓冲事件，定期刷新
- `flushFunc` 自定义处理逻辑（写入数据库、发送到消息队列等）
- `FlushAnalytics()` 可手动触发刷新

### Event 结构

```go
type Event struct {
    Type      string    // "hit" / "miss" / "set" / "del" / "evict" / "rehash"
    Key       string
    Timestamp time.Time
}
```

## 系统监控 — Monitor

`monitor` 子包采集系统级资源指标（CPU、内存、IO、网络），提供最近 N 个快照的历史查询，主要用于 MBR 决策引擎。CLI `mcache monitor` 则通过服务端 `ServerStats` API 展示**进程级**资源使用情况。

### 创建 Monitor

```go
import (
    "fmt"
    "github.com/atoncooper/mcache/monitor"
    "time"
)

opts := monitor.NewOptions().
    WithInterval(5 * time.Second).
    WithCapacity(60) // 保留最近 60 个快照

m := monitor.New(opts)
m.Start()
defer m.Stop()

// 获取最新快照
snap, ok := m.Latest()
if ok {
    fmt.Printf("CPU: %.1f%%\n", snap.CPU.UsagePercent)
    fmt.Printf("Memory: %.1f%% (%d/%d MB)\n",
        snap.Memory.UsedPercent,
        snap.Memory.Used/1024/1024,
        snap.Memory.Total/1024/1024,
    )
}

// 获取历史快照
history := m.History()
for _, s := range history {
    fmt.Printf("[%s] CPU: %.1f%%\n", s.Timestamp.Format(time.RFC3339), s.CPU.UsagePercent)
}
```

### Option 参考

| 方法 | 默认值 | 说明 |
|------|--------|------|
| `WithInterval(d)` | `5s` | 采集间隔 |
| `WithCapacity(n)` | `60` | 环形缓冲区容量 |

### 指标结构

```go
type SystemSnapshot struct {
    Timestamp time.Time
    CPU       *CPUMetrics
    Memory    *MemoryMetrics
    IO        []*IOMetrics
    Network   []*NetMetrics
}

type CPUMetrics struct {
    UsagePercent float64
    CoreCount    int
    LoadAvg1     float64
    LoadAvg5     float64
    LoadAvg15    float64
}

type MemoryMetrics struct {
    Total       uint64
    Used        uint64
    Free        uint64
    UsedPercent float64
}

type IOMetrics struct {
    Device         string
    ReadBytes      uint64
    WriteBytes     uint64
    ReadBytesRate  float64 // bytes/s
    WriteBytesRate float64 // bytes/s
}

type NetMetrics struct {
    Interface   string
    BytesSent   uint64
    BytesRecv   uint64
    SendRate    float64 // bytes/s
    RecvRate    float64 // bytes/s
}
```

### Collector 接口

```go
type Collector interface {
    Name() string
    Collect() (*SystemSnapshot, error)
}
```

内置采集器：

| 采集器 | 平台 | 数据源 |
|--------|------|--------|
| `ProcCollector` | Linux | `/proc/stat`, `/proc/meminfo`, `/proc/diskstats`, `/proc/net/dev` |
| `RuntimeCollector` | 跨平台 | `runtime.ReadMemStats()`, `runtime.GOMAXPROCS()` |

非 Linux 平台自动回退到 `RuntimeCollector`。

### 注册自定义采集器

```go
monitor.RegisterCollector("nvidia-smi", func() monitor.Collector {
    return &GPUCollector{}
})
```

### CLI 集成

```bash
# 查看服务端进程资源使用
mcache monitor
mcache monitor --watch 2s
```

输出示例：
```
=== mcache Process Monitor ===
Uptime:      5m32s
Go Version:  go1.24.3 linux/amd64

Memory:
  Heap Used:    30.25 MiB / 500.00 MiB  [====----------------]  6.1%
  Goroutines:   42

Cache:
  Entries:      1250

Network I/O:
  Connections:  12 (peak 25)
  Requests:     15320 total
  Bytes Read:   2.15 MiB
  Bytes Written: 4.82 MiB
  Read Rate:    6.63 KiB/s
  Write Rate:   14.89 KiB/s
  Req Rate:     46.2/s
```

`mcache monitor` 的数据来源是服务端 `Stats()` API，展示进程级 runtime 指标（堆内存、goroutine、连接数、网络 IO），而非系统级 CPU/磁盘。若需系统级监控，请直接使用 `monitor` 子包的编程 API。
