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

package taints

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	taintutils "k8s.io/kubernetes/pkg/util/taints"
)

type Tainter struct {
	client clientset.Interface
}

func NewTainter(client clientset.Interface) *Tainter {
	return &Tainter{
		client: client,
	}
}

func (t *Tainter) Taint(ctx context.Context, n *corev1.Node) error {
	newNode, changed, err := taintutils.AddOrUpdateTaint(n, &corev1.Taint{
		Effect: corev1.TaintEffectPreferNoSchedule,
		Key:    "sigs.k8s.io/descheduler",
	})
	if err != nil {
		return fmt.Errorf("add or update taint: %w", err)
	}
	if !changed {
		return nil
	}
	_, err = t.client.CoreV1().Nodes().Update(ctx, newNode, v1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("taint node: %w", err)
	}
	return nil
}
