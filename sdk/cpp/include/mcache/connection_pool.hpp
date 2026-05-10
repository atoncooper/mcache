#ifndef MCACHE_CONNECTION_POOL_HPP
#define MCACHE_CONNECTION_POOL_HPP

#include "mcache/connection.hpp"
#include "mcache/options.hpp"

#include <vector>
#include <atomic>
#include <memory>

namespace mcache {

// Round-robin connection pool.
class connection_pool {
public:
    connection_pool(const std::string& addr, const options& opts = {})
        : addr_(addr), opts_(opts)
    {
        conns_.reserve((size_t)opts.pool_size);
        for (int i = 0; i < opts.pool_size; ++i) {
            conns_.push_back(std::make_unique<connection>(addr, opts));
        }
    }

    // Get the next connection in round-robin order.
    connection& next() {
        uint32_t idx = counter_.fetch_add(1, std::memory_order_relaxed) % (uint32_t)conns_.size();
        return *conns_[idx];
    }

    void close() {
        for (auto& c : conns_) c->close();
    }

    bool healthy() const noexcept {
        for (auto& c : conns_) if (!c->healthy()) return false;
        return true;
    }

    ~connection_pool() { close(); }

    connection_pool(connection_pool&&) = delete;
    connection_pool& operator=(connection_pool&&) = delete;
    connection_pool(const connection_pool&) = delete;
    connection_pool& operator=(const connection_pool&) = delete;

private:
    std::string addr_;
    options opts_;
    std::vector<std::unique_ptr<connection>> conns_;
    std::atomic<uint32_t> counter_{0};
};

} // namespace mcache

#endif // MCACHE_CONNECTION_POOL_HPP
