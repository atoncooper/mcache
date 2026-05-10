# mcache C SDK

C 客户端库，用于连接 mcache 服务端。

## 编译

### CMake（推荐）

```bash
mkdir build && cd build
cmake ..
make
```

### Makefile

```bash
make          # 编译静态库和动态库 → lib/
make test     # 编译并运行测试
make install  # 安装到 /usr/local
```

## 快速开始

```c
#include <mcache/mcache.h>

int main(void) {
    mcache_conn_t* conn = mcache_connect("127.0.0.1:11211", 5000);
    if (!conn) return 1;

    mcache_set(conn, "hello", (const uint8_t*)"world", 5, 0);

    uint8_t* val = NULL;
    uint32_t len = 0;
    mcache_get(conn, "hello", &val, &len);
    printf("%.*s\n", (int)len, val);
    mcache_free(val);

    mcache_disconnect(conn);
    return 0;
}
```

## API 概览

| 函数 | 说明 |
|------|------|
| `mcache_connect(addr, timeout_ms)` | 连接服务端 |
| `mcache_disconnect(conn)` | 断开连接 |
| `mcache_get(conn, key, &val, &len)` | 读取 key |
| `mcache_set(conn, key, val, len, ttl_ms)` | 写入 key |
| `mcache_del(conn, key)` | 删除 key |
| `mcache_len(conn, &count)` | 条目数 |
| `mcache_cleanup(conn, &removed)` | 清理过期 |
| `mcache_ping(conn, &rtt_us)` | 连通性测试 |
| `mcache_last_error(conn)` | 服务端错误消息 |
| `mcache_error_string(err)` | 错误码描述 |
| `mcache_free(ptr)` | 释放 mcache_get 分配的内存 |
| `mcache_set_timeout(conn, read_ms, write_ms)` | 设置超时 |

所有函数返回 `mcache_error_t`（0 = 成功，负数 = 错误）。

## 线程安全

每个连接内部持有互斥锁，多线程共享同一连接是安全的（串行化），但推荐多线程场景下各自创建独立连接以获得更好并发性能。

## 依赖

- C11 编译器
- Linux: `-lpthread`（pthread 基本所有系统都自带）
- Windows: `ws2_32.lib`（系统自带）
- macOS: 无额外依赖（pthread 在 libSystem 中）

## 平台支持

- Linux (x86_64, ARM64)
- macOS (x86_64, ARM64)
- Windows (MSVC / MinGW)
