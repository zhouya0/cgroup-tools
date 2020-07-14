// +build linux

package cgroupcontroller

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	libcontainercgroups "github.com/opencontainers/runc/libcontainer/cgroups"
	"k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/api/v1/resource"
	"k8s.io/apimachinery/pkg/types"
)

const (
	podCgroupNamePrefix = "pod"
	defaultNodeAllocatableCgroupName = "kubepods"

	// Taken from lmctfy https://github.com/google/lmctfy/blob/master/lmctfy/controllers/cpu_controller.cc
	MinShares     = 2
	SharesPerCPU  = 1024
	MilliCPUToCPU = 1000

	// 100000 is equivalent to 100ms
	QuotaPeriod    = 100000
	MinQuotaPeriod = 1000
)

func getCgroupSubsystemsV1() (*CgroupSubsystems, error) {
	allCgroups, err := libcontainercgroups.GetCgroupMounts(true)
	if err != nil {
		return &CgroupSubsystems{}, err
	}
	if len(allCgroups) == 0 {
		return &CgroupSubsystems{}, fmt.Errorf("failed to find cgroup mount")
	}
	mountPoints := make(map[string]string, len(allCgroups))
	for _, mount := range allCgroups {
		for _, subsystem := range mount.Subsystems {
			mountPoints[subsystem] = mount.Mountpoint
		}
	}
	return &CgroupSubsystems{
		Mounts:      allCgroups,
		MountPoints: mountPoints,
	}, nil
}

// GetCgroupSubsystems returns information about the mounted cgroup subsystems
func GetCgroupSubsystems() (*CgroupSubsystems, error) {
	return getCgroupSubsystemsV1()
}


func getCgroupProcs(dir string) ([]int, error) {
	procsFile := filepath.Join(dir, "cgroup.procs")
	f, err := os.Open(procsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []int{}, nil
		}
		return nil, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	out := []int{}
	for s.Scan() {
		if t := s.Text(); t != "" {
			pid, err := strconv.Atoi(t)
			if err !=nil {
				return nil, fmt.Errorf("unexpected line in %v; could not convert to pid: %v", procsFile, err)
			}
			out = append(out, pid)
		}
	}
	return out, nil
}

func GetPodCgroupNameSuffix(podUID types.UID) string {
	return podCgroupNamePrefix + string(podUID)
}

// NodeAllocatableRoot returns the literal cgroup path for the node allocatable cgroup
func NodeAllocatableRoot(cgroupRoot string, cgroupsPerQOS bool, cgroupDriver string) string {
	nodeAllocatableRoot := ParseCgroupfsToCgroupName(cgroupRoot)
	if cgroupsPerQOS {
		nodeAllocatableRoot = NewCgroupName(nodeAllocatableRoot, defaultNodeAllocatableCgroupName)
	}
	if libcontainerCgroupManagerType(cgroupDriver) == libcontainerSystemd {
		return nodeAllocatableRoot.ToSystemd()
	}
	return nodeAllocatableRoot.ToCgroupfs()
}

func ResourceConfigForPod(pod *v1.Pod, enforceCPULimits bool, cpuPeriod uint64) *ResourceConfig {
	reqs, limits := resource.PodRequestsAndLimits(pod)

	cpuRequests := int64(0)
	cpuLimits := int64(0)
	memoryLimits := int64(0)
	if request, found := reqs[v1.ResourceCPU]; found {
		cpuRequests = request.MilliValue()
	}
	if limits, found := limits[v1.ResourceCPU]; found {
		cpuLimits = limits.MilliValue()
	}
	if limit, found := limits[v1.ResourceMemory]; found {
		memoryLimits = limit.Value()
	}

	cpuShares := MilliCPUToShares(cpuRequests)
	cpuQuota := MilliCPUToQuota(cpuLimits, int64(cpuPeriod))

	memoryLimitsDeclared := true
	cpuLimitsDeclared := true
	hugePageLimits := map[int64]int64{}
	for _, container := range pod.Spec.Containers {
		if container.Resources.Limits.Cpu().IsZero() {
			cpuLimitsDeclared = false
		}
		if container.Resources.Limits.Memory().IsZero() {
			memoryLimitsDeclared = false
		}
		containerHugePageLimits := HugePageLimits(container.Resources.Requests)
		for k, v := range containerHugePageLimits {
			if value, exists := hugePageLimits[k]; exists {
				hugePageLimits[k] = value + v
			} else {
				hugePageLimits[k] = v
			}
		}
	}

	// quota is not capped when cfs quota is disabled
	if !enforceCPULimits {
		cpuQuota = int64(-1)
	}

	// determine the qos class
	qosClass := v1qos.GetPodQOS(pod)

	// build the result
	result := &ResourceConfig{}
	if qosClass == v1.PodQOSGuaranteed {
		result.CpuShares = &cpuShares
		result.CpuQuota = &cpuQuota
		result.CpuPeriod = &cpuPeriod
		result.Memory = &memoryLimits
	} else if qosClass == v1.PodQOSBurstable {
		result.CpuShares = &cpuShares
		if cpuLimitsDeclared {
			result.CpuQuota = &cpuQuota
			result.CpuPeriod = &cpuPeriod
		}
		if memoryLimitsDeclared {
			result.Memory = &memoryLimits
		}
	} else {
		shares := uint64(MinShares)
		result.CpuShares = &shares
	}
	result.HugePageLimit = hugePageLimits
	return result
}

// MilliCPUToShares converts the milliCPU to CFS shares.
func MilliCPUToShares(milliCPU int64) uint64 {
	if milliCPU == 0 {
		// Docker converts zero milliCPU to unset, which maps to kernel default
		// for unset: 1024. Return 2 here to really match kernel default for
		// zero milliCPU.
		return MinShares
	}
	// Conceptually (milliCPU / milliCPUToCPU) * sharesPerCPU, but factored to improve rounding.
	shares := (milliCPU * SharesPerCPU) / MilliCPUToCPU
	if shares < MinShares {
		return MinShares
	}
	return uint64(shares)
}

func MilliCPUToQuota(milliCPU int64, period int64) (quota int64) {
	// CFS quota is measured in two values:
	//  - cfs_period_us=100ms (the amount of time to measure usage across given by period)
	//  - cfs_quota=20ms (the amount of cpu time allowed to be used across a period)
	// so in the above example, you are limited to 20% of a single CPU
	// for multi-cpu environments, you just scale equivalent amounts
	// see https://www.kernel.org/doc/Documentation/scheduler/sched-bwc.txt for details

	if milliCPU == 0 {
		return
	}

	// we then convert your milliCPU to a value normalized over a period
	quota = (milliCPU * period) / MilliCPUToCPU

	// quota needs to be a minimum of 1ms.
	if quota < MinQuotaPeriod {
		quota = MinQuotaPeriod
	}
	return
}

// GetPodCgroupNameSuffix returns the last element of the pod CgroupName identifier
func GetPodCgroupNameSuffix(podUID types.UID) string {
	return podCgroupNamePrefix + string(podUID)
}
