#!/usr/bin/env python3
"""mcache 性能瓶颈三层诊断脚本。

回答三个问题：
  1. Python SDK 单线程极限是多少？     → SDK 层
  2. 多线程并发能扩展到多少？           → GIL / 服务端层
  3. pool_size 改变影响多大？           → 连接池层

将这三组数据与 kv_bench_mcache.py 的实际吞吐对比，即可定位瓶颈。

用法：
    python diagnose.py --mcache 127.0.0.1:11211
"""
from __future__ import annotations

import argparse
import os
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor

# 加载本地 SDK
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'sdk', 'python'))
from mcache import Client  # type: ignore


def make_value(i: int, size: int = 256) -> bytes:
    prefix = f"{i:016d}-".encode()
    return prefix + b'x' * max(0, size - len(prefix))


# ════════════════════════════════════════════════════════════════════════════
# Test 1: 单线程 SDK 极限（无并发、无 latency 跟踪）
# ════════════════════════════════════════════════════════════════════════════
def test_single_thread(host: str, port: int, n: int = 10_000) -> float:
    c = Client(host=host, port=port, pool_size=1, timeout=10.0)
    try:
        for i in range(1000):
            c.set(f'__diag1__{i}', make_value(i))
        t0 = time.perf_counter()
        for i in range(n):
            c.set(f'__diag1__{i % 1000}', make_value(i))
        elapsed = time.perf_counter() - t0
        return n / elapsed
    finally:
        c.close()


# ════════════════════════════════════════════════════════════════════════════
# Test 2: 多线程，每线程独立 Client（无 latency 跟踪，最小化脚本开销）
# ════════════════════════════════════════════════════════════════════════════
def test_multi_thread(host: str, port: int, num_clients: int,
                       n_per_client: int = 500) -> float:
    barrier = threading.Barrier(num_clients)

    def worker(cid: int) -> None:
        c = Client(host=host, port=port, pool_size=1, timeout=10.0)
        try:
            barrier.wait()
            for i in range(n_per_client):
                c.set(f'__diag2__{cid}_{i % 1000}', make_value(i))
        finally:
            c.close()

    t0 = time.perf_counter()
    with ThreadPoolExecutor(max_workers=num_clients) as pool:
        futures = [pool.submit(worker, cid) for cid in range(num_clients)]
        for f in futures:
            f.result()
    elapsed = time.perf_counter() - t0
    return (num_clients * n_per_client) / elapsed


# ════════════════════════════════════════════════════════════════════════════
# Test 3: 共享 Client，不同 pool_size 的影响（揭示连接池是否成为瓶颈）
# ════════════════════════════════════════════════════════════════════════════
def test_pool_size(host: str, port: int, pool_size: int,
                    num_clients: int = 128, n_per_client: int = 200) -> float:
    c = Client(host=host, port=port, pool_size=pool_size, timeout=10.0)
    barrier = threading.Barrier(num_clients)

    def worker(cid: int) -> None:
        barrier.wait()
        for i in range(n_per_client):
            c.set(f'__diag3__{cid}_{i % 1000}', make_value(i))

    try:
        t0 = time.perf_counter()
        with ThreadPoolExecutor(max_workers=num_clients) as pool:
            futures = [pool.submit(worker, cid) for cid in range(num_clients)]
            for f in futures:
                f.result()
        elapsed = time.perf_counter() - t0
        return (num_clients * n_per_client) / elapsed
    finally:
        c.close()


# ════════════════════════════════════════════════════════════════════════════
# Main
# ════════════════════════════════════════════════════════════════════════════
def main() -> None:
    p = argparse.ArgumentParser(description='mcache 性能瓶颈三层诊断')
    p.add_argument('--mcache', default='127.0.0.1:11211',
                   help='mcache 地址 (default: 127.0.0.1:11211)')
    args = p.parse_args()

    host, port_s = args.mcache.rsplit(':', 1)
    port = int(port_s)

    print('━' * 60)
    print('  mcache 性能瓶颈三层诊断')
    print('━' * 60)
    print(f'  目标: {args.mcache}\n')

    # ── Test 1 ──
    print('[1] 单线程 SDK 极限（10k ops, pool_size=1）')
    ops1 = test_single_thread(host, port)
    per_op_us = 1_000_000 / ops1
    print(f'    → {ops1:>10,.0f} ops/s')
    print(f'    → 单次 op ≈ {per_op_us:.0f} μs (含 Python 调用 + TCP 往返)\n')

    # ── Test 2 ──
    print('[2] 多线程扩展（每线程独立 Client + 独立连接）')
    print(f'    单线程基线: {ops1:,.0f} ops/s')
    print(f'    {"clients":>8} {"total ops/s":>12} {"per-client":>12} {"扩展比":>10}')
    multi_results: dict[int, float] = {}
    for nc in [1, 4, 16, 64, 128]:
        total = test_multi_thread(host, port, nc)
        multi_results[nc] = total
        per_client = total / nc
        ratio = total / ops1
        print(f'    {nc:>8} {total:>12,.0f} {per_client:>12,.0f} {ratio:>9.1f}x')
    print()

    # ── Test 3 ──
    print('[3] 共享 Client + 不同 pool_size（128 threads）')
    print(f'    {"pool_size":>10} {"ops/s":>12} {"vs ps=4":>10}')
    base_ps4 = None
    pool_results: dict[int, float] = {}
    for ps in [1, 4, 16, 64, 128]:
        ops = test_pool_size(host, port, ps)
        pool_results[ps] = ops
        if ps == 4:
            base_ps4 = ops
        ratio_str = f'{ops / base_ps4:.2f}x' if base_ps4 else '-'
        print(f'    {ps:>10} {ops:>12,.0f} {ratio_str:>10}')
    print()

    # ── 自动诊断 ──
    print('━' * 60)
    print('  诊断结论')
    print('━' * 60)
    max_multi = max(multi_results.values())
    ext_ratio = max_multi / ops1

    # GIL 判断
    if ext_ratio > 20:
        scaling = '✓ 良好扩展（GIL 不是主要瓶颈，I/O 等待释放 GIL）'
    elif ext_ratio > 5:
        scaling = '◐ 中度扩展（GIL 有一定影响，但服务端能消化更多）'
    else:
        scaling = '✗ 扩展受限（GIL 或服务端 worker 池是瓶颈）'

    # pool_size 判断
    if base_ps4 and pool_results.get(64, 0) > base_ps4 * 2:
        pool_diag = '✗ pool_size=4 偏小（提高到 64+ 能显著提升）'
    elif base_ps4 and pool_results.get(64, 0) > base_ps4 * 1.2:
        pool_diag = '◐ pool_size 略小（适度调大有帮助）'
    else:
        pool_diag = '✓ pool_size=4 已足够（连接复用 OK）'

    print(f'  • 单线程 Python SDK 极限:  {ops1:>8,.0f} ops/s ({per_op_us:.0f}μs/op)')
    print(f'  • 多线程峰值:              {max_multi:>8,.0f} ops/s ({ext_ratio:.1f}x 单线程)')
    print(f'  • 连接池影响:              {pool_diag}')
    print(f'  • GIL/服务端可扩展性:      {scaling}')
    print()
    print('  对比 kv_bench_mcache.py 结果，可定位瓶颈：')
    print(f'    • 若 kv_bench 总吞吐 < 多线程峰值 → kv_bench 脚本本身有开销（latency 跟踪等）')
    print(f'    • 若 kv_bench 总吞吐 ≈ 多线程峰值 → Python SDK / GIL 是瓶颈')
    print(f'    • 若两者都远低于服务能力      → 用 Go SDK 测真实极限')
    print('━' * 60)


if __name__ == '__main__':
    main()
