#!/usr/bin/env python3
"""mcache 服务器连通性检测脚本。

用法::

    # 单次 ping
    python ping.py

    # 指定地址
    python ping.py --host 127.0.0.1 --port 11211

    # 持续 ping（类似系统 ping）
    python ping.py -c 10 -i 1.0

    # 一直 ping 直到 Ctrl+C
    python ping.py -c 0
"""

from __future__ import annotations

import argparse
import os
import signal
import sys
import time
from dataclasses import dataclass

# 把项目 SDK 路径加入到 sys.path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'sdk', 'python'))

from mcache import Client
from mcache.errors import McacheError, ConnectionError as McacheConnectionError


@dataclass
class PingStats:
    sent: int = 0
    received: int = 0
    rtts_ms: list[float] = None

    def __post_init__(self):
        if self.rtts_ms is None:
            self.rtts_ms = []

    @property
    def loss_pct(self) -> float:
        if self.sent == 0:
            return 0.0
        return (self.sent - self.received) / self.sent * 100.0


def ping_once(client: Client) -> float | None:
    """单次 ping，返回 RTT（毫秒），失败返回 None。"""
    t0 = time.perf_counter()
    try:
        ok = client.ping()
    except (McacheError, McacheConnectionError, OSError):
        return None
    if not ok:
        return None
    return (time.perf_counter() - t0) * 1000.0


def print_summary(host: str, port: int, stats: PingStats) -> None:
    print()
    print(f"--- {host}:{port} ping 统计 ---")
    print(f"{stats.sent} packets transmitted, "
          f"{stats.received} received, "
          f"{stats.loss_pct:.1f}% packet loss")
    if stats.rtts_ms:
        rtts = sorted(stats.rtts_ms)
        rtt_min = rtts[0]
        rtt_max = rtts[-1]
        rtt_avg = sum(rtts) / len(rtts)
        rtt_mdev = (sum((x - rtt_avg) ** 2 for x in rtts) / len(rtts)) ** 0.5
        print(f"rtt min/avg/max/mdev = "
              f"{rtt_min:.3f}/{rtt_avg:.3f}/{rtt_max:.3f}/{rtt_mdev:.3f} ms")


def main() -> int:
    parser = argparse.ArgumentParser(
        description='检测 mcache 服务器连通性',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  python ping.py
  python ping.py --host 10.0.0.1 --port 11211
  python ping.py -c 100 -i 0.5
  python ping.py -c 0          # 持续 ping，Ctrl+C 退出
""",
    )
    parser.add_argument('--host', '-H', default='127.0.0.1', help='mcache 服务器地址 (default: 127.0.0.1)')
    parser.add_argument('--port', '-p', type=int, default=11211, help='mcache 端口 (default: 11211)')
    parser.add_argument('--count', '-c', type=int, default=4,
                        help='ping 次数，0 表示持续 (default: 4)')
    parser.add_argument('--interval', '-i', type=float, default=1.0,
                        help='ping 间隔（秒）(default: 1.0)')
    parser.add_argument('--timeout', '-t', type=float, default=3.0,
                        help='单次 ping 超时（秒）(default: 3.0)')
    parser.add_argument('--quiet', '-q', action='store_true', help='只输出汇总')
    args = parser.parse_args()

    # 尝试连接
    try:
        client = Client(host=args.host, port=args.port, pool_size=1, timeout=args.timeout)
    except (McacheConnectionError, McacheError, OSError) as e:
        print(f"PING {args.host}:{args.port} 失败：无法连接 ({e})", file=sys.stderr)
        return 2

    print(f"PING {args.host}:{args.port}")

    stats = PingStats()
    interrupted = False

    def handle_sigint(_sig, _frame):
        nonlocal interrupted
        interrupted = True

    signal.signal(signal.SIGINT, handle_sigint)

    try:
        i = 0
        while True:
            if args.count > 0 and i >= args.count:
                break
            if interrupted:
                break

            stats.sent += 1
            rtt = ping_once(client)
            if rtt is None:
                if not args.quiet:
                    print(f"icmp_seq={i + 1} timeout")
            else:
                stats.received += 1
                stats.rtts_ms.append(rtt)
                if not args.quiet:
                    print(f"icmp_seq={i + 1} time={rtt:.3f} ms")

            i += 1
            if (args.count == 0 or i < args.count) and not interrupted:
                time.sleep(args.interval)
    finally:
        client.close()

    print_summary(args.host, args.port, stats)
    return 0 if stats.received > 0 else 1


if __name__ == '__main__':
    sys.exit(main())
