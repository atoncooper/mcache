# CLI 命令行工具

`mcache` 是基于 [Cobra](https://github.com/spf13/cobra) 的统一命令行入口，同时包含服务端启动和客户端操作功能。

## 全局选项

| 选项 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--addr` | `-a` | `127.0.0.1:11211` | 服务器地址 |
| `--timeout` | `-t` | `10s` | 操作超时 |
| `--pool` | | `4` | 连接池大小 |

## 子命令一览

### 服务管理

| 命令 | 说明 |
|------|------|
| `mcache server` | 启动 TCP 服务端 |
| `mcache version` | 版本信息 |
| `mcache help` | 帮助 |

### KV 操作

| 命令 | 说明 |
|------|------|
| `mcache get <k>` | 读取键值 |
| `mcache set <k> <v> [--ttl]` | 写入键值 |
| `mcache del <k>` | 删除键 |
| `mcache len` | 总条目数 |
| `mcache cleanup` | 清理过期条目 |
| `mcache ping` | 连通性测试 |
| `mcache stats` | 服务器统计 |

### Hash 操作

| 命令 | 说明 |
|------|------|
| `mcache hset <k> <f> <v>` | 设置字段 |
| `mcache hsetnx <k> <f> <v>` | 仅字段不存在时设置 |
| `mcache hget <k> <f>` | 获取字段值 |
| `mcache hdel <k> <f> [f...]` | 删除字段 |
| `mcache hexists <k> <f>` | 判断字段是否存在 |
| `mcache hgetall <k>` | 获取全部字段和值 |
| `mcache hkeys <k>` | 获取全部字段名 |
| `mcache hvals <k>` | 获取全部值 |
| `mcache hlen <k>` | 字段数量 |
| `mcache hstrlen <k> <f>` | 字段值长度 |
| `mcache hincrby <k> <f> <d>` | 整数增量 |
| `mcache hincrbyfloat <k> <f> <d>` | 浮点增量 |
| `mcache hmget <k> <f> [f...]` | 批量获取 |
| `mcache hmset <k> <f> <v> [f v...]` | 批量设置 |

### List 操作

| 命令 | 说明 |
|------|------|
| `mcache lpush <k> <e> [e...]` | 头部插入 |
| `mcache rpush <k> <e> [e...]` | 尾部插入 |
| `mcache lpop <k>` | 头部弹出 |
| `mcache rpop <k>` | 尾部弹出 |
| `mcache blpop <k> <timeout>` | 阻塞头部弹出（秒） |
| `mcache brpop <k> <timeout>` | 阻塞尾部弹出（秒） |
| `mcache llen <k>` | 列表长度 |
| `mcache lrange <k> <s> <e>` | 范围查询 |
| `mcache lindex <k> <idx>` | 按索引取值 |
| `mcache lset <k> <idx> <v>` | 按索引设置 |
| `mcache lrem <k> <cnt> <v>` | 移除元素 |
| `mcache ltrim <k> <s> <e>` | 范围修剪 |
| `mcache linsert <k> before\|after <p> <v>` | 插入元素 |
| `mcache lpos <k> <v> [rank] [cnt] [maxlen]` | 查找位置 |

### Set 操作

| 命令 | 说明 |
|------|------|
| `mcache sadd <k> <e>` | 添加元素 |
| `mcache srem <k> <e>` | 移除元素 |
| `mcache sismember <k> <e>` | 成员判断 |
| `mcache smembers <k>` | 列出所有元素 |
| `mcache scard <k>` | 元素数量 |
| `mcache spop <k>` | 随机弹出 |
| `mcache srandmember <k> [cnt]` | 随机返回 |
| `mcache sunion <k> [k...]` | 并集 |
| `mcache sinter <k> [k...]` | 交集 |
| `mcache sdiff <k> [k...]` | 差集 |

### Key 管理

| 命令 | 说明 |
|------|------|
| `mcache exists <k>` | 键是否存在 |
| `mcache type <k>` | 键类型 |
| `mcache expire <k> <sec>` | 设置过期（秒） |
| `mcache pexpire <k> <ms>` | 设置过期（毫秒） |
| `mcache ttl <k>` | 剩余 TTL（秒） |
| `mcache pttl <k>` | 剩余 TTL（毫秒） |
| `mcache persist <k>` | 移除过期 |
| `mcache keys <pattern>` | 模式匹配 |

## 启动服务端

```bash
# 默认配置（./config.yaml）
mcache server

# 指定配置文件
mcache server --config /etc/mcache/config.yaml
```

## 客户端操作示例

```bash
# KV
mcache set foo "hello world"
mcache get foo                    # hello world
mcache del foo
mcache set session token --ttl 30m

# Hash
mcache hset user:1 name "alice"
mcache hget user:1 name           # alice
mcache hgetall user:1
mcache hincrby user:1 age 1

# List
mcache lpush tasks "write doc"
mcache rpush tasks "review code"
mcache lrange tasks 0 -1
mcache blpop queue 5              # 阻塞等待 5 秒

# Set
mcache sadd tags go cache
mcache sunion tags other-tags

# Key 管理
mcache exists user:1              # 1
mcache type user:1                # hash
mcache expire user:1 3600
mcache keys "user:*"

# 连接远程节点
mcache -a 192.168.1.10:11211 get foo
mcache -a 10.0.0.1:11211 -t 5s --pool 8 hset user:1 name alice

# 诊断
mcache ping                       # PONG (0.234ms)
mcache stats
mcache len
mcache cleanup
```

## REPL 交互模式

```bash
mcache repl
```

```
mcache dev connected to 127.0.0.1:11211
Type 'help' for available commands, 'exit' to quit.
> hset user:1 name alice
(integer) 1
> hget user:1 name
"alice"
> lpush queue task1 task2
(integer) 2
> lrange queue 0 -1
1) task1
2) task2
> exists user:1
(integer) 1
> type user:1
hash
> keys "user:*"
1) user:1
> exit
Bye.
```

REPL 内建命令覆盖全部 53 个 CLI 命令：`get`, `set`, `del`, `len`, `cleanup`, `ping`, `stats`（KV）；14 个 `h*` 命令（Hash）；14 个 `l*`/`r*`/`b*` 命令（List）；10 个 `s*` 命令（Set）；8 个 Key 管理命令。输入 `help` 查看完整列表。

## 监控命令

```bash
mcache monitor
```

输出当前系统快照：CPU 使用率、内存使用、磁盘 IO、网络流量等。

详见 [可观测性](observability.md#系统监控-monitor)。

## 版本信息

```bash
mcache version
# mcache version 1.0.0 (commit: abc1234, built: 2026-05-09T10:00:00Z, go1.24.3)
```

版本信息通过 ldflags 注入。

## 日志

CLI 所有操作自动记录结构化日志（同时输出到控制台和文件 `logs/mcache-cli.log`），包含：

- 请求命令、目标地址、key
- 操作耗时
- 成功/失败状态
- 错误详情

```json
{"level":"info","ts":"2026-05-09T10:00:00Z","msg":"request done","cmd":"hset","addr":"127.0.0.1:11211","key":"user:1","duration_ms":2,"success":true}
```
