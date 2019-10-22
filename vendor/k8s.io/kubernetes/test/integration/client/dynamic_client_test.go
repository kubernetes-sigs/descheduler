/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	kubeapiservertesting "k8s.io/kubernetes/cmd/kube-apiserver/app/testing"
	"k8s.io/kubernetes/pkg/api/testapi"
	"k8s.io/kubernetes/test/integration/framework"
)

func TestDynamicClient(t *testing.T) {
	result := kubeapiservertesting.StartTestServerOrDie(t, nil, []string{"--disable-admission-plugins", "ServiceAccount"}, framework.SharedEtcd())
	defer result.TearDownFn()

	client := clientset.NewForConfigOrDie(result.ClientConfig)
	dynamicClient, err := dynamic.NewForConfig(result.ClientConfig)
	if err != nil {
		t.Fatalf("unexpected error creating dynamic client: %v", err)
	}

	resource := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

	// Create a Pod with the normal client
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "test",
					Image: "test-image",
				},
			},
		},
	}

	actual, err := client.CoreV1().Pods("default").Create(pod)
	if err != nil {
		t.Fatalf("unexpected error when creating pod: %v", err)
	}

	// check dynamic list
	unstructuredList, err := dynamicClient.Resource(resource).Namespace("default").List(metav1.ListOptions{})
	if err != nil {
		t.Fatalf("unexpected error when listing pods: %v", err)
	}

	if len(unstructuredList.Items) != 1 {
		t.Fatalf("expected one pod, got %d", len(unstructuredList.Items))
	}

	got, err := unstructuredToPod(&unstructuredList.Items[0])
	if err != nil {
		t.Fatalf("unexpected error converting Unstructured to v1.Pod: %v", err)
	}

	if !reflect.DeepEqual(actual, got) {
		t.Fatalf("unexpected pod in list. wanted %#v, got %#v", actual, got)
	}

	// check dynamic get
	unstruct, err := dynamicClient.Resource(resource).Namespace("default").Get(actual.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("unexpected error when getting pod %q: %v", actual.Name, err)
	}

	got, err = unstructuredToPod(unstruct)
	if err != nil {
		t.Fatalf("unexpected error converting Unstructured to v1.Pod: %v", err)
	}

	if !reflect.DeepEqual(actual, got) {
		t.Fatalf("unexpected pod in list. wanted %#v, got %#v", actual, got)
	}

	// delete the pod dynamically
	err = dynamicClient.Resource(resource).Namespace("default").Delete(actual.Name, nil)
	if err != nil {
		t.Fatalf("unexpected error when deleting pod: %v", err)
	}

	list, err := client.CoreV1().Pods("default").List(metav1.ListOptions{})
	if err != nil {
		t.Fatalf("unexpected error when listing pods: %v", err)
	}

	if len(list.Items) != 0 {
		t.Fatalf("expected zero pods, got %d", len(list.Items))
	}
}

func TestDynamicClientWatch(t *testing.T) {
	result := kubeapiservertesting.StartTestServerOrDie(t, nil, nil, framework.SharedEtcd())
	defer result.TearDownFn()

	client := clientset.NewForConfigOrDie(result.ClientConfig)
	dynamicClient, err := dynamic.NewForConfig(result.ClientConfig)
	if err != nil {
		t.Fatalf("unexpected error creating dynamic client: %v", err)
	}

	resource := v1.SchemeGroupVersion.WithResource("events")

	mkEvent := func(i int) *v1.Event {
		name := fmt.Sprintf("event-%v", i)
		return &v1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      name,
			},
			InvolvedObject: v1.ObjectReference{
				Namespace: "default",
				Name:      name,
			},
			Reason: fmt.Sprintf("event %v", i),
		}
	}

	rv1 := ""
	for i := 0; i < 10; i++ {
		event := mkEvent(i)
		got, err := client.CoreV1().Events("default").Create(event)
		if err != nil {
			t.Fatalf("Failed creating event %#q: %v", event, err)
		}
		if rv1 == "" {
			rv1 = got.ResourceVersion
			if rv1 == "" {
				t.Fatal("did not get a resource version.")
			}
		}
		t.Logf("Created event %#v", got.ObjectMeta)
	}

	w, err := dynamicClient.Resource(resource).Namespace("default").Watch(metav1.ListOptions{
		ResourceVersion: rv1,
		Watch:           true,
		FieldSelector:   fields.OneTermEqualSelector("metadata.name", "event-9").String(),
	})

	if err != nil {
		t.Fatalf("Failed watch: %v", err)
	}
	defer w.Stop()

	select {
	case <-time.After(wait.ForeverTestTimeout):
		t.Fatalf("watch took longer than %s", wait.ForeverTestTimeout.String())
	case got, ok := <-w.ResultChan():
		if !ok {
			t.Fatal("Watch channel closed unexpectedly.")
		}

		// We expect to see an ADD of event-9 and only event-9. (This
		// catches a bug where all the events would have been sent down
		// the channel.)
		if e, a := watch.Added, got.Type; e != a {
			t.Errorf("Wanted %v, got %v", e, a)
		}

		unstructured, ok := got.Object.(*unstructured.Unstructured)
		if !ok {
			t.Fatalf("Unexpected watch event containing object %#q", got.Object)
		}
		event, err := unstructuredToEvent(unstructured)
		if err != nil {
			t.Fatalf("unexpected error converting Unstructured to v1.Event: %v", err)
		}
		if e, a := "event-9", event.Name; e != a {
			t.Errorf("Wanted %v, got %v", e, a)
		}
	}
}

func unstructuredToPod(obj *unstructured.Unstructured) (*v1.Pod, error) {
	json, err := runtime.Encode(unstructured.UnstructuredJSONScheme, obj)
	if err != nil {
		return nil, err
	}
	pod := new(v1.Pod)
	err = runtime.DecodeInto(testapi.Default.Codec(), json, pod)
	pod.Kind = ""
	pod.APIVersion = ""
	return pod, err
}

func unstructuredToEvent(obj *unstructured.Unstructured) (*v1.Event, error) {
	json, err := runtime.Encode(unstructured.UnstructuredJSONScheme, obj)
	if err != nil {
		return nil, err
	}
	event := new(v1.Event)
	err = runtime.DecodeInto(testapi.Default.Codec(), json, event)
	return event, err
}
