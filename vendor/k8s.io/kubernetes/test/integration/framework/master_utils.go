/*
Copyright 2014 The Kubernetes Authors.

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

package framework

import (
	"net"
	"net/http"
	"net/http/httptest"
	"path"
	goruntime "runtime"
	"strconv"
	"sync"
	"time"

	"github.com/go-openapi/spec"
	"github.com/golang/glog"
	"github.com/pborman/uuid"

	apps "k8s.io/api/apps/v1beta1"
	autoscaling "k8s.io/api/autoscaling/v1"
	certificates "k8s.io/api/certificates/v1beta1"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	rbac "k8s.io/api/rbac/v1alpha1"
	storage "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	authauthenticator "k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/authenticatorfactory"
	authenticatorunion "k8s.io/apiserver/pkg/authentication/request/union"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	authauthorizer "k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
	authorizerunion "k8s.io/apiserver/pkg/authorization/union"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/options"
	serverstorage "k8s.io/apiserver/pkg/server/storage"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/client-go/informers"
	extinformers "k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"k8s.io/kubernetes/pkg/api/testapi"
	"k8s.io/kubernetes/pkg/apis/batch"
	policy "k8s.io/kubernetes/pkg/apis/policy/v1beta1"
	"k8s.io/kubernetes/pkg/controller"
	replicationcontroller "k8s.io/kubernetes/pkg/controller/replication"
	"k8s.io/kubernetes/pkg/generated/openapi"
	kubeletclient "k8s.io/kubernetes/pkg/kubelet/client"
	"k8s.io/kubernetes/pkg/master"
	"k8s.io/kubernetes/pkg/version"
	"k8s.io/kubernetes/plugin/pkg/admission/admit"
)

const (
	// Timeout used in benchmarks, to eg: scale an rc
	DefaultTimeout = 30 * time.Minute

	// Rc manifest used to create pods for benchmarks.
	// TODO: Convert this to a full path?
	TestRCManifest = "benchmark-controller.json"
)

// MasterComponents is a control struct for all master components started via NewMasterComponents.
// TODO: Include all master components (scheduler, nodecontroller).
// TODO: Reconcile with integration.go, currently the master used there doesn't understand
// how to restart cleanly, which is required for each iteration of a benchmark. The integration
// tests also don't make it easy to isolate and turn off components at will.
type MasterComponents struct {
	// Raw http server in front of the master
	ApiServer *httptest.Server
	// Kubernetes master, contains an embedded etcd storage
	KubeMaster *master.Master
	// Restclient used to talk to the kubernetes master
	ClientSet clientset.Interface
	// Replication controller manager
	ControllerManager *replicationcontroller.ReplicationManager
	// CloseFn shuts down the server
	CloseFn CloseFunc
	// Channel for stop signals to rc manager
	rcStopCh chan struct{}
	// Used to stop master components individually, and via MasterComponents.Stop
	once sync.Once
}

// Config is a struct of configuration directives for NewMasterComponents.
type Config struct {
	// If nil, a default is used, partially filled configs will not get populated.
	MasterConfig            *master.Config
	StartReplicationManager bool
	// Client throttling qps
	QPS float32
	// Client burst qps, also burst replicas allowed in rc manager
	Burst int
	// TODO: Add configs for endpoints controller, scheduler etc
}

// NewMasterComponents creates, initializes and starts master components based on the given config.
func NewMasterComponents(c *Config) *MasterComponents {
	m, s, closeFn := startMasterOrDie(c.MasterConfig, nil, nil)
	// TODO: Allow callers to pipe through a different master url and create a client/start components using it.
	glog.Infof("Master %+v", s.URL)
	// TODO: caesarxuchao: remove this client when the refactoring of client libraray is done.
	clientset := clientset.NewForConfigOrDie(&restclient.Config{Host: s.URL, ContentConfig: restclient.ContentConfig{GroupVersion: testapi.Groups[v1.GroupName].GroupVersion()}, QPS: c.QPS, Burst: c.Burst})
	rcStopCh := make(chan struct{})
	informerFactory := informers.NewSharedInformerFactory(clientset, controller.NoResyncPeriodFunc())
	controllerManager := replicationcontroller.NewReplicationManager(informerFactory.Core().V1().Pods(), informerFactory.Core().V1().ReplicationControllers(), clientset, c.Burst)

	// TODO: Support events once we can cleanly shutdown an event recorder.
	controllerManager.SetEventRecorder(&record.FakeRecorder{})
	if c.StartReplicationManager {
		informerFactory.Start(rcStopCh)
		go controllerManager.Run(goruntime.NumCPU(), rcStopCh)
	}
	return &MasterComponents{
		ApiServer:         s,
		KubeMaster:        m,
		ClientSet:         clientset,
		ControllerManager: controllerManager,
		CloseFn:           closeFn,
		rcStopCh:          rcStopCh,
	}
}

// alwaysAllow always allows an action
type alwaysAllow struct{}

func (alwaysAllow) Authorize(requestAttributes authauthorizer.Attributes) (authorizer.Decision, string, error) {
	return authorizer.DecisionAllow, "always allow", nil
}

// alwaysEmpty simulates "no authentication" for old tests
func alwaysEmpty(req *http.Request) (user.Info, bool, error) {
	return &user.DefaultInfo{
		Name: "",
	}, true, nil
}

// MasterReceiver can be used to provide the master to a custom incoming server function
type MasterReceiver interface {
	SetMaster(m *master.Master)
}

// MasterHolder implements
type MasterHolder struct {
	Initialized chan struct{}
	M           *master.Master
}

func (h *MasterHolder) SetMaster(m *master.Master) {
	h.M = m
	close(h.Initialized)
}

// startMasterOrDie starts a kubernetes master and an httpserver to handle api requests
func startMasterOrDie(masterConfig *master.Config, incomingServer *httptest.Server, masterReceiver MasterReceiver) (*master.Master, *httptest.Server, CloseFunc) {
	var m *master.Master
	var s *httptest.Server

	if incomingServer != nil {
		s = incomingServer
	} else {
		s = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			m.GenericAPIServer.Handler.ServeHTTP(w, req)
		}))
	}

	stopCh := make(chan struct{})
	closeFn := func() {
		close(stopCh)
		s.Close()
	}

	if masterConfig == nil {
		masterConfig = NewMasterConfig()
		masterConfig.GenericConfig.EnableProfiling = true
		masterConfig.GenericConfig.EnableMetrics = true
		masterConfig.GenericConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(openapi.GetOpenAPIDefinitions, legacyscheme.Scheme)
		masterConfig.GenericConfig.OpenAPIConfig.Info = &spec.Info{
			InfoProps: spec.InfoProps{
				Title:   "Kubernetes",
				Version: "unversioned",
			},
		}
		masterConfig.GenericConfig.OpenAPIConfig.DefaultResponse = &spec.Response{
			ResponseProps: spec.ResponseProps{
				Description: "Default Response.",
			},
		}
		masterConfig.GenericConfig.OpenAPIConfig.GetDefinitions = openapi.GetOpenAPIDefinitions
		masterConfig.GenericConfig.SwaggerConfig = genericapiserver.DefaultSwaggerConfig()
	}

	// set the loopback client config
	if masterConfig.GenericConfig.LoopbackClientConfig == nil {
		masterConfig.GenericConfig.LoopbackClientConfig = &restclient.Config{QPS: 50, Burst: 100, ContentConfig: restclient.ContentConfig{NegotiatedSerializer: legacyscheme.Codecs}}
	}
	masterConfig.GenericConfig.LoopbackClientConfig.Host = s.URL

	privilegedLoopbackToken := uuid.NewRandom().String()
	// wrap any available authorizer
	tokens := make(map[string]*user.DefaultInfo)
	tokens[privilegedLoopbackToken] = &user.DefaultInfo{
		Name:   user.APIServerUser,
		UID:    uuid.NewRandom().String(),
		Groups: []string{user.SystemPrivilegedGroup},
	}

	tokenAuthenticator := authenticatorfactory.NewFromTokens(tokens)
	if masterConfig.GenericConfig.Authenticator == nil {
		masterConfig.GenericConfig.Authenticator = authenticatorunion.New(tokenAuthenticator, authauthenticator.RequestFunc(alwaysEmpty))
	} else {
		masterConfig.GenericConfig.Authenticator = authenticatorunion.New(tokenAuthenticator, masterConfig.GenericConfig.Authenticator)
	}

	if masterConfig.GenericConfig.Authorizer != nil {
		tokenAuthorizer := authorizerfactory.NewPrivilegedGroups(user.SystemPrivilegedGroup)
		masterConfig.GenericConfig.Authorizer = authorizerunion.New(tokenAuthorizer, masterConfig.GenericConfig.Authorizer)
	} else {
		masterConfig.GenericConfig.Authorizer = alwaysAllow{}
	}

	masterConfig.GenericConfig.LoopbackClientConfig.BearerToken = privilegedLoopbackToken

	clientset, err := clientset.NewForConfig(masterConfig.GenericConfig.LoopbackClientConfig)
	if err != nil {
		glog.Fatal(err)
	}

	sharedInformers := extinformers.NewSharedInformerFactory(clientset, masterConfig.GenericConfig.LoopbackClientConfig.Timeout)
	m, err = masterConfig.Complete(sharedInformers).New(genericapiserver.EmptyDelegate)
	if err != nil {
		closeFn()
		glog.Fatalf("error in bringing up the master: %v", err)
	}
	if masterReceiver != nil {
		masterReceiver.SetMaster(m)
	}

	// TODO have this start method actually use the normal start sequence for the API server
	// this method never actually calls the `Run` method for the API server
	// fire the post hooks ourselves
	m.GenericAPIServer.PrepareRun()
	m.GenericAPIServer.RunPostStartHooks(stopCh)

	cfg := *masterConfig.GenericConfig.LoopbackClientConfig
	cfg.ContentConfig.GroupVersion = &schema.GroupVersion{}
	privilegedClient, err := restclient.RESTClientFor(&cfg)
	if err != nil {
		closeFn()
		glog.Fatal(err)
	}
	err = wait.PollImmediate(100*time.Millisecond, 30*time.Second, func() (bool, error) {
		result := privilegedClient.Get().AbsPath("/healthz").Do()
		status := 0
		result.StatusCode(&status)
		if status == 200 {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		closeFn()
		glog.Fatal(err)
	}

	return m, s, closeFn
}

// Returns a basic master config.
func NewMasterConfig() *master.Config {
	// This causes the integration tests to exercise the etcd
	// prefix code, so please don't change without ensuring
	// sufficient coverage in other ways.
	etcdOptions := options.NewEtcdOptions(storagebackend.NewDefaultConfig(uuid.New(), nil))
	etcdOptions.StorageConfig.ServerList = []string{GetEtcdURL()}

	info, _ := runtime.SerializerInfoForMediaType(legacyscheme.Codecs.SupportedMediaTypes(), runtime.ContentTypeJSON)
	ns := NewSingleContentTypeSerializer(legacyscheme.Scheme, info)

	resourceEncoding := serverstorage.NewDefaultResourceEncodingConfig(legacyscheme.Registry)
	// FIXME (soltysh): this GroupVersionResource override should be configurable
	// we need to set both for the whole group and for cronjobs, separately
	resourceEncoding.SetVersionEncoding(batch.GroupName, *testapi.Batch.GroupVersion(), schema.GroupVersion{Group: batch.GroupName, Version: runtime.APIVersionInternal})
	resourceEncoding.SetResourceEncoding(schema.GroupResource{Group: "batch", Resource: "cronjobs"}, schema.GroupVersion{Group: batch.GroupName, Version: "v1beta1"}, schema.GroupVersion{Group: batch.GroupName, Version: runtime.APIVersionInternal})

	storageFactory := serverstorage.NewDefaultStorageFactory(etcdOptions.StorageConfig, runtime.ContentTypeJSON, ns, resourceEncoding, master.DefaultAPIResourceConfigSource(), nil)
	storageFactory.SetSerializer(
		schema.GroupResource{Group: v1.GroupName, Resource: serverstorage.AllResources},
		"",
		ns)
	storageFactory.SetSerializer(
		schema.GroupResource{Group: autoscaling.GroupName, Resource: serverstorage.AllResources},
		"",
		ns)
	storageFactory.SetSerializer(
		schema.GroupResource{Group: batch.GroupName, Resource: serverstorage.AllResources},
		"",
		ns)
	storageFactory.SetSerializer(
		schema.GroupResource{Group: apps.GroupName, Resource: serverstorage.AllResources},
		"",
		ns)
	storageFactory.SetSerializer(
		schema.GroupResource{Group: extensions.GroupName, Resource: serverstorage.AllResources},
		"",
		ns)
	storageFactory.SetSerializer(
		schema.GroupResource{Group: policy.GroupName, Resource: serverstorage.AllResources},
		"",
		ns)
	storageFactory.SetSerializer(
		schema.GroupResource{Group: rbac.GroupName, Resource: serverstorage.AllResources},
		"",
		ns)
	storageFactory.SetSerializer(
		schema.GroupResource{Group: certificates.GroupName, Resource: serverstorage.AllResources},
		"",
		ns)
	storageFactory.SetSerializer(
		schema.GroupResource{Group: storage.GroupName, Resource: serverstorage.AllResources},
		"",
		ns)

	genericConfig := genericapiserver.NewConfig(legacyscheme.Codecs)
	kubeVersion := version.Get()
	genericConfig.Version = &kubeVersion
	genericConfig.Authorizer = authorizerfactory.NewAlwaysAllowAuthorizer()
	genericConfig.AdmissionControl = admit.NewAlwaysAdmit()
	genericConfig.EnableMetrics = true

	err := etcdOptions.ApplyWithStorageFactoryTo(storageFactory, genericConfig)
	if err != nil {
		panic(err)
	}

	return &master.Config{
		GenericConfig: genericConfig,
		ExtraConfig: master.ExtraConfig{
			APIResourceConfigSource: master.DefaultAPIResourceConfigSource(),
			StorageFactory:          storageFactory,
			EnableCoreControllers:   true,
			KubeletClientConfig:     kubeletclient.KubeletClientConfig{Port: 10250},
			APIServerServicePort:    443,
			MasterCount:             1,
		},
	}
}

// Returns the master config appropriate for most integration tests.
func NewIntegrationTestMasterConfig() *master.Config {
	masterConfig := NewMasterConfig()
	masterConfig.ExtraConfig.EnableCoreControllers = true
	masterConfig.GenericConfig.PublicAddress = net.ParseIP("192.168.10.4")
	masterConfig.ExtraConfig.APIResourceConfigSource = master.DefaultAPIResourceConfigSource()
	return masterConfig
}

func (m *MasterComponents) stopRCManager() {
	close(m.rcStopCh)
}

func (m *MasterComponents) Stop(apiServer, rcManager bool) {
	glog.Infof("Stopping master components")
	if rcManager {
		// Ordering matters because the apiServer will only shutdown when pending
		// requests are done
		m.once.Do(m.stopRCManager)
	}
	if apiServer {
		m.CloseFn()
	}
}

// CloseFunc can be called to cleanup the master
type CloseFunc func()

func RunAMaster(masterConfig *master.Config) (*master.Master, *httptest.Server, CloseFunc) {
	if masterConfig == nil {
		masterConfig = NewMasterConfig()
		masterConfig.GenericConfig.EnableProfiling = true
		masterConfig.GenericConfig.EnableMetrics = true
	}
	return startMasterOrDie(masterConfig, nil, nil)
}

func RunAMasterUsingServer(masterConfig *master.Config, s *httptest.Server, masterReceiver MasterReceiver) (*master.Master, *httptest.Server, CloseFunc) {
	return startMasterOrDie(masterConfig, s, masterReceiver)
}

// Task is a function passed to worker goroutines by RunParallel.
// The function needs to implement its own thread safety.
type Task func(id int) error

// RunParallel spawns a goroutine per task in the given queue
func RunParallel(task Task, numTasks, numWorkers int) {
	start := time.Now()
	if numWorkers <= 0 {
		numWorkers = numTasks
	}
	defer func() {
		glog.Infof("RunParallel took %v for %d tasks and %d workers", time.Since(start), numTasks, numWorkers)
	}()
	var wg sync.WaitGroup
	semCh := make(chan struct{}, numWorkers)
	wg.Add(numTasks)
	for id := 0; id < numTasks; id++ {
		go func(id int) {
			semCh <- struct{}{}
			err := task(id)
			if err != nil {
				glog.Fatalf("Worker failed with %v", err)
			}
			<-semCh
			wg.Done()
		}(id)
	}
	wg.Wait()
	close(semCh)
}

// FindFreeLocalPort returns the number of an available port number on
// the loopback interface.  Useful for determining the port to launch
// a server on.  Error handling required - there is a non-zero chance
// that the returned port number will be bound by another process
// after this function returns.
func FindFreeLocalPort() (int, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	_, portStr, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, err
	}
	return port, nil
}

// SharedEtcd creates a storage config for a shared etcd instance, with a unique prefix.
func SharedEtcd() *storagebackend.Config {
	cfg := storagebackend.NewDefaultConfig(path.Join(uuid.New(), "registry"), nil)
	cfg.ServerList = []string{GetEtcdURL()}
	return cfg
}
