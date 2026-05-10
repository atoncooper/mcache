#ifndef MCACHE_CONNECTION_HPP
#define MCACHE_CONNECTION_HPP

#include "mcache/frame.hpp"
#include "mcache/protocol.hpp"
#include "mcache/errors.hpp"
#include "mcache/options.hpp"

#include <thread>
#include <mutex>
#include <unordered_map>
#include <future>
#include <atomic>
#include <memory>
#include <chrono>

namespace mcache {

// Single multiplexed TCP connection to an mcache server.
// Spawns a background read thread; supports concurrent in-flight requests via StreamID.
class connection {
public:
    // Connect to addr ("host:port").
    explicit connection(const std::string& addr, const options& opts = {});
    ~connection();

    connection(connection&&) = delete;
    connection& operator=(connection&&) = delete;
    connection(const connection&) = delete;
    connection& operator=(const connection&) = delete;

    // Send a request and return a future for the response.
    std::future<response> send(const request& req);

    // Check if the connection is still healthy.
    bool healthy() const noexcept { return !closed_.load(std::memory_order_acquire); }

    // Close the connection.
    void close();

private:
    void read_loop();
    void fail_all_pending(const std::string& err_msg);
    std::vector<std::byte> read_exact_bytes(size_t n);
    void write_exact_bytes(std::span<const std::byte> data);

    // Platform-specific socket handle (pimpl to keep header clean)
    struct impl;
    std::unique_ptr<impl> impl_;

    std::mutex                            write_mutex_;
    std::mutex                            pending_mutex_;
    std::unordered_map<uint32_t, std::promise<response>> pending_;
    std::atomic<uint32_t>                 next_stream_id_{1};
    std::thread                           read_thread_;
    std::atomic<bool>                     closed_{false};
    std::chrono::milliseconds             read_timeout_;
    std::chrono::milliseconds             write_timeout_;
};

} // namespace mcache

#endif // MCACHE_CONNECTION_HPP
