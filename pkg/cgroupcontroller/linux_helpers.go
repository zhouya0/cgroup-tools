// +build linux

package cgroupcontroller

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	libcontainercgroups "github.com/opencontainers/runc/libcontainer/cgroups"

	"k8s.io/apimachinery/pkg/types"
)

const (
	podCgroupNamePrefix = "pod"
	defaultNodeAllocatableCgroupName = "kubepods"
	MinShares = 2
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

