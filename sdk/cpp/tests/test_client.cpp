#include "mcache/mcache.hpp"
#include <iostream>
#include <cassert>
#include <chrono>
#include <thread>

static int passed = 0, failed = 0;

#define TEST(name) do { \
    std::cout << "  " << #name << "... "; \
    try { test_##name(); std::cout << "PASS" << std::endl; passed++; } \
    catch (const std::exception& e) { std::cout << "FAIL: " << e.what() << std::endl; failed++; } \
} while(0)

#define CHECK(cond) do { if (!(cond)) throw std::runtime_error(#cond); } while(0)

mcache::client make_client() {
    return mcache::client("127.0.0.1:11211");
}

void test_set_and_get() {
    auto c = make_client();
    auto r = c.set("test:cpp:set_get", std::string("hello cpp"), std::chrono::minutes(1));
    CHECK(r.has_value());

    auto g = c.get<std::string>("test:cpp:set_get");
    CHECK(g.has_value());
    CHECK(g.value() == "hello cpp");

    c.del("test:cpp:set_get");
}

void test_get_not_found() {
    auto c = make_client();
    auto r = c.get<std::string>("nonexistent_key_42");
    CHECK(!r.has_value());
    CHECK(r.error_code() == mcache::errc::not_found);
}

void test_del_then_get() {
    auto c = make_client();
    c.set("test:cpp:del_then_get", std::string("x"), std::chrono::minutes(1));
    c.del("test:cpp:del_then_get");

    auto r = c.get<std::string>("test:cpp:del_then_get");
    CHECK(!r.has_value());
}

void test_len_works() {
    auto c = make_client();
    auto r = c.len();
    CHECK(r.has_value());
}

void test_cleanup_works() {
    auto c = make_client();
    auto r = c.cleanup();
    CHECK(r.has_value());
}

void test_large_value() {
    size_t size = 1024 * 64;
    std::vector<std::byte> big(size);
    for (size_t i = 0; i < size; ++i) big[i] = std::byte((uint8_t)(i & 0xFF));

    auto c = make_client();
    c.set("test:cpp:large", big, std::chrono::minutes(1));

    auto r = c.get<std::vector<std::byte>>("test:cpp:large");
    CHECK(r.has_value());
    CHECK(r.value().size() == size);
    CHECK(r.value() == big);

    c.del("test:cpp:large");
}

int main() {
    std::cout << "=== mcache C++ SDK Integration Tests ===" << std::endl;
    std::cout << "(Requires mcache server at 127.0.0.1:11211)" << std::endl << std::endl;

    try {
        auto test_conn = make_client();
        auto ping = test_conn.len();
        if (!ping) throw std::runtime_error("cannot reach server");
    } catch (...) {
        std::cout << "SKIP: Could not connect to mcache server." << std::endl;
        std::cout << "Start with: mcache server --config config.yaml" << std::endl;
        return 0;
    }

    TEST(set_and_get);
    TEST(get_not_found);
    TEST(del_then_get);
    TEST(len_works);
    TEST(cleanup_works);
    TEST(large_value);

    std::cout << std::endl << passed + failed << " tests, " << failed << " failed" << std::endl;
    return failed > 0 ? 1 : 0;
}
