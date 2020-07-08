// +build linux

package main

import (
	"fmt"
	"github.com/zhouya0/cgroup-tools/pkg/cgroupcontroller"
)

func main() {
	//testString := "env in ( de, ds )"
	//
	//// kubeClient, _ := client.BuildInClusterClientset()
	//kubeClient,_ := client.BuildLocalClientSet()
	//pods,_ := kubeClient.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{LabelSelector: testString})
	//fmt.Println("Listing pods:")
	//fmt.Println(pods)
	rootCgroupfsName := cgroupcontroller.ParseSystemdToCgroupName(cgroupcontroller.CgroupRoot)
	test := cgroupcontroller.NewCgroupName(cgroupcontroller.RootCgroupName, "kubepods", "burstable", "pod658827a8-0fb0-46b3-bde6-911c3e8d473c")
	if test != nil {}
	fmt.Println(rootCgroupfsName)
	fmt.Println(test)
	subSystems,_ := cgroupcontroller.GetCgroupSubsystems()
	fmt.Println(subSystems)
	cgroupManager := cgroupcontroller.NewCgroupManager(subSystems, "systemd")
	if cgroupManager.Exists(test) {
		fmt.Println("yes, it works!")
	}
}
