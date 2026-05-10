#ifndef MCACHE_ERRORS_H
#define MCACHE_ERRORS_H

#ifdef __cplusplus
extern "C" {
#endif

// Error codes: 0 = success, negative = error.
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

// Returns a static human-readable string for the error code (thread-safe).
const char* mcache_error_string(int err);

#ifdef __cplusplus
}
#endif

#endif // MCACHE_ERRORS_H
