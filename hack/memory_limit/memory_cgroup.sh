#!/bin/bash

cd /sys/fs/cgroup/memory
mkdir test_memory

# 物理内存 + SWAP <= 300 MBl; 1024*1024*300 = 314572800
echo 314572800 > test_memory/memory.limit_in_bytes
echo 0 > test_memory/memory.swappiness
