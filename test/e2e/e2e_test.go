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

package e2e

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/descheduler/cmd/descheduler/app/options"
	deschedulerapi "github.com/kubernetes-incubator/descheduler/pkg/api"
	eutils "github.com/kubernetes-incubator/descheduler/pkg/descheduler/evictions/utils"
	nodeutil "github.com/kubernetes-incubator/descheduler/pkg/descheduler/node"
	"github.com/kubernetes-incubator/descheduler/pkg/descheduler/strategies"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientv1core "k8s.io/client-go/kubernetes/typed/core/v1"
	clientv1 "k8s.io/client-go/pkg/api/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/testapi"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
	informers "k8s.io/kubernetes/pkg/client/informers/informers_generated/externalversions"
	"k8s.io/kubernetes/plugin/pkg/scheduler"
	_ "k8s.io/kubernetes/plugin/pkg/scheduler/algorithmprovider"
	"k8s.io/kubernetes/plugin/pkg/scheduler/factory"
	"k8s.io/kubernetes/test/integration/framework"
	"k8s.io/apimachinery/pkg/runtime"
	refv1 "k8s.io/kubernetes/pkg/api/v1/ref"
	testutils "k8s.io/kubernetes/test/utils"
)


var (
	basePodTemplate = & v1.Pod {
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "sched-perf-pod-",
			SelfLink:     testapi.Default.SelfLink("pods", "pod"),
		},
		// TODO: this needs to be configurable.
		Spec: testutils.MakePodSpec(),
	}
	baseNodeTemplate = &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "sample-node-",
		},
		Spec: v1.NodeSpec{
			// TODO: investigate why this is needed.
			ExternalID: "foo",
		},
		Status: v1.NodeStatus{
			Capacity: v1.ResourceList{
				v1.ResourcePods:   *resource.NewQuantity(10, resource.DecimalSI),
				v1.ResourceCPU:    resource.MustParse("4"),
				v1.ResourceMemory: resource.MustParse("3Gi"),
			},
			Allocatable: v1.ResourceList{
				v1.ResourcePods:   *resource.NewQuantity(10, resource.DecimalSI),
				v1.ResourceCPU:    resource.MustParse("4"),
				v1.ResourceMemory: resource.MustParse("3Gi"),
			},
			Phase: v1.NodeRunning,
			Conditions: []v1.NodeCondition{
				{Type: v1.NodeReady, Status: v1.ConditionTrue},
			},
		},
	}
)

// mustSetupScheduler starts the following components:
// - k8s api server (a.k.a. master)
// - scheduler
// It returns scheduler config factory and destroyFunc which should be used to
// remove resources after finished.
// Notes on rate limiter:
//   - client rate limit is set to 5000.
func mustSetupScheduler() (schedulerConfigurator scheduler.Configurator, destroyFunc func()) {
	h := &framework.MasterHolder{Initialized: make(chan struct{})}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		<-h.Initialized
		h.M.GenericAPIServer.Handler.ServeHTTP(w, req)
	}))

	framework.RunAMasterUsingServer(framework.NewIntegrationTestMasterConfig(), s, h)

	clientSet := clientset.NewForConfigOrDie(&restclient.Config{
		Host:          s.URL,
		ContentConfig: restclient.ContentConfig{GroupVersion: &api.Registry.GroupOrDie(v1.GroupName).GroupVersion},
		QPS:           5000.0,
		Burst:         5000,
	})

	informerFactory := informers.NewSharedInformerFactory(clientSet, 0)

	schedulerConfigurator = factory.NewConfigFactory(
		v1.DefaultSchedulerName,
		clientSet,
		informerFactory.Core().V1().Nodes(),
		informerFactory.Core().V1().Pods(),
		informerFactory.Core().V1().PersistentVolumes(),
		informerFactory.Core().V1().PersistentVolumeClaims(),
		informerFactory.Core().V1().ReplicationControllers(),
		informerFactory.Extensions().V1beta1().ReplicaSets(),
		informerFactory.Apps().V1beta1().StatefulSets(),
		informerFactory.Core().V1().Services(),
		v1.DefaultHardPodAffinitySymmetricWeight,
	)

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&clientv1core.EventSinkImpl{Interface: clientv1core.New(clientSet.Core().RESTClient()).Events("")})

	sched, err := scheduler.NewFromConfigurator(schedulerConfigurator, func(conf *scheduler.Config) {
		conf.Recorder = eventBroadcaster.NewRecorder(api.Scheme, clientv1.EventSource{Component: "scheduler"})
	})
	if err != nil {
		glog.Fatalf("Error creating scheduler: %v", err)
	}

	stop := make(chan struct{})
	informerFactory.Start(stop)

	sched.Run()

	destroyFunc = func() {
		glog.Infof("destroying")
		sched.StopEverything()
		close(stop)
		// Closing active connections from HTTP server as descheduler is not having a destroy object to invoke.
		s.CloseClientConnections()
		s.Close()
		time.Sleep(5 * time.Second)
		glog.Infof("destroyed")
	}
	return
}

func generatePods(config scheduler.Configurator, podCount int) {
	for i := 0; i < podCount; i++ {
		basePodTemplate.ObjectMeta.Annotations = map[string]string{v1.CreatedByAnnotation: RefJSON(basePodTemplate)}
		config.GetClient().Core().Pods("test").Create(basePodTemplate)

	}
}

func generateNodes(config scheduler.Configurator, nodeCount int) {
	for i := 0; i < nodeCount; i++ {
		config.GetClient().Core().Nodes().Create(baseNodeTemplate)
	}
}

// startEndToEndForLowNodeUtilization tests the lownode utilization strategy.
func startEndToEndForLowNodeUtilization(schedulerConfigFactory scheduler.Configurator) {
	// Create another node.
	generateNodes(schedulerConfigFactory, 1)

	var thresholds = make(deschedulerapi.ResourceThresholds)
	var targetThresholds = make(deschedulerapi.ResourceThresholds)
	thresholds[v1.ResourceMemory] = 30
	thresholds[v1.ResourcePods] = 30
	targetThresholds[v1.ResourceMemory] = 40
	targetThresholds[v1.ResourcePods] = 40
	// Run descheduler.
	evictionPolicyGroupVersion, err := eutils.SupportEviction(schedulerConfigFactory.GetClient())
	if err != nil || len(evictionPolicyGroupVersion) == 0 {
		glog.Fatalf("%v", err)
	}
	stopChannel := make(chan struct{})
	nodes, err := nodeutil.ReadyNodes(schedulerConfigFactory.GetClient(), stopChannel)
	if err != nil {
		glog.Fatalf("%v", err)
	}
	nodeUtilizationThresholds := deschedulerapi.NodeResourceUtilizationThresholds{Thresholds: thresholds, TargetThresholds: thresholds}
	nodeUtilizationStrategyParams := deschedulerapi.StrategyParameters{NodeResourceUtilizationThresholds: nodeUtilizationThresholds}
	lowNodeUtilizationStrategy := deschedulerapi.DeschedulerStrategy{Enabled: true, Params: nodeUtilizationStrategyParams}
	ds := &options.DeschedulerServer{Client: schedulerConfigFactory.GetClient()}
	strategies.LowNodeUtilization(ds, lowNodeUtilizationStrategy, evictionPolicyGroupVersion, nodes)
	// Delete nodes and pods.
	schedulerConfigFactory.GetClient().Core().Nodes().DeleteCollection(nil, metav1.ListOptions{})
	schedulerConfigFactory.GetClient().Core().Namespaces().DeleteCollection(nil, metav1.ListOptions{})
	return
}

func TestE2E(t *testing.T) {
	schedulerConfigFactory, destroyFunc := mustSetupScheduler()
	defer destroyFunc()
	// Create Nodes and Pods
	generatePods(schedulerConfigFactory, 10)
	generateNodes(schedulerConfigFactory, 2)
	var scheduled []*v1.Pod
	var err error
	// Wait till all the pods gets scheduled onto nodes.
	for {
		time.Sleep(50 * time.Millisecond)
		scheduled, err = schedulerConfigFactory.GetScheduledPodLister().List(labels.Everything())
		if err != nil {
			glog.Fatalf("%v", err)
		}
		if len(scheduled) == 10 {
			break
		}
	}
	startEndToEndForLowNodeUtilization(schedulerConfigFactory)
}


func RefJSON(o runtime.Object) string {
	ref, err := refv1.GetReference(api.Scheme, o)
	if err != nil {
		panic(err)
	}
	codec := testapi.Default.Codec()
	json := runtime.EncodeOrDie(codec, &v1.SerializedReference{Reference: *ref})
	return string(json)
}
