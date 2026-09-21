/*
Copyright 2026 The Kubernetes Authors.

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

package deploymentevictionlimit

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	frameworkfake "sigs.k8s.io/descheduler/pkg/framework/fake"
)

func boolPtr(b bool) *bool { return &b }

// deploymentPod builds a pod exactly as the Deployment controller would: owned
// by an apps/v1 ReplicaSet named "<deployment>-<hash>" and labelled with
// pod-template-hash=<hash>.
func deploymentPod(ns, name, deployment, hash string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
			UID:       types.UID(name),
			Labels:    map[string]string{appsv1.DefaultDeploymentUniqueLabelKey: hash},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "ReplicaSet",
				Name:       deployment + "-" + hash,
				UID:        types.UID(deployment + "-" + hash),
				Controller: boolPtr(true),
			}},
		},
	}
}

func newPlugin(t *testing.T, args *DeploymentEvictionLimitArgs) *DeploymentEvictionLimit {
	t.Helper()
	pl, err := New(context.Background(), args, &frameworkfake.HandleImpl{})
	if err != nil {
		t.Fatalf("unable to initialize plugin: %v", err)
	}
	return pl.(*DeploymentEvictionLimit)
}

func limits(entries ...DeploymentLimit) *DeploymentEvictionLimitArgs {
	return &DeploymentEvictionLimitArgs{Deployments: entries}
}

func TestPreEvictionFilterEnforcesBudgetPerDeployment(t *testing.T) {
	pl := newPlugin(t, limits(DeploymentLimit{Namespace: "shop", Name: "web", MaxPodsToEvict: 2}))

	web1 := deploymentPod("shop", "web-1", "web", "6d4cf56db6")
	web2 := deploymentPod("shop", "web-2", "web", "6d4cf56db6")
	web3 := deploymentPod("shop", "web-3", "web", "6d4cf56db6")
	api1 := deploymentPod("shop", "api-1", "api", "7f9c8b5d4e")

	if !pl.Filter(web1) {
		t.Fatal("Filter should pass web pod before any budget is consumed")
	}
	if !pl.PreEvictionFilter(web1) {
		t.Fatal("first web pod should be cleared")
	}
	if !pl.PreEvictionFilter(web2) {
		t.Fatal("second web pod should be cleared")
	}
	if pl.PreEvictionFilter(web3) {
		t.Fatal("third web pod should be rejected: budget is 2")
	}
	if pl.Filter(web3) {
		t.Fatal("Filter should reject an uncleared web pod once the budget is exhausted")
	}

	for i := 0; i < 5; i++ {
		if !pl.PreEvictionFilter(api1) {
			t.Fatalf("unlisted deployment should not be limited (attempt %d)", i+1)
		}
	}
}

func TestRepeatedCallsForTheSamePodConsumeBudgetOnce(t *testing.T) {
	pl := newPlugin(t, limits(DeploymentLimit{Namespace: "shop", Name: "web", MaxPodsToEvict: 2}))

	web1 := deploymentPod("shop", "web-1", "web", "6d4cf56db6")
	web2 := deploymentPod("shop", "web-2", "web", "6d4cf56db6")
	web3 := deploymentPod("shop", "web-3", "web", "6d4cf56db6")

	// Strategies such as RemovePodsViolatingNodeAffinity call PreEvictionFilter
	// while listing candidates and again right before evicting.
	for i := 0; i < 3; i++ {
		if !pl.PreEvictionFilter(web1) {
			t.Fatalf("repeated PreEvictionFilter for the same pod must stay true (call %d)", i+1)
		}
	}
	if !pl.PreEvictionFilter(web2) {
		t.Fatal("second distinct pod should be cleared: only one unit was consumed so far")
	}
	if pl.PreEvictionFilter(web3) {
		t.Fatal("third distinct pod should be rejected")
	}
}

func TestFilterPassesClearedPodsAfterBudgetIsExhausted(t *testing.T) {
	pl := newPlugin(t, limits(DeploymentLimit{Namespace: "shop", Name: "web", MaxPodsToEvict: 1}))

	web1 := deploymentPod("shop", "web-1", "web", "6d4cf56db6")
	web2 := deploymentPod("shop", "web-2", "web", "6d4cf56db6")

	if !pl.PreEvictionFilter(web1) {
		t.Fatal("web-1 should be cleared")
	}
	// Right before evicting, strategies typically re-run Filter on the pod they
	// cleared during listing. That must not be blocked by the exhausted budget.
	if !pl.Filter(web1) {
		t.Fatal("Filter must pass a pod that was already cleared")
	}
	if pl.Filter(web2) {
		t.Fatal("Filter must reject an uncleared pod once the budget is exhausted")
	}
}

func TestDescheduleResetsBudget(t *testing.T) {
	pl := newPlugin(t, limits(DeploymentLimit{Namespace: "shop", Name: "web", MaxPodsToEvict: 1}))
	web1 := deploymentPod("shop", "web-1", "web", "6d4cf56db6")
	web2 := deploymentPod("shop", "web-2", "web", "6d4cf56db6")

	if !pl.PreEvictionFilter(web1) {
		t.Fatal("first pod should be cleared")
	}
	if pl.PreEvictionFilter(web2) {
		t.Fatal("second pod in the same cycle should be rejected")
	}

	if status := pl.Deschedule(context.Background(), nil); status == nil || status.Err != nil {
		t.Fatalf("Deschedule should succeed without error, got %v", status)
	}
	if !pl.PreEvictionFilter(web2) {
		t.Fatal("a pod should be cleared again after the per-cycle reset")
	}
}

func TestZeroBudgetBlocksAllEvictions(t *testing.T) {
	pl := newPlugin(t, limits(DeploymentLimit{Namespace: "shop", Name: "web", MaxPodsToEvict: 0}))
	pod := deploymentPod("shop", "web-1", "web", "6d4cf56db6")

	if pl.Filter(pod) {
		t.Fatal("Filter should reject pods of a Deployment with a zero budget")
	}
	if pl.PreEvictionFilter(pod) {
		t.Fatal("PreEvictionFilter should reject pods of a Deployment with a zero budget")
	}
}

func TestPodsNotOwnedByAConfiguredDeploymentAreNotLimited(t *testing.T) {
	pl := newPlugin(t, limits(DeploymentLimit{Namespace: "shop", Name: "web", MaxPodsToEvict: 0}))

	bare := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "bare", UID: "bare"}}

	sts := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "shop",
			Name:            "db-0",
			UID:             "db-0",
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "db", UID: "db", Controller: boolPtr(true)}},
		},
	}

	// A hand-made ReplicaSet: pods carry no pod-template-hash label.
	noHash := deploymentPod("shop", "web-x", "web", "6d4cf56db6")
	noHash.Labels = nil

	// The ReplicaSet name does not end with the pod's hash, so it is not a
	// Deployment-managed ReplicaSet named "web".
	mismatch := deploymentPod("shop", "web-y", "web", "6d4cf56db6")
	mismatch.OwnerReferences[0].Name = "web-deadbeef"

	// Same Deployment name in a different namespace than the configured one.
	otherNS := deploymentPod("other", "web-1", "web", "6d4cf56db6")

	// A ReplicaSet from a non-apps API group that happens to be called ReplicaSet.
	foreignGroup := deploymentPod("shop", "web-z", "web", "6d4cf56db6")
	foreignGroup.OwnerReferences[0].APIVersion = "example.io/v1"

	for _, tc := range []struct {
		name string
		pod  *v1.Pod
	}{
		{"bare pod", bare},
		{"statefulset pod", sts},
		{"replicaset pod without pod-template-hash label", noHash},
		{"replicaset name not ending in the pod's hash", mismatch},
		{"same deployment name in another namespace", otherNS},
		{"ReplicaSet kind from a foreign API group", foreignGroup},
	} {
		if !pl.Filter(tc.pod) || !pl.PreEvictionFilter(tc.pod) {
			t.Errorf("%s should not be limited", tc.name)
		}
	}
}

func TestDeploymentNameOf(t *testing.T) {
	for _, tc := range []struct {
		name   string
		pod    *v1.Pod
		want   string
		wantOK bool
	}{
		{"regular deployment pod", deploymentPod("shop", "p", "web", "6d4cf56db6"), "web", true},
		{"deployment name containing dashes", deploymentPod("shop", "p", "web-frontend-v2", "abc123"), "web-frontend-v2", true},
		{"bare pod", &v1.Pod{}, "", false},
	} {
		got, ok := deploymentNameOf(tc.pod)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestValidateDeploymentEvictionLimitArgs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    *DeploymentEvictionLimitArgs
		wantErr bool
	}{
		{name: "empty list", args: &DeploymentEvictionLimitArgs{}, wantErr: true},
		{name: "missing namespace", args: limits(DeploymentLimit{Name: "web", MaxPodsToEvict: 1}), wantErr: true},
		{name: "missing name", args: limits(DeploymentLimit{Namespace: "shop", MaxPodsToEvict: 1}), wantErr: true},
		{name: "duplicate entry", args: limits(
			DeploymentLimit{Namespace: "shop", Name: "web", MaxPodsToEvict: 1},
			DeploymentLimit{Namespace: "shop", Name: "web", MaxPodsToEvict: 2},
		), wantErr: true},
		{name: "valid", args: limits(
			DeploymentLimit{Namespace: "shop", Name: "web", MaxPodsToEvict: 1},
			DeploymentLimit{Namespace: "shop", Name: "api", MaxPodsToEvict: 0},
			DeploymentLimit{Namespace: "other", Name: "web", MaxPodsToEvict: 3},
		)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDeploymentEvictionLimitArgs(tc.args)
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got err=%v", tc.wantErr, err)
			}
		})
	}
}
