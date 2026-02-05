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
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	promapi "github.com/prometheus/client_golang/api"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"sigs.k8s.io/descheduler/pkg/api"
	"sigs.k8s.io/descheduler/pkg/descheduler/client"
)

const (
	prometheusAuthTokenSecretKey = "prometheusAuthToken"
	workQueueKey                 = "key"
)

// inClusterPromClientController manages prometheus client using in-cluster SA token
type inClusterPromClientController struct {
	mu                                sync.RWMutex
	promClient                        promapi.Client
	previousPrometheusClientTransport *http.Transport
	currentPrometheusAuthToken        string
	prometheusConfig                  *api.Prometheus
	createPrometheusClient            createPrometheusClientFunc
	inClusterConfig                   inClusterConfigFunc
}

// secretBasedPromClientController manages prometheus client using Kubernetes secret
type secretBasedPromClientController struct {
	mu                                sync.RWMutex
	promClient                        promapi.Client
	previousPrometheusClientTransport *http.Transport
	queue                             workqueue.RateLimitingInterface
	currentPrometheusAuthToken        string
	namespacedSecretsLister           corev1listers.SecretNamespaceLister
	prometheusConfig                  *api.Prometheus
	createPrometheusClient            createPrometheusClientFunc
}

type (
	createPrometheusClientFunc func(url, token string) (promapi.Client, *http.Transport, error)
	inClusterConfigFunc        func() (*rest.Config, error)
)

func newInClusterPromClientController(prometheusClient promapi.Client, prometheusConfig *api.Prometheus) *inClusterPromClientController {
	return &inClusterPromClientController{
		promClient:             prometheusClient,
		prometheusConfig:       prometheusConfig,
		createPrometheusClient: client.CreatePrometheusClient,
		inClusterConfig:        rest.InClusterConfig,
	}
}

func newSecretBasedPromClientController(prometheusClient promapi.Client, prometheusConfig *api.Prometheus, namespacedSharedInformerFactory informers.SharedInformerFactory) (*secretBasedPromClientController, error) {
	if prometheusConfig == nil || prometheusConfig.AuthToken == nil || prometheusConfig.AuthToken.SecretReference == nil {
		return nil, fmt.Errorf("prometheus metrics source configuration is missing authentication token secret")
	}
	authTokenSecret := prometheusConfig.AuthToken.SecretReference
	if authTokenSecret.Name == "" || authTokenSecret.Namespace == "" {
		return nil, fmt.Errorf("prometheus metrics source configuration is missing authentication token secret")
	}

	if namespacedSharedInformerFactory == nil {
		return nil, fmt.Errorf("namespacedSharedInformerFactory not configured")
	}

	ctrl := &secretBasedPromClientController{
		promClient:             prometheusClient,
		queue:                  workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(), workqueue.RateLimitingQueueConfig{Name: "descheduler"}),
		prometheusConfig:       prometheusConfig,
		createPrometheusClient: client.CreatePrometheusClient,
	}

	namespacedSharedInformerFactory.Core().V1().Secrets().Informer().AddEventHandler(ctrl.eventHandler())
	ctrl.namespacedSecretsLister = namespacedSharedInformerFactory.Core().V1().Secrets().Lister().Secrets(authTokenSecret.Namespace)

	return ctrl, nil
}

func (d *inClusterPromClientController) prometheusClient() promapi.Client {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.promClient
}

func (d *secretBasedPromClientController) prometheusClient() promapi.Client {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.promClient
}

func (d *inClusterPromClientController) reconcileInClusterSAToken() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Read the sa token and assume it has the sufficient permissions to authenticate
	cfg, err := d.inClusterConfig()
	if err == nil {
		if d.currentPrometheusAuthToken != cfg.BearerToken {
			klog.V(2).Infof("Creating Prometheus client (with SA token)")
			prometheusClient, transport, err := d.createPrometheusClient(d.prometheusConfig.URL, cfg.BearerToken)
			if err != nil {
				d.clearConnection()
				return fmt.Errorf("unable to create a prometheus client: %v", err)
			}
			d.promClient = prometheusClient
			if d.previousPrometheusClientTransport != nil {
				d.previousPrometheusClientTransport.CloseIdleConnections()
			}
			d.previousPrometheusClientTransport = transport
			d.currentPrometheusAuthToken = cfg.BearerToken
		}
		return nil
	}
	if err == rest.ErrNotInCluster {
		return nil
	}
	return fmt.Errorf("unexpected error when reading in cluster config: %v", err)
}

func clearPromClientConnection(currentPrometheusAuthToken *string, previousPrometheusClientTransport **http.Transport, promClient *promapi.Client) {
	*currentPrometheusAuthToken = ""
	if *previousPrometheusClientTransport != nil {
		(*previousPrometheusClientTransport).CloseIdleConnections()
	}
	*previousPrometheusClientTransport = nil
	*promClient = nil
}

func (d *inClusterPromClientController) clearConnection() {
	clearPromClientConnection(&d.currentPrometheusAuthToken, &d.previousPrometheusClientTransport, &d.promClient)
}

func (d *secretBasedPromClientController) clearConnection() {
	clearPromClientConnection(&d.currentPrometheusAuthToken, &d.previousPrometheusClientTransport, &d.promClient)
}

func (d *secretBasedPromClientController) runAuthenticationSecretReconciler(ctx context.Context) {
	defer utilruntime.HandleCrash()
	defer d.queue.ShutDown()

	klog.Infof("Starting authentication secret reconciler")
	defer klog.Infof("Shutting down authentication secret reconciler")

	go wait.UntilWithContext(ctx, d.runAuthenticationSecretReconcilerWorker, time.Second)

	<-ctx.Done()
}

func (d *secretBasedPromClientController) runAuthenticationSecretReconcilerWorker(ctx context.Context) {
	for d.processNextWorkItem(ctx) {
	}
}

func (d *secretBasedPromClientController) processNextWorkItem(ctx context.Context) bool {
	dsKey, quit := d.queue.Get()
	if quit {
		return false
	}
	defer d.queue.Done(dsKey)

	err := d.sync()
	if err == nil {
		d.queue.Forget(dsKey)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("%v failed with : %v", dsKey, err))
	d.queue.AddRateLimited(dsKey)

	return true
}

func (d *secretBasedPromClientController) sync() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	prometheusConfig := d.prometheusConfig
	ns := prometheusConfig.AuthToken.SecretReference.Namespace
	name := prometheusConfig.AuthToken.SecretReference.Name
	secretObj, err := d.namespacedSecretsLister.Get(name)
	if err != nil {
		// clear the token if the secret is not found
		if apierrors.IsNotFound(err) {
			d.clearConnection()
		}
		return fmt.Errorf("unable to get %v/%v secret", ns, name)
	}
	authToken := string(secretObj.Data[prometheusAuthTokenSecretKey])
	if authToken == "" {
		d.clearConnection()
		return fmt.Errorf("prometheus authentication token secret missing %q data or empty", prometheusAuthTokenSecretKey)
	}
	if d.currentPrometheusAuthToken == authToken {
		return nil
	}

	klog.V(2).Infof("authentication secret token updated, recreating prometheus client")
	prometheusClient, transport, err := d.createPrometheusClient(prometheusConfig.URL, authToken)
	if err != nil {
		d.clearConnection()
		return fmt.Errorf("unable to create a prometheus client: %v", err)
	}
	d.promClient = prometheusClient
	if d.previousPrometheusClientTransport != nil {
		d.previousPrometheusClientTransport.CloseIdleConnections()
	}
	d.previousPrometheusClientTransport = transport
	d.currentPrometheusAuthToken = authToken
	return nil
}

func (d *secretBasedPromClientController) eventHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { d.queue.Add(workQueueKey) },
		UpdateFunc: func(old, new interface{}) { d.queue.Add(workQueueKey) },
		DeleteFunc: func(obj interface{}) { d.queue.Add(workQueueKey) },
	}
}
