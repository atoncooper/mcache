#include "errors.h"

const char* mcache_error_string(int err) {
    switch (err) {
        case MCACHE_OK:            return "success";
        case MCACHE_ERR_CONNECT:   return "connection failed";
        case MCACHE_ERR_TIMEOUT:   return "operation timed out";
        case MCACHE_ERR_CLOSED:    return "connection closed";
        case MCACHE_ERR_NOT_FOUND: return "key not found";
        case MCACHE_ERR_PROTOCOL:  return "protocol error";
        case MCACHE_ERR_TOO_LARGE: return "payload too large";
        case MCACHE_ERR_INVALID:   return "invalid argument";
        case MCACHE_ERR_IO:        return "I/O error";
        case MCACHE_ERR_MEMORY:    return "out of memory";
        default:                   return "unknown error";
    }
}
