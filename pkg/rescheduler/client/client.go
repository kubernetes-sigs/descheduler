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

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"

	"github.com/aveshagarwal/rescheduler/pkg/utils"
)

func CreateClient(kubeconfig string) (clientset.Interface, error) {
	var cfg *rest.Config
	if len(kubeconfig) != 0 {
		master, err := utils.GetMasterFromKubeconfig(kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse kubeconfig file: %v ", err)
		}

		cfg, err = clientcmd.BuildConfigFromFlags(master, kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to build config: %v", err)
		}

	} else {
		var err error
		cfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("Unable to build in cluster config: %v", err)
		}
	}

	return clientset.NewForConfig(cfg)
}
