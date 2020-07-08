// +build linux

package cgroupcontroller

import (
	"fmt"

	"github.com/opencontainers/runc/libcontainer/cgroups"
)


func getContainer(pid int) (string, error) {
	cgs, err := cgroups.ParseCgroupFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return  "", err
	}

	cpu, found := cgs["cpu"]
	if !found {
		return "", cgroups.NewNotFoundError("cpu")
	}
	memory, found := cgs["memory"]
	if !found {
		return "", cgroups.NewNotFoundError("memory")
	}

	if cpu != memory {
		return "", fmt.Errorf("cpu and memory cgroup hierarchy not unified.  cpu: %s, memory: %s", cpu, memory)
	}
	return cpu, nil
}

