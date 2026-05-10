#include "mcache/protocol.hpp"
#include "mcache/frame.hpp"
#include "mcache/errors.hpp"
#include <cassert>
#include <iostream>
#include <cstring>

static int passed = 0, failed = 0;

#define TEST(name) do { \
    std::cout << "  " << #name << "... "; \
    try { test_##name(); std::cout << "PASS" << std::endl; passed++; } \
    catch (const std::exception& e) { std::cout << "FAIL: " << e.what() << std::endl; failed++; } \
} while(0)

#define CHECK(cond) do { if (!(cond)) throw std::runtime_error(#cond); } while(0)

// --- Request ---

void test_req_get_roundtrip() {
    mcache::request req;
    req.cmd  = mcache::command::get;
    req.key  = "hello";
    req.ttl_ms = 0;

    auto encoded = req.encode();
    CHECK(encoded.size() == 20); // 15 + 5 + 0

    auto decoded = mcache::request::decode(encoded);
    CHECK(decoded.cmd == mcache::command::get);
    CHECK(decoded.key == "hello");
    CHECK(decoded.value.empty());
    CHECK(decoded.ttl_ms == 0);
}

void test_req_set_roundtrip() {
    mcache::request req;
    req.cmd  = mcache::command::set;
    req.key  = "key1";
    req.value.assign(std::byte{'w'}, std::byte{'o'}, std::byte{'r'}, std::byte{'l'}, std::byte{'d'});
    req.ttl_ms = 30000;

    auto encoded = req.encode();
    CHECK(encoded.size() == 24); // 15 + 4 + 5

    auto decoded = mcache::request::decode(encoded);
    CHECK(decoded.cmd == mcache::command::set);
    CHECK(decoded.key == "key1");
    CHECK(decoded.value.size() == 5);
    CHECK(decoded.ttl_ms == 30000);
}

void test_req_del_roundtrip() {
    mcache::request req;
    req.cmd = mcache::command::del;
    req.key = "bye";

    auto encoded = req.encode();
    auto decoded = mcache::request::decode(encoded);
    CHECK(decoded.cmd == mcache::command::del);
    CHECK(decoded.key == "bye");
}

void test_req_len_roundtrip() {
    mcache::request req;
    req.cmd = mcache::command::len;

    auto encoded = req.encode();
    CHECK(encoded.size() == 15);

    auto decoded = mcache::request::decode(encoded);
    CHECK(decoded.cmd == mcache::command::len);
    CHECK(decoded.key.empty());
    CHECK(decoded.value.empty());
}

void test_req_cleanup_roundtrip() {
    mcache::request req;
    req.cmd = mcache::command::cleanup;

    auto encoded = req.encode();
    CHECK(encoded.size() == 15);

    auto decoded = mcache::request::decode(encoded);
    CHECK(decoded.cmd == mcache::command::cleanup);
}

// --- Response ---

void test_resp_ok_roundtrip() {
    mcache::response resp;
    resp.stat = mcache::status::ok;
    resp.value.assign(std::byte{'r'}, std::byte{'e'}, std::byte{'s'}, std::byte{'u'}, std::byte{'l'}, std::byte{'t'});

    auto encoded = resp.encode();
    auto decoded = mcache::response::decode(encoded);
    CHECK(decoded.stat == mcache::status::ok);
    CHECK(decoded.value.size() == 6);
    CHECK(decoded.err_msg.empty());
}

void test_resp_err_roundtrip() {
    mcache::response resp;
    resp.stat = mcache::status::error;
    resp.err_msg = "something went wrong";

    auto encoded = resp.encode();
    auto decoded = mcache::response::decode(encoded);
    CHECK(decoded.stat == mcache::status::error);
    CHECK(decoded.err_msg == "something went wrong");
}

void test_resp_not_found_roundtrip() {
    mcache::response resp;
    resp.stat = mcache::status::not_found;

    auto encoded = resp.encode();
    auto decoded = mcache::response::decode(encoded);
    CHECK(decoded.stat == mcache::status::not_found);
    CHECK(decoded.value.empty());
    CHECK(decoded.err_msg.empty());
}

// --- Frame ---

void test_frame_roundtrip() {
    mcache::frame f;
    f.stream_id = 42;
    f.type = mcache::frame_type::request;
    f.flags = 0;
    f.payload.assign(std::byte{'p'}, std::byte{'a'}, std::byte{'y'}, std::byte{'l'}, std::byte{'o'}, std::byte{'a'}, std::byte{'d'});

    auto encoded = f.encode();
    auto decoded = mcache::frame::decode(encoded);
    CHECK(decoded.stream_id == 42);
    CHECK(decoded.type == mcache::frame_type::request);
    CHECK(decoded.flags == 0);
    CHECK(decoded.payload.size() == 7);
}

void test_frame_too_short() {
    std::vector<std::byte> data(5);
    bool threw = false;
    try { mcache::frame::decode(data); }
    catch (const mcache::protocol_error&) { threw = true; }
    CHECK(threw);
}

// --- result<T> ---

void test_result_value() {
    mcache::result<int> r(42);
    CHECK(r.has_value());
    CHECK(r.value() == 42);
    CHECK(r.error_code() == mcache::errc::ok);
}

void test_result_error() {
    mcache::result<int> r(mcache::errc::not_found, "missing");
    CHECK(!r.has_value());
    CHECK(r.error_code() == mcache::errc::not_found);
    CHECK(r.error_msg() == "missing");
}

int main() {
    std::cout << "=== mcache C++ SDK Protocol Tests ===" << std::endl << std::endl;

    TEST(req_get_roundtrip);
    TEST(req_set_roundtrip);
    TEST(req_del_roundtrip);
    TEST(req_len_roundtrip);
    TEST(req_cleanup_roundtrip);
    TEST(resp_ok_roundtrip);
    TEST(resp_err_roundtrip);
    TEST(resp_not_found_roundtrip);
    TEST(frame_roundtrip);
    TEST(frame_too_short);
    TEST(result_value);
    TEST(result_error);

    std::cout << std::endl << passed + failed << " tests, " << failed << " failed" << std::endl;
    return failed > 0 ? 1 : 0;
}
