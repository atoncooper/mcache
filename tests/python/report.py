"""Generate comparison charts and a paper-ready summary report."""

from __future__ import annotations

import json
import os
from collections import defaultdict

_MPL_AVAILABLE = False
try:
    import matplotlib
    matplotlib.use('Agg')
    import matplotlib.pyplot as plt
    _MPL_AVAILABLE = True
except ImportError:
    pass


def generate_report(results: list[dict], output_dir: str) -> None:
    """Generate charts + markdown report from benchmark results.

    *results*: list of dicts as produced by ``run_comparison()``.
    """
    os.makedirs(os.path.join(output_dir, 'charts'), exist_ok=True)

    # Filter out error rows
    valid = [r for r in results if 'throughput_ops' in r]

    if not valid:
        print('No valid results to report.')
        return

    # Aggregate: for each (workload, backend), take median of runs
    groups = defaultdict(list)
    for r in valid:
        groups[(r['workload'], r['backend'])].append(r)

    agg = []
    for (wl, be), runs in groups.items():
        best = max(runs, key=lambda r: r['throughput_ops'])
        tputs = [r['throughput_ops'] for r in runs]
        agg.append({
            'workload': wl,
            'backend': be,
            'description': runs[0].get('description', ''),
            'runs': len(runs),
            'throughput_median': round(sorted(tputs)[len(tputs) // 2], 1),
            'throughput_best': round(max(tputs), 1),
            'p50_us': best['p50_us'],
            'p95_us': best['p95_us'],
            'p99_us': best['p99_us'],
            'avg_us': best['avg_us'],
            'cpu_avg_pct': best['cpu_avg_pct'],
            'mem_peak_mb': best['mem_peak_mb'],
        })

    # Comparison pairs
    pairs = defaultdict(dict)
    for a in agg:
        pairs[a['workload']][a['backend']] = a

    # ---- Charts ----
    if _MPL_AVAILABLE:
        _chart_throughput_bars(pairs, output_dir)
        _chart_latency_bars(pairs, output_dir)
        _chart_resource_bars(pairs, output_dir)

    # ---- Markdown report ----
    _write_report(pairs, agg, output_dir)


# ---------------------------------------------------------------------------
# Charts
# ---------------------------------------------------------------------------


def _chart_throughput_bars(pairs: dict, output_dir: str) -> None:
    workloads = sorted(pairs.keys())
    n = len(workloads)
    x = list(range(n))
    w = 0.35

    m_vals = [pairs[wl].get('mcache', {}).get('throughput_best', 0) for wl in workloads]
    r_vals = [pairs[wl].get('redis', {}).get('throughput_best', 0) for wl in workloads]

    fig, ax = plt.subplots(figsize=(max(10, n * 1.2), 6))
    bars1 = ax.bar(x - w / 2, m_vals, w, label='mcache', color='#2196F3', edgecolor='white')
    bars2 = ax.bar(x + w / 2, r_vals, w, label='Redis', color='#FF5722', edgecolor='white')

    # Speedup labels
    for i, (mv, rv) in enumerate(zip(m_vals, r_vals)):
        if rv > 0 and mv > 0:
            ratio = mv / rv
            y = max(mv, rv)
            ax.text(i, y + y * 0.02, f'{ratio:.1f}x', ha='center', fontsize=9, fontweight='bold',
                    color='#4CAF50' if ratio >= 1 else '#F44336')

    ax.set_xlabel('Workload')
    ax.set_ylabel('Throughput (ops/s)')
    ax.set_title('Throughput: mcache vs Redis')
    ax.set_xticks(x)
    ax.set_xticklabels(workloads, rotation=30, ha='right', fontsize=8)
    ax.legend()
    ax.grid(axis='y', alpha=0.3)
    fig.tight_layout()
    fig.savefig(os.path.join(output_dir, 'charts', 'throughput.png'), dpi=150)
    plt.close(fig)


def _chart_latency_bars(pairs: dict, output_dir: str) -> None:
    workloads = sorted(pairs.keys())
    n = len(workloads)
    x = list(range(n))
    w = 0.2

    fig, ax = plt.subplots(figsize=(max(10, n * 1.2), 6))
    for idx, (label, field, color) in enumerate([
        ('p50', 'p50_us', '#4CAF50'),
        ('p95', 'p95_us', '#FF9800'),
        ('p99', 'p99_us', '#F44336'),
    ]):
        m_vals = [pairs[wl].get('mcache', {}).get(field, 0) for wl in workloads]
        offset = (idx - 1) * w
        ax.bar(x + offset, m_vals, w / 2, label=f'mcache {label}', color=color, alpha=0.8,
               edgecolor='white')
        r_vals = [pairs[wl].get('redis', {}).get(field, 0) for wl in workloads]
        ax.bar(x + offset + w / 2, r_vals, w / 2, label=f'redis {label}', color=color, alpha=0.4,
               edgecolor='white', hatch='//')

    ax.set_xlabel('Workload')
    ax.set_ylabel('Latency (us)')
    ax.set_title('Latency Distribution: mcache vs Redis')
    ax.set_xticks(x)
    ax.set_xticklabels(workloads, rotation=30, ha='right', fontsize=8)
    ax.legend(ncol=3, fontsize=7)
    ax.grid(axis='y', alpha=0.3)
    fig.tight_layout()
    fig.savefig(os.path.join(output_dir, 'charts', 'latency.png'), dpi=150)
    plt.close(fig)


def _chart_resource_bars(pairs: dict, output_dir: str) -> None:
    workloads = sorted(pairs.keys())
    n = len(workloads)
    x = list(range(n))
    w = 0.35

    # CPU
    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(max(12, n * 1.5), 5))

    m_cpu = [pairs[wl].get('mcache', {}).get('cpu_avg_pct', 0) for wl in workloads]
    r_cpu = [pairs[wl].get('redis', {}).get('cpu_avg_pct', 0) for wl in workloads]
    ax1.bar(x - w / 2, m_cpu, w, label='mcache', color='#2196F3')
    ax1.bar(x + w / 2, r_cpu, w, label='Redis', color='#FF5722')
    ax1.set_title('CPU Usage (%)')
    ax1.set_xticks(x)
    ax1.set_xticklabels(workloads, rotation=30, ha='right', fontsize=7)
    ax1.legend()
    ax1.grid(axis='y', alpha=0.3)

    # Memory
    m_mem = [pairs[wl].get('mcache', {}).get('mem_peak_mb', 0) for wl in workloads]
    r_mem = [pairs[wl].get('redis', {}).get('mem_peak_mb', 0) for wl in workloads]
    ax2.bar(x - w / 2, m_mem, w, label='mcache', color='#2196F3')
    ax2.bar(x + w / 2, r_mem, w, label='Redis', color='#FF5722')
    ax2.set_title('Peak Memory (MB)')
    ax2.set_xticks(x)
    ax2.set_xticklabels(workloads, rotation=30, ha='right', fontsize=7)
    ax2.legend()
    ax2.grid(axis='y', alpha=0.3)

    fig.suptitle('Resource Usage: mcache vs Redis')
    fig.tight_layout()
    fig.savefig(os.path.join(output_dir, 'charts', 'resources.png'), dpi=150)
    plt.close(fig)


# ---------------------------------------------------------------------------
# Report
# ---------------------------------------------------------------------------


def _write_report(pairs: dict, agg: list[dict], output_dir: str) -> None:
    lines = []
    lines.append('# mcache vs Redis — Benchmark Report')
    lines.append('')
    lines.append('## Summary')
    lines.append('')

    # Overall speedup
    m_tputs = [a['throughput_best'] for a in agg if a['backend'] == 'mcache']
    r_tputs = [a['throughput_best'] for a in agg if a['backend'] == 'redis']
    if m_tputs and r_tputs:
        avg_speedup = (sum(m_tputs) / len(m_tputs)) / (sum(r_tputs) / len(r_tputs))
        lines.append(f'- **Average throughput ratio:** mcache is **{avg_speedup:.1f}x** Redis')
    lines.append('')

    # ---- Throughput table ----
    lines.append('## Throughput')
    lines.append('')
    lines.append('| Workload | Description | mcache (ops/s) | Redis (ops/s) | Ratio |')
    lines.append('|----------|-------------|----------------|---------------|-------|')

    for wl_name in sorted(pairs.keys()):
        m = pairs[wl_name].get('mcache', {})
        r = pairs[wl_name].get('redis', {})
        desc = m.get('description', '')
        mt = m.get('throughput_best', 0)
        rt = r.get('throughput_best', 0)
        ratio = f'{mt / rt:.1f}x' if rt > 0 and mt > 0 else '-'
        lines.append(f'| {wl_name} | {desc} | {mt:,.0f} | {rt:,.0f} | {ratio} |')

    lines.append('')

    # ---- Latency table ----
    lines.append('## Latency (best run)')
    lines.append('')
    for be_name, be_label in [('mcache', 'mcache'), ('redis', 'Redis')]:
        lines.append(f'### {be_label}')
        lines.append('')
        lines.append('| Workload | p50 (us) | p95 (us) | p99 (us) | Avg (us) |')
        lines.append('|----------|----------|----------|----------|----------|')
        for wl_name in sorted(pairs.keys()):
            b = pairs[wl_name].get(be_name, {})
            lines.append(f'| {wl_name} | {b.get("p50_us", "-")} | {b.get("p95_us", "-")} | '
                         f'{b.get("p99_us", "-")} | {b.get("avg_us", "-")} |')
        lines.append('')

    # ---- Resource table ----
    lines.append('## Resource Usage (best run)')
    lines.append('')
    lines.append('| Workload | mcache CPU% | Redis CPU% | mcache Mem MB | Redis Mem MB |')
    lines.append('|----------|-------------|------------|---------------|--------------|')
    for wl_name in sorted(pairs.keys()):
        m = pairs[wl_name].get('mcache', {})
        r = pairs[wl_name].get('redis', {})
        lines.append(f'| {wl_name} | {m.get("cpu_avg_pct", "-")} | {r.get("cpu_avg_pct", "-")} | '
                     f'{m.get("mem_peak_mb", "-")} | {r.get("mem_peak_mb", "-")} |')
    lines.append('')

    # ---- Charts section ----
    lines.append('## Charts')
    lines.append('')
    lines.append('![Throughput](charts/throughput.png)')
    lines.append('')
    lines.append('![Latency](charts/latency.png)')
    lines.append('')
    lines.append('![Resources](charts/resources.png)')
    lines.append('')

    with open(os.path.join(output_dir, 'report.md'), 'w', encoding='utf-8') as f:
        f.write('\n'.join(lines))
