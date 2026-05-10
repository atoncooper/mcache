"""Cluster-aware mcache clients.

Three cluster modes (matching the Go server cluster package):

  ShardClient   — consistent-hashing shard across nodes (FNV-1a weighted hash)
  MasterSlaveClient — writes to master, reads round-robin across slaves
  SentinelClient — monitors master, auto-failover to replica

All modes route on the client side — the server never returns MOVED/ASK redirects.
If the cluster topology changes, call :meth:`refresh()` or enable background probing.
"""

from __future__ import annotations

import threading
import time
from typing import Optional

from .client import Client
from .errors import McacheError
from . import protocol

# FNV-1a 32-bit constants (same as Go hash/fnv.New32a)
_FNV_OFFSET = 0x811c9dc5
_FNV_PRIME = 0x01000193


def _fnv32a(data: bytes) -> int:
    """FNV-1a 32-bit hash (matches Go hash/fnv.New32a)."""
    h = _FNV_OFFSET
    for b in data:
        h ^= b
        h = (h * _FNV_PRIME) & 0xFFFFFFFF
    return h


def _hash_key(key: str) -> int:
    return _fnv32a(key.encode())


# ---------------------------------------------------------------------------
# ShardClient — consistent-hashing shard
# ---------------------------------------------------------------------------

class ShardClient:
    """Distributes keys across nodes using FNV-1a weighted consistent hashing.

    Usage::

        sc = ShardClient([
            ('10.0.0.1', 11211, 2),   # (host, port, weight)
            ('10.0.0.2', 11211, 1),
        ])
        sc.set('hello', b'world')
        sc.close()
    """

    def __init__(self, nodes: list[tuple[str, int, int]] | list[str],
                 timeout: float = 10.0, pool_size: int = 2,
                 health_interval: float = 5.0):
        """
        *nodes*: list of ``(host, port, weight)`` or ``"host:port"`` strings.
        *health_interval*: seconds between health probes (0 = disable).
        """
        self._mu = threading.RLock()
        self._clients: list[tuple[Client, int, bool]] = []  # (client, weight, healthy)
        self._timeout = timeout
        self._pool_size = pool_size
        self._health_interval = health_interval
        self._stop = threading.Event()
        self._health_thread: Optional[threading.Thread] = None

        for n in nodes:
            if isinstance(n, str):
                host, port = n.rsplit(':', 1)
                weight = 1
            else:
                host, port, weight = n[0], n[1], n[2] if len(n) > 2 else 1
            c = Client(host=host, port=port, pool_size=pool_size, timeout=timeout)
            self._clients.append((c, max(weight, 1), True))

        if not self._clients:
            raise ValueError('ShardClient requires at least one node')

        if health_interval > 0:
            self._health_thread = threading.Thread(target=self._health_loop, daemon=True)
            self._health_thread.start()

    def _pick(self, key: str) -> Client:
        with self._mu:
            # Compute weighted index from FNV hash
            h = _hash_key(key)
            total_weight = sum(w for _, w, healthy in self._clients if healthy)
            if total_weight == 0:
                # Fallback: try all nodes
                for c, _, _ in self._clients:
                    return c
                raise McacheError('no healthy shard node available')

            idx = h % total_weight
            cursor = 0
            for c, w, healthy in self._clients:
                if not healthy:
                    continue
                cursor += w
                if idx < cursor:
                    return c
            return self._clients[0][0]  # fallback

    def refresh(self, nodes: list[tuple[str, int, int]] | list[str]) -> None:
        """Replace the node list (e.g. after cluster expansion).

        Old connections are closed, new ones opened.
        """
        new_clients: list[tuple[Client, int, bool]] = []
        for n in nodes:
            if isinstance(n, str):
                host, port = n.rsplit(':', 1)
                weight = 1
            else:
                host, port, weight = n[0], n[1], n[2] if len(n) > 2 else 1
            c = Client(host=host, port=port, pool_size=self._pool_size, timeout=self._timeout)
            new_clients.append((c, max(weight, 1), True))

        with self._mu:
            old = self._clients
            self._clients = new_clients

        for c, _, _ in old:
            c.close()

    def close(self) -> None:
        self._stop.set()
        with self._mu:
            for c, _, _ in self._clients:
                c.close()

    def _health_loop(self) -> None:
        while not self._stop.wait(self._health_interval):
            with self._mu:
                for i, (c, w, _) in enumerate(self._clients):
                    try:
                        c.ping()
                        self._clients[i] = (c, w, True)
                    except Exception:
                        self._clients[i] = (c, w, False)

    # --- delegate all commands with key routing ---

    def _route(self, key: str, method: str, *args, **kwargs):
        return getattr(self._pick(key), method)(*args, **kwargs)

    def get(self, key: str) -> Optional[bytes]:
        return self._pick(key).get(key)

    def set(self, key: str, value: bytes, ttl: int = 0) -> bool:
        return self._pick(key).set(key, value, ttl)

    def delete(self, key: str) -> bool:
        return self._pick(key).delete(key)

    def exists(self, key: str) -> bool:
        return self._pick(key).exists(key)

    def type(self, key: str) -> str:
        return self._pick(key).type(key)

    def expire(self, key: str, seconds: int) -> bool:
        return self._pick(key).expire(key, seconds)

    def pexpire(self, key: str, ms: int) -> bool:
        return self._pick(key).pexpire(key, ms)

    def ttl(self, key: str) -> int:
        return self._pick(key).ttl(key)

    def pttl(self, key: str) -> int:
        return self._pick(key).pttl(key)

    def persist(self, key: str) -> bool:
        return self._pick(key).persist(key)

    # hash
    def hset(self, key: str, field: str, value: str) -> int:
        return self._pick(key).hset(key, field, value)

    def hget(self, key: str, field: str) -> Optional[str]:
        return self._pick(key).hget(key, field)

    def hdel(self, key: str, *fields: str) -> int:
        return self._pick(key).hdel(key, *fields)

    def hexists(self, key: str, field: str) -> bool:
        return self._pick(key).hexists(key, field)

    def hgetall(self, key: str) -> dict[str, str]:
        return self._pick(key).hgetall(key)

    def hlen(self, key: str) -> int:
        return self._pick(key).hlen(key)

    def hincrby(self, key: str, field: str, delta: int) -> int:
        return self._pick(key).hincrby(key, field, delta)

    def hincrbyfloat(self, key: str, field: str, delta: float) -> float:
        return self._pick(key).hincrbyfloat(key, field, delta)

    def hmset(self, key: str, mapping: dict = None, **kwargs) -> bool:
        return self._pick(key).hmset(key, mapping, **kwargs)

    def hmget(self, key: str, *fields: str) -> list:
        return self._pick(key).hmget(key, *fields)

    def hsetnx(self, key: str, field: str, value: str) -> bool:
        return self._pick(key).hsetnx(key, field, value)

    def hkeys(self, key: str) -> list[str]:
        return self._pick(key).hkeys(key)

    def hvals(self, key: str) -> list[str]:
        return self._pick(key).hvals(key)

    def hstrlen(self, key: str, field: str) -> int:
        return self._pick(key).hstrlen(key, field)

    # list
    def lpush(self, key: str, *elements: str) -> int:
        return self._pick(key).lpush(key, *elements)

    def rpush(self, key: str, *elements: str) -> int:
        return self._pick(key).rpush(key, *elements)

    def lpop(self, key: str) -> Optional[str]:
        return self._pick(key).lpop(key)

    def rpop(self, key: str) -> Optional[str]:
        return self._pick(key).rpop(key)

    def llen(self, key: str) -> int:
        return self._pick(key).llen(key)

    def lrange(self, key: str, start: int, stop: int) -> list[str]:
        return self._pick(key).lrange(key, start, stop)

    def lindex(self, key: str, index: int) -> Optional[str]:
        return self._pick(key).lindex(key, index)

    def lset(self, key: str, index: int, value: str) -> bool:
        return self._pick(key).lset(key, index, value)

    def lrem(self, key: str, count: int, value: str) -> int:
        return self._pick(key).lrem(key, count, value)

    def ltrim(self, key: str, start: int, stop: int) -> bool:
        return self._pick(key).ltrim(key, start, stop)

    def linsert(self, key: str, where: str, pivot: str, value: str) -> int:
        return self._pick(key).linsert(key, where, pivot, value)

    def blpop(self, key: str, timeout: float = 0) -> Optional[str]:
        return self._pick(key).blpop(key, timeout)

    def brpop(self, key: str, timeout: float = 0) -> Optional[str]:
        return self._pick(key).brpop(key, timeout)

    def lpos(self, key: str, value: str, rank: int = 1,
             count: int = 1, maxlen: int = 0) -> list[int]:
        return self._pick(key).lpos(key, value, rank, count, maxlen)

    # set
    def sadd(self, key: str, *elements: str) -> int:
        return self._pick(key).sadd(key, *elements)

    def srem(self, key: str, *elements: str) -> int:
        return self._pick(key).srem(key, *elements)

    def sismember(self, key: str, element: str) -> bool:
        return self._pick(key).sismember(key, element)

    def smembers(self, key: str) -> list[str]:
        return self._pick(key).smembers(key)

    def scard(self, key: str) -> int:
        return self._pick(key).scard(key)

    def spop(self, key: str) -> Optional[str]:
        return self._pick(key).spop(key)

    def srandmember(self, key: str, count: int = 1) -> list[str]:
        return self._pick(key).srandmember(key, count)

    def sunion(self, *keys: str) -> list[str]:
        # Uses first key for routing (consistent with Go behavior)
        if not keys:
            return []
        return self._pick(keys[0]).sunion(*keys)

    def sinter(self, *keys: str) -> list[str]:
        if not keys:
            return []
        return self._pick(keys[0]).sinter(*keys)

    def sdiff(self, *keys: str) -> list[str]:
        if not keys:
            return []
        return self._pick(keys[0]).sdiff(*keys)

    # global ops — fan-out to all healthy nodes
    def len(self) -> int:
        total = 0
        with self._mu:
            for c, _, healthy in self._clients:
                if healthy:
                    try:
                        total += c.len()
                    except Exception:
                        pass
        return total

    def keys(self, pattern: str = '*') -> list[str]:
        result = []
        with self._mu:
            for c, _, healthy in self._clients:
                if healthy:
                    try:
                        result.extend(c.keys(pattern))
                    except Exception:
                        pass
        return result

    def cleanup(self) -> int:
        total = 0
        with self._mu:
            for c, _, healthy in self._clients:
                if healthy:
                    try:
                        total += c.cleanup()
                    except Exception:
                        pass
        return total

    def ping(self) -> bool:
        with self._mu:
            for c, _, _ in self._clients:
                c.ping()
        return True


# ---------------------------------------------------------------------------
# MasterSlaveClient — writes to master, reads from slaves
# ---------------------------------------------------------------------------

class MasterSlaveClient:
    """Writes go to master, reads round-robin across healthy slaves.

    Usage::

        ms = MasterSlaveClient(
            master=('10.0.0.1', 11211),
            slaves=[('10.0.0.2', 11211), ('10.0.0.3', 11211)],
        )
        ms.set('k', b'v')   # → master
        ms.get('k')          # → slave (round-robin)
    """

    def __init__(self, master: tuple[str, int],
                 slaves: list[tuple[str, int]],
                 timeout: float = 10.0, pool_size: int = 2,
                 health_interval: float = 5.0):
        self._master = Client(host=master[0], port=master[1],
                              pool_size=pool_size, timeout=timeout)
        self._slaves = [Client(host=s[0], port=s[1],
                               pool_size=pool_size, timeout=timeout)
                        for s in slaves]
        self._master_healthy = True
        self._slave_healthy = [True] * len(self._slaves)
        self._next_slave = 0
        self._mu = threading.Lock()
        self._health_interval = health_interval
        self._stop = threading.Event()
        if health_interval > 0:
            self._health_thread = threading.Thread(target=self._health_loop, daemon=True)
            self._health_thread.start()

    def _pick_read(self) -> Client:
        with self._mu:
            for _ in range(len(self._slaves)):
                self._next_slave = (self._next_slave + 1) % max(len(self._slaves), 1)
                idx = self._next_slave
                if self._slave_healthy[idx]:
                    return self._slaves[idx]
        # Fallback to master
        if self._master_healthy:
            return self._master
        raise McacheError('no healthy node available')

    def _master_or_die(self) -> Client:
        if not self._master_healthy:
            raise McacheError('master is not healthy')
        return self._master

    def close(self) -> None:
        self._stop.set()
        self._master.close()
        for s in self._slaves:
            s.close()

    def _health_loop(self) -> None:
        while not self._stop.wait(self._health_interval):
            try:
                self._master.ping()
                self._master_healthy = True
            except Exception:
                self._master_healthy = False
            for i, s in enumerate(self._slaves):
                try:
                    s.ping()
                    self._slave_healthy[i] = True
                except Exception:
                    self._slave_healthy[i] = False

    # ---- write ops → master ----

    def set(self, key: str, value: bytes, ttl: int = 0) -> bool:
        return self._master_or_die().set(key, value, ttl)

    def delete(self, key: str) -> bool:
        return self._master_or_die().delete(key)

    def hset(self, key: str, field: str, value: str) -> int:
        return self._master_or_die().hset(key, field, value)

    def hdel(self, key: str, *fields: str) -> int:
        return self._master_or_die().hdel(key, *fields)

    def hincrby(self, key: str, field: str, delta: int) -> int:
        return self._master_or_die().hincrby(key, field, delta)

    def hincrbyfloat(self, key: str, field: str, delta: float) -> float:
        return self._master_or_die().hincrbyfloat(key, field, delta)

    def hmset(self, key: str, mapping: dict = None, **kwargs) -> bool:
        return self._master_or_die().hmset(key, mapping, **kwargs)

    def hsetnx(self, key: str, field: str, value: str) -> bool:
        return self._master_or_die().hsetnx(key, field, value)

    def lpush(self, key: str, *elements: str) -> int:
        return self._master_or_die().lpush(key, *elements)

    def rpush(self, key: str, *elements: str) -> int:
        return self._master_or_die().rpush(key, *elements)

    def lpop(self, key: str) -> Optional[str]:
        return self._master_or_die().lpop(key)

    def rpop(self, key: str) -> Optional[str]:
        return self._master_or_die().rpop(key)

    def lset(self, key: str, index: int, value: str) -> bool:
        return self._master_or_die().lset(key, index, value)

    def lrem(self, key: str, count: int, value: str) -> int:
        return self._master_or_die().lrem(key, count, value)

    def ltrim(self, key: str, start: int, stop: int) -> bool:
        return self._master_or_die().ltrim(key, start, stop)

    def linsert(self, key: str, where: str, pivot: str, value: str) -> int:
        return self._master_or_die().linsert(key, where, pivot, value)

    def sadd(self, key: str, *elements: str) -> int:
        return self._master_or_die().sadd(key, *elements)

    def srem(self, key: str, *elements: str) -> int:
        return self._master_or_die().srem(key, *elements)

    def expire(self, key: str, seconds: int) -> bool:
        return self._master_or_die().expire(key, seconds)

    def pexpire(self, key: str, ms: int) -> bool:
        return self._master_or_die().pexpire(key, ms)

    def persist(self, key: str) -> bool:
        return self._master_or_die().persist(key)

    # ---- read ops → slave (fallback to master) ----

    def get(self, key: str) -> Optional[bytes]:
        return self._pick_read().get(key)

    def exists(self, key: str) -> bool:
        return self._pick_read().exists(key)

    def type(self, key: str) -> str:
        return self._pick_read().type(key)

    def ttl(self, key: str) -> int:
        return self._pick_read().ttl(key)

    def pttl(self, key: str) -> int:
        return self._pick_read().pttl(key)

    def hget(self, key: str, field: str) -> Optional[str]:
        return self._pick_read().hget(key, field)

    def hexists(self, key: str, field: str) -> bool:
        return self._pick_read().hexists(key, field)

    def hgetall(self, key: str) -> dict[str, str]:
        return self._pick_read().hgetall(key)

    def hlen(self, key: str) -> int:
        return self._pick_read().hlen(key)

    def hkeys(self, key: str) -> list[str]:
        return self._pick_read().hkeys(key)

    def hvals(self, key: str) -> list[str]:
        return self._pick_read().hvals(key)

    def hstrlen(self, key: str, field: str) -> int:
        return self._pick_read().hstrlen(key, field)

    def hmget(self, key: str, *fields: str) -> list:
        return self._pick_read().hmget(key, *fields)

    def llen(self, key: str) -> int:
        return self._pick_read().llen(key)

    def lrange(self, key: str, start: int, stop: int) -> list[str]:
        return self._pick_read().lrange(key, start, stop)

    def lindex(self, key: str, index: int) -> Optional[str]:
        return self._pick_read().lindex(key, index)

    def blpop(self, key: str, timeout: float = 0) -> Optional[str]:
        return self._pick_read().blpop(key, timeout)

    def brpop(self, key: str, timeout: float = 0) -> Optional[str]:
        return self._pick_read().brpop(key, timeout)

    def lpos(self, key: str, value: str, rank: int = 1,
             count: int = 1, maxlen: int = 0) -> list[int]:
        return self._pick_read().lpos(key, value, rank, count, maxlen)

    def sismember(self, key: str, element: str) -> bool:
        return self._pick_read().sismember(key, element)

    def smembers(self, key: str) -> list[str]:
        return self._pick_read().smembers(key)

    def scard(self, key: str) -> int:
        return self._pick_read().scard(key)

    def spop(self, key: str) -> Optional[str]:
        return self._master_or_die().spop(key)  # modifies

    def srandmember(self, key: str, count: int = 1) -> list[str]:
        return self._pick_read().srandmember(key, count)

    def sunion(self, *keys: str) -> list[str]:
        if not keys:
            return []
        return self._pick_read().sunion(*keys)

    def sinter(self, *keys: str) -> list[str]:
        if not keys:
            return []
        return self._pick_read().sinter(*keys)

    def sdiff(self, *keys: str) -> list[str]:
        if not keys:
            return []
        return self._pick_read().sdiff(*keys)

    def len(self) -> int:
        return self._master_or_die().len()

    def keys(self, pattern: str = '*') -> list[str]:
        try:
            return self._master_or_die().keys(pattern)
        except McacheError:
            pass
        try:
            return self._pick_read().keys(pattern)
        except McacheError:
            return []

    def cleanup(self) -> int:
        return self._master_or_die().cleanup()

    def ping(self) -> bool:
        self._master_or_die().ping()
        self._pick_read().ping()
        return True


# ---------------------------------------------------------------------------
# SentinelClient — auto-failover
# ---------------------------------------------------------------------------

class SentinelClient:
    """Monitors a master node; if it fails, promotes a healthy replica.

    Usage::

        sc = SentinelClient(
            master=('10.0.0.1', 11211),
            replicas=[('10.0.0.2', 11211), ('10.0.0.3', 11211)],
        )
        sc.set('k', b'v')  # always to current master
        # If master dies, the next replica is promoted automatically.
    """

    def __init__(self, master: tuple[str, int],
                 replicas: list[tuple[str, int]],
                 timeout: float = 10.0, pool_size: int = 2,
                 health_interval: float = 3.0):
        self._timeout = timeout
        self._pool_size = pool_size
        self._mu = threading.Lock()
        self._failover_mu = threading.Lock()
        self._master: Client = Client(host=master[0], port=master[1],
                                      pool_size=pool_size, timeout=timeout)
        self._master_addr = master
        self._replicas = [Client(host=r[0], port=r[1],
                                 pool_size=pool_size, timeout=timeout)
                          for r in replicas]
        self._replica_addrs = replicas
        self._healthy = True
        self._stop = threading.Event()
        self._health_thread = threading.Thread(target=self._monitor_loop, daemon=True)
        self._health_thread.start()

    @property
    def master_addr(self) -> tuple[str, int]:
        """Current master address (may change after failover)."""
        return self._master_addr

    def close(self) -> None:
        self._stop.set()
        self._master.close()
        for r in self._replicas:
            r.close()

    def _monitor_loop(self) -> None:
        while not self._stop.wait(self._timeout / 3 or 3.0):
            try:
                self._master.ping()
                self._healthy = True
            except Exception:
                self._healthy = False
                self._try_failover()

    def _try_failover(self) -> None:
        if not self._failover_mu.acquire(blocking=False):
            return  # another thread is already failing over
        try:
            if self._healthy:
                return  # already recovered
            for i, r in enumerate(self._replicas):
                try:
                    r.ping()
                    # Promote this replica
                    old = self._master
                    old_addr = self._master_addr
                    self._master = r
                    self._master_addr = self._replica_addrs[i]
                    self._healthy = True
                    print(f'[sentinel] failover: {old_addr} → {self._master_addr}')
                    # Close old master in background
                    threading.Thread(target=old.close, daemon=True).start()
                    return
                except Exception:
                    continue
            print('[sentinel] failover failed: no healthy replica')
        finally:
            self._failover_mu.release()

    # Delegate all ops to current master
    def __getattr__(self, name: str):
        if name.startswith('_'):
            raise AttributeError(name)
        return getattr(self._master, name)
