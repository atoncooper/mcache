"""High-level mcache client with a redis-py-like API."""

from __future__ import annotations

from types import TracebackType
from typing import Optional

from . import protocol
from .pool import ConnectionPool
from .errors import (
    McacheError, KeyNotFoundError, ServerError,
    ReadTimeout, ConnectionError,
)

_KEYTYPE_MAP = {
    0: 'none', 1: 'string', 2: 'set', 3: 'hash', 4: 'list',
}


class Client:
    """mcache TCP client.

    Usage::

        c = Client(host='127.0.0.1', port=11211)
        c.set('hello', b'world')
        print(c.get('hello'))
        c.close()

        # Context manager
        with Client() as c:
            c.ping()
    """

    def __init__(self, host: str = '127.0.0.1', port: int = 11211,
                 pool_size: int = 4, timeout: float = 10.0):
        self._pool = ConnectionPool(host, port, pool_size, timeout)
        self._pool.connect()

    # ---- context manager ----

    def __enter__(self) -> 'Client':
        return self

    def __exit__(self, exc_type: type[BaseException] | None,
                 exc_val: BaseException | None,
                 exc_tb: TracebackType | None) -> None:
        self.close()

    def close(self) -> None:
        """Close all connections."""
        self._pool.close()

    # ---- raw request helpers ----

    def _kv(self, cmd: int, key: str, value: bytes = b'', ttl_ms: int = 0) -> tuple[int, bytes, str]:
        payload = protocol.encode_kv_request(cmd, key, value, ttl_ms)
        resp = self._pool.get_connection().send(payload)
        return protocol.decode_kv_response(resp)

    def _set(self, cmd: int, key: str, elems: list = None,
             count: int = 0, keys: list = None) -> dict:
        payload = protocol.encode_set_request(cmd, key, elems, count, keys)
        raw = self._pool.get_connection().send(payload)
        # server now prepends cmd byte; strip it
        if raw and 10 <= raw[0] <= 19:
            raw = raw[1:]
        return protocol.decode_set_response(raw, cmd)

    def _hash(self, cmd: int, key: str, field: str = '', value: str = '',
              fields: list = None, fv_pairs: list = None,
              delta_i64: int = 0, delta_f64: float = 0.0) -> dict:
        payload = protocol.encode_hash_request(cmd, key, field, value, fields, fv_pairs, delta_i64, delta_f64)
        raw = self._pool.get_connection().send(payload)
        if raw and 32 <= raw[0] <= 45:
            raw = raw[1:]
        return protocol.decode_hash_response(raw, cmd)

    def _list(self, cmd: int, key: str, elements: list = None, value: str = '',
              index: int = 0, start: int = 0, stop: int = 0,
              count: int = 0, pivot: str = '', before: bool = False,
              timeout: int = 0, rank: int = 1, max_len: int = 0) -> dict:
        payload = protocol.encode_list_request(cmd, key, elements, value,
                                               index, start, stop, count,
                                               pivot, before, timeout, rank, max_len)
        raw = self._pool.get_connection().send(payload)
        if raw and 48 <= raw[0] <= 61:
            raw = raw[1:]
        return protocol.decode_list_response(raw, cmd)

    def _key(self, cmd: int, key: str, extra: int = 0) -> tuple[int, bytes, str]:
        payload = protocol.encode_kv_request(cmd, key, ttl_ms=extra)
        resp = self._pool.get_connection().send(payload)
        return protocol.decode_kv_response(resp)

    def _raise(self, status: int, err_msg: str) -> None:
        if status == protocol.STATUS_NOT_FOUND:
            raise KeyNotFoundError(err_msg or 'key not found')
        if status == protocol.STATUS_ERR:
            raise ServerError(err_msg or 'server error')

    # ---- KV commands ----

    def get(self, key: str) -> Optional[bytes]:
        """Get the value of *key*, or *None* if not found."""
        status, val, err = self._kv(protocol.CMD_GET, key)
        if status == protocol.STATUS_NOT_FOUND:
            return None
        self._raise(status, err)
        return val

    def set(self, key: str, value: bytes, ttl: int = 0) -> bool:
        """Set *key* to *value*. *ttl* in seconds (0 = default, <0 = no expiry)."""
        ttl_ms = ttl * 1000 if ttl > 0 else 0
        status, _, err = self._kv(protocol.CMD_SET, key, value, ttl_ms)
        self._raise(status, err)
        return True

    def delete(self, key: str) -> bool:
        """Delete *key*."""
        status, _, err = self._kv(protocol.CMD_DEL, key)
        self._raise(status, err)
        return True

    def __len__(self) -> int:
        status, val, err = self._kv(protocol.CMD_LEN, '')
        self._raise(status, err)
        if len(val) >= 8:
            import struct
            return struct.unpack('>Q', val)[0]
        return 0

    def len(self) -> int:
        """Return the total number of entries."""
        return len(self)

    def cleanup(self) -> int:
        """Remove expired entries. Returns count removed."""
        status, val, err = self._kv(protocol.CMD_CLEANUP, '')
        self._raise(status, err)
        if len(val) >= 8:
            import struct
            return struct.unpack('>Q', val)[0]
        return 0

    def ping(self) -> bool:
        """Check connectivity."""
        self.len()
        return True

    # ---- Key management ----

    def exists(self, key: str) -> bool:
        """Return True if *key* exists (any type)."""
        status, val, err = self._key(protocol.CMD_EXISTS, key)
        self._raise(status, err)
        return len(val) > 0 and val[0] == 1

    def type(self, key: str) -> str:
        """Return the type of *key*: 'string', 'set', 'hash', 'list', or 'none'."""
        status, val, err = self._key(protocol.CMD_TYPE, key)
        self._raise(status, err)
        t = val[0] if len(val) > 0 else 0
        return _KEYTYPE_MAP.get(t, 'none')

    def expire(self, key: str, seconds: int) -> bool:
        """Set a TTL in seconds on *key*. Returns True if successful."""
        status, val, err = self._key(protocol.CMD_EXPIRE, key, extra=seconds * 1000)
        self._raise(status, err)
        return len(val) > 0 and val[0] == 1

    def pexpire(self, key: str, ms: int) -> bool:
        """Set a TTL in milliseconds on *key*."""
        status, val, err = self._key(protocol.CMD_PEXPIRE, key, extra=ms)
        self._raise(status, err)
        return len(val) > 0 and val[0] == 1

    def ttl(self, key: str) -> int:
        """Get remaining TTL in seconds. -1=no expiry, -2=not found."""
        status, val, err = self._key(protocol.CMD_TTL, key)
        self._raise(status, err)
        if len(val) >= 8:
            import struct
            return struct.unpack('>q', val)[0]
        return -2

    def pttl(self, key: str) -> int:
        """Get remaining TTL in milliseconds."""
        status, val, err = self._key(protocol.CMD_PTTL, key)
        self._raise(status, err)
        if len(val) >= 8:
            import struct
            return struct.unpack('>q', val)[0]
        return -2

    def persist(self, key: str) -> bool:
        """Remove the TTL from *key*."""
        status, val, err = self._key(protocol.CMD_PERSIST, key)
        self._raise(status, err)
        return len(val) > 0 and val[0] == 1

    def keys(self, pattern: str = '*') -> list[str]:
        """Return keys matching *pattern* (glob)."""
        status, val, err = self._kv(protocol.CMD_KEYS, pattern)
        self._raise(status, err)
        if len(val) < 4:
            return []
        count = int.from_bytes(val[:4], 'big')
        off = 4
        result = []
        for _ in range(count):
            if off + 2 > len(val):
                break
            klen = int.from_bytes(val[off:off + 2], 'big')
            off += 2
            if off + klen > len(val):
                break
            result.append(val[off:off + klen].decode())
            off += klen
        return result

    # ---- Hash commands ----

    def hset(self, key: str, field: str, value: str) -> int:
        r = self._hash(protocol.CMD_HSET, key, field, value)
        self._raise(r['status'], r.get('err_msg', ''))
        return int(r['int_result'])

    def hsetnx(self, key: str, field: str, value: str) -> bool:
        r = self._hash(protocol.CMD_HSETNX, key, field, value)
        self._raise(r['status'], r.get('err_msg', ''))
        return r['bool_result']

    def hget(self, key: str, field: str) -> Optional[str]:
        r = self._hash(protocol.CMD_HGET, key, field)
        if r['status'] == protocol.STATUS_NOT_FOUND:
            return None
        self._raise(r['status'], r.get('err_msg', ''))
        return r['str_result']

    def hdel(self, key: str, *fields: str) -> int:
        r = self._hash(protocol.CMD_HDEL, key, fields=list(fields))
        self._raise(r['status'], r.get('err_msg', ''))
        return int(r['int_result'])

    def hexists(self, key: str, field: str) -> bool:
        r = self._hash(protocol.CMD_HEXISTS, key, field)
        self._raise(r['status'], r.get('err_msg', ''))
        return r['bool_result']

    def hgetall(self, key: str) -> dict[str, str]:
        r = self._hash(protocol.CMD_HGETALL, key)
        if r['status'] == protocol.STATUS_NOT_FOUND:
            return {}
        self._raise(r['status'], r.get('err_msg', ''))
        return r.get('map_result', {})

    def hkeys(self, key: str) -> list[str]:
        r = self._hash(protocol.CMD_HKEYS, key)
        if r['status'] == protocol.STATUS_NOT_FOUND:
            return []
        self._raise(r['status'], r.get('err_msg', ''))
        return r.get('slice_result', [])

    def hvals(self, key: str) -> list[str]:
        r = self._hash(protocol.CMD_HVALS, key)
        if r['status'] == protocol.STATUS_NOT_FOUND:
            return []
        self._raise(r['status'], r.get('err_msg', ''))
        return r.get('slice_result', [])

    def hlen(self, key: str) -> int:
        r = self._hash(protocol.CMD_HLEN, key)
        self._raise(r['status'], r.get('err_msg', ''))
        return int(r['int_result'])

    def hstrlen(self, key: str, field: str) -> int:
        r = self._hash(protocol.CMD_HSTRLEN, key, field)
        self._raise(r['status'], r.get('err_msg', ''))
        return int(r['int_result'])

    def hincrby(self, key: str, field: str, delta: int) -> int:
        r = self._hash(protocol.CMD_HINCRBY, key, field, delta_i64=delta)
        self._raise(r['status'], r.get('err_msg', ''))
        return int(r['int_result'])

    def hincrbyfloat(self, key: str, field: str, delta: float) -> float:
        r = self._hash(protocol.CMD_HINCRBYFLOAT, key, field, delta_f64=delta)
        self._raise(r['status'], r.get('err_msg', ''))
        return r['float_result']

    def hmget(self, key: str, *fields: str) -> list:
        r = self._hash(protocol.CMD_HMGET, key, fields=list(fields))
        self._raise(r['status'], r.get('err_msg', ''))
        return r.get('any_slice', [])

    def hmset(self, key: str, mapping: dict = None, **kwargs) -> bool:
        fv_pairs = []
        if mapping:
            for k, v in mapping.items():
                fv_pairs.extend([str(k), str(v)])
        for k, v in kwargs.items():
            fv_pairs.extend([str(k), str(v)])
        r = self._hash(protocol.CMD_HMSET, key, fv_pairs=fv_pairs)
        self._raise(r['status'], r.get('err_msg', ''))
        return True

    # ---- List commands ----

    def lpush(self, key: str, *elements: str) -> int:
        r = self._list(protocol.CMD_LPUSH, key, elements=list(elements))
        self._raise(r['status'], r.get('err_msg', ''))
        return int(r['int_result'])

    def rpush(self, key: str, *elements: str) -> int:
        r = self._list(protocol.CMD_RPUSH, key, elements=list(elements))
        self._raise(r['status'], r.get('err_msg', ''))
        return int(r['int_result'])

    def lpop(self, key: str) -> Optional[str]:
        r = self._list(protocol.CMD_LPOP, key)
        if r['status'] == protocol.STATUS_NOT_FOUND:
            return None
        self._raise(r['status'], r.get('err_msg', ''))
        return r['str_result']

    def rpop(self, key: str) -> Optional[str]:
        r = self._list(protocol.CMD_RPOP, key)
        if r['status'] == protocol.STATUS_NOT_FOUND:
            return None
        self._raise(r['status'], r.get('err_msg', ''))
        return r['str_result']

    def llen(self, key: str) -> int:
        r = self._list(protocol.CMD_LLEN, key)
        self._raise(r['status'], r.get('err_msg', ''))
        return int(r['int_result'])

    def lrange(self, key: str, start: int, stop: int) -> list[str]:
        r = self._list(protocol.CMD_LRANGE, key, start=start, stop=stop)
        if r['status'] == protocol.STATUS_NOT_FOUND:
            return []
        self._raise(r['status'], r.get('err_msg', ''))
        return r.get('slice_result', [])

    def lindex(self, key: str, index: int) -> Optional[str]:
        r = self._list(protocol.CMD_LINDEX, key, index=index)
        if r['status'] == protocol.STATUS_NOT_FOUND:
            return None
        self._raise(r['status'], r.get('err_msg', ''))
        return r['str_result']

    def lset(self, key: str, index: int, value: str) -> bool:
        r = self._list(protocol.CMD_LSET, key, value=value, index=index)
        if r['status'] == protocol.STATUS_NOT_FOUND:
            raise KeyNotFoundError('key not found')
        self._raise(r['status'], r.get('err_msg', ''))
        return True

    def lrem(self, key: str, count: int, value: str) -> int:
        r = self._list(protocol.CMD_LREM, key, value=value, count=count)
        self._raise(r['status'], r.get('err_msg', ''))
        return int(r['int_result'])

    def ltrim(self, key: str, start: int, stop: int) -> bool:
        r = self._list(protocol.CMD_LTRIM, key, start=start, stop=stop)
        self._raise(r['status'], r.get('err_msg', ''))
        return True

    def linsert(self, key: str, where: str, pivot: str, value: str) -> int:
        before = where.lower() in ('before', 'BEFORE')
        r = self._list(protocol.CMD_LINSERT, key, pivot=pivot, value=value, before=before)
        self._raise(r['status'], r.get('err_msg', ''))
        return int(r['int_result'])

    def blpop(self, key: str, timeout: float = 0) -> Optional[str]:
        ms = int(timeout * 1000)
        r = self._list(protocol.CMD_BLPOP, key, timeout=ms)
        if r['status'] == protocol.STATUS_NOT_FOUND:
            return None
        self._raise(r['status'], r.get('err_msg', ''))
        return r['str_result']

    def brpop(self, key: str, timeout: float = 0) -> Optional[str]:
        ms = int(timeout * 1000)
        r = self._list(protocol.CMD_BRPOP, key, timeout=ms)
        if r['status'] == protocol.STATUS_NOT_FOUND:
            return None
        self._raise(r['status'], r.get('err_msg', ''))
        return r['str_result']

    def lpos(self, key: str, value: str, rank: int = 1,
             count: int = 1, maxlen: int = 0) -> list[int]:
        r = self._list(protocol.CMD_LPOS, key, value=value,
                       rank=rank, count=count, max_len=maxlen)
        self._raise(r['status'], r.get('err_msg', ''))
        return [int(x) for x in r.get('pos_result', [])]

    # ---- Set commands ----

    def sadd(self, key: str, *elements: str) -> int:
        r = self._set(protocol.CMD_SADD, key, elems=list(elements))
        self._raise(r['status'], r.get('err_msg', ''))
        return int(r['changed'])

    def srem(self, key: str, *elements: str) -> int:
        r = self._set(protocol.CMD_SREM, key, elems=list(elements))
        self._raise(r['status'], r.get('err_msg', ''))
        return int(r['changed'])

    def sismember(self, key: str, element: str) -> bool:
        r = self._set(protocol.CMD_SISMEMBER, key, elems=[element])
        self._raise(r['status'], r.get('err_msg', ''))
        return r['is_member']

    def smembers(self, key: str) -> list[str]:
        r = self._set(protocol.CMD_SMEMBERS, key)
        self._raise(r['status'], r.get('err_msg', ''))
        return r.get('elems', [])

    def scard(self, key: str) -> int:
        r = self._set(protocol.CMD_SCARD, key)
        self._raise(r['status'], r.get('err_msg', ''))
        return int(r['card'])

    def spop(self, key: str) -> Optional[str]:
        r = self._set(protocol.CMD_SPOP, key)
        if r['status'] == protocol.STATUS_NOT_FOUND:
            return None
        self._raise(r['status'], r.get('err_msg', ''))
        return r.get('elem')

    def srandmember(self, key: str, count: int = 1) -> list[str]:
        r = self._set(protocol.CMD_SRANDMEMBER, key, count=count)
        self._raise(r['status'], r.get('err_msg', ''))
        return r.get('elems', [])

    def sunion(self, *keys: str) -> list[str]:
        r = self._set(protocol.CMD_SUNION, '', keys=list(keys))
        self._raise(r['status'], r.get('err_msg', ''))
        return r.get('elems', [])

    def sinter(self, *keys: str) -> list[str]:
        r = self._set(protocol.CMD_SINTER, '', keys=list(keys))
        self._raise(r['status'], r.get('err_msg', ''))
        return r.get('elems', [])

    def sdiff(self, *keys: str) -> list[str]:
        r = self._set(protocol.CMD_SDIFF, '', keys=list(keys))
        self._raise(r['status'], r.get('err_msg', ''))
        return r.get('elems', [])
