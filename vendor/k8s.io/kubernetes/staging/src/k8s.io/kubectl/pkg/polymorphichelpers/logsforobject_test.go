/*
Copyright 2017 The Kubernetes Authors.

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

package polymorphichelpers

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	fakeexternal "k8s.io/client-go/kubernetes/fake"
	testclient "k8s.io/client-go/testing"
)

var (
	podsResource = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	podsKind     = schema.GroupVersionKind{Version: "v1", Kind: "Pod"}
)

func TestLogsForObject(t *testing.T) {
	tests := []struct {
		name          string
		obj           runtime.Object
		opts          *corev1.PodLogOptions
		allContainers bool
		clientsetPods []runtime.Object
		actions       []testclient.Action

		expectedErr     string
		expectedSources []corev1.ObjectReference
	}{
		{
			name: "pod logs",
			obj:  testPodWithOneContainers(),
			actions: []testclient.Action{
				getLogsAction("test", nil),
			},
			expectedSources: []corev1.ObjectReference{
				{
					Kind:       testPodWithOneContainers().Kind,
					APIVersion: testPodWithOneContainers().APIVersion,
					Name:       testPodWithOneContainers().Name,
					Namespace:  testPodWithOneContainers().Namespace,
					FieldPath:  fmt.Sprintf("spec.containers{%s}", testPodWithOneContainers().Spec.Containers[0].Name),
				},
			},
		},
		{
			name:          "pod logs: all containers",
			obj:           testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers(),
			opts:          &corev1.PodLogOptions{},
			allContainers: true,
			actions: []testclient.Action{
				getLogsAction("test", &corev1.PodLogOptions{Container: "foo-2-and-2-and-1-initc1"}),
				getLogsAction("test", &corev1.PodLogOptions{Container: "foo-2-and-2-and-1-initc2"}),
				getLogsAction("test", &corev1.PodLogOptions{Container: "foo-2-and-2-and-1-c1"}),
				getLogsAction("test", &corev1.PodLogOptions{Container: "foo-2-and-2-and-1-c2"}),
				getLogsAction("test", &corev1.PodLogOptions{Container: "foo-2-and-2-and-1-e1"}),
			},
			expectedSources: []corev1.ObjectReference{
				{
					Kind:       testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Kind,
					APIVersion: testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().APIVersion,
					Name:       testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Name,
					Namespace:  testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Namespace,
					FieldPath:  fmt.Sprintf("spec.initContainers{%s}", testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Spec.InitContainers[0].Name),
				},
				{
					Kind:       testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Kind,
					APIVersion: testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().APIVersion,
					Name:       testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Name,
					Namespace:  testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Namespace,
					FieldPath:  fmt.Sprintf("spec.initContainers{%s}", testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Spec.InitContainers[1].Name),
				},
				{
					Kind:       testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Kind,
					APIVersion: testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().APIVersion,
					Name:       testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Name,
					Namespace:  testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Namespace,
					FieldPath:  fmt.Sprintf("spec.containers{%s}", testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Spec.Containers[0].Name),
				},
				{
					Kind:       testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Kind,
					APIVersion: testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().APIVersion,
					Name:       testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Name,
					Namespace:  testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Namespace,
					FieldPath:  fmt.Sprintf("spec.containers{%s}", testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Spec.Containers[1].Name),
				},
				{
					Kind:       testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Kind,
					APIVersion: testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().APIVersion,
					Name:       testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Name,
					Namespace:  testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Namespace,
					FieldPath:  fmt.Sprintf("spec.ephemeralContainers{%s}", testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers().Spec.EphemeralContainers[0].Name),
				},
			},
		},
		{
			name:        "pod logs: error - must provide container name",
			obj:         testPodWithTwoContainers(),
			expectedErr: "a container name must be specified for pod foo-two-containers, choose one of: [foo-2-c1 foo-2-c2]",
		},
		{
			name: "pods list logs",
			obj: &corev1.PodList{
				Items: []corev1.Pod{*testPodWithOneContainers()},
			},
			actions: []testclient.Action{
				getLogsAction("test", nil),
			},
			expectedSources: []corev1.ObjectReference{{
				Kind:       testPodWithOneContainers().Kind,
				APIVersion: testPodWithOneContainers().APIVersion,
				Name:       testPodWithOneContainers().Name,
				Namespace:  testPodWithOneContainers().Namespace,
				FieldPath:  fmt.Sprintf("spec.containers{%s}", testPodWithOneContainers().Spec.Containers[0].Name),
			}},
		},
		{
			name: "pods list logs: all containers",
			obj: &corev1.PodList{
				Items: []corev1.Pod{*testPodWithTwoContainersAndTwoInitContainers()},
			},
			opts:          &corev1.PodLogOptions{},
			allContainers: true,
			actions: []testclient.Action{
				getLogsAction("test", &corev1.PodLogOptions{Container: "foo-2-and-2-initc1"}),
				getLogsAction("test", &corev1.PodLogOptions{Container: "foo-2-and-2-initc2"}),
				getLogsAction("test", &corev1.PodLogOptions{Container: "foo-2-and-2-c1"}),
				getLogsAction("test", &corev1.PodLogOptions{Container: "foo-2-and-2-c2"}),
			},
			expectedSources: []corev1.ObjectReference{
				{
					Kind:       testPodWithTwoContainersAndTwoInitContainers().Kind,
					APIVersion: testPodWithTwoContainersAndTwoInitContainers().APIVersion,
					Name:       testPodWithTwoContainersAndTwoInitContainers().Name,
					Namespace:  testPodWithTwoContainersAndTwoInitContainers().Namespace,
					FieldPath:  fmt.Sprintf("spec.initContainers{%s}", testPodWithTwoContainersAndTwoInitContainers().Spec.InitContainers[0].Name),
				},
				{
					Kind:       testPodWithTwoContainersAndTwoInitContainers().Kind,
					APIVersion: testPodWithTwoContainersAndTwoInitContainers().APIVersion,
					Name:       testPodWithTwoContainersAndTwoInitContainers().Name,
					Namespace:  testPodWithTwoContainersAndTwoInitContainers().Namespace,
					FieldPath:  fmt.Sprintf("spec.initContainers{%s}", testPodWithTwoContainersAndTwoInitContainers().Spec.InitContainers[1].Name),
				},
				{
					Kind:       testPodWithTwoContainersAndTwoInitContainers().Kind,
					APIVersion: testPodWithTwoContainersAndTwoInitContainers().APIVersion,
					Name:       testPodWithTwoContainersAndTwoInitContainers().Name,
					Namespace:  testPodWithTwoContainersAndTwoInitContainers().Namespace,
					FieldPath:  fmt.Sprintf("spec.containers{%s}", testPodWithTwoContainersAndTwoInitContainers().Spec.Containers[0].Name),
				},
				{
					Kind:       testPodWithTwoContainersAndTwoInitContainers().Kind,
					APIVersion: testPodWithTwoContainersAndTwoInitContainers().APIVersion,
					Name:       testPodWithTwoContainersAndTwoInitContainers().Name,
					Namespace:  testPodWithTwoContainersAndTwoInitContainers().Namespace,
					FieldPath:  fmt.Sprintf("spec.containers{%s}", testPodWithTwoContainersAndTwoInitContainers().Spec.Containers[1].Name),
				},
			},
		},
		{
			name: "pods list logs: error - must provide container name",
			obj: &corev1.PodList{
				Items: []corev1.Pod{*testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers()},
			},
			expectedErr: "a container name must be specified for pod foo-two-containers-and-two-init-containers, choose one of: [foo-2-and-2-and-1-c1 foo-2-and-2-and-1-c2] or one of the init containers: [foo-2-and-2-and-1-initc1 foo-2-and-2-and-1-initc2] or one of the ephemeral containers: [foo-2-and-2-and-1-e1]",
		},
		{
			name: "replication controller logs",
			obj: &corev1.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "test"},
				Spec: corev1.ReplicationControllerSpec{
					Selector: map[string]string{"foo": "bar"},
				},
			},
			clientsetPods: []runtime.Object{testPodWithOneContainers()},
			actions: []testclient.Action{
				testclient.NewListAction(podsResource, podsKind, "test", metav1.ListOptions{LabelSelector: "foo=bar"}),
				getLogsAction("test", nil),
			},
			expectedSources: []corev1.ObjectReference{{
				Kind:       testPodWithOneContainers().Kind,
				APIVersion: testPodWithOneContainers().APIVersion,
				Name:       testPodWithOneContainers().Name,
				Namespace:  testPodWithOneContainers().Namespace,
				FieldPath:  fmt.Sprintf("spec.containers{%s}", testPodWithOneContainers().Spec.Containers[0].Name),
			}},
		},
		{
			name: "replica set logs",
			obj: &extensionsv1beta1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "test"},
				Spec: extensionsv1beta1.ReplicaSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
			},
			clientsetPods: []runtime.Object{testPodWithOneContainers()},
			actions: []testclient.Action{
				testclient.NewListAction(podsResource, podsKind, "test", metav1.ListOptions{LabelSelector: "foo=bar"}),
				getLogsAction("test", nil),
			},
			expectedSources: []corev1.ObjectReference{{
				Kind:       testPodWithOneContainers().Kind,
				APIVersion: testPodWithOneContainers().APIVersion,
				Name:       testPodWithOneContainers().Name,
				Namespace:  testPodWithOneContainers().Namespace,
				FieldPath:  fmt.Sprintf("spec.containers{%s}", testPodWithOneContainers().Spec.Containers[0].Name),
			}},
		},
		{
			name: "deployment logs",
			obj: &extensionsv1beta1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "test"},
				Spec: extensionsv1beta1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
			},
			clientsetPods: []runtime.Object{testPodWithOneContainers()},
			actions: []testclient.Action{
				testclient.NewListAction(podsResource, podsKind, "test", metav1.ListOptions{LabelSelector: "foo=bar"}),
				getLogsAction("test", nil),
			},
			expectedSources: []corev1.ObjectReference{{
				Kind:       testPodWithOneContainers().Kind,
				APIVersion: testPodWithOneContainers().APIVersion,
				Name:       testPodWithOneContainers().Name,
				Namespace:  testPodWithOneContainers().Namespace,
				FieldPath:  fmt.Sprintf("spec.containers{%s}", testPodWithOneContainers().Spec.Containers[0].Name),
			}},
		},
		{
			name: "job logs",
			obj: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "test"},
				Spec: batchv1.JobSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
			},
			clientsetPods: []runtime.Object{testPodWithOneContainers()},
			actions: []testclient.Action{
				testclient.NewListAction(podsResource, podsKind, "test", metav1.ListOptions{LabelSelector: "foo=bar"}),
				getLogsAction("test", nil),
			},
			expectedSources: []corev1.ObjectReference{{
				Kind:       testPodWithOneContainers().Kind,
				APIVersion: testPodWithOneContainers().APIVersion,
				Name:       testPodWithOneContainers().Name,
				Namespace:  testPodWithOneContainers().Namespace,
				FieldPath:  fmt.Sprintf("spec.containers{%s}", testPodWithOneContainers().Spec.Containers[0].Name),
			}},
		},
		{
			name: "stateful set logs",
			obj: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "test"},
				Spec: appsv1.StatefulSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
			},
			clientsetPods: []runtime.Object{testPodWithOneContainers()},
			actions: []testclient.Action{
				testclient.NewListAction(podsResource, podsKind, "test", metav1.ListOptions{LabelSelector: "foo=bar"}),
				getLogsAction("test", nil),
			},
			expectedSources: []corev1.ObjectReference{{
				Kind:       testPodWithOneContainers().Kind,
				APIVersion: testPodWithOneContainers().APIVersion,
				Name:       testPodWithOneContainers().Name,
				Namespace:  testPodWithOneContainers().Namespace,
				FieldPath:  fmt.Sprintf("spec.containers{%s}", testPodWithOneContainers().Spec.Containers[0].Name),
			}},
		},
	}

	for _, test := range tests {
		fakeClientset := fakeexternal.NewSimpleClientset(test.clientsetPods...)
		responses, err := logsForObjectWithClient(fakeClientset.CoreV1(), test.obj, test.opts, 20*time.Second, test.allContainers)
		if test.expectedErr == "" && err != nil {
			t.Errorf("%s: unexpected error: %v", test.name, err)
			continue
		}

		if err != nil && test.expectedErr != err.Error() {
			t.Errorf("%s: expected error: %v, got: %v", test.name, test.expectedErr, err)
			continue
		}

		if len(test.expectedSources) != len(responses) {
			t.Errorf(
				"%s: the number of expected sources doesn't match the number of responses: %v, got: %v",
				test.name,
				len(test.expectedSources),
				len(responses),
			)
			continue
		}

		for _, ref := range test.expectedSources {
			if _, ok := responses[ref]; !ok {
				t.Errorf("%s: didn't find expected log source object reference: %#v", test.name, ref)
			}
		}

		var i int
		for i = range test.actions {
			if len(fakeClientset.Actions()) < i {
				t.Errorf("%s: expected action %d does not exists in actual actions: %#v",
					test.name, i, fakeClientset.Actions())
				continue
			}
			got := fakeClientset.Actions()[i]
			want := test.actions[i]
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s: unexpected diff for action: %s", test.name, diff.ObjectDiff(got, want))
			}
		}
		for i++; i < len(fakeClientset.Actions()); i++ {
			t.Errorf("%s: actual action %d does not exist in expected: %v", test.name, i, fakeClientset.Actions()[i])
		}
	}
}

func testPodWithOneContainers() *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
			Labels:    map[string]string{"foo": "bar"},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyAlways,
			DNSPolicy:     corev1.DNSClusterFirst,
			Containers: []corev1.Container{
				{Name: "c1"},
			},
		},
	}
}

func testPodWithTwoContainers() *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-two-containers",
			Namespace: "test",
			Labels:    map[string]string{"foo": "bar"},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyAlways,
			DNSPolicy:     corev1.DNSClusterFirst,
			Containers: []corev1.Container{
				{Name: "foo-2-c1"},
				{Name: "foo-2-c2"},
			},
		},
	}
}

func testPodWithTwoContainersAndTwoInitContainers() *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-two-containers-and-two-init-containers",
			Namespace: "test",
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: "foo-2-and-2-initc1"},
				{Name: "foo-2-and-2-initc2"},
			},
			Containers: []corev1.Container{
				{Name: "foo-2-and-2-c1"},
				{Name: "foo-2-and-2-c2"},
			},
		},
	}
}

func testPodWithTwoContainersAndTwoInitAndOneEphemeralContainers() *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-two-containers-and-two-init-containers",
			Namespace: "test",
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: "foo-2-and-2-and-1-initc1"},
				{Name: "foo-2-and-2-and-1-initc2"},
			},
			Containers: []corev1.Container{
				{Name: "foo-2-and-2-and-1-c1"},
				{Name: "foo-2-and-2-and-1-c2"},
			},
			EphemeralContainers: []corev1.EphemeralContainer{
				{
					EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "foo-2-and-2-and-1-e1"},
				},
			},
		},
	}
}

func getLogsAction(namespace string, opts *corev1.PodLogOptions) testclient.Action {
	action := testclient.GenericActionImpl{}
	action.Verb = "get"
	action.Namespace = namespace
	action.Resource = podsResource
	action.Subresource = "log"
	action.Value = opts
	return action
}
