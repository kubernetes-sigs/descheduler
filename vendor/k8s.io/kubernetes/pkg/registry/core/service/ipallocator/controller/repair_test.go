/*
Copyright 2015 The Kubernetes Authors.

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

package controller

import (
	"fmt"
	"net"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	api "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/registry/core/service/ipallocator"

	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/features"
)

type mockRangeRegistry struct {
	getCalled bool
	item      *api.RangeAllocation
	err       error

	updateCalled bool
	updated      *api.RangeAllocation
	updateErr    error
}

func (r *mockRangeRegistry) Get() (*api.RangeAllocation, error) {
	r.getCalled = true
	return r.item, r.err
}

func (r *mockRangeRegistry) CreateOrUpdate(alloc *api.RangeAllocation) error {
	r.updateCalled = true
	r.updated = alloc
	return r.updateErr
}

func TestRepair(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	ipregistry := &mockRangeRegistry{
		item: &api.RangeAllocation{Range: "192.168.1.0/24"},
	}
	_, cidr, _ := net.ParseCIDR(ipregistry.item.Range)
	r := NewRepair(0, fakeClient.CoreV1(), fakeClient.CoreV1(), cidr, ipregistry, nil, nil)

	if err := r.RunOnce(); err != nil {
		t.Fatal(err)
	}
	if !ipregistry.updateCalled || ipregistry.updated == nil || ipregistry.updated.Range != cidr.String() || ipregistry.updated != ipregistry.item {
		t.Errorf("unexpected ipregistry: %#v", ipregistry)
	}

	ipregistry = &mockRangeRegistry{
		item:      &api.RangeAllocation{Range: "192.168.1.0/24"},
		updateErr: fmt.Errorf("test error"),
	}
	r = NewRepair(0, fakeClient.CoreV1(), fakeClient.CoreV1(), cidr, ipregistry, nil, nil)
	if err := r.RunOnce(); !strings.Contains(err.Error(), ": test error") {
		t.Fatal(err)
	}
}

func TestRepairLeak(t *testing.T) {
	_, cidr, _ := net.ParseCIDR("192.168.1.0/24")
	previous, err := ipallocator.NewCIDRRange(cidr)
	if err != nil {
		t.Fatal(err)
	}
	previous.Allocate(net.ParseIP("192.168.1.10"))

	var dst api.RangeAllocation
	err = previous.Snapshot(&dst)
	if err != nil {
		t.Fatal(err)
	}

	fakeClient := fake.NewSimpleClientset()
	ipregistry := &mockRangeRegistry{
		item: &api.RangeAllocation{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "1",
			},
			Range: dst.Range,
			Data:  dst.Data,
		},
	}

	r := NewRepair(0, fakeClient.CoreV1(), fakeClient.CoreV1(), cidr, ipregistry, nil, nil)
	// Run through the "leak detection holdoff" loops.
	for i := 0; i < (numRepairsBeforeLeakCleanup - 1); i++ {
		if err := r.RunOnce(); err != nil {
			t.Fatal(err)
		}
		after, err := ipallocator.NewFromSnapshot(ipregistry.updated)
		if err != nil {
			t.Fatal(err)
		}
		if !after.Has(net.ParseIP("192.168.1.10")) {
			t.Errorf("expected ipallocator to still have leaked IP")
		}
	}
	// Run one more time to actually remove the leak.
	if err := r.RunOnce(); err != nil {
		t.Fatal(err)
	}
	after, err := ipallocator.NewFromSnapshot(ipregistry.updated)
	if err != nil {
		t.Fatal(err)
	}
	if after.Has(net.ParseIP("192.168.1.10")) {
		t.Errorf("expected ipallocator to not have leaked IP")
	}
}

func TestRepairWithExisting(t *testing.T) {
	_, cidr, _ := net.ParseCIDR("192.168.1.0/24")
	previous, err := ipallocator.NewCIDRRange(cidr)
	if err != nil {
		t.Fatal(err)
	}

	var dst api.RangeAllocation
	err = previous.Snapshot(&dst)
	if err != nil {
		t.Fatal(err)
	}

	fakeClient := fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: "one", Name: "one"},
			Spec:       corev1.ServiceSpec{ClusterIP: "192.168.1.1"},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: "two", Name: "two"},
			Spec:       corev1.ServiceSpec{ClusterIP: "192.168.1.100"},
		},
		&corev1.Service{ // outside CIDR, will be dropped
			ObjectMeta: metav1.ObjectMeta{Namespace: "three", Name: "three"},
			Spec:       corev1.ServiceSpec{ClusterIP: "192.168.0.1"},
		},
		&corev1.Service{ // empty, ignored
			ObjectMeta: metav1.ObjectMeta{Namespace: "four", Name: "four"},
			Spec:       corev1.ServiceSpec{ClusterIP: ""},
		},
		&corev1.Service{ // duplicate, dropped
			ObjectMeta: metav1.ObjectMeta{Namespace: "five", Name: "five"},
			Spec:       corev1.ServiceSpec{ClusterIP: "192.168.1.1"},
		},
		&corev1.Service{ // headless
			ObjectMeta: metav1.ObjectMeta{Namespace: "six", Name: "six"},
			Spec:       corev1.ServiceSpec{ClusterIP: "None"},
		},
	)

	ipregistry := &mockRangeRegistry{
		item: &api.RangeAllocation{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "1",
			},
			Range: dst.Range,
			Data:  dst.Data,
		},
	}
	r := NewRepair(0, fakeClient.CoreV1(), fakeClient.CoreV1(), cidr, ipregistry, nil, nil)
	if err := r.RunOnce(); err != nil {
		t.Fatal(err)
	}
	after, err := ipallocator.NewFromSnapshot(ipregistry.updated)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Has(net.ParseIP("192.168.1.1")) || !after.Has(net.ParseIP("192.168.1.100")) {
		t.Errorf("unexpected ipallocator state: %#v", after)
	}
	if free := after.Free(); free != 252 {
		t.Errorf("unexpected ipallocator state: %d free", free)
	}
}

func makeRangeRegistry(t *testing.T, cidrRange string) *mockRangeRegistry {
	_, cidr, _ := net.ParseCIDR(cidrRange)
	previous, err := ipallocator.NewCIDRRange(cidr)
	if err != nil {
		t.Fatal(err)
	}

	var dst api.RangeAllocation
	err = previous.Snapshot(&dst)
	if err != nil {
		t.Fatal(err)
	}

	return &mockRangeRegistry{
		item: &api.RangeAllocation{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "1",
			},
			Range: dst.Range,
			Data:  dst.Data,
		},
	}
}

func makeFakeClientSet() *fake.Clientset {
	return fake.NewSimpleClientset()
}
func makeIPNet(cidr string) *net.IPNet {
	_, net, _ := net.ParseCIDR(cidr)
	return net
}
func TestShouldWorkOnSecondary(t *testing.T) {
	testCases := []struct {
		name            string
		enableDualStack bool
		expectedResult  bool
		primaryNet      *net.IPNet
		secondaryNet    *net.IPNet
	}{
		{
			name:            "not a dual stack, primary only",
			enableDualStack: false,
			expectedResult:  false,
			primaryNet:      makeIPNet("10.0.0.0/16"),
			secondaryNet:    nil,
		},
		{
			name:            "not a dual stack, primary and secondary provided",
			enableDualStack: false,
			expectedResult:  false,
			primaryNet:      makeIPNet("10.0.0.0/16"),
			secondaryNet:    makeIPNet("2000::/120"),
		},
		{
			name:            "dual stack, primary only",
			enableDualStack: true,
			expectedResult:  false,
			primaryNet:      makeIPNet("10.0.0.0/16"),
			secondaryNet:    nil,
		},
		{
			name:            "dual stack, primary and secondary",
			enableDualStack: true,
			expectedResult:  true,
			primaryNet:      makeIPNet("10.0.0.0/16"),
			secondaryNet:    makeIPNet("2000::/120"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, tc.enableDualStack)()

			fakeClient := makeFakeClientSet()
			primaryRegistry := makeRangeRegistry(t, tc.primaryNet.String())
			var secondaryRegistery *mockRangeRegistry

			if tc.secondaryNet != nil {
				secondaryRegistery = makeRangeRegistry(t, tc.secondaryNet.String())
			}

			repair := NewRepair(0, fakeClient.CoreV1(), fakeClient.CoreV1(), tc.primaryNet, primaryRegistry, tc.secondaryNet, secondaryRegistery)
			if repair.shouldWorkOnSecondary() != tc.expectedResult {
				t.Errorf("shouldWorkOnSecondary should be %v and found %v", tc.expectedResult, repair.shouldWorkOnSecondary())
			}
		})
	}
}

func TestRepairDualStack(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, true)()

	fakeClient := fake.NewSimpleClientset()
	ipregistry := &mockRangeRegistry{
		item: &api.RangeAllocation{Range: "192.168.1.0/24"},
	}
	secondaryIPRegistry := &mockRangeRegistry{
		item: &api.RangeAllocation{Range: "2000::/108"},
	}

	_, cidr, _ := net.ParseCIDR(ipregistry.item.Range)
	_, secondaryCIDR, _ := net.ParseCIDR(secondaryIPRegistry.item.Range)
	r := NewRepair(0, fakeClient.CoreV1(), fakeClient.CoreV1(), cidr, ipregistry, secondaryCIDR, secondaryIPRegistry)

	if err := r.RunOnce(); err != nil {
		t.Fatal(err)
	}
	if !ipregistry.updateCalled || ipregistry.updated == nil || ipregistry.updated.Range != cidr.String() || ipregistry.updated != ipregistry.item {
		t.Errorf("unexpected ipregistry: %#v", ipregistry)
	}
	if !secondaryIPRegistry.updateCalled || secondaryIPRegistry.updated == nil || secondaryIPRegistry.updated.Range != secondaryCIDR.String() || secondaryIPRegistry.updated != secondaryIPRegistry.item {
		t.Errorf("unexpected ipregistry: %#v", ipregistry)
	}

	ipregistry = &mockRangeRegistry{
		item:      &api.RangeAllocation{Range: "192.168.1.0/24"},
		updateErr: fmt.Errorf("test error"),
	}
	secondaryIPRegistry = &mockRangeRegistry{
		item:      &api.RangeAllocation{Range: "2000::/108"},
		updateErr: fmt.Errorf("test error"),
	}

	r = NewRepair(0, fakeClient.CoreV1(), fakeClient.CoreV1(), cidr, ipregistry, secondaryCIDR, secondaryIPRegistry)
	if err := r.RunOnce(); !strings.Contains(err.Error(), ": test error") {
		t.Fatal(err)
	}
}

func TestRepairLeakDualStack(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, true)()

	_, cidr, _ := net.ParseCIDR("192.168.1.0/24")
	previous, err := ipallocator.NewCIDRRange(cidr)
	if err != nil {
		t.Fatal(err)
	}

	previous.Allocate(net.ParseIP("192.168.1.10"))

	_, secondaryCIDR, _ := net.ParseCIDR("2000::/108")
	secondaryPrevious, err := ipallocator.NewCIDRRange(secondaryCIDR)
	if err != nil {
		t.Fatal(err)
	}
	secondaryPrevious.Allocate(net.ParseIP("2000::1"))

	var dst api.RangeAllocation
	err = previous.Snapshot(&dst)
	if err != nil {
		t.Fatal(err)
	}

	var secondaryDST api.RangeAllocation
	err = secondaryPrevious.Snapshot(&secondaryDST)
	if err != nil {
		t.Fatal(err)
	}

	fakeClient := fake.NewSimpleClientset()

	ipregistry := &mockRangeRegistry{
		item: &api.RangeAllocation{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "1",
			},
			Range: dst.Range,
			Data:  dst.Data,
		},
	}
	secondaryIPRegistry := &mockRangeRegistry{
		item: &api.RangeAllocation{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "1",
			},
			Range: secondaryDST.Range,
			Data:  secondaryDST.Data,
		},
	}

	r := NewRepair(0, fakeClient.CoreV1(), fakeClient.CoreV1(), cidr, ipregistry, secondaryCIDR, secondaryIPRegistry)
	// Run through the "leak detection holdoff" loops.
	for i := 0; i < (numRepairsBeforeLeakCleanup - 1); i++ {
		if err := r.RunOnce(); err != nil {
			t.Fatal(err)
		}
		after, err := ipallocator.NewFromSnapshot(ipregistry.updated)
		if err != nil {
			t.Fatal(err)
		}
		if !after.Has(net.ParseIP("192.168.1.10")) {
			t.Errorf("expected ipallocator to still have leaked IP")
		}
		secondaryAfter, err := ipallocator.NewFromSnapshot(secondaryIPRegistry.updated)
		if err != nil {
			t.Fatal(err)
		}
		if !secondaryAfter.Has(net.ParseIP("2000::1")) {
			t.Errorf("expected ipallocator to still have leaked IP")
		}
	}
	// Run one more time to actually remove the leak.
	if err := r.RunOnce(); err != nil {
		t.Fatal(err)
	}

	after, err := ipallocator.NewFromSnapshot(ipregistry.updated)
	if err != nil {
		t.Fatal(err)
	}
	if after.Has(net.ParseIP("192.168.1.10")) {
		t.Errorf("expected ipallocator to not have leaked IP")
	}
	secondaryAfter, err := ipallocator.NewFromSnapshot(secondaryIPRegistry.updated)
	if err != nil {
		t.Fatal(err)
	}
	if secondaryAfter.Has(net.ParseIP("2000::1")) {
		t.Errorf("expected ipallocator to not have leaked IP")
	}
}

func TestRepairWithExistingDualStack(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.IPv6DualStack, true)()
	_, cidr, _ := net.ParseCIDR("192.168.1.0/24")
	previous, err := ipallocator.NewCIDRRange(cidr)
	if err != nil {
		t.Fatal(err)
	}

	_, secondaryCIDR, _ := net.ParseCIDR("2000::/108")
	secondaryPrevious, err := ipallocator.NewCIDRRange(secondaryCIDR)
	if err != nil {
		t.Fatal(err)
	}

	var dst api.RangeAllocation
	err = previous.Snapshot(&dst)
	if err != nil {
		t.Fatal(err)
	}

	var secondaryDST api.RangeAllocation
	err = secondaryPrevious.Snapshot(&secondaryDST)
	if err != nil {
		t.Fatal(err)
	}

	fakeClient := fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: "one", Name: "one"},
			Spec:       corev1.ServiceSpec{ClusterIP: "192.168.1.1"},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: "one", Name: "one-v6"},
			Spec:       corev1.ServiceSpec{ClusterIP: "2000::1"},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: "two", Name: "two"},
			Spec:       corev1.ServiceSpec{ClusterIP: "192.168.1.100"},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: "two", Name: "two-6"},
			Spec:       corev1.ServiceSpec{ClusterIP: "2000::2"},
		},
		&corev1.Service{ // outside CIDR, will be dropped
			ObjectMeta: metav1.ObjectMeta{Namespace: "three", Name: "three"},
			Spec:       corev1.ServiceSpec{ClusterIP: "192.168.0.1"},
		},
		&corev1.Service{ // outside CIDR, will be dropped
			ObjectMeta: metav1.ObjectMeta{Namespace: "three", Name: "three-v6"},
			Spec:       corev1.ServiceSpec{ClusterIP: "3000::1"},
		},
		&corev1.Service{ // empty, ignored
			ObjectMeta: metav1.ObjectMeta{Namespace: "four", Name: "four"},
			Spec:       corev1.ServiceSpec{ClusterIP: ""},
		},
		&corev1.Service{ // duplicate, dropped
			ObjectMeta: metav1.ObjectMeta{Namespace: "five", Name: "five"},
			Spec:       corev1.ServiceSpec{ClusterIP: "192.168.1.1"},
		},
		&corev1.Service{ // duplicate, dropped
			ObjectMeta: metav1.ObjectMeta{Namespace: "five", Name: "five-v6"},
			Spec:       corev1.ServiceSpec{ClusterIP: "2000::2"},
		},

		&corev1.Service{ // headless
			ObjectMeta: metav1.ObjectMeta{Namespace: "six", Name: "six"},
			Spec:       corev1.ServiceSpec{ClusterIP: "None"},
		},
	)

	ipregistry := &mockRangeRegistry{
		item: &api.RangeAllocation{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "1",
			},
			Range: dst.Range,
			Data:  dst.Data,
		},
	}

	secondaryIPRegistry := &mockRangeRegistry{
		item: &api.RangeAllocation{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "1",
			},
			Range: secondaryDST.Range,
			Data:  secondaryDST.Data,
		},
	}

	r := NewRepair(0, fakeClient.CoreV1(), fakeClient.CoreV1(), cidr, ipregistry, secondaryCIDR, secondaryIPRegistry)
	if err := r.RunOnce(); err != nil {
		t.Fatal(err)
	}
	after, err := ipallocator.NewFromSnapshot(ipregistry.updated)
	if err != nil {
		t.Fatal(err)
	}

	if !after.Has(net.ParseIP("192.168.1.1")) || !after.Has(net.ParseIP("192.168.1.100")) {
		t.Errorf("unexpected ipallocator state: %#v", after)
	}
	if free := after.Free(); free != 252 {
		t.Errorf("unexpected ipallocator state: %d free (number of free ips is not 252)", free)
	}
	secondaryAfter, err := ipallocator.NewFromSnapshot(secondaryIPRegistry.updated)
	if err != nil {
		t.Fatal(err)
	}
	if !secondaryAfter.Has(net.ParseIP("2000::1")) || !secondaryAfter.Has(net.ParseIP("2000::2")) {
		t.Errorf("unexpected ipallocator state: %#v", secondaryAfter)
	}
	if free := secondaryAfter.Free(); free != 65532 {
		t.Errorf("unexpected ipallocator state: %d free (number of free ips is not 65532)", free)
	}

}
