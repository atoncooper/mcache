# Changelog

All notable changes to the mcache Go SDK are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] — 2026-05-12

### Added

- 单节点 `Client` — KV / Hash / List / Set 全数据结构支持
- `ClusterClient` — 基于 FNV-1a 32 位哈希的客户端分片
- 连接池(默认 4 条,可通过 `WithPoolSize` 调整)
- 函数式选项:`WithPoolSize` / `WithDialTimeout` / `WithReadTimeout` / `WithWriteTimeout` / `WithCodec` / `WithAddrs`
- 内置 codec:`RawCodec`(默认,字节透传)、`JSONCodec`(JSON 序列化)
- 可自定义 codec:实现 `Codec` interface 即可
- 集群专用 `Stats()` 方法 — 一次性查询所有节点状态
- 阻塞操作:`BLPop` / `BRPop`
- Sentinel 错误支持 `errors.Is`:
  - `ErrKeyNotFound` / `ErrKeyEmpty` / `ErrValueNil`
  - `ErrConnClosed` / `ErrTimeout` / `ErrNoNodes`
  - `ErrNotLeader`(Raft 模式下的写重定向提示)
- 并发安全:`Client` 与 `ClusterClient` 均可在 goroutine 间共享
- 完整的 `pkg.go.dev` 包级文档(`doc.go`)

### Requirements

- Go 1.24+
- mcache server v0.1.x+

[Unreleased]: https://github.com/atoncooper/mcache/compare/sdk/go/v1.0.0...HEAD
[1.0.0]: https://github.com/atoncooper/mcache/releases/tag/sdk%2Fgo%2Fv1.0.0
