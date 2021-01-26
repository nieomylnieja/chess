cmake_minimum_required(VERSION 3.18.2)
project(server)

set(THREADS_PREFER_PTHREAD_FLAG ON)
set(CMAKE_C_STANDARD 11)

find_package(Threads REQUIRED)

add_executable(${PROJECT_NAME} main.c ../lib/log.c/src/log.c)

target_link_libraries(${PROJECT_NAME} PRIVATE Threads::Threads)
