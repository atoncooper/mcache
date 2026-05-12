"""PyPI smoke test — verify `pip install mcache-py` works end-to-end.

Run after starting an mcache server on 127.0.0.1:11211.

Usage:
    pip install mcache-py
    python pypi_smoke_test.py [--addr HOST:PORT]
"""

from __future__ import annotations

import argparse
import sys
import time
from typing import Callable

import mcache


def _print_header(title: str) -> None:
    print(f"\n=== {title} ===")


def _assert_eq(actual, expected, label: str) -> None:
    status = "PASS" if actual == expected else "FAIL"
    print(f"  [{status}] {label}: got={actual!r} expected={expected!r}")
    if actual != expected:
        raise AssertionError(f"{label} mismatch")


def test_kv(client: mcache.Client) -> None:
    _print_header("KV (Get / Set / Del / Exists / TTL)")
    client.set("smoke:k1", b"hello")
    _assert_eq(client.get("smoke:k1"), b"hello", "set/get bytes")

    client.set("smoke:k2", b"\x00\x01\x02", 60)
    _assert_eq(client.get("smoke:k2"), b"\x00\x01\x02", "set/get bytes + ttl")

    _assert_eq(client.exists("smoke:k1"), True, "exists existing")
    _assert_eq(client.exists("smoke:missing"), False, "exists missing")

    ttl = client.ttl("smoke:k2")
    print(f"  [INFO] ttl(smoke:k2)={ttl}s (expected ~60)")
    assert 50 <= ttl <= 60, f"ttl out of range: {ttl}"

    client.delete("smoke:k1")
    _assert_eq(client.exists("smoke:k1"), False, "delete worked")


def test_ping_and_stats(client: mcache.Client) -> None:
    _print_header("Ping / Len / Cleanup")
    t0 = time.perf_counter()
    pong = client.ping()
    rtt_ms = (time.perf_counter() - t0) * 1000
    _assert_eq(pong, True, "ping returns True")
    print(f"  [INFO] ping RTT: {rtt_ms:.2f} ms")

    n = client.len()
    print(f"  [INFO] cache length: {n}")

    removed = client.cleanup()
    print(f"  [INFO] cleanup removed: {removed}")


def test_hash(client: mcache.Client) -> None:
    _print_header("Hash (HSet / HGet / HGetAll / HDel)")
    client.hset("smoke:h", "name", "alice")
    client.hset("smoke:h", "age", "30")
    _assert_eq(client.hget("smoke:h", "name"), "alice", "hget existing")
    _assert_eq(client.hlen("smoke:h"), 2, "hlen")

    all_fields = client.hgetall("smoke:h")
    print(f"  [INFO] hgetall: {all_fields}")
    assert "name" in all_fields and "age" in all_fields, "hgetall missing fields"

    client.hdel("smoke:h", "age")
    _assert_eq(client.hlen("smoke:h"), 1, "hlen after hdel")

    client.delete("smoke:h")


def test_list(client: mcache.Client) -> None:
    _print_header("List (LPush / RPush / LRange / LPop)")
    client.delete("smoke:l")
    client.rpush("smoke:l", "a", "b", "c")
    _assert_eq(client.llen("smoke:l"), 3, "llen after rpush")

    rng = client.lrange("smoke:l", 0, -1)
    _assert_eq(rng, ["a", "b", "c"], "lrange full")

    first = client.lpop("smoke:l")
    _assert_eq(first, "a", "lpop head")
    _assert_eq(client.llen("smoke:l"), 2, "llen after lpop")

    client.delete("smoke:l")


def test_set(client: mcache.Client) -> None:
    _print_header("Set (SAdd / SMembers / SIsMember / SRem)")
    client.delete("smoke:s")
    client.sadd("smoke:s", "x", "y", "z")
    _assert_eq(client.scard("smoke:s"), 3, "scard")
    _assert_eq(client.sismember("smoke:s", "x"), True, "sismember existing")
    _assert_eq(client.sismember("smoke:s", "missing"), False, "sismember missing")

    members = set(client.smembers("smoke:s"))
    _assert_eq(members, {"x", "y", "z"}, "smembers")

    client.srem("smoke:s", "y")
    _assert_eq(client.scard("smoke:s"), 2, "scard after srem")

    client.delete("smoke:s")


def run_suite(addr: str) -> int:
    host, _, port_str = addr.partition(":")
    port = int(port_str) if port_str else 11211

    print(f"mcache-py version: {mcache.__version__}")
    print(f"connecting to {host}:{port}...")

    client = mcache.Client(host=host, port=port)

    suites: list[Callable[[mcache.Client], None]] = [
        test_ping_and_stats,
        test_kv,
        test_hash,
        test_list,
        test_set,
    ]

    failed: list[str] = []
    for suite in suites:
        try:
            suite(client)
        except Exception as exc:  # noqa: BLE001
            failed.append(f"{suite.__name__}: {exc}")
            print(f"  [ERROR] {exc!r}")

    print("\n" + "=" * 50)
    if failed:
        print(f"FAILED ({len(failed)}):")
        for f in failed:
            print(f"  - {f}")
        return 1
    print("ALL TESTS PASSED")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--addr",
        default="127.0.0.1:11211",
        help="mcache server address (default: 127.0.0.1:11211)",
    )
    args = parser.parse_args()
    return run_suite(args.addr)


if __name__ == "__main__":
    sys.exit(main())
