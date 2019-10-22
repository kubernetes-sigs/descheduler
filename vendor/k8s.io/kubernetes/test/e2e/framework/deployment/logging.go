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

package deployment

import (
	appsv1 "k8s.io/api/apps/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	testutils "k8s.io/kubernetes/test/utils"
)

func logReplicaSetsOfDeployment(deployment *appsv1.Deployment, allOldRSs []*appsv1.ReplicaSet, newRS *appsv1.ReplicaSet) {
	testutils.LogReplicaSetsOfDeployment(deployment, allOldRSs, newRS, framework.Logf)
}

func logPodsOfDeployment(c clientset.Interface, deployment *appsv1.Deployment, rsList []*appsv1.ReplicaSet) {
	testutils.LogPodsOfDeployment(c, deployment, rsList, framework.Logf)
}
