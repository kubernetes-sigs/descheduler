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

package evictions

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadResolver maps a pod to the (kind, name) of the workload owning it,
// reported as the workload_kind/workload_name labels of the
// descheduler_pod_evictions_total metric. Implementations must return stable,
// bounded values (no per-pod or per-rollout generated names) and are expected
// to be safe for concurrent use. Returning two empty strings means "no owning
// workload"; the metric then reports a constant placeholder.
type WorkloadResolver func(pod *v1.Pod) (kind, name string)

// workloadNone is the workload kind/name reported for pods without a
// resolvable owning workload (bare pods). A constant placeholder is used
// instead of the pod name to keep the metric label cardinality bounded.
const workloadNone = "<none>"

// defaultWorkloadResolver reports the pod's direct controller. It is used when
// no resolver is injected via WithWorkloadResolver. The descheduler injects a
// resolver that additionally follows the ReplicaSet->Deployment and
// Job->CronJob hops to keep generated intermediate-owner names out of the
// metric.
func defaultWorkloadResolver(pod *v1.Pod) (string, string) {
	ref := metav1.GetControllerOf(pod)
	if ref == nil {
		return "", ""
	}
	return ref.Kind, ref.Name
}

// workloadLabelValues converts a resolver result to metric label values,
// substituting the workloadNone placeholder for absent values.
func workloadLabelValues(kind, name string) (string, string) {
	if kind == "" {
		kind = workloadNone
	}
	if name == "" {
		name = workloadNone
	}
	return kind, name
}
