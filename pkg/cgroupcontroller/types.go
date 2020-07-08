package cgroupcontroller

type ResourceConfig struct {
	Memory *int64
	CpuShares *uint64
	CpuQuota *int64
	CpuPeriod *uint64
	HugePageLimit map[int64]int64
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