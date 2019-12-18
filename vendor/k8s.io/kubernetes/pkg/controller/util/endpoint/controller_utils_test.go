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

package endpoint

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDetermineNeededServiceUpdates(t *testing.T) {
	testCases := []struct {
		name  string
		a     sets.String
		b     sets.String
		union sets.String
		xor   sets.String
	}{
		{
			name:  "no services changed",
			a:     sets.NewString("a", "b", "c"),
			b:     sets.NewString("a", "b", "c"),
			xor:   sets.NewString(),
			union: sets.NewString("a", "b", "c"),
		},
		{
			name:  "all old services removed, new services added",
			a:     sets.NewString("a", "b", "c"),
			b:     sets.NewString("d", "e", "f"),
			xor:   sets.NewString("a", "b", "c", "d", "e", "f"),
			union: sets.NewString("a", "b", "c", "d", "e", "f"),
		},
		{
			name:  "all old services removed, no new services added",
			a:     sets.NewString("a", "b", "c"),
			b:     sets.NewString(),
			xor:   sets.NewString("a", "b", "c"),
			union: sets.NewString("a", "b", "c"),
		},
		{
			name:  "no old services, but new services added",
			a:     sets.NewString(),
			b:     sets.NewString("a", "b", "c"),
			xor:   sets.NewString("a", "b", "c"),
			union: sets.NewString("a", "b", "c"),
		},
		{
			name:  "one service removed, one service added, two unchanged",
			a:     sets.NewString("a", "b", "c"),
			b:     sets.NewString("b", "c", "d"),
			xor:   sets.NewString("a", "d"),
			union: sets.NewString("a", "b", "c", "d"),
		},
		{
			name:  "no services",
			a:     sets.NewString(),
			b:     sets.NewString(),
			xor:   sets.NewString(),
			union: sets.NewString(),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			retval := determineNeededServiceUpdates(testCase.a, testCase.b, false)
			if !retval.Equal(testCase.xor) {
				t.Errorf("%s (with podChanged=false): expected: %v  got: %v", testCase.name, testCase.xor.List(), retval.List())
			}

			retval = determineNeededServiceUpdates(testCase.a, testCase.b, true)
			if !retval.Equal(testCase.union) {
				t.Errorf("%s (with podChanged=true): expected: %v  got: %v", testCase.name, testCase.union.List(), retval.List())
			}
		})
	}
}

// There are 3*5 possibilities(3 types of RestartPolicy by 5 types of PodPhase).
// Not listing them all here. Just listing all of the 3 false cases and 3 of the
// 12 true cases.
func TestShouldPodBeInEndpoints(t *testing.T) {
	testCases := []struct {
		name     string
		pod      *v1.Pod
		expected bool
	}{
		// Pod should not be in endpoints:
		{
			name: "Failed pod with Never RestartPolicy",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyNever,
				},
				Status: v1.PodStatus{
					Phase: v1.PodFailed,
					PodIP: "1.2.3.4",
				},
			},
			expected: false,
		},
		{
			name: "Succeeded pod with Never RestartPolicy",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyNever,
				},
				Status: v1.PodStatus{
					Phase: v1.PodSucceeded,
					PodIP: "1.2.3.4",
				},
			},
			expected: false,
		},
		{
			name: "Succeeded pod with OnFailure RestartPolicy",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyOnFailure,
				},
				Status: v1.PodStatus{
					Phase: v1.PodSucceeded,
					PodIP: "1.2.3.4",
				},
			},
			expected: false,
		},
		{
			name: "Empty Pod IPs, Running pod with OnFailure RestartPolicy",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyNever,
				},
				Status: v1.PodStatus{
					Phase:  v1.PodRunning,
					PodIP:  "",
					PodIPs: []v1.PodIP{},
				},
			},
			expected: false,
		},
		// Pod should be in endpoints:
		{
			name: "Failed pod with Always RestartPolicy",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyAlways,
				},
				Status: v1.PodStatus{
					Phase: v1.PodFailed,
					PodIP: "1.2.3.4",
				},
			},
			expected: true,
		},
		{
			name: "Pending pod with Never RestartPolicy",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyNever,
				},
				Status: v1.PodStatus{
					Phase: v1.PodPending,
					PodIP: "1.2.3.4",
				},
			},
			expected: true,
		},
		{
			name: "Unknown pod with OnFailure RestartPolicy",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyOnFailure,
				},
				Status: v1.PodStatus{
					Phase: v1.PodUnknown,
					PodIP: "1.2.3.4",
				},
			},
			expected: true,
		},
		{
			name: "Running pod with Never RestartPolicy",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyNever,
				},
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
					PodIP: "1.2.3.4",
				},
			},
			expected: true,
		},
		{
			name: "Multiple Pod IPs, Running pod with OnFailure RestartPolicy",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyNever,
				},
				Status: v1.PodStatus{
					Phase:  v1.PodRunning,
					PodIPs: []v1.PodIP{{IP: "1.2.3.4"}, {IP: "1234::5678:0000:0000:9abc:def0"}},
				},
			},
			expected: true,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := ShouldPodBeInEndpoints(test.pod)
			if result != test.expected {
				t.Errorf("expected: %t, got: %t", test.expected, result)
			}
		})
	}
}

func TestShouldSetHostname(t *testing.T) {
	testCases := map[string]struct {
		pod      *v1.Pod
		service  *v1.Service
		expected bool
	}{
		"all matching": {
			pod:      genSimplePod("ns", "foo", "svc-name"),
			service:  genSimpleSvc("ns", "svc-name"),
			expected: true,
		},
		"all matching, hostname not set": {
			pod:      genSimplePod("ns", "", "svc-name"),
			service:  genSimpleSvc("ns", "svc-name"),
			expected: false,
		},
		"all set, different name/subdomain": {
			pod:      genSimplePod("ns", "hostname", "subdomain"),
			service:  genSimpleSvc("ns", "name"),
			expected: false,
		},
		"all set, different namespace": {
			pod:      genSimplePod("ns1", "hostname", "svc-name"),
			service:  genSimpleSvc("ns2", "svc-name"),
			expected: false,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			result := ShouldSetHostname(testCase.pod, testCase.service)
			if result != testCase.expected {
				t.Errorf("expected: %t, got: %t", testCase.expected, result)
			}
		})
	}
}

func genSimplePod(namespace, hostname, subdomain string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: v1.PodSpec{
			Hostname:  hostname,
			Subdomain: subdomain,
		},
	}
}

func genSimpleSvc(namespace, name string) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func TestServiceSelectorCache_GetPodServiceMemberships(t *testing.T) {
	fakeInformerFactory := informers.NewSharedInformerFactory(&fake.Clientset{}, 0*time.Second)
	for i := 0; i < 3; i++ {
		service := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("service-%d", i),
				Namespace: "test",
			},
			Spec: v1.ServiceSpec{
				Selector: map[string]string{
					"app": fmt.Sprintf("test-%d", i),
				},
			},
		}
		fakeInformerFactory.Core().V1().Services().Informer().GetStore().Add(service)
	}
	var pods []*v1.Pod
	for i := 0; i < 5; i++ {
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "test",
				Name:      fmt.Sprintf("test-pod-%d", i),
				Labels: map[string]string{
					"app":   fmt.Sprintf("test-%d", i),
					"label": fmt.Sprintf("label-%d", i),
				},
			},
		}
		pods = append(pods, pod)
	}

	cache := NewServiceSelectorCache()
	tests := []struct {
		name   string
		pod    *v1.Pod
		expect sets.String
	}{
		{
			name:   "get servicesMemberships for pod-0",
			pod:    pods[0],
			expect: sets.NewString("test/service-0"),
		},
		{
			name:   "get servicesMemberships for pod-1",
			pod:    pods[1],
			expect: sets.NewString("test/service-1"),
		},
		{
			name:   "get servicesMemberships for pod-2",
			pod:    pods[2],
			expect: sets.NewString("test/service-2"),
		},
		{
			name:   "get servicesMemberships for pod-3",
			pod:    pods[3],
			expect: sets.NewString(),
		},
		{
			name:   "get servicesMemberships for pod-4",
			pod:    pods[4],
			expect: sets.NewString(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			services, err := cache.GetPodServiceMemberships(fakeInformerFactory.Core().V1().Services().Lister(), test.pod)
			if err != nil {
				t.Errorf("Error from cache.GetPodServiceMemberships: %v", err)
			} else if !services.Equal(test.expect) {
				t.Errorf("Expect service %v, but got %v", test.expect, services)
			}
		})
	}
}

func TestServiceSelectorCache_Update(t *testing.T) {
	var selectors []labels.Selector
	for i := 0; i < 5; i++ {
		selector := labels.Set(map[string]string{"app": fmt.Sprintf("test-%d", i)}).AsSelectorPreValidated()
		selectors = append(selectors, selector)
	}
	tests := []struct {
		name   string
		key    string
		cache  *ServiceSelectorCache
		update map[string]string
		expect labels.Selector
	}{
		{
			name:   "add test/service-0",
			key:    "test/service-0",
			cache:  generateServiceSelectorCache(map[string]labels.Selector{}),
			update: map[string]string{"app": "test-0"},
			expect: selectors[0],
		},
		{
			name:   "add test/service-1",
			key:    "test/service-1",
			cache:  generateServiceSelectorCache(map[string]labels.Selector{"test/service-0": selectors[0]}),
			update: map[string]string{"app": "test-1"},
			expect: selectors[1],
		},
		{
			name:   "update test/service-2",
			key:    "test/service-2",
			cache:  generateServiceSelectorCache(map[string]labels.Selector{"test/service-2": selectors[2]}),
			update: map[string]string{"app": "test-0"},
			expect: selectors[0],
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selector := test.cache.Update(test.key, test.update)
			if !reflect.DeepEqual(selector, test.expect) {
				t.Errorf("Expect selector %v , but got %v", test.expect, selector)
			}
		})
	}
}

func generateServiceSelectorCache(cache map[string]labels.Selector) *ServiceSelectorCache {
	return &ServiceSelectorCache{
		cache: cache,
	}
}

func BenchmarkGetPodServiceMemberships(b *testing.B) {
	// init fake service informer.
	fakeInformerFactory := informers.NewSharedInformerFactory(&fake.Clientset{}, 0*time.Second)
	for i := 0; i < 1000; i++ {
		service := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("service-%d", i),
				Namespace: "test",
			},
			Spec: v1.ServiceSpec{
				Selector: map[string]string{
					"app": fmt.Sprintf("test-%d", i),
				},
			},
		}
		fakeInformerFactory.Core().V1().Services().Informer().GetStore().Add(service)
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "test-pod-0",
			Labels: map[string]string{
				"app": "test-0",
			},
		},
	}

	cache := NewServiceSelectorCache()
	expect := sets.NewString("test/service-0")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		services, err := cache.GetPodServiceMemberships(fakeInformerFactory.Core().V1().Services().Lister(), pod)
		if err != nil {
			b.Fatalf("Error from GetPodServiceMemberships(): %v", err)
		}
		if len(services) != len(expect) {
			b.Errorf("Expect services size %d, but got: %v", len(expect), len(services))
		}
	}
}
