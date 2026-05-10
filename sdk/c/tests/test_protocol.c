#include "protocol.h"
#include "errors.h"
#include <stdio.h>
#include <string.h>
#include <stdlib.h>

static int tests_run = 0;
static int tests_failed = 0;

#define TEST(name) static void test_##name(void)
#define RUN(name) do { \
    tests_run++; \
    printf("  %s... ", #name); \
    test_##name(); \
    printf("PASS\n"); \
} while(0)
#define ASSERT(cond) do { if (!(cond)) { printf("FAIL at %s:%d: %s\n", __FILE__, __LINE__, #cond); tests_failed++; } } while(0)

// --- Request encode/decode ---

TEST(req_roundtrip_get) {
    mcache_request_t req = {
        .cmd = MCACHE_CMD_GET, .key = "hello", .key_len = 5, .value = NULL, .value_len = 0, .ttl_ms = 0
    };
    uint8_t buf[256];
    int len = mcache_request_encode(&req, buf, sizeof(buf));
    ASSERT(len == 20); // 15 + 5 + 0

    mcache_request_t decoded = {0};
    int err = mcache_request_decode(buf, (uint32_t)len, &decoded);
    ASSERT(err == 0);
    ASSERT(decoded.cmd == MCACHE_CMD_GET);
    ASSERT(decoded.key_len == 5);
    ASSERT(strcmp(decoded.key, "hello") == 0);
    ASSERT(decoded.value_len == 0);
    ASSERT(decoded.ttl_ms == 0);
    mcache_request_free(&decoded);
}

TEST(req_roundtrip_set) {
    const char* val = "world";
    mcache_request_t req = {
        .cmd = MCACHE_CMD_SET, .key = "key1", .key_len = 4,
        .value = (uint8_t*)val, .value_len = 5, .ttl_ms = 30000
    };
    uint8_t buf[256];
    int len = mcache_request_encode(&req, buf, sizeof(buf));
    ASSERT(len == 24); // 15 + 4 + 5

    mcache_request_t decoded = {0};
    int err = mcache_request_decode(buf, (uint32_t)len, &decoded);
    ASSERT(err == 0);
    ASSERT(decoded.cmd == MCACHE_CMD_SET);
    ASSERT(decoded.key_len == 4);
    ASSERT(decoded.value_len == 5);
    ASSERT(decoded.ttl_ms == 30000);
    ASSERT(memcmp(decoded.value, "world", 5) == 0);
    mcache_request_free(&decoded);
}

TEST(req_roundtrip_del) {
    mcache_request_t req = { .cmd = MCACHE_CMD_DEL, .key = "bye", .key_len = 3 };
    uint8_t buf[256];
    int len = mcache_request_encode(&req, buf, sizeof(buf));

    mcache_request_t decoded = {0};
    int err = mcache_request_decode(buf, (uint32_t)len, &decoded);
    ASSERT(err == 0);
    ASSERT(decoded.cmd == MCACHE_CMD_DEL);
    mcache_request_free(&decoded);
}

TEST(req_roundtrip_len) {
    mcache_request_t req = { .cmd = MCACHE_CMD_LEN };
    uint8_t buf[256];
    int len = mcache_request_encode(&req, buf, sizeof(buf));
    ASSERT(len == 15); // header only, no key/value

    mcache_request_t decoded = {0};
    int err = mcache_request_decode(buf, (uint32_t)len, &decoded);
    ASSERT(err == 0);
    ASSERT(decoded.cmd == MCACHE_CMD_LEN);
    ASSERT(decoded.key_len == 0);
    ASSERT(decoded.value_len == 0);
    mcache_request_free(&decoded);
}

TEST(req_roundtrip_cleanup) {
    mcache_request_t req = { .cmd = MCACHE_CMD_CLEANUP };
    uint8_t buf[256];
    int len = mcache_request_encode(&req, buf, sizeof(buf));
    ASSERT(len == 15);

    mcache_request_t decoded = {0};
    int err = mcache_request_decode(buf, (uint32_t)len, &decoded);
    ASSERT(err == 0);
    ASSERT(decoded.cmd == MCACHE_CMD_CLEANUP);
    mcache_request_free(&decoded);
}

// --- Response encode/decode ---

TEST(resp_roundtrip_ok) {
    const char* val = "result";
    mcache_response_t resp = {
        .status = MCACHE_STATUS_OK, .value = (uint8_t*)val, .value_len = 6, .err_msg = NULL, .err_len = 0
    };
    uint8_t buf[256];
    int len = mcache_response_encode(&resp, buf, sizeof(buf));
    ASSERT(len == 13); // 7 + 6 + 0

    mcache_response_t decoded = {0};
    int err = mcache_response_decode(buf, (uint32_t)len, &decoded);
    ASSERT(err == 0);
    ASSERT(decoded.status == MCACHE_STATUS_OK);
    ASSERT(decoded.value_len == 6);
    ASSERT(memcmp(decoded.value, "result", 6) == 0);
    ASSERT(decoded.err_len == 0);
    mcache_response_free(&decoded);
}

TEST(resp_roundtrip_err) {
    const char* err = "something went wrong";
    mcache_response_t resp = {
        .status = MCACHE_STATUS_ERR, .value = NULL, .value_len = 0,
        .err_msg = (char*)err, .err_len = (uint16_t)strlen(err)
    };
    uint8_t buf[256];
    int len = mcache_response_encode(&resp, buf, sizeof(buf));

    mcache_response_t decoded = {0};
    int err_code = mcache_response_decode(buf, (uint32_t)len, &decoded);
    ASSERT(err_code == 0);
    ASSERT(decoded.status == MCACHE_STATUS_ERR);
    ASSERT(decoded.value_len == 0);
    ASSERT(decoded.err_len == strlen(err));
    ASSERT(strcmp(decoded.err_msg, err) == 0);
    mcache_response_free(&decoded);
}

TEST(resp_roundtrip_not_found) {
    mcache_response_t resp = { .status = MCACHE_STATUS_NOT_FOUND };
    uint8_t buf[256];
    int len = mcache_response_encode(&resp, buf, sizeof(buf));
    ASSERT(len == 7);

    mcache_response_t decoded = {0};
    int err = mcache_response_decode(buf, (uint32_t)len, &decoded);
    ASSERT(err == 0);
    ASSERT(decoded.status == MCACHE_STATUS_NOT_FOUND);
    mcache_response_free(&decoded);
}

// --- Frame encode/decode ---

TEST(frame_roundtrip) {
    const char* payload_str = "test-payload";
    mcache_frame_t f = {
        .stream_id = 42, .type = MCACHE_FRAME_REQUEST, .flags = 0,
        .payload = (uint8_t*)payload_str, .payload_len = (uint32_t)strlen(payload_str)
    };
    uint8_t buf[512];
    int len = mcache_frame_encode(&f, buf, sizeof(buf));
    ASSERT(len == MCACHE_FRAME_HEADER_SIZE + (int)strlen(payload_str));

    mcache_frame_t decoded = {0};
    int err = mcache_frame_decode(buf, (uint32_t)len, &decoded);
    ASSERT(err == 0);
    ASSERT(decoded.stream_id == 42);
    ASSERT(decoded.type == MCACHE_FRAME_REQUEST);
    ASSERT(decoded.payload_len == strlen(payload_str));
    ASSERT(memcmp(decoded.payload, payload_str, strlen(payload_str)) == 0);
    mcache_frame_free(&decoded);
}

TEST(frame_decode_too_short) {
    uint8_t header[5] = {0};
    mcache_frame_t f = {0};
    int err = mcache_frame_decode(header, 5, &f);
    ASSERT(err != 0); // too short for header
}

// --- Error strings ---

TEST(error_strings) {
    ASSERT(strcmp(mcache_error_string(MCACHE_OK), "success") == 0);
    ASSERT(strcmp(mcache_error_string(MCACHE_ERR_CONNECT), "connection failed") == 0);
    ASSERT(strcmp(mcache_error_string(MCACHE_ERR_NOT_FOUND), "key not found") == 0);
    ASSERT(strcmp(mcache_error_string(MCACHE_ERR_TIMEOUT), "operation timed out") == 0);
}

// --- Endianness ---

TEST(endianness_roundtrip) {
    uint64_t orig = 0x123456789ABCDEF0ULL;
    uint64_t net  = mcache_hton64(orig);
    uint64_t host = mcache_ntoh64(net);
    ASSERT(orig == host);
}

int main(void) {
    printf("=== mcache C SDK Protocol Tests ===\n\n");

    RUN(req_roundtrip_get);
    RUN(req_roundtrip_set);
    RUN(req_roundtrip_del);
    RUN(req_roundtrip_len);
    RUN(req_roundtrip_cleanup);
    RUN(resp_roundtrip_ok);
    RUN(resp_roundtrip_err);
    RUN(resp_roundtrip_not_found);
    RUN(frame_roundtrip);
    RUN(frame_decode_too_short);
    RUN(error_strings);
    RUN(endianness_roundtrip);

    printf("\n%d tests, %d failed\n", tests_run, tests_failed);
    return tests_failed > 0 ? 1 : 0;
}
