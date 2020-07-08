package main

import (
	"context"
	"fmt"

	"github.com/zhouya0/cgroup-tools/pkg/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	testString := "env in ( de, ds )"

	// kubeClient, _ := client.BuildInClusterClientset()
	kubeClient,_ := client.BuildLocalClientSet()
	pods,_ := kubeClient.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{LabelSelector: testString})
	fmt.Println("Listing pods:")
	fmt.Println(pods)
}
