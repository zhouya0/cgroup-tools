package client

import (
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
)

var kubeConfigPath = "/root/.kube/config"

// BuildInClusterClientSet creates an in-cluster ClientSet
func BuildInClusterClientset() (*kubernetes.Clientset, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		klog.Errorf("Failed to build cluster config: %v", err)
		return nil, err
	}

	return kubernetes.NewForConfig(cfg)
}

func BuildLocalClientSet() (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("",kubeConfigPath)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil

}