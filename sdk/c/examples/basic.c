#include <mcache/mcache.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

int main(void) {
    // Connect to local mcache server
    printf("Connecting to mcache server at 127.0.0.1:11211...\n");
    mcache_conn_t* conn = mcache_connect("127.0.0.1:11211", 5000);
    if (!conn) {
        fprintf(stderr, "Failed to connect. Is the server running?\n");
        fprintf(stderr, "Start with: mcache server --config config.yaml\n");
        return 1;
    }
    printf("Connected.\n\n");

    // Set a key
    const char* key = "example:greeting";
    const char* val = "Hello from C SDK!";
    printf("SET %s = \"%s\"\n", key, val);
    int err = mcache_set(conn, key, (const uint8_t*)val, (uint32_t)strlen(val), 60000);
    if (err != MCACHE_OK) {
        fprintf(stderr, "SET failed: %s\n", mcache_error_string(err));
        if (mcache_last_error(conn)) fprintf(stderr, "Server: %s\n", mcache_last_error(conn));
        mcache_disconnect(conn);
        return 1;
    }
    printf("OK\n\n");

    // Get the key back
    printf("GET %s\n", key);
    uint8_t* got_val = NULL;
    uint32_t got_len = 0;
    err = mcache_get(conn, key, &got_val, &got_len);
    if (err != MCACHE_OK) {
        fprintf(stderr, "GET failed: %s\n", mcache_error_string(err));
        mcache_disconnect(conn);
        return 1;
    }
    printf("= \"%.*s\"\n\n", (int)got_len, got_val);
    mcache_free(got_val);

    // Length
    uint64_t count;
    mcache_len(conn, &count);
    printf("LEN = %llu\n\n", (unsigned long long)count);

    // Ping
    int64_t rtt_us;
    mcache_ping(conn, &rtt_us);
    printf("PING = %lld us\n\n", (long long)rtt_us);

    // Cleanup
    uint64_t removed;
    mcache_cleanup(conn, &removed);
    printf("CLEANUP removed %llu expired entries\n\n", (unsigned long long)removed);

    // Delete
    printf("DEL %s\n", key);
    err = mcache_del(conn, key);
    if (err != MCACHE_OK) {
        fprintf(stderr, "DEL failed: %s\n", mcache_error_string(err));
    } else {
        printf("OK\n\n");
    }

    // Verify deletion
    err = mcache_get(conn, key, &got_val, &got_len);
    if (err == MCACHE_ERR_NOT_FOUND) {
        printf("GET %s → key not found (expected)\n", key);
    }

    mcache_disconnect(conn);
    printf("\nDone.\n");
    return 0;
}
