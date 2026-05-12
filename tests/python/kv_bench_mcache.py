#!/usr/bin/env python3
"""mcache 单独 KV 性能测试（无需 Redis）。

设计原则：
  - 单一后端（mcache only），用于无 Redis 环境或纯压测
  - Trace 生成 / value 编码 / worker 模型与 kv_bench.py 完全一致
  - 仅测试 KV 数据结构（Get / Set）

用法::

    python kv_bench_mcache.py
    python kv_bench_mcache.py --mcache 127.0.0.1:11211 \
        --keys 100000 --ops 500000 --clients 128

输出::

    output/kv_bench_mcache/
    ├── trace.json         操作序列（可复现）
    ├── results.json       汇总数据
    ├── results.csv        表格
    └── charts/            图表（需 matplotlib）
        ├── throughput.png
        ├── latency_pct.png
        └── latency_cdf.png
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import random
import sys
import threading
import time
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
    """生成确定性操作序列。"""
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


# ═════════════════════════════════════════════════════════════════════════════
# 结果
# ═════════════════════════════════════════════════════════════════════════════


@dataclass
class BackendResult:
    backend: str = 'mcache'
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
    """为给定 key 索引生成确定性 value。"""
    prefix = f"{key_idx:016d}-".encode()
    if value_size <= len(prefix):
        return prefix[:value_size]
    padding = value_size - len(prefix)
    return prefix + b'x' * padding


# ═════════════════════════════════════════════════════════════════════════════
# 测试执行
# ═════════════════════════════════════════════════════════════════════════════


def run_mcache(
    driver_factory,
    trace: list[OpTrace],
    key_count: int,
    value_size: int,
    num_clients: int,
) -> BackendResult:
    """对 mcache 回放 trace，返回结果。"""
    print(f"\n{'─' * 50}")
    prefill_workers = min(num_clients, 64)
    print(f"  [mcache] 预填充 {key_count:,} keys (并发 {prefill_workers}) …")

    progress = [0]
    plock = threading.Lock()

    def _prefill_chunk(start: int, end: int) -> None:
        d = driver_factory()
        try:
            for i in range(start, end):
                d.set(f'__bench__{i}', _make_value(i, value_size))
                if (i - start) % 1000 == 999:
                    with plock:
                        progress[0] += 1000
                        print(f"    … {progress[0]:,} / {key_count:,} "
                              f"({progress[0] * 100 // key_count}%)", flush=True)
        finally:
            d.close()

    chunk = (key_count + prefill_workers - 1) // prefill_workers
    pt0 = time.perf_counter()
    with ThreadPoolExecutor(max_workers=prefill_workers) as ppool:
        pfutures = []
        for w in range(prefill_workers):
            s = w * chunk
            e = min(s + chunk, key_count)
            if s >= e:
                break
            pfutures.append(ppool.submit(_prefill_chunk, s, e))
        for f in pfutures:
            f.result()
    pre_elapsed = time.perf_counter() - pt0

    driver = driver_factory()
    try:
        print(f"  [mcache] 预填充完成 ({pre_elapsed:.1f}s),开始回放 {len(trace):,} ops "
              f"({num_clients} clients) …")

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
                except Exception:
                    errs += 1
                lats.append((time.perf_counter() - t0) * 1_000_000)
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

    result = BackendResult()
    result.total_ops = total_ops
    result.duration_s = round(elapsed, 3)
    result.throughput_ops = round(total_ops / elapsed, 1) if elapsed > 0 else 0
    for i in range(num_clients):
        result.latency_us.extend(all_latencies[i])
        result.error_ops += error_counts[i]
    result.success_ops = result.total_ops - result.error_ops

    print(f"  [mcache] 完成: {result.throughput_ops:,.0f} ops/s  "
          f"p50={_pct(result.latency_us, 50):.0f}us  "
          f"p99={_pct(result.latency_us, 99):.0f}us  "
          f"errors={result.error_ops}")
    return result


# ═════════════════════════════════════════════════════════════════════════════
# 报告
# ═════════════════════════════════════════════════════════════════════════════


def print_summary(result: BackendResult, params: dict) -> None:
    lats = result.latency_us
    print(f"\n{'=' * 60}")
    print(f"  mcache KV 测 试 结 果")
    print(f"{'=' * 60}")
    print(f"  Keys: {params['key_count']:,}  "
          f"Ops: {params['total_ops']:,}  "
          f"Clients: {params['num_clients']}  "
          f"Value: {params['value_size']}B")
    print(f"  Read ratio: {params['read_ratio']}  "
          f"Zipf α: {params['zipf_alpha']}")
    print()

    avg = sum(lats) / len(lats) if lats else 0
    rows = [
        ('吞吐量', f"{result.throughput_ops:,.0f}", 'ops/s'),
        ('耗时', f"{result.duration_s:.3f}", 's'),
        ('p50', f"{_pct(lats, 50):.0f}", 'us'),
        ('p95', f"{_pct(lats, 95):.0f}", 'us'),
        ('p99', f"{_pct(lats, 99):.0f}", 'us'),
        ('p999', f"{_pct(lats, 99.9):.0f}", 'us'),
        ('avg', f"{avg:.0f}", 'us'),
        ('min', f"{min(lats) if lats else 0:.0f}", 'us'),
        ('max', f"{max(lats) if lats else 0:.0f}", 'us'),
        ('errors', str(result.error_ops), ''),
    ]
    for label, value, unit in rows:
        print(f"  {label:<10} {value:>14} {unit}")
    print()


def save_results(result: BackendResult, params: dict, output_dir: str) -> None:
    os.makedirs(output_dir, exist_ok=True)
    lats = result.latency_us

    # JSON
    summary = {
        'params': params,
        'mcache': {
            'total_ops': result.total_ops,
            'success_ops': result.success_ops,
            'error_ops': result.error_ops,
            'duration_s': result.duration_s,
            'throughput_ops': result.throughput_ops,
            'latency_us': {
                'avg': round(sum(lats) / len(lats), 1) if lats else 0,
                'p50': round(_pct(lats, 50), 1),
                'p95': round(_pct(lats, 95), 1),
                'p99': round(_pct(lats, 99), 1),
                'p999': round(_pct(lats, 99.9), 1),
                'min': round(min(lats), 1) if lats else 0,
                'max': round(max(lats), 1) if lats else 0,
            },
        },
    }
    json_path = os.path.join(output_dir, 'results.json')
    with open(json_path, 'w') as f:
        json.dump(summary, f, indent=2)

    # CSV
    csv_path = os.path.join(output_dir, 'results.csv')
    with open(csv_path, 'w', newline='', encoding='utf-8') as f:
        writer = csv.writer(f)
        writer.writerow(['backend', 'total_ops', 'success_ops', 'error_ops',
                         'duration_s', 'throughput_ops',
                         'p50_us', 'p95_us', 'p99_us', 'p999_us', 'avg_us',
                         'min_us', 'max_us'])
        writer.writerow([
            'mcache', result.total_ops, result.success_ops, result.error_ops,
            result.duration_s, result.throughput_ops,
            round(_pct(lats, 50), 1), round(_pct(lats, 95), 1),
            round(_pct(lats, 99), 1), round(_pct(lats, 99.9), 1),
            round(sum(lats) / len(lats), 1) if lats else 0,
            round(min(lats), 1) if lats else 0,
            round(max(lats), 1) if lats else 0,
        ])

    print(f"  → {json_path}")
    print(f"  → {csv_path}")


def draw_charts(result: BackendResult, params: dict, output_dir: str) -> None:
    if not HAS_MPL:
        print("  (跳过图表: pip install matplotlib numpy)")
        return

    charts_dir = os.path.join(output_dir, 'charts')
    os.makedirs(charts_dir, exist_ok=True)
    lats = result.latency_us
    color = '#4CAF50'

    # ── 吞吐量（单柱）──
    fig, ax = plt.subplots(figsize=(5, 4))
    bars = ax.bar(['mcache'], [result.throughput_ops], color=color, width=0.4)
    for b, v in zip(bars, [result.throughput_ops]):
        ax.text(b.get_x() + b.get_width() / 2, v * 1.01, f'{v:,.0f}',
                ha='center', fontsize=14, fontweight='bold')
    ax.set_ylabel('ops/s', fontsize=12)
    ax.set_title(f'mcache Throughput ({params["num_clients"]} clients, '
                 f'{params["value_size"]}B, r{params["read_ratio"]:.0%})',
                 fontsize=11)
    ax.yaxis.set_major_formatter(mticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))
    ax.set_ylim(0, max(result.throughput_ops * 1.15, 1))
    fig.tight_layout()
    fig.savefig(os.path.join(charts_dir, 'throughput.png'), dpi=150)
    plt.close(fig)

    # ── 延迟百分位（单组）──
    fig, ax = plt.subplots(figsize=(6, 4))
    pcts = [('p50', 50.0), ('p95', 95.0), ('p99', 99.0), ('p999', 99.9)]
    labels = [name for name, _ in pcts]
    vals = [_pct(lats, pv) for _, pv in pcts]
    bars = ax.bar(labels, vals, color=color, width=0.5)
    for b, v in zip(bars, vals):
        ax.text(b.get_x() + b.get_width() / 2, v + max(vals) * 0.02,
                f'{v:.0f}', ha='center', fontsize=10)
    ax.set_ylabel('latency (us)', fontsize=12)
    ax.set_title('mcache Latency Percentiles', fontsize=11)
    fig.tight_layout()
    fig.savefig(os.path.join(charts_dir, 'latency_pct.png'), dpi=150)
    plt.close(fig)

    # ── CDF 单曲线 ──
    fig, ax = plt.subplots(figsize=(7, 4))
    s = sorted(lats)
    y = [i / len(s) * 100 for i in range(len(s))]
    ax.plot(s, y, label='mcache', color=color, linewidth=1.5)
    ax.set_xlabel('latency (us)', fontsize=12)
    ax.set_ylabel('percentile', fontsize=12)
    ax.set_title('mcache Latency CDF', fontsize=11)
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
        description='mcache 单独 KV 性能测试（无需 Redis）',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  python kv_bench_mcache.py
  python kv_bench_mcache.py --mcache 127.0.0.1:11211 \\
      --keys 100000 --ops 500000 --clients 128 --value-size 1024
  python kv_bench_mcache.py --mcache 10.0.0.1:11211 \\
      --read-ratio 0.95 --keys 20000 --ops 200000
        """,
    )
    p.add_argument('--mcache', default='127.0.0.1:11211',
                   help='mcache 地址 (default: 127.0.0.1:11211)')
    p.add_argument('--keys', '-k', type=int, default=50000,
                   help='不同 key 数量 (default: 50000)')
    p.add_argument('--ops', '-n', type=int, default=200000,
                   help='总操作数 (default: 200000)')
    p.add_argument('--clients', '-c', type=int, default=64,
                   help='并发 client 数 (default: 64)')
    p.add_argument('--value-size', '-s', type=int, default=256,
                   help='value 字节数 (default: 256)')
    p.add_argument('--read-ratio', '-r', type=float, default=0.8,
                   help='读操作比例 (default: 0.8)')
    p.add_argument('--zipf-alpha', type=float, default=1.2,
                   help='Zipf 倾斜参数 (default: 1.2)')
    p.add_argument('--seed', type=int, default=42,
                   help='随机种子 (default: 42)')
    p.add_argument('--output', '-o', default='output/kv_bench_mcache',
                   help='输出目录 (default: output/kv_bench_mcache)')
    p.add_argument('--skip-charts', action='store_true', help='跳过图表生成')
    args = p.parse_args()

    # 解析地址
    host, port_str = args.mcache.rsplit(':', 1)
    port = int(port_str)

    params = {
        'key_count': args.keys, 'total_ops': args.ops, 'num_clients': args.clients,
        'value_size': args.value_size, 'read_ratio': args.read_ratio,
        'zipf_alpha': args.zipf_alpha, 'seed': args.seed,
    }

    print(f"{'=' * 60}")
    print(f"  mcache 单独 KV 性能测试")
    print(f"{'=' * 60}")
    print(f"  Keys: {args.keys:,}  Ops: {args.ops:,}  Clients: {args.clients}")
    print(f"  Value: {args.value_size}B  Read: {args.read_ratio:.0%}  "
          f"Zipf α: {args.zipf_alpha}")
    print(f"  mcache: {args.mcache}")
    print()

    # Phase 1: 生成 trace
    print(f"[1/2] 生成操作序列 ({args.ops:,} ops, seed={args.seed}) …")
    trace = generate_trace(
        key_count=args.keys,
        total_ops=args.ops,
        read_ratio=args.read_ratio,
        value_size=args.value_size,
        zipf_alpha=args.zipf_alpha,
        seed=args.seed,
    )
    os.makedirs(args.output, exist_ok=True)
    trace_path = os.path.join(args.output, 'trace.json')
    with open(trace_path, 'w') as f:
        json.dump([{'key_idx': t.key_idx, 'op': t.op, 'value_size': t.value_size}
                   for t in trace], f)
    print(f"  → {trace_path}")

    # Phase 2: 测试 mcache
    print(f"\n[2/2] 测试 mcache …")
    result = run_mcache(
        lambda: McacheDriver(host, port),
        trace, args.keys, args.value_size, args.clients,
    )

    # 报告
    print_summary(result, params)
    save_results(result, params, args.output)
    if not args.skip_charts:
        draw_charts(result, params, args.output)

    print(f"\n完成。输出目录: {args.output}/")


if __name__ == '__main__':
    main()
