# 服务端架构

`net.Server` 是 mcache 的 TCP 服务端，基于多路复用帧协议实现高并发处理。

## 架构概览

```
        ┌──────────────┐  Accept   ┌──────────────┐
        │   Listener   │─────────► │ handleConn N │
        └──────────────┘           └──────┬───────┘
                                          │ DecodeFrame
                                          ▼
                              ┌──────────────────────┐
                              │  jobCh (buffered)    │
                              └──────────┬───────────┘
                                         │
                  ┌──────────┬───────────┼───────────┬──────────┐
                  ▼          ▼           ▼           ▼          ▼
              ┌───────┐  ┌───────┐   ┌───────┐  ┌───────┐  ┌───────┐
              │Worker │  │Worker │   │Worker │  │Worker │  │Worker │
              └───────┘  └───────┘   └───────┘  └───────┘  └───────┘
                  │          │           │           │          │
                  └──────────┴────►cache.Get/Set/Del◄┴──────────┘
```

核心设计原则：**连接数与 goroutine 数解耦**。N 个 TCP 连接 + M 个固定 worker，避免瞬时连接突增导致 goroutine 数量失控。

## Server 结构

```go
type Server struct {
    cache       *mcache.Cache
    listener    net.Listener
    jobCh       chan *job        // 有界 job 队列（默认 65536）
    workerCount int              // 固定 worker 数（默认 256）
    maxConns    int              // 最大连接数（默认 100000）
    readTimeout time.Duration    // 帧读取超时（默认 30s）
    conns       map[*serverConn]struct{}
}
```

## 创建与启动

```go
import (
    "github.com/atoncooper/mcache"
    mnet "github.com/atoncooper/mcache/net"
    "time"
)

c, _ := mcache.New(mcache.NewOptions().WithShards(16))

srv := mnet.NewServer(c,
    mnet.WithWorkers(256),
    mnet.WithMaxConns(100000),
    mnet.WithReadTimeout(30*time.Second),
    mnet.WithErrorLog(func(format string, v ...any) {
        log.Printf(format, v...)
    }),
)

// 阻塞直到关闭
srv.ListenAndServe(":11211")
```

## ServerOption 参考

| Option | 默认值 | 说明 |
|--------|--------|------|
| `WithWorkers(n)` | `256` | 固定 worker goroutine 数 |
| `WithMaxConns(n)` | `100000` | 最大并发 TCP 连接数，超限直接关闭 |
| `WithReadTimeout(d)` | `30s` | 单帧读取超时 |
| `WithErrorLog(fn)` | `nil` | 自定义 accept 错误日志（nil = 静默丢弃） |

## 请求处理流程

1. **Accept**：Listener 接受新 TCP 连接，检查 `maxConns` 限制
2. **handleConn**：每个连接启动一个 `handleConn` goroutine
   - 循环调用 `DecodeFrame` 读取帧
   - 只接受 `FrameTypeRequest` 类型的帧
   - 解码请求负载得到 `Request`
   - 投递 `job{conn, streamID, request}` 到 `jobCh`
   - 若 `jobCh` 已满，直接返回 `"server overloaded"` 错误
3. **worker**：M 个固定 goroutine 从 `jobCh` 消费
   - 调用 `s.process(req)` 执行缓存操作
   - 通过 `writeResponse` 写回响应帧
4. **writeResponse**：获取 `serverConn.writeMu` 写锁，确保帧完整性

## 响应写入的并发安全

每个 `serverConn` 持有独立的 `writeMu`。多个 worker 可能同时向同一连接写响应，`writeMu` 确保帧不会交错。读取侧只有 `handleConn` goroutine 操作，无需锁。

## 优雅关闭

```go
srv.Close()  // 非阻塞
```

Close 流程：
1. 设置 `closed` 标志
2. 关闭 Listener，停止 Accept
3. 关闭所有活跃连接的 TCP socket
4. 等待所有 `handleConn` goroutine 退出（`wg.Wait()`）
5. 关闭 `jobCh`，worker 自然退出

## 连接生命周期

```
Accept → handleConn (goroutine)
  ├── 循环读帧 → 投递 jobCh
  ├── 读错误/EOF → return
  └── defer: 从 conns map 移除 + 关闭 TCP socket
```

## 错误处理

| 场景 | 行为 |
|------|------|
| Accept 错误（非关闭） | 记录日志，继续 Accept |
| 帧解码错误 | 关闭该连接 |
| 请求负载解码错误 | 丢弃该帧，继续读取 |
| jobCh 满 | 返回 `server overloaded` 错误 |
| Cache 操作错误 | 返回 `StatusErr` + 错误信息 |
| key 不存在 | 返回 `StatusNotFound` |
