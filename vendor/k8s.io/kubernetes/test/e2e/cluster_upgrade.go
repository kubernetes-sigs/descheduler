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

package e2e

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/kubernetes/pkg/util/version"
	"k8s.io/kubernetes/test/e2e/chaosmonkey"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/ginkgowrapper"
	"k8s.io/kubernetes/test/e2e/upgrades"
	"k8s.io/kubernetes/test/utils/junit"

	. "github.com/onsi/ginkgo"
)

var upgradeTests = []upgrades.Test{
	&upgrades.ServiceUpgradeTest{},
	&upgrades.SecretUpgradeTest{},
	&upgrades.StatefulSetUpgradeTest{},
	&upgrades.DeploymentUpgradeTest{},
	&upgrades.JobUpgradeTest{},
	&upgrades.ConfigMapUpgradeTest{},
	&upgrades.HPAUpgradeTest{},
	&upgrades.PersistentVolumeUpgradeTest{},
	&upgrades.DaemonSetUpgradeTest{},
	&upgrades.IngressUpgradeTest{},
	&upgrades.AppArmorUpgradeTest{},
}

var _ = framework.KubeDescribe("Upgrade [Feature:Upgrade]", func() {
	f := framework.NewDefaultFramework("cluster-upgrade")

	// Create the frameworks here because we can only create them
	// in a "Describe".
	testFrameworks := createUpgradeFrameworks()
	framework.KubeDescribe("master upgrade", func() {
		It("should maintain a functioning cluster [Feature:MasterUpgrade]", func() {
			upgCtx, err := getUpgradeContext(f.ClientSet.Discovery(), framework.TestContext.UpgradeTarget)
			framework.ExpectNoError(err)

			testSuite := &junit.TestSuite{Name: "Master upgrade"}
			masterUpgradeTest := &junit.TestCase{Name: "master-upgrade", Classname: "upgrade_tests"}
			testSuite.TestCases = append(testSuite.TestCases, masterUpgradeTest)

			upgradeFunc := func() {
				start := time.Now()
				defer finalizeUpgradeTest(start, masterUpgradeTest)
				target := upgCtx.Versions[1].Version.String()
				framework.ExpectNoError(framework.MasterUpgrade(target))
				framework.ExpectNoError(framework.CheckMasterVersion(f.ClientSet, target))
			}
			runUpgradeSuite(f, testFrameworks, testSuite, upgCtx, upgrades.MasterUpgrade, upgradeFunc)
		})
	})

	framework.KubeDescribe("node upgrade", func() {
		It("should maintain a functioning cluster [Feature:NodeUpgrade]", func() {
			upgCtx, err := getUpgradeContext(f.ClientSet.Discovery(), framework.TestContext.UpgradeTarget)
			framework.ExpectNoError(err)

			testSuite := &junit.TestSuite{Name: "Node upgrade"}
			nodeUpgradeTest := &junit.TestCase{Name: "node-upgrade", Classname: "upgrade_tests"}

			upgradeFunc := func() {
				start := time.Now()
				defer finalizeUpgradeTest(start, nodeUpgradeTest)
				target := upgCtx.Versions[1].Version.String()
				framework.ExpectNoError(framework.NodeUpgrade(f, target, framework.TestContext.UpgradeImage))
				framework.ExpectNoError(framework.CheckNodesVersions(f.ClientSet, target))
			}
			runUpgradeSuite(f, testFrameworks, testSuite, upgCtx, upgrades.NodeUpgrade, upgradeFunc)
		})
	})

	framework.KubeDescribe("cluster upgrade", func() {
		It("should maintain a functioning cluster [Feature:ClusterUpgrade]", func() {
			upgCtx, err := getUpgradeContext(f.ClientSet.Discovery(), framework.TestContext.UpgradeTarget)
			framework.ExpectNoError(err)

			testSuite := &junit.TestSuite{Name: "Cluster upgrade"}
			clusterUpgradeTest := &junit.TestCase{Name: "cluster-upgrade", Classname: "upgrade_tests"}
			testSuite.TestCases = append(testSuite.TestCases, clusterUpgradeTest)
			upgradeFunc := func() {
				start := time.Now()
				defer finalizeUpgradeTest(start, clusterUpgradeTest)
				target := upgCtx.Versions[1].Version.String()
				framework.ExpectNoError(framework.MasterUpgrade(target))
				framework.ExpectNoError(framework.CheckMasterVersion(f.ClientSet, target))
				framework.ExpectNoError(framework.NodeUpgrade(f, target, framework.TestContext.UpgradeImage))
				framework.ExpectNoError(framework.CheckNodesVersions(f.ClientSet, target))
			}
			runUpgradeSuite(f, testFrameworks, testSuite, upgCtx, upgrades.ClusterUpgrade, upgradeFunc)
		})
	})
})

var _ = framework.KubeDescribe("Downgrade [Feature:Downgrade]", func() {
	f := framework.NewDefaultFramework("cluster-downgrade")

	// Create the frameworks here because we can only create them
	// in a "Describe".
	testFrameworks := map[string]*framework.Framework{}
	for _, t := range upgradeTests {
		testFrameworks[t.Name()] = framework.NewDefaultFramework(t.Name())
	}

	framework.KubeDescribe("cluster downgrade", func() {
		It("should maintain a functioning cluster [Feature:ClusterDowngrade]", func() {
			upgCtx, err := getUpgradeContext(f.ClientSet.Discovery(), framework.TestContext.UpgradeTarget)
			framework.ExpectNoError(err)

			testSuite := &junit.TestSuite{Name: "Cluster downgrade"}
			clusterDowngradeTest := &junit.TestCase{Name: "cluster-downgrade", Classname: "upgrade_tests"}
			testSuite.TestCases = append(testSuite.TestCases, clusterDowngradeTest)

			upgradeFunc := func() {
				start := time.Now()
				defer finalizeUpgradeTest(start, clusterDowngradeTest)
				// Yes this really is a downgrade. And nodes must downgrade first.
				target := upgCtx.Versions[1].Version.String()
				framework.ExpectNoError(framework.NodeUpgrade(f, target, framework.TestContext.UpgradeImage))
				framework.ExpectNoError(framework.CheckNodesVersions(f.ClientSet, target))
				framework.ExpectNoError(framework.MasterUpgrade(target))
				framework.ExpectNoError(framework.CheckMasterVersion(f.ClientSet, target))
			}
			runUpgradeSuite(f, testFrameworks, testSuite, upgCtx, upgrades.ClusterUpgrade, upgradeFunc)
		})
	})
})

var _ = framework.KubeDescribe("etcd Upgrade [Feature:EtcdUpgrade]", func() {
	f := framework.NewDefaultFramework("etc-upgrade")

	// Create the frameworks here because we can only create them
	// in a "Describe".
	testFrameworks := createUpgradeFrameworks()
	framework.KubeDescribe("etcd upgrade", func() {
		It("should maintain a functioning cluster", func() {
			upgCtx, err := getUpgradeContext(f.ClientSet.Discovery(), "")
			framework.ExpectNoError(err)

			testSuite := &junit.TestSuite{Name: "Etcd upgrade"}
			etcdTest := &junit.TestCase{Name: "etcd-upgrade", Classname: "upgrade_tests"}
			testSuite.TestCases = append(testSuite.TestCases, etcdTest)

			upgradeFunc := func() {
				start := time.Now()
				defer finalizeUpgradeTest(start, etcdTest)
				framework.ExpectNoError(framework.EtcdUpgrade(framework.TestContext.EtcdUpgradeStorage, framework.TestContext.EtcdUpgradeVersion))
			}
			runUpgradeSuite(f, testFrameworks, testSuite, upgCtx, upgrades.EtcdUpgrade, upgradeFunc)
		})
	})
})

type chaosMonkeyAdapter struct {
	test        upgrades.Test
	testReport  *junit.TestCase
	framework   *framework.Framework
	upgradeType upgrades.UpgradeType
	upgCtx      upgrades.UpgradeContext
}

func (cma *chaosMonkeyAdapter) Test(sem *chaosmonkey.Semaphore) {
	start := time.Now()
	var once sync.Once
	ready := func() {
		once.Do(func() {
			sem.Ready()
		})
	}
	defer finalizeUpgradeTest(start, cma.testReport)
	defer ready()
	if skippable, ok := cma.test.(upgrades.Skippable); ok && skippable.Skip(cma.upgCtx) {
		By("skipping test " + cma.test.Name())
		cma.testReport.Skipped = "skipping test " + cma.test.Name()
		return
	}

	defer cma.test.Teardown(cma.framework)
	cma.test.Setup(cma.framework)
	ready()
	cma.test.Test(cma.framework, sem.StopCh, cma.upgradeType)
}

func finalizeUpgradeTest(start time.Time, tc *junit.TestCase) {
	tc.Time = time.Since(start).Seconds()
	r := recover()
	if r == nil {
		return
	}

	switch r := r.(type) {
	case ginkgowrapper.FailurePanic:
		tc.Failures = []*junit.Failure{
			{
				Message: r.Message,
				Type:    "Failure",
				Value:   fmt.Sprintf("%s\n\n%s", r.Message, r.FullStackTrace),
			},
		}
	case ginkgowrapper.SkipPanic:
		tc.Skipped = fmt.Sprintf("%s:%d %q", r.Filename, r.Line, r.Message)
	default:
		tc.Errors = []*junit.Error{
			{
				Message: fmt.Sprintf("%v", r),
				Type:    "Panic",
				Value:   fmt.Sprintf("%v", r),
			},
		}
	}
}

func createUpgradeFrameworks() map[string]*framework.Framework {
	testFrameworks := map[string]*framework.Framework{}
	for _, t := range upgradeTests {
		testFrameworks[t.Name()] = framework.NewDefaultFramework(t.Name())
	}
	return testFrameworks
}

func runUpgradeSuite(
	f *framework.Framework,
	testFrameworks map[string]*framework.Framework,
	testSuite *junit.TestSuite,
	upgCtx *upgrades.UpgradeContext,
	upgradeType upgrades.UpgradeType,
	upgradeFunc func(),
) {
	upgCtx, err := getUpgradeContext(f.ClientSet.Discovery(), framework.TestContext.UpgradeTarget)
	framework.ExpectNoError(err)

	cm := chaosmonkey.New(upgradeFunc)
	for _, t := range upgradeTests {
		testCase := &junit.TestCase{
			Name:      t.Name(),
			Classname: "upgrade_tests",
		}
		testSuite.TestCases = append(testSuite.TestCases, testCase)
		cma := chaosMonkeyAdapter{
			test:        t,
			testReport:  testCase,
			framework:   testFrameworks[t.Name()],
			upgradeType: upgradeType,
			upgCtx:      *upgCtx,
		}
		cm.Register(cma.Test)
	}

	start := time.Now()
	defer func() {
		testSuite.Update()
		testSuite.Time = time.Since(start).Seconds()
		if framework.TestContext.ReportDir != "" {
			fname := filepath.Join(framework.TestContext.ReportDir, fmt.Sprintf("junit_%supgrades.xml", framework.TestContext.ReportPrefix))
			f, err := os.Create(fname)
			if err != nil {
				return
			}
			defer f.Close()
			xml.NewEncoder(f).Encode(testSuite)
		}
	}()
	cm.Do()
}

func getUpgradeContext(c discovery.DiscoveryInterface, upgradeTarget string) (*upgrades.UpgradeContext, error) {
	current, err := c.ServerVersion()
	if err != nil {
		return nil, err
	}

	curVer, err := version.ParseSemantic(current.String())
	if err != nil {
		return nil, err
	}

	upgCtx := &upgrades.UpgradeContext{
		Versions: []upgrades.VersionContext{
			{
				Version:   *curVer,
				NodeImage: framework.TestContext.NodeOSDistro,
			},
		},
	}

	if len(upgradeTarget) == 0 {
		return upgCtx, nil
	}

	next, err := framework.RealVersion(upgradeTarget)
	if err != nil {
		return nil, err
	}

	nextVer, err := version.ParseSemantic(next)
	if err != nil {
		return nil, err
	}

	upgCtx.Versions = append(upgCtx.Versions, upgrades.VersionContext{
		Version:   *nextVer,
		NodeImage: framework.TestContext.UpgradeImage,
	})

	return upgCtx, nil
}
