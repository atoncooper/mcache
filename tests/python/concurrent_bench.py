#!/usr/bin/env python3
"""Real-world cache load simulator — reads a data file, sends concurrent requests
to an mcache server with realistic access patterns, and reports results.

Usage::

    # 1. Start the mcache server first:
    #    ./bin/mcache server --config config.local.yaml
    #
    # 2. Install Python deps:
    #    pip install -r requirements.txt
    #
    # 3. Generate a data file (optional — built-in generator available):
    #    python concurrent_bench.py --gen-data data.json --keys 50000
    #
    # 4. Run the benchmark:
    #    python concurrent_bench.py --data data.json --concurrency 64 --duration 60
    #
    # 5. Or run with built-in Zipf workload (no data file needed):
    #    python concurrent_bench.py --concurrency 128 --duration 120 --keys 100000

Output goes to ``output/concurrent_bench/``:
  - results.json     — latency percentiles, throughput, error rates
  - latency_hist.csv — raw latency samples (for external plotting)
  - summary.txt      — human-readable summary
"""

from __future__ import annotations

import argparse
import csv
import json
import math
import os
import random
import sys
import time
import threading
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor, as_completed, wait
from dataclasses import dataclass, field
from typing import Optional

# Ensure SDK importable
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'sdk', 'python'))

try:
    import psutil
    HAS_PSUTIL = True
except ImportError:
    HAS_PSUTIL = False

try:
    from mcache import Client
except ImportError:
    print("Cannot import mcache SDK. Ensure sdk/python/mcache/ is in the path.")
    sys.exit(1)


# ---------------------------------------------------------------------------
# Zipf key generator — produces realistic hot-key distributions
# ---------------------------------------------------------------------------


class ZipfKeyGen:
    """Generates keys following a Zipf (power-law) distribution.

    In real cache workloads, ~20% of keys receive ~80% of traffic.
    This generator produces such patterns.  Higher *alpha* means
    more extreme skew.

    Parameters
    ----------
    n : int
        Number of distinct keys.
    alpha : float
        Skew parameter.  1.0 = moderate, 1.5 = steep, 2.0+ = extreme.
    """

    def __init__(self, n: int, alpha: float = 1.2):
        self._n = n
        # Pre-compute CDF for O(log n) sampling via binary search
        c = [0.0] * (n + 1)
        for i in range(1, n + 1):
            c[i] = c[i - 1] + 1.0 / (i ** alpha)
        self._cdf = [v / c[n] for v in c]

    def next(self) -> int:
        """Return a key index in [0, n).  Hotter keys have smaller indices."""
        r = random.random()
        lo, hi = 0, self._n
        while lo < hi:
            mid = (lo + hi) // 2
            if self._cdf[mid + 1] < r:
                lo = mid + 1
            else:
                hi = mid
        return lo


# ---------------------------------------------------------------------------
# Data file generation and loading
# ---------------------------------------------------------------------------


def generate_data_file(path: str, num_keys: int, value_sizes: tuple = (64, 256, 1024, 4096, 65536)):
    """Generate a JSON lines data file with keys and values of varying sizes.

    Each line is a JSON object::

        {"key": "user:000001", "type": "kv", "value": "<base64>", "size": 256}
        {"key": "hash:000042", "type": "hash", "fields": {"name": "alice", "age": "30"}}
        {"key": "list:000123", "type": "list", "elements": ["a","b","c"]}
        {"key": "set:000789",  "type": "set",  "members": ["tag1","tag2"]}
    """
    import base64

    os.makedirs(os.path.dirname(path) or '.', exist_ok=True)

    type_weights = [('kv', 0.55), ('hash', 0.15), ('list', 0.15), ('set', 0.15)]
    types, weights = zip(*type_weights)

    with open(path, 'w', encoding='utf-8') as f:
        for i in range(num_keys):
            dt = random.choices(types, weights=weights)[0]

            if dt == 'kv':
                vs = random.choice(value_sizes)
                raw = os.urandom(vs)
                record = {
                    'key': f'kv:{i:06d}',
                    'type': 'kv',
                    'value': base64.b64encode(raw).decode('ascii'),
                    'size': vs,
                }
            elif dt == 'hash':
                nf = random.randint(1, 8)
                fields = {}
                for j in range(nf):
                    fields[f'f{j}'] = base64.b64encode(os.urandom(random.choice(value_sizes[:3]))).decode('ascii')[:32]
                record = {'key': f'hash:{i:06d}', 'type': 'hash', 'fields': fields}
            elif dt == 'list':
                ne = random.randint(1, 20)
                elems = [base64.b64encode(os.urandom(random.choice(value_sizes[:2]))).decode('ascii')[:16] for _ in range(ne)]
                record = {'key': f'list:{i:06d}', 'type': 'list', 'elements': elems}
            else:  # set
                nm = random.randint(1, 15)
                members = [f'tag{random.randint(0, 9999)}' for _ in range(nm)]
                record = {'key': f'set:{i:06d}', 'type': 'set', 'members': members}

            f.write(json.dumps(record) + '\n')

    print(f'Generated {num_keys} records → {path}')


def load_data_file(path: str) -> list[dict]:
    """Load records from a JSON-lines data file."""
    records = []
    with open(path, 'r', encoding='utf-8') as f:
        for line in f:
            line = line.strip()
            if line:
                records.append(json.loads(line))
    return records


# ---------------------------------------------------------------------------
# Latency collector (lock-free-ish, per-thread buckets → merge)
# ---------------------------------------------------------------------------


@dataclass
class BenchResult:
    total_ops: int = 0
    success_ops: int = 0
    error_ops: int = 0
    latency_us: list[float] = field(default_factory=list)
    errors_by_type: dict[str, int] = field(default_factory=lambda: defaultdict(int))
    duration_s: float = 0.0
    throughput_ops: float = 0.0
    cpu_avg_pct: float = 0.0
    mem_avg_mb: float = 0.0
    mem_peak_mb: float = 0.0

    def merge(self, other: BenchResult) -> None:
        self.total_ops += other.total_ops
        self.success_ops += other.success_ops
        self.error_ops += other.error_ops
        self.latency_us.extend(other.latency_us)
        for k, v in other.errors_by_type.items():
            self.errors_by_type[k] += v


def percentile(data: list[float], p: float) -> float:
    """Return the p-th percentile (0-100) of sorted data."""
    if not data:
        return 0.0
    s = sorted(data)
    k = (len(s) - 1) * p / 100.0
    f = int(k)
    c = k - f
    if f + 1 < len(s):
        return s[f] * (1 - c) + s[f + 1] * c
    return float(s[f])


# ---------------------------------------------------------------------------
# Resource sampler
# ---------------------------------------------------------------------------


class ResourceSampler:
    def __init__(self, interval: float = 0.1):
        self._interval = interval
        self._stop = threading.Event()
        self._samples: list[tuple[float, float]] = []

    def start(self) -> None:
        if not HAS_PSUTIL:
            return
        self._proc = psutil.Process()
        self._samples.clear()
        self._stop.clear()
        self._thread = threading.Thread(target=self._run, daemon=True)
        self._thread.start()

    def stop(self) -> dict:
        if not HAS_PSUTIL:
            return {'cpu_avg': 0.0, 'mem_avg_mb': 0.0, 'mem_peak_mb': 0.0}
        self._stop.set()
        if hasattr(self, '_thread'):
            self._thread.join(timeout=1)
        if not self._samples:
            return {'cpu_avg': 0.0, 'mem_avg_mb': 0.0, 'mem_peak_mb': 0.0}
        cpus = [s[0] for s in self._samples]
        mems = [s[1] for s in self._samples]
        return {
            'cpu_avg': round(sum(cpus) / len(cpus), 2),
            'mem_avg_mb': round(sum(mems) / len(mems), 2),
            'mem_peak_mb': round(max(mems), 2),
        }

    def _run(self) -> None:
        while not self._stop.wait(self._interval):
            try:
                cpu = self._proc.cpu_percent(interval=None) / psutil.cpu_count()
                mem = self._proc.memory_info().rss / (1024 * 1024)
                self._samples.append((cpu, mem))
            except Exception:
                pass


# ---------------------------------------------------------------------------
# Per-client worker
# ---------------------------------------------------------------------------


def _client_worker(
    client_id: int,
    key_gen: ZipfKeyGen,
    records: list[dict],
    num_ops: int,
    read_ratio: float,
    write_ratio: float,
    addr: str,
    bar: Optional[threading.Barrier],
) -> BenchResult:
    """Execute *num_ops* operations against the mcache server.

    Each client creates its own connection pool for maximum concurrency.
    """
    host, port_str = addr.rsplit(':', 1)
    client = Client(host=host, port=int(port_str), pool_size=2, timeout=10.0)

    result = BenchResult()
    n_records = len(records)
    latencies: list[float] = []

    # Pre-generate operation sequence for speed
    rng = random.Random(os.urandom(8))
    ops = []
    for _ in range(num_ops):
        r = rng.random()
        if r < read_ratio:
            ops.append(('read', key_gen.next()))
        elif r < read_ratio + write_ratio:
            ops.append(('write', key_gen.next()))
        else:
            ops.append(('delete', key_gen.next()))

    # Synchronise start of all clients
    if bar is not None:
        bar.wait()

    for op_type, key_idx in ops:
        rec = records[key_idx % n_records]
        key = rec['key']
        t0 = time.perf_counter()

        try:
            if op_type == 'read':
                _do_read(client, rec)
            elif op_type == 'write':
                _do_write(client, rec)
            else:
                _do_delete(client, rec)
            elapsed = (time.perf_counter() - t0) * 1_000_000
            result.success_ops += 1
            latencies.append(elapsed)
        except Exception as e:
            elapsed = (time.perf_counter() - t0) * 1_000_000
            result.error_ops += 1
            err_name = type(e).__name__
            result.errors_by_type[err_name] += 1
            latencies.append(elapsed)

        result.total_ops += 1

    result.latency_us = latencies
    client.close()
    return result


def _do_read(client: Client, rec: dict) -> None:
    dt = rec['type']
    key = rec['key']
    if dt == 'kv':
        client.get(key)
    elif dt == 'hash':
        client.hgetall(key)
    elif dt == 'list':
        client.lrange(key, 0, -1)
    elif dt == 'set':
        client.smembers(key)


def _do_write(client: Client, rec: dict) -> None:
    import base64
    dt = rec['type']
    key = rec['key']
    if dt == 'kv':
        raw = base64.b64decode(rec['value'])
        client.set(key, raw)
    elif dt == 'hash':
        fields = rec.get('fields', {})
        if fields:
            f, v = next(iter(fields.items()))
            client.hset(key, f, v)
    elif dt == 'list':
        elems = rec.get('elements', [])
        if elems:
            client.rpush(key, *elems)
    elif dt == 'set':
        members = rec.get('members', [])
        if members:
            client.sadd(key, *members)


def _do_delete(client: Client, rec: dict) -> None:
    client.delete(rec['key'])


# ---------------------------------------------------------------------------
# Main benchmark runner
# ---------------------------------------------------------------------------


def run_benchmark(
    records: list[dict],
    addr: str = '127.0.0.1:11211',
    concurrency: int = 64,
    total_ops: int = 1_000_000,
    read_ratio: float = 0.80,
    write_ratio: float = 0.15,
    zipf_alpha: float = 1.2,
    output_dir: str = 'output/concurrent_bench',
) -> BenchResult:
    """Run the concurrent benchmark and return aggregated results."""

    os.makedirs(output_dir, exist_ok=True)
    n_keys = len(records)
    if n_keys == 0:
        raise ValueError('No records loaded — generate a data file first with --gen-data')

    print(f'{"=" * 60}')
    print(f'mcache Concurrent Load Simulator')
    print(f'{"=" * 60}')
    print(f'  Server:       {addr}')
    print(f'  Records:      {n_keys:,}')
    print(f'  Concurrency:  {concurrency}')
    print(f'  Total ops:    {total_ops:,}')
    print(f'  Read/Write/Del: {read_ratio:.0%}/{write_ratio:.0%}/{1-read_ratio-write_ratio:.0%}')
    print(f'  Zipf alpha:   {zipf_alpha}')
    print(f'  Output:       {output_dir}')
    print()

    # Verify connectivity
    host, port_str = addr.rsplit(':', 1)
    try:
        probe = Client(host=host, port=int(port_str), pool_size=1, timeout=5.0)
        probe.set('__probe__', b'1')
        probe.close()
        print('  [OK] Server reachable')
    except Exception as e:
        print(f'  [FAIL] Cannot reach server: {e}')
        print('  Start the server first: ./bin/mcache server --config config.local.yaml')
        sys.exit(1)

    key_gen = ZipfKeyGen(n_keys, alpha=zipf_alpha)
    ops_per_client = total_ops // concurrency
    sampler = ResourceSampler(interval=0.05)

    bar = threading.Barrier(concurrency) if concurrency > 1 else None

    print(f'  Launching {concurrency} clients ({ops_per_client:,} ops each) ...')
    sampler.start()
    t0 = time.perf_counter()

    aggregated = BenchResult()
    with ThreadPoolExecutor(max_workers=concurrency) as pool:
        futures = []
        for cid in range(concurrency):
            f = pool.submit(
                _client_worker,
                cid, key_gen, records, ops_per_client,
                read_ratio, write_ratio, addr, bar,
            )
            futures.append(f)

        done = 0
        for f in as_completed(futures):
            r = f.result()
            aggregated.merge(r)
            done += 1
            elapsed = time.perf_counter() - t0
            rate = aggregated.total_ops / elapsed if elapsed > 0 else 0
            print(f'\r  Progress: {done}/{concurrency} clients done  '
                  f'{aggregated.total_ops:,} ops  {rate:,.0f} ops/s  '
                  f'errors={aggregated.error_ops}', end='', flush=True)

    elapsed = time.perf_counter() - t0
    res_info = sampler.stop()
    aggregated.duration_s = round(elapsed, 3)
    aggregated.throughput_ops = round(aggregated.total_ops / elapsed, 1) if elapsed > 0 else 0
    aggregated.cpu_avg_pct = res_info['cpu_avg']
    aggregated.mem_avg_mb = res_info['mem_avg_mb']
    aggregated.mem_peak_mb = res_info['mem_peak_mb']

    print()
    print(f'  Done in {elapsed:.1f}s')
    return aggregated


# ---------------------------------------------------------------------------
# Report generation
# ---------------------------------------------------------------------------


def print_summary(result: BenchResult) -> None:
    lats = result.latency_us
    success_lats = [l for l in lats if l > 0]  # filter out zero-latency errors
    if not success_lats:
        success_lats = lats

    print(f'\n{"=" * 60}')
    print(f'RESULTS')
    print(f'{"=" * 60}')
    print(f'  Total ops:     {result.total_ops:>12,}')
    print(f'  Successful:    {result.success_ops:>12,}  ({result.success_ops/max(result.total_ops,1)*100:.2f}%)')
    print(f'  Errors:        {result.error_ops:>12,}')
    print(f'  Duration:      {result.duration_s:>12.2f}s')
    print(f'  Throughput:    {result.throughput_ops:>12,.0f} ops/s')
    print()
    print(f'  Latency (us):')
    print(f'    min:         {min(success_lats):>12,.1f}')
    print(f'    avg:         {sum(success_lats)/len(success_lats):>12,.1f}')
    print(f'    p50:         {percentile(success_lats, 50):>12,.1f}')
    print(f'    p95:         {percentile(success_lats, 95):>12,.1f}')
    print(f'    p99:         {percentile(success_lats, 99):>12,.1f}')
    print(f'    p999:        {percentile(success_lats, 99.9):>12,.1f}')
    print(f'    max:         {max(success_lats):>12,.1f}')
    print()
    if HAS_PSUTIL:
        print(f'  CPU avg:       {result.cpu_avg_pct:>12.2f}%')
        print(f'  Mem avg:       {result.mem_avg_mb:>12.1f} MB')
        print(f'  Mem peak:      {result.mem_peak_mb:>12.1f} MB')
        print()

    if result.errors_by_type:
        print(f'  Errors by type:')
        for err_name, count in sorted(result.errors_by_type.items(), key=lambda x: -x[1]):
            print(f'    {err_name:<30s} {count:>8,}')
        print()

    # Latency histogram (log-scale buckets)
    print(f'  Latency distribution:')
    buckets = [
        (0, 100), (100, 200), (200, 500), (500, 1000),
        (1000, 2000), (2000, 5000), (5000, 10000),
        (10000, 50000), (50000, 100000), (100000, float('inf')),
    ]
    for lo, hi in buckets:
        count = sum(1 for l in success_lats if lo <= l < hi)
        pct = count / len(success_lats) * 100 if success_lats else 0
        bar = '█' * int(pct * 2) if pct > 0 else ''
        label = f'{lo/1000:.0f}-{hi/1000:.0f}ms' if hi < 100_000 else f'{lo/1000:.0f}ms+'
        print(f'    {label:<12s} {count:>8,}  ({pct:5.1f}%)  {bar}')
    print()


def save_results(result: BenchResult, output_dir: str) -> None:
    os.makedirs(output_dir, exist_ok=True)

    lats = result.latency_us
    success_lats = [l for l in lats if l > 0] or lats

    # JSON summary
    summary = {
        'total_ops': result.total_ops,
        'success_ops': result.success_ops,
        'error_ops': result.error_ops,
        'duration_s': result.duration_s,
        'throughput_ops': result.throughput_ops,
        'latency_us': {
            'min': round(min(success_lats), 1),
            'avg': round(sum(success_lats) / len(success_lats), 1),
            'p50': round(percentile(success_lats, 50), 1),
            'p95': round(percentile(success_lats, 95), 1),
            'p99': round(percentile(success_lats, 99), 1),
            'p999': round(percentile(success_lats, 99.9), 1),
            'max': round(max(success_lats), 1),
        },
        'cpu_avg_pct': result.cpu_avg_pct,
        'mem_avg_mb': result.mem_avg_mb,
        'mem_peak_mb': result.mem_peak_mb,
        'errors_by_type': dict(result.errors_by_type),
    }
    json_path = os.path.join(output_dir, 'results.json')
    with open(json_path, 'w') as f:
        json.dump(summary, f, indent=2)
    print(f'  Results → {json_path}')

    # Raw latency CSV
    csv_path = os.path.join(output_dir, 'latency_hist.csv')
    with open(csv_path, 'w', newline='', encoding='utf-8') as f:
        writer = csv.writer(f)
        writer.writerow(['latency_us'])
        for l in success_lats:
            writer.writerow([round(l, 1)])
    print(f'  Latency samples → {csv_path} ({len(success_lats):,} rows)')

    # Human-readable summary
    txt_path = os.path.join(output_dir, 'summary.txt')
    with open(txt_path, 'w', encoding='utf-8') as f:
        import sys as _sys
        old_stdout = _sys.stdout
        _sys.stdout = f
        print_summary(result)
        _sys.stdout = old_stdout
    print(f'  Summary → {txt_path}')


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def main() -> None:
    p = argparse.ArgumentParser(
        description='mcache concurrent load simulator — realistic cache workload benchmark',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Generate a 50K-record data file
  python concurrent_bench.py --gen-data data.json --keys 50000

  # Run with 128 concurrent clients for 60s using the data file
  python concurrent_bench.py --data data.json --concurrency 128 --duration 60

  # Run with built-in Zipf workload (no data file needed)
  python concurrent_bench.py --concurrency 64 --duration 30 --keys 20000

  # Extreme concurrency test
  python concurrent_bench.py --concurrency 512 --duration 120 --keys 200000 --read-ratio 0.95
        """,
    )
    p.add_argument('--addr', default='127.0.0.1:11211',
                   help='mcache server address (default: 127.0.0.1:11211)')
    p.add_argument('--concurrency', '-c', type=int, default=64,
                   help='Number of concurrent clients (default: 64)')
    p.add_argument('--duration', '-d', type=float, default=60.0,
                   help='Target benchmark duration in seconds (default: 60)')
    p.add_argument('--ops', type=int, default=0,
                   help='Exact total operation count (overrides --duration)')
    p.add_argument('--keys', '-k', type=int, default=100_000,
                   help='Number of distinct keys (default: 100000)')
    p.add_argument('--read-ratio', type=float, default=0.80,
                   help='Fraction of reads (default: 0.80)')
    p.add_argument('--write-ratio', type=float, default=0.15,
                   help='Fraction of writes (default: 0.15)')
    p.add_argument('--zipf-alpha', type=float, default=1.2,
                   help='Zipf skew parameter — higher = hotter keys (default: 1.2)')
    p.add_argument('--data', default='',
                   help='Path to JSON-lines data file (generated with --gen-data)')
    p.add_argument('--gen-data', default='',
                   help='Generate a data file at this path and exit')
    p.add_argument('--output', '-o', default='output/concurrent_bench',
                   help='Output directory (default: output/concurrent_bench)')

    args = p.parse_args()

    # Generate data file mode
    if args.gen_data:
        generate_data_file(args.gen_data, args.keys)
        return

    # Load data
    if args.data:
        if not os.path.exists(args.data):
            print(f'Data file not found: {args.data}')
            print(f'Generate one first: python {sys.argv[0]} --gen-data {args.data} --keys {args.keys}')
            sys.exit(1)
        records = load_data_file(args.data)
        print(f'Loaded {len(records):,} records from {args.data}')
    else:
        # Build synthetic records in-memory
        print(f'Building {args.keys:,} synthetic records in memory ...')
        import base64
        records = []
        for i in range(args.keys):
            vs = random.choice([64, 256, 1024, 4096])
            records.append({
                'key': f'kv:{i:08d}',
                'type': 'kv',
                'value': base64.b64encode(os.urandom(vs)).decode('ascii'),
                'size': vs,
            })
        print(f'  Done.')

    # Determine total ops
    if args.ops > 0:
        total_ops = args.ops
    else:
        # Estimate: we want ~duration seconds.  Each op is ~200us → ~5000 ops/s/client
        # This is a rough estimate; actual throughput depends on server performance.
        est_rate_per_client = 4000  # conservative ops/s per client
        total_ops = int(args.duration * est_rate_per_client * args.concurrency)

    # Validate ratios
    if args.read_ratio + args.write_ratio > 1.0:
        print('Error: read_ratio + write_ratio must not exceed 1.0')
        sys.exit(1)

    # Run
    result = run_benchmark(
        records=records,
        addr=args.addr,
        concurrency=args.concurrency,
        total_ops=total_ops,
        read_ratio=args.read_ratio,
        write_ratio=args.write_ratio,
        zipf_alpha=args.zipf_alpha,
        output_dir=args.output,
    )

    print_summary(result)
    save_results(result, args.output)


if __name__ == '__main__':
    main()
