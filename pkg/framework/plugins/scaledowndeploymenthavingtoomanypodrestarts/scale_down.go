/*
Copyright 2022 The Kubernetes Authors.

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

package scaledowndeploymenthavingtoomanypodrestarts

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	podutil "sigs.k8s.io/descheduler/pkg/descheduler/pod"
	frameworktypes "sigs.k8s.io/descheduler/pkg/framework/types"
)

const PluginName = "ScaleDownDeploymentHavingTooManyPodRestarts"

// ScaleDownDeploymentHavingTooManyPodRestarts scales down a deployment to zero when its pods have too many restarts.
// This strategy prevents pods with a high number of restarts from attempting further restarts.
// Unlike 'RemovePodsHavingTooManyRestarts', this strategy is useful when changing to a different node will not solve the root cause of the failure.
// This is particularly useful for saving resources when an application is stuck in an endless startup loop.
// It's also useful in development environments where application failures are common occurrences.
type ScaleDownDeploymentHavingTooManyPodRestarts struct {
	handle    frameworktypes.Handle
	args      *ScaleDownDeploymentHavingTooManyPodRestartsArgs
	podFilter podutil.FilterFunc
}

var _ frameworktypes.DeschedulePlugin = &ScaleDownDeploymentHavingTooManyPodRestarts{}

// New builds plugin from its arguments while passing a handle
func New(args runtime.Object, handle frameworktypes.Handle) (frameworktypes.Plugin, error) {
	pluginArgs, ok := args.(*ScaleDownDeploymentHavingTooManyPodRestartsArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type ScaleDownDeploymentHavingTooManyPodRestartsArgs, got %T", args)
	}

	var includedNamespaces, excludedNamespaces sets.String
	if pluginArgs.Namespaces != nil {
		includedNamespaces = sets.NewString(pluginArgs.Namespaces.Include...)
		excludedNamespaces = sets.NewString(pluginArgs.Namespaces.Exclude...)
	}

	// We can combine Filter and PreEvictionFilter since for this strategy it does not matter where we run PreEvictionFilter
	podFilter, err := podutil.NewOptions().
		WithFilter(podutil.WrapFilterFuncs(handle.Evictor().Filter, handle.Evictor().PreEvictionFilter)).
		WithNamespaces(includedNamespaces).
		WithoutNamespaces(excludedNamespaces).
		WithLabelSelector(pluginArgs.LabelSelector).
		BuildFilterFunc()
	if err != nil {
		return nil, fmt.Errorf("error initializing pod filter function: %v", err)
	}

	podFilter = podutil.WrapFilterFuncs(podFilter, func(pod *v1.Pod) bool {
		if err := validateCanScaleDown(handle.ClientSet(), pod, pluginArgs); err != nil {
			klog.V(4).InfoS("ignore the pod", "pod", klog.KObj(pod), "error", err)
			return false
		}
		return true
	})

	return &ScaleDownDeploymentHavingTooManyPodRestarts{
		handle:    handle,
		args:      pluginArgs,
		podFilter: podFilter,
	}, nil
}

// Name retrieves the plugin name
func (d *ScaleDownDeploymentHavingTooManyPodRestarts) Name() string {
	return PluginName
}

// Deschedule extension point implementation for the plugin
func (d *ScaleDownDeploymentHavingTooManyPodRestarts) Deschedule(ctx context.Context, nodes []*v1.Node) *frameworktypes.Status {
	// deploymentStat is a map of deployment namespaced name to the number of pods that belong to the deployment and restart over the threshold.
	deploymentStat := map[string]int32{}

	for _, node := range nodes {
		klog.V(2).InfoS("Processing node", "node", klog.KObj(node))
		pods, err := podutil.ListAllPodsOnANode(node.Name, d.handle.GetPodsAssignedToNodeFunc(), d.podFilter)
		if err != nil {
			return &frameworktypes.Status{
				Err: fmt.Errorf("error listing pods on a node: %v", err),
			}
		}

		for i := 0; i < len(pods); i++ {
			deployment, err := getPodDeployment(d.handle.ClientSet(), pods[i])
			if err != nil {
				klog.V(1).InfoS("warning: failed to get the deployment for pod, skip it", "pod", klog.KObj(pods[i]), "error", err)
				continue
			}

			klog.V(4).InfoS("Processing the deployment", "namespace", deployment.Name, "name", deployment.Name)

			key := deployment.Namespace + deployment.Name
			deploymentStat[key]++
			if deploymentStat[key] >= *d.args.ReplicasThreshold {
				if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
					// Get the latest version of deployment before attempting update
					deploy, err := d.handle.ClientSet().AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
					if err != nil {
						return err
					}

					if deploy.Spec.Replicas != nil && *deploy.Spec.Replicas > 0 {
						deploy.Spec.Replicas = pointer.Int32(0)
						_, err := d.handle.ClientSet().AppsV1().Deployments(deployment.Namespace).Update(ctx, deploy, metav1.UpdateOptions{})
						return err
					}

					klog.V(4).InfoS("scale down the deployment to zero", "namespace", deployment.Name, "name", deployment.Name)
					return nil
				}); err != nil {
					klog.ErrorS(err, "failed to scale down the deployment", "deployment", klog.KObj(deployment))
				}
			}
		}
	}

	return nil
}

// validateCanScaleDown checks if the pod has a deployment owner and calculates the number of restarts to see if the deployment can be scaled down.
func validateCanScaleDown(clientSet kubernetes.Interface, pod *v1.Pod, args *ScaleDownDeploymentHavingTooManyPodRestartsArgs) error {
	var err error

	_, err = getPodDeployment(clientSet, pod)
	if err != nil {
		return fmt.Errorf("failed to get the deployment for pod: %v", err)
	}

	restarts := calcPodRestarts(pod, args.IncludingInitContainers)
	if restarts < args.PodRestartThreshold {
		err = fmt.Errorf("number of container restarts (%v) not exceeding the threshold", restarts)
	}

	return err
}

// calcContainerRestartsFromStatuses get container restarts from container statuses.
func calcPodRestarts(pod *v1.Pod, includingInitContainers bool) int32 {
	restarts := calcContainerRestartsFromStatuses(pod.Status.ContainerStatuses)

	if includingInitContainers {
		restarts += calcContainerRestartsFromStatuses(pod.Status.InitContainerStatuses)
	}

	return restarts
}

func calcContainerRestartsFromStatuses(statuses []v1.ContainerStatus) int32 {
	var restarts int32
	for _, cs := range statuses {
		restarts += cs.RestartCount
	}
	return restarts
}

func getPodDeployment(cli kubernetes.Interface, pod *v1.Pod) (deploy *appsv1.Deployment, err error) {
	var rs *appsv1.ReplicaSet

	for _, ownerRef := range pod.GetOwnerReferences() {
		if ownerRef.Kind == "ReplicaSet" {
			rs, err = cli.AppsV1().ReplicaSets(pod.Namespace).Get(context.TODO(), ownerRef.Name, metav1.GetOptions{})
			break
		}
	}

	if rs == nil || err != nil {
		return nil, fmt.Errorf("unable to get the ReplicaSet for pod %s: %v", pod.Name, err)
	}

	for _, ownerRef := range rs.GetOwnerReferences() {
		if ownerRef.Kind == "Deployment" {
			return cli.AppsV1().Deployments(pod.Namespace).Get(context.TODO(), ownerRef.Name, metav1.GetOptions{})
		}
	}

	return nil, fmt.Errorf("cannot find the Deployment for pod %s", pod.Name)
}
