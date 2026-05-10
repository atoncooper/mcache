"""mcache Python SDK — native socket client for the mcache in-memory cache."""

from .client import Client
from .cluster import ShardClient, MasterSlaveClient, SentinelClient
from .errors import (
    McacheError,
    ConnectionError,
    ReadTimeout,
    KeyNotFoundError,
    ServerError,
    ProtocolError,
    InvalidCommandError,
    PoolExhaustedError,
)

__version__ = '1.0.0'
__all__ = [
    'Client',
    'ShardClient',
    'MasterSlaveClient',
    'SentinelClient',
    'McacheError',
    'ConnectionError',
    'ReadTimeout',
    'KeyNotFoundError',
    'ServerError',
    'ProtocolError',
    'InvalidCommandError',
    'PoolExhaustedError',
]
