# mcache C++ SDK

现代 C++17 客户端库，用于连接 mcache 服务端。支持多路复用连接池、Codec 抽象和集群。

## 编译

### CMake

```bash
mkdir build && cd build
cmake ..
make
```

可选：启用 JSON Codec（需要 nlohmann/json）

```bash
cmake .. -Dnlohmann_json_DIR=/path/to/nlohmann_json
```

## 快速开始

### 单节点

```cpp
#include <mcache/mcache.hpp>

int main() {
    mcache::client client("127.0.0.1:11211");

    client.set("hello", std::string("world"), std::chrono::minutes(1));

    auto r = client.get<std::string>("hello");
    if (r) std::cout << r.value() << std::endl; // "world"

    client.del("hello");
    return 0;
}
```

### 集群

```cpp
mcache::cluster_client cluster({
    "10.0.0.1:11211",
    "10.0.0.2:11211",
    "10.0.0.3:11211",
});

// key 通过 FNV-1a 一致性哈希自动路由
cluster.set("user:42", user_data, std::chrono::hours(1));
auto r = cluster.get<UserData>("user:42");
```

### JSON Codec

```cpp
#define MCACHE_USE_NLOHMANN_JSON
#include <mcache/mcache.hpp>

mcache::json_codec codec;
mcache::basic_client<mcache::json_codec> client("127.0.0.1:11211", codec);

client.set("user", std::map<std::string, std::string>{{"name", "alice"}});
auto r = client.get<std::map<std::string, std::string>>("user");
```

## API 概览

### client

`template<typename Codec = raw_codec> class basic_client`

| 方法 | 说明 |
|------|------|
| `set(key, value, ttl)` | 写入（自动编码） |
| `get<T>(key)` → `result<T>` | 读取（自动解码） |
| `del(key)` → `result<void>` | 删除 |
| `len()` → `result<uint64_t>` | 条目数 |
| `cleanup()` → `result<uint64_t>` | 清理过期 |
| `close()` | 关闭连接池 |

### cluster_client

| 方法 | 说明 |
|------|------|
| `set(key, value, ttl)` | 写入（一致性哈希路由） |
| `get<T>(key)` | 读取 |
| `del(key)` | 删除 |
| `len()` | 汇总所有节点 |
| `cleanup()` | 汇总所有节点 |

### 异常类型

| 类 | 说明 |
|----|------|
| `mcache::error` | 基类 |
| `mcache::not_found_error` | key 不存在 |
| `mcache::timeout_error` | 操作超时 |
| `mcache::connection_error` | 连接失败 |
| `mcache::protocol_error` | 协议错误 |

### result<T>

无异常 API：

```cpp
auto r = client.get<std::string>("key");
if (r) {
    std::cout << r.value() << std::endl;
} else {
    std::cerr << r.error_msg() << std::endl;
}
```

## 线程安全

- `basic_client` 内部使用连接池，多线程安全
- 每个连接独立 read thread + write mutex
- 集群客户端通过 FNV-1a 哈希确定路由节点，线程安全

## 依赖

- C++17 编译器
- 无外部库依赖（JSON Codec 可选依赖 nlohmann/json）
- Linux: `-lpthread`
- Windows: `ws2_32.lib`

## 平台支持

- Linux (x86_64, ARM64)
- macOS (x86_64, ARM64)
- Windows (MSVC 2019+)
