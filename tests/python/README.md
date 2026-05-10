# mcache vs Redis — Comparative Benchmark Suite

Full-stack KV performance comparison between mcache and Redis, designed for academic paper-grade benchmarking.

## Quick Start

### 1. Install Dependencies

```bash
cd tests/python
pip install -r requirements.txt
```

Requirements: Python 3.10+, `redis`, `psutil`, `matplotlib`, `numpy` (optional).

### 2. Start Both Servers

**mcache:**

```bash
# From project root
go build -o bin/mcache.exe ./cmd/mcache
./bin/mcache server --config config.yaml
# Listening on :11211
```

**Redis:**

```bash
redis-server --port 6379 --save "" --appendonly no
# Listening on :6379
```

### 3. Run Benchmarks

```bash
python run_all.py
```

Each scenario runs 3 times against both backends. Total runtime ~5–10 minutes depending on hardware.

### 4. View Results

```
output/
├── results.csv      ← Raw per-run data (import into Excel / Python / R)
├── results.json     ← Machine-readable summary
├── report.md        ← Paper-ready tables
└── charts/
    ├── throughput.png
    ├── latency.png
    └── resources.png
```

## Configuration

Override default addresses via environment variables:

```bash
# Remote servers
MCACHE_ADDR=192.168.1.10:11211 REDIS_ADDR=192.168.1.20:6379 python run_all.py

# Non-standard ports
MCACHE_ADDR=127.0.0.1:12345 REDIS_ADDR=127.0.0.1:6380 python run_all.py
```

## Workload Scenarios

11 predefined scenarios across 4 dimensions:

### Value Size

| Scenario | Value | Keys | Ops | Clients |
|----------|-------|------|-----|---------|
| `kv_small` | 64 B | 10K | 100K | 8 |
| `kv_medium` | 1 KB | 10K | 100K | 8 |
| `kv_large` | 64 KB | 1K | 10K | 4 |

### Read/Write Ratio

| Scenario | Read% | Write% | Value | Keys | Ops |
|----------|-------|--------|-------|------|-----|
| `kv_read_heavy` | 95% | 5% | 128B | 10K | 100K |
| `kv_write_heavy` | 5% | 95% | 128B | 10K | 100K |
| `kv_balanced` | 50% | 50% | 128B | 10K | 100K |

### Concurrency

| Scenario | Clients | Value | Keys | Ops |
|----------|---------|-------|------|-----|
| `kv_c1` | 1 | 128B | 10K | 50K |
| `kv_c4` | 4 | 128B | 10K | 100K |
| `kv_c16` | 16 | 128B | 10K | 200K |
| `kv_c64` | 64 | 128B | 10K | 200K |

### Key Space

| Scenario | Keys | Value | Ops | Clients |
|----------|------|-------|-----|---------|
| `kv_wide` | 100K | 128B | 200K | 8 |

## Custom Workloads

Add your own scenarios in `run_all.py`:

```python
from harness import Workload

my_workload = Workload(
    name='my_test',
    description='Custom: 10KB values, 80R/20W, 32 clients',
    key_count=5_000,
    value_size=10240,
    total_ops=50_000,
    read_ratio=0.8,
    write_ratio=0.2,
    num_clients=32,
    prepopulate=True,
)

workloads = [my_workload]
results = run_comparison(workloads)
```

## Collected Metrics

| Metric | Description |
|--------|-------------|
| `throughput_ops` | Operations per second |
| `p50_us` / `p95_us` / `p99_us` | Latency percentiles (microseconds) |
| `avg_us` / `min_us` / `max_us` | Latency statistics |
| `success_pct` | Success rate |
| `cpu_avg_pct` | Average CPU usage during benchmark |
| `mem_avg_mb` | Average RSS memory |
| `mem_peak_mb` | Peak RSS memory |

Resource metrics are collected via `psutil` at 50ms intervals in a background thread during the benchmark.

## Architecture

```
run_all.py            ← 11 workload definitions
    ↓
harness.py            ← Benchmark engine
    ├── Driver (abstract)
    │   ├── McacheDriver   → Python SDK (mcache.client)
    │   └── RedisDriver    → redis-py
    ├── Workload           ← Scenario parameters
    ├── Benchmark          ← Run orchestration
    ├── ResourceSampler    ← CPU / memory sampling
    └── run_comparison()   ← Multi-backend runner
    ↓
report.py             ← Charts + tables
    ↓
output/               ← Results
```

## Troubleshooting

**Connection refused:**
Ensure both servers are running. Check ports with `netstat -an | grep -E "11211|6379"`.

**No matplotlib charts:**
Install matplotlib: `pip install matplotlib numpy`. Charts are optional — CSV and markdown report always generated.

**High variance between runs:**
Close other applications. Increase warmup: set `Workload.warmup_ops=5000`. Disable prepopulate if keys already exist.

**Redis persists data across runs:**
Add `--save "" --appendonly no` to Redis command line, or set `prepopulate=False` on workloads.
