#ifndef MCACHE_FRAME_HPP
#define MCACHE_FRAME_HPP

#include <cstdint>
#include <vector>
#include <span>
#include <cstring>
#include <stdexcept>

namespace mcache {

// Frame type
enum class frame_type : uint8_t {
    request  = 0,
    response = 1,
};

// Multiplexed frame (must match Go net/protocol.go Frame exactly).
// Binary layout (big-endian):
//   [0:4]  PayloadLen uint32
//   [4:8]  StreamID   uint32
//   [8]    Type       uint8  (0=request, 1=response)
//   [9]    Flags      uint8
//   [10:]  Payload
struct frame {
    uint32_t              stream_id = 0;
    frame_type            type      = frame_type::request;
    uint8_t               flags     = 0;
    std::vector<std::byte> payload;

    // Serialize to wire format.
    std::vector<std::byte> encode() const;

    // Deserialize from wire format. Throws on invalid data.
    static frame decode(std::span<const std::byte> data);

    // Encode inline helpers
    static constexpr size_t header_size() { return FRAME_HEADER_SIZE; }
};

} // namespace mcache

#endif // MCACHE_FRAME_HPP
