"""Benchmark harness — runs identical workloads against mcache and Redis.

Design:
  - ``Driver`` abstracts a cache backend (mcache / redis).
  - ``Workload`` defines an operation pattern (KV / hash / list / mixed).
  - ``Benchmark`` wires everything together, collects latency + metrics + results.
"""

from __future__ import annotations

import csv
import json
import os
import time
import threading
import queue
from abc import ABC, abstractmethod
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
from typing import Optional

try:
    import numpy as np
except ImportError:
    np = None

# ---------------------------------------------------------------------------
# Data types
# ---------------------------------------------------------------------------


@dataclass
class OpResult:
    """Result of a single operation."""
    op: str          # 'get', 'set', 'hset', 'lpush', ...
    latency_us: float
    success: bool
    error: str = ''


@dataclass
class RunResult:
    """Aggregated results from one benchmark run."""
    name: str
    backend: str       # 'mcache' or 'redis'
    total_ops: int
    duration_s: float
    throughput_ops: float
    latency_p50_us: float
    latency_p95_us: float
    latency_p99_us: float
    latency_avg_us: float
    latency_min_us: float
    latency_max_us: float
    success_rate: float
    cpu_avg_pct: float
    mem_avg_mb: float
    mem_peak_mb: float
    raw: list[OpResult] = field(default_factory=list)


# ---------------------------------------------------------------------------
# Driver interface
# ---------------------------------------------------------------------------


class Driver(ABC):
    """Abstract cache backend."""

    name: str = 'abstract'

    @abstractmethod
    def connect(self) -> None: ...

    @abstractmethod
    def get(self, key: str) -> Optional[bytes]: ...

    @abstractmethod
    def set(self, key: str, value: bytes, ttl: int = 0) -> bool: ...

    @abstractmethod
    def delete(self, key: str) -> bool: ...

    @abstractmethod
    def close(self) -> None: ...

    def ping(self) -> bool:
        try: self.set('__ping__', b'1'); return True
        except: return False


class McacheDriver(Driver):
    name = 'mcache'

    def __init__(self, host: str = '127.0.0.1', port: int = 11211):
        self._host = host
        self._port = port
        self._client = None

    def connect(self) -> None:
        import sys
        sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'sdk', 'python'))
        from mcache import Client
        self._client = Client(host=self._host, port=self._port, pool_size=4, timeout=10.0)

    def get(self, key: str) -> Optional[bytes]:
        return self._client.get(key)

    def set(self, key: str, value: bytes, ttl: int = 0) -> bool:
        return self._client.set(key, value, ttl)

    def delete(self, key: str) -> bool:
        return self._client.delete(key)

    def close(self) -> None:
        if self._client:
            self._client.close()


class RedisDriver(Driver):
    name = 'redis'

    def __init__(self, host: str = '127.0.0.1', port: int = 6379, db: int = 0):
        self._host = host
        self._port = port
        self._db = db
        self._client = None

    def connect(self) -> None:
        import redis
        self._client = redis.Redis(host=self._host, port=self._port, db=self._db,
                                   socket_timeout=10, socket_connect_timeout=10,
                                   decode_responses=False)

    def get(self, key: str) -> Optional[bytes]:
        return self._client.get(key)

    def set(self, key: str, value: bytes, ttl: int = 0) -> bool:
        if ttl > 0:
            return self._client.set(key, value, ex=ttl) is True
        return self._client.set(key, value) is True

    def delete(self, key: str) -> bool:
        return self._client.delete(key) > 0

    def close(self) -> None:
        if self._client:
            self._client.close()


# ---------------------------------------------------------------------------
# Workload
# ---------------------------------------------------------------------------


@dataclass
class Workload:
    """Defines a benchmark scenario."""
    name: str
    description: str = ''
    # Warmup
    warmup_ops: int = 1000
    # Main run
    total_ops: int = 100_000
    # Concurrency
    num_clients: int = 4
    # Key space
    key_count: int = 10_000
    # Value size
    value_size: int = 128
    # Operation mix (must sum to 1.0)
    read_ratio: float = 0.5
    write_ratio: float = 0.5
    delete_ratio: float = 0.0
    # Whether to prepopulate before benchmark
    prepopulate: bool = True


# ---------------------------------------------------------------------------
# Resource sampler
# ---------------------------------------------------------------------------


class ResourceSampler:
    """Samples CPU and memory in a background thread during a benchmark."""

    def __init__(self, interval: float = 0.1):
        self._interval = interval
        self._stop = threading.Event()
        self._thread: Optional[threading.Thread] = None
        self._samples: list[tuple[float, float, float]] = []  # (ts, cpu_pct, mem_mb)
        self._proc = None

    def start(self) -> None:
        import psutil
        self._proc = psutil.Process()
        self._samples.clear()
        self._stop.clear()
        self._thread = threading.Thread(target=self._run, daemon=True)
        self._thread.start()

    def stop(self) -> dict:
        self._stop.set()
        if self._thread:
            self._thread.join(timeout=1)
        if not self._samples:
            return {'cpu_avg': 0.0, 'mem_avg_mb': 0.0, 'mem_peak_mb': 0.0}
        cpus = [s[1] for s in self._samples]
        mems = [s[2] for s in self._samples]
        return {
            'cpu_avg': round(sum(cpus) / len(cpus), 2),
            'mem_avg_mb': round(sum(mems) / len(mems), 2),
            'mem_peak_mb': round(max(mems), 2),
        }

    def _run(self) -> None:
        import psutil
        while not self._stop.wait(self._interval):
            try:
                cpu = self._proc.cpu_percent(interval=None) / psutil.cpu_count()
                mem = self._proc.memory_info().rss / (1024 * 1024)
                self._samples.append((time.time(), cpu, mem))
            except Exception:
                pass


# ---------------------------------------------------------------------------
# Benchmark engine
# ---------------------------------------------------------------------------


class Benchmark:
    """Runs a workload against a driver and collects results."""

    def __init__(self, driver: Driver, workload: Workload,
                 output_dir: str = 'output'):
        self.driver = driver
        self.workload = workload
        self.output_dir = output_dir
        self.sampler = ResourceSampler(interval=0.05)
        self._results: queue.Queue[OpResult] = queue.Queue()

    def run(self) -> RunResult:
        w = self.workload
        driver = self.driver

        # -- prepopulate --
        if w.prepopulate:
            v = b'x' * w.value_size
            for i in range(w.key_count):
                driver.set(f'__bench__{i}', v)

        # -- warmup --
        for i in range(w.warmup_ops):
            k = f'__bench__{i % w.key_count}'
            driver.get(k) if i % 2 == 0 else driver.set(k, b'w' * w.value_size)

        # -- main run --
        self.sampler.start()
        t0 = time.perf_counter()

        with ThreadPoolExecutor(max_workers=w.num_clients) as pool:
            futures = []
            ops_per_client = w.total_ops // w.num_clients
            for _ in range(w.num_clients):
                futures.append(pool.submit(self._client_worker, ops_per_client))

            for f in as_completed(futures):
                f.result()  # propagate exceptions

        elapsed = time.perf_counter() - t0
        res_info = self.sampler.stop()

        # -- collect latencies --
        results: list[OpResult] = []
        while True:
            try:
                results.append(self._results.get_nowait())
            except queue.Empty:
                break

        if not results:
            return RunResult(name=w.name, backend=driver.name,
                             total_ops=0, duration_s=elapsed, throughput_ops=0,
                             latency_p50_us=0, latency_p95_us=0, latency_p99_us=0,
                             latency_avg_us=0, latency_min_us=0, latency_max_us=0,
                             success_rate=0, cpu_avg_pct=0, mem_avg_mb=0, mem_peak_mb=0)

        lats = sorted([r.latency_us for r in results if r.success])
        total = len(results)
        ok = sum(1 for r in results if r.success)

        def _pct(data, p):
            if not data:
                return 0.0
            k = (len(data) - 1) * p / 100.0
            f = int(k)
            c = k - f
            if f + 1 < len(data):
                return data[f] * (1 - c) + data[f + 1] * c
            return float(data[f])

        avg_lat = sum(lats) / len(lats) if lats else 0.0

        return RunResult(
            name=w.name,
            backend=driver.name,
            total_ops=total,
            duration_s=round(elapsed, 3),
            throughput_ops=round(total / elapsed, 1),
            latency_p50_us=round(_pct(lats, 50), 1),
            latency_p95_us=round(_pct(lats, 95), 1),
            latency_p99_us=round(_pct(lats, 99), 1),
            latency_avg_us=round(avg_lat, 1),
            latency_min_us=round(lats[0], 1) if lats else 0,
            latency_max_us=round(lats[-1], 1) if lats else 0,
            success_rate=round(ok / total * 100, 2) if total > 0 else 0,
            cpu_avg_pct=res_info['cpu_avg'],
            mem_avg_mb=res_info['mem_avg_mb'],
            mem_peak_mb=res_info['mem_peak_mb'],
            raw=results,
        )

    def _client_worker(self, n: int) -> None:
        """Run n operations, randomly mixing reads/writes/deletes per workload ratios."""
        import random
        w = self.workload
        v = b'x' * w.value_size

        for _ in range(n):
            k = f'__bench__{random.randint(0, w.key_count - 1)}'
            r = random.random()
            t0 = time.perf_counter()

            try:
                if r < w.read_ratio:
                    self.driver.get(k)
                elif r < w.read_ratio + w.write_ratio:
                    self.driver.set(k, v)
                else:
                    self.driver.delete(k)
                elapsed = (time.perf_counter() - t0) * 1_000_000
                self._results.put(OpResult(op='mixed', latency_us=elapsed, success=True))
            except Exception as e:
                elapsed = (time.perf_counter() - t0) * 1_000_000
                self._results.put(OpResult(op='mixed', latency_us=elapsed, success=False, error=str(e)))


# ---------------------------------------------------------------------------
# Multi-scenario runner
# ---------------------------------------------------------------------------


def run_comparison(workloads: list[Workload],
                   mcache_addr: str = '127.0.0.1:11211',
                   redis_addr: str = '127.0.0.1:6379',
                   output_dir: str = 'output',
                   runs: int = 3) -> list[dict]:
    """Run all workloads against both backends and return result rows."""
    os.makedirs(output_dir, exist_ok=True)

    mcache_host, mcache_port_str = mcache_addr.rsplit(':', 1)
    redis_host, redis_port_str = redis_addr.rsplit(':', 1)
    mcache_port = int(mcache_port_str)
    redis_port = int(redis_port_str)

    all_results: list[dict] = []

    for wl in workloads:
        for backend_cls, host, port in [
            (McacheDriver, mcache_host, mcache_port),
            (RedisDriver, redis_host, redis_port),
        ]:
            for run_idx in range(runs):
                label = f'{wl.name}_{backend_cls.name}_run{run_idx+1}'
                print(f'  Running {label} ...', end=' ', flush=True)
                driver = backend_cls(host=host, port=port)
                try:
                    driver.connect()
                    bench = Benchmark(driver, wl, output_dir)
                    result = bench.run()
                    row = {
                        'workload': wl.name,
                        'backend': backend_cls.name,
                        'run': run_idx + 1,
                        'description': wl.description,
                        'keys': wl.key_count,
                        'value_bytes': wl.value_size,
                        'clients': wl.num_clients,
                        'read_ratio': wl.read_ratio,
                        'write_ratio': wl.write_ratio,
                        'total_ops': result.total_ops,
                        'duration_s': result.duration_s,
                        'throughput_ops': result.throughput_ops,
                        'p50_us': result.latency_p50_us,
                        'p95_us': result.latency_p95_us,
                        'p99_us': result.latency_p99_us,
                        'avg_us': result.latency_avg_us,
                        'min_us': result.latency_min_us,
                        'max_us': result.latency_max_us,
                        'success_pct': result.success_rate,
                        'cpu_avg_pct': result.cpu_avg_pct,
                        'mem_avg_mb': result.mem_avg_mb,
                        'mem_peak_mb': result.mem_peak_mb,
                    }
                    all_results.append(row)
                    print(f'{result.throughput_ops:.0f} ops/s  p50={result.latency_p50_us:.0f}us')
                except Exception as e:
                    print(f'FAILED: {e}')
                    all_results.append({'workload': wl.name, 'backend': backend_cls.name,
                                        'run': run_idx + 1, 'error': str(e)})
                finally:
                    driver.close()

    # -- save CSV --
    if all_results:
        csv_path = os.path.join(output_dir, 'results.csv')
        keys = all_results[0].keys()
        with open(csv_path, 'w', newline='', encoding='utf-8') as f:
            writer = csv.DictWriter(f, fieldnames=keys)
            writer.writeheader()
            writer.writerows(all_results)
        print(f'\nResults saved to {csv_path}')

    # -- save JSON --
    json_path = os.path.join(output_dir, 'results.json')
    with open(json_path, 'w') as f:
        json.dump(all_results, f, indent=2)

    return all_results
