cmake_minimum_required(VERSION 3.14)
project(mcache_cpp_sdk VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Compiled library (platform + connection socket I/O)
set(CPP_SOURCES
    src/protocol.cpp
    src/frame.cpp
    src/connection.cpp
)

add_library(mcache_cpp STATIC ${CPP_SOURCES})
target_include_directories(mcache_cpp PUBLIC
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
    $<INSTALL_INTERFACE:include>
)
target_include_directories(mcache_cpp PRIVATE src)
target_compile_features(mcache_cpp PUBLIC cxx_std_17)

if(WIN32)
    target_link_libraries(mcache_cpp PUBLIC ws2_32)
else()
    target_link_libraries(mcache_cpp PUBLIC pthread)
endif()

# Installation
include(GNUInstallDirs)
install(TARGETS mcache_cpp
    LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
    ARCHIVE DESTINATION ${CMAKE_INSTALL_LIBDIR}
)
install(DIRECTORY include/ DESTINATION ${CMAKE_INSTALL_INCLUDEDIR})

# Optional: find nlohmann_json
find_package(nlohmann_json QUIET)
if(nlohmann_json_FOUND)
    target_link_libraries(mcache_cpp PUBLIC nlohmann_json::nlohmann_json)
    target_compile_definitions(mcache_cpp PUBLIC MCACHE_USE_NLOHMANN_JSON)
endif()

# Tests
option(MCACHE_BUILD_TESTS "Build tests" ON)
if(MCACHE_BUILD_TESTS)
    enable_testing()

    # Protocol tests (no server needed)
    add_executable(mcache_cpp_test_protocol
        tests/test_protocol.cpp
        src/protocol.cpp
        src/frame.cpp
    )
    target_include_directories(mcache_cpp_test_protocol PRIVATE include src)
    target_compile_features(mcache_cpp_test_protocol PRIVATE cxx_std_17)
    add_test(NAME mcache_cpp_test_protocol COMMAND mcache_cpp_test_protocol)

    # Client tests (needs running server)
    add_executable(mcache_cpp_test_client
        tests/test_client.cpp
        src/protocol.cpp
        src/frame.cpp
        src/connection.cpp
    )
    target_include_directories(mcache_cpp_test_client PRIVATE include src)
    target_compile_features(mcache_cpp_test_client PRIVATE cxx_std_17)
    if(WIN32)
        target_link_libraries(mcache_cpp_test_client ws2_32)
    else()
        target_link_libraries(mcache_cpp_test_client pthread)
    endif()
    add_test(NAME mcache_cpp_test_client COMMAND mcache_cpp_test_client)
endif()
