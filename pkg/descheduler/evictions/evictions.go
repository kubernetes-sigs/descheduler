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

package evictions

import (
	"fmt"

	"k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"

	eutils "sigs.k8s.io/descheduler/pkg/descheduler/evictions/utils"
)

func EvictPod(client clientset.Interface, pod *v1.Pod, policyGroupVersion string, dryRun bool) (bool, error) {
	if dryRun {
		return true, nil
	}
	deleteOptions := &metav1.DeleteOptions{}
	// GracePeriodSeconds ?
	eviction := &policy.Eviction{
		TypeMeta: metav1.TypeMeta{
			APIVersion: policyGroupVersion,
			Kind:       eutils.EvictionKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		DeleteOptions: deleteOptions,
	}
	err := client.PolicyV1beta1().Evictions(eviction.Namespace).Evict(eviction)

	if err == nil {
		return true, nil
	} else if apierrors.IsTooManyRequests(err) {
		return false, fmt.Errorf("error when evicting pod (ignoring) %q: %v", pod.Name, err)
	} else if apierrors.IsNotFound(err) {
		return true, fmt.Errorf("pod not found when evicting %q: %v", pod.Name, err)
	} else {
		return false, err
	}
}
