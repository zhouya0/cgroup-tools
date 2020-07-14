// +build linux

package cgroupcontroller

import (
	"fmt"
	"io/ioutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"os"
	"path"
	"strings"
)

const (
	podCgroupNamePrefix = "pod"
)

type podContainerManagerImpl struct {
	// qosContainersInfo hold absolute paths of the top level qos containers
	qosContainersInfo QOSContainersInfo
	// Stores the mounted cgroup subsystems
	subsystems *CgroupSubsystems
	// cgroupManager is the cgroup Manager Object responsible for managing all
	// pod cgroups.
	cgroupManager CgroupManager
	// Maximum number of pids in a pod
	podPidsLimit int64
	// enforceCPULimits controls whether cfs quota is enforced or not
	enforceCPULimits bool
	// cpuCFSQuotaPeriod is the cfs period value, cfs_period_us, setting per
	// node for all containers in usec
	cpuCFSQuotaPeriod uint64
}

// Make sure that podContainerManagerImpl implements the PodContainerManager interface
var _ PodContainerManager = &podContainerManagerImpl{}

// Exists checks if the pod's cgroup already exists
func (m *podContainerManagerImpl) Exists(pod *v1.Pod) bool {
	podContainerName, _ := m.GetPodContainerName(pod)
	return m.cgroupManager.Exists(podContainerName)
}

func (m *podContainerManagerImpl) EnsureExists(pod *v1.Pod) error {
	podContainerName, _ := m.GetPodContainerName(pod)
	alreadyExists := m.Exists(pod)
	if !alreadyExists {
		containerConfig := &CgroupConfig{
			Name: podContainerName,
			ResourceParameters: ResourceConfigForPod(pod, m.enforceCPULimits, m.cpuCFSQuotaPeriod),
		}
		if m.podPidsLimit >0 {
			containerConfig.ResourceParameters.PidsLimit = &m.podPidsLimit
		}
		if err := m.cgroupManager.Create(containerConfig); err != nil {
			return fmt.Errorf("failed to create container for %v : %v", podContainerName, err)
		}
	}
	if err := m.applyLimits(pod); err != nil {
		return fmt.Errorf("failed to apply resource limits on container for %v : %v", podContainerName, err)
	}
	return nil
}

// GetPodContainerName returns the CgroupName identifier, and its literal cgroupfs form on the host.
func (m *podContainerManagerImpl) GetPodContainerName(pod *v1.Pod) (CgroupName, string) {
	podQOS := v1qos.GetPodQOS(pod)
	// Get the parent QOS container name
	var parentContainer CgroupName
	switch podQOS {
	case v1.PodQOSGuaranteed:
		parentContainer = m.qosContainersInfo.Guaranteed
	case v1.PodQOSBurstable:
		parentContainer = m.qosContainersInfo.Burstable
	case v1.PodQOSBestEffort:
		parentContainer = m.qosContainersInfo.BestEffort
	}
	podContainer := GetPodCgroupNameSuffix(pod.UID)

	// Get the absolute path of the cgroup
	cgroupName := NewCgroupName(parentContainer, podContainer)
	// Get the literal cgroupfs name
	cgroupfsName := m.cgroupManager.Name(cgroupName)

	return cgroupName, cgroupfsName
}

func (m *podContainerManagerImpl) killOnePid(pid int) error {
	p, _ := os.FindProcess(pid)
	if err := p.Kill(); err != nil {
		if strings.Contains(err.Error(), "process already finished") {
			klog.V(3).Infof("process with pid %v no longer exists", pid)
			return nil
		}
		return err
	}
	return nil
}

// Scan through the whole cgroup directory and kill all processes either
// attached to the pod cgroup or to a container cgroup under the pod cgroup
func (m *podContainerManagerImpl) tryKillingCgroupProcesses(podCgroup CgroupName) error {
	pidsToKill := m.cgroupManager.Pids(podCgroup)
	// No pids charged to the terminated pod cgroup return
	if len(pidsToKill) == 0 {
		return nil
	}

	var errlist []error
	// os.Kill often errors out,
	// We try killing all the pids multiple times
	removed := map[int]bool{}
	for i := 0; i < 5; i++ {
		if i != 0 {
			klog.V(3).Infof("Attempt %v failed to kill all unwanted process from cgroup: %v. Retyring", i, podCgroup)
		}
		errlist = []error{}
		for _, pid := range pidsToKill {
			if _, ok := removed[pid]; ok {
				continue
			}
			klog.V(3).Infof("Attempt to kill process with pid: %v from cgroup: %v", pid, podCgroup)
			if err := m.killOnePid(pid); err != nil {
				klog.V(3).Infof("failed to kill process with pid: %v from cgroup: %v", pid, podCgroup)
				errlist = append(errlist, err)
			} else {
				removed[pid] = true
			}
		}
		if len(errlist) == 0 {
			klog.V(3).Infof("successfully killed all unwanted processes from cgroup: %v", podCgroup)
			return nil
		}
	}
	return utilerrors.NewAggregate(errlist)
}

// Destroy destroys the pod container cgroup paths
func (m *podContainerManagerImpl) Destroy(podCgroup CgroupName) error {
	// Try killing all the processes attached to the pod cgroup
	if err := m.tryKillingCgroupProcesses(podCgroup); err != nil {
		klog.Warningf("failed to kill all the processes attached to the %v cgroups", podCgroup)
		return fmt.Errorf("failed to kill all the processes attached to the %v cgroups : %v", podCgroup, err)
	}

	// Now its safe to remove the pod's cgroup
	containerConfig := &CgroupConfig{
		Name:               podCgroup,
		ResourceParameters: &ResourceConfig{},
	}
	if err := m.cgroupManager.Destroy(containerConfig); err != nil {
		klog.Warningf("failed to delete cgroup paths for %v : %v", podCgroup, err)
		return fmt.Errorf("failed to delete cgroup paths for %v : %v", podCgroup, err)
	}
	return nil
}

// ReduceCPULimits reduces the CPU CFS values to the minimum amount of shares.
func (m *podContainerManagerImpl) ReduceCPULimits(podCgroup CgroupName) error {
	return m.cgroupManager.ReduceCPULimits(podCgroup)
}

// IsPodCgroup returns true if the literal cgroupfs name corresponds to a pod
func (m *podContainerManagerImpl) IsPodCgroup(cgroupfs string) (bool, types.UID) {
	// convert the literal cgroupfs form to the driver specific value
	cgroupName := m.cgroupManager.CgroupName(cgroupfs)
	qosContainersList := [3]CgroupName{m.qosContainersInfo.BestEffort, m.qosContainersInfo.Burstable, m.qosContainersInfo.Guaranteed}
	basePath := ""
	for _, qosContainerName := range qosContainersList {
		// a pod cgroup is a direct child of a qos node, so check if its a match
		if len(cgroupName) == len(qosContainerName)+1 {
			basePath = cgroupName[len(qosContainerName)]
		}
	}
	if basePath == "" {
		return false, types.UID("")
	}
	if !strings.HasPrefix(basePath, podCgroupNamePrefix) {
		return false, types.UID("")
	}
	parts := strings.Split(basePath, podCgroupNamePrefix)
	if len(parts) != 2 {
		return false, types.UID("")
	}
	return true, types.UID(parts[1])
}

// GetAllPodsFromCgroups scans through all the subsystems of pod cgroups
// Get list of pods whose cgroup still exist on the cgroup mounts
func (m *podContainerManagerImpl) GetAllPodsFromCgroups() (map[types.UID]CgroupName, error) {
	// Map for storing all the found pods on the disk
	foundPods := make(map[types.UID]CgroupName)
	qosContainersList := [3]CgroupName{m.qosContainersInfo.BestEffort, m.qosContainersInfo.Burstable, m.qosContainersInfo.Guaranteed}
	// Scan through all the subsystem mounts
	// and through each QoS cgroup directory for each subsystem mount
	// If a pod cgroup exists in even a single subsystem mount
	// we will attempt to delete it
	for _, val := range m.subsystems.MountPoints {
		for _, qosContainerName := range qosContainersList {
			// get the subsystems QoS cgroup absolute name
			qcConversion := m.cgroupManager.Name(qosContainerName)
			qc := path.Join(val, qcConversion)
			dirInfo, err := ioutil.ReadDir(qc)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("failed to read the cgroup directory %v : %v", qc, err)
			}
			for i := range dirInfo {
				// its not a directory, so continue on...
				if !dirInfo[i].IsDir() {
					continue
				}
				// convert the concrete cgroupfs name back to an internal identifier
				// this is needed to handle path conversion for systemd environments.
				// we pass the fully qualified path so decoding can work as expected
				// since systemd encodes the path in each segment.
				cgroupfsPath := path.Join(qcConversion, dirInfo[i].Name())
				internalPath := m.cgroupManager.CgroupName(cgroupfsPath)
				// we only care about base segment of the converted path since that
				// is what we are reading currently to know if it is a pod or not.
				basePath := internalPath[len(internalPath)-1]
				if !strings.Contains(basePath, podCgroupNamePrefix) {
					continue
				}
				// we then split the name on the pod prefix to determine the uid
				parts := strings.Split(basePath, podCgroupNamePrefix)
				// the uid is missing, so we log the unexpected cgroup not of form pod<uid>
				if len(parts) != 2 {
					klog.Errorf("pod cgroup manager ignoring unexpected cgroup %v because it is not a pod", cgroupfsPath)
					continue
				}
				podUID := parts[1]
				foundPods[types.UID(podUID)] = internalPath
			}
		}
	}
	return foundPods, nil
}