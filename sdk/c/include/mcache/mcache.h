#ifndef MCACHE_MCACHE_H
#define MCACHE_MCACHE_H

// Public API header for the mcache C SDK.
// Include this single header to get the full API.

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// Version
#define MCACHE_VERSION_MAJOR 1
#define MCACHE_VERSION_MINOR 0
#define MCACHE_VERSION_PATCH 0
#define MCACHE_VERSION_STRING "1.0.0"

// Error codes
typedef enum {
    MCACHE_OK           =  0,
    MCACHE_ERR_CONNECT  = -1,
    MCACHE_ERR_TIMEOUT  = -2,
    MCACHE_ERR_CLOSED   = -3,
    MCACHE_ERR_NOT_FOUND= -4,
    MCACHE_ERR_PROTOCOL = -5,
    MCACHE_ERR_TOO_LARGE= -6,
    MCACHE_ERR_INVALID  = -7,
    MCACHE_ERR_IO       = -8,
    MCACHE_ERR_MEMORY   = -9,
} mcache_error_t;

// Opaque connection handle
typedef struct mcache_conn_s mcache_conn_t;

// mcache_connect: connect to an mcache server at addr ("host:port").
// timeout_ms <0 means blocking connect, 0 means default (5000ms).
// Returns NULL on failure.
mcache_conn_t* mcache_connect(const char* addr, int timeout_ms);

// mcache_disconnect: close connection and free all resources.
void mcache_disconnect(mcache_conn_t* conn);

// mcache_get: retrieve a key's value.
// On success, *out_value is malloc'd; caller must free with mcache_free().
// Returns MCACHE_ERR_NOT_FOUND if the key does not exist.
int mcache_get(mcache_conn_t* conn, const char* key, uint8_t** out_value, uint32_t* out_len);

// mcache_set: store a value with optional TTL.
// ttl_ms > 0: expire after ttl_ms milliseconds.
// ttl_ms = 0: use server default TTL.
// ttl_ms < 0: no expiry.
int mcache_set(mcache_conn_t* conn, const char* key, const uint8_t* value, uint32_t value_len, int64_t ttl_ms);

// mcache_del: remove a key.
int mcache_del(mcache_conn_t* conn, const char* key);

// mcache_len: get the total number of entries in the cache.
int mcache_len(mcache_conn_t* conn, uint64_t* out_count);

// mcache_cleanup: remove all expired entries, return count removed.
int mcache_cleanup(mcache_conn_t* conn, uint64_t* out_removed);

// mcache_ping: check server connectivity, return round-trip time in microseconds.
int mcache_ping(mcache_conn_t* conn, int64_t* out_rtt_us);

// mcache_last_error: get the last server-side error message for this connection.
// Returned pointer is valid until the next call on conn.
const char* mcache_last_error(mcache_conn_t* conn);

// mcache_error_string: get a human-readable description of an error code.
const char* mcache_error_string(int err);

// mcache_free: free memory allocated by mcache_get.
void mcache_free(void* ptr);

// mcache_set_timeout: override read/write timeouts in milliseconds.
// <0 means no timeout (blocking indefinitely).
void mcache_set_timeout(mcache_conn_t* conn, int read_ms, int write_ms);

#ifdef __cplusplus
}
#endif

#endif // MCACHE_MCACHE_H
