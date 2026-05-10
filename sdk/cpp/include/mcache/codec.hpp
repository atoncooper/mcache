#ifndef MCACHE_CODEC_HPP
#define MCACHE_CODEC_HPP

#include <vector>
#include <string>
#include <string_view>
#include <span>
#include <cstddef>
#include <type_traits>
#include <stdexcept>

namespace mcache {

// Codec concept (documentation only — checked at compile time via static_assert).
// A Codec must provide:
//   std::vector<std::byte> encode(const T& value) const;
//   void decode(std::span<const std::byte> data, T& value) const;

// raw_codec: pass-through for byte buffers and strings.
struct raw_codec {
    // Encode string as bytes
    std::vector<std::byte> encode(std::string_view s) const {
        return { reinterpret_cast<const std::byte*>(s.data()),
                 reinterpret_cast<const std::byte*>(s.data()) + s.size() };
    }

    // Encode byte vector (zero-copy move)
    std::vector<std::byte> encode(const std::vector<std::byte>& v) const {
        return v;
    }
    std::vector<std::byte> encode(std::vector<std::byte>&& v) const {
        return std::move(v);
    }

    // Decode bytes to string
    void decode(std::span<const std::byte> data, std::string& out) const {
        out.assign(reinterpret_cast<const char*>(data.data()), data.size());
    }

    // Decode bytes to byte vector
    void decode(std::span<const std::byte> data, std::vector<std::byte>& out) const {
        out.assign(data.begin(), data.end());
    }
};

// json_codec: JSON serialization via nlohmann/json (optional dependency).
// Enable by defining MCACHE_USE_NLOHMANN_JSON before including this header.
#ifdef MCACHE_USE_NLOHMANN_JSON

#include <nlohmann/json.hpp>

struct json_codec {
    template<typename T>
    std::vector<std::byte> encode(const T& value) const {
        auto j = nlohmann::json(value);
        auto s = j.dump();
        return { reinterpret_cast<const std::byte*>(s.data()),
                 reinterpret_cast<const std::byte*>(s.data()) + s.size() };
    }

    template<typename T>
    void decode(std::span<const std::byte> data, T& value) const {
        auto s = std::string(reinterpret_cast<const char*>(data.data()), data.size());
        value = nlohmann::json::parse(s).get<T>();
    }
};

#endif // MCACHE_USE_NLOHMANN_JSON

} // namespace mcache

#endif // MCACHE_CODEC_HPP
