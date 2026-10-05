# The sszpp benchmark driver: the generated types and the protocol client
# against the header-only library in work/sszpp, intx in work/intx and the
# hashtree static library build.sh builds with the library's own Makefile.
# Release flags are CMake's (-O3 -DNDEBUG), without the library's
# -march=native and -ffast-math, so that the binary runs the same code on
# every box; hashtree picks its SHA-256 implementation at run time.
cmake_minimum_required(VERSION 3.20)
project(sszpp_bench LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 23)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE Release)
endif()

set(SSZPP_DIR "${CMAKE_SOURCE_DIR}/work/sszpp" CACHE PATH "the sszpp checkout")
set(INTX_DIR "${CMAKE_SOURCE_DIR}/work/intx" CACHE PATH "the intx checkout")
set(HASHTREE_DIR "${CMAKE_SOURCE_DIR}/work/hashtree" CACHE PATH "the hashtree checkout")
set(HASHTREE_LIB "${CMAKE_BINARY_DIR}/hashtree/lib/libhashtree.a" CACHE FILEPATH "the built libhashtree.a")

find_package(Threads REQUIRED)

add_executable(bench src/main.cpp src/protocol.cpp)
target_include_directories(bench PRIVATE
  "${CMAKE_SOURCE_DIR}/src"
  "${SSZPP_DIR}/lib"
  "${INTX_DIR}/include"
  "${HASHTREE_DIR}/src")
target_link_libraries(bench PRIVATE "${HASHTREE_LIB}" Threads::Threads)
target_compile_options(bench PRIVATE -Wall -Wno-unused-variable)
