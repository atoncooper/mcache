#ifndef MCACHE_OPTIONS_HPP
#define MCACHE_OPTIONS_HPP

#include <chrono>

namespace mcache {

struct options {
    int                      pool_size        = 4;
    std::chrono::milliseconds connect_timeout{5000};
    std::chrono::milliseconds read_timeout   {10000};
    std::chrono::milliseconds write_timeout  {5000};

    options& set_pool_size(int n)        { pool_size = n; return *this; }
    options& set_connect_timeout(std::chrono::milliseconds d) { connect_timeout = d; return *this; }
    options& set_read_timeout(std::chrono::milliseconds d)    { read_timeout = d; return *this; }
    options& set_write_timeout(std::chrono::milliseconds d)   { write_timeout = d; return *this; }
};

} // namespace mcache

#endif // MCACHE_OPTIONS_HPP
