#ifndef MCACHE_ERRORS_HPP
#define MCACHE_ERRORS_HPP

#include <stdexcept>
#include <string>
#include <variant>
#include <system_error>

namespace mcache {

// Strongly-typed error codes
enum class errc {
    ok          = 0,
    connect     = 1,
    timeout     = 2,
    closed      = 3,
    not_found   = 4,
    protocol    = 5,
    too_large   = 6,
    invalid     = 7,
    io_error    = 8,
    memory      = 9,
};

// Base exception for all mcache errors
class error : public std::runtime_error {
public:
    explicit error(errc code, const std::string& msg = "")
        : std::runtime_error(msg.empty() ? to_string(code) : msg)
        , code_(code) {}

    errc code() const noexcept { return code_; }

    static const char* to_string(errc c) noexcept {
        switch (c) {
            case errc::ok:          return "success";
            case errc::connect:     return "connection failed";
            case errc::timeout:     return "operation timed out";
            case errc::closed:      return "connection closed";
            case errc::not_found:   return "key not found";
            case errc::protocol:    return "protocol error";
            case errc::too_large:   return "payload too large";
            case errc::invalid:     return "invalid argument";
            case errc::io_error:    return "I/O error";
            case errc::memory:      return "out of memory";
            default:                return "unknown error";
        }
    }

private:
    errc code_;
};

// Specific exception types for catch-by-type
class not_found_error : public error {
public:
    explicit not_found_error(const std::string& key = "")
        : error(errc::not_found, "key not found: " + key) {}
};

class timeout_error : public error {
public:
    timeout_error() : error(errc::timeout) {}
};

class connection_error : public error {
public:
    explicit connection_error(const std::string& msg = "")
        : error(errc::connect, msg.empty() ? "connection failed" : msg) {}
};

class protocol_error : public error {
public:
    explicit protocol_error(const std::string& msg = "")
        : error(errc::protocol, msg.empty() ? "protocol error" : msg) {}
};

// result<T> — C++17 alternative to std::expected for APIs that can't throw.
// Holds either a value T or an errc + message.
template<typename T>
class result {
public:
    // Success constructor
    result(T value) : storage_(std::move(value)) {}

    // Error constructor
    result(errc code, std::string msg = "")
        : storage_(error_info{code, std::move(msg)}) {}

    bool has_value() const noexcept { return std::holds_alternative<T>(storage_); }
    explicit operator bool() const noexcept { return has_value(); }

    T& value() {
        if (!has_value()) throw error(get_error().code, get_error().msg);
        return std::get<T>(storage_);
    }

    const T& value() const {
        if (!has_value()) throw error(get_error().code, get_error().msg);
        return std::get<T>(storage_);
    }

    errc error_code() const noexcept {
        return has_value() ? errc::ok : get_error().code;
    }

    const std::string& error_msg() const {
        static const std::string empty;
        return has_value() ? empty : get_error().msg;
    }

private:
    struct error_info { errc code; std::string msg; };
    const error_info& get_error() const { return std::get<error_info>(storage_); }
    std::variant<T, error_info> storage_;
};

// void result specialization
template<>
class result<void> {
public:
    result() : err_(errc::ok) {}
    result(errc code, std::string msg = "")
        : err_(code), msg_(std::move(msg)) {}

    bool has_value() const noexcept { return err_ == errc::ok; }
    explicit operator bool() const noexcept { return has_value(); }
    errc error_code() const noexcept { return err_; }
    const std::string& error_msg() const { return msg_; }

private:
    errc err_ = errc::ok;
    std::string msg_;
};

} // namespace mcache

#endif // MCACHE_ERRORS_HPP
