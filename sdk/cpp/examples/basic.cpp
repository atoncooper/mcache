#include <mcache/mcache.hpp>
#include <iostream>
#include <string>
#include <chrono>

int main() {
    try {
        // Connect to mcache server
        std::cout << "Connecting to mcache server at 127.0.0.1:11211..." << std::endl;

        mcache::options opts;
        opts.set_pool_size(4)
            .set_connect_timeout(std::chrono::seconds(5));

        mcache::client client("127.0.0.1:11211", opts);

        // Set a key
        std::string key = "example:greeting";
        std::string val = "Hello from C++ SDK!";
        std::cout << "SET " << key << " = \"" << val << "\"" << std::endl;

        auto set_result = client.set(key, val, std::chrono::minutes(1));
        if (!set_result) {
            std::cerr << "SET failed: " << set_result.error_msg() << std::endl;
            return 1;
        }
        std::cout << "OK" << std::endl << std::endl;

        // Get the key back
        std::cout << "GET " << key << std::endl;
        auto get_result = client.get<std::string>(key);
        if (!get_result) {
            std::cerr << "GET failed: " << get_result.error_msg() << std::endl;
            return 1;
        }
        std::cout << "= \"" << get_result.value() << "\"" << std::endl << std::endl;

        // Length
        auto len_result = client.len();
        if (len_result) {
            std::cout << "LEN = " << len_result.value() << std::endl << std::endl;
        }

        // Cleanup
        auto cleanup_result = client.cleanup();
        if (cleanup_result) {
            std::cout << "CLEANUP removed " << cleanup_result.value()
                      << " expired entries" << std::endl << std::endl;
        }

        // Delete
        std::cout << "DEL " << key << std::endl;
        auto del_result = client.del(key);
        if (!del_result) {
            std::cerr << "DEL failed: " << del_result.error_msg() << std::endl;
        } else {
            std::cout << "OK" << std::endl << std::endl;
        }

        // Verify deletion
        auto verify = client.get<std::string>(key);
        if (!verify && verify.error_code() == mcache::errc::not_found) {
            std::cout << "GET " << key << " → key not found (expected)" << std::endl;
        }

    } catch (const mcache::error& e) {
        std::cerr << "mcache error: " << e.what() << std::endl;
        return 1;
    } catch (const std::exception& e) {
        std::cerr << "error: " << e.what() << std::endl;
        return 1;
    }

    std::cout << "Done." << std::endl;
    return 0;
}
