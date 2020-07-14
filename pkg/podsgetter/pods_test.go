package podsgetter

import (
	"fmt"
	"k8s.io/kubectl/pkg/scheme"
	"net/http"
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	restfake "k8s.io/client-go/rest/fake"
	core "k8s.io/client-go/testing"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
)

func genPodList() *v1.PodList{
	obj := &v1.PodList{}
	podNamePrefix := "mypod"
	namespace := "mynamespace"
	for i := 0; i < 5; i ++ {
		podName := fmt.Sprintf("%s-%d", podNamePrefix, i)
		pod := v1.Pod{
			Status: v1.PodStatus{
				Phase: v1.PodRunning,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: podName,
				UID: types.UID(podName),
				Namespace: namespace,
				Labels: map[string]string{
					"name": podName,
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name: "containerName",
						Image: "containerImage",
						VolumeMounts: []v1.VolumeMount{
							{
								Name: "volumeMountName",
								ReadOnly: false,
								MountPath: "/mnt",
							},
						},
					},
				},
				Volumes: []v1.Volume{
					{
						Name: "volumeName",
						VolumeSource: v1.VolumeSource{
							GCEPersistentDisk: &v1.GCEPersistentDiskVolumeSource{
								PDName: "pdName",
								FSType: "ext4",
								ReadOnly: false,
							},
						},
					},
				},
			},
		}
		obj.Items = append(obj.Items, pod)

	}
	return obj
}

func TestGetPodsByNamespacesBasic(t *testing.T) {
	tf := cmdtesting.NewTestFactory().WithNamespace("default")
	defer tf.Cleanup()

	codec := scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)
	ns := scheme.Codecs.WithoutConversion()

	fakeRestClient := &restfake.RESTClient{
		NegotiatedSerializer: ns,
		Client: restfake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch p,m := req.URL.Path, req.Method; {
			case p == "/api/v1/namespaces/default/pods" && m == "GET":
				return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: cmdtesting.ObjBody(codec, genPodList())}, nil
			default:
				t.Fatalf("unexpected request: %#v\n%#v", req.URL, req)
				return nil, nil
			}
		}),
	}
	tf.Client = fakeRestClient
	fakeClientSet,err := tf.KubernetesClientSet()
	pods,err := GetPodsByNamespaces(fakeClientSet, "default")
	if err != nil {
		t.Fatalf("Error happens: %v", err)
	}
	fmt.Println(pods)

}

// No real send request action.
func TestGetPodsByNamespaces(t *testing.T) {
	fakeClient := &fake.Clientset{}
	fakeClient.AddReactor("list", "pods", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, genPodList(), nil
	})
	// It won't care about namespace
	pods,err := GetPodsByNamespaces(fakeClient, "default")
	if err != nil {
		t.Fatalf("Error happens: %v", err)
	}
	fmt.Println(pods)
}


// Panic with k1.18, related issues:https://github.com/kubernetes/kubernetes/issues/84203
//func TestWritePodLogs(t *testing.T) {
//	fakeClient := &fake.Clientset{}
//	err := WritePodLogs(fakeClient, "default", "test", "test", os.Stdout)
//	if err != nil {
//		t.Fatalf("Error happens:%v", err)
//	}
//}