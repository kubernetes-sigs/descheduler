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

package descheduler

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1listers "k8s.io/client-go/listers/apps/v1"
	batchv1listers "k8s.io/client-go/listers/batch/v1"
	"k8s.io/client-go/tools/cache"
)

func controllerRef(apiVersion, kind, name string) metav1.OwnerReference {
	isController := true
	return metav1.OwnerReference{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		Controller: &isController,
	}
}

func podWithOwners(ns, name string, owners ...metav1.OwnerReference) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       ns,
			Name:            name,
			OwnerReferences: owners,
		},
	}
}

func replicaSetWithOwners(ns, name string, owners ...metav1.OwnerReference) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       ns,
			Name:            name,
			OwnerReferences: owners,
		},
	}
}

func jobWithOwners(ns, name string, owners ...metav1.OwnerReference) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       ns,
			Name:            name,
			OwnerReferences: owners,
		},
	}
}

func newReplicaSetLister(rss ...*appsv1.ReplicaSet) appsv1listers.ReplicaSetLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	for _, rs := range rss {
		_ = indexer.Add(rs)
	}
	return appsv1listers.NewReplicaSetLister(indexer)
}

func newJobLister(jobs ...*batchv1.Job) batchv1listers.JobLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	for _, job := range jobs {
		_ = indexer.Add(job)
	}
	return batchv1listers.NewJobLister(indexer)
}

func TestWorkloadResolver(t *testing.T) {
	tests := []struct {
		name         string
		pod          *v1.Pod
		replicaSets  []*appsv1.ReplicaSet
		jobs         []*batchv1.Job
		expectedKind string
		expectedName string
	}{
		{
			name:         "deployment resolved through replicaset",
			pod:          podWithOwners("default", "web-abc123-xyz", controllerRef("apps/v1", "ReplicaSet", "web-abc123")),
			replicaSets:  []*appsv1.ReplicaSet{replicaSetWithOwners("default", "web-abc123", controllerRef("apps/v1", "Deployment", "web"))},
			expectedKind: "Deployment",
			expectedName: "web",
		},
		{
			name:         "cronjob resolved through job",
			pod:          podWithOwners("default", "mycron-28934712-xyz", controllerRef("batch/v1", "Job", "mycron-28934712")),
			jobs:         []*batchv1.Job{jobWithOwners("default", "mycron-28934712", controllerRef("batch/v1", "CronJob", "mycron"))},
			expectedKind: "CronJob",
			expectedName: "mycron",
		},
		{
			name:         "statefulset owns pod directly",
			pod:          podWithOwners("default", "db-0", controllerRef("apps/v1", "StatefulSet", "db")),
			expectedKind: "StatefulSet",
			expectedName: "db",
		},
		{
			name:         "daemonset owns pod directly",
			pod:          podWithOwners("kube-system", "node-exporter-9f2", controllerRef("apps/v1", "DaemonSet", "node-exporter")),
			expectedKind: "DaemonSet",
			expectedName: "node-exporter",
		},
		{
			name:         "bare pod with no controller",
			pod:          podWithOwners("default", "lonely-pod"),
			expectedKind: "",
			expectedName: "",
		},
		{
			name:         "replicaset missing from cache falls back to replicaset",
			pod:          podWithOwners("default", "web-abc123-xyz", controllerRef("apps/v1", "ReplicaSet", "web-abc123")),
			replicaSets:  nil,
			expectedKind: "ReplicaSet",
			expectedName: "web-abc123",
		},
		{
			name:         "orphan replicaset without controller falls back to replicaset",
			pod:          podWithOwners("default", "web-abc123-xyz", controllerRef("apps/v1", "ReplicaSet", "web-abc123")),
			replicaSets:  []*appsv1.ReplicaSet{replicaSetWithOwners("default", "web-abc123")},
			expectedKind: "ReplicaSet",
			expectedName: "web-abc123",
		},
		{
			name:         "standalone job without controller falls back to job",
			pod:          podWithOwners("default", "migrate-xyz", controllerRef("batch/v1", "Job", "migrate")),
			jobs:         []*batchv1.Job{jobWithOwners("default", "migrate")},
			expectedKind: "Job",
			expectedName: "migrate",
		},
		{
			name:         "custom-group ReplicaSet kind is not resolved through the apps lister",
			pod:          podWithOwners("default", "custom-pod", controllerRef("example.io/v1", "ReplicaSet", "web-abc123")),
			replicaSets:  []*appsv1.ReplicaSet{replicaSetWithOwners("default", "web-abc123", controllerRef("apps/v1", "Deployment", "web"))},
			expectedKind: "ReplicaSet",
			expectedName: "web-abc123",
		},
		{
			name:         "custom-group Job kind is not resolved through the batch lister",
			pod:          podWithOwners("default", "custom-pod", controllerRef("example.io/v1", "Job", "mycron-28934712")),
			jobs:         []*batchv1.Job{jobWithOwners("default", "mycron-28934712", controllerRef("batch/v1", "CronJob", "mycron"))},
			expectedKind: "Job",
			expectedName: "mycron-28934712",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolve := newWorkloadResolver(newReplicaSetLister(tt.replicaSets...), newJobLister(tt.jobs...))
			kind, name := resolve(tt.pod)
			if kind != tt.expectedKind || name != tt.expectedName {
				t.Errorf("workload resolver returned (%q, %q), want (%q, %q)", kind, name, tt.expectedKind, tt.expectedName)
			}
		})
	}
}
