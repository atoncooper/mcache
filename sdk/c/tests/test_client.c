#include "client.h"
#include <stdio.h>
#include <string.h>
#include <stdlib.h>

static int tests_run = 0;
static int tests_failed = 0;

#define TEST(name) static void test_##name(mcache_conn_t* conn)
#define RUN(name, conn) do { \
    tests_run++; \
    printf("  %s... ", #name); \
    test_##name(conn); \
    printf("PASS\n"); \
} while(0)
#define ASSERT(cond) do { if (!(cond)) { printf("FAIL at %s:%d: %s\n", __FILE__, __LINE__, #cond); tests_failed++; return; } } while(0)
#define ASSERT_OK(err) ASSERT((err) == MCACHE_OK)
#define ASSERT_NOT_FOUND(err) ASSERT((err) == MCACHE_ERR_NOT_FOUND)

TEST(set_and_get) {
    const char* key = "test:set_and_get";
    const char* val = "integration-test-value";
    uint32_t vlen = (uint32_t)strlen(val);

    ASSERT_OK(mcache_set(conn, key, (const uint8_t*)val, vlen, 0));

    uint8_t* got = NULL;
    uint32_t glen = 0;
    ASSERT_OK(mcache_get(conn, key, &got, &glen));
    ASSERT(glen == vlen);
    ASSERT(memcmp(got, val, vlen) == 0);
    mcache_free(got);

    ASSERT_OK(mcache_del(conn, key));
}

TEST(get_not_found) {
    uint8_t* val = NULL;
    uint32_t vlen = 0;
    ASSERT_NOT_FOUND(mcache_get(conn, "nonexistent_key_42", &val, &vlen));
}

TEST(del_then_get) {
    const char* key = "test:del_then_get";
    ASSERT_OK(mcache_set(conn, key, (const uint8_t*)"x", 1, 0));
    ASSERT_OK(mcache_del(conn, key));

    uint8_t* val = NULL;
    uint32_t vlen = 0;
    ASSERT_NOT_FOUND(mcache_get(conn, key, &val, &vlen));
}

TEST(len_increases) {
    uint64_t before, after;
    ASSERT_OK(mcache_len(conn, &before));

    ASSERT_OK(mcache_set(conn, "test:len_test", (const uint8_t*)"val", 3, 0));

    ASSERT_OK(mcache_len(conn, &after));
    ASSERT(after >= before);

    ASSERT_OK(mcache_del(conn, "test:len_test"));
}

TEST(ping_works) {
    int64_t rtt;
    ASSERT_OK(mcache_ping(conn, &rtt));
    ASSERT(rtt >= 0);
}

TEST(cleanup_works) {
    uint64_t removed;
    ASSERT_OK(mcache_cleanup(conn, &removed));
}

TEST(large_value) {
    size_t size = 1024 * 64; // 64KB
    uint8_t* big = (uint8_t*)malloc(size);
    for (size_t i = 0; i < size; i++) big[i] = (uint8_t)(i & 0xFF);

    ASSERT_OK(mcache_set(conn, "test:large", big, (uint32_t)size, 0));

    uint8_t* got = NULL;
    uint32_t glen = 0;
    ASSERT_OK(mcache_get(conn, "test:large", &got, &glen));
    ASSERT(glen == (uint32_t)size);
    ASSERT(memcmp(got, big, size) == 0);
    mcache_free(got);
    free(big);

    ASSERT_OK(mcache_del(conn, "test:large"));
}

int main(void) {
    printf("=== mcache C SDK Integration Tests ===\n");
    printf("(Requires mcache server at 127.0.0.1:11211)\n\n");

    mcache_conn_t* conn = mcache_connect("127.0.0.1:11211", 5000);
    if (!conn) {
        printf("SKIP: Could not connect to mcache server.\n");
        printf("Start with: mcache server --config config.yaml\n");
        return 0;
    }

    RUN(set_and_get, conn);
    RUN(get_not_found, conn);
    RUN(del_then_get, conn);
    RUN(len_increases, conn);
    RUN(ping_works, conn);
    RUN(cleanup_works, conn);
    RUN(large_value, conn);

    mcache_disconnect(conn);

    printf("\n%d tests, %d failed\n", tests_run, tests_failed);
    return tests_failed > 0 ? 1 : 0;
}
