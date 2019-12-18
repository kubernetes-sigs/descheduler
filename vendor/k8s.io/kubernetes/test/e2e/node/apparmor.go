/*
Copyright 2016 The Kubernetes Authors.

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

package node

import (
	"k8s.io/kubernetes/test/e2e/framework"
	e2esecurity "k8s.io/kubernetes/test/e2e/framework/security"

	"github.com/onsi/ginkgo"
)

var _ = SIGDescribe("AppArmor", func() {
	f := framework.NewDefaultFramework("apparmor")

	ginkgo.Context("load AppArmor profiles", func() {
		ginkgo.BeforeEach(func() {
			framework.SkipIfAppArmorNotSupported()
			e2esecurity.LoadAppArmorProfiles(f.Namespace.Name, f.ClientSet)
		})
		ginkgo.AfterEach(func() {
			if !ginkgo.CurrentGinkgoTestDescription().Failed {
				return
			}
			framework.LogFailedContainers(f.ClientSet, f.Namespace.Name, framework.Logf)
		})

		ginkgo.It("should enforce an AppArmor profile", func() {
			e2esecurity.CreateAppArmorTestPod(f.Namespace.Name, f.ClientSet, f.PodClient(), false, true)
		})

		ginkgo.It("can disable an AppArmor profile, using unconfined", func() {
			e2esecurity.CreateAppArmorTestPod(f.Namespace.Name, f.ClientSet, f.PodClient(), true, true)
		})
	})
})
