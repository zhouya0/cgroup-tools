// +build linux

package main

import (
	"fmt"
	"github.com/zhouya0/cgroup-tools/pkg/client"
	"github.com/zhouya0/cgroup-tools/pkg/cgroupcontroller"
	"github.com/zhouya0/cgroup-tools/pkg/podsgetter"
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
	test := cgroupcontroller.NewCgroupName(cgroupcontroller.RootCgroupName, "kubepods", "burstable", "pod52bcad2fbd5e77e241df097f496d7b0c")
	if test != nil {}
	fmt.Println(rootCgroupfsName)
	fmt.Println(test)
	subSystems,_ := cgroupcontroller.GetCgroupSubsystems()
	fmt.Println(subSystems)
	cgroupManager := cgroupcontroller.NewCgroupManager(subSystems, "systemd")
	if cgroupManager.Exists(test) {
		fmt.Println("yes, it works!")
	}

	client,err := client.BuildLocalClientSet()
	if err != nil {
		fmt.Println(err)
		return
	}
	pods,err := podsgetter.GetPodsByNamespaces(client, "default")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(pods)
}
