#!/bin/bash

cd /sys/fs/cgroup/cpu
mkdir test_cpu
echo 100000 > test_cpu/cpu.cfs_period_us
echo 10000 > test_cpu/cpu.cfs_quota_us

