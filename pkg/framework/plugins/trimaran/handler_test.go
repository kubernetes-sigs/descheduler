/*
Copyright 2023 The Kubernetes Authors.
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

package trimaran

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/descheduler/test"
)

func TestHandlerCacheCleanup(t *testing.T) {
	testNode := "node-1"
	pod1 := test.BuildTestPod("Pod-1", 0, 0, "", test.SetRSOwnerRef)
	pod2 := test.BuildTestPod("Pod-2", 0, 0, "", test.SetRSOwnerRef)
	pod3 := test.BuildTestPod("Pod-3", 0, 0, "", test.SetRSOwnerRef)
	pod4 := test.BuildTestPod("Pod-4", 0, 0, "", test.SetRSOwnerRef)

	tests := []struct {
		name              string
		podInfoList       []*PodInfo
		podToUpdate       string
		expectedCacheSize int
		expectedCachePods []string
	}{
		{
			name: "OnUpdate doesn't add unassigned pods",
			podInfoList: []*PodInfo{
				{Pod: pod1},
				{Pod: pod2},
				{Pod: pod3},
			},
			podToUpdate:       "Pod-4",
			expectedCacheSize: 1,
			expectedCachePods: []string{"Pod-4"},
		},
		{
			name: "cleanupCache doesn't delete newly added pods",
			podInfoList: []*PodInfo{
				{Pod: pod1},
				{Pod: pod2},
				{Pod: pod3},
				{Timestamp: time.Now(), Pod: pod4},
			},
			podToUpdate:       "Pod-5",
			expectedCacheSize: 2,
			expectedCachePods: []string{"Pod-4", "Pod-5"},
		},
		{
			name: "cleanupCache deletes old pods",
			podInfoList: []*PodInfo{
				{Timestamp: time.Now().Add(-5 * time.Minute), Pod: pod1},
				{Timestamp: time.Now().Add(-10 * time.Second), Pod: pod2},
				{Timestamp: time.Now().Add(-5 * time.Second), Pod: pod3},
			},
			expectedCacheSize: 2,
			expectedCachePods: []string{pod2.Name, pod3.Name},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(nil)
			impl := handler.(*podAssignEventHandler)
			impl.scheduledPodsCache[testNode] = append(impl.scheduledPodsCache[testNode], tt.podInfoList...)
			if tt.podToUpdate != "" {
				oldPod := test.BuildTestPod(tt.podToUpdate, 0, 0, "", test.SetRSOwnerRef)
				newPod := test.BuildTestPod(tt.podToUpdate, 0, 0, "", test.SetRSOwnerRef)
				newPod.Spec.NodeName = testNode
				impl.OnUpdate(oldPod, newPod)
			}
			impl.cleanupCache()
			if diff := cmp.Diff(len(impl.scheduledPodsCache[testNode]), tt.expectedCacheSize); len(diff) > 0 {
				t.Errorf("HandlerCacheCleanup() failed, mismatch:\n%s", diff)
			}
			gotCachePods := []string{}
			for _, podInfo := range impl.scheduledPodsCache[testNode] {
				gotCachePods = append(gotCachePods, podInfo.Pod.Name)
			}
			if diff := cmp.Diff(gotCachePods, tt.expectedCachePods); len(diff) > 0 {
				t.Errorf("HandlerCacheCleanup() failed, mismatch:\n%s", diff)
			}
		})
	}
}
