#include "client.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>
#include <time.h>

// Connection state
struct mcache_conn_s {
    mcache_socket_t fd;
    mcache_mutex_t  mutex;
    uint32_t        next_stream_id;
    char*           last_server_error;
    int             read_timeout_ms;
    int             write_timeout_ms;
    bool            closed;
};

// --- Mutex helpers ---

#ifdef _WIN32
  static void mcache_mutex_init(mcache_mutex_t* m) { InitializeCriticalSection(m); }
  static void mcache_mutex_destroy(mcache_mutex_t* m) { DeleteCriticalSection(m); }
  static void mcache_mutex_lock(mcache_mutex_t* m) { EnterCriticalSection(m); }
  static void mcache_mutex_unlock(mcache_mutex_t* m) { LeaveCriticalSection(m); }
#else
  static void mcache_mutex_init(mcache_mutex_t* m) { pthread_mutex_init(m, NULL); }
  static void mcache_mutex_destroy(mcache_mutex_t* m) { pthread_mutex_destroy(m); }
  static void mcache_mutex_lock(mcache_mutex_t* m) { pthread_mutex_lock(m); }
  static void mcache_mutex_unlock(mcache_mutex_t* m) { pthread_mutex_unlock(m); }
#endif

// --- Socket helpers ---

static bool set_nonblock(mcache_socket_t fd) {
#ifdef MCACHE_PLATFORM_WINDOWS
    u_long mode = 1;
    return ioctlsocket(fd, FIONBIO, &mode) == 0;
#else
    int flags = fcntl(fd, F_GETFL, 0);
    return flags >= 0 && fcntl(fd, F_SETFL, flags | O_NONBLOCK) >= 0;
#endif
}

static bool set_socket_timeout(mcache_socket_t fd, int ms, bool is_read) {
    int optname = is_read ? SO_RCVTIMEO : SO_SNDTIMEO;
    int sec = ms / 1000;
    int usec = (ms % 1000) * 1000;
#ifdef MCACHE_PLATFORM_WINDOWS
    DWORD tv = (DWORD)ms;
    return setsockopt(fd, SOL_SOCKET, optname, (const char*)&tv, sizeof(tv)) == 0;
#else
    struct timeval tv = { .tv_sec = sec, .tv_usec = usec };
    return setsockopt(fd, SOL_SOCKET, optname, &tv, sizeof(tv)) == 0;
#endif
}

// --- DNS resolve and connect ---

mcache_conn_t* mcache_connect(const char* addr, int timeout_ms) {
    if (!addr) return NULL;
    if (timeout_ms <= 0) timeout_ms = 5000;

    mcache_wsa_init();

    // Parse host:port
    char host[256];
    char port[16] = "11211";
    const char* colon = strrchr(addr, ':');
    if (colon) {
        size_t host_len = (size_t)(colon - addr);
        if (host_len >= sizeof(host)) return NULL;
        memcpy(host, addr, host_len);
        host[host_len] = '\0';
        strncpy(port, colon + 1, sizeof(port) - 1);
    } else {
        strncpy(host, addr, sizeof(host) - 1);
    }

    struct addrinfo hints = {0}, *result = NULL;
    hints.ai_family   = AF_UNSPEC;
    hints.ai_socktype = SOCK_STREAM;
    hints.ai_protocol = IPPROTO_TCP;

    if (getaddrinfo(host, port, &hints, &result) != 0) return NULL;

    mcache_socket_t fd = MCACHE_INVALID_SOCKET;
    struct addrinfo* rp;
    for (rp = result; rp; rp = rp->ai_next) {
        fd = socket(rp->ai_family, rp->ai_socktype, rp->ai_protocol);
        if (fd == MCACHE_INVALID_SOCKET) continue;

        set_nonblock(fd);
        int ret = connect(fd, rp->ai_addr, (int)rp->ai_addrlen);
        if (ret == 0) break; // immediate connect

#ifdef MCACHE_PLATFORM_WINDOWS
        if (mcache_socket_errno() == WSAEWOULDBLOCK) {
#else
        if (errno == EINPROGRESS) {
#endif
            fd_set wfds;
            FD_ZERO(&wfds);
            FD_SET(fd, &wfds);
            struct timeval tv = { .tv_sec = timeout_ms / 1000, .tv_usec = (timeout_ms % 1000) * 1000 };
            ret = select((int)(fd + 1), NULL, &wfds, NULL, &tv);
            if (ret > 0) {
                int so_err = 0;
                socklen_t len = sizeof(so_err);
                getsockopt(fd, SOL_SOCKET, SO_ERROR, (char*)&so_err, &len);
                if (so_err == 0) break;
            }
        }
        mcache_close_socket(fd);
        fd = MCACHE_INVALID_SOCKET;
    }
    freeaddrinfo(result);
    if (fd == MCACHE_INVALID_SOCKET) return NULL;

    // Set TCP_NODELAY
    int yes = 1;
    setsockopt(fd, IPPROTO_TCP, TCP_NODELAY, (const char*)&yes, sizeof(yes));

    // Create connection struct
    mcache_conn_t* conn = (mcache_conn_t*)calloc(1, sizeof(*conn));
    if (!conn) { mcache_close_socket(fd); return NULL; }

    conn->fd             = fd;
    conn->next_stream_id  = 1;
    conn->read_timeout_ms  = 10000;
    conn->write_timeout_ms = 5000;
    mcache_mutex_init(&conn->mutex);

    return conn;
}

void mcache_disconnect(mcache_conn_t* conn) {
    if (!conn) return;
    mcache_mutex_lock(&conn->mutex);
    if (!conn->closed) {
        conn->closed = true;
        mcache_close_socket(conn->fd);
    }
    mcache_mutex_unlock(&conn->mutex);
    mcache_mutex_destroy(&conn->mutex);
    free(conn->last_server_error);
    free(conn);
}

void mcache_free(void* ptr) { free(ptr); }

void mcache_set_timeout(mcache_conn_t* conn, int read_ms, int write_ms) {
    if (!conn) return;
    conn->read_timeout_ms  = read_ms;
    conn->write_timeout_ms = write_ms;
}

const char* mcache_last_error(mcache_conn_t* conn) {
    return conn ? conn->last_server_error : NULL;
}

// --- Internal: read/write exact bytes ---

static int read_exact(mcache_conn_t* conn, uint8_t* buf, uint32_t len, int timeout_ms) {
    uint32_t total = 0;
    while (total < len) {
        if (timeout_ms > 0) {
            fd_set rfds;
            FD_ZERO(&rfds);
            FD_SET(conn->fd, &rfds);
            struct timeval tv = { .tv_sec = timeout_ms / 1000, .tv_usec = (timeout_ms % 1000) * 1000 };
            int ret = select((int)(conn->fd + 1), &rfds, NULL, NULL, &tv);
            if (ret <= 0) return MCACHE_ERR_TIMEOUT;
        }
        int n = recv(conn->fd, (char*)(buf + total), len - total, 0);
        if (n <= 0) return MCACHE_ERR_IO;
        total += n;
    }
    return MCACHE_OK;
}

static int write_exact(mcache_conn_t* conn, const uint8_t* buf, uint32_t len, int timeout_ms) {
    uint32_t total = 0;
    while (total < len) {
        if (timeout_ms > 0) {
            fd_set wfds;
            FD_ZERO(&wfds);
            FD_SET(conn->fd, &wfds);
            struct timeval tv = { .tv_sec = timeout_ms / 1000, .tv_usec = (timeout_ms % 1000) * 1000 };
            int ret = select((int)(conn->fd + 1), NULL, &wfds, NULL, &tv);
            if (ret <= 0) return MCACHE_ERR_TIMEOUT;
        }
        int n = send(conn->fd, (const char*)(buf + total), len - total, 0);
        if (n <= 0) return MCACHE_ERR_IO;
        total += n;
    }
    return MCACHE_OK;
}

// --- Internal: do request/response cycle ---

static int do_request(mcache_conn_t* conn, const mcache_request_t* req, mcache_response_t* resp) {
    // Encode request payload
    uint8_t req_buf[MCACHE_MAX_PAYLOAD_SIZE];
    int req_len = mcache_request_encode(req, req_buf, sizeof(req_buf));
    if (req_len < 0) return MCACHE_ERR_PROTOCOL;

    // Build frame
    mcache_frame_t frame_out = {
        .stream_id   = conn->next_stream_id++,
        .type        = MCACHE_FRAME_REQUEST,
        .flags       = 0,
        .payload     = req_buf,
        .payload_len = (uint32_t)req_len,
    };
    if (conn->next_stream_id == 0) conn->next_stream_id = 1;

    uint8_t frame_buf[MCACHE_MAX_PAYLOAD_SIZE + MCACHE_FRAME_HEADER_SIZE];
    int frame_len = mcache_frame_encode(&frame_out, frame_buf, sizeof(frame_buf));
    if (frame_len < 0) return MCACHE_ERR_PROTOCOL;

    // Write frame
    int err = write_exact(conn, frame_buf, (uint32_t)frame_len, conn->write_timeout_ms);
    if (err != MCACHE_OK) return err;

    // Read response frame header
    uint8_t header[MCACHE_FRAME_HEADER_SIZE];
    err = read_exact(conn, header, MCACHE_FRAME_HEADER_SIZE, conn->read_timeout_ms);
    if (err != MCACHE_OK) return err;

    mcache_frame_t frame_in = {0};
    err = mcache_frame_decode(header, MCACHE_FRAME_HEADER_SIZE, &frame_in);
    if (err != 0) return MCACHE_ERR_PROTOCOL;

    // Read payload if any
    if (frame_in.payload_len > 0) {
        free(frame_in.payload);
        frame_in.payload = (uint8_t*)malloc(frame_in.payload_len);
        if (!frame_in.payload) return MCACHE_ERR_MEMORY;
        err = read_exact(conn, frame_in.payload, frame_in.payload_len, conn->read_timeout_ms);
        if (err != MCACHE_OK) { free(frame_in.payload); return err; }
        frame_in = (mcache_frame_t){ // rebuild after read
            .stream_id   = frame_in.stream_id,
            .type        = frame_in.type,
            .flags       = frame_in.flags,
            .payload     = frame_in.payload,
            .payload_len = frame_in.payload_len,
        };
    }

    (void)frame_in.stream_id; // single-connection mode, ignore

    // Decode response payload
    err = mcache_response_decode(frame_in.payload, frame_in.payload_len, resp);
    mcache_frame_free(&frame_in);
    if (err != 0) return MCACHE_ERR_PROTOCOL;

    // Store server-side error for retrieval
    free(conn->last_server_error);
    if (resp->err_msg) {
        conn->last_server_error = strdup(resp->err_msg);
    } else {
        conn->last_server_error = NULL;
    }

    // Map status to error code
    if (resp->status == MCACHE_STATUS_NOT_FOUND) return MCACHE_ERR_NOT_FOUND;
    if (resp->status == MCACHE_STATUS_ERR)      return MCACHE_ERR_PROTOCOL;
    return MCACHE_OK;
}

// --- Public API ---

int mcache_get(mcache_conn_t* conn, const char* key, uint8_t** out_value, uint32_t* out_len) {
    if (!conn || !key || !out_value || !out_len) return MCACHE_ERR_INVALID;

    mcache_mutex_lock(&conn->mutex);
    if (conn->closed) { mcache_mutex_unlock(&conn->mutex); return MCACHE_ERR_CLOSED; }

    mcache_request_t req = { .cmd = MCACHE_CMD_GET, .key = (char*)key, .key_len = (uint16_t)strlen(key) };
    mcache_response_t resp = {0};
    int err = do_request(conn, &req, &resp);
    mcache_mutex_unlock(&conn->mutex);

    if (err == MCACHE_OK) {
        *out_value = resp.value;
        *out_len   = resp.value_len;
        free(resp.err_msg);
    }
    return err;
}

int mcache_set(mcache_conn_t* conn, const char* key, const uint8_t* value, uint32_t value_len, int64_t ttl_ms) {
    if (!conn || !key || (!value && value_len > 0)) return MCACHE_ERR_INVALID;

    mcache_mutex_lock(&conn->mutex);
    if (conn->closed) { mcache_mutex_unlock(&conn->mutex); return MCACHE_ERR_CLOSED; }

    mcache_request_t req = {
        .cmd = MCACHE_CMD_SET, .key = (char*)key, .key_len = (uint16_t)strlen(key),
        .value = (uint8_t*)value, .value_len = value_len, .ttl_ms = ttl_ms,
    };
    mcache_response_t resp = {0};
    int err = do_request(conn, &req, &resp);
    mcache_mutex_unlock(&conn->mutex);
    mcache_response_free(&resp);
    return err;
}

int mcache_del(mcache_conn_t* conn, const char* key) {
    if (!conn || !key) return MCACHE_ERR_INVALID;

    mcache_mutex_lock(&conn->mutex);
    if (conn->closed) { mcache_mutex_unlock(&conn->mutex); return MCACHE_ERR_CLOSED; }

    mcache_request_t req = { .cmd = MCACHE_CMD_DEL, .key = (char*)key, .key_len = (uint16_t)strlen(key) };
    mcache_response_t resp = {0};
    int err = do_request(conn, &req, &resp);
    mcache_mutex_unlock(&conn->mutex);
    mcache_response_free(&resp);
    return err;
}

int mcache_len(mcache_conn_t* conn, uint64_t* out_count) {
    if (!conn || !out_count) return MCACHE_ERR_INVALID;

    mcache_mutex_lock(&conn->mutex);
    if (conn->closed) { mcache_mutex_unlock(&conn->mutex); return MCACHE_ERR_CLOSED; }

    mcache_request_t req = { .cmd = MCACHE_CMD_LEN };
    mcache_response_t resp = {0};
    int err = do_request(conn, &req, &resp);
    mcache_mutex_unlock(&conn->mutex);

    if (err == MCACHE_OK && resp.value_len >= 8) {
        uint64_t val_net;
        memcpy(&val_net, resp.value, 8);
        *out_count = mcache_ntoh64(val_net);
    }
    mcache_response_free(&resp);
    return err;
}

int mcache_cleanup(mcache_conn_t* conn, uint64_t* out_removed) {
    if (!conn || !out_removed) return MCACHE_ERR_INVALID;

    mcache_mutex_lock(&conn->mutex);
    if (conn->closed) { mcache_mutex_unlock(&conn->mutex); return MCACHE_ERR_CLOSED; }

    mcache_request_t req = { .cmd = MCACHE_CMD_CLEANUP };
    mcache_response_t resp = {0};
    int err = do_request(conn, &req, &resp);
    mcache_mutex_unlock(&conn->mutex);

    if (err == MCACHE_OK && resp.value_len >= 8) {
        uint64_t val_net;
        memcpy(&val_net, resp.value, 8);
        *out_removed = mcache_ntoh64(val_net);
    }
    mcache_response_free(&resp);
    return err;
}

int mcache_ping(mcache_conn_t* conn, int64_t* out_rtt_us) {
    if (!conn || !out_rtt_us) return MCACHE_ERR_INVALID;
    uint64_t count;
    int64_t start_us = (int64_t)(clock() * 1000000LL / CLOCKS_PER_SEC);
    int err = mcache_len(conn, &count);
    int64_t end_us = (int64_t)(clock() * 1000000LL / CLOCKS_PER_SEC);
    *out_rtt_us = end_us - start_us;
    return err;
}
