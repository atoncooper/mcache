"""Round-robin connection pool."""

import threading

from .connection import Connection


class ConnectionPool:
    """A thread-safe round-robin pool of multiplexed TCP connections."""

    def __init__(self, host: str, port: int, size: int = 4, timeout: float = 10.0):
        if size < 1:
            raise ValueError('pool size must be >= 1')
        self._conns = [Connection(host, port, timeout) for _ in range(size)]
        self._lock = threading.Lock()
        self._idx = 0

    def connect(self) -> None:
        """Connect all pooled connections."""
        for c in self._conns:
            c.connect()

    def get_connection(self) -> Connection:
        """Return the next connection (round-robin)."""
        with self._lock:
            c = self._conns[self._idx]
            self._idx = (self._idx + 1) % len(self._conns)
            return c

    def close(self) -> None:
        """Close all connections."""
        for c in self._conns:
            c.close()

    @property
    def size(self) -> int:
        return len(self._conns)
