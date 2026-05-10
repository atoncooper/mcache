#!/usr/bin/env python3
"""Full comparison benchmark: mcache vs Redis.

Run::

    # Start both servers first:
    #   mcache server --config config.yaml           # port 11211
    #   redis-server --port 6379                     # port 6379
    #
    pip install -r requirements.txt
    python run_all.py

Results go to ``output/``:
  - results.csv    — per-run raw data
  - results.json   — machine-readable
  - charts/*.png   — performance comparison charts
  - report.md      — summary tables
"""

from __future__ import annotations

import sys
import os

# Ensure SDK importable
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'sdk', 'python'))

from harness import Workload, run_comparison
from report import generate_report


def main() -> None:
    # ---- Workload definitions ----

    workloads = [
        # ===== 1. Varying value size =====
        Workload(
            name='kv_small', description='KV 64B values, 50R/50W',
            key_count=10_000, value_size=64, total_ops=100_000,
            read_ratio=0.5, write_ratio=0.5, num_clients=8,
        ),
        Workload(
            name='kv_medium', description='KV 1KB values, 50R/50W',
            key_count=10_000, value_size=1024, total_ops=100_000,
            read_ratio=0.5, write_ratio=0.5, num_clients=8,
        ),
        Workload(
            name='kv_large', description='KV 64KB values, 50R/50W',
            key_count=1_000, value_size=65536, total_ops=10_000,
            read_ratio=0.5, write_ratio=0.5, num_clients=4,
        ),

        # ===== 2. Varying read/write ratio =====
        Workload(
            name='kv_read_heavy', description='KV 128B, 95R/5W',
            key_count=10_000, value_size=128, total_ops=100_000,
            read_ratio=0.95, write_ratio=0.05, num_clients=8,
        ),
        Workload(
            name='kv_write_heavy', description='KV 128B, 5R/95W',
            key_count=10_000, value_size=128, total_ops=100_000,
            read_ratio=0.05, write_ratio=0.95, num_clients=8,
        ),
        Workload(
            name='kv_balanced', description='KV 128B, 50R/50W',
            key_count=10_000, value_size=128, total_ops=100_000,
            read_ratio=0.5, write_ratio=0.5, num_clients=8,
        ),

        # ===== 3. Varying concurrency =====
        Workload(
            name='kv_c1', description='KV 128B, 50R/50W, 1 client',
            key_count=10_000, value_size=128, total_ops=50_000,
            read_ratio=0.5, write_ratio=0.5, num_clients=1,
        ),
        Workload(
            name='kv_c4', description='KV 128B, 50R/50W, 4 clients',
            key_count=10_000, value_size=128, total_ops=100_000,
            read_ratio=0.5, write_ratio=0.5, num_clients=4,
        ),
        Workload(
            name='kv_c16', description='KV 128B, 50R/50W, 16 clients',
            key_count=10_000, value_size=128, total_ops=200_000,
            read_ratio=0.5, write_ratio=0.5, num_clients=16,
        ),
        Workload(
            name='kv_c64', description='KV 128B, 50R/50W, 64 clients',
            key_count=10_000, value_size=128, total_ops=200_000,
            read_ratio=0.5, write_ratio=0.5, num_clients=64,
        ),

        # ===== 4. Key space pressure =====
        Workload(
            name='kv_wide', description='KV 128B, 50R/50W, 100K keys',
            key_count=100_000, value_size=128, total_ops=200_000,
            read_ratio=0.5, write_ratio=0.5, num_clients=8,
        ),
    ]

    print('=' * 60)
    print('mcache vs Redis — KV Benchmark Suite')
    print('=' * 60)
    print(f'Scenarios: {len(workloads)}')
    print(f'Runs per scenario: 3')
    print()

    results = run_comparison(
        workloads=workloads,
        mcache_addr=os.environ.get('MCACHE_ADDR', '127.0.0.1:11211'),
        redis_addr=os.environ.get('REDIS_ADDR', '127.0.0.1:6379'),
        output_dir=os.path.join(os.path.dirname(__file__), 'output'),
        runs=3,
    )

    # Generate charts + report
    output_dir = os.path.join(os.path.dirname(__file__), 'output')
    generate_report(results, output_dir)

    print('\nDone. See output/ for report and charts.')


if __name__ == '__main__':
    main()
