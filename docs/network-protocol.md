# 网络协议

`net` 子包实现了 mcache 的自研多路复用 TCP 帧协议。单条 TCP 连接可承载多个并发 stream 的交错请求/响应。

## 帧结构

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                        PayloadLen (32)                        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                        StreamID (32)                          |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|   Type (8)   |  Flags (8)   |        Payload ...              |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

| 字段 | 大小 | 说明 |
|------|------|------|
| `PayloadLen` | 4 bytes (BigEndian) | 负载字节数，最大 16 MB |
| `StreamID` | 4 bytes (BigEndian) | 连接内区分并发请求/响应的流标识 |
| `Type` | 1 byte | `0` = Request，`1` = Response |
| `Flags` | 1 byte | 保留 |
| `Payload` | N bytes | 命令/响应负载 |

## 命令编码

| 编码 | 命令 | 说明 |
|------|------|------|
| `1` | `CmdGet` | 读取 key |
| `2` | `CmdSet` | 写入 key（携带 TTL，毫秒）|
| `3` | `CmdDel` | 删除 key |
| `4` | `CmdLen` | 当前条目总数 |
| `5` | `CmdCleanup` | 触发过期清理 |

### Request 负载布局

```
[1:Cmd][2:KeyLen][4:ValueLen][8:TTL][Key][Value]
```

- `Cmd`：1 byte，命令编码
- `KeyLen`：2 bytes (BigEndian)，key 长度
- `ValueLen`：4 bytes (BigEndian)，value 长度
- `TTL`：8 bytes (BigEndian)，TTL 毫秒数（0 = 使用默认值）
- `Key`：UTF-8 字符串
- `Value`：原始字节

## 响应状态

| 编码 | 状态 | 说明 |
|------|------|------|
| `0` | `StatusOK` | 成功，Value 字段包含数据 |
| `1` | `StatusErr` | 错误，ErrMsg 字段包含错误信息 |
| `2` | `StatusNotFound` | key 不存在 |

### Response 负载布局

```
[1:Status][4:ValueLen][2:ErrLen][Value][ErrMsg]
```

- `Status`：1 byte，状态编码
- `ValueLen`：4 bytes (BigEndian)，value 长度
- `ErrLen`：2 bytes (BigEndian)，错误消息长度
- `Value`：原始字节（CmdLen/CmdCleanup 返回 8 字节 BigEndian uint64）
- `ErrMsg`：UTF-8 字符串

## 多路复用机制

每条 TCP 连接维护一个 `pending` 映射表（`StreamID → chan *Response`）：

1. 客户端发送请求时分配唯一 `StreamID`（原子递增）
2. 将 `StreamID` 注册到 `pending` 表，等待响应 channel
3. 服务端处理完成后，在响应帧中回传相同的 `StreamID`
4. 客户端的 `readLoop` goroutine 匹配 `StreamID`，投递到对应 channel

这使得单连接可同时承载多个 in-flight 请求，无需为每个请求建立新连接。

## 背压机制

服务端 `jobCh` 是有界 channel（默认容量 65536）。当队列满时：

- 新到达的请求不会入队
- 服务端直接返回 `StatusErr` + `"server overloaded"` 响应
- 客户端正常接收错误，不会阻塞
