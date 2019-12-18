/*
Copyright 2018 The Kubernetes Authors.

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

package componentconfigs

import (
	"github.com/pkg/errors"
	"k8s.io/klog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/version"
	clientset "k8s.io/client-go/kubernetes"
	kubeproxyconfigv1alpha1 "k8s.io/kube-proxy/config/v1alpha1"
	kubeletconfigv1beta1 "k8s.io/kubelet/config/v1beta1"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/apiclient"
)

// GetFromKubeletConfigMap returns the pointer to the ComponentConfig API object read from the kubelet-config-version
// ConfigMap map stored in the cluster
func GetFromKubeletConfigMap(client clientset.Interface, version *version.Version) (runtime.Object, error) {

	// Read the ConfigMap from the cluster based on what version the kubelet is
	configMapName := kubeadmconstants.GetKubeletConfigMapName(version)
	kubeletCfg, err := apiclient.GetConfigMapWithRetry(client, metav1.NamespaceSystem, configMapName)
	if err != nil {
		return nil, err
	}

	kubeletConfigData, ok := kubeletCfg.Data[kubeadmconstants.KubeletBaseConfigurationConfigMapKey]
	if !ok {
		return nil, errors.Errorf("unexpected error when reading %s ConfigMap: %s key value pair missing",
			configMapName, kubeadmconstants.KubeletBaseConfigurationConfigMapKey)
	}

	// Decodes the kubeletConfigData into the internal component config
	obj := &kubeletconfigv1beta1.KubeletConfiguration{}
	err = unmarshalObject(obj, []byte(kubeletConfigData))
	if err != nil {
		return nil, err
	}

	return obj, nil
}

// GetFromKubeProxyConfigMap returns the pointer to the ComponentConfig API object read from the kube-proxy
// ConfigMap map stored in the cluster
func GetFromKubeProxyConfigMap(client clientset.Interface, version *version.Version) (runtime.Object, error) {

	// Read the ConfigMap from the cluster
	kubeproxyCfg, err := apiclient.GetConfigMapWithRetry(client, metav1.NamespaceSystem, kubeadmconstants.KubeProxyConfigMap)
	if err != nil {
		// The Kube-Proxy config map may be non-existent, because the user has decided to manage it by themselves
		// or to use other proxy solution. It may also be forbidden - if the kube-proxy phase was skipped we have neither
		// the config map, nor the RBAC rules allowing join access to it.
		if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
			klog.Warningf("Warning: No kube-proxy config is loaded. Continuing without it: %v", err)
			return nil, nil
		}
		return nil, err
	}

	kubeproxyConfigData, ok := kubeproxyCfg.Data[kubeadmconstants.KubeProxyConfigMapKey]
	if !ok {
		return nil, errors.Errorf("unexpected error when reading %s ConfigMap: %s key value pair missing",
			kubeadmconstants.KubeProxyConfigMap, kubeadmconstants.KubeProxyConfigMapKey)
	}

	// Decodes the Config map dat into the internal component config
	obj := &kubeproxyconfigv1alpha1.KubeProxyConfiguration{}
	err = unmarshalObject(obj, []byte(kubeproxyConfigData))
	if err != nil {
		return nil, err
	}

	return obj, nil
}
