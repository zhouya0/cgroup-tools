package client

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
)

// BuildInClusterClientSet creates an in-cluster ClientSet
func BuildInClusterClientset() (*kubernetes.Clientset, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		klog.Errorf("Failed to build cluster config: %v", err)
		return nil, err
	}

	return kubernetes.NewForConfig(cfg)
}