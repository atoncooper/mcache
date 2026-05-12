#!/usr/bin/env python3
"""mcache vs Redis 公平 KV 对比测试。

设计原则：
  - 两个后端使用**完全相同的** key 序列、value 内容、操作序列
  - 仅测试 KV 数据结构（单一类型保证公平）
  - 单机本地回环（排除网络干扰）

用法::

    # 确保两个服务已启动
    #   mcache server --config config.yaml     → :11211
    #   redis-server --port 6379 --save ""      → :6379

    python kv_bench.py \
        --mcache 127.0.0.1:11211 \
        --redis 127.0.0.1:6379 \
        --keys 50000 --ops 200000 --clients 64

输出::

    output/kv_bench/
    ├── trace.json         操作序列（可复现）
    ├── results.json       汇总数据
    ├── results.csv        表格
    └── charts/            对比图表（需 matplotlib）
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import random
import sys
import time
import threading
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass, field
from typing import Optional

# SDK 路径
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'sdk', 'python'))

# ── matplotlib 可选 ─────────────────────────────────────────────────────────
try:
    import matplotlib
    matplotlib.use('Agg')
    import matplotlib.pyplot as plt
    import matplotlib.ticker as mticker
    HAS_MPL = True
except ImportError:
    HAS_MPL = False


# ═════════════════════════════════════════════════════════════════════════════
# Trace 生成
# ═════════════════════════════════════════════════════════════════════════════

def _zipf_cdf(n: int, alpha: float) -> list[float]:
    """预计算 Zipf CDF，O(log n) 采样。"""
    c = [0.0] * (n + 1)
    for i in range(1, n + 1):
        c[i] = c[i - 1] + 1.0 / (i ** alpha)
    return [v / c[n] for v in c]


def _zipf_sample(cdf: list[float], rng: random.Random) -> int:
    r = rng.random()
    lo, hi = 0, len(cdf) - 1
    while lo < hi:
        mid = (lo + hi) // 2
        if cdf[mid + 1] < r:
            lo = mid + 1
        else:
            hi = mid
    return lo


@dataclass
class OpTrace:
    """单条操作记录。"""
    key_idx: int       # key 索引 [0, key_count)
    op: str            # 'get' | 'set'
    value_size: int    # value 字节数


def generate_trace(
    key_count: int,
    total_ops: int,
    read_ratio: float,
    value_size: int,
    zipf_alpha: float,
    seed: int = 42,
) -> list[OpTrace]:
    """生成确定性操作序列。

    固定随机种子确保每次生成完全相同的 trace。
    """
    rng = random.Random(seed)
    cdf = _zipf_cdf(key_count, zipf_alpha)

    trace: list[OpTrace] = []
    for _ in range(total_ops):
        key_idx = _zipf_sample(cdf, rng)
        op = 'get' if rng.random() < read_ratio else 'set'
        trace.append(OpTrace(key_idx=key_idx, op=op, value_size=value_size))
    return trace


# ═════════════════════════════════════════════════════════════════════════════
# 后端驱动
# ═════════════════════════════════════════════════════════════════════════════


class McacheDriver:
    name = 'mcache'

    def __init__(self, host: str, port: int):
        from mcache import Client
        self._client = Client(host=host, port=port, pool_size=4, timeout=10.0)

    def get(self, key: str) -> Optional[bytes]:
        return self._client.get(key)

    def set(self, key: str, value: bytes) -> bool:
        return self._client.set(key, value)

    def delete(self, key: str) -> bool:
        return self._client.delete(key)

    def close(self) -> None:
        self._client.close()


class RedisDriver:
    name = 'redis'

    def __init__(self, host: str, port: int):
        import redis
        self._client = redis.Redis(host=host, port=port, socket_timeout=10,
                                   socket_connect_timeout=10, decode_responses=False)

    def get(self, key: str) -> Optional[bytes]:
        return self._client.get(key)

    def set(self, key: str, value: bytes) -> bool:
        return self._client.set(key, value) is True

    def delete(self, key: str) -> bool:
        return self._client.delete(key) > 0

    def close(self) -> None:
        self._client.close()


# ═════════════════════════════════════════════════════════════════════════════
# 结果
# ═════════════════════════════════════════════════════════════════════════════


@dataclass
class BackendResult:
    backend: str
    total_ops: int = 0
    success_ops: int = 0
    error_ops: int = 0
    latency_us: list[float] = field(default_factory=list)
    duration_s: float = 0.0
    throughput_ops: float = 0.0


def _pct(data: list[float], p: float) -> float:
    if not data:
        return 0.0
    s = sorted(data)
    k = (len(s) - 1) * p / 100.0
    f = int(k)
    c = k - f
    if f + 1 < len(s):
        return s[f] * (1 - c) + s[f + 1] * c
    return float(s[f])


def _make_value(key_idx: int, value_size: int) -> bytes:
    """为给定 key 索引生成确定性 value（相同索引 = 相同内容）。"""
    prefix = f"{key_idx:016d}-".encode()
    if value_size <= len(prefix):
        return prefix[:value_size]
    padding = value_size - len(prefix)
    return prefix + b'x' * padding


# ═════════════════════════════════════════════════════════════════════════════
# 测试执行
# ═════════════════════════════════════════════════════════════════════════════


def run_backend(
    driver_factory,
    trace: list[OpTrace],
    key_count: int,
    value_size: int,
    num_clients: int,
    backend_name: str,
) -> BackendResult:
    """对单个后端回放 trace，返回结果。"""
    print(f"\n{'─' * 50}")
    print(f"  [{backend_name}] 预填充 {key_count:,} keys …")

    driver = driver_factory()
    try:
        # 预填充：所有 key 写入相同 value
        for i in range(key_count):
            driver.set(f'__bench__{i}', _make_value(i, value_size))
        print(f"  [{backend_name}] 预填充完成，开始回放 {len(trace):,} ops ({num_clients} clients) …")

        # 分发操作到各 client
        total_ops = len(trace)
        ops_per_client = total_ops // num_clients
        all_latencies: list[list[float]] = [[] for _ in range(num_clients)]
        error_counts = [0] * num_clients
        bar = threading.Barrier(num_clients)

        def worker(cid: int, start: int, end: int):
            d = driver_factory()
            lats: list[float] = []
            errs = 0
            bar.wait()
            for idx in range(start, end):
                t = trace[idx]
                key = f'__bench__{t.key_idx}'
                t0 = time.perf_counter()
                try:
                    if t.op == 'get':
                        d.get(key)
                    else:
                        d.set(key, _make_value(t.key_idx, t.value_size))
                    elapsed = (time.perf_counter() - t0) * 1_000_000
                    lats.append(elapsed)
                except Exception:
                    errs += 1
                    elapsed = (time.perf_counter() - t0) * 1_000_000
                    lats.append(elapsed)
            d.close()
            all_latencies[cid] = lats
            error_counts[cid] = errs

        t0 = time.perf_counter()
        with ThreadPoolExecutor(max_workers=num_clients) as pool:
            futures = []
            for cid in range(num_clients):
                s = cid * ops_per_client
                e = s + ops_per_client if cid < num_clients - 1 else total_ops
                futures.append(pool.submit(worker, cid, s, e))
            for f in futures:
                f.result()
        elapsed = time.perf_counter() - t0

    finally:
        driver.close()

    # 汇总
    result = BackendResult(backend=backend_name)
    result.total_ops = total_ops
    result.duration_s = round(elapsed, 3)
    result.throughput_ops = round(total_ops / elapsed, 1) if elapsed > 0 else 0
    for i in range(num_clients):
        result.latency_us.extend(all_latencies[i])
        result.error_ops += error_counts[i]
    result.success_ops = result.total_ops - result.error_ops

    print(f"  [{backend_name}] 完成: {result.throughput_ops:,.0f} ops/s  "
          f"p50={_pct(result.latency_us, 50):.0f}us  "
          f"p99={_pct(result.latency_us, 99):.0f}us  "
          f"errors={result.error_ops}")
    return result


# ═════════════════════════════════════════════════════════════════════════════
# 报告
# ═════════════════════════════════════════════════════════════════════════════


def print_summary(results: dict[str, BackendResult], params: dict) -> None:
    print(f"\n{'=' * 60}")
    print(f"  对 比 结 果")
    print(f"{'=' * 60}")
    print(f"  Keys: {params['key_count']:,}  "
          f"Ops: {params['total_ops']:,}  "
          f"Clients: {params['num_clients']}  "
          f"Value: {params['value_size']}B")
    print(f"  Read ratio: {params['read_ratio']}  "
          f"Zipf α: {params['zipf_alpha']}")
    print()

    header = f"{'':>12} {'mcache':>14} {'redis':>14} {'diff':>14}"
    print(header)
    print("-" * len(header))

    m = results.get('mcache')
    r = results.get('redis')

    if m and r:
        rows = [
            ('ops/s', f"{m.throughput_ops:,.0f}", f"{r.throughput_ops:,.0f}",
             f"{m.throughput_ops / r.throughput_ops:.2f}x" if r.throughput_ops > 0 else '-'),
            ('p50', f"{_pct(m.latency_us, 50):.0f} us", f"{_pct(r.latency_us, 50):.0f} us", '-'),
            ('p95', f"{_pct(m.latency_us, 95):.0f} us", f"{_pct(r.latency_us, 95):.0f} us", '-'),
            ('p99', f"{_pct(m.latency_us, 99):.0f} us", f"{_pct(r.latency_us, 99):.0f} us", '-'),
            ('p999', f"{_pct(m.latency_us, 99.9):.0f} us", f"{_pct(r.latency_us, 99.9):.0f} us", '-'),
            ('avg', f"{sum(m.latency_us)/len(m.latency_us):.0f} us" if m.latency_us else '-',
             f"{sum(r.latency_us)/len(r.latency_us):.0f} us" if r.latency_us else '-', '-'),
            ('errors', str(m.error_ops), str(r.error_ops), '-'),
        ]
        for label, mv, rv, diff in rows:
            print(f"  {label:<10} {mv:>14} {rv:>14} {diff:>14}")
    print()


def save_results(results: dict[str, BackendResult], params: dict, output_dir: str) -> None:
    os.makedirs(output_dir, exist_ok=True)

    # JSON
    summary = {'params': params}
    for name, r in results.items():
        lats = r.latency_us
        summary[name] = {
            'total_ops': r.total_ops,
            'success_ops': r.success_ops,
            'error_ops': r.error_ops,
            'duration_s': r.duration_s,
            'throughput_ops': r.throughput_ops,
            'latency_us': {
                'avg': round(sum(lats) / len(lats), 1) if lats else 0,
                'p50': round(_pct(lats, 50), 1),
                'p95': round(_pct(lats, 95), 1),
                'p99': round(_pct(lats, 99), 1),
                'p999': round(_pct(lats, 99.9), 1),
                'min': round(min(lats), 1) if lats else 0,
                'max': round(max(lats), 1) if lats else 0,
            },
        }
    with open(os.path.join(output_dir, 'results.json'), 'w') as f:
        json.dump(summary, f, indent=2)

    # CSV
    csv_path = os.path.join(output_dir, 'results.csv')
    with open(csv_path, 'w', newline='', encoding='utf-8') as f:
        writer = csv.writer(f)
        writer.writerow(['backend', 'total_ops', 'success_ops', 'error_ops',
                         'duration_s', 'throughput_ops',
                         'p50_us', 'p95_us', 'p99_us', 'p999_us', 'avg_us',
                         'min_us', 'max_us'])
        for name, r in results.items():
            lats = r.latency_us
            writer.writerow([
                name, r.total_ops, r.success_ops, r.error_ops,
                r.duration_s, r.throughput_ops,
                round(_pct(lats, 50), 1), round(_pct(lats, 95), 1),
                round(_pct(lats, 99), 1), round(_pct(lats, 99.9), 1),
                round(sum(lats) / len(lats), 1) if lats else 0,
                round(min(lats), 1) if lats else 0,
                round(max(lats), 1) if lats else 0,
            ])

    print(f"  → {os.path.join(output_dir, 'results.json')}")
    print(f"  → {csv_path}")


def draw_charts(results: dict[str, BackendResult], params: dict, output_dir: str) -> None:
    if not HAS_MPL:
        print("  (跳过图表: pip install matplotlib)")
        return

    charts_dir = os.path.join(output_dir, 'charts')
    os.makedirs(charts_dir, exist_ok=True)

    m = results.get('mcache')
    r = results.get('redis')
    if not m or not r:
        return

    # ── 吞吐量对比 ──
    fig, ax = plt.subplots(figsize=(6, 4))
    backends = ['mcache', 'redis']
    values = [m.throughput_ops, r.throughput_ops]
    colors = ['#4CAF50', '#F44336']
    bars = ax.bar(backends, values, color=colors, width=0.4)
    for bar, v in zip(bars, values):
        ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + max(values) * 0.01,
                f'{v:,.0f}', ha='center', fontsize=12, fontweight='bold')
    ax.set_ylabel('ops/s', fontsize=12)
    ax.set_title(f'Throughput ({params["num_clients"]} clients, '
                 f'{params["value_size"]}B, r{params["read_ratio"]:.0%})', fontsize=11)
    ax.yaxis.set_major_formatter(mticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))
    fig.tight_layout()
    fig.savefig(os.path.join(charts_dir, 'throughput.png'), dpi=150)
    plt.close(fig)

    # ── 延迟百分位对比 ──
    fig, ax = plt.subplots(figsize=(7, 4))
    pcts = ['p50', 'p95', 'p99', 'p999']
    x = range(len(pcts))
    w = 0.35
    m_vals = [_pct(m.latency_us, float(p[1:])) for p in pcts]
    r_vals = [_pct(r.latency_us, float(p[1:])) for p in pcts]
    ax.bar([i - w/2 for i in x], m_vals, w, label='mcache', color='#4CAF50')
    ax.bar([i + w/2 for i in x], r_vals, w, label='redis', color='#F44336')
    for i, (mv, rv) in enumerate(zip(m_vals, r_vals)):
        ax.text(i - w/2, mv + max(m_vals) * 0.02, f'{mv:.0f}', ha='center', fontsize=8)
        ax.text(i + w/2, rv + max(r_vals) * 0.02, f'{rv:.0f}', ha='center', fontsize=8)
    ax.set_xticks(x)
    ax.set_xticklabels(pcts)
    ax.set_ylabel('latency (us)', fontsize=12)
    ax.legend(fontsize=10)
    ax.set_title('Latency Percentiles', fontsize=11)
    fig.tight_layout()
    fig.savefig(os.path.join(charts_dir, 'latency_pct.png'), dpi=150)
    plt.close(fig)

    # ── CDF 叠加 ──
    fig, ax = plt.subplots(figsize=(7, 4))
    for name, r, color in [('mcache', m, '#4CAF50'), ('redis', r, '#F44336')]:
        lats = sorted(r.latency_us)
        y = [i / len(lats) * 100 for i in range(len(lats))]
        ax.plot(lats, y, label=name, color=color, linewidth=1.5)
    ax.set_xlabel('latency (us)', fontsize=12)
    ax.set_ylabel('percentile', fontsize=12)
    ax.set_title('Latency CDF', fontsize=11)
    ax.legend(fontsize=10)
    ax.set_xlim(left=0)
    ax.set_ylim(0, 100)
    fig.tight_layout()
    fig.savefig(os.path.join(charts_dir, 'latency_cdf.png'), dpi=150)
    plt.close(fig)

    print(f"  → {charts_dir}/")


# ═════════════════════════════════════════════════════════════════════════════
# CLI
# ═════════════════════════════════════════════════════════════════════════════


def main() -> None:
    p = argparse.ArgumentParser(
        description='mcache vs Redis 公平 KV 对比测试',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  python kv_bench.py --mcache 127.0.0.1:11211 --redis 127.0.0.1:6379
  python kv_bench.py --mcache 127.0.0.1:11211 --redis 127.0.0.1:6379 \\
      --keys 100000 --ops 500000 --clients 128 --value-size 1024
  python kv_bench.py --mcache 10.0.0.1:11211 --redis 10.0.0.2:6379 \\
      --read-ratio 0.95 --keys 20000 --ops 200000
        """,
    )
    p.add_argument('--mcache', default='127.0.0.1:11211', help='mcache 地址 (default: 127.0.0.1:11211)')
    p.add_argument('--redis', default='127.0.0.1:6379', help='Redis 地址 (default: 127.0.0.1:6379)')
    p.add_argument('--keys', '-k', type=int, default=50000, help='不同 key 数量 (default: 50000)')
    p.add_argument('--ops', '-n', type=int, default=200000, help='总操作数 (default: 200000)')
    p.add_argument('--clients', '-c', type=int, default=64, help='并发 client 数 (default: 64)')
    p.add_argument('--value-size', '-s', type=int, default=256, help='value 字节数 (default: 256)')
    p.add_argument('--read-ratio', '-r', type=float, default=0.8, help='读操作比例 (default: 0.8)')
    p.add_argument('--zipf-alpha', type=float, default=1.2, help='Zipf 倾斜参数 (default: 1.2)')
    p.add_argument('--seed', type=int, default=42, help='随机种子 (default: 42)')
    p.add_argument('--output', '-o', default='output/kv_bench', help='输出目录 (default: output/kv_bench)')
    p.add_argument('--skip-charts', action='store_true', help='跳过图表生成')
    args = p.parse_args()

    # 解析地址
    m_host, m_port_str = args.mcache.rsplit(':', 1)
    r_host, r_port_str = args.redis.rsplit(':', 1)
    m_port = int(m_port_str)
    r_port = int(r_port_str)

    params = {
        'key_count': args.keys, 'total_ops': args.ops, 'num_clients': args.clients,
        'value_size': args.value_size, 'read_ratio': args.read_ratio,
        'zipf_alpha': args.zipf_alpha, 'seed': args.seed,
    }

    print(f"{'=' * 60}")
    print(f"  mcache vs Redis — 公平 KV 对比测试")
    print(f"{'=' * 60}")
    print(f"  Keys: {args.keys:,}  Ops: {args.ops:,}  Clients: {args.clients}")
    print(f"  Value: {args.value_size}B  Read: {args.read_ratio:.0%}  Zipf α: {args.zipf_alpha}")
    print(f"  mcache: {args.mcache}  redis: {args.redis}")
    print()

    # Phase 1: 生成 trace（只生成一次，两边共用）
    print(f"[1/3] 生成操作序列 ({args.ops:,} ops, seed={args.seed}) …")
    trace = generate_trace(
        key_count=args.keys,
        total_ops=args.ops,
        read_ratio=args.read_ratio,
        value_size=args.value_size,
        zipf_alpha=args.zipf_alpha,
        seed=args.seed,
    )
    # 保存 trace
    os.makedirs(args.output, exist_ok=True)
    trace_path = os.path.join(args.output, 'trace.json')
    with open(trace_path, 'w') as f:
        json.dump([{'key_idx': t.key_idx, 'op': t.op, 'value_size': t.value_size} for t in trace], f)
    print(f"  → {trace_path}")

    results: dict[str, BackendResult] = {}

    # Phase 2: mcache
    print(f"\n[2/3] 测试 mcache …")
    r_mc = run_backend(
        lambda: McacheDriver(m_host, m_port),
        trace, args.keys, args.value_size, args.clients, 'mcache',
    )
    results['mcache'] = r_mc

    # Phase 3: redis
    print(f"\n[3/3] 测试 redis …")
    r_rd = run_backend(
        lambda: RedisDriver(r_host, r_port),
        trace, args.keys, args.value_size, args.clients, 'redis',
    )
    results['redis'] = r_rd

    # 报告
    print_summary(results, params)
    save_results(results, params, args.output)
    if not args.skip_charts:
        draw_charts(results, params, args.output)

    print(f"\n完成。输出目录: {args.output}/")


if __name__ == '__main__':
    main()
