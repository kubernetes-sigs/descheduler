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
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	appsv1listers "k8s.io/client-go/listers/apps/v1"
	batchv1listers "k8s.io/client-go/listers/batch/v1"

	"sigs.k8s.io/descheduler/pkg/descheduler/evictions"
)

// newWorkloadResolver returns the resolver used for the workload_kind and
// workload_name labels of the pod_evictions_total metric.
//
// The pod's controller ownerRef is used directly for workloads that own pods
// themselves (StatefulSet, DaemonSet, standalone Job, custom controllers).
// Two intermediate owners with generated per-rollout/per-run names are
// resolved one hop further to keep the label values stable and bounded:
// apps ReplicaSet -> Deployment and batch Job -> CronJob. If the intermediate
// object cannot be resolved (cache miss) or has no controller of its own
// (orphan), the intermediate itself is reported.
func newWorkloadResolver(replicaSetLister appsv1listers.ReplicaSetLister, jobLister batchv1listers.JobLister) evictions.WorkloadResolver {
	return func(pod *v1.Pod) (string, string) {
		ref := metav1.GetControllerOf(pod)
		if ref == nil {
			return "", ""
		}
		switch {
		case ref.Kind == "ReplicaSet" && ownerRefGroup(ref) == appsv1.GroupName:
			if rs, err := replicaSetLister.ReplicaSets(pod.Namespace).Get(ref.Name); err == nil {
				if owner := metav1.GetControllerOf(rs); owner != nil {
					return owner.Kind, owner.Name
				}
			}
		case ref.Kind == "Job" && ownerRefGroup(ref) == batchv1.GroupName:
			if job, err := jobLister.Jobs(pod.Namespace).Get(ref.Name); err == nil {
				if owner := metav1.GetControllerOf(job); owner != nil {
					return owner.Kind, owner.Name
				}
			}
		}
		return ref.Kind, ref.Name
	}
}

func ownerRefGroup(ref *metav1.OwnerReference) string {
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return ""
	}
	return gv.Group
}
