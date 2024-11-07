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

package client

import (
	"fmt"

	// Ensure to load all auth plugins.
	clientset "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	componentbaseconfig "k8s.io/component-base/config"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

func createConfig(clientConnection componentbaseconfig.ClientConnectionConfiguration, userAgt string) (*rest.Config, error) {
	var cfg *rest.Config
	if len(clientConnection.Kubeconfig) != 0 {
		master, err := GetMasterFromKubeconfig(clientConnection.Kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to parse kubeconfig file: %v ", err)
		}

		cfg, err = clientcmd.BuildConfigFromFlags(master, clientConnection.Kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("unable to build config: %v", err)
		}

	} else {
		var err error
		cfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("unable to build in cluster config: %v", err)
		}
	}

	cfg.Burst = int(clientConnection.Burst)
	cfg.QPS = clientConnection.QPS

	if len(userAgt) != 0 {
		cfg = rest.AddUserAgent(cfg, userAgt)
	}

	return cfg, nil
}

func CreateClient(clientConnection componentbaseconfig.ClientConnectionConfiguration, userAgt string) (clientset.Interface, error) {
	cfg, err := createConfig(clientConnection, userAgt)
	if err != nil {
		return nil, fmt.Errorf("unable to create config: %v", err)
	}

	return clientset.NewForConfig(cfg)
}

func CreateMetricsClient(clientConnection componentbaseconfig.ClientConnectionConfiguration, userAgt string) (metricsclient.Interface, error) {
	cfg, err := createConfig(clientConnection, userAgt)
	if err != nil {
		return nil, fmt.Errorf("unable to create config: %v", err)
	}

	// Create the metrics clientset to access the metrics.k8s.io API
	return metricsclient.NewForConfig(cfg)
}

func GetMasterFromKubeconfig(filename string) (string, error) {
	config, err := clientcmd.LoadFromFile(filename)
	if err != nil {
		return "", err
	}

	context, ok := config.Contexts[config.CurrentContext]
	if !ok {
		return "", fmt.Errorf("failed to get master address from kubeconfig: current context not found")
	}

	if val, ok := config.Clusters[context.Cluster]; ok {
		return val.Server, nil
	}
	return "", fmt.Errorf("failed to get master address from kubeconfig: cluster information not found")
}
