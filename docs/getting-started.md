# Getting Started

## 环境要求

- Go 1.24.3+
- Linux / macOS / Windows

## 安装

```bash
# 作为库引用
go get github.com/atoncooper/mcache

# 同时引用 SDK（独立 module）
go get github.com/atoncooper/mcache/sdk/go
```

## 编译

`mcache` 是统一二进制，使用 Makefile 统一编译。

```bash
make build          # 编译当前平台 → bin/mcache
make build-all      # 全平台交叉编译
make install        # 安装到 GOPATH/bin
```

### 手动编译

```bash
go build -o mcache ./cmd/mcache
```

### 交叉编译

```bash
make build-linux-amd64
make build-linux-arm64
make build-darwin-amd64
make build-darwin-arm64
make build-windows-amd64
```

### 注入版本信息

Makefile 自动注入，手动方式：

```bash
go build -ldflags "\
  -X github.com/atoncooper/mcache/cli.Version=1.0.0 \
  -X github.com/atoncooper/mcache/cli.GitCommit=$(git rev-parse --short HEAD) \
  -X github.com/atoncooper/mcache/cli.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o mcache ./cmd/mcache
```

## 5 分钟快速体验

### 1. 启动服务端

```bash
mcache server --config config.yaml
```

### 2. 使用 CLI 操作

```bash
mcache set hello "world"
mcache get hello          # world
mcache len                # 1
mcache del hello
```

### 3. 作为 Go 库使用

```go
package main

import (
    "fmt"
    "github.com/atoncooper/mcache"
)

func main() {
    opts := mcache.NewOptions().
        WithShards(16).
        WithMaxSize(1000).
        WithEvictionPolicy("lru")

    c, _ := mcache.New(opts)
    defer c.Close()

    c.Set("key", []byte("value"))
    val, _ := c.Get("key")
    fmt.Println(string(val))
}
```

### 4. 使用 Go SDK 连接服务端

```go
package main

import (
    "fmt"
    "time"
    sdk "github.com/atoncooper/mcache/sdk/go"
)

func main() {
    client, _ := sdk.NewClient("127.0.0.1:11211",
        sdk.WithPoolSize(8),
        sdk.WithDialTimeout(3*time.Second),
    )
    defer client.Close()

    client.Set("user:1", []byte("alice"), time.Hour)
    val, _ := client.Get("user:1")
    fmt.Println(string(val))
}
```

## 下一步

- [配置参考](configuration.md) — 完整的 config.yaml 字段说明
- [核心库 API](core-cache.md) — 分片、淘汰策略、动态扩缩容
- [网络协议](network-protocol.md) — TCP 帧协议与命令编码
- [服务端架构](server.md) — worker 池、背压、连接管理
- [Go SDK](sdk.md) — 连接池、Codec、集群客户端
- [CLI 参考](cli.md) — 全部子命令与 REPL
