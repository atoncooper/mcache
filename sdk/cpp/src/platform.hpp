// Internal: platform abstraction for socket I/O (not part of public API)
#ifndef MCACHE_PLATFORM_IMPL_HPP
#define MCACHE_PLATFORM_IMPL_HPP

#ifdef _WIN32
  #ifndef _WIN32_WINNT
    #define _WIN32_WINNT 0x0600
  #endif
  #include <winsock2.h>
  #include <ws2tcpip.h>
  #pragma comment(lib, "Ws2_32.lib")
#else
  #include <sys/socket.h>
  #include <netinet/in.h>
  #include <netinet/tcp.h>
  #include <arpa/inet.h>
  #include <netdb.h>
  #include <unistd.h>
  #include <fcntl.h>
  #include <errno.h>
#endif

#include <chrono>
#include <string>
#include <stdexcept>
#include <span>
#include <cstring>
#include <vector>
#include <cstddef>

namespace mcache::detail {

// Socket handle (RAII, move-only)
class socket_handle {
public:
#ifdef _WIN32
    using native_t = SOCKET;
    static constexpr native_t invalid = INVALID_SOCKET;
#else
    using native_t = int;
    static constexpr native_t invalid = -1;
#endif

    socket_handle() = default;
    explicit socket_handle(native_t fd) : fd_(fd) {}

    ~socket_handle() { close(); }

    socket_handle(socket_handle&& other) noexcept : fd_(other.fd_) { other.fd_ = invalid; }
    socket_handle& operator=(socket_handle&& other) noexcept {
        if (this != &other) { close(); fd_ = other.fd_; other.fd_ = invalid; }
        return *this;
    }

    socket_handle(const socket_handle&) = delete;
    socket_handle& operator=(const socket_handle&) = delete;

    native_t native() const { return fd_; }
    bool valid() const { return fd_ != invalid; }

    void close() {
        if (valid()) {
#ifdef _WIN32
            closesocket(fd_);
#else
            ::close(fd_);
#endif
            fd_ = invalid;
        }
    }

private:
    native_t fd_ = invalid;
};

// WSA init guard (Windows only)
class wsa_guard {
public:
    wsa_guard() {
#ifdef _WIN32
        WSADATA wsa;
        WSAStartup(MAKEWORD(2,2), &wsa);
#endif
    }
    ~wsa_guard() {
#ifdef _WIN32
        WSACleanup();
#endif
    }
};

// Set socket to non-blocking
inline void set_nonblock(socket_handle::native_t fd) {
#ifdef _WIN32
    u_long mode = 1;
    ioctlsocket(fd, FIONBIO, &mode);
#else
    fcntl(fd, F_SETFL, fcntl(fd, F_GETFL, 0) | O_NONBLOCK);
#endif
}

// Set socket timeout
inline void set_sock_timeout(socket_handle::native_t fd, int ms, bool is_read) {
    int optname = is_read ? SO_RCVTIMEO : SO_SNDTIMEO;
#ifdef _WIN32
    DWORD tv = (DWORD)ms;
    setsockopt(fd, SOL_SOCKET, optname, (const char*)&tv, sizeof(tv));
#else
    struct timeval tv;
    tv.tv_sec  = ms / 1000;
    tv.tv_usec = (ms % 1000) * 1000;
    setsockopt(fd, SOL_SOCKET, optname, &tv, sizeof(tv));
#endif
}

// Connect with timeout
inline socket_handle connect_with_timeout(const std::string& host, const std::string& port, int timeout_ms) {
    struct addrinfo hints = {}, *result = nullptr;
    hints.ai_family   = AF_UNSPEC;
    hints.ai_socktype = SOCK_STREAM;
    hints.ai_protocol = IPPROTO_TCP;

    if (getaddrinfo(host.c_str(), port.c_str(), &hints, &result) != 0)
        throw std::runtime_error("getaddrinfo failed for " + host + ":" + port);

    socket_handle::native_t fd = socket_handle::invalid;
    for (auto* rp = result; rp; rp = rp->ai_next) {
        fd = socket(rp->ai_family, rp->ai_socktype, rp->ai_protocol);
        if (fd == socket_handle::invalid) continue;

        set_nonblock(fd);
        int ret = connect(fd, rp->ai_addr, (int)rp->ai_addrlen);
        if (ret == 0) break;

#ifdef _WIN32
        if (WSAGetLastError() == WSAEWOULDBLOCK) {
#else
        if (errno == EINPROGRESS) {
#endif
            fd_set wfds;
            FD_ZERO(&wfds);
            FD_SET(fd, &wfds);
            struct timeval tv;
            tv.tv_sec  = timeout_ms / 1000;
            tv.tv_usec = (timeout_ms % 1000) * 1000;
            ret = select((int)(fd + 1), nullptr, &wfds, nullptr, &tv);
            if (ret > 0) {
                int so_err = 0;
                socklen_t len = sizeof(so_err);
                getsockopt(fd, SOL_SOCKET, SO_ERROR, (char*)&so_err, &len);
                if (so_err == 0) break;
            }
        }
#ifdef _WIN32
        closesocket(fd);
#else
        ::close(fd);
#endif
        fd = socket_handle::invalid;
    }
    freeaddrinfo(result);

    if (fd == socket_handle::invalid)
        throw std::runtime_error("connect failed to " + host + ":" + port);

    int yes = 1;
    setsockopt(fd, IPPROTO_TCP, TCP_NODELAY, (const char*)&yes, sizeof(yes));
    return socket_handle(fd);
}

// Read/write exact byte counts with timeout
inline void read_exact(socket_handle::native_t fd, std::span<std::byte> buf, int timeout_ms) {
    size_t total = 0;
    while (total < buf.size()) {
        if (timeout_ms > 0) {
            fd_set rfds;
            FD_ZERO(&rfds);
            FD_SET(fd, &rfds);
            struct timeval tv;
            tv.tv_sec  = timeout_ms / 1000;
            tv.tv_usec = (timeout_ms % 1000) * 1000;
            int ret = select((int)(fd + 1), &rfds, nullptr, nullptr, &tv);
            if (ret <= 0) throw timeout_error();
        }
        int n = recv(fd, (char*)(buf.data() + total), (int)(buf.size() - total), 0);
        if (n <= 0) throw connection_error("read failed");
        total += n;
    }
}

inline void write_exact(socket_handle::native_t fd, std::span<const std::byte> buf, int timeout_ms) {
    size_t total = 0;
    while (total < buf.size()) {
        if (timeout_ms > 0) {
            fd_set wfds;
            FD_ZERO(&wfds);
            FD_SET(fd, &wfds);
            struct timeval tv;
            tv.tv_sec  = timeout_ms / 1000;
            tv.tv_usec = (timeout_ms % 1000) * 1000;
            int ret = select((int)(fd + 1), nullptr, &wfds, nullptr, &tv);
            if (ret <= 0) throw timeout_error();
        }
        int n = send(fd, (const char*)(buf.data() + total), (int)(buf.size() - total), 0);
        if (n <= 0) throw connection_error("write failed");
        total += n;
    }
}

} // namespace mcache::detail

#endif // MCACHE_PLATFORM_IMPL_HPP
