# Changelog

All notable changes to **mcache-py** are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-05-12

### Added
- Initial PyPI release.
- `Client` — single-node TCP client with redis-py-like API.
- `ShardClient` — consistent-hashing shard client (FNV-1a 32-bit, weighted).
- `MasterSlaveClient` — writes to master, reads round-robin across slaves.
- `SentinelClient` — automatic failover on master loss.
- Full data structure support: **KV / Hash / List / Set**.
- Key management: `exists`, `type`, `expire`, `pexpire`, `ttl`, `pttl`, `persist`, `keys`.
- Built-in connection pool with thread-safety.
- Typed exceptions: `McacheError`, `ConnectionError`, `ReadTimeout`,
  `KeyNotFoundError`, `ServerError`, `ProtocolError`, `InvalidCommandError`,
  `PoolExhaustedError`.
- PEP 561 typing support (`py.typed` marker).

[1.0.0]: https://github.com/atoncooper/mcache/releases/tag/py-v1.0.0
