#ifndef MCACHE_CLUSTER_HPP
#define MCACHE_CLUSTER_HPP

#include "mcache/client.hpp"

#include <vector>
#include <string>
#include <string_view>
#include <memory>
#include <cstdint>

namespace mcache {

// FNV-1a 32-bit hash — must match Go hash/fnv exactly.
// offset_basis = 2166136261, prime = 16777619
constexpr uint32_t fnv1a_32(std::string_view key) {
    uint32_t hash = 2166136261u;
    for (char c : key) {
        hash ^= (uint32_t)(uint8_t)c;
        hash *= 16777619u;
    }
    return hash;
}

// Cluster client: consistent hashing across multiple mcache nodes.
// Each node gets its own connection pool; key routing uses FNV-1a hash.
template<typename Codec = raw_codec>
class basic_cluster_client {
public:
    // addrs: list of "host:port" strings for each node.
    basic_cluster_client(const std::vector<std::string>& addrs, const options& opts = {})
    {
        clients_.reserve(addrs.size());
        for (const auto& addr : addrs) {
            clients_.push_back(std::make_unique<basic_client<Codec>>(addr, opts));
        }
    }

    basic_cluster_client(const std::vector<std::string>& addrs, Codec codec, const options& opts = {})
    {
        clients_.reserve(addrs.size());
        for (const auto& addr : addrs) {
            clients_.push_back(std::make_unique<basic_client<Codec>>(addr, codec, opts));
        }
    }

    template<typename T>
    result<T> get(std::string_view key) {
        return pick_node(key).template get<T>(key);
    }

    template<typename T>
    result<void> set(std::string_view key, const T& value, std::chrono::milliseconds ttl = {}) {
        return pick_node(key).set(key, value, ttl);
    }

    result<void> del(std::string_view key) {
        return pick_node(key).del(key);
    }

    // Aggregate Len across all nodes.
    result<uint64_t> len() {
        uint64_t total = 0;
        for (auto& c : clients_) {
            auto r = c->len();
            if (!r) return r;
            total += r.value();
        }
        return result<uint64_t>(total);
    }

    // Aggregate cleanup across all nodes.
    result<uint64_t> cleanup() {
        uint64_t total = 0;
        for (auto& c : clients_) {
            auto r = c->cleanup();
            if (!r) return r;
            total += r.value();
        }
        return result<uint64_t>(total);
    }

    void close() {
        for (auto& c : clients_) c->close();
    }

    ~basic_cluster_client() { close(); }

    basic_cluster_client(basic_cluster_client&&) = delete;
    basic_cluster_client& operator=(basic_cluster_client&&) = delete;
    basic_cluster_client(const basic_cluster_client&) = delete;
    basic_cluster_client& operator=(const basic_cluster_client&) = delete;

private:
    basic_client<Codec>& pick_node(std::string_view key) {
        uint32_t h = fnv1a_32(key);
        return *clients_[h % clients_.size()];
    }

    std::vector<std::unique_ptr<basic_client<Codec>>> clients_;
};

using cluster_client = basic_cluster_client<raw_codec>;

} // namespace mcache

#endif // MCACHE_CLUSTER_HPP
