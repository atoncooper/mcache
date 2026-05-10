#ifndef MCACHE_PROTOCOL_H
#define MCACHE_PROTOCOL_H

#include "platform.h"

#ifdef __cplusplus
extern "C" {
#endif

// Frame types
#define MCACHE_FRAME_REQUEST   ((uint8_t)0)
#define MCACHE_FRAME_RESPONSE  ((uint8_t)1)

// Commands (must match Go net/protocol.go exactly)
#define MCACHE_CMD_GET          ((uint8_t)1)
#define MCACHE_CMD_SET          ((uint8_t)2)
#define MCACHE_CMD_DEL          ((uint8_t)3)
#define MCACHE_CMD_LEN          ((uint8_t)4)
#define MCACHE_CMD_CLEANUP      ((uint8_t)5)

// Response statuses
#define MCACHE_STATUS_OK        ((uint8_t)0)
#define MCACHE_STATUS_ERR       ((uint8_t)1)
#define MCACHE_STATUS_NOT_FOUND ((uint8_t)2)

// Limits
#define MCACHE_MAX_PAYLOAD_SIZE (16 * 1024 * 1024) // 16 MB
#define MCACHE_FRAME_HEADER_SIZE 10

// Frame: wire format unit for the multiplexed protocol.
// Binary layout (big-endian):
//   [0:4]  PayloadLen uint32
//   [4:8]  StreamID   uint32
//   [8]    Type       uint8  (0=request, 1=response)
//   [9]    Flags      uint8
//   [10:]  Payload
typedef struct {
    uint32_t stream_id;
    uint8_t  type;
    uint8_t  flags;
    uint8_t* payload;
    uint32_t payload_len;
} mcache_frame_t;

// Request: cache operation sent by client.
// Payload layout (big-endian):
//   [0]    Cmd      uint8
//   [1:3]  KeyLen   uint16
//   [3:7]  ValueLen uint32
//   [7:15] TTL      int64 (milliseconds, 0 = default)
//   [15:]  Key      bytes
//   [15+KeyLen:] Value bytes
typedef struct {
    uint8_t  cmd;
    char*    key;
    uint16_t key_len;
    uint8_t* value;
    uint32_t value_len;
    int64_t  ttl_ms;
} mcache_request_t;

// Response: cache operation result from server.
// Payload layout (big-endian):
//   [0]    Status   uint8
//   [1:5]  ValueLen uint32
//   [5:7]  ErrLen   uint16
//   [7:]   Value    bytes
//   [7+ValueLen:] ErrMsg bytes
typedef struct {
    uint8_t  status;
    uint8_t* value;
    uint32_t value_len;
    char*    err_msg;
    uint16_t err_len;
} mcache_response_t;

// --- Encode ---

// mcache_frame_encode writes a frame to buf (must be pre-sized by caller).
// Returns number of bytes written, or -1 on error.
int mcache_frame_encode(const mcache_frame_t* f, uint8_t* buf, uint32_t buf_size);

// mcache_request_encode encodes a request into payload (caller allocates).
// Returns payload length, or -1 on error.
int mcache_request_encode(const mcache_request_t* req, uint8_t* buf, uint32_t buf_size);

// mcache_response_encode encodes a response into payload.
// Returns payload length, or -1 on error.
int mcache_response_encode(const mcache_response_t* resp, uint8_t* buf, uint32_t buf_size);

// --- Decode ---

// mcache_frame_decode parses a frame from raw bytes.
// On success, allocates f->payload via malloc (caller must free).
// Returns 0 on success, -1 on error.
int mcache_frame_decode(const uint8_t* data, uint32_t data_len, mcache_frame_t* f);

// mcache_request_decode parses a request from payload bytes.
// Allocates req->key and req->value via malloc (caller must free).
// Returns 0 on success, -1 on error.
int mcache_request_decode(const uint8_t* payload, uint32_t payload_len, mcache_request_t* req);

// mcache_response_decode parses a response from payload bytes.
// Allocates resp->value and resp->err_msg via malloc (caller must free).
// Returns 0 on success, -1 on error.
int mcache_response_decode(const uint8_t* payload, uint32_t payload_len, mcache_response_t* resp);

// --- Cleanup ---

void mcache_frame_free(mcache_frame_t* f);
void mcache_request_free(mcache_request_t* req);
void mcache_response_free(mcache_response_t* resp);

#ifdef __cplusplus
}
#endif

#endif // MCACHE_PROTOCOL_H
