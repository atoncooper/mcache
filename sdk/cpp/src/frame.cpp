#include "mcache/frame.hpp"
#include "mcache/protocol.hpp"

#ifdef _WIN32
  #include <winsock2.h>
#else
  #include <arpa/inet.h>
#endif

namespace mcache {

std::vector<std::byte> frame::encode() const {
    uint32_t plen = (uint32_t)payload.size();
    std::vector<std::byte> buf(FRAME_HEADER_SIZE + plen);

    uint32_t plen_net = host_to_net32(plen);
    uint32_t sid_net  = host_to_net32(stream_id);

    std::memcpy(&buf[0], &plen_net, 4);
    std::memcpy(&buf[4], &sid_net,  4);
    buf[8] = static_cast<std::byte>((uint8_t)type);
    buf[9] = static_cast<std::byte>(flags);
    if (plen > 0)
        std::memcpy(&buf[FRAME_HEADER_SIZE], payload.data(), plen);
    return buf;
}

frame frame::decode(std::span<const std::byte> data) {
    if (data.size() < FRAME_HEADER_SIZE)
        throw protocol_error("frame too short");

    uint32_t plen_net, sid_net;
    std::memcpy(&plen_net, &data[0], 4);
    std::memcpy(&sid_net,  &data[4], 4);

    uint32_t plen = net_to_host32(plen_net);
    if (plen > MAX_PAYLOAD_SIZE)
        throw protocol_error("payload too large");

    if (data.size() < FRAME_HEADER_SIZE + plen)
        throw protocol_error("frame truncated");

    frame f;
    f.stream_id = net_to_host32(sid_net);
    f.type      = static_cast<frame_type>((uint8_t)data[8]);
    f.flags     = (uint8_t)data[9];

    if (plen > 0) {
        f.payload.resize(plen);
        std::memcpy(f.payload.data(), &data[FRAME_HEADER_SIZE], plen);
    }
    return f;
}

} // namespace mcache
