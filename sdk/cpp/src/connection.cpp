#include "mcache/connection.hpp"
#include "platform.hpp"

namespace mcache {

struct connection::impl {
    detail::socket_handle sock;
    detail::wsa_guard     wsa; // must be first destroyed last
};

connection::connection(const std::string& addr, const options& opts)
    : read_timeout_(opts.read_timeout)
    , write_timeout_(opts.write_timeout)
{
    // Parse host:port
    std::string host, port = "11211";
    auto colon = addr.rfind(':');
    if (colon != std::string::npos) {
        host = addr.substr(0, colon);
        port = addr.substr(colon + 1);
    } else {
        host = addr;
    }

    impl_ = std::make_unique<impl>();
    impl_->sock = detail::connect_with_timeout(host, port,
        static_cast<int>(opts.connect_timeout.count()));

    read_thread_ = std::thread(&connection::read_loop, this);
}

connection::~connection() {
    close();
}

void connection::close() {
    bool expected = false;
    if (!closed_.compare_exchange_strong(expected, true)) return;

    impl_->sock.close(); // unblocks read_loop
    if (read_thread_.joinable())
        read_thread_.join();
}

std::future<response> connection::send(const request& req) {
    if (closed_.load(std::memory_order_acquire))
        throw connection_error("connection closed");

    uint32_t sid = next_stream_id_.fetch_add(1, std::memory_order_relaxed);
    if (sid == 0) sid = next_stream_id_.fetch_add(1, std::memory_order_relaxed);

    std::promise<response> prom;
    auto fut = prom.get_future();

    {
        std::lock_guard<std::mutex> lk(pending_mutex_);
        pending_[sid] = std::move(prom);
    }

    // Encode frame
    auto payload = req.encode();
    frame f{ sid, frame_type::request, 0, std::move(payload) };
    auto wire = f.encode();

    // Write under write mutex
    {
        std::lock_guard<std::mutex> lk(write_mutex_);
        try {
            detail::write_exact(impl_->sock.native(),
                std::span<const std::byte>(wire.data(), wire.size()),
                static_cast<int>(write_timeout_.count()));
        } catch (...) {
            std::lock_guard<std::mutex> plk(pending_mutex_);
            auto it = pending_.find(sid);
            if (it != pending_.end()) {
                it->second.set_exception(std::current_exception());
                pending_.erase(it);
            }
            throw;
        }
    }

    return fut;
}

void connection::read_loop() {
    try {
        while (!closed_.load(std::memory_order_acquire)) {
            // Read frame header
            std::byte header_buf[FRAME_HEADER_SIZE];
            detail::read_exact(impl_->sock.native(),
                std::span<std::byte>(header_buf, FRAME_HEADER_SIZE),
                static_cast<int>(read_timeout_.count()));

            auto f = frame::decode(std::span<const std::byte>(header_buf, FRAME_HEADER_SIZE));

            // Read payload
            if (!f.payload.empty()) {
                detail::read_exact(impl_->sock.native(),
                    std::span<std::byte>(f.payload.data(), f.payload.size()),
                    static_cast<int>(read_timeout_.count()));
            }

            if (f.type != frame_type::response) continue;

            auto resp = response::decode(std::span<const std::byte>(f.payload.data(), f.payload.size()));

            // Deliver to pending promise
            std::lock_guard<std::mutex> lk(pending_mutex_);
            auto it = pending_.find(f.stream_id);
            if (it != pending_.end()) {
                it->second.set_value(std::move(resp));
                pending_.erase(it);
            }
        }
    } catch (const std::exception& e) {
        fail_all_pending(e.what());
    }
}

void connection::fail_all_pending(const std::string& err_msg) {
    std::lock_guard<std::mutex> lk(pending_mutex_);
    auto ex = std::make_exception_ptr(connection_error(err_msg));
    for (auto& [sid, prom] : pending_) {
        prom.set_exception(ex);
    }
    pending_.clear();
}

} // namespace mcache
