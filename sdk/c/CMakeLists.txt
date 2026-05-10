cmake_minimum_required(VERSION 3.14)
project(mcache_c_sdk VERSION 1.0.0 LANGUAGES C)

set(CMAKE_C_STANDARD 11)
set(CMAKE_C_STANDARD_REQUIRED ON)

# Library target
set(SOURCES
    src/protocol.c
    src/client.c
    src/errors.c
)

add_library(mcache_c STATIC ${SOURCES})
add_library(mcache_c_shared SHARED ${SOURCES})

target_include_directories(mcache_c PUBLIC
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
    $<INSTALL_INTERFACE:include>
)
target_include_directories(mcache_c PRIVATE src)

target_include_directories(mcache_c_shared PUBLIC
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
    $<INSTALL_INTERFACE:include>
)
target_include_directories(mcache_c_shared PRIVATE src)

if(WIN32)
    target_link_libraries(mcache_c PUBLIC ws2_32)
    target_link_libraries(mcache_c_shared PUBLIC ws2_32)
else()
    target_link_libraries(mcache_c PUBLIC pthread)
    target_link_libraries(mcache_c_shared PUBLIC pthread)
endif()

# Installation
include(GNUInstallDirs)
install(TARGETS mcache_c mcache_c_shared
    LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
    ARCHIVE DESTINATION ${CMAKE_INSTALL_LIBDIR}
)
install(DIRECTORY include/ DESTINATION ${CMAKE_INSTALL_INCLUDEDIR})

# Tests
option(MCACHE_BUILD_TESTS "Build tests" ON)
if(MCACHE_BUILD_TESTS)
    enable_testing()
    add_executable(mcache_c_test
        tests/test_protocol.c
        tests/test_client.c
        src/protocol.c
        src/client.c
        src/errors.c
    )
    target_include_directories(mcache_c_test PRIVATE include src tests)
    if(WIN32)
        target_link_libraries(mcache_c_test ws2_32)
    else()
        target_link_libraries(mcache_c_test pthread)
    endif()
    add_test(NAME mcache_c_test COMMAND mcache_c_test)
endif()
