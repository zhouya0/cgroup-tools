package podsgetter

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/informers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

func GetNginxFromLister(cs clientset.Interface) {
	stop := make(chan struct{})
	defer close(stop)
	sharedInformers := informers.NewSharedInformerFactory(cs, 0)
	sharedInformers.Start(stop)
	podInformer := sharedInformers.Core().V1().Pods()
	go podInformer.Informer().Run(stop)
	podLister := podInformer.Lister()
	podListerSynced := podInformer.Informer().HasSynced

	if !cache.WaitForCacheSync(stop, podListerSynced) {
		return
	}

	nginx, err := podLister.Pods("default").Get("nginx")
	if err != nil {
		return
	}
	fmt.Println("Old labels\n", nginx.Labels)

	labels := make(map[string]string)
	labels["test"] = "nginxtest"
	nginx.Labels = labels

	newNginx, err := podLister.Pods("default").Get("nginx")
	if err != nil {
		return
	}
	fmt.Println("New labels\n", newNginx.Labels)
}

func GetPodsByNamespaces(cs clientset.Interface, namespace string) (*corev1.PodList,error) {
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


func NewLogsOptions(container string, follow bool) *corev1.PodLogOptions {
	return 	&corev1.PodLogOptions{
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