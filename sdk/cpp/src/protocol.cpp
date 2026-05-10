#include "mcache/protocol.hpp"

#ifdef _WIN32
  #include <winsock2.h>
#else
  #include <arpa/inet.h>
#endif

namespace mcache {

// --- request ---

std::vector<std::byte> request::encode() const {
    uint32_t total = 15 + (uint32_t)key.size() + (uint32_t)value.size();
    std::vector<std::byte> buf(total);

    uint16_t key_len_net = host_to_net16((uint16_t)key.size());
    uint32_t val_len_net = host_to_net32((uint32_t)value.size());
    uint64_t ttl_net     = host_to_net64((uint64_t)ttl_ms);

    buf[0] = static_cast<std::byte>((uint8_t)cmd);
    std::memcpy(&buf[1],  &key_len_net, 2);
    std::memcpy(&buf[3],  &val_len_net, 4);
    std::memcpy(&buf[7],  &ttl_net,     8);
    if (!key.empty())   std::memcpy(&buf[15], key.data(), key.size());
    if (!value.empty()) std::memcpy(&buf[15 + key.size()], value.data(), value.size());
    return buf;
}

request request::decode(std::span<const std::byte> payload) {
    if (payload.size() < 15) throw protocol_error("request payload too short");

    uint16_t key_len_net;
    uint32_t val_len_net;
    uint64_t ttl_net;
    std::memcpy(&key_len_net, &payload[1], 2);
    std::memcpy(&val_len_net, &payload[3], 4);
    std::memcpy(&ttl_net,     &payload[7], 8);

    request req;
    req.cmd    = static_cast<command>((uint8_t)payload[0]);
    req.ttl_ms = (int64_t)net_to_host64(ttl_net);

    uint16_t key_len = net_to_host16(key_len_net);
    uint32_t val_len = net_to_host32(val_len_net);

    if (payload.size() < (size_t)(15 + key_len + val_len))
        throw protocol_error("request payload truncated");

    if (key_len > 0)
        req.key.assign(reinterpret_cast<const char*>(&payload[15]), key_len);
    if (val_len > 0) {
        req.value.resize(val_len);
        std::memcpy(req.value.data(), &payload[15 + key_len], val_len);
    }
    return req;
}

// --- response ---

std::vector<std::byte> response::encode() const {
    uint32_t total = 7 + (uint32_t)value.size() + (uint32_t)err_msg.size();
    std::vector<std::byte> buf(total);

    uint32_t val_len_net = host_to_net32((uint32_t)value.size());
    uint16_t err_len_net = host_to_net16((uint16_t)err_msg.size());

    buf[0] = static_cast<std::byte>((uint8_t)stat);
    std::memcpy(&buf[1], &val_len_net, 4);
    std::memcpy(&buf[5], &err_len_net, 2);
    if (!value.empty())  std::memcpy(&buf[7], value.data(), value.size());
    if (!err_msg.empty()) std::memcpy(&buf[7 + value.size()], err_msg.data(), err_msg.size());
    return buf;
}

response response::decode(std::span<const std::byte> payload) {
    if (payload.size() < 7) throw protocol_error("response payload too short");

    uint32_t val_len_net;
    uint16_t err_len_net;
    std::memcpy(&val_len_net, &payload[1], 4);
    std::memcpy(&err_len_net, &payload[5], 2);

    response resp;
    resp.stat = static_cast<status>((uint8_t)payload[0]);

    uint32_t val_len = net_to_host32(val_len_net);
    uint16_t err_len = net_to_host16(err_len_net);

    if (payload.size() < (size_t)(7 + val_len + err_len))
        throw protocol_error("response payload truncated");

    if (val_len > 0) {
        resp.value.resize(val_len);
        std::memcpy(resp.value.data(), &payload[7], val_len);
    }
    if (err_len > 0)
        resp.err_msg.assign(reinterpret_cast<const char*>(&payload[7 + val_len]), err_len);
    return resp;
}

} // namespace mcache
