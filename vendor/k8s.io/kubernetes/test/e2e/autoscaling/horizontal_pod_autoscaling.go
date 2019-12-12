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

package autoscaling

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kubernetes/test/e2e/framework"
	e2eautoscaling "k8s.io/kubernetes/test/e2e/framework/autoscaling"

	"github.com/onsi/ginkgo"
)

// These tests don't seem to be running properly in parallel: issue: #20338.
//
var _ = SIGDescribe("[Feature:HPA] Horizontal pod autoscaling (scale resource: CPU)", func() {
	var rc *e2eautoscaling.ResourceConsumer
	f := framework.NewDefaultFramework("horizontal-pod-autoscaling")

	titleUp := "Should scale from 1 pod to 3 pods and from 3 to 5"
	titleDown := "Should scale from 5 pods to 3 pods and from 3 to 1"

	SIGDescribe("[Serial] [Slow] Deployment", func() {
		// CPU tests via deployments
		ginkgo.It(titleUp, func() {
			scaleUp("test-deployment", e2eautoscaling.KindDeployment, false, rc, f)
		})
		ginkgo.It(titleDown, func() {
			scaleDown("test-deployment", e2eautoscaling.KindDeployment, false, rc, f)
		})
	})

	SIGDescribe("[Serial] [Slow] ReplicaSet", func() {
		// CPU tests via ReplicaSets
		ginkgo.It(titleUp, func() {
			scaleUp("rs", e2eautoscaling.KindReplicaSet, false, rc, f)
		})
		ginkgo.It(titleDown, func() {
			scaleDown("rs", e2eautoscaling.KindReplicaSet, false, rc, f)
		})
	})

	// These tests take ~20 minutes each.
	SIGDescribe("[Serial] [Slow] ReplicationController", func() {
		// CPU tests via replication controllers
		ginkgo.It(titleUp+" and verify decision stability", func() {
			scaleUp("rc", e2eautoscaling.KindRC, true, rc, f)
		})
		ginkgo.It(titleDown+" and verify decision stability", func() {
			scaleDown("rc", e2eautoscaling.KindRC, true, rc, f)
		})
	})

	SIGDescribe("ReplicationController light", func() {
		ginkgo.It("Should scale from 1 pod to 2 pods", func() {
			scaleTest := &HPAScaleTest{
				initPods:                    1,
				totalInitialCPUUsage:        150,
				perPodCPURequest:            200,
				targetCPUUtilizationPercent: 50,
				minPods:                     1,
				maxPods:                     2,
				firstScale:                  2,
			}
			scaleTest.run("rc-light", e2eautoscaling.KindRC, rc, f)
		})
		ginkgo.It("Should scale from 2 pods to 1 pod [Slow]", func() {
			scaleTest := &HPAScaleTest{
				initPods:                    2,
				totalInitialCPUUsage:        50,
				perPodCPURequest:            200,
				targetCPUUtilizationPercent: 50,
				minPods:                     1,
				maxPods:                     2,
				firstScale:                  1,
			}
			scaleTest.run("rc-light", e2eautoscaling.KindRC, rc, f)
		})
	})
})

// HPAScaleTest struct is used by the scale(...) function.
type HPAScaleTest struct {
	initPods                    int
	totalInitialCPUUsage        int
	perPodCPURequest            int64
	targetCPUUtilizationPercent int32
	minPods                     int32
	maxPods                     int32
	firstScale                  int
	firstScaleStasis            time.Duration
	cpuBurst                    int
	secondScale                 int32
	secondScaleStasis           time.Duration
}

// run is a method which runs an HPA lifecycle, from a starting state, to an expected
// The initial state is defined by the initPods parameter.
// The first state change is due to the CPU being consumed initially, which HPA responds to by changing pod counts.
// The second state change (optional) is due to the CPU burst parameter, which HPA again responds to.
// TODO The use of 3 states is arbitrary, we could eventually make this test handle "n" states once this test stabilizes.
func (scaleTest *HPAScaleTest) run(name string, kind schema.GroupVersionKind, rc *e2eautoscaling.ResourceConsumer, f *framework.Framework) {
	const timeToWait = 15 * time.Minute
	rc = e2eautoscaling.NewDynamicResourceConsumer(name, f.Namespace.Name, kind, scaleTest.initPods, scaleTest.totalInitialCPUUsage, 0, 0, scaleTest.perPodCPURequest, 200, f.ClientSet, f.ScalesGetter)
	defer rc.CleanUp()
	hpa := e2eautoscaling.CreateCPUHorizontalPodAutoscaler(rc, scaleTest.targetCPUUtilizationPercent, scaleTest.minPods, scaleTest.maxPods)
	defer e2eautoscaling.DeleteHorizontalPodAutoscaler(rc, hpa.Name)

	rc.WaitForReplicas(scaleTest.firstScale, timeToWait)
	if scaleTest.firstScaleStasis > 0 {
		rc.EnsureDesiredReplicasInRange(scaleTest.firstScale, scaleTest.firstScale+1, scaleTest.firstScaleStasis, hpa.Name)
	}
	if scaleTest.cpuBurst > 0 && scaleTest.secondScale > 0 {
		rc.ConsumeCPU(scaleTest.cpuBurst)
		rc.WaitForReplicas(int(scaleTest.secondScale), timeToWait)
	}
}

func scaleUp(name string, kind schema.GroupVersionKind, checkStability bool, rc *e2eautoscaling.ResourceConsumer, f *framework.Framework) {
	stasis := 0 * time.Minute
	if checkStability {
		stasis = 10 * time.Minute
	}
	scaleTest := &HPAScaleTest{
		initPods:                    1,
		totalInitialCPUUsage:        250,
		perPodCPURequest:            500,
		targetCPUUtilizationPercent: 20,
		minPods:                     1,
		maxPods:                     5,
		firstScale:                  3,
		firstScaleStasis:            stasis,
		cpuBurst:                    700,
		secondScale:                 5,
	}
	scaleTest.run(name, kind, rc, f)
}

func scaleDown(name string, kind schema.GroupVersionKind, checkStability bool, rc *e2eautoscaling.ResourceConsumer, f *framework.Framework) {
	stasis := 0 * time.Minute
	if checkStability {
		stasis = 10 * time.Minute
	}
	scaleTest := &HPAScaleTest{
		initPods:                    5,
		totalInitialCPUUsage:        325,
		perPodCPURequest:            500,
		targetCPUUtilizationPercent: 30,
		minPods:                     1,
		maxPods:                     5,
		firstScale:                  3,
		firstScaleStasis:            stasis,
		cpuBurst:                    10,
		secondScale:                 1,
	}
	scaleTest.run(name, kind, rc, f)
}
