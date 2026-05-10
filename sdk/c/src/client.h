#ifndef MCACHE_CLIENT_H
#define MCACHE_CLIENT_H

#include "platform.h"
#include "protocol.h"
#include "errors.h"

#include <stdbool.h>

#ifdef _WIN32
  #include <windows.h>
  typedef CRITICAL_SECTION mcache_mutex_t;
#else
  #include <pthread.h>
  typedef pthread_mutex_t mcache_mutex_t;
#endif

#ifdef __cplusplus
extern "C" {
#endif

// Opaque connection handle.
typedef struct mcache_conn_s mcache_conn_t;

// mcache_connect creates a connection to addr ("host:port").
// timeout_ms: connect timeout in milliseconds (<0 = blocking, 0 = default 5000).
// Returns NULL on failure; call mcache_error_string() for details on return code.
mcache_conn_t* mcache_connect(const char* addr, int timeout_ms);

// mcache_disconnect closes the connection and frees all resources.
void mcache_disconnect(mcache_conn_t* conn);

// CRUD operations — all return MCACHE_OK (0) on success, negative error code on failure.
// On success, *out_value is malloc'd; caller must free with mcache_free().

int mcache_get(mcache_conn_t* conn, const char* key, uint8_t** out_value, uint32_t* out_len);

int mcache_set(mcache_conn_t* conn, const char* key, const uint8_t* value, uint32_t value_len, int64_t ttl_ms);

int mcache_del(mcache_conn_t* conn, const char* key);

int mcache_len(mcache_conn_t* conn, uint64_t* out_count);

int mcache_cleanup(mcache_conn_t* conn, uint64_t* out_removed);

int mcache_ping(mcache_conn_t* conn, int64_t* out_rtt_us);

// mcache_last_error returns the last server-side error message for this connection.
// Returned pointer is valid until the next operation on conn.
const char* mcache_last_error(mcache_conn_t* conn);

// mcache_free releases memory allocated by mcache_get.
void mcache_free(void* ptr);

// mcache_set_timeout overrides the read/write timeouts (milliseconds, <0 = no timeout).
void mcache_set_timeout(mcache_conn_t* conn, int read_ms, int write_ms);

#ifdef __cplusplus
}
#endif

#endif // MCACHE_CLIENT_H
