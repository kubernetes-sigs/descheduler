/*
Copyright 2019 The Kubernetes Authors.

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

package events

import (
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	typedv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/record"
	ref "k8s.io/client-go/tools/reference"
	kubeapiservertesting "k8s.io/kubernetes/cmd/kube-apiserver/app/testing"
	"k8s.io/kubernetes/test/integration/framework"
)

func TestEventCompatibility(t *testing.T) {
	result := kubeapiservertesting.StartTestServerOrDie(t, nil, []string{"--disable-admission-plugins", "ServiceAccount"}, framework.SharedEtcd())
	defer result.TearDownFn()

	client := clientset.NewForConfigOrDie(result.ClientConfig)

	testPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			SelfLink:  "/api/v1/namespaces/default/pods/foo",
			Name:      "foo",
			Namespace: "default",
			UID:       "bar",
		},
	}

	regarding, err := ref.GetReference(scheme.Scheme, testPod)
	if err != nil {
		t.Fatal(err)
	}

	related, err := ref.GetPartialReference(scheme.Scheme, testPod, ".spec.containers[0]")
	if err != nil {
		t.Fatal(err)
	}
	stopCh := make(chan struct{})
	oldBroadcaster := record.NewBroadcaster()
	oldRecorder := oldBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "integration"})
	oldBroadcaster.StartRecordingToSink(&typedv1.EventSinkImpl{Interface: client.CoreV1().Events("")})
	oldRecorder.Eventf(regarding, v1.EventTypeNormal, "started", "note")

	newBroadcaster := events.NewBroadcaster(&events.EventSinkImpl{Interface: client.EventsV1beta1().Events("")})
	newRecorder := newBroadcaster.NewRecorder(scheme.Scheme, "k8s.io/kube-scheduler")
	newBroadcaster.StartRecordingToSink(stopCh)
	newRecorder.Eventf(regarding, related, v1.EventTypeNormal, "memoryPressure", "killed", "memory pressure")
	err = wait.PollImmediate(100*time.Millisecond, 20*time.Second, func() (done bool, err error) {
		v1beta1Events, err := client.EventsV1beta1().Events("").List(metav1.ListOptions{})
		if err != nil {
			return false, err
		}

		if len(v1beta1Events.Items) != 2 {
			return false, nil
		}

		events, err := client.CoreV1().Events("").List(metav1.ListOptions{})
		if err != nil {
			return false, err
		}

		if len(events.Items) != 2 {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
