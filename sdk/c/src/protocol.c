#include "protocol.h"
#include <stdio.h>

// --- Frame ---

int mcache_frame_encode(const mcache_frame_t* f, uint8_t* buf, uint32_t buf_size) {
    uint32_t total = MCACHE_FRAME_HEADER_SIZE + f->payload_len;
    if (total > buf_size) return -1;

    uint32_t plen_net = mcache_hton32(f->payload_len);
    uint32_t sid_net  = mcache_hton32(f->stream_id);

    memcpy(buf,      &plen_net, 4);
    memcpy(buf + 4,  &sid_net,  4);
    buf[8] = f->type;
    buf[9] = f->flags;
    if (f->payload_len > 0) {
        memcpy(buf + MCACHE_FRAME_HEADER_SIZE, f->payload, f->payload_len);
    }
    return (int)total;
}

int mcache_frame_decode(const uint8_t* data, uint32_t data_len, mcache_frame_t* f) {
    if (data_len < MCACHE_FRAME_HEADER_SIZE) return -1;

    uint32_t plen_net, sid_net;
    memcpy(&plen_net, data,      4);
    memcpy(&sid_net,  data + 4,  4);

    f->payload_len = mcache_ntoh32(plen_net);
    if (f->payload_len > MCACHE_MAX_PAYLOAD_SIZE) return -1;

    uint32_t total = MCACHE_FRAME_HEADER_SIZE + f->payload_len;
    if (data_len < total) return -1;

    f->stream_id = mcache_ntoh32(sid_net);
    f->type      = data[8];
    f->flags     = data[9];

    if (f->payload_len > 0) {
        f->payload = (uint8_t*)malloc(f->payload_len);
        if (!f->payload) return -1;
        memcpy(f->payload, data + MCACHE_FRAME_HEADER_SIZE, f->payload_len);
    } else {
        f->payload = NULL;
    }
    return 0;
}

// --- Request ---

int mcache_request_encode(const mcache_request_t* req, uint8_t* buf, uint32_t buf_size) {
    uint32_t total = 15 + req->key_len + req->value_len;
    if (total > buf_size) return -1;

    uint16_t key_len_net = mcache_hton16(req->key_len);
    uint32_t val_len_net = mcache_hton32(req->value_len);
    uint64_t ttl_net     = mcache_hton64((uint64_t)req->ttl_ms);

    buf[0] = req->cmd;
    memcpy(buf + 1,  &key_len_net, 2);
    memcpy(buf + 3,  &val_len_net, 4);
    memcpy(buf + 7,  &ttl_net,     8);
    if (req->key_len > 0)   memcpy(buf + 15, req->key, req->key_len);
    if (req->value_len > 0) memcpy(buf + 15 + req->key_len, req->value, req->value_len);
    return (int)total;
}

int mcache_request_decode(const uint8_t* payload, uint32_t payload_len, mcache_request_t* req) {
    if (payload_len < 15) return -1;

    uint16_t key_len_net;
    uint32_t val_len_net;
    uint64_t ttl_net;
    memcpy(&key_len_net, payload + 1, 2);
    memcpy(&val_len_net, payload + 3, 4);
    memcpy(&ttl_net,     payload + 7, 8);

    req->cmd       = payload[0];
    req->key_len   = mcache_ntoh16(key_len_net);
    req->value_len = mcache_ntoh32(val_len_net);
    req->ttl_ms    = (int64_t)mcache_ntoh64(ttl_net);

    uint32_t expected = 15 + req->key_len + req->value_len;
    if (payload_len < expected) return -1;

    if (req->key_len > 0) {
        req->key = (char*)malloc(req->key_len + 1);
        if (!req->key) return -1;
        memcpy(req->key, payload + 15, req->key_len);
        req->key[req->key_len] = '\0';
    } else {
        req->key = NULL;
    }

    if (req->value_len > 0) {
        req->value = (uint8_t*)malloc(req->value_len);
        if (!req->value) { free(req->key); return -1; }
        memcpy(req->value, payload + 15 + req->key_len, req->value_len);
    } else {
        req->value = NULL;
    }
    return 0;
}

// --- Response ---

int mcache_response_encode(const mcache_response_t* resp, uint8_t* buf, uint32_t buf_size) {
    uint32_t total = 7 + resp->value_len + resp->err_len;
    if (total > buf_size) return -1;

    uint32_t val_len_net = mcache_hton32(resp->value_len);
    uint16_t err_len_net = mcache_hton16(resp->err_len);

    buf[0] = resp->status;
    memcpy(buf + 1, &val_len_net, 4);
    memcpy(buf + 5, &err_len_net, 2);
    if (resp->value_len > 0) memcpy(buf + 7, resp->value, resp->value_len);
    if (resp->err_len > 0)   memcpy(buf + 7 + resp->value_len, resp->err_msg, resp->err_len);
    return (int)total;
}

int mcache_response_decode(const uint8_t* payload, uint32_t payload_len, mcache_response_t* resp) {
    if (payload_len < 7) return -1;

    uint32_t val_len_net;
    uint16_t err_len_net;
    memcpy(&val_len_net, payload + 1, 4);
    memcpy(&err_len_net, payload + 5, 2);

    resp->status    = payload[0];
    resp->value_len = mcache_ntoh32(val_len_net);
    resp->err_len   = mcache_ntoh16(err_len_net);

    uint32_t expected = 7 + resp->value_len + resp->err_len;
    if (payload_len < expected) return -1;

    if (resp->value_len > 0) {
        resp->value = (uint8_t*)malloc(resp->value_len);
        if (!resp->value) return -1;
        memcpy(resp->value, payload + 7, resp->value_len);
    } else {
        resp->value = NULL;
    }

    if (resp->err_len > 0) {
        resp->err_msg = (char*)malloc(resp->err_len + 1);
        if (!resp->err_msg) { free(resp->value); return -1; }
        memcpy(resp->err_msg, payload + 7 + resp->value_len, resp->err_len);
        resp->err_msg[resp->err_len] = '\0';
    } else {
        resp->err_msg = NULL;
    }
    return 0;
}

// --- Cleanup ---

void mcache_frame_free(mcache_frame_t* f) {
    if (f) { free(f->payload); f->payload = NULL; }
}

void mcache_request_free(mcache_request_t* req) {
    if (req) { free(req->key); free(req->value); req->key = NULL; req->value = NULL; }
}

void mcache_response_free(mcache_response_t* resp) {
    if (resp) { free(resp->value); free(resp->err_msg); resp->value = NULL; resp->err_msg = NULL; }
}
