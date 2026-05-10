#ifndef MCACHE_PLATFORM_H
#define MCACHE_PLATFORM_H

// Platform detection
#if defined(_WIN32) || defined(_WIN64)
  #define MCACHE_PLATFORM_WINDOWS 1
  #ifndef _WIN32_WINNT
    #define _WIN32_WINNT 0x0600
  #endif
  #include <winsock2.h>
  #include <ws2tcpip.h>
  #pragma comment(lib, "Ws2_32.lib")
#elif defined(__linux__)
  #define MCACHE_PLATFORM_LINUX 1
  #include <sys/socket.h>
  #include <netinet/in.h>
  #include <netinet/tcp.h>
  #include <arpa/inet.h>
  #include <netdb.h>
  #include <unistd.h>
  #include <errno.h>
#elif defined(__APPLE__)
  #define MCACHE_PLATFORM_MACOS 1
  #include <sys/socket.h>
  #include <netinet/in.h>
  #include <netinet/tcp.h>
  #include <arpa/inet.h>
  #include <netdb.h>
  #include <unistd.h>
  #include <errno.h>
#else
  #error "Unsupported platform"
#endif

#include <stdint.h>
#include <string.h>
#include <stdlib.h>

// Socket type
#ifdef MCACHE_PLATFORM_WINDOWS
  typedef SOCKET mcache_socket_t;
  #define MCACHE_INVALID_SOCKET INVALID_SOCKET
  #define MCACHE_SOCKET_ERROR   SOCKET_ERROR
#else
  typedef int mcache_socket_t;
  #define MCACHE_INVALID_SOCKET (-1)
  #define MCACHE_SOCKET_ERROR   (-1)
#endif

// Socket close
#ifdef MCACHE_PLATFORM_WINDOWS
  #define mcache_close_socket(s) closesocket(s)
#else
  #define mcache_close_socket(s) close(s)
#endif

// Last socket error
#ifdef MCACHE_PLATFORM_WINDOWS
  #define mcache_socket_errno() WSAGetLastError()
  #define MCACHE_EWOULDBLOCK WSAEWOULDBLOCK
  #define MCACHE_EINPROGRESS WSAEINPROGRESS
#else
  #define mcache_socket_errno() errno
  #define MCACHE_EWOULDBLOCK EWOULDBLOCK
  #define MCACHE_EINPROGRESS EINPROGRESS
#endif

// Endianness: network (big) <-> host
// All platforms provide htons/ntohs/htonl/ntohl.
// For 64-bit, we implement manually since htonll is not standard.

static inline uint64_t mcache_hton64(uint64_t host) {
#if defined(__BYTE_ORDER__) && __BYTE_ORDER__ == __ORDER_BIG_ENDIAN__
    return host;
#else
    return ((uint64_t)htonl((uint32_t)(host >> 32))) |
           ((uint64_t)htonl((uint32_t)(host & 0xFFFFFFFF)) << 32);
#endif
}

static inline uint64_t mcache_ntoh64(uint64_t net) {
    return mcache_hton64(net); // symmetric
}

#define mcache_hton16(v) htons((uint16_t)(v))
#define mcache_ntoh16(v) ntohs((uint16_t)(v))
#define mcache_hton32(v) htonl((uint32_t)(v))
#define mcache_ntoh32(v) ntohl((uint32_t)(v))

// WSA init/cleanup (Windows only)
#ifdef MCACHE_PLATFORM_WINDOWS
  #define mcache_wsa_init() do { \
    WSADATA __wsa; \
    WSAStartup(MAKEWORD(2,2), &__wsa); \
  } while(0)
  #define mcache_wsa_cleanup() WSACleanup()
#else
  #define mcache_wsa_init()    ((void)0)
  #define mcache_wsa_cleanup() ((void)0)
#endif

#endif // MCACHE_PLATFORM_H
