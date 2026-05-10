# MBR 智能决策引擎

MBR（Multi-dimensional matrix Based intelligent scheduling decision coroutine）是 mcache 的可选智能调度模块。它通过维护一个多维特征矩阵，实时预测系统应该执行**增量迁移（加桶扩容）**还是**信任 LRU 淘汰策略**。

## 架构

```
                         ┌──────────────────┐
                         │  monitor.Monitor │── CPU/Mem/IO/Net
                         └────────┬─────────┘
                                  │
  ┌──────────────────┐           ▼
  │ cache.CacheObserver│──► DefaultStatsProvider ──► FeatureMatrix (环形缓冲区)
  │ (hit/miss/evict)  │           │                        │
  └──────────────────┘           ▼                        ▼
                         ┌──────────────┐        ┌──────────────┐
                         │ PIDController│        │   Decide()   │
                         │ (内存偏差)    │        │ 加权打分卡    │
                         └──────┬───────┘        └──────┬───────┘
                                │                      │
                                ▼               DecisionEvent
                         PIDSetpointDeviation    (仅变化时)
                                │                      │
                                ▼                      ▼
                         WindowStats          ┌──────────────────┐
                                              │ MigrationExecutor│
                                              │ ├─ Resize(N)     │
                                              │ ├─ 压力感知暂停   │
                                              │ └─ 进度反馈      │
                                              └──────────────────┘
```

## 快速启用

### 配置文件

在 `config.yaml` 中添加 MBR 配置段：

```yaml
mbr:
  enabled: true
  matrix_capacity: 60        # 保留最近 60 个窗口
  decision_interval: "500ms" # 决策间隔
  setpoint: 0.60             # PID 目标内存使用率
  pid:
    kp: 1.0
    ki: 0.1
    kd: 0.05
  weights:
    mem_growth: 0.35         # 内存增长权重
    hit_rate: 0.25           # 命中率权重
    new_keys: 0.20           # 新 key 权重
    eviction_pressure: 0.15  # 淘汰压力权重
    buffer_penalty: 0.05     # 缓冲区惩罚权重
  migration:
    check_interval: "100ms"            # 迁移进度检查间隔
    max_migration_time: "5m"           # 最大迁移时长
    pause_on_cpu_threshold: 0.80       # CPU 超此阈值暂停
    pause_on_mem_threshold: 0.85       # 内存超此阈值暂停
    target_load_per_shard: 512         # 每 shard 目标 key 数
    min_shards: 4                      # 最小 shard 数
    max_shards: 1024                   # 最大 shard 数
```

### 启动

```bash
mcache server --config config.yaml
```

服务启动时若 `mbr.enabled: true`，会自动启动 MBR 闭环。

### 编程方式

```go
import (
    "context"
    "github.com/atoncooper/mcache"
    "github.com/atoncooper/mcache/mbr"
    "github.com/atoncooper/mcache/monitor"
)

// 1. 创建组件
c, _ := mcache.New(mcache.NewOptions().WithShards(16))
mon := monitor.New(monitor.NewOptions().WithInterval(5 * time.Second))
mon.Start()

// 2. 创建 MBR 选项
opts := mbr.NewOptions().
    WithMatrixCapacity(60).
    WithDecisionInterval(500 * time.Millisecond).
    WithPID(mbr.PIDConfig{Kp: 1.0, Ki: 0.1, Kd: 0.05, Setpoint: 0.6, Min: -1, Max: 1}).
    WithWeights(mbr.DefaultWeights()).
    WithMigration(mbr.DefaultMigratorConfig())

// 3. 创建组件
pid := mbr.NewPIDController(opts.PID)
matrix := mbr.NewFeatureMatrix(opts.MatrixCapacity)
provider := mbr.NewDefaultStatsProvider(c, mon, pid)
c.SetObserver(provider.Observer())

// 4. 启动决策循环
ctx := context.Background()
decisionCh := make(chan mbr.DecisionEvent, 16)
go mbr.RunDecisionLoop(ctx, provider, matrix, decisionCh)
go mbr.RunMigrationExecutor(ctx, decisionCh, c, mon, provider, opts.Migration)
```

## 特征矩阵

`WindowStats` 包含 16 个特征维度，每次采集形成矩阵的一行：

| 类别 | 字段 | 说明 |
|------|------|------|
| **容量压力** | `MemUsageRatio` | 内存使用率 [0,1] |
| | `MemGrowthRate` | 内存增长率 bytes/s |
| | `KeysGrowthRate` | key 数量增长率 keys/s |
| **缓存效率** | `HitRate` | 命中率 [0,1] |
| | `EvictionsPerSec` | 每秒淘汰数 |
| | `AvgEvictedIdle` | 被淘汰 key 平均空闲时间 |
| **访问模式** | `NewKeysRate` | 新 key 占比 [0,1] |
| | `ReadWriteRatio` | 读写比 |
| **资源压力** | `CPUUtil` | CPU 使用率 [0,1] |
| | `DiskIOPressure` | 磁盘 IO 压力 [0,1] |
| | `NetUtil` | 网络利用率 [0,1] |
| **迁移状态** | `RehashActive` | 是否正在 rehash |
| | `RehashTempMem` | rehash 临时内存 |
| | `MigrationActive` | 迁移执行器是否运行 |
| **缓冲区压力** | `ClientOutputBufferUsage` | 客户端输出缓冲区 |
| | `ReplBacklogUsage` | 复制积压 |
| | `AofRewriteBufferSize` | AOF 重写缓冲区 |
| | `LargeInputBufClients` | 大输入缓冲区客户端数 |
| **策略状态** | `CurrentEvictionPolicy` | 当前淘汰策略枚举 |
| | `PIDSetpointDeviation` | PID 控制偏差 [-1,1] |

`FeatureMatrix` 是线程安全的环形缓冲区，默认容量 60 个窗口（30 秒历史）：

```go
matrix := mbr.NewFeatureMatrix(60)
matrix.Push(stats)

// 获取最近 3 个窗口（连续确认用）
recent := matrix.GetRecent(3)

// 获取最新窗口
last, ok := matrix.Last()
```

## 打分卡规则引擎

### 评分流程

```
Step 1: 各分项得分映射 (阈值查表)
   ├── memGrowthScore:   分段线性 [0, 0.3] → [0, 1]
   ├── hitRateScore:     分段线性 [0.7, 0.95] → [1, 0]  (命中率低 → 分高)
   ├── newKeysScore:     分段线性 [0.1, 0.5] → [0, 1]
   ├── evictionPressureScore: 分段线性 [10, 100] → [0, 1]
   └── bufferPenalty:    缓冲区超阈值 → [-1, 0]

Step 2: 加权求和
   raw = w1*memGrowth + w2*hitRate + w3*newKeys + w4*eviction + w5*buffer
   clamp(raw, 0, 1)

Step 3: 假性压力抑制
   if ClientOutputBuf > 80% or AofBuf > 512MB or LargeInputClients > 0:
       raw *= 0.4

Step 4: 迁移抑制因子
   if RehashActive or MigrationActive:
       final = raw * 0.3
   else:
       final = raw

Step 5: 连续窗口确认
   if final > 0.7 AND 前2个窗口也都 > 0.7:
       return MIGRATE
   else:
       return EVICT
```

### 默认权重

| 权重 | 值 | 含义 |
|------|-----|------|
| `MemGrowth` | 0.35 | 内存增长是最强的扩容信号 |
| `HitRate` | 0.25 | 低命中率说明缓存配置不足 |
| `NewKeys` | 0.20 | 大量新 key 需要更多 shard |
| `EvictionPressure` | 0.15 | 高淘汰率说明 LRU 吃力 |
| `BufferPenalty` | 0.05 | 缓冲区压力是负向信号 |

### 可调权重

通过配置文件或代码调整权重以适应不同负载特征：

```go
weights := mbr.ScoreWeights{
    MemGrowth:        0.40,  // 提高内存权重（内存敏感场景）
    HitRate:          0.20,
    NewKeys:          0.15,
    EvictionPressure: 0.20,  // 提高淘汰权重（读多场景）
    BufferPenalty:    0.05,
}
```

## 假性压力检测

假性压力：内存使用率高是由于缓冲区（client output buffer、AOF rewrite buffer）而非真实热数据增长。

| 检测条件 | 阈值 |
|----------|------|
| `ClientOutputBufferUsage` | > 80% |
| `AofRewriteBufferSize` | > 512 MB |
| `LargeInputBufClients` | > 0 |

任一条件触发 → 总分 × 0.4，避免缓冲区导致的错误迁移决策。

## 增量迁移执行器

### MigrationState 状态机

```
IDLE → RUNNING → PAUSED → RUNNING → COMPLETED
         ↑          │         │
         └──────────┘         │
         (压力解除自动恢复)     │
                              │
                    超时 → forceComplete
```

### 目标 shard 计算

```
target = ceil(currentKeys / TargetLoadPerShard)
target = nextPowerOfTwo(target)
target = clamp(target, MinShards, MaxShards)
```

例如：10000 个 key，每 shard 512 → 20 → 向上取 2 的幂 → 32 shards。

### 压力感知暂停

迁移执行器每 100ms 检查系统压力：

- `CPU > PauseOnCPUThreshold` → 暂停，状态变为 PAUSED
- `Memory > PauseOnMemThreshold` → 暂停，状态变为 PAUSED

压力解除后自动恢复。最大迁移时长超限时调用 `forceComplete` 快速完成。

### 执行流程

```go
executor := mbr.NewMigrationExecutor(cache, mon, cfg)
// Execute 阻塞直到迁移完成或 ctx 取消
err := executor.Execute(ctx)

// 查询进度
progress := executor.Progress()
fmt.Printf("State: %s, Migrated: %d, Remaining: %d\n",
    progress.State, progress.MigratedKeys, progress.RemainingKeys)
```

## PID 控制器

PID 控制器监控内存使用率与目标值的偏差，输出纳入 `WindowStats.PIDSetpointDeviation`：

```go
pid := mbr.NewPIDController(mbr.PIDConfig{
    Kp:       1.0,   // 比例系数
    Ki:       0.1,   // 积分系数
    Kd:       0.05,  // 微分系数
    Setpoint: 0.60,  // 目标内存使用率 60%
    Min:      -1.0,
    Max:      1.0,
})

// 每轮采集时调用
deviation := pid.Compute(currentMemUsage, dt)
```

## 决策闭环

```
Provider (每 500ms)
  ├── cache.Len(), cache.IsRehashing()
  ├── mon.Latest() → CPU/Mem/IO/Net
  ├── observer counters → HitRate, EvictionsPerSec
  └── pid.Compute() → PIDSetpointDeviation
       │
       ▼
  WindowStats → Matrix.Push()
       │
       ▼
  Decide(stats, matrix) → MIGRATE / EVICT
       │
       ▼ (仅决策变化时)
  decisionCh → MigrationExecutor
       │
       ├── Resize(targetShards) → cache (触发渐进 rehash)
       ├── 每 100ms 检查压力 → Pause/Resume
       └── Progress → Provider (RehashActive=true)
              │
              ▼
         下一轮 Decide: score × 0.3 抑制
```

## 运行日志

启用 MBR 后，服务端日志会输出：

```
[INFO] MBR decision engine started matrix_capacity=60 interval=500ms setpoint=0.6
[INFO] MBR decision changed decision=MIGRATE score=0.82 mem_usage=0.85 hit_rate=0.62
[INFO] migration started old_shards=16 new_shards=64 target_keys=32768
[INFO] migration paused reason=system_overload cpu=0.85
[INFO] migration resumed
[INFO] migration completed elapsed_s=12.3
```

## 后续扩展

打分卡引擎可替换为更复杂的模型。只需实现相同的函数签名：

```go
type DecisionEngine interface {
    Decide(stats WindowStats, matrix *FeatureMatrix) (Decision, float64)
}
```

从而实现决策算法的可插拔。
