# 配置参考

服务端通过 `config.yaml` 配置，由 `mcache.LoadConfig(path)` 加载。

## 完整示例

```yaml
cache:
  shards: 16
  max_size: 10000
  default_ttl: "1h"
  eviction_policy: lru
  rehasher: incremental
  cleanup_interval: "10m"
  observer_enabled: true

server:
  address: ":11211"
  workers: 256
  max_conns: 100000
  read_timeout: "30s"
  write_timeout: "5s"
  graceful_shutdown_timeout: "30s"
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
  auth:
    enabled: false
    token: ""
  logging:
    level: info
    format: text
    output: stdout
  metrics:
    enabled: false
    address: ":9090"

monitor:
  enabled: false
  interval: "5s"
  capacity: 60

cluster:
  mode: standalone
  nodes:
    - "127.0.0.1:11211"
    - "127.0.0.1:11212"
  replication_factor: 1

client:
  pool_size: 4
  dial_timeout: "5s"
  read_timeout: "10s"
  write_timeout: "5s"

mbr:
  enabled: false
  matrix_capacity: 60
  decision_interval: "500ms"
  setpoint: 0.60
  pid:
    kp: 1.0
    ki: 0.1
    kd: 0.05
  weights:
    mem_growth: 0.35
    hit_rate: 0.25
    new_keys: 0.20
    eviction_pressure: 0.15
    buffer_penalty: 0.05
  migration:
    check_interval: "100ms"
    max_migration_time: "5m"
    pause_on_cpu_threshold: 0.80
    pause_on_mem_threshold: 0.85
    target_load_per_shard: 512
    min_shards: 4
    max_shards: 1024
```

## cache — 缓存核心配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `shards` | int | `16` | 分片数，必须是 2 的幂 |
| `max_size` | int | `10000` | 全局最大条目数，0 = 无限制 |
| `default_ttl` | string | `""` | 默认 TTL，支持 `s`/`m`/`h` 后缀，空 = 不过期 |
| `eviction_policy` | string | `lru` | 淘汰策略：`noop` / `lru` / `lfu` |
| `rehasher` | string | `incremental` | Rehash 策略：`incremental` / `batch` / `noop` |
| `cleanup_interval` | string | `""` | 定期清理过期条目间隔，空 = 不自动清理 |
| `observer_enabled` | bool | `true` | 是否启用 infra 可观测性 |

## server — TCP 服务端配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `address` | string | `:11211` | 监听地址 |
| `workers` | int | `256` | 固定 worker goroutine 数 |
| `max_conns` | int | `100000` | 最大并发连接数 |
| `read_timeout` | string | `30s` | 帧读取超时 |
| `write_timeout` | string | `5s` | 响应写入超时 |
| `graceful_shutdown_timeout` | string | `30s` | 优雅关闭等待时间 |

### server.tls

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 是否启用 TLS（预留） |
| `cert_file` | string | `""` | 证书文件路径 |
| `key_file` | string | `""` | 私钥文件路径 |

### server.auth

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 是否启用认证（预留） |
| `token` | string | `""` | 认证令牌 |

### server.logging

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `level` | string | `info` | 日志级别：`debug` / `info` / `warn` / `error` |
| `format` | string | `text` | 日志格式：`text` / `json` |
| `output` | string | `stdout` | 输出目标：`stdout` / `file` 或文件路径 |

### server.metrics

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 是否启用 Prometheus 指标（预留） |
| `address` | string | `:9090` | Metrics HTTP 监听地址 |

## monitor — 系统监控配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 是否启用系统资源监控 |
| `interval` | string | `5s` | 采集间隔 |
| `capacity` | int | `60` | 环形缓冲区容量（保留最近 N 个快照） |

## cluster — 集群配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `mode` | string | `standalone` | 集群模式：`standalone` / `shard` / `sentinel` / `master_slave` |
| `nodes` | []string | `[]` | 集群节点地址列表 |
| `replication_factor` | int | `1` | 复制因子 |

## raft — Raft 共识配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 是否启用 Raft 多节点复制 |
| `node_id` | uint64 | `1` | 当前节点唯一标识（从 1 开始递增） |
| `bind_addr` | string | `":12001"` | Raft 节点间 TCP 通信监听地址 |
| `peers` | []object | `[]` | 其他对等节点列表（不含自身） |

### raft.peers

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | uint64 | 对等节点 ID |
| `addr` | string | 对等节点通信地址（host:port） |

**三节点示例：**

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

启用后，所有写操作（KV / Hash / List / Set）自动通过 Raft 日志复制到多数节点，读操作仍直接查询本地缓存。非 Leader 节点收到写请求会返回 `not leader` 错误，客户端应重定向到 Leader。

## client — SDK 客户端配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `pool_size` | int | `4` | 连接池大小 |
| `dial_timeout` | string | `5s` | 拨号超时 |
| `read_timeout` | string | `10s` | 读取超时 |
| `write_timeout` | string | `5s` | 写入超时 |

## mbr — MBR 智能决策引擎

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 是否启用 MBR |
| `matrix_capacity` | int | `60` | 特征矩阵窗口数 |
| `decision_interval` | string | `500ms` | 决策循环间隔 |
| `setpoint` | float64 | `0.60` | PID 目标内存使用率 |

### mbr.pid

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `kp` | float64 | `1.0` | PID 比例系数 |
| `ki` | float64 | `0.1` | PID 积分系数 |
| `kd` | float64 | `0.05` | PID 微分系数 |

### mbr.weights

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `mem_growth` | float64 | `0.35` | 内存增长权重 |
| `hit_rate` | float64 | `0.25` | 命中率权重（低命中率 → 倾向迁移） |
| `new_keys` | float64 | `0.20` | 新 key 权重 |
| `eviction_pressure` | float64 | `0.15` | 淘汰压力权重 |
| `buffer_penalty` | float64 | `0.05` | 缓冲区惩罚权重 |

### mbr.migration

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `check_interval` | string | `100ms` | 迁移进度检查间隔 |
| `max_migration_time` | string | `5m` | 最大迁移时长（超时强制完成） |
| `pause_on_cpu_threshold` | float64 | `0.80` | CPU 超此阈值暂停迁移 |
| `pause_on_mem_threshold` | float64 | `0.85` | 内存超此阈值暂停迁移 |
| `target_load_per_shard` | int | `512` | 每 shard 目标 key 数 |
| `min_shards` | int | `4` | 最小 shard 数 |
| `max_shards` | int | `1024` | 最大 shard 数 |

## 编程方式加载

```go
import "github.com/atoncooper/mcache"

cfg, err := mcache.LoadConfig("config.yaml")
if err != nil {
    panic(err)
}

// 转换为 mcache.Options
opts, err := cfg.Cache.BuildOptions()
if err != nil {
    panic(err)
}

c, _ := mcache.New(opts)
```

## 默认配置

`mcache.DefaultConfig()` 返回所有字段的默认值，适合程序化配置场景：

```go
cfg := mcache.DefaultConfig()
cfg.Cache.Shards = 32
cfg.Server.Address = ":11212"
```
