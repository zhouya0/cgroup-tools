// +build linux

package cgroupcontroller

import (
	"fmt"
	"io/ioutil"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/kubelet/metrics"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	libcontainercgroups "github.com/opencontainers/runc/libcontainer/cgroups"
	cgroupfs "github.com/opencontainers/runc/libcontainer/cgroups/fs"
	cgroupsystemd "github.com/opencontainers/runc/libcontainer/cgroups/systemd"
	libcontainerconfigs "github.com/opencontainers/runc/libcontainer/configs"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"k8s.io/apimachinery/pkg/util/sets"
)

type libcontainerCgroupManagerType string

const (
	libcontainerCgroupfs libcontainerCgroupManagerType = "cgroupfs"
	libcontainerSystemd libcontainerCgroupManagerType = "systemd"
	systemdSuffix string = ".slice"
	CgroupRoot = "/sys/fs/cgroup"
)

var RootCgroupName = CgroupName([]string{})

func NewCgroupName(base CgroupName, components ...string) CgroupName {
	for _, component := range components {
		if strings.Contains(component, "/") || strings.Contains(component, "_") {
			panic(fmt.Errorf("invalid character in component [%q] of CgroupName", component))
		}
	}
	return CgroupName(append(append([]string{}, base...), components...))
}

func escapeSystemdCgroupName(part string) string {
	return strings.Replace(part, "-", "_", -1)
}

func unescapeSystemdCgroupName(part string) string {
	return strings.Replace(part, "_", "-", -1)
}

// For example, the name {"kubepods", "burstable", "pod1234-abcd-5678-efgh"} becomes
// "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod1234_abcd_5678_efgh.slice"
func (cgroupName CgroupName) ToSystemd() string {
	if len(cgroupName) == 0 || (len(cgroupName) == 1 && cgroupName[0] == "") {
		return "/"
	}
	newparts := []string{}
	for _, part := range cgroupName {
		part = escapeSystemdCgroupName(part)
		newparts = append(newparts, part)
	}

	result, err := cgroupsystemd.ExpandSlice(strings.Join(newparts, "-") + systemdSuffix)
	if err != nil {
		// Should never happen...
		panic(fmt.Errorf("error converting cgroup name [%v] to systemd format: %v", cgroupName, err))
	}
	return result
}

func ParseSystemdToCgroupName(name string) CgroupName {
	driverName := path.Base(name)
	driverName = strings.TrimSuffix(driverName, systemdSuffix)
	parts := strings.Split(driverName, "-")
	result := []string{}
	for _, part := range parts {
		result = append(result, unescapeSystemdCgroupName(part))
	}
	return CgroupName(result)
}

func (cgroupName CgroupName) ToCgroupfs() string {
	return "/" + path.Join(cgroupName...)
}

func ParseCgroupfsToCgroupName(name string) CgroupName {
	components := strings.Split(strings.TrimPrefix(name, "/"), "/")
	if len(components) == 1 && components[0] == "" {
		components = []string{}
	}
	return CgroupName(components)
}

func IsSystemdStyleName(name string) bool {
	return strings.HasSuffix(name, systemdSuffix)
}

type libcontainerAdapter struct {
	cgroupManagerType libcontainerCgroupManagerType
}

func newLibcontainerAdapter(cgroupManagerType libcontainerCgroupManagerType) *libcontainerAdapter {
	return &libcontainerAdapter{cgroupManagerType: cgroupManagerType}
}

func (l *libcontainerAdapter) newManager(cgroups *libcontainerconfigs.Cgroup, paths map[string]string) (libcontainercgroups.Manager, error) {
	switch l.cgroupManagerType {
	case libcontainerCgroupfs:
		return cgroupfs.NewManager(cgroups, paths, false), nil
	case libcontainerSystemd:
		// this means you asked systemd to manage cgroups, but systemd was not on the host, so all you can do is panic...
		if !cgroupsystemd.IsRunningSystemd() {
			panic("systemd cgroup manager not available")
		}
		return cgroupsystemd.NewLegacyManager(cgroups, paths), nil
	}
	return nil, fmt.Errorf("invalid cgroup manager configuration")
}

type CgroupSubsystems struct {
	// Cgroup subsystem mounts.
	// e.g.: "/sys/fs/cgroup/cpu" -> ["cpu", "cpuacct"]
	Mounts []libcontainercgroups.Mount

	// Cgroup subsystem to their mount location.
	// e.g.: "cpu" -> "/sys/fs/cgroup/cpu"
	MountPoints map[string]string
}

// cgroupManagerImpl implements the CgroupManager interface.
// Its a stateless object which can be used to
// update,create or delete any number of cgroups
// It uses the Libcontainer raw fs cgroup manager for cgroup management.
type cgroupManagerImpl struct {
	// subsystems holds information about all the
	// mounted cgroup subsystems on the node
	subsystems *CgroupSubsystems
	// simplifies interaction with libcontainer and its cgroup managers
	adapter *libcontainerAdapter
}

var _ CgroupManager = &cgroupManagerImpl{}


// NewCgroupManager is a factory method that returns a CgroupManager
func NewCgroupManager(cs *CgroupSubsystems, cgroupDriver string) CgroupManager {
	managerType := libcontainerCgroupfs
	if cgroupDriver == string(libcontainerSystemd) {
		managerType = libcontainerSystemd
	}
	return &cgroupManagerImpl{
		subsystems: cs,
		adapter:    newLibcontainerAdapter(managerType),
	}
}

// Name converts the cgroup to the driver specific value in cgroup form.
func (m *cgroupManagerImpl) Name(name CgroupName) string {
	if m.adapter.cgroupManagerType == libcontainerSystemd {
		return name.ToSystemd()
	}
	return name.ToCgroupfs()
}

// CgroupName converts the literal cgroupfs name on the host to an internal identifier.
func (m *cgroupManagerImpl) CgroupName(name string) CgroupName {
	if m.adapter.cgroupManagerType == libcontainerSystemd {
		return ParseSystemdToCgroupName(name)
	}
	return ParseCgroupfsToCgroupName(name)
}

// buildCgroupPaths builds a path to each cgroup subsystem for the specified name.
func (m *cgroupManagerImpl) buildCgroupPaths(name CgroupName) map[string]string {
	cgroupFsAdaptedName := m.Name(name)
	cgroupPaths := make(map[string]string, len(m.subsystems.MountPoints))
	for key, val := range m.subsystems.MountPoints {
		cgroupPaths[key] = path.Join(val, cgroupFsAdaptedName)
	}
	return cgroupPaths
}

func (m *cgroupManagerImpl) buildCgroupUnifiedPath(name CgroupName) string {
	cgroupFsAdaptedName := m.Name(name)
	return path.Join(CgroupRoot, cgroupFsAdaptedName)
}

func updateSystemdCgroupInfo(cgroupConfig *libcontainerconfigs.Cgroup, cgroupName CgroupName) {
	dir, base := path.Split(cgroupName.ToSystemd())
	if dir == "/" {
		dir = "-.slice"
	} else {
		dir = path.Base(dir)
	}
	cgroupConfig.Parent = dir
	cgroupConfig.Name = base
}

func (m *cgroupManagerImpl) Exists(name CgroupName) bool {
	cgroupPaths := m.buildCgroupPaths(name)
	whitelistControllers := sets.NewString("cpu", "cpuacct", "cpuset", "memory", "systemd", "pids", "hugetlb")
	var misssingPaths []string
	for controller, path := range cgroupPaths {
		if !whitelistControllers.Has(controller) {
			continue
		}
		if !libcontainercgroups.PathExists(path) {
			misssingPaths = append(misssingPaths, path)
		}
	}

	if len(misssingPaths) > 0 {
		klog.V(4).Info("The Cgroup %v has some missing paths: %v", name, misssingPaths)
		return false
	}
	return true
}

func (m *cgroupManagerImpl) Destroy(cgroupConfig *CgroupConfig) error {
	cgroupPaths := m.buildCgroupPaths(cgroupConfig.Name)
	libcontainerCgroupConfig := &libcontainerconfigs.Cgroup{}
	if m.adapter.cgroupManagerType == libcontainerSystemd {
		updateSystemdCgroupInfo(libcontainerCgroupConfig, cgroupConfig.Name)
	} else {
		libcontainerCgroupConfig.Path = cgroupConfig.Name.ToCgroupfs()
	}

	manager, err := m.adapter.newManager(libcontainerCgroupConfig, cgroupPaths)
	if err != nil {
		return err
	}
	// So Destroy would call libcontainercgroups.Manager.Destroy()
	if err = manager.Destroy(); err != nil {
		return fmt.Errorf("unable to destroy cgroup paths for cgroup %v : %v", cgroupConfig.Name, err)
	}
	return nil
}

type subsystem interface {
	Name() string
	Set(path string, cgroup *libcontainerconfigs.Cgroup) error
	GetStats(path string, stats *libcontainercgroups.Stats) error
}

func getSupportedSubsystems() map[subsystem]bool {
	supportedSubsystems := map[subsystem]bool {
		&cgroupfs.MemoryGroup{}: true,
		&cgroupfs.CpuGroup{}: true,
		&cgroupfs.PidsGroup{}: false,
		&cgroupfs.HugetlbGroup{}: false,
		&cgroupfs.PidsGroup{}: true,
	}
	return supportedSubsystems
}

func setSupportedSubsystemsV1(cgroupConfig *libcontainerconfigs.Cgroup) error {
	for sys, required := range getSupportedSubsystems() {
		if _, ok := cgroupConfig.Paths[sys.Name()]; !ok {
			if required {
				return fmt.Errorf("failed to find subsystem mount for required subsystem: %v", sys.Name())
			}
			// the cgroup is not mounted, but its not required so continue...
			klog.V(6).Infof("Unable to find subsystem mount for optional subsystem: %v", sys.Name())
			continue
		}
		if err := sys.Set(cgroupConfig.Paths[sys.Name()], cgroupConfig); err != nil {
			return fmt.Errorf("failed to set config for supported subsystems : %v", err)
		}
	}
	return nil
}

// getCpuWeight converts from the range [2, 262144] to [1, 10000]
func getCpuWeight(cpuShares *uint64) uint64 {
	if cpuShares == nil {
		return 0
	}
	if *cpuShares >= 262144 {
		return 10000
	}
	return 1 + ((*cpuShares-2)*9999)/262142
}

// readUnifiedControllers reads the controllers available at the specified cgroup
func readUnifiedControllers(path string) (sets.String, error) {
	controllersFileContent, err := ioutil.ReadFile(filepath.Join(path, "cgroup.controllers"))
	if err != nil {
		return nil, err
	}
	controllers := strings.Fields(string(controllersFileContent))
	return sets.NewString(controllers...), nil
}

var (
	availableRootControllersOnce sync.Once
	availableRootControllers sets.String
)

// getSupportedUnifiedControllers returns a set of supported controllers when running on cgroup v2
// just extend cgroup.controllers
func getSupportedUnifiedControllers() sets.String {
	// This is the set of controllers used by the Kubelet
	supportedControllers := sets.NewString("cpu", "cpuset", "memory", "hugetlb", "pids")
	// Memoize the set of controllers that are present in the root cgroup
	availableRootControllersOnce.Do(func() {
		var err error
		availableRootControllers, err = readUnifiedControllers(CgroupRoot)
		if err != nil {
			panic(fmt.Errorf("cannot read cgroup controllers at %s", CgroupRoot))
		}
	})
	// Return the set of controllers that are supported both by the Kubelet and by the kernel
	return supportedControllers.Intersection(availableRootControllers)
}

func propagateControllers(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("failed to read controllers from %q : %v", CgroupRoot, err)
	}
	controllersFileContent, err := ioutil.ReadFile(filepath.Join(CgroupRoot, "cgroup.controllers"))
	if err != nil {
		return fmt.Errorf("failed to read controllers from %q : %v", CgroupRoot, err)
	}

	supportedControllers := getSupportedUnifiedControllers()
	// The retrieved content looks like: "cpuset cpu io memory hugetlb pids".  Prepend each of the controllers
	// with '+', so we have something like "+cpuset +cpu +io +memory +hugetlb +pids"
	controllers := ""
	for _, controller := range strings.Fields(string(controllersFileContent)) {
		// ignore controllers we don't care about
		if !supportedControllers.Has(controller) {
			continue
		}

		sep := " +"
		if controllers == "" {
			sep = "+"
		}
		controllers = controllers + sep + controller
	}

	current := CgroupRoot
	relPath, err := filepath.Rel(CgroupRoot, path)
	if err != nil {
		return fmt.Errorf("failed to get relative path to cgroup root from %q: %v", path, err)
	}
	// Write the controllers list to each "cgroup.subtree_control" file until it reaches the parent cgroup.
	// For the /foo/bar/baz cgroup, controllers must be enabled sequentially in the files:
	// - /sys/fs/cgroup/foo/cgroup.subtree_control
	// - /sys/fs/cgroup/foo/bar/cgroup.subtree_control
	for _, p := range strings.Split(filepath.Dir(relPath), "/") {
		current = filepath.Join(current, p)
		if err := ioutil.WriteFile(filepath.Join(current, "cgroup.subtree_control"), []byte(controllers), 0755); err != nil {
			return fmt.Errorf("failed to enable controllers on %q: %v", CgroupRoot, err)
		}
	}
	return nil
}

func (m *cgroupManagerImpl) toResources(resourceConfig *ResourceConfig) *libcontainerconfigs.Resources {
	resources := &libcontainerconfigs.Resources{
		Devices: []*libcontainerconfigs.DeviceRule{
			{
				Type:        'a',
				Permissions: "rwm",
				Allow:       true,
				Minor:       libcontainerconfigs.Wildcard,
				Major:       libcontainerconfigs.Wildcard,
			},
		},
	}
	if resourceConfig == nil {
		return resources
	}
	if resourceConfig.Memory != nil {
		resources.Memory = *resourceConfig.Memory
	}
	if resourceConfig.CpuShares != nil {
		resources.CpuShares = *resourceConfig.CpuShares
	}
	if resourceConfig.CpuQuota != nil {
		resources.CpuQuota = *resourceConfig.CpuQuota
	}
	if resourceConfig.CpuPeriod != nil {
		resources.CpuPeriod = *resourceConfig.CpuPeriod
	}
	if resourceConfig.PidsLimit != nil {
		resources.PidsLimit = *resourceConfig.PidsLimit
	}
	return resources
}

// Update updates the cgroup with the specified Cgroup Configuration
func (m *cgroupManagerImpl) Update(cgroupConfig *CgroupConfig) error {
	start := time.Now()
	defer func() {
		metrics.CgroupManagerDuration.WithLabelValues("update").Observe(metrics.SinceInSeconds(start))
	}()

	// Extract the cgroup resource parameters
	resourceConfig := cgroupConfig.ResourceParameters
	resources := m.toResources(resourceConfig)

	libcontainerCgroupConfig := &libcontainerconfigs.Cgroup{
		Resources: resources,
	}



	libcontainerCgroupConfig.Paths = m.buildCgroupPaths(cgroupConfig.Name)


	// libcontainer consumes a different field and expects a different syntax
	// depending on the cgroup driver in use, so we need this conditional here.
	if m.adapter.cgroupManagerType == libcontainerSystemd {
		updateSystemdCgroupInfo(libcontainerCgroupConfig, cgroupConfig.Name)
	}

	if cgroupConfig.ResourceParameters != nil && cgroupConfig.ResourceParameters.PidsLimit != nil {
		libcontainerCgroupConfig.PidsLimit = *cgroupConfig.ResourceParameters.PidsLimit
	}

	if err := setSupportedSubsystemsV1(libcontainerCgroupConfig); err != nil {
		return fmt.Errorf("failed to set supported cgroup subsystems for cgroup %v: %v", cgroupConfig.Name, err)
	}

	return nil
}

// Create creates the specified cgroup
func (m *cgroupManagerImpl) Create(cgroupConfig *CgroupConfig) error {
	start := time.Now()
	defer func() {
		metrics.CgroupManagerDuration.WithLabelValues("create").Observe(metrics.SinceInSeconds(start))
	}()

	resources := m.toResources(cgroupConfig.ResourceParameters)

	libcontainerCgroupConfig := &libcontainerconfigs.Cgroup{
		Resources: resources,
	}
	// libcontainer consumes a different field and expects a different syntax
	// depending on the cgroup driver in use, so we need this conditional here.
	if m.adapter.cgroupManagerType == libcontainerSystemd {
		updateSystemdCgroupInfo(libcontainerCgroupConfig, cgroupConfig.Name)
	} else {
		libcontainerCgroupConfig.Path = cgroupConfig.Name.ToCgroupfs()
	}

	libcontainerCgroupConfig.PidsLimit = *cgroupConfig.ResourceParameters.PidsLimit

	// get the manager with the specified cgroup configuration
	manager, err := m.adapter.newManager(libcontainerCgroupConfig, nil)
	if err != nil {
		return err
	}

	// Apply(-1) is a hack to create the cgroup directories for each resource
	// subsystem. The function [cgroups.Manager.apply()] applies cgroup
	// configuration to the process with the specified pid.
	// It creates cgroup files for each subsystems and writes the pid
	// in the tasks file. We use the function to create all the required
	// cgroup files but not attach any "real" pid to the cgroup.
	if err := manager.Apply(-1); err != nil {
		return err
	}

	// it may confuse why we call set after we do apply, but the issue is that runc
	// follows a similar pattern.  it's needed to ensure cpu quota is set properly.
	if err := m.Update(cgroupConfig); err != nil {
		utilruntime.HandleError(fmt.Errorf("cgroup update failed %v", err))
	}

	return nil
}


// Scans through all subsystems to find pids associated with specified cgroup.
func (m *cgroupManagerImpl) Pids(name CgroupName) []int {
	// we need the driver specific name
	cgroupFsName := m.Name(name)

	// Get a list of processes that we need to kill
	pidsToKill := sets.NewInt()
	var pids []int
	for _, val := range m.subsystems.MountPoints {
		dir := path.Join(val, cgroupFsName)
		_, err := os.Stat(dir)
		if os.IsNotExist(err) {
			// The subsystem pod cgroup is already deleted
			// do nothing, continue
			continue
		}
		// Get a list of pids that are still charged to the pod's cgroup
		pids, err = getCgroupProcs(dir)
		if err != nil {
			continue
		}
		pidsToKill.Insert(pids...)

		// WalkFunc which is called for each file and directory in the pod cgroup dir
		visitor := func(path string, info os.FileInfo, err error) error {
			if err != nil {
				klog.V(4).Infof("cgroup manager encountered error scanning cgroup path %q: %v", path, err)
				return filepath.SkipDir
			}
			if !info.IsDir() {
				return nil
			}
			pids, err = getCgroupProcs(path)
			if err != nil {
				klog.V(4).Infof("cgroup manager encountered error getting procs for cgroup path %q: %v", path, err)
				return filepath.SkipDir
			}
			pidsToKill.Insert(pids...)
			return nil
		}
		// Walk through the pod cgroup directory to check if
		// container cgroups haven't been GCed yet. Get attached processes to
		// all such unwanted containers under the pod cgroup
		if err = filepath.Walk(dir, visitor); err != nil {
			klog.V(4).Infof("cgroup manager encountered error scanning pids for directory: %q: %v", dir, err)
		}
	}
	return pidsToKill.List()
}

// ReduceCPULimits reduces the cgroup's cpu shares to the lowest possible value
func (m *cgroupManagerImpl) ReduceCPULimits(cgroupName CgroupName) error {
	// Set lowest possible CpuShares value for the cgroup
	minimumCPUShares := uint64(MinShares)
	resources := &ResourceConfig{
		CpuShares: &minimumCPUShares,
	}
	containerConfig := &CgroupConfig{
		Name:               cgroupName,
		ResourceParameters: resources,
	}
	return m.Update(containerConfig)
}

func getStatsSupportedSubsystems(cgroupPaths map[string]string) (*libcontainercgroups.Stats, error) {
	stats := libcontainercgroups.NewStats()
	for sys, required := range getSupportedSubsystems() {
		if _, ok := cgroupPaths[sys.Name()]; !ok {
			if required {
				return nil, fmt.Errorf("failed to find subsystem mount for required subsystem: %v", sys.Name())
			}
			// the cgroup is not mounted, but its not required so continue...
			klog.V(6).Infof("Unable to find subsystem mount for optional subsystem: %v", sys.Name())
			continue
		}
		if err := sys.GetStats(cgroupPaths[sys.Name()], stats); err != nil {
			return nil, fmt.Errorf("failed to get stats for supported subsystems : %v", err)
		}
	}
	return stats, nil
}

func toResourceStats(stats *libcontainercgroups.Stats) *ResourceStats {
	return &ResourceStats{
		MemoryStats: &MemoryStats{
			Usage: int64(stats.MemoryStats.Usage.Usage),
		},
	}
}

// Get sets the ResourceParameters of the specified cgroup as read from the cgroup fs
func (m *cgroupManagerImpl) GetResourceStats(name CgroupName) (*ResourceStats, error) {
	var err error
	var stats *libcontainercgroups.Stats
	cgroupPaths := m.buildCgroupPaths(name)
	stats, err = getStatsSupportedSubsystems(cgroupPaths)
	if err != nil {
			return nil, fmt.Errorf("failed to get stats supported cgroup subsystems for cgroup %v: %v", name, err)
		}
	return toResourceStats(stats), nil
}