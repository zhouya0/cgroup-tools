package podsgetter

import (
	"bufio"
	"context"
	"io"

	"k8s.io/client-go/rest"
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

func WritePodLogs(cs clientset.Interface, namespace string, podName string, containerName string, out io.Writer) error {
	logOptions := NewLogsOptions(containerName, false)
	podLogRequest := cs.CoreV1().Pods(namespace).GetLogs(podName, logOptions)
	err := DefaultConsumeRequest(podLogRequest, out)
	if err != nil {
		return err
	}
	return nil
}


func NewLogsOptions(container string, follow bool) *v1.PodLogOptions {
	return 	&v1.PodLogOptions{
		Container:                    container,
		Follow:                       follow,
	}
}

func DefaultConsumeRequest(request rest.ResponseWrapper, out io.Writer) error {
	readCloser, err := request.Stream(context.TODO())
	if err != nil {
		return err
	}
	defer readCloser.Close()

	r := bufio.NewReader(readCloser)
	for {
		bytes, err := r.ReadBytes('\n')
		if _, err := out.Write(bytes); err != nil {
			return err
		}

		if err != nil {
			if err != io.EOF {
				return err
			}
			return nil
		}
	}
}