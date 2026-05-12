# mcache vs Redis — 对比测试套件

## 目录

- [快速开始](#快速开始)
- [kv_bench — 公平 KV 对比测试](#kv_bench--公平-kv-对比测试)
- [run_all — 11 场景全量对比](#run_all--11-场景全量对比)
- [concurrent_bench — 高并发压力测试](#concurrent_bench--高并发压力测试)
- [环境变量](#环境变量)
- [常见问题](#常见问题)

---

## 快速开始

### 1. 安装依赖

```bash
cd tests/python
pip install -r requirements.txt

# 图表需要（可选）
pip install matplotlib numpy
```

### 2. 启动服务

```bash
# mcache（项目根目录）
go build -o bin/mcache.exe ./cmd/mcache
./bin/mcache.exe server --config config.yaml
# → 监听 :11211

# Redis
redis-server --port 6379 --save "" --appendonly no
# → 监听 :6379
```

### 3. 运行测试

```bash
# 公平对比（推荐）
python kv_bench.py

# 全量 11 场景
python run_all.py

# 高并发压力
python concurrent_bench.py --concurrency 128 --duration 60
```

---

## kv_bench — 公平 KV 对比测试

`kv_bench.py` 是核心对比脚本。两个后端使用**完全相同的** key 序列、value 内容和操作序列，保证对比公平。

### 设计原则

| 维度 | 保证 |
|------|------|
| Key 序列 | 预生成确定性 trace（seed=42），两个后端共用 |
| Value | 相同 key 索引 = 相同字节内容 |
| 操作顺序 | 同一份 trace 文件回放 |
| 并发模型 | 相同 client 数、相同 ThreadPoolExecutor |
| 数据结构 | 仅 KV（单一类型，公平） |
| 网络条件 | 默认本地回环 |

### 用法

```bash
# 默认参数（5 万 key、20 万 ops、64 并发、256B value、80% 读）
python kv_bench.py

# 自定义参数
python kv_bench.py \
    --mcache 127.0.0.1:11211 \
    --redis 127.0.0.1:6379 \
    --keys 100000 \
    --ops 500000 \
    --clients 128 \
    --value-size 1024 \
    --read-ratio 0.8

# 纯读测试
python kv_bench.py --read-ratio 1.0 --keys 20000 --ops 100000

# 纯写测试
python kv_bench.py --read-ratio 0.0 --keys 20000 --ops 100000

# 连接远程
python kv_bench.py --mcache 192.168.1.10:11211 --redis 192.168.1.20:6379
```

### 参数

| 参数 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--mcache` | | `127.0.0.1:11211` | mcache 地址 |
| `--redis` | | `127.0.0.1:6379` | Redis 地址 |
| `--keys` | `-k` | `50000` | 不同 key 数量 |
| `--ops` | `-n` | `200000` | 总操作数 |
| `--clients` | `-c` | `64` | 并发 client 数 |
| `--value-size` | `-s` | `256` | value 字节数 |
| `--read-ratio` | `-r` | `0.8` | 读操作比例 |
| `--zipf-alpha` | | `1.2` | Zipf 倾斜参数（越大热点越集中） |
| `--seed` | | `42` | 随机种子（相同种子 = 相同 trace） |
| `--output` | `-o` | `output/kv_bench` | 输出目录 |
| `--skip-charts` | | | 跳过图表生成 |

### 输出

```
output/kv_bench/
├── trace.json              # 操作序列（可复现）
├── results.json            # 汇总数据（机器可读）
├── results.csv             # 表格（Excel 可打开）
└── charts/
    ├── throughput.png      # 吞吐量对比柱状图
    ├── latency_pct.png     # 延迟百分位对比（p50/p95/p99/p999）
    └── latency_cdf.png     # CDF 延迟分布曲线叠加
```

### 示例输出

```
============================================================
  对 比 结 果
============================================================
  Keys: 50,000  Ops: 200,000  Clients: 64  Value: 256B
  Read ratio: 0.8  Zipf α: 1.2

                   mcache          redis            diff
--------------------------------------------------------------
  ops/s             85,234          72,110          1.18x
  p50                312 us          389 us              -
  p95                891 us        1,203 us              -
  p99              1,456 us        2,341 us              -
  p999             3,201 us        5,678 us              -
  avg                445 us          567 us              -
```

---

## run_all — 11 场景全量对比

`run_all.py` 是综合对比套件，覆盖 4 个维度共 11 个场景。

### 维度

| 维度 | 场景 | 说明 |
|------|------|------|
| Value 大小 | `kv_small` (64B) / `kv_medium` (1KB) / `kv_large` (64KB) | 不同负载 |
| 读写比 | `kv_read_heavy` (95R/5W) / `kv_write_heavy` (5R/95W) / `kv_balanced` (50R/50W) | 访问模式 |
| 并发度 | `kv_c1` / `kv_c4` / `kv_c16` / `kv_c64` | 1~64 clients |
| Key 空间 | `kv_wide` (100K keys) | key 空间压力 |

### 运行

```bash
python run_all.py
# 每个场景跑 3 次，总耗时 ~5-10 分钟
```

### 输出

```
output/
├── results.csv
├── results.json
├── report.md
└── charts/
    ├── throughput.png
    ├── latency.png
    └── resources.png
```

### 自定义场景

```python
from harness import Workload, run_comparison

my = Workload(
    name='my_test', description='自定义',
    key_count=5000, value_size=10240, total_ops=50000,
    read_ratio=0.8, write_ratio=0.2, num_clients=32,
    prepopulate=True,
)
results = run_comparison([my])
```

---

## concurrent_bench — 高并发压力测试

`concurrent_bench.py` 模拟真实缓存负载，支持从文件读取数据、Zipf 热点分布、多数据类型混合。

### 运行

```bash
# 生成数据文件
python concurrent_bench.py --gen-data data.json --keys 50000

# 高并发压测
python concurrent_bench.py --data data.json --concurrency 128 --duration 60

# 纯读极限测试
python concurrent_bench.py -c 256 -d 120 -k 200000 --read-ratio 0.95
```

### 输出

```
output/concurrent_bench/
├── results.json
├── latency_hist.csv
└── summary.txt
```

---

## 环境变量

远端服务器地址：

```bash
MCACHE_ADDR=192.168.1.10:11211 REDIS_ADDR=192.168.1.20:6379 python run_all.py

# kv_bench 用命令行参数
python kv_bench.py --mcache 192.168.1.10:11211 --redis 192.168.1.20:6379
```

---

## 常见问题

### `ConnectionError: Connection refused`

确保两个服务已启动：
```bash
# 检查端口
netstat -an | grep -E "11211|6379"
```

### 图表不显示

安装 matplotlib：
```bash
pip install matplotlib numpy
```

生产环境跳过图表：`--skip-charts` 或 `python run_all.py`（自动跳过无 matplotlib 环境）。

### 结果波动大

- 关闭其他应用程序
- 增加预热：`Workload.warmup_ops=5000`
- 用 `--seed` 固定随机种子复现结果
- 多次运行取中位数

### Redis 持久化干扰

Redis 启动时加 `--save "" --appendonly no` 关闭持久化。

### 单机内存不足

减小 `--keys` 和 `--value-size`，或增加机器内存。Redis 的 `--maxmemory` 也会影响结果。
