#include <mcache/mcache.hpp>
#include <iostream>
#include <vector>
#include <chrono>

int main() {
    try {
        std::vector<std::string> nodes = {
            "127.0.0.1:11211",
            "127.0.0.1:11212",
            "127.0.0.1:11213",
        };

        std::cout << "Connecting to mcache cluster: ";
        for (auto& n : nodes) std::cout << n << " ";
        std::cout << std::endl;

        mcache::cluster_client cluster(nodes);

        // Set keys — automatically routed to correct node by FNV-1a hash
        for (int i = 0; i < 10; ++i) {
            std::string key = "user:" + std::to_string(i);
            std::string val = "data_" + std::to_string(i);

            auto r = cluster.set(key, val, std::chrono::minutes(5));
            if (!r) {
                std::cerr << "SET " << key << " failed: " << r.error_msg() << std::endl;
            }
        }
        std::cout << "Stored 10 keys across cluster." << std::endl;

        // Read them back
        for (int i = 0; i < 10; ++i) {
            std::string key = "user:" + std::to_string(i);
            auto r = cluster.get<std::string>(key);
            if (r) {
                std::cout << "GET " << key << " = \"" << r.value() << "\"" << std::endl;
            }
        }

        // Aggregate length
        auto len_r = cluster.len();
        if (len_r) {
            std::cout << "Total entries across cluster: " << len_r.value() << std::endl;
        }

        // Cleanup
        for (int i = 0; i < 10; ++i) {
            cluster.del("user:" + std::to_string(i));
        }
        std::cout << "Cleaned up." << std::endl;

    } catch (const mcache::error& e) {
        std::cerr << "mcache error: " << e.what() << std::endl;
        return 1;
    }

    return 0;
}
