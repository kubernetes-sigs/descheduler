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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	"github.com/prometheus/common/config"

	// Ensure to load all auth plugins.
	clientset "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/transport"
	componentbaseconfig "k8s.io/component-base/config"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

var K8sPodCAFilePath = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

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

func loadCAFile(filepath string) (*x509.CertPool, error) {
	caCert, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("failed to append CA certificate to the pool")
	}

	return caCertPool, nil
}

func CreatePrometheusClient(prometheusURL, authToken string, insecureSkipVerify bool) (promapi.Client, error) {
	// Ignore TLS verify errors if InsecureSkipVerify is set
	roundTripper := promapi.DefaultRoundTripper
	if insecureSkipVerify {
		roundTripper = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		}
	} else {
		// Retrieve Pod CA cert
		caCertPool, err := loadCAFile(K8sPodCAFilePath)
		if err != nil {
			return nil, fmt.Errorf("Error loading CA file: %v", err)
		}

		// Get Prometheus Host
		u, err := url.Parse(prometheusURL)
		if err != nil {
			return nil, fmt.Errorf("Error parsing prometheus URL: %v", err)
		}
		roundTripper = transport.NewBearerAuthRoundTripper(
			authToken,
			&http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout: 10 * time.Second,
				TLSClientConfig: &tls.Config{
					RootCAs:    caCertPool,
					ServerName: u.Host,
				},
			},
		)
	}

	if authToken != "" {
		return promapi.NewClient(promapi.Config{
			Address:      prometheusURL,
			RoundTripper: config.NewAuthorizationCredentialsRoundTripper("Bearer", config.NewInlineSecret(authToken), roundTripper),
		})
	}
	return promapi.NewClient(promapi.Config{
		Address: prometheusURL,
	})
}
