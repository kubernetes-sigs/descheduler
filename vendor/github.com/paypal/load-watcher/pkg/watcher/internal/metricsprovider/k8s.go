/*
Copyright 2020 PayPal

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

package metricsprovider

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/paypal/load-watcher/pkg/watcher"
	log "github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

var (
	kubeConfigPresent = false
	kubeConfigPath    string
)

const (
	// env variable that provides path to kube config file, if deploying from outside K8s cluster
	kubeConfig = "KUBE_CONFIG"
)

func init() {
	var ok bool
	kubeConfigPath, ok = os.LookupEnv(kubeConfig)
	if ok {
		kubeConfigPresent = true
	}
}

// This is a client for K8s provided Metric Server
type metricsServerClient struct {
	// This client fetches node metrics from metric server
	metricsClientSet *metricsv.Clientset
	// This client fetches node capacity
	coreClientSet *kubernetes.Clientset
}

func NewMetricsServerClient() (watcher.MetricsProviderClient, error) {
	var config *rest.Config
	var err error
	kubeConfig := ""
	if kubeConfigPresent {
		kubeConfig = kubeConfigPath
	}
	config, err = clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, err
	}

	metricsClientSet, err := metricsv.NewForConfig(config)
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return metricsServerClient{
		metricsClientSet: metricsClientSet,
		coreClientSet:    clientSet}, nil
}

func (m metricsServerClient) Name() string {
	return watcher.K8sClientName
}

func (m metricsServerClient) FetchHostMetrics(host string, window *watcher.Window) ([]watcher.Metric, error) {
	var metrics = []watcher.Metric{}

	nodeMetrics, err := m.metricsClientSet.MetricsV1beta1().NodeMetricses().Get(context.TODO(), host, metav1.GetOptions{})
	if err != nil {
		return metrics, err
	}
	var cpuFetchedMetric watcher.Metric
	var memFetchedMetric watcher.Metric
	node, err := m.coreClientSet.CoreV1().Nodes().Get(context.Background(), host, metav1.GetOptions{})
	if err != nil {
		return metrics, err
	}

	// Added CPU latest metric
	cpuFetchedMetric.Value = float64(100*nodeMetrics.Usage.Cpu().MilliValue()) / float64(node.Status.Capacity.Cpu().MilliValue())
	cpuFetchedMetric.Type = watcher.CPU
	cpuFetchedMetric.Operator = watcher.Latest
	metrics = append(metrics, cpuFetchedMetric)

	// Added Memory latest metric
	memFetchedMetric.Value = float64(100*nodeMetrics.Usage.Memory().Value()) / float64(node.Status.Capacity.Memory().Value())
	memFetchedMetric.Type = watcher.Memory
	memFetchedMetric.Operator = watcher.Latest
	metrics = append(metrics, memFetchedMetric)
	return metrics, nil
}

func (m metricsServerClient) FetchAllHostsMetrics(window *watcher.Window) (map[string][]watcher.Metric, error) {
	metrics := make(map[string][]watcher.Metric)

	nodeMetricsList, err := m.metricsClientSet.MetricsV1beta1().NodeMetricses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return metrics, err
	}
	nodeList, err := m.coreClientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return metrics, err
	}

	cpuNodeCapacityMap := make(map[string]int64)
	memNodeCPUCapacityMap := make(map[string]int64)
	for _, host := range nodeList.Items {
		cpuNodeCapacityMap[host.Name] = host.Status.Capacity.Cpu().MilliValue()
		memNodeCPUCapacityMap[host.Name] = host.Status.Capacity.Memory().Value()
	}
	for _, host := range nodeMetricsList.Items {
		var cpuFetchedMetric watcher.Metric
		cpuFetchedMetric.Type = watcher.CPU
		cpuFetchedMetric.Operator = watcher.Latest
		if _, ok := cpuNodeCapacityMap[host.Name]; !ok {
			log.Errorf("unable to find host %v in node list caching cpu capacity", host.Name)
			continue
		}

		cpuFetchedMetric.Value = float64(100*host.Usage.Cpu().MilliValue()) / float64(cpuNodeCapacityMap[host.Name])
		metrics[host.Name] = append(metrics[host.Name], cpuFetchedMetric)

		var memFetchedMetric watcher.Metric
		memFetchedMetric.Type = watcher.Memory
		memFetchedMetric.Operator = watcher.Latest
		if _, ok := memNodeCPUCapacityMap[host.Name]; !ok {
			log.Errorf("unable to find host %v in node list caching memory capacity", host.Name)
			continue
		}
		memFetchedMetric.Value = float64(100*host.Usage.Memory().Value()) / float64(memNodeCPUCapacityMap[host.Name])
		metrics[host.Name] = append(metrics[host.Name], memFetchedMetric)
	}

	return metrics, nil
}

func (m metricsServerClient) Health() (int, error) {
	var status int
	m.metricsClientSet.RESTClient().Verb("HEAD").Do(context.Background()).StatusCode(&status)
	if status != http.StatusOK {
		return -1, fmt.Errorf("received response status code: %v", status)
	}
	return 0, nil
}
