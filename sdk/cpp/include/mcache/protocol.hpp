#ifndef MCACHE_PROTOCOL_HPP
#define MCACHE_PROTOCOL_HPP

#include <cstdint>
#include <vector>
#include <string>
#include <string_view>
#include <span>
#include <cstring>
#include <stdexcept>

namespace mcache {

// Commands (must match Go net/protocol.go exactly)
enum class command : uint8_t {
    get     = 1,
    set     = 2,
    del     = 3,
    len     = 4,
    cleanup = 5,
};

// Response statuses
enum class status : uint8_t {
    ok        = 0,
    error     = 1,
    not_found = 2,
};

// Limits
constexpr uint32_t MAX_PAYLOAD_SIZE = 16 * 1024 * 1024; // 16 MB
constexpr size_t   FRAME_HEADER_SIZE = 10;

// Request payload layout (big-endian, must match Go):
//   [0]    Cmd      uint8
//   [1:3]  KeyLen   uint16
//   [3:7]  ValueLen uint32
//   [7:15] TTL      int64 (milliseconds, 0 = default)
//   [15:]  Key      bytes
//   [15+KeyLen:] Value bytes
struct request {
    command     cmd      = command::get;
    std::string key;
    std::vector<std::byte> value;
    int64_t     ttl_ms   = 0;

    std::vector<std::byte> encode() const;
    static request decode(std::span<const std::byte> payload);
};

// Response payload layout (big-endian):
//   [0]    Status   uint8
//   [1:5]  ValueLen uint32
//   [5:7]  ErrLen   uint16
//   [7:]   Value    bytes
//   [7+ValueLen:] ErrMsg bytes
struct response {
    status      stat     = status::ok;
    std::vector<std::byte> value;
    std::string err_msg;

    std::vector<std::byte> encode() const;
    static response decode(std::span<const std::byte> payload);

    bool ok() const { return stat == status::ok; }
};

// Helpers for big-endian serialization (mirrors encoding/binary in Go)
inline uint16_t host_to_net16(uint16_t v) { return htons(v); }
inline uint32_t host_to_net32(uint32_t v) { return htonl(v); }
inline uint64_t host_to_net64(uint64_t v) {
    return ((uint64_t)htonl((uint32_t)(v >> 32))) |
           ((uint64_t)htonl((uint32_t)(v & 0xFFFFFFFF)) << 32);
}
inline uint16_t net_to_host16(uint16_t v) { return ntohs(v); }
inline uint32_t net_to_host32(uint32_t v) { return ntohl(v); }
inline uint64_t net_to_host64(uint64_t v) { return host_to_net64(v); }

} // namespace mcache

#endif // MCACHE_PROTOCOL_HPP
