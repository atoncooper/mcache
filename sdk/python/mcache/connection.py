"""Multiplexed TCP connection with stream-based request/response."""

import socket
import threading
import itertools
import queue

from . import protocol
from .errors import ConnectionError, ReadTimeout, ProtocolError

_MAX_FRAME_HEADER = 10


class Connection:
    """A single multiplexed TCP connection to an mcache server."""

    def __init__(self, host: str, port: int, timeout: float = 10.0):
        self._host = host
        self._port = port
        self._timeout = timeout
        self._sock: socket.socket | None = None
        self._write_lock = threading.Lock()
        self._next_id = itertools.count(1)
        self._pending: dict[int, queue.Queue] = {}
        self._pending_lock = threading.Lock()
        self._reader: threading.Thread | None = None
        self._closed = False

    def connect(self) -> None:
        """Establish the TCP connection and start the reader thread."""
        self._sock = socket.create_connection(
            (self._host, self._port), timeout=self._timeout
        )
        self._sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
        self._closed = False
        self._reader = threading.Thread(target=self._reader_loop, daemon=True)
        self._reader.start()

    def send(self, payload: bytes, read_timeout: float | None = None) -> bytes:
        """Send a request and wait for its response.
        Returns the raw response payload (without the prepended cmd byte for typed responses).
        """
        if self._closed or self._sock is None:
            raise ConnectionError('connection is closed')

        sid = next(self._next_id)
        q: queue.Queue = queue.Queue(maxsize=1)
        with self._pending_lock:
            self._pending[sid] = q

        try:
            frame = protocol.encode_frame(sid, payload)
            with self._write_lock:
                self._sock.sendall(frame)

            timeout = read_timeout if read_timeout is not None else self._timeout
            resp = q.get(timeout=timeout)
            return resp
        finally:
            with self._pending_lock:
                self._pending.pop(sid, None)

    def close(self) -> None:
        """Close the connection."""
        self._closed = True
        if self._sock:
            try:
                self._sock.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
            self._sock.close()
            self._sock = None

    def _reader_loop(self) -> None:
        """Background thread: continuously read frames and dispatch to pending queues."""
        buf = b''
        while not self._closed:
            try:
                while len(buf) < _MAX_FRAME_HEADER:
                    chunk = self._sock.recv(65536)
                    if not chunk:
                        self._close_all_pending(ConnectionError('connection lost'))
                        return
                    buf += chunk

                payload_len, stream_id, ftype, flags = protocol.decode_frame_header(buf[:_MAX_FRAME_HEADER])
                buf = buf[_MAX_FRAME_HEADER:]

                if payload_len > protocol.MAX_PAYLOAD_SIZE:
                    self._close_all_pending(ProtocolError(f'payload too large: {payload_len}'))
                    return

                while len(buf) < payload_len:
                    chunk = self._sock.recv(65536)
                    if not chunk:
                        self._close_all_pending(ConnectionError('connection lost'))
                        return
                    buf += chunk

                payload = buf[:payload_len]
                buf = buf[payload_len:]

                if ftype != protocol.FRAME_TYPE_RESPONSE:
                    continue

                with self._pending_lock:
                    q = self._pending.get(stream_id)
                if q is not None:
                    try:
                        q.put(payload, block=False)
                    except queue.Full:
                        pass

            except OSError as e:
                if not self._closed:
                    self._close_all_pending(ConnectionError(str(e)))
                return

    def _close_all_pending(self, error: Exception) -> None:
        """Push an error into all pending queues and clear them."""
        self._closed = True
        with self._pending_lock:
            for q in self._pending.values():
                try:
                    q.put_nowait(error)
                except queue.Full:
                    pass
            self._pending.clear()
