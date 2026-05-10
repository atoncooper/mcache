#include "mcache/mcache.hpp"
#include <iostream>
#include <cassert>

static int passed = 0, failed = 0;

#define TEST(name) do { \
    std::cout << "  " << #name << "... "; \
    try { test_##name(); std::cout << "PASS" << std::endl; passed++; } \
    catch (const std::exception& e) { std::cout << "FAIL: " << e.what() << std::endl; failed++; } \
} while(0)

#define CHECK(cond) do { if (!(cond)) throw std::runtime_error(#cond); } while(0)

void test_fnv1a_deterministic() {
    uint32_t h1 = mcache::fnv1a_32("hello");
    uint32_t h2 = mcache::fnv1a_32("hello");
    uint32_t h3 = mcache::fnv1a_32("world");
    CHECK(h1 == h2);
    CHECK(h1 != h3);
}

void test_fnv1a_known_values() {
    // Known FNV-1a values
    uint32_t h = mcache::fnv1a_32("");
    CHECK(h == 2166136261u); // offset_basis

    h = mcache::fnv1a_32("a");
    CHECK(h != 2166136261u);
}

void test_cluster_routing() {
    std::vector<std::string> nodes = {
        "127.0.0.1:11211",
        "127.0.0.1:11212",
        "127.0.0.1:11213",
    };

    // Same key always routes to same node
    uint32_t h1 = mcache::fnv1a_32("user:42") % (uint32_t)nodes.size();
    uint32_t h2 = mcache::fnv1a_32("user:42") % (uint32_t)nodes.size();
    CHECK(h1 == h2);
}

void test_hash_distribution() {
    // Verify hash distributes reasonably
    int buckets[16] = {0};
    for (int i = 0; i < 10000; ++i) {
        uint32_t h = mcache::fnv1a_32(std::to_string(i));
        buckets[h % 16]++;
    }
    // All buckets should have data
    for (int i = 0; i < 16; ++i) {
        CHECK(buckets[i] > 0);
    }
}

int main() {
    std::cout << "=== mcache C++ SDK Cluster/Hash Tests ===" << std::endl << std::endl;

    TEST(fnv1a_deterministic);
    TEST(fnv1a_known_values);
    TEST(cluster_routing);
    TEST(hash_distribution);

    std::cout << std::endl << passed + failed << " tests, " << failed << " failed" << std::endl;
    return failed > 0 ? 1 : 0;
}
