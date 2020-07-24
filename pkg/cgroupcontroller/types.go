package cgroupcontroller

import (
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type ResourceConfig struct {
	Memory *int64
	CpuShares *uint64
	CpuQuota *int64
	CpuPeriod *uint64
	PidsLimit *int64
}

type CgroupName []string

type CgroupConfig struct {
	Name CgroupName
	ResourceParameters *ResourceConfig
}

type MemoryStats struct {
	Usage int64
}

type ResourceStats struct {
	MemoryStats *MemoryStats
}

type CgroupManager interface {
	Create(*CgroupConfig) error
	Destroy(*CgroupConfig) error
	Update(*CgroupConfig) error
	Exists(name CgroupName) bool
	Name(name CgroupName) string
	CgroupName(name string) CgroupName
	Pids(name CgroupName) []int
	ReduceCPULimits(cgroupName CgroupName) error
	GetResourceStats(name CgroupName) (*ResourceStats, error)
}

type QOSContainersInfo struct {
	Guaranteed CgroupName
	BestEffort CgroupName
	Burstable CgroupName
}

type PodContainerManager interface {
	GetPodContainerName(*v1.Pod) (CgroupName, string)
	EnsureExists(*v1.Pod) error
	Exists(*v1.Pod) bool
	Destroy(name CgroupName) error
	ReduceCPULimits(name CgroupName) error
	GetAllPodsFromCgroups() (map[types.UID]CgroupName, error)
	IsPodCgroup(cgroupfs string) (bool, types.UID)
}