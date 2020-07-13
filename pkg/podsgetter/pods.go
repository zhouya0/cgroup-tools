package podsgetter

import (
	"context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

func GetPodsByNamespaces(cs clientset.Interface, namespace string) (*v1.PodList,error) {
	listOptions := metav1.ListOptions{}
	pods,err := cs.CoreV1().Pods(namespace).List(context.TODO(), listOptions)
	if err != nil {
		return nil, err
	}
	return pods, nil
}
