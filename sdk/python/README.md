# mcache-py

[![PyPI version](https://img.shields.io/pypi/v/mcache-py.svg)](https://pypi.org/project/mcache-py/)
[![Python versions](https://img.shields.io/pypi/pyversions/mcache-py.svg)](https://pypi.org/project/mcache-py/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**mcache** 的官方 Python 客户端 SDK —— 一个高性能内存缓存服务的 redis-py 风格客户端。

> mcache 是用 Go 编写的高性能内存缓存服务，支持 KV / Hash / List / Set 四种数据结构，可嵌入或独立部署。
> 主仓库：[github.com/atoncooper/mcache](https://github.com/atoncooper/mcache)

## 特性

- **零依赖**：纯 Python，仅使用标准库
- **redis-py 风格 API**：熟悉的命名 / 调用方式，迁移成本低
- **类型注解**：完整 PEP 561 类型支持
- **连接池**：内置线程安全的连接池
- **集群客户端**：
  - `ShardClient` — 一致性哈希分片
  - `MasterSlaveClient` — 主写从读
  - `SentinelClient` — 自动故障切换
- **数据结构齐全**：KV、Hash、List、Set 全部支持

## 安装

```bash
pip install mcache-py
```

需要 Python 3.10+。

## 快速开始

```python
from mcache import Client

# 单机连接
with Client(host='127.0.0.1', port=11211) as c:
    c.set('hello', b'world')
    print(c.get('hello'))          # b'world'
    print(c.ping())                # True
```

### Hash

```python
c.hset('user:1', 'name', 'Alice')
c.hset('user:1', 'age', '30')
c.hgetall('user:1')                # {'name': 'Alice', 'age': '30'}
c.hincrby('user:1', 'age', 1)      # 31
```

### List

```python
c.lpush('queue', 'task-1', 'task-2')
c.rpush('queue', 'task-3')
c.lrange('queue', 0, -1)           # ['task-2', 'task-1', 'task-3']
c.lpop('queue')                    # 'task-2'
```

### Set

```python
c.sadd('tags', 'go', 'python', 'rust')
c.sismember('tags', 'python')      # True
c.smembers('tags')                 # ['go', 'python', 'rust']
c.scard('tags')                    # 3
```

### TTL / Key 管理

```python
c.set('session', b'abc123', ttl=300)   # 300 秒后过期
c.ttl('session')                       # 剩余秒数
c.expire('session', 600)               # 重置 TTL
c.persist('session')                   # 移除 TTL
c.exists('session')                    # True
c.type('session')                      # 'string'
```

## 集群模式

### ShardClient — 一致性哈希分片

```python
from mcache import ShardClient

sc = ShardClient([
    ('10.0.0.1', 11211, 2),   # (host, port, weight)
    ('10.0.0.2', 11211, 1),
    ('10.0.0.3', 11211, 1),
])
sc.set('hello', b'world')         # 自动路由到对应节点
sc.close()
```

### MasterSlaveClient — 读写分离

```python
from mcache import MasterSlaveClient

ms = MasterSlaveClient(
    master=('10.0.0.1', 11211),
    slaves=[('10.0.0.2', 11211), ('10.0.0.3', 11211)],
)
ms.set('k', b'v')   # 写 → master
ms.get('k')          # 读 → slave (round-robin)
```

### SentinelClient — 自动故障切换

```python
from mcache import SentinelClient

sc = SentinelClient(
    master=('10.0.0.1', 11211),
    replicas=[('10.0.0.2', 11211), ('10.0.0.3', 11211)],
)
sc.set('k', b'v')           # 自动跟随当前 master
sc.master_addr              # 查看当前 master
```

## 错误处理

```python
from mcache import Client, KeyNotFoundError, ServerError, ConnectionError

try:
    val = c.get('missing-key')   # 返回 None，不抛异常
except KeyNotFoundError as e:
    pass
except ServerError as e:
    print('server error:', e)
except ConnectionError as e:
    print('connection failed:', e)
```

完整异常类型：

| 异常 | 说明 |
|------|------|
| `McacheError` | 所有异常的基类 |
| `ConnectionError` | 连接失败 / 中断 |
| `ReadTimeout` | 读取超时 |
| `KeyNotFoundError` | 键不存在 |
| `ServerError` | 服务端返回错误 |
| `ProtocolError` | 协议异常 / 响应损坏 |
| `InvalidCommandError` | 无效命令 |
| `PoolExhaustedError` | 连接池耗尽 |

## API 参考

### Client(host='127.0.0.1', port=11211, pool_size=4, timeout=10.0)

| 方法 | 说明 |
|------|------|
| `get(key)` | 取值，不存在返回 None |
| `set(key, value, ttl=0)` | 写值，ttl 单位秒 |
| `delete(key)` | 删除键 |
| `len()` | 总条目数 |
| `cleanup()` | 清理过期条目 |
| `ping()` | 连通性检测 |
| `exists(key)`, `type(key)` | 键检查 |
| `expire(key, sec)`, `pexpire(key, ms)` | 设置 TTL |
| `ttl(key)`, `pttl(key)` | 查询 TTL |
| `persist(key)` | 移除 TTL |
| `keys(pattern='*')` | 模式匹配 |

完整命令列表参见 [文档](https://github.com/atoncooper/mcache/blob/master/docs/sdk.md)。

## 启动 mcache 服务

```bash
# 下载 mcache server（Go 二进制）
git clone https://github.com/atoncooper/mcache.git
cd mcache
go build -o bin/mcache ./cmd/mcache
./bin/mcache server --config config.yaml
# 默认监听 127.0.0.1:11211
```

或用 Docker：

```bash
docker run -d -p 11211:11211 atoncooper/mcache:latest
```

## 性能

在本地回环、64 客户端并发、200K 操作下：

| 指标 | mcache | Redis |
|------|--------|-------|
| 吞吐 (ops/s) | ~XXX K | ~XXX K |
| p50 延迟 | XX us | XX us |
| p99 延迟 | XXX us | XXX us |

详见 [性能对比测试](https://github.com/atoncooper/mcache/tree/master/tests/python)。

## 链接

- 主仓库：https://github.com/atoncooper/mcache
- 完整文档：https://github.com/atoncooper/mcache/tree/master/docs
- Issue 反馈：https://github.com/atoncooper/mcache/issues
- 变更日志：[CHANGELOG.md](https://github.com/atoncooper/mcache/blob/master/sdk/python/CHANGELOG.md)

## License

MIT
