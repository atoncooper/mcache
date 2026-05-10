#ifndef MCACHE_CLIENT_HPP
#define MCACHE_CLIENT_HPP

#include "mcache/connection_pool.hpp"
#include "mcache/protocol.hpp"
#include "mcache/errors.hpp"
#include "mcache/options.hpp"
#include "mcache/codec.hpp"

#include <string_view>
#include <chrono>
#include <memory>

namespace mcache {

// High-level client wrapping a connection pool with codec support.
// Template parameter Codec defaults to raw_codec.
template<typename Codec = raw_codec>
class basic_client {
public:
    basic_client(const std::string& addr, const options& opts = {})
        : pool_(std::make_unique<connection_pool>(addr, opts))
        , codec_() {}

    basic_client(const std::string& addr, Codec codec, const options& opts = {})
        : pool_(std::make_unique<connection_pool>(addr, opts))
        , codec_(std::move(codec)) {}

    // Get decodes the value for key using the codec.
    // Returns result containing the decoded value or an error.
    template<typename T>
    result<T> get(std::string_view key) {
        request req;
        req.cmd = command::get;
        req.key = std::string(key);

        auto fut = pool_->next().send(req);
        try {
            auto resp = fut.get();
            if (resp.stat == status::not_found)
                return result<T>(errc::not_found, std::string(key));
            if (resp.stat != status::ok)
                return result<T>(errc::protocol, resp.err_msg);

            T value;
            codec_.decode(resp.value, value);
            return result<T>(std::move(value));
        } catch (const error& e) {
            return result<T>(e.code(), e.what());
        } catch (const std::exception& e) {
            return result<T>(errc::io_error, e.what());
        }
    }

    // Set encodes the value and stores it with optional TTL.
    template<typename T>
    result<void> set(std::string_view key, const T& value, std::chrono::milliseconds ttl = {}) {
        request req;
        req.cmd    = command::set;
        req.key    = std::string(key);
        req.value  = codec_.encode(value);
        req.ttl_ms = ttl.count();

        auto fut = pool_->next().send(req);
        try {
            auto resp = fut.get();
            if (resp.stat != status::ok)
                return result<void>(errc::protocol, resp.err_msg);
            return {};
        } catch (const error& e) {
            return result<void>(e.code(), e.what());
        } catch (const std::exception& e) {
            return result<void>(errc::io_error, e.what());
        }
    }

    // Del removes a key.
    result<void> del(std::string_view key) {
        request req;
        req.cmd = command::del;
        req.key = std::string(key);

        auto fut = pool_->next().send(req);
        try {
            auto resp = fut.get();
            if (resp.stat != status::ok)
                return result<void>(errc::protocol, resp.err_msg);
            return {};
        } catch (const error& e) {
            return result<void>(e.code(), e.what());
        } catch (const std::exception& e) {
            return result<void>(errc::io_error, e.what());
        }
    }

    // Len returns the total number of entries.
    result<uint64_t> len() {
        request req;
        req.cmd = command::len;

        auto fut = pool_->next().send(req);
        try {
            auto resp = fut.get();
            if (resp.stat != status::ok)
                return result<uint64_t>(errc::protocol, resp.err_msg);
            if (resp.value.size() < 8)
                return result<uint64_t>(errc::protocol, "bad response");
            uint64_t val;
            std::memcpy(&val, resp.value.data(), 8);
            return result<uint64_t>(net_to_host64(val));
        } catch (const error& e) {
            return result<uint64_t>(e.code(), e.what());
        } catch (const std::exception& e) {
            return result<uint64_t>(errc::io_error, e.what());
        }
    }

    // Cleanup removes expired entries and returns count.
    result<uint64_t> cleanup() {
        request req;
        req.cmd = command::cleanup;

        auto fut = pool_->next().send(req);
        try {
            auto resp = fut.get();
            if (resp.stat != status::ok)
                return result<uint64_t>(errc::protocol, resp.err_msg);
            if (resp.value.size() < 8)
                return result<uint64_t>(errc::protocol, "bad response");
            uint64_t val;
            std::memcpy(&val, resp.value.data(), 8);
            return result<uint64_t>(net_to_host64(val));
        } catch (const error& e) {
            return result<uint64_t>(e.code(), e.what());
        } catch (const std::exception& e) {
            return result<uint64_t>(errc::io_error, e.what());
        }
    }

    void close() { pool_->close(); }

private:
    std::unique_ptr<connection_pool> pool_;
    Codec codec_;
};

using client = basic_client<raw_codec>;

} // namespace mcache

#endif // MCACHE_CLIENT_HPP
